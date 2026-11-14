// Package hierarchy models the Azure scope hierarchy: tenant root ("/"),
// management groups, subscriptions, resource groups and resources. It is
// shared by ARM routing, RBAC inheritance and (later) policy evaluation.
package hierarchy

import (
	"encoding/json"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// Subscription is an emulated subscription.
type Subscription struct {
	ID              string            `json:"subscriptionId"`
	DisplayName     string            `json:"displayName"`
	TenantID        string            `json:"tenantId"`
	State           string            `json:"state"`
	ManagementGroup string            `json:"managementGroup,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
}

// Store wraps the persistence of the hierarchy.
type Store struct{ st store.Store }

// New returns a hierarchy store.
func New(st store.Store) *Store { return &Store{st: st} }

// PutSubscription stores a subscription.
func (h *Store) PutSubscription(s Subscription) error {
	if s.State == "" {
		s.State = "Enabled"
	}
	return store.PutJSON(h.st, store.BucketMeta, "subscriptions/"+strings.ToLower(s.ID), s)
}

// GetSubscription loads a subscription.
func (h *Store) GetSubscription(id string) (Subscription, bool) {
	var s Subscription
	ok, err := store.GetJSON(h.st, store.BucketMeta, "subscriptions/"+strings.ToLower(id), &s)
	return s, ok && err == nil
}

// Subscriptions lists all subscriptions.
func (h *Store) Subscriptions() []Subscription {
	var out []Subscription
	_ = h.st.Scan(store.BucketMeta, "subscriptions/", func(_ string, v []byte) bool {
		var s Subscription
		if json.Unmarshal(v, &s) == nil {
			out = append(out, s)
		}
		return true
	})
	return out
}

// ManagementGroup is a minimal management group record used for scope
// inheritance (the full Microsoft.Management RP lands in phase 3).
type ManagementGroup struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	TenantID    string `json:"tenantId"`
	Parent      string `json:"parent,omitempty"` // parent MG name; empty for the tenant root group
}

// PutManagementGroup stores a management group.
func (h *Store) PutManagementGroup(m ManagementGroup) error {
	return store.PutJSON(h.st, store.BucketMeta, "mgs/"+strings.ToLower(m.Name), m)
}

// GetManagementGroup loads a management group.
func (h *Store) GetManagementGroup(name string) (ManagementGroup, bool) {
	var m ManagementGroup
	ok, err := store.GetJSON(h.st, store.BucketMeta, "mgs/"+strings.ToLower(name), &m)
	return m, ok && err == nil
}

// ManagementGroups lists all management groups.
func (h *Store) ManagementGroups() []ManagementGroup {
	var out []ManagementGroup
	_ = h.st.Scan(store.BucketMeta, "mgs/", func(_ string, v []byte) bool {
		var m ManagementGroup
		if json.Unmarshal(v, &m) == nil {
			out = append(out, m)
		}
		return true
	})
	return out
}

// MGScope returns the ARM scope of a management group.
func MGScope(name string) string {
	return "/providers/Microsoft.Management/managementGroups/" + name
}

// Segments splits an ARM path into its segments.
func Segments(id string) []string {
	id = strings.Trim(id, "/")
	if id == "" {
		return nil
	}
	return strings.Split(id, "/")
}

// Parent returns the parent scope of an ARM scope, following management
// group membership for subscriptions. The root scope "/" has no parent.
func (h *Store) Parent(scope string) (string, bool) {
	seg := Segments(scope)
	n := len(seg)
	switch {
	case n == 0:
		return "", false
	case n == 2 && strings.EqualFold(seg[0], "subscriptions"):
		if s, ok := h.GetSubscription(seg[1]); ok && s.ManagementGroup != "" {
			return MGScope(s.ManagementGroup), true
		}
		return "/", true
	case n == 4 && strings.EqualFold(seg[0], "providers") && strings.EqualFold(seg[1], "Microsoft.Management") && strings.EqualFold(seg[2], "managementGroups"):
		if m, ok := h.GetManagementGroup(seg[3]); ok && m.Parent != "" {
			return MGScope(m.Parent), true
		}
		return "/", true
	case n == 4 && strings.EqualFold(seg[0], "subscriptions") && strings.EqualFold(seg[2], "resourceGroups"):
		return "/" + strings.Join(seg[:2], "/"), true
	}
	p := LastProviders(seg)
	if p < 0 {
		// unknown shape: walk up two segments
		if n <= 2 {
			return "/", true
		}
		return "/" + strings.Join(seg[:n-2], "/"), true
	}
	rest := seg[p+2:]
	if len(rest) > 2 {
		return "/" + strings.Join(seg[:n-2], "/"), true
	}
	if p == 0 {
		return "/", true
	}
	return "/" + strings.Join(seg[:p], "/"), true
}

// Chain returns scope followed by all its ancestors up to "/".
func (h *Store) Chain(scope string) []string {
	out := []string{normalize(scope)}
	cur := scope
	for i := 0; i < 64; i++ {
		p, ok := h.Parent(cur)
		if !ok {
			break
		}
		out = append(out, normalize(p))
		cur = p
	}
	return out
}

// IsAncestorOrSelf reports whether a is scope b or one of its ancestors.
func (h *Store) IsAncestorOrSelf(a, b string) bool {
	for _, c := range h.Chain(b) {
		if strings.EqualFold(c, normalize(a)) {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	s = "/" + strings.Trim(s, "/")
	return s
}

// LastProviders returns the index of the last "providers" segment that is
// followed by a namespace, or -1.
func LastProviders(seg []string) int {
	for i := len(seg) - 2; i >= 0; i-- {
		if strings.EqualFold(seg[i], "providers") {
			return i
		}
	}
	return -1
}
