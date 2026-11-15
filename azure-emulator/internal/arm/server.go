package arm

import (
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/activitylog"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/asyncop"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rbac"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// Routes for paths that are not provider-namespaced. They are filled in by
// the Microsoft.Resources RP package.
type Routes struct {
	Tenants            Handler // /tenants
	Subscriptions      Handler // /subscriptions
	Subscription       Handler // /subscriptions/{s}
	Locations          Handler // /subscriptions/{s}/locations
	Providers          Handler // /providers and /subscriptions/{s}/providers
	Provider           Handler // .../providers/{ns}  (+ /register, /unregister)
	ResourceGroups     Handler // /subscriptions/{s}/resourcegroups
	ResourceGroup      Handler // /subscriptions/{s}/resourcegroups/{rg}
	Resources          Handler // .../resources (sub or rg)
	OperationResults   Handler // /subscriptions/{s}/operationresults/{id}
	ResourceGroupScope func(sub, rg string) (canonical string, ok bool)
}

// Server is the ARM front end.
type Server struct {
	Cfg      config.Config
	Store    store.Store
	Auth     *auth.Service
	Dir      *entra.Directory
	H        *hierarchy.Store
	RBAC     *rbac.Engine
	Ops      *asyncop.Manager
	Activity *activitylog.Log
	Registry *Registry
	Routes   Routes

	// WriteMu serialises mutating requests; the emulator favours
	// determinism over write throughput.
	WriteMu sync.Mutex

	// Hooks run before the handler for mutating requests (policy, locks,
	// chaos). Returning false aborts the request (the hook wrote a response).
	PreWrite []func(*Request) bool
	cleanup  map[string]func(map[string]any)
}

var apiVersionRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(-[a-zA-Z0-9.]+)?$`)

// ServeHTTP routes an ARM request.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := &Request{S: s, W: w, R: r, Seg: hierarchy.Segments(r.URL.Path), RequestID: w.Header().Get("x-ms-request-id")}
	if q.RequestID == "" {
		q.RequestID = entra.NewGUID()
	}
	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Pragma", "no-cache")
	h.Set("x-ms-request-id", q.RequestID)
	corr := r.Header.Get("x-ms-correlation-request-id")
	if corr == "" {
		corr = q.RequestID
	}
	h.Set("x-ms-correlation-request-id", corr)
	h.Set("x-ms-routing-request-id", "CLOUDTYKE:"+q.RequestID)
	h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		h.Set("x-ms-ratelimit-remaining-subscription-reads", "11999")
	} else {
		h.Set("x-ms-ratelimit-remaining-subscription-writes", "1199")
	}

	if len(q.Seg) == 0 {
		q.Error(404, "NotFound", "No route registered for '/'.")
		return
	}

	q.APIVersion = r.URL.Query().Get("api-version")
	if q.APIVersion == "" {
		q.Error(400, "MissingApiVersionParameter", "The api-version query parameter (?api-version=) is required for all requests.")
		return
	}
	if !apiVersionRe.MatchString(q.APIVersion) {
		q.Error(400, "InvalidApiVersionParameter", Errorf("The api-version '%s' is invalid. The supported versions are '%s'.", q.APIVersion, "yyyy-mm-dd[-preview]"))
		return
	}

	id, code, msg := s.Auth.Authenticate(r, s.Cfg.Host(config.SubManagement))
	if id == nil {
		h.Set("WWW-Authenticate", `Bearer authorization_uri="`+s.Cfg.URL(config.SubLogin)+`/common", error="invalid_token", error_description="`+msg+`"`)
		q.Error(401, code, msg)
		return
	}
	q.Identity = id
	r = r.WithContext(auth.WithIdentity(r.Context(), id))
	q.R = r

	rec := &statusRecorder{ResponseWriter: w, status: 200}
	q.W = rec
	mutating := r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete || r.Method == http.MethodPost
	if mutating {
		s.WriteMu.Lock()
		defer s.WriteMu.Unlock()
	}
	s.route(q)
	if mutating && q.Action != "" && rec.status < 500 && rec.wrote {
		s.record(q, rec.status, corr)
	}
}

func (s *Server) record(q *Request, status int, corr string) {
	scope := q.Scope
	ns, typ := "", ""
	if q.ID.Namespace != "" {
		ns = q.ID.Namespace
		typ = q.ID.Namespace + "/" + strings.Join(q.ID.Types, "/")
	} else if parts := strings.Split(q.Action, "/"); len(parts) > 1 {
		ns = parts[0]
		typ = strings.Join(parts[:len(parts)-1], "/")
	}
	sub := ScopeSubscription(scope)
	if sub == "" {
		return
	}
	s.Activity.Record(activitylog.Input{
		Action: q.Action, Caller: q.Identity.Caller(), CorrelationID: corr,
		ClientRequestID: q.R.Header.Get("x-ms-client-request-id"), ClientIP: q.R.RemoteAddr, Method: q.R.Method,
		ResourceID: scope, ResourceGroup: ScopeResourceGroup(scope), Namespace: ns, ResourceType: typ,
		SubscriptionID: sub, TenantID: q.Identity.TenantID, RequestID: q.RequestID, StatusCode: status,
	})
}

// Authorize checks RBAC for action at scope and writes AuthorizationFailed
// when denied. It records action/scope on the request.
func (q *Request) Authorize(action, scope string) bool {
	q.Action, q.Scope = action, scope
	if q.S.RBAC.Allowed(q.Identity, action, scope, false) {
		return true
	}
	q.Error(403, "AuthorizationFailed", Errorf("The client '%s' with object id '%s' does not have authorization to perform action '%s' over scope '%s' or the scope is invalid. If access was recently granted, please refresh your credentials.", q.Identity.Caller(), q.Identity.ObjectID, action, scope))
	return false
}

// verb maps an HTTP method to an RBAC verb.
func verb(method string) string {
	switch method {
	case http.MethodPut, http.MethodPatch:
		return "write"
	case http.MethodDelete:
		return "delete"
	case http.MethodPost:
		return "action"
	}
	return "read"
}

func eq(a, b string) bool { return strings.EqualFold(a, b) }

func (s *Server) route(q *Request) {
	seg := q.Seg
	n := len(seg)
	rt := s.Routes
	m := q.R.Method

	// tenant-level, non-provider paths
	switch {
	case n == 1 && eq(seg[0], "tenants"):
		rt.Tenants(q)
		return
	case n == 1 && eq(seg[0], "subscriptions"):
		rt.Subscriptions(q)
		return
	case n == 1 && eq(seg[0], "providers"):
		rt.Providers(q)
		return
	case n == 2 && eq(seg[0], "providers"):
		rt.Provider(q)
		return
	}
	if !eq(seg[0], "subscriptions") && !eq(seg[0], "providers") {
		q.Error(404, "InvalidResourceType", Errorf("The resource type '%s' could not be found in the namespace 'Microsoft.Resources' for api version '%s'.", seg[0], q.APIVersion))
		return
	}

	if eq(seg[0], "subscriptions") {
		sub, ok := s.H.GetSubscription(seg[1])
		if !ok {
			q.Error(404, "SubscriptionNotFound", Errorf("The subscription '%s' could not be found.", seg[1]))
			return
		}
		if !strings.EqualFold(sub.TenantID, q.Identity.TenantID) {
			q.Error(401, "InvalidAuthenticationTokenTenant", Errorf("The access token is from the wrong issuer '%s'. It must match the tenant '%s/%s/' associated with this subscription. Please use the authority (URL) '%s/%s' to get the token.", q.Identity.Claims["iss"], s.Auth.Issuer(sub.TenantID), "", s.Cfg.URL(config.SubLogin), sub.TenantID))
			return
		}
		switch {
		case n == 2:
			rt.Subscription(q)
			return
		case n == 3 && eq(seg[2], "locations"):
			rt.Locations(q)
			return
		case n == 3 && eq(seg[2], "providers"):
			rt.Providers(q)
			return
		case n == 4 && eq(seg[2], "providers"):
			rt.Provider(q)
			return
		case n == 5 && eq(seg[2], "providers") && (eq(seg[4], "register") || eq(seg[4], "unregister")) && m == http.MethodPost:
			rt.Provider(q)
			return
		case n == 4 && eq(seg[2], "operationresults"):
			rt.OperationResults(q)
			return
		case n == 3 && eq(seg[2], "resources"):
			rt.Resources(q)
			return
		case n == 3 && eq(seg[2], "resourceGroups"):
			rt.ResourceGroups(q)
			return
		case n == 4 && eq(seg[2], "resourceGroups"):
			rt.ResourceGroup(q)
			return
		case n == 5 && eq(seg[2], "resourceGroups") && eq(seg[4], "resources"):
			rt.Resources(q)
			return
		}
	}

	id, ok := ParseProviderPath(seg)
	if !ok || len(id.Types) == 0 {
		q.Error(404, "InvalidResourceType", Errorf("The resource path '%s' is not a valid Azure Resource Manager path.", q.R.URL.Path))
		return
	}
	q.ID = id

	// operation status polling: .../providers/{ns}/locations/{loc}/operationStatuses|operationResults|asyncOperations/{id}
	if len(id.Types) == 2 && eq(id.Types[0], "locations") && len(id.Names) == 2 &&
		(eq(id.Types[1], "operationStatuses") || eq(id.Types[1], "operationResults") || eq(id.Types[1], "asyncOperations") || eq(id.Types[1], "operations")) {
		s.operationStatus(q, eq(id.Types[1], "operationResults"))
		return
	}

	// POST action on a resource: odd number of type/name segments
	action := ""
	if m == http.MethodPost && id.IsCollection() && len(id.Types) > 1 {
		action = id.Types[len(id.Types)-1]
		id.Types = id.Types[:len(id.Types)-1]
		q.ID = id
	}

	reg, ok := s.Registry.Lookup(id.TypeKey())
	if !ok {
		q.Error(501, "NotImplemented", Errorf("CloudTyke does not emulate resource type '%s/%s' (api-version '%s') yet. See the coverage matrix: %s", id.Namespace, strings.Join(id.Types, "/"), q.APIVersion, s.Cfg.DocsURL))
		return
	}
	q.Reg = reg
	if s.Cfg.StrictAPIVersions && !reg.SupportsAPIVersion(q.APIVersion) {
		q.Error(400, "NoRegisteredProviderFound", Errorf("No registered resource provider found for location '%s' and API version '%s' for type '%s'. The supported api-versions are '%s'.", "global", q.APIVersion, strings.Join(reg.Types, "/"), strings.Join(reg.APIVersions, ", ")))
		return
	}

	// scope validation
	if !s.validateScope(q, id.Scope) {
		return
	}

	// RBAC
	target := id.Path(reg.Namespace, reg.Types)
	scope := target
	if id.IsCollection() {
		scope = id.ParentPath(reg.Namespace, reg.Types)
		if scope == "" {
			scope = "/"
		}
	}
	act := reg.FullType() + "/" + verb(m)
	if action != "" {
		act = reg.FullType() + "/" + action + "/action"
	}
	if !q.Authorize(act, scope) {
		return
	}
	if action != "" {
		q.R.Header.Set("X-CloudTyke-Action", action)
	}
	if m != http.MethodGet && m != http.MethodHead {
		for _, hook := range s.PreWrite {
			if !hook(q) {
				return
			}
		}
	}
	reg.Handler(q)
}

// ActionName returns the POST action name for action requests.
func (q *Request) ActionName() string { return q.R.Header.Get("X-CloudTyke-Action") }

// validateScope checks that the scope a provider resource lives in exists.
func (s *Server) validateScope(q *Request, scope string) bool {
	seg := hierarchy.Segments(scope)
	if len(seg) == 4 && eq(seg[0], "providers") && eq(seg[1], "Microsoft.Management") && eq(seg[2], "managementGroups") {
		if _, ok := s.H.GetManagementGroup(seg[3]); !ok {
			q.Error(404, "ManagementGroupNotFound", Errorf("The management group '%s' could not be found.", seg[3]))
			return false
		}
		return true
	}
	if len(seg) >= 4 && eq(seg[0], "subscriptions") && eq(seg[2], "resourceGroups") {
		if _, ok := s.Routes.ResourceGroupScope(seg[1], seg[3]); !ok {
			q.Error(404, "ResourceGroupNotFound", Errorf("Resource group '%s' could not be found.", seg[3]))
			return false
		}
	}
	// extension resource on a resource: the parent resource must exist
	if hierarchy.LastProviders(seg) >= 0 {
		if _, ok, _ := s.Store.Get(store.BucketARM, StoreKey(scope)); !ok {
			pid, _ := ParseProviderPath(seg)
			q.Error(404, "ResourceNotFound", Errorf("The Resource '%s/%s' under resource group '%s' was not found. For more details please go to https://aka.ms/ARMResourceNotFoundFix", pid.Namespace, strings.Join(zipPath(pid), "/"), ScopeResourceGroup(scope)))
			return false
		}
	}
	return true
}

func zipPath(id ResourceID) []string {
	var out []string
	for i, t := range id.Types {
		out = append(out, t)
		if i < len(id.Names) {
			out = append(out, id.Names[i])
		}
	}
	return out
}

func (s *Server) operationStatus(q *Request, isResult bool) {
	op, ok := s.Ops.Get(q.ID.Names[1])
	if !ok {
		q.Error(404, "OperationNotFound", Errorf("The operation '%s' could not be found.", q.ID.Names[1]))
		return
	}
	if isResult {
		if !op.Done() {
			q.W.Header().Set("Location", q.URL(q.R.URL.Path, q.R.URL.Query()))
			q.W.Header().Set("Retry-After", "1")
			q.Empty(202)
			return
		}
		if op.Status != "Succeeded" {
			q.JSON(200, map[string]any{"error": op.Error})
			return
		}
		q.Empty(204)
		return
	}
	if !op.Done() {
		q.W.Header().Set("Retry-After", "1")
	}
	q.JSON(200, op.StatusBody())
}

// AsyncHeaders sets Azure-AsyncOperation / Location headers for op.
func (q *Request) AsyncHeaders(op asyncop.Operation, ns, location string) {
	if location == "" {
		location = "global"
	}
	sub := op.SubscriptionID
	base := "/subscriptions/" + sub + "/providers/" + ns + "/locations/" + NormalizeLocation(location)
	qs := map[string][]string{"api-version": {q.APIVersion}}
	q.W.Header().Set("Azure-AsyncOperation", q.URL(base+"/operationStatuses/"+op.ID, qs))
	q.W.Header().Set("Location", q.URL(base+"/operationResults/"+op.ID, qs))
	q.W.Header().Set("Retry-After", "1")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wrote {
		r.status, r.wrote = 200, true
	}
	return r.ResponseWriter.Write(b)
}
