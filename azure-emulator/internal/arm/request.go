// Package arm is the Azure Resource Manager front end: request routing,
// api-version handling, authentication, RBAC enforcement, ARM error
// format, paging, ETags, async operations and the resource-provider
// registry that individual RPs plug into.
package arm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
)

// Request is an in-flight ARM request.
type Request struct {
	S          *Server
	W          http.ResponseWriter
	R          *http.Request
	Seg        []string
	APIVersion string
	Identity   *auth.Identity
	ID         ResourceID // populated for provider-namespaced routes
	Reg        *TypeRegistration
	RequestID  string

	// Action is the RBAC action authorised for this request; it is also
	// used for the activity log.
	Action string
	// Scope is the RBAC scope of the request.
	Scope string

	body    map[string]any
	bodyErr error
	parsed  bool
}

// Body parses the JSON request body (once). An empty body yields an empty map.
func (q *Request) Body() (map[string]any, error) {
	if q.parsed {
		return q.body, q.bodyErr
	}
	q.parsed = true
	b, err := io.ReadAll(io.LimitReader(q.R.Body, 8<<20))
	if err != nil {
		q.bodyErr = err
		return nil, err
	}
	q.body = map[string]any{}
	if len(strings.TrimSpace(string(b))) == 0 {
		return q.body, nil
	}
	if err := json.Unmarshal(b, &q.body); err != nil {
		q.bodyErr = err
		return nil, err
	}
	return q.body, nil
}

// BodyOrError parses the body and writes InvalidRequestContent on failure.
func (q *Request) BodyOrError() (map[string]any, bool) {
	b, err := q.Body()
	if err != nil {
		q.Error(400, "InvalidRequestContent", "The request content was invalid and could not be deserialized: '"+err.Error()+"'.")
		return nil, false
	}
	return b, true
}

// JSON writes a JSON response.
func (q *Request) JSON(status int, v any) {
	q.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	q.W.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(q.W).Encode(v)
	}
}

// Empty writes a response with no body.
func (q *Request) Empty(status int) {
	q.W.Header().Set("Content-Length", "0")
	q.W.WriteHeader(status)
}

// Error writes an ARM error response.
func (q *Request) Error(status int, code, message string, details ...map[string]any) {
	WriteError(q.W, status, code, message, details...)
}

// WriteError writes {"error":{"code","message","details"}}.
func WriteError(w http.ResponseWriter, status int, code, message string, details ...map[string]any) {
	e := map[string]any{"code": code, "message": message}
	if len(details) > 0 {
		e["details"] = details
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("x-ms-failure-cause", "gateway")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": e})
}

// SubscriptionID returns the subscription in the request path, if any.
func (q *Request) SubscriptionID() string {
	if len(q.Seg) >= 2 && strings.EqualFold(q.Seg[0], "subscriptions") {
		return q.Seg[1]
	}
	return ""
}

// ResourceGroupName returns the resource group in the path, if any.
func (q *Request) ResourceGroupName() string {
	if len(q.Seg) >= 4 && strings.EqualFold(q.Seg[0], "subscriptions") && strings.EqualFold(q.Seg[2], "resourceGroups") {
		return q.Seg[3]
	}
	return ""
}

// URL builds an absolute management URL for path with the given query.
func (q *Request) URL(path string, query url.Values) string {
	u := q.S.Cfg.URL("management") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

// List writes a paged list response {"value": [...], "nextLink": ...}.
// Items are sorted by id for stable paging.
func (q *Request) List(items []map[string]any) {
	sort.SliceStable(items, func(i, j int) bool {
		a, _ := items[i]["id"].(string)
		b, _ := items[j]["id"].(string)
		return strings.ToLower(a) < strings.ToLower(b)
	})
	page := q.S.Cfg.PageSize
	if t, err := strconv.Atoi(q.R.URL.Query().Get("$top")); err == nil && t > 0 && t < page {
		page = t
	}
	start := 0
	if s, err := strconv.Atoi(q.R.URL.Query().Get("$skiptoken")); err == nil && s > 0 {
		start = s
	}
	if start > len(items) {
		start = len(items)
	}
	end := start + page
	if end > len(items) {
		end = len(items)
	}
	out := map[string]any{"value": nonNil(items[start:end])}
	if end < len(items) {
		qs := q.R.URL.Query()
		qs.Set("$skiptoken", strconv.Itoa(end))
		out["nextLink"] = q.URL(q.R.URL.Path, qs)
	}
	q.JSON(200, out)
}

func nonNil(v []map[string]any) []map[string]any {
	if v == nil {
		return []map[string]any{}
	}
	return v
}

// SystemData returns ARM systemData for a write by the current caller.
func (q *Request) SystemData(existing map[string]any) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	caller, ctype := q.Identity.Caller(), q.Identity.PrincipalType()
	sd := map[string]any{"createdBy": caller, "createdByType": ctype, "createdAt": now}
	if old, ok := existing["systemData"].(map[string]any); ok {
		for _, k := range []string{"createdBy", "createdByType", "createdAt"} {
			if v, ok := old[k]; ok {
				sd[k] = v
			}
		}
	}
	sd["lastModifiedBy"], sd["lastModifiedByType"], sd["lastModifiedAt"] = caller, ctype, now
	return sd
}

// NewETag returns a fresh ETag value in ARM's W/"guid" style.
func NewETag() string { return `W/"` + entra.NewGUID() + `"` }

// ------------------------------------------------------------------ IDs

// ResourceID is a parsed provider-namespaced ARM path.
type ResourceID struct {
	Scope     string   // path before the last /providers/ ("" == tenant root)
	Namespace string   // e.g. Microsoft.Network
	Types     []string // e.g. [virtualNetworks subnets]
	Names     []string // len == len(Types) for an instance, len-1 for a collection
}

// ParseProviderPath parses segments that contain a provider namespace.
// It returns ok=false if no /providers/{ns} pair exists.
func ParseProviderPath(seg []string) (ResourceID, bool) {
	p := hierarchy.LastProviders(seg)
	if p < 0 {
		return ResourceID{}, false
	}
	id := ResourceID{Namespace: seg[p+1]}
	if p > 0 {
		id.Scope = "/" + strings.Join(seg[:p], "/")
	}
	rest := seg[p+2:]
	for i := 0; i < len(rest); i += 2 {
		id.Types = append(id.Types, rest[i])
		if i+1 < len(rest) {
			id.Names = append(id.Names, rest[i+1])
		}
	}
	return id, true
}

// IsCollection reports whether the path addresses a collection.
func (id ResourceID) IsCollection() bool { return len(id.Names) < len(id.Types) }

// TypeKey is the lower-cased "namespace/type/subtype" registry key.
func (id ResourceID) TypeKey() string {
	return strings.ToLower(id.Namespace + "/" + strings.Join(id.Types, "/"))
}

// Name returns the leaf resource name ("" for collections).
func (id ResourceID) Name() string {
	if id.IsCollection() || len(id.Names) == 0 {
		return ""
	}
	return id.Names[len(id.Names)-1]
}

// Path renders the ID (or collection path) using the given canonical casing
// for namespace and types.
func (id ResourceID) Path(ns string, types []string) string {
	var b strings.Builder
	b.WriteString(id.Scope)
	b.WriteString("/providers/")
	b.WriteString(ns)
	for i, t := range types {
		b.WriteString("/")
		b.WriteString(t)
		if i < len(id.Names) {
			b.WriteString("/")
			b.WriteString(id.Names[i])
		}
	}
	return b.String()
}

// ParentPath returns the canonical path of the parent resource for child
// resources, or the scope for top-level resources.
func (id ResourceID) ParentPath(ns string, types []string) string {
	if len(types) <= 1 {
		return id.Scope
	}
	p := ResourceID{Scope: id.Scope, Namespace: id.Namespace, Types: id.Types[:len(id.Types)-1], Names: id.Names[:min(len(id.Names), len(id.Types)-1)]}
	return p.Path(ns, types[:len(types)-1])
}

// StoreKey returns the store key for a resource path.
func StoreKey(path string) string { return strings.ToLower(strings.TrimSuffix(path, "/")) }

// ScopeSubscription extracts a subscription id from a scope path.
func ScopeSubscription(scope string) string {
	seg := hierarchy.Segments(scope)
	if len(seg) >= 2 && strings.EqualFold(seg[0], "subscriptions") {
		return seg[1]
	}
	return ""
}

// ScopeResourceGroup extracts a resource group name from a scope path.
func ScopeResourceGroup(scope string) string {
	seg := hierarchy.Segments(scope)
	if len(seg) >= 4 && strings.EqualFold(seg[0], "subscriptions") && strings.EqualFold(seg[2], "resourceGroups") {
		return seg[3]
	}
	return ""
}

// NormalizeLocation turns "West Europe" into "westeurope".
func NormalizeLocation(l string) string {
	return strings.ToLower(strings.ReplaceAll(l, " ", ""))
}

// Errorf is a convenience for building messages.
func Errorf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
