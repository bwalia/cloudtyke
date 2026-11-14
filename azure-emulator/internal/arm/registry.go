package arm

import (
	"slices"
	"sort"
	"strings"
)

// Handler handles a routed ARM request.
type Handler func(*Request)

// ScopeKind describes where a resource type may be deployed.
type ScopeKind string

// Scope kinds.
const (
	KindResourceGroup   ScopeKind = "resourceGroup"
	KindSubscription    ScopeKind = "subscription"
	KindTenant          ScopeKind = "tenant"
	KindManagementGroup ScopeKind = "managementGroup"
	KindExtension       ScopeKind = "extension" // any scope (roleAssignments, locks, policy assignments ...)
)

// TypeRegistration declares an emulated resource type.
type TypeRegistration struct {
	Namespace   string   // canonical casing, e.g. "Microsoft.Network"
	Types       []string // canonical casing, e.g. ["virtualNetworks", "subnets"]
	APIVersions []string // supported api-versions (newest first is conventional)
	Scopes      []ScopeKind
	Handler     Handler
	Phase       int
	Notes       string
	// Locations lists regions the type is offered in; empty means all.
	Locations []string
}

// FullType returns "Namespace/type/subtype".
func (t *TypeRegistration) FullType() string {
	return t.Namespace + "/" + strings.Join(t.Types, "/")
}

// key is the lower-cased registry key.
func (t *TypeRegistration) key() string { return strings.ToLower(t.FullType()) }

// SupportsAPIVersion reports whether v is declared.
func (t *TypeRegistration) SupportsAPIVersion(v string) bool {
	return slices.ContainsFunc(t.APIVersions, func(x string) bool { return strings.EqualFold(x, v) })
}

// Registry holds registrations.
type Registry struct {
	types map[string]*TypeRegistration
	// builtins are non-provider-path features (resource groups, subscriptions)
	// listed for coverage purposes only.
	builtins []*TypeRegistration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{types: map[string]*TypeRegistration{}} }

// Register adds a resource type.
func (r *Registry) Register(t *TypeRegistration) { r.types[t.key()] = t }

// RegisterBuiltin records a built-in feature for the coverage matrix.
func (r *Registry) RegisterBuiltin(t *TypeRegistration) { r.builtins = append(r.builtins, t) }

// Lookup finds a registration by lower-cased type key.
func (r *Registry) Lookup(key string) (*TypeRegistration, bool) {
	t, ok := r.types[strings.ToLower(key)]
	return t, ok
}

// All returns every registration (builtins first), sorted by type.
func (r *Registry) All() []*TypeRegistration {
	out := append([]*TypeRegistration{}, r.builtins...)
	for _, t := range r.types {
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].key() < out[j].key() })
	return out
}

// Namespaces returns emulated namespaces with their types.
func (r *Registry) Namespaces() map[string][]*TypeRegistration {
	m := map[string][]*TypeRegistration{}
	for _, t := range r.All() {
		m[t.Namespace] = append(m[t.Namespace], t)
	}
	return m
}
