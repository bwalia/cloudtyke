// Package policy implements Azure Policy: definitions, initiatives (policy
// set definitions), assignments and exemptions in Microsoft.Authorization,
// enforcement on every ARM write (deny, modify, append, denyAction on
// delete), and compliance (audit) states queryable through
// Microsoft.PolicyInsights, like the Compliance blade.
package policy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/managedidentity"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

const ns = "Microsoft.Authorization"

var versions = []string{"2025-03-01", "2024-05-01", "2023-04-01", "2022-06-01", "2021-06-01", "2020-09-01"}

var effects = map[string]bool{"deny": true, "audit": true, "modify": true, "append": true, "disabled": true, "auditifnotexists": true, "deployifnotexists": true, "denyaction": true, "manual": true}

// Engine evaluates policies.
type Engine struct{ s *arm.Server }

// Register wires the RPs and installs the enforcement hook.
func Register(s *arm.Server) *Engine {
	e := &Engine{s: s}
	defScopes := []arm.ScopeKind{arm.KindTenant, arm.KindSubscription, arm.KindManagementGroup}
	reg := func(types []string, scopes []arm.ScopeKind, notes string, o arm.GenericOptions) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: versions, Scopes: scopes, Phase: 3, Notes: notes, Handler: arm.Generic(o)})
	}
	reg([]string{"policyDefinitions"}, defScopes, "custom definitions at subscription / management group scope: mode All or Indexed, parameters, if/then rules with aliases, field/value conditions and template functions",
		arm.GenericOptions{Prepare: e.prepareDefinition})
	reg([]string{"policySetDefinitions"}, defScopes, "initiatives: grouped definitions with their own parameters (e.g. a PCI-DSS baseline)",
		arm.GenericOptions{Prepare: e.prepareSet})
	reg([]string{"policyAssignments"}, []arm.ScopeKind{arm.KindExtension}, "at management group, subscription, resource group or resource scope; parameters, notScopes, enforcementMode, non-compliance messages, managed identity for modify; enforced on every ARM write (RequestDisallowedByPolicy)",
		arm.GenericOptions{KeepLocation: true, Prepare: e.prepareAssignment, AfterWrite: func(q *arm.Request, b map[string]any) { e.ScanAssignment(strOf(b["id"])) }})
	reg([]string{"policyExemptions"}, []arm.ScopeKind{arm.KindExtension}, "Waiver / Mitigated exemptions with expiry and per-definition references",
		arm.GenericOptions{Prepare: e.prepareExemption, AfterWrite: func(q *arm.Request, b map[string]any) { e.ScanAssignment(strOf(props(b)["policyAssignmentId"])) }})
	s.Registry.Register(&arm.TypeRegistration{Namespace: "Microsoft.PolicyInsights", Types: []string{"policyStates"}, APIVersions: []string{"2024-10-01", "2019-10-01"},
		Scopes: []arm.ScopeKind{arm.KindExtension}, Phase: 3, Notes: "POST policyStates/latest/queryResults | summarize | triggerEvaluation: compliance per assignment and resource", Handler: e.insights})

	s.OnDeleted(ns+"/policyExemptions", func(b map[string]any) { e.ScanAssignment(strOf(props(b)["policyAssignmentId"])) })
	s.OnDeleted(ns+"/policyAssignments", func(b map[string]any) {
		e.dropStates(strOf(b["id"]))
		managedidentity.CleanupSystemIdentity(s, b)
	})
	registerManagementGroups(s)
	s.PreWrite = append(s.PreWrite, e.preWrite)
	return e
}

func props(b map[string]any) map[string]any {
	p, _ := b["properties"].(map[string]any)
	if p == nil {
		p = map[string]any{}
		b["properties"] = p
	}
	return p
}

// ------------------------------------------------------------ resource validation

func (e *Engine) prepareDefinition(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	rule, _ := p["policyRule"].(map[string]any)
	if rule == nil || rule["if"] == nil || rule["then"] == nil {
		return arm.BadRequest("InvalidPolicyRule", "The policy definition needs properties.policyRule with 'if' and 'then'.")
	}
	then, _ := rule["then"].(map[string]any)
	eff := strOf(then["effect"])
	if !strings.HasPrefix(eff, "[") && !effects[strings.ToLower(eff)] {
		return arm.BadRequest("InvalidPolicyRuleEffect", arm.Errorf("The policy effect '%s' is not valid. Allowed: Deny, Audit, Modify, Append, AuditIfNotExists, DeployIfNotExists, DenyAction, Disabled, Manual.", eff))
	}
	if strOf(p["mode"]) == "" {
		p["mode"] = "Indexed"
	}
	if m := strings.ToLower(strOf(p["mode"])); m != "all" && m != "indexed" && !strings.HasPrefix(m, "microsoft.") {
		return arm.BadRequest("InvalidPolicyMode", "mode must be All or Indexed (or a resource provider mode).")
	}
	p["policyType"] = "Custom"
	if p["parameters"] == nil {
		p["parameters"] = map[string]any{}
	}
	return nil
}

func (e *Engine) prepareSet(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	refs := asList(p["policyDefinitions"])
	if len(refs) == 0 {
		return arm.BadRequest("InvalidPolicySetDefinition", "An initiative needs at least one entry in properties.policyDefinitions.")
	}
	seen := map[string]bool{}
	for i, r := range refs {
		m, _ := r.(map[string]any)
		id := strOf(m["policyDefinitionId"])
		if _, ok := e.s.LoadResource(id); !ok {
			return &arm.APIError{Status: 404, Code: "PolicyDefinitionNotFound", Message: arm.Errorf("The policy definition '%s' could not be found.", id)}
		}
		if strOf(m["policyDefinitionReferenceId"]) == "" {
			m["policyDefinitionReferenceId"] = fmt.Sprintf("%s_%d", lastSeg(id), i)
		}
		if seen[strOf(m["policyDefinitionReferenceId"])] {
			return arm.BadRequest("InvalidPolicySetDefinition", arm.Errorf("Duplicate policyDefinitionReferenceId '%v'.", m["policyDefinitionReferenceId"]))
		}
		seen[strOf(m["policyDefinitionReferenceId"])] = true
	}
	p["policyType"] = "Custom"
	if p["parameters"] == nil {
		p["parameters"] = map[string]any{}
	}
	return nil
}

func (e *Engine) prepareAssignment(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	defID := strOf(p["policyDefinitionId"])
	def, ok := e.s.LoadResource(defID)
	if !ok {
		return &arm.APIError{Status: 404, Code: "PolicyDefinitionNotFound", Message: arm.Errorf("The policy definition '%s' could not be found.", defID)}
	}
	given, _ := p["parameters"].(map[string]any)
	declared, _ := props(def)["parameters"].(map[string]any)
	for name, d := range declared {
		dm, _ := d.(map[string]any)
		if _, has := dm["defaultValue"]; has {
			continue
		}
		if _, has := given[name]; !has {
			return arm.BadRequest("MissingPolicyParameter", arm.Errorf("The policy parameter '%s' is missing.", name))
		}
	}
	for name := range given {
		if _, has := declared[name]; !has {
			return arm.BadRequest("UndefinedPolicyParameter", arm.Errorf("The policy parameter '%s' is not defined in policy definition '%s'.", name, def["name"]))
		}
	}
	switch strOf(p["enforcementMode"]) {
	case "":
		p["enforcementMode"] = "Default"
	case "Default", "DoNotEnforce":
	default:
		return arm.BadRequest("InvalidEnforcementMode", "enforcementMode must be Default or DoNotEnforce.")
	}
	p["scope"] = assignmentScope(strOf(body["id"]))
	if p["notScopes"] == nil {
		p["notScopes"] = []any{}
	}
	if body["identity"] != nil {
		if strOf(body["location"]) == "" {
			return arm.BadRequest("LocationRequired", "A policy assignment with a managed identity needs a location.")
		}
		if err := managedidentity.ApplyIdentity(q, body, existing); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) prepareExemption(q *arm.Request, body, existing map[string]any) error {
	p := props(body)
	aid := strOf(p["policyAssignmentId"])
	if _, ok := e.s.LoadResource(aid); !ok {
		return &arm.APIError{Status: 404, Code: "PolicyAssignmentNotFound", Message: arm.Errorf("The policy assignment '%s' is not found.", aid)}
	}
	switch strOf(p["exemptionCategory"]) {
	case "Waiver", "Mitigated":
	default:
		return arm.BadRequest("InvalidExemptionCategory", "exemptionCategory must be Waiver or Mitigated.")
	}
	scope := assignmentScope(strOf(body["id"]))
	if !e.s.H.IsAncestorOrSelf(strOf(props(mustLoad(e.s, aid))["scope"]), scope) {
		return arm.BadRequest("InvalidPolicyExemptionScope", "The exemption scope must be at or below the assignment's scope.")
	}
	return nil
}

func mustLoad(s *arm.Server, id string) map[string]any {
	m, _ := s.LoadResource(id)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func lastSeg(id string) string { return id[strings.LastIndex(id, "/")+1:] }

// assignmentScope strips "/providers/Microsoft.Authorization/policyX/name".
func assignmentScope(id string) string {
	l := strings.ToLower(id)
	i := strings.LastIndex(l, "/providers/microsoft.authorization/")
	if i <= 0 {
		return "/"
	}
	return id[:i]
}

// ------------------------------------------------------------ evaluation

type applicable struct {
	assignment map[string]any
	def        map[string]any
	refID      string
	params     map[string]any
	exempt     bool
}

// applicableTo returns every (assignment, definition) pair whose scope covers id.
func (e *Engine) applicableTo(id string) []applicable {
	var assignments []map[string]any
	_ = e.s.Store.Scan(store.BucketARM, "", func(k string, v []byte) bool {
		if strings.Contains(k, "/providers/microsoft.authorization/policyassignments/") && strings.Count(k[strings.Index(k, "/policyassignments/")+19:], "/") == 0 {
			var m map[string]any
			if json.Unmarshal(v, &m) == nil {
				assignments = append(assignments, m)
			}
		}
		return true
	})
	sort.Slice(assignments, func(i, j int) bool { return strOf(assignments[i]["id"]) < strOf(assignments[j]["id"]) })
	var out []applicable
	for _, a := range assignments {
		ap := props(a)
		scope := strOf(ap["scope"])
		if !e.s.H.IsAncestorOrSelf(scope, id) {
			continue
		}
		skip := false
		for _, ns := range asList(ap["notScopes"]) {
			if e.s.H.IsAncestorOrSelf(fmt.Sprint(ns), id) {
				skip = true
			}
		}
		if skip {
			continue
		}
		def, ok := e.s.LoadResource(strOf(ap["policyDefinitionId"]))
		if !ok {
			continue
		}
		given, _ := ap["parameters"].(map[string]any)
		params := resolveParams(props(def)["parameters"], given, nil)
		if strings.Contains(strings.ToLower(strOf(def["type"])), "policysetdefinitions") {
			for _, r := range asList(props(def)["policyDefinitions"]) {
				rm, _ := r.(map[string]any)
				child, ok := e.s.LoadResource(strOf(rm["policyDefinitionId"]))
				if !ok {
					continue
				}
				refParams, _ := rm["parameters"].(map[string]any)
				t := &target{params: params}
				childGiven := map[string]any{}
				for k, v := range refParams {
					vm, _ := v.(map[string]any)
					val, _ := t.expr(vm["value"])
					childGiven[k] = map[string]any{"value": val}
				}
				ref := strOf(rm["policyDefinitionReferenceId"])
				out = append(out, applicable{assignment: a, def: child, refID: ref, params: resolveParams(props(child)["parameters"], childGiven, nil), exempt: e.exempted(a, ref, id)})
			}
			continue
		}
		out = append(out, applicable{assignment: a, def: def, params: params, exempt: e.exempted(a, "", id)})
	}
	return out
}

func resolveParams(declared any, given map[string]any, _ any) map[string]any {
	out := map[string]any{}
	dm, _ := declared.(map[string]any)
	for name, d := range dm {
		m, _ := d.(map[string]any)
		if v, ok := m["defaultValue"]; ok {
			out[name] = v
		}
	}
	for name, g := range given {
		gm, _ := g.(map[string]any)
		if v, ok := gm["value"]; ok {
			out[name] = v
		}
	}
	return out
}

func (e *Engine) exempted(a map[string]any, refID, id string) bool {
	found := false
	_ = e.s.Store.Scan(store.BucketARM, "", func(k string, v []byte) bool {
		if !strings.Contains(k, "/providers/microsoft.authorization/policyexemptions/") {
			return true
		}
		var x map[string]any
		if json.Unmarshal(v, &x) != nil {
			return true
		}
		p := props(x)
		if !strings.EqualFold(strOf(p["policyAssignmentId"]), strOf(a["id"])) {
			return true
		}
		if exp := strOf(p["expiresOn"]); exp != "" {
			if t, err := time.Parse(time.RFC3339, exp); err == nil && t.Before(time.Now()) {
				return true
			}
		}
		if refs := asList(p["policyDefinitionReferenceIds"]); len(refs) > 0 && refID != "" {
			match := false
			for _, r := range refs {
				if strings.EqualFold(fmt.Sprint(r), refID) {
					match = true
				}
			}
			if !match {
				return true
			}
		}
		if e.s.H.IsAncestorOrSelf(assignmentScope(strOf(x["id"])), id) {
			found = true
			return false
		}
		return true
	})
	return found
}

// result of evaluating one definition against a resource.
type result struct {
	ap        applicable
	effect    string
	matched   bool // the "if" condition is true (non-compliant for audit/deny)
	err       error
	operation map[string]any
}

func (e *Engine) newTarget(id, typ string, body map[string]any, params map[string]any) *target {
	sub := arm.ScopeSubscription(id)
	rg := arm.ScopeResourceGroup(id)
	t := &target{body: body, typ: typ, subID: sub, rgName: rg, params: params}
	if rg != "" {
		if m, ok := e.s.LoadResource("/subscriptions/" + sub + "/resourceGroups/" + rg); ok {
			t.rgLoc = strOf(m["location"])
			t.rgTags, _ = m["tags"].(map[string]any)
		} else if strings.EqualFold(typ, "Microsoft.Resources/subscriptions/resourceGroups") {
			t.rgLoc = strOf(body["location"])
		}
	}
	return t
}

func (e *Engine) evaluate(id, typ string, body map[string]any) []result {
	var out []result
	for _, ap := range e.applicableTo(id) {
		dp := props(ap.def)
		mode := strings.ToLower(strOf(dp["mode"]))
		if mode == "indexed" && (body["location"] == nil || strings.EqualFold(typ, "Microsoft.Resources/subscriptions/resourceGroups")) {
			continue
		}
		rule, _ := dp["policyRule"].(map[string]any)
		cond, _ := rule["if"].(map[string]any)
		then, _ := rule["then"].(map[string]any)
		t := e.newTarget(id, typ, body, ap.params)
		effV, err := t.expr(then["effect"])
		eff := strings.ToLower(fmt.Sprint(effV))
		if err != nil {
			out = append(out, result{ap: ap, err: err})
			continue
		}
		if eff == "disabled" {
			continue
		}
		ok, err := t.eval(cond)
		out = append(out, result{ap: ap, effect: eff, matched: ok, err: err, operation: then})
	}
	return out
}

// ------------------------------------------------------------ enforcement hook

func (e *Engine) preWrite(q *arm.Request) bool {
	m := q.R.Method
	var id, typ string
	switch {
	case q.Reg != nil:
		if q.ActionName() != "" || m == http.MethodPost {
			return true
		}
		ns := strings.ToLower(q.Reg.Namespace)
		if ns == "microsoft.authorization" || ns == "microsoft.policyinsights" {
			return true // never let policy lock out policy / RBAC management
		}
		typ = q.Reg.FullType()
		id = q.ID.Path(q.Reg.Namespace, q.Reg.Types)
	case len(q.Seg) == 4 && strings.EqualFold(q.Seg[0], "subscriptions") && strings.EqualFold(q.Seg[2], "resourceGroups"):
		typ = "Microsoft.Resources/subscriptions/resourceGroups"
		id = "/subscriptions/" + q.Seg[1] + "/resourceGroups/" + q.Seg[3]
	default:
		return true
	}
	if m == http.MethodDelete {
		existing, ok := e.s.LoadResource(id)
		if !ok {
			return true
		}
		for _, r := range e.evaluate(id, typ, existing) {
			if r.effect == "denyaction" && r.matched && !r.ap.exempt && enforced(r.ap) {
				details, _ := r.operation["details"].(map[string]any)
				for _, a := range asList(details["actionNames"]) {
					if strings.EqualFold(fmt.Sprint(a), "delete") {
						e.reject(q, id, []result{r}, "delete")
						return false
					}
				}
			}
		}
		return true
	}
	if m != http.MethodPut && m != http.MethodPatch {
		return true
	}
	body, err := q.Body()
	if err != nil {
		return true // the handler reports the malformed body
	}
	cand := arm.DeepCopy(body)
	if m == http.MethodPatch {
		if existing, ok := e.s.LoadResource(id); ok {
			cand = merge(arm.DeepCopy(existing), body)
		}
	}
	cand["id"], cand["name"], cand["type"] = id, lastSeg(id), typ
	if l := strOf(cand["location"]); l != "" {
		cand["location"] = arm.NormalizeLocation(l)
	}

	results := e.evaluate(id, typ, cand)
	var denies []result
	for _, r := range results {
		if r.err != nil || !r.matched || r.ap.exempt || !enforced(r.ap) {
			continue
		}
		switch r.effect {
		case "deny":
			denies = append(denies, r)
		case "modify":
			details, _ := r.operation["details"].(map[string]any)
			t := e.newTarget(id, typ, cand, r.ap.params)
			for _, o := range asList(details["operations"]) {
				om, _ := o.(map[string]any)
				val, _ := t.expr(om["value"])
				applyOp(body, typ, strOf(om["operation"]), strOf(om["field"]), val)
				applyOp(cand, typ, strOf(om["operation"]), strOf(om["field"]), val)
			}
		case "append":
			t := e.newTarget(id, typ, cand, r.ap.params)
			for _, d := range asList(r.operation["details"]) {
				dm, _ := d.(map[string]any)
				val, _ := t.expr(dm["value"])
				applyOp(body, typ, "add", strOf(dm["field"]), val)
				applyOp(cand, typ, "add", strOf(dm["field"]), val)
			}
		}
	}
	if len(denies) > 0 {
		e.reject(q, id, denies, "write")
		return false
	}
	// record compliance as it will be after the write (re-evaluate post-modify)
	e.record(id, typ, cand, e.evaluate(id, typ, cand))
	return true
}

func enforced(ap applicable) bool {
	return !strings.EqualFold(strOf(props(ap.assignment)["enforcementMode"]), "DoNotEnforce")
}

func (e *Engine) reject(q *arm.Request, id string, denies []result, action string) {
	var info []any
	var reasons []string
	for _, r := range denies {
		a := r.ap.assignment
		ap := props(a)
		msg := ""
		for _, n := range asList(ap["nonComplianceMessages"]) {
			nm, _ := n.(map[string]any)
			if ref := strOf(nm["policyDefinitionReferenceId"]); ref == "" || strings.EqualFold(ref, r.ap.refID) {
				msg = strOf(nm["message"])
			}
		}
		if msg == "" {
			msg = strOf(props(r.ap.def)["displayName"])
		}
		reasons = append(reasons, msg)
		info = append(info, map[string]any{"type": "PolicyViolation", "info": map[string]any{
			"policyDefinitionDisplayName": props(r.ap.def)["displayName"], "policyDefinitionId": r.ap.def["id"], "policyDefinitionName": r.ap.def["name"],
			"policyDefinitionEffect": r.effect, "policyAssignmentId": a["id"], "policyAssignmentName": a["name"],
			"policyAssignmentDisplayName": ap["displayName"], "policyAssignmentScope": ap["scope"], "policyDefinitionReferenceId": r.ap.refID,
			"policyExemptionIds": []any{},
		}})
	}
	verb := "was disallowed by policy"
	if action == "delete" {
		verb = "is protected from deletion by policy"
	}
	q.JSON(403, map[string]any{"error": map[string]any{
		"code": "RequestDisallowedByPolicy", "target": lastSeg(id),
		"message":        fmt.Sprintf("Resource '%s' %s. Reasons: '%s'. See error details for policy resource IDs.", lastSeg(id), verb, strings.Join(reasons, "', '")),
		"additionalInfo": info,
	}})
}

func merge(dst, patch map[string]any) map[string]any {
	for k, v := range patch {
		if pm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok && k != "tags" {
				dst[k] = merge(dm, pm)
				continue
			}
		}
		dst[k] = v
	}
	return dst
}

// applyOp implements modify / append operations on tags and simple aliases.
func applyOp(body map[string]any, typ, op, field string, val any) {
	lf := strings.ToLower(field)
	if strings.HasPrefix(lf, "tags[") || strings.HasPrefix(lf, "tags.") {
		key := strings.Trim(strings.TrimPrefix(field[4:], "."), "[]'\"")
		tags, _ := body["tags"].(map[string]any)
		if tags == nil {
			tags = map[string]any{}
			body["tags"] = tags
		}
		switch strings.ToLower(op) {
		case "remove":
			delete(tags, key)
		case "add":
			if _, ok := tags[key]; !ok {
				tags[key] = val
			}
		default:
			tags[key] = val
		}
		return
	}
	path := field
	if strings.HasPrefix(strings.ToLower(field), strings.ToLower(typ)+"/") {
		path = field[len(typ)+1:]
	}
	parts := strings.Split(path, ".")
	cur := body
	if parts[0] != "sku" && parts[0] != "identity" && parts[0] != "kind" && parts[0] != "location" {
		cur = props(body)
	}
	for i, p := range parts {
		star := strings.HasSuffix(p, "[*]")
		name := strings.TrimSuffix(p, "[*]")
		if i == len(parts)-1 {
			if star {
				arr := asList(cur[name])
				if l, ok := val.([]any); ok {
					arr = append(arr, l...)
				} else {
					arr = append(arr, val)
				}
				cur[name] = arr
				return
			}
			switch strings.ToLower(op) {
			case "remove":
				delete(cur, name)
			case "add":
				if _, ok := cur[name]; !ok {
					cur[name] = val
				}
			default:
				cur[name] = val
			}
			return
		}
		next, _ := cur[name].(map[string]any)
		if next == nil {
			next = map[string]any{}
			cur[name] = next
		}
		cur = next
	}
}

// ------------------------------------------------------------ compliance states

type state struct {
	Timestamp                   string `json:"timestamp"`
	ResourceID                  string `json:"resourceId"`
	ResourceType                string `json:"resourceType"`
	ResourceLocation            string `json:"resourceLocation,omitempty"`
	ResourceGroup               string `json:"resourceGroup,omitempty"`
	SubscriptionID              string `json:"subscriptionId"`
	PolicyAssignmentID          string `json:"policyAssignmentId"`
	PolicyAssignmentName        string `json:"policyAssignmentName"`
	PolicyAssignmentScope       string `json:"policyAssignmentScope"`
	PolicyDefinitionID          string `json:"policyDefinitionId"`
	PolicyDefinitionName        string `json:"policyDefinitionName"`
	PolicyDefinitionDisplayName string `json:"policyDefinitionDisplayName,omitempty"`
	PolicyDefinitionAction      string `json:"policyDefinitionAction"`
	PolicyDefinitionReferenceID string `json:"policyDefinitionReferenceId,omitempty"`
	PolicySetDefinitionID       string `json:"policySetDefinitionId,omitempty"`
	ComplianceState             string `json:"complianceState"`
	IsCompliant                 bool   `json:"isCompliant"`
}

func stateKey(assignment, ref, resource string) string {
	return strings.ToLower(assignment + "|" + ref + "|" + resource)
}

func (e *Engine) record(id, typ string, body map[string]any, results []result) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range results {
		if r.err != nil {
			continue
		}
		cs := "Compliant"
		switch {
		case r.ap.exempt:
			cs = "Exempt"
		case r.matched && (r.effect == "audit" || r.effect == "deny" || r.effect == "auditifnotexists" || r.effect == "modify" || r.effect == "append"):
			cs = "NonCompliant"
		}
		a := r.ap.assignment
		ap := props(a)
		st := state{Timestamp: now, ResourceID: id, ResourceType: typ, ResourceLocation: strOf(body["location"]), ResourceGroup: arm.ScopeResourceGroup(id),
			SubscriptionID: arm.ScopeSubscription(id), PolicyAssignmentID: strOf(a["id"]), PolicyAssignmentName: strOf(a["name"]), PolicyAssignmentScope: strOf(ap["scope"]),
			PolicyDefinitionID: strOf(r.ap.def["id"]), PolicyDefinitionName: strOf(r.ap.def["name"]), PolicyDefinitionDisplayName: strOf(props(r.ap.def)["displayName"]),
			PolicyDefinitionAction: r.effect, PolicyDefinitionReferenceID: r.ap.refID, ComplianceState: cs, IsCompliant: cs != "NonCompliant"}
		if r.ap.refID != "" {
			st.PolicySetDefinitionID = strOf(ap["policyDefinitionId"])
		}
		_ = store.PutJSON(e.s.Store, store.BucketPolicy, stateKey(strOf(a["id"]), r.ap.refID, id), st)
	}
}

func (e *Engine) dropStates(assignmentID string) {
	prefix := strings.ToLower(assignmentID) + "|"
	var keys []string
	_ = e.s.Store.Scan(store.BucketPolicy, prefix, func(k string, _ []byte) bool { keys = append(keys, k); return true })
	for _, k := range keys {
		_, _ = e.s.Store.Delete(store.BucketPolicy, k)
	}
}

// ScanAssignment re-evaluates every existing resource an assignment covers
// (what Azure's periodic compliance scan does).
func (e *Engine) ScanAssignment(assignmentID string) {
	a, ok := e.s.LoadResource(assignmentID)
	if !ok {
		return
	}
	e.dropStates(assignmentID)
	scope := strings.ToLower(strOf(props(a)["scope"]))
	var docs []map[string]any
	_ = e.s.Store.Scan(store.BucketARM, "", func(k string, v []byte) bool {
		if scope != "/" && !strings.HasPrefix(k, scope) {
			if !strings.HasPrefix(k, "/subscriptions/") || !e.s.H.IsAncestorOrSelf(scope, k) {
				return true
			}
		}
		if strings.Contains(k, "/providers/microsoft.authorization/") {
			return true
		}
		var m map[string]any
		if json.Unmarshal(v, &m) == nil && strOf(m["type"]) != "" {
			docs = append(docs, m)
		}
		return true
	})
	for _, d := range docs {
		id, typ := strOf(d["id"]), strOf(d["type"])
		var mine []result
		for _, r := range e.evaluate(id, typ, d) {
			if strings.EqualFold(strOf(r.ap.assignment["id"]), assignmentID) {
				mine = append(mine, r)
			}
		}
		e.record(id, typ, d, mine)
	}
}

// ScanAll re-evaluates every assignment.
func (e *Engine) ScanAll() {
	var ids []string
	_ = e.s.Store.Scan(store.BucketARM, "", func(k string, v []byte) bool {
		if strings.Contains(k, "/providers/microsoft.authorization/policyassignments/") {
			var m map[string]any
			if json.Unmarshal(v, &m) == nil {
				ids = append(ids, strOf(m["id"]))
			}
		}
		return true
	})
	for _, id := range ids {
		e.ScanAssignment(id)
	}
}

// States returns compliance states under scope ("" = all).
func (e *Engine) States(scope string) []state {
	var out []state
	scope = strings.ToLower(strings.TrimSuffix(scope, "/"))
	_ = e.s.Store.Scan(store.BucketPolicy, "", func(_ string, v []byte) bool {
		var st state
		if json.Unmarshal(v, &st) == nil && (scope == "" || strings.HasPrefix(strings.ToLower(st.ResourceID), scope) || e.s.H.IsAncestorOrSelf(scope, st.ResourceID)) {
			if _, ok := e.s.LoadResource(st.ResourceID); ok || strings.EqualFold(st.ResourceType, "Microsoft.Resources/subscriptions/resourceGroups") {
				out = append(out, st)
			}
		}
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].PolicyAssignmentName != out[j].PolicyAssignmentName {
			return out[i].PolicyAssignmentName < out[j].PolicyAssignmentName
		}
		return out[i].ResourceID < out[j].ResourceID
	})
	return out
}

// insights serves Microsoft.PolicyInsights/policyStates/latest/{queryResults,summarize,triggerEvaluation}.
func (e *Engine) insights(q *arm.Request) {
	if q.R.Method != http.MethodPost {
		q.Error(405, "MethodNotAllowed", "Use POST .../policyStates/latest/queryResults.")
		return
	}
	scope := q.ID.Scope
	switch strings.ToLower(q.ActionName()) {
	case "triggerevaluation":
		e.ScanAll()
		op := q.S.Ops.Start(arm.ScopeSubscription(scope), scope, "Succeeded", nil)
		q.AsyncHeaders(op, "Microsoft.PolicyInsights", "global")
		q.Empty(202)
	case "queryresults":
		states := e.States(scope)
		filter := strings.ToLower(q.R.URL.Query().Get("$filter"))
		var out []any
		for _, st := range states {
			if strings.Contains(filter, "compliancestate eq 'noncompliant'") && st.ComplianceState != "NonCompliant" {
				continue
			}
			out = append(out, st)
		}
		if out == nil {
			out = []any{}
		}
		q.JSON(200, map[string]any{"@odata.context": "https://management.azure.com/providers/Microsoft.PolicyInsights/policyStates/$metadata#latest", "@odata.count": len(out), "value": out})
	case "summarize":
		type agg struct{ nc, total int }
		per := map[string]*agg{}
		resNC := map[string]bool{}
		for _, st := range e.States(scope) {
			a := per[st.PolicyAssignmentID]
			if a == nil {
				a = &agg{}
				per[st.PolicyAssignmentID] = a
			}
			a.total++
			if st.ComplianceState == "NonCompliant" {
				a.nc++
				resNC[strings.ToLower(st.ResourceID)] = true
			}
		}
		var assignments []any
		for id, a := range per {
			assignments = append(assignments, map[string]any{"policyAssignmentId": id, "results": map[string]any{"nonCompliantResources": a.nc, "resourceDetails": []any{
				map[string]any{"complianceState": "noncompliant", "count": a.nc}, map[string]any{"complianceState": "compliant", "count": a.total - a.nc}}}})
		}
		q.JSON(200, map[string]any{"value": []any{map[string]any{"results": map[string]any{"nonCompliantResources": len(resNC)}, "policyAssignments": assignments}}})
	default:
		q.Error(404, "NotFound", "Supported: POST .../policyStates/latest/queryResults, /summarize, /triggerEvaluation.")
	}
}
