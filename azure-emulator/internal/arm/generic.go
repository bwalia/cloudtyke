package arm

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// APIError is an ARM error a hook can return.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// BadRequest returns a 400 APIError.
func BadRequest(code, msg string) *APIError { return &APIError{Status: 400, Code: code, Message: msg} }

// GenericOptions customises the generic CRUD handler.
type GenericOptions struct {
	// Tracked resources require a location and support tags.
	Tracked bool
	// KeepLocation keeps an optional location on untracked resources
	// (policy assignments with a managed identity).
	KeepLocation bool
	// Async makes PUT and DELETE answer 201/202 with Azure-AsyncOperation.
	Async bool
	// CreateStatus overrides the 201 answered on create (Storage and
	// PostgreSQL answer 202 Accepted, and their SDK clients reject 201).
	CreateStatus int
	// PatchStatus overrides the 200 answered on PATCH (PostgreSQL answers 202).
	PatchStatus int
	// PatchCreates lets PATCH set a child that Azure treats as always present
	// (PostgreSQL server parameters exist with defaults, so clients PATCH them).
	PatchCreates bool
	// DeleteStatus makes DELETE synchronous with this status (Key Vault and
	// Storage accounts delete synchronously even though their PUT is async).
	DeleteStatus int
	// Prepare fills defaults / validates. body is the stored representation
	// being built (id, name, type, location, tags, properties set already);
	// existing is nil on create. Return an *APIError to reject.
	Prepare func(q *Request, body, existing map[string]any) error
	// AfterWrite runs after the resource is stored.
	AfterWrite func(q *Request, body map[string]any)
	// OnDelete runs before the resource and its children are removed.
	OnDelete func(q *Request, existing map[string]any) error
	// Actions handles POST {resource}/{action}.
	Actions map[string]Handler
	// Present transforms a stored resource before it is returned.
	Present func(q *Request, body map[string]any) map[string]any
}

// LoadResource loads a stored resource by path.
func (s *Server) LoadResource(path string) (map[string]any, bool) {
	var m map[string]any
	ok, err := store.GetJSON(s.Store, store.BucketARM, StoreKey(path), &m)
	return m, ok && err == nil
}

// SaveResource stores a resource under its id.
func (s *Server) SaveResource(body map[string]any) error {
	id, _ := body["id"].(string)
	return store.PutJSON(s.Store, store.BucketARM, StoreKey(id), body)
}

// DeleteTree removes a resource and everything below it (children and
// extension resources). It returns the number of removed documents.
func (s *Server) DeleteTree(path string) int {
	key := StoreKey(path)
	n := 0
	var keys []string
	var docs []map[string]any
	if v, ok, _ := s.Store.Get(store.BucketARM, key); ok {
		keys = append(keys, key)
		var m map[string]any
		if json.Unmarshal(v, &m) == nil {
			docs = append(docs, m)
		}
	}
	_ = s.Store.Scan(store.BucketARM, key+"/", func(k string, v []byte) bool {
		keys = append(keys, k)
		var m map[string]any
		if json.Unmarshal(v, &m) == nil {
			docs = append(docs, m)
		}
		return true
	})
	for _, k := range keys {
		if ok, _ := s.Store.Delete(store.BucketARM, k); ok {
			n++
		}
	}
	// cleanups run after removal so they may delete other trees (AKS node
	// resource groups) without recursing into what was just removed.
	for _, m := range docs {
		if f := s.cleanup[strings.ToLower(strOf(m["type"]))]; f != nil {
			f(m)
		}
	}
	return n
}

// OnDeleted registers a cleanup that runs whenever a resource of fullType is
// removed, including by a resource group (cascade) delete: e.g. a managed
// identity removes its service principal, AKS removes its node resource group.
func (s *Server) OnDeleted(fullType string, f func(body map[string]any)) {
	if s.cleanup == nil {
		s.cleanup = map[string]func(map[string]any){}
	}
	s.cleanup[strings.ToLower(fullType)] = f
}

// Children lists direct children of parentPath of the given child type
// (child types use "parent/type/name"; top-level types use
// "scope/providers/ns/type/name").
func (s *Server) Children(collectionPath string) []map[string]any {
	prefix := StoreKey(collectionPath) + "/"
	var out []map[string]any
	_ = s.Store.Scan(store.BucketARM, prefix, func(k string, v []byte) bool {
		if strings.Contains(strings.TrimPrefix(k, prefix), "/") {
			return true
		}
		var m map[string]any
		if json.Unmarshal(v, &m) == nil {
			out = append(out, m)
		}
		return true
	})
	return out
}

// ListAcrossResourceGroups returns every resource of fullType in a subscription.
func (s *Server) ListAcrossResourceGroups(sub, fullType string) []map[string]any {
	prefix := strings.ToLower("/subscriptions/" + sub + "/resourcegroups/")
	var out []map[string]any
	_ = s.Store.Scan(store.BucketARM, prefix, func(_ string, v []byte) bool {
		var m map[string]any
		if json.Unmarshal(v, &m) == nil && strings.EqualFold(strOf(m["type"]), fullType) {
			out = append(out, m)
		}
		return true
	})
	return out
}

// NotFound writes the ARM ResourceNotFound error for the current request.
func (q *Request) NotFound() {
	rg := ScopeResourceGroup(q.ID.Scope)
	q.Error(404, "ResourceNotFound", Errorf("The Resource '%s' under resource group '%s' was not found. For more details please go to https://aka.ms/ARMResourceNotFoundFix", q.Reg.Namespace+"/"+strings.Join(zipPath(ResourceID{Types: q.Reg.Types, Names: q.ID.Names}), "/"), rg))
}

// Generic returns a handler implementing standard ARM CRUD semantics.
func Generic(o GenericOptions) Handler {
	return func(q *Request) {
		reg := q.Reg
		path := q.ID.Path(reg.Namespace, reg.Types)
		present := func(b map[string]any) map[string]any {
			if o.Present != nil {
				return o.Present(q, b)
			}
			return b
		}

		if a := q.ActionName(); a != "" {
			h := o.Actions[strings.ToLower(a)]
			if h == nil {
				for k, v := range o.Actions {
					if strings.EqualFold(k, a) {
						h = v
					}
				}
			}
			if h == nil {
				q.Error(404, "InvalidResourceType", Errorf("The resource type '%s' could not be found in the namespace '%s' for api version '%s'.", strings.Join(reg.Types, "/")+"/"+a, reg.Namespace, q.APIVersion))
				return
			}
			if _, ok := q.S.LoadResource(path); !ok {
				q.NotFound()
				return
			}
			h(q)
			return
		}

		if q.ID.IsCollection() {
			if q.R.Method != http.MethodGet {
				q.Error(405, "MethodNotAllowed", "The requested method is not allowed on a collection.")
				return
			}
			var items []map[string]any
			if len(reg.Types) == 1 && ScopeResourceGroup(q.ID.Scope) == "" && ScopeSubscription(q.ID.Scope) != "" {
				// Resources that live in resource groups, plus any that live at the
				// subscription itself (custom policy definitions, ...).
				items = q.S.Children(path)
				seen := map[string]bool{}
				for _, it := range items {
					seen[strings.ToLower(strOf(it["id"]))] = true
				}
				for _, it := range q.S.ListAcrossResourceGroups(ScopeSubscription(q.ID.Scope), reg.FullType()) {
					if !seen[strings.ToLower(strOf(it["id"]))] {
						items = append(items, it)
					}
				}
			} else {
				items = q.S.Children(path)
			}
			for i := range items {
				items[i] = present(items[i])
			}
			q.List(items)
			return
		}

		existing, exists := q.S.LoadResource(path)
		switch q.R.Method {
		case http.MethodGet, http.MethodHead:
			if !exists {
				q.NotFound()
				return
			}
			if q.R.Method == http.MethodHead {
				q.Empty(200)
				return
			}
			q.JSON(200, present(existing))

		case http.MethodPut, http.MethodPatch:
			if q.R.Method == http.MethodPatch && !exists && !o.PatchCreates {
				q.NotFound()
				return
			}
			if !checkPreconditions(q, existing, exists) {
				return
			}
			in, ok := q.BodyOrError()
			if !ok {
				return
			}
			// parent must exist for child resources
			if len(reg.Types) > 1 {
				parent := q.ID.ParentPath(reg.Namespace, reg.Types)
				if _, ok := q.S.LoadResource(parent); !ok {
					q.Error(404, "ParentResourceNotFound", Errorf("Can not perform requested operation on nested resource. Parent resource '%s' not found.", parentName(q.ID)))
					return
				}
			}
			var body map[string]any
			if q.R.Method == http.MethodPatch {
				body = deepCopy(existing)
				if body == nil { // PatchCreates
					body = map[string]any{}
				}
				mergePatch(body, in)
			} else {
				body = in
			}
			body["id"] = path
			body["name"] = q.ID.Name()
			body["type"] = reg.FullType()
			if o.Tracked {
				loc, _ := body["location"].(string)
				if loc == "" {
					q.Error(400, "LocationRequired", "The location property is required for this definition.")
					return
				}
				body["location"] = NormalizeLocation(loc)
				if exists {
					if old, _ := existing["location"].(string); old != "" && old != body["location"] {
						q.Error(400, "LocationNotAvailableForResourceType", Errorf("Resource '%s' already exists in location '%s'. A resource with the same name cannot be created in location '%s'. Please select a new resource name.", q.ID.Name(), old, body["location"]))
						return
					}
				}
			} else if !o.KeepLocation || strOf(body["location"]) == "" {
				delete(body, "location")
			} else {
				body["location"] = NormalizeLocation(strOf(body["location"]))
			}
			props, _ := body["properties"].(map[string]any)
			if props == nil {
				props = map[string]any{}
			}
			body["properties"] = props
			if o.Prepare != nil {
				if err := o.Prepare(q, body, existingOrNil(existing, exists)); err != nil {
					if ae, ok := err.(*APIError); ok {
						q.Error(ae.Status, ae.Code, ae.Message)
					} else {
						q.Error(500, "InternalServerError", err.Error())
					}
					return
				}
			}
			props["provisioningState"] = "Succeeded"
			body["etag"] = NewETag()
			body["systemData"] = q.SystemData(existing)
			if err := q.S.SaveResource(body); err != nil {
				q.Error(500, "InternalServerError", err.Error())
				return
			}
			if o.AfterWrite != nil {
				o.AfterWrite(q, body)
			}
			status := 200
			if !exists {
				status = 201
				if o.CreateStatus != 0 {
					status = o.CreateStatus
				}
			} else if q.R.Method == http.MethodPatch && o.PatchStatus != 0 {
				status = o.PatchStatus
			}
			out := present(deepCopy(body))
			if o.Async {
				op := q.S.Ops.Start(ScopeSubscription(path), path, "Succeeded", nil)
				q.AsyncHeaders(op, reg.Namespace, strOf(body["location"]))
				if !op.Done() {
					if p, ok := out["properties"].(map[string]any); ok {
						if exists {
							p["provisioningState"] = "Updating"
						} else {
							p["provisioningState"] = "Creating"
						}
					}
				}
			}
			q.JSON(status, out)

		case http.MethodDelete:
			if !exists {
				q.Empty(204)
				return
			}
			if o.OnDelete != nil {
				if err := o.OnDelete(q, existing); err != nil {
					if ae, ok := err.(*APIError); ok {
						q.Error(ae.Status, ae.Code, ae.Message)
					} else {
						q.Error(500, "InternalServerError", err.Error())
					}
					return
				}
			}
			q.S.DeleteTree(path)
			if o.DeleteStatus != 0 {
				q.Empty(o.DeleteStatus)
				return
			}
			if o.Async {
				op := q.S.Ops.Start(ScopeSubscription(path), path, "Succeeded", nil)
				q.AsyncHeaders(op, reg.Namespace, strOf(existing["location"]))
				q.Empty(202)
				return
			}
			q.Empty(200)

		default:
			q.Error(405, "MethodNotAllowed", "The requested method is not allowed.")
		}
	}
}

func parentName(id ResourceID) string {
	if len(id.Names) < 2 {
		return ""
	}
	return id.Names[len(id.Names)-2]
}

func existingOrNil(m map[string]any, ok bool) map[string]any {
	if !ok {
		return nil
	}
	return m
}

func strOf(v any) string { s, _ := v.(string); return s }

// checkPreconditions implements If-Match / If-None-Match.
func checkPreconditions(q *Request, existing map[string]any, exists bool) bool {
	if im := q.R.Header.Get("If-Match"); im != "" && im != "*" {
		if !exists || strOf(existing["etag"]) != im {
			q.Error(412, "PreconditionFailed", "The condition specified using HTTP conditional header(s) is not met.")
			return false
		}
	} else if im == "*" && !exists {
		q.Error(412, "PreconditionFailed", "The condition specified using HTTP conditional header(s) is not met.")
		return false
	}
	if inm := q.R.Header.Get("If-None-Match"); inm == "*" && exists {
		q.Error(412, "PreconditionFailed", "The resource already exists and If-None-Match: * was specified.")
		return false
	}
	return true
}

func deepCopy(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// DeepCopy exposes deepCopy for RPs.
func DeepCopy(m map[string]any) map[string]any { return deepCopy(m) }

// mergePatch applies an RFC 7386 JSON merge patch.
func mergePatch(dst, patch map[string]any) {
	for k, v := range patch {
		if v == nil {
			delete(dst, k)
			continue
		}
		if pm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok && k != "tags" {
				mergePatch(dm, pm)
				continue
			}
		}
		dst[k] = v
	}
}
