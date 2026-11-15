package selftest

import (
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"strings"
)

// Checks for the governance and data providers: Azure Policy, management
// groups, Policy Insights, Key Vault, Storage, PostgreSQL flexible servers
// and Cosmos DB.
//
// Every check works in its own resource group and removes it (and any
// subscription-scoped policy definitions) at the end, also when the check
// fails, so a Deny assignment can never leak into other checks. Globally
// unique names (vaults, storage accounts, servers, Cosmos accounts) come
// from c.Name.

const (
	apiPol = "2023-04-01"
	apiPI  = "2019-10-01"
	apiMG  = "2023-04-01"
	apiKV  = "2023-07-01"
	apiST  = "2023-05-01"
	apiPG  = "2024-08-01"
	apiCos = "2024-11-15"
)

const subSandbox = "/subscriptions/" + SubSandbox

// globalName is c.Name plus a random suffix, for names that are unique
// across all subscriptions (vaults, storage accounts, servers, Cosmos
// accounts) so repeated runs against one emulator never collide.
func globalName(c *C, prefix string) string {
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	return c.Name(prefix) + "-" + hex.EncodeToString(b)
}

// tidy deletes resources without failing the check, so it is safe to defer
// (a failing assertion here would hide the check's real error).
func tidy(c *C, tok string, urls ...string) {
	for _, u := range urls {
		r := c.Do(del, u, tok, nil)
		if r.idx < len(c.calls) {
			c.calls[r.idx].Step = "clean up"
		}
	}
}

// scratchRG creates a resource group (optionally tagged) and returns its id
// and a func that deletes it.
func scratchRG(c *C, tok string, tags map[string]any) (string, func()) {
	id := subSandbox + "/resourceGroups/" + c.Name("rg")
	body := map[string]any{"location": "uksouth"}
	if tags != nil {
		body["tags"] = tags
	}
	c.Status(c.Do(put, c.ARM(id, apiRes), tok, body), 201, "", "create a resource group")
	return id, func() { tidy(c, tok, c.ARM(id, apiRes)) }
}

// ------------------------------------------------------------ policy helpers

func policyDef(c *C, tok, prefix string, props map[string]any) (string, func()) {
	id := subSandbox + "/providers/Microsoft.Authorization/policyDefinitions/" + c.Name(prefix)
	c.Status(c.Do(put, c.ARM(id, apiPol), tok, map[string]any{"properties": props}), 201, "", "create policy definition "+prefix)
	return id, func() { tidy(c, tok, c.ARM(id, apiPol)) }
}

// denyUntaggedIdentity denies user-assigned identities missing the tag named
// by the tagName parameter (which has no default).
func denyUntaggedIdentity(c *C, tok string) (string, func()) {
	return policyDef(c, tok, "deny-untagged", map[string]any{
		"displayName": "Identities need an owner tag", "mode": "All",
		"parameters": map[string]any{"tagName": map[string]any{"type": "String"}},
		"policyRule": map[string]any{
			"if": map[string]any{"allOf": []any{
				map[string]any{"field": "type", "equals": "Microsoft.ManagedIdentity/userAssignedIdentities"},
				map[string]any{"field": "[concat('tags[', parameters('tagName'), ']')]", "exists": "false"},
			}},
			"then": map[string]any{"effect": "deny"},
		},
	})
}

func assignPolicy(c *C, tok, scope, name, defID string, props map[string]any) (string, Resp) {
	id := scope + "/providers/Microsoft.Authorization/policyAssignments/" + name
	p := map[string]any{"policyDefinitionId": defID}
	for k, v := range props {
		p[k] = v
	}
	return id, c.Do(put, c.ARM(id, apiPol), tok, map[string]any{"properties": p})
}

func putIdentity(c *C, tok, rg, name string, tags map[string]any) Resp {
	body := map[string]any{"location": "uksouth"}
	if tags != nil {
		body["tags"] = tags
	}
	return c.Do(put, c.ARM(rg+"/providers/Microsoft.ManagedIdentity/userAssignedIdentities/"+name, apiMI), tok, body)
}

// ------------------------------------------------------------ Key Vault helpers

func vaultBody(extra map[string]any) map[string]any {
	p := map[string]any{"tenantId": Tenant, "sku": map[string]any{"family": "A", "name": "standard"}}
	for k, v := range extra {
		p[k] = v
	}
	return map[string]any{"location": "uksouth", "properties": p}
}

func newVault(c *C, tok, rg string, extra map[string]any) (string, string) {
	name := globalName(c, "kv")
	id := rg + "/providers/Microsoft.KeyVault/vaults/" + name
	c.Status(c.Do(put, c.ARM(id, apiKV), tok, vaultBody(extra)), 201, "", "create vault "+name)
	return id, name
}

// purgeVault removes a soft-deleted vault (deferred cleanup).
func purgeVault(c *C, tok, name string) func() {
	return func() {
		r := c.Do(post, c.ARM(subSandbox+"/providers/Microsoft.KeyVault/locations/uksouth/deletedVaults/"+name+"/purge", apiKV), tok, nil)
		if r.idx < len(c.calls) {
			c.calls[r.idx].Step = "clean up"
		}
	}
}

// ------------------------------------------------------------ Storage helpers

func newAccount(c *C, tok, rg string, sku string) (string, string) {
	name := strings.ReplaceAll(globalName(c, "st"), "-", "")
	id := rg + "/providers/Microsoft.Storage/storageAccounts/" + name
	c.Status(c.Do(put, c.ARM(id, apiST), tok, map[string]any{"location": "uksouth", "kind": "StorageV2", "sku": map[string]any{"name": sku}}), 202, "", "create storage account "+name)
	return id, name
}

// ------------------------------------------------------------ PostgreSQL helpers

func pgBody(tier, skuName string, extra map[string]any) map[string]any {
	p := map[string]any{"version": "16", "administratorLogin": "ctadmin", "administratorLoginPassword": "Selftest-Passw0rd!", "storage": map[string]any{"storageSizeGB": 32}}
	for k, v := range extra {
		p[k] = v
	}
	return map[string]any{"location": "uksouth", "sku": map[string]any{"name": skuName, "tier": tier}, "properties": p}
}

func newPG(c *C, tok, rg string, extra map[string]any) string {
	id := rg + "/providers/Microsoft.DBforPostgreSQL/flexibleServers/" + globalName(c, "pg")
	c.Status(c.Do(put, c.ARM(id, apiPG), tok, pgBody("Burstable", "Standard_B1ms", extra)), 202, "", "create PostgreSQL flexible server")
	return id
}

// ------------------------------------------------------------ Cosmos DB helpers

func cosmosBody(kind string, serverless bool, regions ...string) map[string]any {
	if len(regions) == 0 {
		regions = []string{"uksouth"}
	}
	var locs []any
	for i, r := range regions {
		locs = append(locs, map[string]any{"locationName": r, "failoverPriority": i})
	}
	p := map[string]any{"databaseAccountOfferType": "Standard", "locations": locs}
	if serverless {
		p["capabilities"] = []any{map[string]any{"name": "EnableServerless"}}
	}
	return map[string]any{"location": "uksouth", "kind": kind, "properties": p}
}

func newCosmos(c *C, tok, rg, kind string, serverless bool) string {
	id := rg + "/providers/Microsoft.DocumentDB/databaseAccounts/" + globalName(c, "cosmos")
	what := "create " + kind + " Cosmos DB account"
	if serverless {
		what += " (serverless)"
	}
	c.Status(c.Do(put, c.ARM(id, apiCos), tok, cosmosBody(kind, serverless)), 200, "", what)
	return id
}

func cosmosChild(c *C, tok, id string, resource, options map[string]any, want int, code, what string) Resp {
	p := map[string]any{"resource": resource}
	if options != nil {
		p["options"] = options
	}
	return c.Status(c.Do(put, c.ARM(id, apiCos), tok, map[string]any{"properties": p}), want, code, what)
}

// throughputCheck covers a Cosmos throughputSettings/default row: the parent
// at path (relative to the account) is created with manual RU/s, read back,
// switched to autoscale, and rejected on a serverless account.
func throughputCheck(kind string, parents []string, bodies []map[string]any) Check {
	return Check{"manual RU/s read back, switch to autoscale, rejected on serverless accounts", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		build := func(acct string, withThroughput bool) string {
			id := acct
			for i, p := range parents {
				id += "/" + p
				var opts map[string]any
				if withThroughput && i == len(parents)-1 {
					opts = map[string]any{"throughput": 400}
				}
				cosmosChild(c, tok, id, bodies[i], opts, 202, "", "create "+p)
			}
			return id + "/throughputSettings/default"
		}
		ts := build(newCosmos(c, tok, rg, kind, false), true)
		g := c.Status(c.Do(get, c.ARM(ts, apiCos), tok, nil), 200, "", "read throughput")
		c.Expect(dig(g.Body, "properties.resource.throughput") == float64(400), "throughput is the 400 RU/s set on create, got %v", dig(g.Body, "properties.resource.throughput"))
		a := c.Status(c.Do(put, c.ARM(ts, apiCos), tok, map[string]any{"properties": map[string]any{"resource": map[string]any{"autoscaleSettings": map[string]any{"maxThroughput": 4000}}}}), 200, "", "migrate to autoscale")
		c.Expect(dig(a.Body, "properties.resource.autoscaleSettings.maxThroughput") == float64(4000), "autoscale max is 4000 RU/s, got %v", dig(a.Body, "properties.resource.autoscaleSettings.maxThroughput"))
		sl := build(newCosmos(c, tok, rg, kind, true), false)
		c.Status(c.Do(get, c.ARM(sl, apiCos), tok, nil), 400, "BadRequest", "throughput on a serverless account")
	}}
}

func init() {
	// ------------------------------------------------------------ Azure Policy
	Checks["Microsoft.Authorization/policyDefinitions"] = Check{"rule and effect validation, Indexed default mode, Custom type, listed at the subscription", func(c *C) {
		tok := c.Boot("management")
		base := subSandbox + "/providers/Microsoft.Authorization/policyDefinitions/"
		bad := base + c.Name("bad")
		c.Status(c.Do(put, c.ARM(bad, apiPol), tok, map[string]any{"properties": map[string]any{"policyRule": map[string]any{"if": map[string]any{"field": "location", "equals": "eastus"}}}}), 400, "InvalidPolicyRule", "rule without 'then'")
		c.Status(c.Do(put, c.ARM(bad, apiPol), tok, map[string]any{"properties": map[string]any{"policyRule": map[string]any{"if": map[string]any{"field": "location", "equals": "eastus"}, "then": map[string]any{"effect": "block"}}}}), 400, "InvalidPolicyRuleEffect", "unknown effect")
		id, done := policyDef(c, tok, "audit-eastus", map[string]any{"displayName": "Audit eastus", "policyRule": map[string]any{"if": map[string]any{"field": "location", "equals": "eastus"}, "then": map[string]any{"effect": "audit"}}})
		defer done()
		g := c.Status(c.Do(get, c.ARM(id, apiPol), tok, nil), 200, "", "read definition")
		c.Expect(g.Str("properties.mode") == "Indexed", "mode defaults to Indexed, got %s", g.Str("properties.mode"))
		c.Expect(g.Str("properties.policyType") == "Custom", "policyType is Custom, got %s", g.Str("properties.policyType"))
		l := c.Status(c.Do(get, c.ARM(subSandbox+"/providers/Microsoft.Authorization/policyDefinitions", apiPol), tok, nil), 200, "", "list definitions in the subscription").Value()
		c.Expect(HasID(l, "id", id), "subscription list includes the definition")
		c.Status(c.Do(del, c.ARM(id, apiPol), tok, nil), 200, "", "delete definition")
		c.Status(c.Do(get, c.ARM(id, apiPol), tok, nil), 404, "", "definition is gone")
	}}
	Checks["Microsoft.Authorization/policySetDefinitions"] = Check{"initiatives need existing definitions, reference ids default, Deny via initiative", func(c *C) {
		tok := c.Boot("management")
		base := subSandbox + "/providers/Microsoft.Authorization/policySetDefinitions/"
		set := base + c.Name("set")
		c.Status(c.Do(put, c.ARM(set, apiPol), tok, map[string]any{"properties": map[string]any{"policyDefinitions": []any{}}}), 400, "InvalidPolicySetDefinition", "initiative without definitions")
		c.Status(c.Do(put, c.ARM(set, apiPol), tok, map[string]any{"properties": map[string]any{"policyDefinitions": []any{map[string]any{"policyDefinitionId": subSandbox + "/providers/Microsoft.Authorization/policyDefinitions/does-not-exist"}}}}), 404, "PolicyDefinitionNotFound", "initiative referencing a missing definition")
		def, doneDef := denyUntaggedIdentity(c, tok)
		defer doneDef()
		s := c.Status(c.Do(put, c.ARM(set, apiPol), tok, map[string]any{"properties": map[string]any{"displayName": "Identity baseline",
			"policyDefinitions": []any{map[string]any{"policyDefinitionId": def, "parameters": map[string]any{"tagName": map[string]any{"value": "owner"}}}}}}), 201, "", "create initiative")
		defer tidy(c, tok, c.ARM(set, apiPol))
		refs, _ := dig(s.Body, "properties.policyDefinitions").([]any)
		ref, _ := refs[0].(map[string]any)
		c.Expect(len(refs) == 1 && strOfAny(ref["policyDefinitionReferenceId"]) != "", "policyDefinitionReferenceId is filled in, got %v", ref["policyDefinitionReferenceId"])
		rg, doneRG := scratchRG(c, tok, nil)
		defer doneRG()
		aid, a := assignPolicy(c, tok, rg, "baseline", set, nil)
		c.Status(a, 201, "", "assign the initiative to the resource group")
		defer tidy(c, tok, c.ARM(aid, apiPol))
		d := c.Status(putIdentity(c, tok, rg, "id-untagged", nil), 403, "RequestDisallowedByPolicy", "untagged identity denied by the initiative")
		c.Expect(strings.Contains(string(d.Raw), `"policyDefinitionReferenceId"`), "error names the initiative reference")
		c.Status(putIdentity(c, tok, rg, "id-tagged", map[string]any{"owner": "payments"}), 201, "", "tagged identity is allowed")
	}}
	Checks["Microsoft.Authorization/policyAssignments"] = Check{"required parameters, Deny with the non-compliance message, DoNotEnforce, scoped to one resource group", func(c *C) {
		tok := c.Boot("management")
		def, doneDef := denyUntaggedIdentity(c, tok)
		defer doneDef()
		rg, doneRG := scratchRG(c, tok, nil)
		defer doneRG()
		other, doneOther := scratchRG(c, tok, nil)
		defer doneOther()
		_, miss := assignPolicy(c, tok, rg, "missing-param", def, nil)
		c.Status(miss, 400, "MissingPolicyParameter", "assignment without the required tagName parameter")
		_, bogus := assignPolicy(c, tok, rg, "bad-param", def, map[string]any{"parameters": map[string]any{"tagName": map[string]any{"value": "owner"}, "nope": map[string]any{"value": 1}}})
		c.Status(bogus, 400, "UndefinedPolicyParameter", "assignment with an undeclared parameter")
		params := map[string]any{"tagName": map[string]any{"value": "owner"}}
		aid, a := assignPolicy(c, tok, rg, "require-owner", def, map[string]any{"parameters": params,
			"nonComplianceMessages": []any{map[string]any{"message": "Identities need an owner tag (standard SEC-07)."}}})
		defer tidy(c, tok, c.ARM(aid, apiPol))
		a = c.Status(a, 201, "", "assign at resource group scope")
		c.Expect(strings.EqualFold(a.Str("properties.scope"), rg), "assignment scope is the resource group, got %s", a.Str("properties.scope"))
		c.Expect(a.Str("properties.enforcementMode") == "Default", "enforcementMode defaults to Default")
		d := c.Status(putIdentity(c, tok, rg, "id-untagged", nil), 403, "RequestDisallowedByPolicy", "untagged identity is denied")
		msg, _ := dig(d.Body, "error.message").(string)
		c.Expect(strings.Contains(msg, "SEC-07"), "error carries the non-compliance message, got %s", msg)
		c.Status(putIdentity(c, tok, rg, "id-tagged", map[string]any{"owner": "payments"}), 201, "", "tagged identity is allowed")
		c.Status(putIdentity(c, tok, other, "id-untagged", nil), 201, "", "other resource groups are out of scope")
		_, a = assignPolicy(c, tok, rg, "require-owner", def, map[string]any{"parameters": params, "enforcementMode": "DoNotEnforce"})
		c.Status(a, 200, "", "switch to DoNotEnforce")
		c.Status(putIdentity(c, tok, rg, "id-untagged", nil), 201, "", "DoNotEnforce lets the write through")
		c.Status(c.Do(del, c.ARM(aid, apiPol), tok, nil), 200, "", "delete assignment")
	}}
	Checks["Microsoft.Authorization/policyExemptions"] = Check{"Waiver lets a denied write through until the exemption is removed", func(c *C) {
		tok := c.Boot("management")
		def, doneDef := denyUntaggedIdentity(c, tok)
		defer doneDef()
		rg, doneRG := scratchRG(c, tok, nil)
		defer doneRG()
		aid, a := assignPolicy(c, tok, rg, "require-owner", def, map[string]any{"parameters": map[string]any{"tagName": map[string]any{"value": "owner"}}})
		defer tidy(c, tok, c.ARM(aid, apiPol))
		c.Status(a, 201, "", "assign deny policy to the resource group")
		c.Status(putIdentity(c, tok, rg, "id-legacy", nil), 403, "RequestDisallowedByPolicy", "untagged identity is denied")
		ex := rg + "/providers/Microsoft.Authorization/policyExemptions/legacy"
		c.Status(c.Do(put, c.ARM(ex, apiPol), tok, map[string]any{"properties": map[string]any{"policyAssignmentId": aid, "exemptionCategory": "Forever"}}), 400, "InvalidExemptionCategory", "unknown exemption category")
		c.Status(c.Do(put, c.ARM(ex, apiPol), tok, map[string]any{"properties": map[string]any{"policyAssignmentId": rg + "/providers/Microsoft.Authorization/policyAssignments/nope", "exemptionCategory": "Waiver"}}), 404, "PolicyAssignmentNotFound", "exemption for a missing assignment")
		c.Status(c.Do(put, c.ARM(ex, apiPol), tok, map[string]any{"properties": map[string]any{"policyAssignmentId": aid, "exemptionCategory": "Waiver"}}), 201, "", "create Waiver exemption")
		c.Status(putIdentity(c, tok, rg, "id-legacy", nil), 201, "", "exempted write is allowed")
		c.Status(c.Do(del, c.ARM(ex, apiPol), tok, nil), 200, "", "delete exemption")
		c.Status(putIdentity(c, tok, rg, "id-legacy-2", nil), 403, "RequestDisallowedByPolicy", "denied again without the exemption")
	}}
	Checks["Microsoft.PolicyInsights/policyStates"] = Check{"audit records compliance; queryResults filter and summarize per assignment", func(c *C) {
		tok := c.Boot("management")
		def, doneDef := policyDef(c, tok, "audit-env", map[string]any{"displayName": "Audit env tag", "mode": "All",
			"policyRule": map[string]any{"if": map[string]any{"field": "tags['env']", "exists": "false"}, "then": map[string]any{"effect": "audit"}}})
		defer doneDef()
		rg, doneRG := scratchRG(c, tok, map[string]any{"env": "selftest"})
		defer doneRG()
		aid, a := assignPolicy(c, tok, rg, "audit-env", def, nil)
		defer tidy(c, tok, c.ARM(aid, apiPol))
		c.Status(a, 201, "", "assign audit policy to the resource group")
		bad := c.Status(putIdentity(c, tok, rg, "id-noenv", nil), 201, "", "audit does not block the write").Str("id")
		good := c.Status(putIdentity(c, tok, rg, "id-env", map[string]any{"env": "prod"}), 201, "", "compliant identity").Str("id")
		ps := rg + "/providers/Microsoft.PolicyInsights/policyStates/latest/"
		c.Status(c.Do(post, c.ARM(ps+"listStates", apiPI), tok, nil), 404, "NotFound", "unsupported policyStates action")
		all := c.Status(c.Do(post, c.ARM(ps+"queryResults", apiPI), tok, nil), 200, "", "query all states").Value()
		c.Expect(HasID(all, "resourceId", bad) && HasID(all, "resourceId", good), "both identities have a compliance state")
		nc := c.Status(c.Do(post, c.ARM(ps+"queryResults?$filter="+url.QueryEscape("complianceState eq 'NonCompliant'"), apiPI), tok, nil), 200, "", "query NonCompliant states").Value()
		c.Expect(HasID(nc, "resourceId", bad), "untagged identity is NonCompliant")
		c.Expect(!HasID(nc, "resourceId", good), "tagged identity is not NonCompliant")
		s := c.Status(c.Do(post, c.ARM(ps+"summarize", apiPI), tok, nil), 200, "", "summarize").Value()
		c.Expect(len(s) == 1, "one summary, got %d", len(s))
		var mine float64 = -1
		for _, pa := range s[0]["policyAssignments"].([]any) {
			m, _ := pa.(map[string]any)
			if strings.EqualFold(strOfAny(m["policyAssignmentId"]), aid) {
				mine, _ = dig(m, "results.nonCompliantResources").(float64)
			}
		}
		c.Expect(mine == 1, "assignment summary counts 1 non-compliant resource, got %v", mine)
		c.Status(c.Do(post, c.ARM(ps+"triggerEvaluation", apiPI), tok, nil), 202, "", "trigger an evaluation scan")
	}}
	Checks["Microsoft.Management/managementGroups"] = Check{"seeded hierarchy: list, children, parent, read-only", func(c *C) {
		tok := c.Boot("management")
		base := "/providers/Microsoft.Management/managementGroups"
		l := c.Status(c.Do(get, c.ARM(base, apiMG), tok, nil), 200, "", "list management groups").Value()
		for _, mg := range []string{"ct-platform", "ct-landingzones", "ct-sandbox"} {
			c.Expect(HasID(l, "name", mg), "seeded group %s is listed", mg)
		}
		g := c.Status(c.Do(get, c.ARM(base+"/ct-platform?$expand=children", apiMG), tok, nil), 200, "", "get ct-platform with children")
		c.Expect(g.Str("properties.displayName") == "Platform", "display name is Platform, got %s", g.Str("properties.displayName"))
		c.Expect(g.Str("properties.details.parent.name") == Tenant, "parent is the tenant root group, got %s", g.Str("properties.details.parent.name"))
		var kids []map[string]any
		for _, k := range dig(g.Body, "properties.children").([]any) {
			kids = append(kids, k.(map[string]any))
		}
		c.Expect(len(kids) == 2 && HasID(kids, "name", SubConn), "children are the two platform subscriptions, got %d", len(kids))
		c.Status(c.Do(get, c.ARM(base+"/ct-nope", apiMG), tok, nil), 404, "NotFound", "unknown management group")
		c.Status(c.Do(put, c.ARM(base+"/ct-new", apiMG), tok, map[string]any{"properties": map[string]any{"displayName": "New"}}), 405, "MethodNotAllowed", "management groups are seed-defined")
	}}

	// ------------------------------------------------------------ Key Vault
	Checks["Microsoft.KeyVault/vaults"] = Check{"name rules, vaultUri, soft-delete defaults, global names, delete to deletedVaults", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.KeyVault/vaults/1-bad", apiKV), tok, vaultBody(nil)), 400, "VaultNameNotValid", "name starting with a digit")
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.KeyVault/vaults/"+globalName(c, "kv"), apiKV), tok, vaultBody(map[string]any{"softDeleteRetentionInDays": 3})), 400, "BadRequest", "soft delete retention below 7 days")
		id, name := newVault(c, tok, rg, nil)
		defer purgeVault(c, tok, name)()
		g := c.Status(c.Do(get, c.ARM(id, apiKV), tok, nil), 200, "", "read vault")
		c.Expect(strings.HasPrefix(g.Str("properties.vaultUri"), "https://"+name+".vault."), "vaultUri is https://<name>.vault.<domain>/, got %s", g.Str("properties.vaultUri"))
		c.Expect(dig(g.Body, "properties.enableSoftDelete") == true && dig(g.Body, "properties.softDeleteRetentionInDays") == float64(90), "soft delete is on with 90 days retention")
		other, doneOther := scratchRG(c, tok, nil)
		defer doneOther()
		c.Status(c.Do(put, c.ARM(other+"/providers/Microsoft.KeyVault/vaults/"+name, apiKV), tok, vaultBody(nil)), 409, "VaultAlreadyExists", "same name in another resource group")
		c.Status(c.Do(del, c.ARM(id, apiKV), tok, nil), 200, "", "delete vault")
		c.Status(c.Do(get, c.ARM(id, apiKV), tok, nil), 404, "", "vault is gone from its resource group")
	}}
	Checks["Microsoft.KeyVault/vaults/accessPolicies"] = Check{"add merges permissions per principal, remove drops the entry", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, name := newVault(c, tok, rg, nil)
		defer purgeVault(c, tok, name)()
		defer tidy(c, tok, c.ARM(id, apiKV))
		pol := func(perms ...any) map[string]any {
			return map[string]any{"properties": map[string]any{"accessPolicies": []any{map[string]any{"tenantId": Tenant, "objectId": ReaderSPID, "permissions": map[string]any{"secrets": perms}}}}}
		}
		c.Status(c.Do(put, c.ARM(id+"/accessPolicies/merge", apiKV), tok, pol("Get")), 400, "BadRequest", "unknown operation name")
		c.Status(c.Do(put, c.ARM(id+"/accessPolicies/add", apiKV), tok, pol("Get", "List")), 200, "", "add get/list for the reader")
		c.Status(c.Do(put, c.ARM(id+"/accessPolicies/add", apiKV), tok, pol("get", "Set")), 200, "", "add get/set for the same principal")
		g := c.Status(c.Do(get, c.ARM(id, apiKV), tok, nil), 200, "", "read vault")
		aps, _ := dig(g.Body, "properties.accessPolicies").([]any)
		c.Expect(len(aps) == 1, "one entry per principal, got %d", len(aps))
		perms, _ := dig(aps[0], "permissions.secrets").([]any)
		c.Expect(len(perms) == 3, "permissions are merged case-insensitively (Get, List, Set), got %v", perms)
		c.Status(c.Do(put, c.ARM(id+"/accessPolicies/remove", apiKV), tok, pol()), 200, "", "remove the principal")
		g = c.Status(c.Do(get, c.ARM(id, apiKV), tok, nil), 200, "", "read vault")
		aps, _ = dig(g.Body, "properties.accessPolicies").([]any)
		c.Expect(len(aps) == 0, "no access policies left, got %d", len(aps))
	}}
	Checks["Microsoft.KeyVault/checkNameAvailability"] = Check{"vault names are global and validated", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, name := newVault(c, tok, rg, nil)
		defer purgeVault(c, tok, name)()
		defer tidy(c, tok, c.ARM(id, apiKV))
		u := c.ARM(subSandbox+"/providers/Microsoft.KeyVault/checkNameAvailability", apiKV)
		chk := func(n, what string) Resp {
			return c.Status(c.Do(post, u, tok, map[string]any{"name": n, "type": "Microsoft.KeyVault/vaults"}), 200, "", what)
		}
		t := chk(name, "check a taken name")
		c.Expect(t.Body["nameAvailable"] == false && t.Str("reason") == "AlreadyExists", "taken name is not available")
		i := chk("x", "check a too-short name")
		c.Expect(i.Body["nameAvailable"] == false && i.Str("reason") == "AccountNameInvalid", "invalid name is reported")
		f := chk(globalName(c, "kvf"), "check a free name")
		c.Expect(f.Body["nameAvailable"] == true, "free name is available")
	}}
	Checks["Microsoft.KeyVault/deletedVaults"] = Check{"deleted vaults listed, name blocked until purged", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, name := newVault(c, tok, rg, nil)
		defer purgeVault(c, tok, name)()
		c.Status(c.Do(del, c.ARM(id, apiKV), tok, nil), 200, "", "delete vault (soft delete)")
		list := c.ARM(subSandbox+"/providers/Microsoft.KeyVault/deletedVaults", apiKV)
		l := c.Status(c.Do(get, list, tok, nil), 200, "", "list deleted vaults").Value()
		c.Expect(HasID(l, "name", name), "deleted vault is listed")
		c.Status(c.Do(put, c.ARM(id, apiKV), tok, vaultBody(nil)), 409, "ConflictError", "recreate while soft-deleted")
		c.Status(c.Do(post, c.ARM(subSandbox+"/providers/Microsoft.KeyVault/locations/uksouth/deletedVaults/"+name+"/purge", apiKV), tok, nil), 202, "", "purge")
		l = c.Status(c.Do(get, list, tok, nil), 200, "", "list deleted vaults").Value()
		c.Expect(!HasID(l, "name", name), "purged vault is no longer listed")
	}}
	Checks["Microsoft.KeyVault/locations/deletedVaults"] = Check{"get a deleted vault, recover it, purge, purge protection", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, name := newVault(c, tok, rg, nil)
		defer purgeVault(c, tok, name)()
		dv := c.ARM(subSandbox+"/providers/Microsoft.KeyVault/locations/uksouth/deletedVaults/"+name, apiKV)
		c.Status(c.Do(get, dv, tok, nil), 404, "DeletedVaultNotFound", "live vault is not deleted")
		c.Status(c.Do(del, c.ARM(id, apiKV), tok, nil), 200, "", "delete vault")
		g := c.Status(c.Do(get, dv, tok, nil), 200, "", "get deleted vault")
		c.Expect(strings.EqualFold(g.Str("properties.vaultId"), id) && g.Str("properties.scheduledPurgeDate") != "", "deleted vault records its id and purge date")
		c.Status(c.Do(put, c.ARM(id, apiKV), tok, map[string]any{"location": "uksouth", "properties": map[string]any{"createMode": "recover"}}), 201, "", "recover with createMode recover")
		c.Status(c.Do(get, dv, tok, nil), 404, "DeletedVaultNotFound", "recovered vault left deletedVaults")
		c.Status(c.Do(del, c.ARM(id, apiKV), tok, nil), 200, "", "delete again")
		c.Status(c.Do(post, c.ARM(subSandbox+"/providers/Microsoft.KeyVault/locations/uksouth/deletedVaults/"+name+"/purge", apiKV), tok, nil), 202, "", "purge")
		c.Status(c.Do(get, dv, tok, nil), 404, "DeletedVaultNotFound", "purged vault is gone")
	}}

	// ------------------------------------------------------------ Storage
	Checks["Microsoft.Storage/storageAccounts"] = Check{"name / SKU / kind rules, endpoints, keys hidden from GET, global names", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		acct := func(name string, body map[string]any) Resp {
			return c.Do(put, c.ARM(rg+"/providers/Microsoft.Storage/storageAccounts/"+name, apiST), tok, body)
		}
		c.Status(acct("Bad_Name", map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Standard_LRS"}}), 400, "AccountNameInvalid", "upper case and underscore in the name")
		c.Status(acct(strings.ReplaceAll(globalName(c, "st"), "-", ""), map[string]any{"location": "uksouth", "kind": "FileStorage", "sku": map[string]any{"name": "Standard_LRS"}}), 400, "InvalidKindForSku", "FileStorage on a Standard SKU")
		id, name := newAccount(c, tok, rg, "Standard_ZRS")
		g := c.Status(c.Do(get, c.ARM(id, apiST), tok, nil), 200, "", "read account")
		c.Expect(strings.HasPrefix(g.Str("properties.primaryEndpoints.blob"), "https://"+name+".blob.storage."), "blob endpoint is https://<name>.blob.storage.<domain>/, got %s", g.Str("properties.primaryEndpoints.blob"))
		c.Expect(dig(g.Body, "properties.allowBlobPublicAccess") == false && g.Str("properties.minimumTlsVersion") == "TLS1_2", "secure defaults: no public blob access, TLS 1.2")
		c.Expect(dig(g.Body, "properties._keys") == nil, "keys are never returned by GET")
		k := c.Status(c.Do(post, c.ARM(id+"/listKeys", apiST), tok, nil), 200, "", "list keys")
		keys, _ := k.Body["keys"].([]any)
		c.Expect(len(keys) == 2, "two account keys, got %d", len(keys))
		other, doneOther := scratchRG(c, tok, nil)
		defer doneOther()
		c.Status(c.Do(put, c.ARM(other+"/providers/Microsoft.Storage/storageAccounts/"+name, apiST), tok, map[string]any{"location": "uksouth", "sku": map[string]any{"name": "Standard_LRS"}}), 409, "StorageAccountAlreadyTaken", "same name in another resource group")
		c.Status(c.Do(del, c.ARM(id, apiST), tok, nil), 200, "", "delete account")
	}}
	Checks["Microsoft.Storage/checkNameAvailability"] = Check{"account names are global and validated", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		_, name := newAccount(c, tok, rg, "Standard_LRS")
		u := c.ARM(subSandbox+"/providers/Microsoft.Storage/checkNameAvailability", apiST)
		chk := func(n, what string) Resp {
			return c.Status(c.Do(post, u, tok, map[string]any{"name": n, "type": "Microsoft.Storage/storageAccounts"}), 200, "", what)
		}
		t := chk(name, "check a taken name")
		c.Expect(t.Body["nameAvailable"] == false && t.Str("reason") == "AlreadyExists", "taken name is not available")
		i := chk("Has-Hyphens", "check an invalid name")
		c.Expect(i.Body["nameAvailable"] == false && i.Str("reason") == "AccountNameInvalid", "invalid name is reported")
		f := chk(strings.ReplaceAll(globalName(c, "free"), "-", ""), "check a free name")
		c.Expect(f.Body["nameAvailable"] == true, "free name is available")
	}}
	Checks["Microsoft.Storage/storageAccounts/accountMigrations"] = Check{"SKU change shows as a Complete migration", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, _ := newAccount(c, tok, rg, "Standard_LRS")
		c.Status(c.Do(patch, c.ARM(id, apiST), tok, map[string]any{"sku": map[string]any{"name": "Standard_ZRS"}}), 200, "", "change redundancy LRS to ZRS")
		m := c.Status(c.Do(get, c.ARM(id+"/accountMigrations/default", apiST), tok, nil), 200, "", "read migration status")
		c.Expect(m.Str("properties.migrationStatus") == "Complete", "migrationStatus is Complete, got %s", m.Str("properties.migrationStatus"))
		c.Expect(m.Str("properties.targetSkuName") == "Standard_ZRS", "target SKU is Standard_ZRS, got %s", m.Str("properties.targetSkuName"))
		c.Status(c.Do(get, c.ARM(id+"/accountMigrations/other", apiST), tok, nil), 404, "", "only the default migration exists")
		c.Status(c.Do(put, c.ARM(id+"/accountMigrations/default", apiST), tok, map[string]any{"properties": map[string]any{"targetSkuName": "Standard_GRS"}}), 405, "MethodNotAllowed", "migrations start by changing the SKU")
	}}
	svcCheck := func(svc, field string) Check {
		return Check{svc + "/default created with the account, only 'default' is valid", func(c *C) {
			tok := c.Boot("management")
			rg, done := scratchRG(c, tok, nil)
			defer done()
			id, _ := newAccount(c, tok, rg, "Standard_LRS")
			g := c.Status(c.Do(get, c.ARM(id+"/"+svc+"/default", apiST), tok, nil), 200, "", "read "+svc+"/default")
			c.Expect(dig(g.Body, "properties."+field) != nil, "service has default %s", field)
			u := c.Status(c.Do(put, c.ARM(id+"/"+svc+"/default", apiST), tok, map[string]any{"properties": map[string]any{"cors": map[string]any{"corsRules": []any{map[string]any{"allowedOrigins": []any{"https://pay.contoso.example"}, "allowedMethods": []any{"GET"}, "allowedHeaders": []any{"*"}, "exposedHeaders": []any{"*"}, "maxAgeInSeconds": 60}}}}}), 200, "", "set CORS rules")
			c.Expect(strings.Contains(string(u.Raw), "pay.contoso.example") && dig(u.Body, "properties."+field) != nil, "CORS saved and other defaults kept")
			c.Status(c.Do(put, c.ARM(id+"/"+svc+"/custom", apiST), tok, map[string]any{"properties": map[string]any{}}), 400, "InvalidResourceName", "service name other than default")
		}}
	}
	Checks["Microsoft.Storage/storageAccounts/blobServices"] = svcCheck("blobServices", "deleteRetentionPolicy")
	Checks["Microsoft.Storage/storageAccounts/fileServices"] = svcCheck("fileServices", "shareDeleteRetentionPolicy")
	Checks["Microsoft.Storage/storageAccounts/queueServices"] = svcCheck("queueServices", "cors")
	Checks["Microsoft.Storage/storageAccounts/tableServices"] = svcCheck("tableServices", "cors")
	Checks["Microsoft.Storage/storageAccounts/blobServices/containers"] = Check{"name rules, PublicAccessNotPermitted, private by default", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, _ := newAccount(c, tok, rg, "Standard_LRS")
		base := id + "/blobServices/default/containers/"
		c.Status(c.Do(put, c.ARM(base+"Receipts_2026", apiST), tok, map[string]any{"properties": map[string]any{}}), 400, "ContainerNameInvalid", "upper case container name")
		c.Status(c.Do(put, c.ARM(base+"public", apiST), tok, map[string]any{"properties": map[string]any{"publicAccess": "Blob"}}), 409, "PublicAccessNotPermitted", "public container on an account without public blob access")
		r := c.Status(c.Do(put, c.ARM(base+"receipts", apiST), tok, map[string]any{"properties": map[string]any{}}), 201, "", "create private container")
		c.Expect(r.Str("properties.publicAccess") == "None", "publicAccess defaults to None, got %s", r.Str("properties.publicAccess"))
		l := c.Status(c.Do(get, c.ARM(strings.TrimSuffix(base, "/"), apiST), tok, nil), 200, "", "list containers").Value()
		c.Expect(HasID(l, "name", "receipts"), "list includes the container")
		c.Status(c.Do(del, c.ARM(base+"receipts", apiST), tok, nil), 200, "", "delete container")
		c.Status(c.Do(get, c.ARM(base+"receipts", apiST), tok, nil), 404, "", "container is gone")
	}}
	Checks["Microsoft.Storage/storageAccounts/fileServices/shares"] = Check{"quota bounds, default quota and tier", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, _ := newAccount(c, tok, rg, "Standard_LRS")
		base := id + "/fileServices/default/shares/"
		c.Status(c.Do(put, c.ARM(base+"settlement", apiST), tok, map[string]any{"properties": map[string]any{"shareQuota": 200000}}), 400, "InvalidShareQuota", "quota above 100 TiB")
		s := c.Status(c.Do(put, c.ARM(base+"settlement", apiST), tok, map[string]any{"properties": map[string]any{}}), 201, "", "create share")
		c.Expect(dig(s.Body, "properties.shareQuota") == float64(5120) && s.Str("properties.accessTier") == "TransactionOptimized", "default quota 5120 GiB, TransactionOptimized tier")
		u := c.Status(c.Do(put, c.ARM(base+"settlement", apiST), tok, map[string]any{"properties": map[string]any{"shareQuota": 100}}), 200, "", "resize share")
		c.Expect(dig(u.Body, "properties.shareQuota") == float64(100), "quota updated to 100 GiB")
		c.Status(c.Do(del, c.ARM(base+"settlement", apiST), tok, nil), 200, "", "delete share")
	}}
	Checks["Microsoft.Storage/storageAccounts/managementPolicies"] = Check{"lifecycle policy needs rules, only 'default'", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id, _ := newAccount(c, tok, rg, "Standard_LRS")
		rules := map[string]any{"properties": map[string]any{"policy": map[string]any{"rules": []any{map[string]any{"name": "archive-receipts", "enabled": true, "type": "Lifecycle",
			"definition": map[string]any{"filters": map[string]any{"blobTypes": []any{"blockBlob"}, "prefixMatch": []any{"receipts/"}},
				"actions": map[string]any{"baseBlob": map[string]any{"tierToArchive": map[string]any{"daysAfterModificationGreaterThan": 90}}}}}}}}}
		c.Status(c.Do(put, c.ARM(id+"/managementPolicies/other", apiST), tok, rules), 400, "InvalidResourceName", "policy name other than default")
		c.Status(c.Do(put, c.ARM(id+"/managementPolicies/default", apiST), tok, map[string]any{"properties": map[string]any{"policy": map[string]any{"rules": []any{}}}}), 400, "InvalidManagementPolicy", "policy without rules")
		p := c.Status(c.Do(put, c.ARM(id+"/managementPolicies/default", apiST), tok, rules), 200, "", "create lifecycle policy")
		c.Expect(p.Str("properties.lastModifiedTime") != "", "policy records lastModifiedTime")
		g := c.Status(c.Do(get, c.ARM(id+"/managementPolicies/default", apiST), tok, nil), 200, "", "read policy")
		c.Expect(strings.Contains(string(g.Raw), "archive-receipts"), "rule is stored")
	}}
	childCheck := func(svc, child, name string) Check {
		return Check{child + " lifecycle, parent account must exist", func(c *C) {
			tok := c.Boot("management")
			rg, done := scratchRG(c, tok, nil)
			defer done()
			c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.Storage/storageAccounts/stmissing0000/"+svc+"/default/"+child+"/"+name, apiST), tok, map[string]any{"properties": map[string]any{}}), 404, "ParentResourceNotFound", child+" in a missing account")
			id, _ := newAccount(c, tok, rg, "Standard_LRS")
			base := id + "/" + svc + "/default/" + child
			c.Status(c.Do(put, c.ARM(base+"/"+name, apiST), tok, map[string]any{"properties": map[string]any{}}), 201, "", "create "+child[:len(child)-1])
			l := c.Status(c.Do(get, c.ARM(base, apiST), tok, nil), 200, "", "list "+child).Value()
			c.Expect(len(l) == 1 && HasID(l, "name", name), "list returns the new item")
			c.Status(c.Do(del, c.ARM(base+"/"+name, apiST), tok, nil), 200, "", "delete "+child[:len(child)-1])
			c.Status(c.Do(get, c.ARM(base+"/"+name, apiST), tok, nil), 404, "", child[:len(child)-1]+" is gone")
		}}
	}
	Checks["Microsoft.Storage/storageAccounts/queueServices/queues"] = childCheck("queueServices", "queues", "payment-events")
	Checks["Microsoft.Storage/storageAccounts/tableServices/tables"] = childCheck("tableServices", "tables", "ledger")

	// ------------------------------------------------------------ PostgreSQL flexible server
	Checks["Microsoft.DBforPostgreSQL/flexibleServers"] = Check{"password complexity, no HA on Burstable, fqdn, password hidden, stop / start", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		bad := rg + "/providers/Microsoft.DBforPostgreSQL/flexibleServers/" + globalName(c, "pg")
		weak := pgBody("Burstable", "Standard_B1ms", map[string]any{"administratorLoginPassword": "password"})
		c.Status(c.Do(put, c.ARM(bad, apiPG), tok, weak), 400, "PasswordNotComplex", "weak administrator password")
		c.Status(c.Do(put, c.ARM(bad, apiPG), tok, pgBody("Burstable", "Standard_B1ms", map[string]any{"highAvailability": map[string]any{"mode": "ZoneRedundant"}})), 400, "InvalidParameterValue", "zone-redundant HA on Burstable")
		id := newPG(c, tok, rg, nil)
		name := id[strings.LastIndex(id, "/")+1:]
		g := c.Status(c.Do(get, c.ARM(id, apiPG), tok, nil), 200, "", "read server")
		c.Expect(strings.HasPrefix(g.Str("properties.fullyQualifiedDomainName"), name+".postgres."), "fqdn is <name>.postgres.<domain>, got %s", g.Str("properties.fullyQualifiedDomainName"))
		c.Expect(g.Str("properties.state") == "Ready" && g.Str("properties.network.publicNetworkAccess") == "Enabled", "server is Ready with public access")
		c.Expect(!strings.Contains(string(g.Raw), "Selftest-Passw0rd!"), "administrator password is never returned")
		c.Status(c.Do(patch, c.ARM(id, apiPG), tok, map[string]any{"properties": map[string]any{"storage": map[string]any{"storageSizeGB": 64}}}), 202, "", "grow storage (PATCH answers 202)")
		c.Status(c.Do(post, c.ARM(id+"/stop", apiPG), tok, nil), 202, "", "stop server")
		s := c.Status(c.Do(get, c.ARM(id, apiPG), tok, nil), 200, "", "read server")
		c.Expect(s.Str("properties.state") == "Stopped", "server is Stopped, got %s", s.Str("properties.state"))
		c.Expect(dig(s.Body, "properties.storage.storageSizeGB") == float64(64), "storage grew to 64 GB")
		c.Status(c.Do(patch, c.ARM(id, apiPG), tok, map[string]any{"properties": map[string]any{"storage": map[string]any{"storageSizeGB": 32}}}), 400, "InvalidParameterValue", "shrink storage")
		c.Status(c.Do(post, c.ARM(id+"/start", apiPG), tok, nil), 202, "", "start server")
		c.Status(c.Do(del, c.ARM(id, apiPG), tok, nil), 202, "", "delete server")
	}}
	Checks["Microsoft.DBforPostgreSQL/checkNameAvailability"] = Check{"server names are global and validated", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		id := newPG(c, tok, rg, nil)
		u := c.ARM(subSandbox+"/providers/Microsoft.DBforPostgreSQL/checkNameAvailability", apiPG)
		chk := func(n, what string) Resp {
			return c.Status(c.Do(post, u, tok, map[string]any{"name": n, "type": "Microsoft.DBforPostgreSQL/flexibleServers"}), 200, "", what)
		}
		t := chk(id[strings.LastIndex(id, "/")+1:], "check a taken name")
		c.Expect(t.Body["nameAvailable"] == false && t.Str("reason") == "AlreadyExists", "taken name is not available")
		i := chk("Bad_Name", "check an invalid name")
		c.Expect(i.Body["nameAvailable"] == false && i.Str("reason") == "Invalid", "invalid name is reported")
		f := chk(globalName(c, "pgfree"), "check a free name")
		c.Expect(f.Body["nameAvailable"] == true, "free name is available")
	}}
	Checks["Microsoft.DBforPostgreSQL/flexibleServers/firewallRules"] = Check{"IP range validation; rejected on a private (VNet-integrated) server", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		rule := func(start, end string) map[string]any {
			return map[string]any{"properties": map[string]any{"startIpAddress": start, "endIpAddress": end}}
		}
		pub := newPG(c, tok, rg, nil)
		c.Status(c.Do(put, c.ARM(pub+"/firewallRules/office", apiPG), tok, rule("10.0.0.9", "10.0.0.1")), 400, "InvalidParameterValue", "start after end")
		c.Status(c.Do(put, c.ARM(pub+"/firewallRules/office", apiPG), tok, rule("203.0.113.0", "203.0.113.255")), 201, "", "allow an office range on a public server")
		vnet := newVNet(c, tok, rg, "vnet-pg", "10.20.0.0/16")
		subnet := vnet + "/subnets/pg"
		c.Status(c.Do(put, c.ARM(subnet, apiNet), tok, map[string]any{"properties": map[string]any{"addressPrefix": "10.20.1.0/24",
			"delegations": []any{map[string]any{"name": "pg", "properties": map[string]any{"serviceName": "Microsoft.DBforPostgreSQL/flexibleServers"}}}}}), 201, "", "subnet delegated to PostgreSQL")
		zone := rg + "/providers/Microsoft.Network/privateDnsZones/" + c.Name("pay") + ".postgres.database.azure.com"
		c.Status(c.Do(put, c.ARM(zone, apiDNS), tok, map[string]any{"location": "global"}), 201, "", "private DNS zone")
		priv := newPG(c, tok, rg, map[string]any{"network": map[string]any{"delegatedSubnetResourceId": subnet, "privateDnsZoneArmResourceId": zone}})
		g := c.Status(c.Do(get, c.ARM(priv, apiPG), tok, nil), 200, "", "read private server")
		c.Expect(g.Str("properties.network.publicNetworkAccess") == "Disabled", "private server has public access disabled")
		c.Status(c.Do(put, c.ARM(priv+"/firewallRules/office", apiPG), tok, rule("203.0.113.0", "203.0.113.255")), 400, "FirewallRuleNotSupportedForPrivateAccess", "firewall rule on a private server")
	}}
	Checks["Microsoft.DBforPostgreSQL/flexibleServers/databases"] = Check{"reserved names, charset / collation defaults", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		srv := newPG(c, tok, rg, nil)
		c.Status(c.Do(put, c.ARM(srv+"/databases/azure_maintenance", apiPG), tok, map[string]any{"properties": map[string]any{}}), 400, "InvalidParameterValue", "reserved database name")
		d := c.Status(c.Do(put, c.ARM(srv+"/databases/payments", apiPG), tok, map[string]any{"properties": map[string]any{}}), 202, "", "create database")
		c.Expect(d.Str("properties.charset") == "UTF8" && d.Str("properties.collation") == "en_US.utf8", "UTF8 / en_US.utf8 defaults")
		l := c.Status(c.Do(get, c.ARM(srv+"/databases", apiPG), tok, nil), 200, "", "list databases").Value()
		c.Expect(HasID(l, "name", "payments"), "list includes the database")
		c.Status(c.Do(del, c.ARM(srv+"/databases/payments", apiPG), tok, nil), 202, "", "delete database")
	}}
	Checks["Microsoft.DBforPostgreSQL/flexibleServers/configurations"] = Check{"server parameters need a value, marked user-override", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		srv := newPG(c, tok, rg, nil)
		u := c.ARM(srv+"/configurations/require_secure_transport", apiPG)
		c.Status(c.Do(put, u, tok, map[string]any{"properties": map[string]any{"source": "user-override"}}), 400, "InvalidParameterValue", "parameter without a value")
		c.Status(c.Do(put, u, tok, map[string]any{"properties": map[string]any{"value": "on", "source": "user-override"}}), 202, "", "set require_secure_transport")
		g := c.Status(c.Do(get, u, tok, nil), 200, "", "read parameter")
		c.Expect(g.Str("properties.value") == "on" && g.Str("properties.source") == "user-override", "value saved as a user override")
	}}
	Checks["Microsoft.DBforPostgreSQL/flexibleServers/administrators"] = Check{"needs Entra auth on the server and a principal in the tenant", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		admin := map[string]any{"properties": map[string]any{"principalType": "ServicePrincipal", "principalName": "cloudtyke-bootstrap"}}
		pwOnly := newPG(c, tok, rg, nil)
		c.Status(c.Do(put, c.ARM(pwOnly+"/administrators/"+BootSPID, apiPG), tok, admin), 400, "InvalidParameterValue", "Entra admin on a password-only server")
		srv := newPG(c, tok, rg, map[string]any{"authConfig": map[string]any{"activeDirectoryAuth": "Enabled", "passwordAuth": "Enabled"}})
		c.Status(c.Do(put, c.ARM(srv+"/administrators/"+entraGUID(), apiPG), tok, admin), 400, "InvalidParameterValue", "principal not in the tenant")
		a := c.Status(c.Do(put, c.ARM(srv+"/administrators/"+BootSPID, apiPG), tok, admin), 202, "", "make the bootstrap service principal an Entra admin")
		c.Expect(a.Str("properties.objectId") == BootSPID && a.Str("properties.tenantId") == Tenant, "objectId and tenantId are filled in")
		l := c.Status(c.Do(get, c.ARM(srv+"/administrators", apiPG), tok, nil), 200, "", "list administrators").Value()
		c.Expect(len(l) == 1, "one administrator, got %d", len(l))
	}}

	// ------------------------------------------------------------ Cosmos DB
	Checks["Microsoft.DocumentDB/databaseAccounts"] = Check{"locations, consistency, serverless single-region, keys hidden from GET, kind immutable", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		bad := rg + "/providers/Microsoft.DocumentDB/databaseAccounts/" + globalName(c, "cosmos")
		noLoc := cosmosBody("GlobalDocumentDB", false)
		noLoc["properties"].(map[string]any)["locations"] = []any{}
		c.Status(c.Do(put, c.ARM(bad, apiCos), tok, noLoc), 400, "BadRequest", "account without locations")
		c.Status(c.Do(put, c.ARM(bad, apiCos), tok, cosmosBody("GlobalDocumentDB", true, "uksouth", "ukwest")), 400, "BadRequest", "serverless account in two regions")
		bs := cosmosBody("GlobalDocumentDB", false)
		bs["properties"].(map[string]any)["consistencyPolicy"] = map[string]any{"defaultConsistencyLevel": "BoundedStaleness", "maxIntervalInSeconds": 1}
		c.Status(c.Do(put, c.ARM(bad, apiCos), tok, bs), 400, "BadRequest", "BoundedStaleness below its limits")
		id := newCosmos(c, tok, rg, "GlobalDocumentDB", false)
		name := id[strings.LastIndex(id, "/")+1:]
		g := c.Status(c.Do(get, c.ARM(id, apiCos), tok, nil), 200, "", "read account")
		c.Expect(strings.HasPrefix(g.Str("properties.documentEndpoint"), "https://"+name+".documents."), "documentEndpoint is https://<name>.documents.<domain>, got %s", g.Str("properties.documentEndpoint"))
		c.Expect(g.Str("properties.consistencyPolicy.defaultConsistencyLevel") == "Session", "consistency defaults to Session")
		c.Expect(dig(g.Body, "properties._keys") == nil, "keys are never returned by GET")
		k := c.Status(c.Do(post, c.ARM(id+"/listKeys", apiCos), tok, nil), 200, "", "list keys")
		c.Expect(k.Str("primaryMasterKey") != "" && k.Str("primaryReadonlyMasterKey") != "", "primary and read-only keys returned")
		c.Status(c.Do(put, c.ARM(id, apiCos), tok, cosmosBody("MongoDB", false)), 400, "BadRequest", "change the API kind")
		c.Status(c.Do(del, c.ARM(id, apiCos), tok, nil), 202, "", "delete account")
	}}
	Checks["Microsoft.DocumentDB/databaseAccountNames"] = Check{"HEAD: 404 while free, 200 once taken", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		name := globalName(c, "cosmos")
		u := c.ARM("/providers/Microsoft.DocumentDB/databaseAccountNames/"+name, apiCos)
		c.Status(c.Do(head, u, tok, nil), 404, "", "name is free")
		c.Status(c.Do(put, c.ARM(rg+"/providers/Microsoft.DocumentDB/databaseAccounts/"+name, apiCos), tok, cosmosBody("GlobalDocumentDB", false)), 200, "", "create account")
		c.Status(c.Do(head, u, tok, nil), 200, "", "name is taken")
		other, doneOther := scratchRG(c, tok, nil)
		defer doneOther()
		c.Status(c.Do(put, c.ARM(other+"/providers/Microsoft.DocumentDB/databaseAccounts/"+name, apiCos), tok, cosmosBody("GlobalDocumentDB", false)), 409, "Conflict", "same name in another resource group")
	}}
	Checks["Microsoft.DocumentDB/databaseAccounts/sqlDatabases"] = Check{"NoSQL API only, no throughput on serverless", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		mongo := newCosmos(c, tok, rg, "MongoDB", false)
		cosmosChild(c, tok, mongo+"/sqlDatabases/payments", map[string]any{}, nil, 400, "BadRequest", "SQL database on a MongoDB account")
		sl := newCosmos(c, tok, rg, "GlobalDocumentDB", true)
		cosmosChild(c, tok, sl+"/sqlDatabases/payments", map[string]any{}, map[string]any{"throughput": 400}, 400, "BadRequest", "throughput on a serverless account")
		d := cosmosChild(c, tok, sl+"/sqlDatabases/payments", map[string]any{}, nil, 202, "", "create database")
		c.Expect(d.Str("properties.resource.id") == "payments", "resource.id defaults to the name, got %s", d.Str("properties.resource.id"))
		l := c.Status(c.Do(get, c.ARM(sl+"/sqlDatabases", apiCos), tok, nil), 200, "", "list databases").Value()
		c.Expect(HasID(l, "name", "payments"), "list includes the database")
	}}
	Checks["Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers"] = Check{"partition key required, default indexing policy", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		db := newCosmos(c, tok, rg, "GlobalDocumentDB", false) + "/sqlDatabases/payments"
		cosmosChild(c, tok, db, map[string]any{}, nil, 202, "", "create database")
		cosmosChild(c, tok, db+"/containers/transactions", map[string]any{}, nil, 400, "BadRequest", "container without a partition key")
		k := cosmosChild(c, tok, db+"/containers/transactions", map[string]any{"partitionKey": map[string]any{"paths": []any{"/merchantId"}}}, nil, 202, "", "create container")
		c.Expect(k.Str("properties.resource.partitionKey.kind") == "Hash", "partition key kind defaults to Hash")
		c.Expect(k.Str("properties.resource.indexingPolicy.indexingMode") == "consistent", "default consistent indexing policy")
	}}
	Checks["Microsoft.DocumentDB/databaseAccounts/mongodbDatabases"] = Check{"MongoDB API only", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		sql := newCosmos(c, tok, rg, "GlobalDocumentDB", false)
		cosmosChild(c, tok, sql+"/mongodbDatabases/ledger", map[string]any{}, nil, 400, "BadRequest", "MongoDB database on a NoSQL account")
		mongo := newCosmos(c, tok, rg, "MongoDB", false)
		g := c.Status(c.Do(get, c.ARM(mongo, apiCos), tok, nil), 200, "", "read MongoDB account")
		c.Expect(g.Str("properties.apiProperties.serverVersion") == "4.2", "server version defaults to 4.2, got %s", g.Str("properties.apiProperties.serverVersion"))
		d := cosmosChild(c, tok, mongo+"/mongodbDatabases/ledger", map[string]any{}, nil, 202, "", "create MongoDB database")
		c.Expect(d.Str("properties.resource.id") == "ledger", "resource.id defaults to the name")
	}}
	Checks["Microsoft.DocumentDB/databaseAccounts/mongodbDatabases/collections"] = Check{"_id index added by default, shard key kept", func(c *C) {
		tok := c.Boot("management")
		rg, done := scratchRG(c, tok, nil)
		defer done()
		db := newCosmos(c, tok, rg, "MongoDB", false) + "/mongodbDatabases/ledger"
		cosmosChild(c, tok, db, map[string]any{}, nil, 202, "", "create database")
		col := cosmosChild(c, tok, db+"/collections/entries", map[string]any{"shardKey": map[string]any{"merchantId": "Hash"}}, nil, 202, "", "create collection")
		c.Expect(strings.Contains(string(col.Raw), `"_id"`), "an _id index is added")
		c.Expect(col.Str("properties.resource.shardKey.merchantId") == "Hash", "shard key is kept")
	}}
	empty := map[string]any{}
	pk := map[string]any{"partitionKey": map[string]any{"paths": []any{"/merchantId"}}}
	Checks["Microsoft.DocumentDB/databaseAccounts/sqlDatabases/throughputSettings"] = throughputCheck("GlobalDocumentDB", []string{"sqlDatabases/payments"}, []map[string]any{empty})
	Checks["Microsoft.DocumentDB/databaseAccounts/sqlDatabases/containers/throughputSettings"] = throughputCheck("GlobalDocumentDB", []string{"sqlDatabases/payments", "containers/transactions"}, []map[string]any{empty, pk})
	Checks["Microsoft.DocumentDB/databaseAccounts/mongodbDatabases/throughputSettings"] = throughputCheck("MongoDB", []string{"mongodbDatabases/ledger"}, []map[string]any{empty})
	Checks["Microsoft.DocumentDB/databaseAccounts/mongodbDatabases/collections/throughputSettings"] = throughputCheck("MongoDB", []string{"mongodbDatabases/ledger", "collections/entries"}, []map[string]any{empty, empty})
}

func strOfAny(v any) string { s, _ := v.(string); return s }
