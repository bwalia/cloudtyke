package auth

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
)

// TokenLifetime is the lifetime of issued access tokens.
const TokenLifetime = time.Hour

// Identity is the authenticated caller of an API request.
type Identity struct {
	TenantID    string
	ObjectID    string
	AppID       string
	DisplayName string
	UPN         string
	Type        string   // "app" or "user"
	Roles       []string // app roles (e.g. Graph application permissions)
	Audience    string
	Claims      map[string]any
}

// Caller returns the string Azure puts in activity logs / systemData.
func (i *Identity) Caller() string {
	if i == nil {
		return ""
	}
	if i.UPN != "" {
		return i.UPN
	}
	if i.AppID != "" {
		return i.AppID
	}
	return i.ObjectID
}

// PrincipalType maps to ARM principalType / systemData createdByType.
func (i *Identity) PrincipalType() string {
	if i != nil && i.Type == "user" {
		return "User"
	}
	return "Application"
}

type ctxKey struct{}

// WithIdentity attaches an identity to a context.
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the identity, if any.
func FromContext(ctx context.Context) *Identity {
	id, _ := ctx.Value(ctxKey{}).(*Identity)
	return id
}

// Service is the emulated Entra token service.
type Service struct {
	Cfg    config.Config
	Signer *Signer
	Dir    *entra.Directory
	// extra signers (e.g. per-AKS-cluster OIDC issuers) by issuer URL
	issuerMu   sync.RWMutex
	issuerKeys map[string]*rsa.PublicKey
	httpClient *http.Client
	// IssuerJWKS returns the signing keys of an AKS cluster OIDC issuer
	// (ok=false when iss is not one).
	IssuerJWKS func(iss string) (keys []map[string]any, ok bool)
}

// NewService builds the token service.
func NewService(cfg config.Config, s *Signer, dir *entra.Directory) *Service {
	return &Service{Cfg: cfg, Signer: s, Dir: dir, issuerKeys: map[string]*rsa.PublicKey{}, httpClient: &http.Client{Timeout: 5 * time.Second}}
}

// RegisterIssuer lets other parts of the emulator (AKS OIDC issuers) declare
// keys used to sign federated assertions.
func (s *Service) RegisterIssuer(issuer string, key *rsa.PublicKey) {
	s.issuerMu.Lock()
	defer s.issuerMu.Unlock()
	s.issuerKeys[strings.TrimSuffix(issuer, "/")] = key
}

// Issuer returns the v1 issuer string for a tenant.
func (s *Service) Issuer(tid string) string { return "https://sts." + s.Cfg.Domain + "/" + tid + "/" }

// IssuerV2 returns the v2 issuer for a tenant.
func (s *Service) IssuerV2(tid string) string {
	return s.Cfg.URL(config.SubLogin) + "/" + tid + "/v2.0"
}

// ServeHTTP handles the login.<domain> host.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, 200, map[string]any{"service": "CloudTyke emulated Entra ID token service"})
		return
	}
	tenantSeg := parts[0]
	rest := strings.Join(parts[1:], "/")
	switch {
	case rest == "oauth2/v2.0/token" || rest == "oauth2/token":
		if r.Method != http.MethodPost {
			oauthError(w, 405, "invalid_request", "AADSTS900561: The endpoint only accepts POST requests.")
			return
		}
		s.token(w, r, tenantSeg, rest == "oauth2/token")
	case rest == "v2.0/.well-known/openid-configuration" || rest == ".well-known/openid-configuration":
		s.discovery(w, tenantSeg, strings.HasPrefix(rest, "v2.0"))
	case rest == "discovery/v2.0/keys" || rest == "discovery/keys":
		writeJSON(w, 200, s.Signer.JWKS())
	case rest == "discovery/instance":
		// MSAL instance discovery
		writeJSON(w, 200, map[string]any{
			"tenant_discovery_endpoint": s.Cfg.URL(config.SubLogin) + "/" + tenantSeg + "/v2.0/.well-known/openid-configuration",
			"api-version":               "1.1",
			"metadata":                  []any{map[string]any{"preferred_network": s.Cfg.Host(config.SubLogin), "preferred_cache": s.Cfg.Host(config.SubLogin), "aliases": []string{s.Cfg.Host(config.SubLogin)}}},
		})
	default:
		oauthError(w, 404, "invalid_request", "AADSTS90002: Unsupported endpoint '"+r.URL.Path+"' on the CloudTyke token service.")
	}
}

func (s *Service) discovery(w http.ResponseWriter, tenantSeg string, v2 bool) {
	base := s.Cfg.URL(config.SubLogin) + "/" + tenantSeg
	tid := tenantSeg
	if t, ok := s.Dir.ResolveTenant(tenantSeg); ok {
		tid = t.ID
	} else if tenantSeg == "common" || tenantSeg == "organizations" {
		tid = "{tenantid}"
	}
	issuer := s.Issuer(tid)
	tokenEP := base + "/oauth2/token"
	authEP := base + "/oauth2/authorize"
	if v2 {
		issuer = s.IssuerV2(tid)
		tokenEP = base + "/oauth2/v2.0/token"
		authEP = base + "/oauth2/v2.0/authorize"
	}
	writeJSON(w, 200, map[string]any{
		"issuer":                                issuer,
		"token_endpoint":                        tokenEP,
		"authorization_endpoint":                authEP,
		"jwks_uri":                              base + "/discovery/v2.0/keys",
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "private_key_jwt", "client_secret_basic"},
		"response_types_supported":              []string{"code", "id_token", "code id_token", "id_token token"},
		"subject_types_supported":               []string{"pairwise"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email", "offline_access"},
		"tenant_region_scope":                   "EU",
		"cloud_instance_name":                   s.Cfg.Domain,
		"cloud_graph_host_name":                 s.Cfg.Host(config.SubGraph),
		"msgraph_host":                          s.Cfg.Host(config.SubGraph),
	})
}

func (s *Service) token(w http.ResponseWriter, r *http.Request, tenantSeg string, v1 bool) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, 400, "invalid_request", "AADSTS900144: The request body must contain a valid form.")
		return
	}
	f := r.PostForm
	clientID := f.Get("client_id")
	clientSecret := f.Get("client_secret")
	if u, p, ok := r.BasicAuth(); ok && clientID == "" {
		clientID, _ = url.QueryUnescape(u)
		clientSecret, _ = url.QueryUnescape(p)
	}
	grant := f.Get("grant_type")
	if grant != "client_credentials" {
		oauthError(w, 400, "unsupported_grant_type", fmt.Sprintf("AADSTS70003: The app requested an unsupported grant type '%s'. CloudTyke supports client_credentials (client secret or federated client assertion).", grant))
		return
	}
	if clientID == "" {
		oauthError(w, 400, "invalid_request", "AADSTS900144: The request body must contain the following parameter: 'client_id'.")
		return
	}

	// resolve tenant
	var tenant entra.Tenant
	var ok bool
	if tenantSeg == "common" || tenantSeg == "organizations" {
		tenant, ok = s.Dir.TenantOfApp(clientID)
	} else {
		tenant, ok = s.Dir.ResolveTenant(tenantSeg)
	}
	if !ok {
		oauthError(w, 400, "invalid_request", fmt.Sprintf("AADSTS90002: Tenant '%s' not found. Check to make sure you have the correct tenant ID and are signing into the correct cloud.", tenantSeg))
		return
	}

	// resource / scope
	resource := f.Get("resource")
	if !v1 || resource == "" {
		scope := f.Get("scope")
		for _, sc := range strings.Fields(scope) {
			if strings.HasSuffix(sc, "/.default") {
				resource = strings.TrimSuffix(sc, "/.default")
				break
			}
		}
		if resource == "" {
			oauthError(w, 400, "invalid_scope", fmt.Sprintf("AADSTS1002012: The provided value for scope %s is not valid. Client credential flows must have a scope value with /.default suffixed to the resource identifier (application ID URI).", scope))
			return
		}
	}

	// authenticate the client
	app, found := s.Dir.FindBy(tenant.ID, entra.Applications, "appId", clientID)
	if !found {
		// managed identities have a service principal but no application;
		// their federated credentials (workload identity) live on the SP.
		if mi, ok := s.Dir.FindBy(tenant.ID, entra.ServicePrincipals, "appId", clientID); ok && mi["servicePrincipalType"] == "ManagedIdentity" {
			if f.Get("client_assertion") == "" {
				oauthError(w, 401, "invalid_client", fmt.Sprintf("AADSTS700027: Managed identity '%s' cannot use a client secret. Use a federated client assertion (workload identity).", clientID))
				return
			}
			app, found = mi, true
		}
	}
	if assertion := f.Get("client_assertion"); assertion != "" {
		if f.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
			oauthError(w, 400, "invalid_request", "AADSTS700016: client_assertion_type must be urn:ietf:params:oauth:client-assertion-type:jwt-bearer.")
			return
		}
		if !found {
			oauthError(w, 400, "unauthorized_client", fmt.Sprintf("AADSTS700016: Application with identifier '%s' was not found in the directory '%s'.", clientID, tenant.DisplayName))
			return
		}
		if err := s.verifyFederatedAssertion(app, assertion); err != nil {
			oauthError(w, 401, "invalid_client", err.Error())
			return
		}
	} else {
		if !found {
			oauthError(w, 400, "unauthorized_client", fmt.Sprintf("AADSTS700016: Application with identifier '%s' was not found in the directory '%s'. This can happen if the application has not been installed by the administrator of the tenant or consented to by any user in the tenant.", clientID, tenant.DisplayName))
			return
		}
		if clientSecret == "" {
			oauthError(w, 401, "invalid_client", "AADSTS7000216: 'client_assertion', 'client_secret' or 'request' is required for the 'client_credentials' grant type.")
			return
		}
		if _, ok := s.Dir.VerifyClientSecret(tenant.ID, clientID, clientSecret); !ok {
			oauthError(w, 401, "invalid_client", fmt.Sprintf("AADSTS7000215: Invalid client secret provided. Ensure the secret being sent in the request is the client secret value, not the client secret ID, for a secret added to app '%s'.", clientID))
			return
		}
	}
	sp, ok := s.Dir.FindBy(tenant.ID, entra.ServicePrincipals, "appId", clientID)
	if !ok {
		oauthError(w, 401, "unauthorized_client", fmt.Sprintf("AADSTS7000229: The client application %s is missing service principal in the tenant %s.", clientID, tenant.ID))
		return
	}
	if enabled, ok := sp["accountEnabled"].(bool); ok && !enabled {
		oauthError(w, 401, "unauthorized_client", "AADSTS7000112: Application is disabled.")
		return
	}

	tok, exp, err := s.IssueAppToken(tenant.ID, sp, resource)
	if err != nil {
		oauthError(w, 500, "server_error", err.Error())
		return
	}
	resp := map[string]any{
		"token_type":     "Bearer",
		"expires_in":     int(TokenLifetime.Seconds()) - 1,
		"ext_expires_in": int(TokenLifetime.Seconds()) - 1,
		"access_token":   tok,
	}
	if v1 {
		resp["expires_in"] = fmt.Sprint(int(TokenLifetime.Seconds()) - 1)
		resp["ext_expires_in"] = fmt.Sprint(int(TokenLifetime.Seconds()) - 1)
		resp["expires_on"] = fmt.Sprint(exp.Unix())
		resp["not_before"] = fmt.Sprint(time.Now().Unix())
		resp["resource"] = resource
	}
	w.Header().Set("Cache-Control", "no-store, no-cache")
	writeJSON(w, 200, resp)
}

// IssueAppToken mints an app-only access token for sp and resource.
func (s *Service) IssueAppToken(tid string, sp entra.Object, resource string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(TokenLifetime)
	oid, _ := sp["id"].(string)
	appID, _ := sp["appId"].(string)
	name, _ := sp["displayName"].(string)
	claims := map[string]any{
		"aud":             resource,
		"iss":             s.Issuer(tid),
		"iat":             now.Unix(),
		"nbf":             now.Unix(),
		"exp":             exp.Unix(),
		"aio":             "cloudtyke",
		"appid":           appID,
		"azp":             appID,
		"appidacr":        "1",
		"app_displayname": name,
		"idp":             s.Issuer(tid),
		"idtyp":           "app",
		"oid":             oid,
		"sub":             oid,
		"tid":             tid,
		"rh":              "0.cloudtyke",
		"uti":             strings.ReplaceAll(entra.NewGUID(), "-", "")[:22],
		"ver":             "1.0",
		"xms_tcdt":        now.Unix(),
	}
	if roles := s.appRolesFor(tid, oid, resource); len(roles) > 0 {
		claims["roles"] = roles
	}
	if groups := s.Dir.TransitiveGroups(tid, oid); len(groups) > 0 {
		claims["groups"] = groups
	}
	tok, err := s.Signer.Sign(claims)
	return tok, exp, err
}

// appRolesFor returns the values of app roles assigned to principal on the
// resource service principal whose identifiers match resource.
func (s *Service) appRolesFor(tid, principalID, resource string) []string {
	res := normalizeResource(resource)
	var resourceSP entra.Object
	for _, sp := range s.Dir.List(tid, entra.ServicePrincipals) {
		for _, n := range entra.StringList(sp["servicePrincipalNames"]) {
			if normalizeResource(n) == res {
				resourceSP = sp
			}
		}
		if appID, _ := sp["appId"].(string); strings.EqualFold(appID, res) {
			resourceSP = sp
		}
		if resourceSP != nil {
			break
		}
	}
	if resourceSP == nil {
		return nil
	}
	roleByID := map[string]string{}
	for _, ar := range entra.ObjectList(resourceSP["appRoles"]) {
		id, _ := ar["id"].(string)
		v, _ := ar["value"].(string)
		roleByID[strings.ToLower(id)] = v
	}
	var out []string
	for _, a := range entra.ObjectList(resourceSP["_appRoleAssignedTo"]) {
		if pid, _ := a["principalId"].(string); strings.EqualFold(pid, principalID) {
			rid, _ := a["appRoleId"].(string)
			if v := roleByID[strings.ToLower(rid)]; v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

// normalizeResource lower-cases a resource identifier and, for URLs, drops
// the port and trailing slash so https://graph.x:8443/ matches https://graph.x.
func normalizeResource(r string) string {
	r = strings.TrimSuffix(strings.ToLower(r), "/")
	if u, err := url.Parse(r); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Hostname() + strings.TrimSuffix(u.Path, "/")
	}
	return r
}

// verifyFederatedAssertion validates a workload-identity-federation assertion
// against the application's federated identity credentials.
func (s *Service) verifyFederatedAssertion(app entra.Object, assertion string) error {
	claims, _, _, _, err := Split(assertion)
	if err != nil {
		return fmt.Errorf("AADSTS50027: JWT token is invalid or malformed.")
	}
	iss, _ := claims["iss"].(string)
	sub, _ := claims["sub"].(string)
	var audiences []string
	switch a := claims["aud"].(type) {
	case string:
		audiences = []string{a}
	case []any:
		audiences = entra.StringList(a)
	}
	var match entra.Object
	for _, fic := range entra.ObjectList(app["_fics"]) {
		fi, _ := fic["issuer"].(string)
		fs, _ := fic["subject"].(string)
		if strings.TrimSuffix(fi, "/") == strings.TrimSuffix(iss, "/") && fs == sub {
			for _, want := range entra.StringList(fic["audiences"]) {
				if entra.Contains(audiences, want) {
					match = fic
				}
			}
		}
	}
	if match == nil {
		return fmt.Errorf("AADSTS700213: No matching federated identity record found for presented assertion subject '%s'. Check your federated identity credential Subject, Audience and Issuer against the presented assertion.", sub)
	}
	key, err := s.issuerKey(iss)
	if err != nil {
		if os.Getenv("CLOUDTYKE_FEDERATION_INSECURE") == "true" {
			return nil
		}
		return fmt.Errorf("AADSTS700211: Unable to verify the assertion signature for issuer '%s': %v. Set CLOUDTYKE_FEDERATION_INSECURE=true to skip signature checks for external issuers.", iss, err)
	}
	if _, err := VerifyWithKey(assertion, key, time.Now()); err != nil {
		return fmt.Errorf("AADSTS700024: Client assertion is not within its valid time range or has an invalid signature: %v", err)
	}
	return nil
}

func (s *Service) issuerKey(iss string) (*rsa.PublicKey, error) {
	iss = strings.TrimSuffix(iss, "/")
	s.issuerMu.RLock()
	k := s.issuerKeys[iss]
	s.issuerMu.RUnlock()
	if k != nil {
		return k, nil
	}
	// AKS OIDC issuers (oidc.<domain>/<tenant>/<cluster>) are signed by the cluster
	if s.IssuerJWKS != nil {
		if keys, ok := s.IssuerJWKS(iss); ok {
			if len(keys) == 0 {
				return nil, fmt.Errorf("the cluster's OIDC issuer has no signing keys (use CLOUDTYKE_AKS_BACKEND=k3s for service account tokens)")
			}
			return ParseJWK(keys[0])
		}
	}
	if strings.Contains(iss, s.Cfg.Domain) {
		return s.Signer.PublicKey(), nil
	}
	// external issuer: OIDC discovery (needs outbound network)
	resp, err := s.httpClient.Get(iss + "/.well-known/openid-configuration")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var disc struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&disc); err != nil || disc.JWKSURI == "" {
		return nil, fmt.Errorf("no jwks_uri in discovery document")
	}
	resp2, err := s.httpClient.Get(disc.JWKSURI)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()
	var jwks struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp2.Body, 1<<20)).Decode(&jwks); err != nil || len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("empty JWKS")
	}
	return ParseJWK(jwks.Keys[0])
}

// Authenticate validates a bearer token for the given accepted audience hosts.
// It returns an Identity or a (code, message) pair describing the failure.
func (s *Service) Authenticate(r *http.Request, audienceHost string) (*Identity, string, string) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return nil, "AuthenticationFailed", "Authentication failed. The 'Authorization' header is missing."
	}
	if len(h) < 7 || !strings.EqualFold(h[:7], "bearer ") {
		return nil, "AuthenticationFailed", "Authentication failed. The 'Authorization' header is provided in an invalid format."
	}
	claims, err := s.Signer.Verify(strings.TrimSpace(h[7:]))
	switch {
	case err == ErrExpired:
		return nil, "ExpiredAuthenticationToken", "The access token expiry UTC time is earlier than current UTC time."
	case err != nil:
		return nil, "InvalidAuthenticationToken", "The access token is invalid."
	}
	aud, _ := claims["aud"].(string)
	if u, err := url.Parse(aud); err != nil || !strings.EqualFold(u.Hostname(), audienceHost) {
		return nil, "InvalidAuthenticationTokenAudience", fmt.Sprintf("The access token has been obtained for wrong audience or resource '%s'. It should exactly match with one of the allowed audiences 'https://%s/'.", aud, audienceHost)
	}
	id := &Identity{Claims: claims, Audience: aud}
	id.TenantID, _ = claims["tid"].(string)
	id.ObjectID, _ = claims["oid"].(string)
	id.AppID, _ = claims["appid"].(string)
	id.DisplayName, _ = claims["app_displayname"].(string)
	id.UPN, _ = claims["upn"].(string)
	id.Type, _ = claims["idtyp"].(string)
	id.Roles = entra.StringList(claims["roles"])
	if id.Type == "" {
		id.Type = "app"
	}
	return id, "", ""
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]any{
		"error":             code,
		"error_description": desc + "\r\nTrace ID: " + entra.NewGUID() + "\r\nTimestamp: " + time.Now().UTC().Format("2006-01-02 15:04:05Z"),
		"error_codes":       []int{},
		"timestamp":         time.Now().UTC().Format("2006-01-02 15:04:05Z"),
		"trace_id":          entra.NewGUID(),
		"correlation_id":    entra.NewGUID(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
