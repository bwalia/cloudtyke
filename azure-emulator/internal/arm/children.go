package arm

import (
	"sort"
	"strings"
)

// Inline describes an array property that Azure exposes both inline on a
// parent and as standalone child resources, e.g. virtualNetworks
// properties.subnets <-> virtualNetworks/subnets, networkSecurityGroups
// properties.securityRules, routeTables properties.routes and
// managedClusters properties.agentPoolProfiles <-> agentPools.
//
// Children are stored once, as child resources. A parent PUT that includes
// the property replaces the set (Azure semantics: omitted children are
// removed); a PUT without it leaves children untouched. GET re-inlines them.
type Inline struct {
	Prop      string // property name under "properties", e.g. "subnets"
	ChildType string // child type segment, e.g. "subnets"
	// NameField is where the child's name lives inside an inline item
	// (default "name"; agentPoolProfiles also use "name").
	NameField string
	// Flat means the inline item carries its fields at top level instead of
	// under "properties" (agentPoolProfiles are flat, subnets are not).
	Flat bool
	// Validate fills defaults and checks one child. siblings are the other
	// children of the same parent in their final state. parentProps are the
	// parent's properties being written (nil when the child is written on its own).
	Validate func(q *Request, parentPath string, parentProps, child map[string]any, siblings []map[string]any) error
	// Strip lists inline-only fields not stored on the child resource.
	Strip []string
	// KeepMissing makes a parent PUT upsert the listed children without
	// removing the others (AKS agentPoolProfiles behave this way).
	KeepMissing bool
}

func (in Inline) nameField() string {
	if in.NameField != "" {
		return in.NameField
	}
	return "name"
}

// ChildPath returns the resource id of a named child.
func (in Inline) ChildPath(parentPath, name string) string {
	return parentPath + "/" + in.ChildType + "/" + name
}

// Sync validates and stores the inline children of a parent being written.
// It is a no-op when the parent body does not carry the property.
func (in Inline) Sync(q *Request, parentPath, parentType string, props map[string]any) error {
	raw, present := props[in.Prop]
	if !present {
		return nil
	}
	items, _ := raw.([]any)
	var children []map[string]any
	seen := map[string]bool{}
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		name := strOf(m[in.nameField()])
		if name == "" {
			return BadRequest("InvalidRequestFormat", Errorf("Each item in properties.%s must have a name.", in.Prop))
		}
		if seen[strings.ToLower(name)] {
			return BadRequest("InvalidRequestFormat", Errorf("Duplicate name '%s' in properties.%s.", name, in.Prop))
		}
		seen[strings.ToLower(name)] = true
		child := map[string]any{
			"id":   in.ChildPath(parentPath, name),
			"name": name,
			"type": parentType + "/" + in.ChildType,
		}
		var cp map[string]any
		if in.Flat {
			cp = map[string]any{}
			for k, v := range m {
				if k != in.nameField() && k != "id" && k != "type" && k != "etag" {
					cp[k] = v
				}
			}
		} else {
			cp, _ = m["properties"].(map[string]any)
			if cp == nil {
				cp = map[string]any{}
			}
		}
		for _, f := range in.Strip {
			delete(cp, f)
		}
		child["properties"] = cp
		children = append(children, child)
	}
	if in.Validate != nil {
		var kept []map[string]any
		if in.KeepMissing {
			for _, old := range q.S.Children(parentPath + "/" + in.ChildType) {
				if !seen[strings.ToLower(strOf(old["name"]))] {
					kept = append(kept, old)
				}
			}
		}
		for i, c := range children {
			sib := make([]map[string]any, 0, len(children)-1+len(kept))
			sib = append(sib, children[:i]...)
			sib = append(sib, children[i+1:]...)
			sib = append(sib, kept...)
			if err := in.Validate(q, parentPath, props, c, sib); err != nil {
				return err
			}
		}
	}
	// replace the set
	for _, old := range q.S.Children(parentPath + "/" + in.ChildType) {
		if !in.KeepMissing && !seen[strings.ToLower(strOf(old["name"]))] {
			q.S.DeleteTree(strOf(old["id"]))
		}
	}
	for _, c := range children {
		existing, _ := q.S.LoadResource(strOf(c["id"]))
		cp := c["properties"].(map[string]any)
		cp["provisioningState"] = "Succeeded"
		c["etag"] = NewETag()
		c["systemData"] = q.SystemData(existing)
		if err := q.S.SaveResource(c); err != nil {
			return err
		}
	}
	delete(props, in.Prop)
	return nil
}

// Present re-inlines the stored children into a parent being returned.
func (in Inline) Present(s *Server, parentPath string, props map[string]any, present func(map[string]any) map[string]any) {
	kids := s.Children(parentPath + "/" + in.ChildType)
	sort.SliceStable(kids, func(i, j int) bool {
		return strings.ToLower(strOf(kids[i]["name"])) < strings.ToLower(strOf(kids[j]["name"]))
	})
	out := make([]any, 0, len(kids))
	for _, k := range kids {
		if present != nil {
			k = present(k)
		}
		if in.Flat {
			flat := map[string]any{in.nameField(): k["name"]}
			if p, ok := k["properties"].(map[string]any); ok {
				for kk, v := range p {
					flat[kk] = v
				}
			}
			out = append(out, flat)
			continue
		}
		delete(k, "systemData")
		out = append(out, k)
	}
	props[in.Prop] = out
}

// Siblings returns the other stored children of the child's parent.
func (in Inline) Siblings(s *Server, parentPath, name string) []map[string]any {
	var out []map[string]any
	for _, k := range s.Children(parentPath + "/" + in.ChildType) {
		if !strings.EqualFold(strOf(k["name"]), name) {
			out = append(out, k)
		}
	}
	return out
}
