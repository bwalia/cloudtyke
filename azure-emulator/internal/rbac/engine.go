// Package rbac implements Azure role-based access control: built-in and
// custom role definitions, role assignments at any scope with inheritance,
// and the authorization check applied to every ARM request.
package rbac

import (
	"encoding/json"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/auth"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/hierarchy"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// AssignmentsSegment is the lower-cased path suffix under which role
// assignments live for any scope.
const AssignmentsSegment = "/providers/microsoft.authorization/roleassignments/"

// Engine evaluates permissions.
type Engine struct {
	st  store.Store
	h   *hierarchy.Store
	dir *entra.Directory
}

// New builds an RBAC engine.
func New(st store.Store, h *hierarchy.Store, dir *entra.Directory) *Engine {
	return &Engine{st: st, h: h, dir: dir}
}

// Assignment is the subset of a role assignment the engine needs.
type Assignment struct {
	ID               string
	Scope            string
	RoleDefinitionID string // guid
	PrincipalID      string
}

// AssignmentKeyPrefix returns the store prefix for assignments directly at scope.
func AssignmentKeyPrefix(scope string) string {
	s := strings.ToLower(strings.TrimSuffix(scope, "/"))
	return s + AssignmentsSegment
}

// AssignmentsAt returns the role assignments defined exactly at scope.
func (e *Engine) AssignmentsAt(scope string) []Assignment {
	var out []Assignment
	_ = e.st.Scan(store.BucketARM, AssignmentKeyPrefix(scope), func(k string, v []byte) bool {
		rest := strings.TrimPrefix(k, AssignmentKeyPrefix(scope))
		if strings.Contains(rest, "/") {
			return true
		}
		if a, ok := parseAssignment(v); ok {
			out = append(out, a)
		}
		return true
	})
	return out
}

// AssignmentsBelow returns assignments at scope or any descendant scope.
func (e *Engine) AssignmentsBelow(scope string) []Assignment {
	var out []Assignment
	prefix := strings.ToLower(strings.TrimSuffix(scope, "/"))
	if prefix == "" {
		prefix = "/"
	}
	_ = e.st.Scan(store.BucketARM, prefix, func(k string, v []byte) bool {
		if !strings.Contains(k, AssignmentsSegment) {
			return true
		}
		if a, ok := parseAssignment(v); ok {
			out = append(out, a)
		}
		return true
	})
	return out
}

func parseAssignment(v []byte) (Assignment, bool) {
	var r struct {
		ID         string `json:"id"`
		Properties struct {
			Scope            string `json:"scope"`
			RoleDefinitionID string `json:"roleDefinitionId"`
			PrincipalID      string `json:"principalId"`
		} `json:"properties"`
	}
	if json.Unmarshal(v, &r) != nil || r.Properties.RoleDefinitionID == "" {
		return Assignment{}, false
	}
	return Assignment{ID: r.ID, Scope: r.Properties.Scope, RoleDefinitionID: LastSegment(r.Properties.RoleDefinitionID), PrincipalID: r.Properties.PrincipalID}, true
}

// LastSegment returns the final path segment of an ID.
func LastSegment(id string) string {
	id = strings.TrimSuffix(id, "/")
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// RoleDefinition returns a built-in or custom role definition by guid.
func (e *Engine) RoleDefinition(guid string) (RoleDefinition, bool) {
	guid = strings.ToLower(guid)
	if r, ok := BuiltinByID[guid]; ok {
		return r, true
	}
	var r RoleDefinition
	ok, err := store.GetJSON(e.st, store.BucketRoleDefs, guid, &r)
	return r, ok && err == nil
}

// CustomRoles lists custom role definitions.
func (e *Engine) CustomRoles() []RoleDefinition {
	var out []RoleDefinition
	_ = e.st.Scan(store.BucketRoleDefs, "", func(_ string, v []byte) bool {
		var r RoleDefinition
		if json.Unmarshal(v, &r) == nil {
			out = append(out, r)
		}
		return true
	})
	return out
}

// PutCustomRole stores a custom role.
func (e *Engine) PutCustomRole(r RoleDefinition) error {
	return store.PutJSON(e.st, store.BucketRoleDefs, strings.ToLower(r.ID), r)
}

// DeleteCustomRole removes a custom role.
func (e *Engine) DeleteCustomRole(guid string) bool {
	ok, _ := e.st.Delete(store.BucketRoleDefs, strings.ToLower(guid))
	return ok
}

// principalIDs returns the caller object id plus transitive group ids.
func (e *Engine) principalIDs(id *auth.Identity) map[string]bool {
	m := map[string]bool{strings.ToLower(id.ObjectID): true}
	for _, g := range e.dir.TransitiveGroups(id.TenantID, id.ObjectID) {
		m[strings.ToLower(g)] = true
	}
	return m
}

// Allowed reports whether the identity may perform action at scope.
// dataAction selects DataActions instead of Actions.
func (e *Engine) Allowed(id *auth.Identity, action, scope string, dataAction bool) bool {
	if id == nil {
		return false
	}
	principals := e.principalIDs(id)
	for _, s := range e.h.Chain(scope) {
		for _, a := range e.AssignmentsAt(s) {
			if !principals[strings.ToLower(a.PrincipalID)] {
				continue
			}
			rd, ok := e.RoleDefinition(a.RoleDefinitionID)
			if !ok {
				continue
			}
			if Permits(rd, action, dataAction) {
				return true
			}
		}
	}
	return false
}

// HasAnyAccess reports whether the identity holds any assignment at scope,
// an ancestor, or a descendant (used to decide subscription visibility).
func (e *Engine) HasAnyAccess(id *auth.Identity, scope string) bool {
	principals := e.principalIDs(id)
	for _, s := range e.h.Chain(scope) {
		for _, a := range e.AssignmentsAt(s) {
			if principals[strings.ToLower(a.PrincipalID)] {
				return true
			}
		}
	}
	for _, a := range e.AssignmentsBelow(scope) {
		if principals[strings.ToLower(a.PrincipalID)] {
			return true
		}
	}
	return false
}

// Permits evaluates a role definition against an action.
func Permits(rd RoleDefinition, action string, dataAction bool) bool {
	for _, p := range rd.Permissions {
		allow, deny := p.Actions, p.NotActions
		if dataAction {
			allow, deny = p.DataActions, p.NotDataActions
		}
		if matchAny(allow, action) && !matchAny(deny, action) {
			return true
		}
	}
	return false
}

func matchAny(patterns []string, action string) bool {
	for _, p := range patterns {
		if Match(p, action) {
			return true
		}
	}
	return false
}

// Match is a case-insensitive wildcard match where '*' spans any characters
// (Azure RBAC semantics: "Microsoft.Compute/*/read", "*/read", "*").
func Match(pattern, action string) bool {
	p, s := strings.ToLower(pattern), strings.ToLower(action)
	// iterative glob with backtracking
	pi, si, star, mark := 0, 0, -1, 0
	for si < len(s) {
		switch {
		case pi < len(p) && p[pi] == s[si]:
			pi++
			si++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
