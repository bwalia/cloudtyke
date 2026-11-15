package integration

import (
	"strings"
	"testing"
)

func TestAzurePolicy(t *testing.T) {
	e := start(t)
	arm := e.url("management", "")
	tok := e.token(bootID, bootSec, arm+"/")
	sub := "/subscriptions/" + sandbox
	const api = "?api-version=2023-04-01"
	put := func(path string, body map[string]any) (int, map[string]any) {
		t.Helper()
		code, _, b := e.do("PUT", arm+path+api, tok, body)
		return code, b
	}
	mustPut := func(path string, body map[string]any) map[string]any {
		t.Helper()
		code, b := put(path, body)
		if code >= 300 {
			t.Fatalf("PUT %s: %d %v", path, code, b)
		}
		return b
	}
	def := func(name string, rule map[string]any, params map[string]any) string {
		id := sub + "/providers/Microsoft.Authorization/policyDefinitions/" + name
		mustPut(id, map[string]any{"properties": map[string]any{"displayName": name, "mode": "All", "policyRule": rule, "parameters": params}})
		return id
	}
	assign := func(name, scope, defID string, params map[string]any, extra map[string]any) string {
		id := scope + "/providers/Microsoft.Authorization/policyAssignments/" + name
		p := map[string]any{"policyDefinitionId": defID, "parameters": params}
		for k, v := range extra {
			p[k] = v
		}
		mustPut(id, map[string]any{"properties": p})
		return id
	}
	rg := func(name string, body map[string]any) (int, map[string]any) {
		code, _, b := e.do("PUT", arm+sub+"/resourcegroups/"+name+"?api-version=2024-11-01", tok, body)
		return code, b
	}

	// 1. require a cost-centre tag on resource groups (deny)
	tagDef := def("require-cost-centre", map[string]any{
		"if":   map[string]any{"allOf": []any{map[string]any{"field": "type", "equals": "Microsoft.Resources/subscriptions/resourceGroups"}, map[string]any{"field": "[concat('tags[', parameters('tagName'), ']')]", "exists": "false"}}},
		"then": map[string]any{"effect": "[parameters('effect')]"},
	}, map[string]any{"tagName": map[string]any{"type": "String"}, "effect": map[string]any{"type": "String", "defaultValue": "Deny"}})
	if code, b := put(sub+"/providers/Microsoft.Authorization/policyAssignments/x", map[string]any{"properties": map[string]any{"policyDefinitionId": tagDef}}); code != 400 || errCode(b) != "MissingPolicyParameter" {
		t.Fatalf("missing parameter: %d %v", code, b)
	}
	tagAssign := assign("require-cost-centre", sub, tagDef, map[string]any{"tagName": map[string]any{"value": "cost-centre"}}, map[string]any{
		"nonComplianceMessages": []any{map[string]any{"message": "Every resource group needs a cost-centre tag (FinOps standard FIN-04)."}}})
	code, b := rg("rg-untagged", map[string]any{"location": "uksouth"})
	if code != 403 || errCode(b) != "RequestDisallowedByPolicy" || !strings.Contains(b["error"].(map[string]any)["message"].(string), "FIN-04") {
		t.Fatalf("untagged RG should be denied with the message: %d %v", code, b)
	}
	if code, b := rg("rg-tagged", map[string]any{"location": "uksouth", "tags": map[string]any{"cost-centre": "CC-100"}}); code != 201 {
		t.Fatalf("tagged RG: %d %v", code, b)
	}

	// 2. exemption lets one RG through
	if code, b := put(sub+"/providers/Microsoft.Authorization/policyExemptions/legacy", map[string]any{"properties": map[string]any{"policyAssignmentId": tagAssign, "exemptionCategory": "Waiver"}}); code >= 300 {
		t.Fatalf("exemption: %d %v", code, b)
	}
	if code, b := rg("rg-legacy", map[string]any{"location": "uksouth"}); code != 201 {
		t.Fatalf("exempted: %d %v", code, b)
	}
	e.do("DELETE", arm+sub+"/providers/Microsoft.Authorization/policyExemptions/legacy"+api, tok, nil)

	// 3. allowed locations in an initiative, DoNotEnforce audits only
	locDef := def("allowed-locations", map[string]any{
		"if":   map[string]any{"field": "location", "notIn": "[parameters('listOfAllowedLocations')]"},
		"then": map[string]any{"effect": "deny"},
	}, map[string]any{"listOfAllowedLocations": map[string]any{"type": "Array"}})
	set := sub + "/providers/Microsoft.Authorization/policySetDefinitions/uk-baseline"
	mustPut(set, map[string]any{"properties": map[string]any{"displayName": "UK baseline", "parameters": map[string]any{"locations": map[string]any{"type": "Array"}},
		"policyDefinitions": []any{map[string]any{"policyDefinitionId": locDef, "policyDefinitionReferenceId": "locations", "parameters": map[string]any{"listOfAllowedLocations": map[string]any{"value": "[parameters('locations')]"}}}}}})
	assign("uk-baseline", sub, set, map[string]any{"locations": map[string]any{"value": []any{"uksouth", "ukwest"}}}, nil)
	if code, b := rg("rg-us", map[string]any{"location": "eastus", "tags": map[string]any{"cost-centre": "CC-1"}}); code != 403 || errCode(b) != "RequestDisallowedByPolicy" {
		t.Fatalf("eastus should be denied by the initiative: %d %v", code, b)
	}
	assign("uk-baseline", sub, set, map[string]any{"locations": map[string]any{"value": []any{"uksouth", "ukwest"}}}, map[string]any{"enforcementMode": "DoNotEnforce"})
	if code, b := rg("rg-us", map[string]any{"location": "eastus", "tags": map[string]any{"cost-centre": "CC-1"}}); code != 201 {
		t.Fatalf("DoNotEnforce should allow: %d %v", code, b)
	}

	// 4. modify adds a tag
	modDef := def("add-owner-tag", map[string]any{
		"if":   map[string]any{"field": "tags['managed-by']", "exists": "false"},
		"then": map[string]any{"effect": "modify", "details": map[string]any{"roleDefinitionIds": []any{"/providers/Microsoft.Authorization/roleDefinitions/b24988ac-6180-42a0-ab88-20f7382dd24c"}, "operations": []any{map[string]any{"operation": "addOrReplace", "field": "tags['managed-by']", "value": "platform"}}}},
	}, nil)
	assign("add-owner-tag", sub, modDef, nil, nil)
	_, b = rg("rg-modified", map[string]any{"location": "uksouth", "tags": map[string]any{"cost-centre": "CC-2"}})
	if b["tags"].(map[string]any)["managed-by"] != "platform" {
		t.Fatalf("modify should add the tag: %v", b)
	}

	// 5. audit + compliance via PolicyInsights
	auditDef := def("audit-env-tag", map[string]any{"if": map[string]any{"field": "tags['env']", "exists": "false"}, "then": map[string]any{"effect": "audit"}}, nil)
	assign("audit-env-tag", sub, auditDef, nil, nil)
	code, _, q := e.do("POST", arm+sub+"/providers/Microsoft.PolicyInsights/policyStates/latest/queryResults?api-version=2019-10-01&$filter="+"complianceState%20eq%20'NonCompliant'", tok, nil)
	if code != 200 || len(q["value"].([]any)) == 0 {
		t.Fatalf("expected non-compliant audit states: %d %v", code, q)
	}

	// 6. denyAction protects deletes
	protect := def("protect-prod", map[string]any{
		"if":   map[string]any{"field": "tags['env']", "equals": "prod"},
		"then": map[string]any{"effect": "denyAction", "details": map[string]any{"actionNames": []any{"delete"}}},
	}, nil)
	assign("protect-prod", sub, protect, nil, nil)
	rg("rg-prod", map[string]any{"location": "uksouth", "tags": map[string]any{"cost-centre": "CC-3", "env": "prod"}})
	if code, _, b := e.do("DELETE", arm+sub+"/resourcegroups/rg-prod?api-version=2024-11-01", tok, nil); code != 403 || errCode(b) != "RequestDisallowedByPolicy" {
		t.Fatalf("delete of prod RG should be denied: %d %v", code, b)
	}
}
