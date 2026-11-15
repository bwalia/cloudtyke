// Package databases implements the control planes of Azure Database for
// PostgreSQL flexible servers (Microsoft.DBforPostgreSQL) and Azure Cosmos DB
// (Microsoft.DocumentDB: NoSQL and MongoDB APIs), with the validation rules
// platform teams hit in real life: private access through a delegated
// subnet and a *.postgres.database.azure.com private DNS zone, HA not on
// Burstable, backup retention, Cosmos consistency / locations / serverless.
package databases

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/managedidentity"
)

const (
	pg     = "Microsoft.DBforPostgreSQL"
	cosmos = "Microsoft.DocumentDB"
)

var pgVersions = []string{"2024-08-01", "2023-12-01-preview", "2023-06-01-preview", "2022-12-01", "2022-03-08-preview", "2021-06-01"}
var cosmosVersions = []string{"2024-11-15", "2024-08-15", "2024-05-15", "2023-11-15", "2023-04-15", "2022-11-15", "2021-10-15"}

var (
	serverNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)
	cosmosNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,42}[a-z0-9]$`)
)

func strOf(v any) string { s, _ := v.(string); return s }

func asList(v any) []any { l, _ := v.([]any); return l }

func props(b map[string]any) map[string]any {
	p, _ := b["properties"].(map[string]any)
	if p == nil {
		p = map[string]any{}
		b["properties"] = p
	}
	return p
}

func sub(m map[string]any, k string) map[string]any {
	v, _ := m[k].(map[string]any)
	if v == nil {
		v = map[string]any{}
		m[k] = v
	}
	return v
}

func setDefault(m map[string]any, k string, v any) {
	if _, ok := m[k]; !ok || m[k] == nil {
		m[k] = v
	}
}

func key() string {
	b := make([]byte, 64)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func invalid(msg string, args ...any) error {
	return arm.BadRequest("InvalidParameterValue", arm.Errorf(msg, args...))
}

// Register wires both providers.
func Register(s *arm.Server) {
	reg := func(ns string, versions []string, types []string, scope arm.ScopeKind, notes string, h arm.Handler) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: versions, Scopes: []arm.ScopeKind{scope}, Phase: 2, Notes: notes, Handler: h})
	}

	// ---------------------------------------------------------------- PostgreSQL flexible server
	reg(pg, pgVersions, []string{"flexibleServers"}, arm.KindResourceGroup,
		"versions 13-17, Burstable / GeneralPurpose / MemoryOptimized SKUs, storage and backup limits, zone-redundant HA (not on Burstable), private access via a delegated subnet + *.postgres.database.azure.com private DNS zone, Entra auth, start / stop / restart",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true, CreateStatus: 202, PatchStatus: 202, Prepare: preparePG(s),
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				delete(props(b), "administratorLoginPassword")
				return b
			},
			Actions: map[string]arm.Handler{
				"restart": pgPower(""), "stop": pgPower("Stopped"), "start": pgPower("Ready"),
			}}))
	reg(pg, pgVersions, []string{"flexibleServers", "databases"}, arm.KindResourceGroup, "databases with charset / collation",
		arm.Generic(arm.GenericOptions{Async: true, CreateStatus: 202, Prepare: func(q *arm.Request, body, existing map[string]any) error {
			p := props(body)
			setDefault(p, "charset", "UTF8")
			setDefault(p, "collation", "en_US.utf8")
			if strings.EqualFold(q.ID.Name(), "postgres") || strings.HasPrefix(strings.ToLower(q.ID.Name()), "azure_") {
				return invalid("The database name '%s' is reserved.", q.ID.Name())
			}
			return nil
		}}))
	reg(pg, pgVersions, []string{"flexibleServers", "firewallRules"}, arm.KindResourceGroup, "public-access firewall rules; rejected on private (VNet-integrated) servers",
		arm.Generic(arm.GenericOptions{Async: true, Prepare: func(q *arm.Request, body, existing map[string]any) error {
			p := props(body)
			a, e1 := netip.ParseAddr(strOf(p["startIpAddress"]))
			b, e2 := netip.ParseAddr(strOf(p["endIpAddress"]))
			if e1 != nil || e2 != nil || b.Less(a) {
				return invalid("startIpAddress and endIpAddress must be IPv4 addresses with start <= end.")
			}
			srv, _ := q.S.LoadResource(q.ID.ParentPath(pg, q.Reg.Types))
			if strOf(sub(props(srv), "network")["delegatedSubnetResourceId"]) != "" {
				return arm.BadRequest("FirewallRuleNotSupportedForPrivateAccess", "Firewall rules are not supported on servers with private access (VNet integration).")
			}
			return nil
		}}))
	reg(pg, pgVersions, []string{"flexibleServers", "configurations"}, arm.KindResourceGroup, "server parameters (e.g. require_secure_transport, log_min_duration_statement); every parameter exists, so PATCH sets it",
		arm.Generic(arm.GenericOptions{Async: true, CreateStatus: 202, PatchStatus: 202, PatchCreates: true, Prepare: func(q *arm.Request, body, existing map[string]any) error {
			p := props(body)
			// Clients "delete" a parameter by resetting it: PATCH with
			// source=system-default, or with an empty value.
			if strings.EqualFold(strOf(p["source"]), "system-default") || (q.R.Method == http.MethodPatch && strOf(p["value"]) == "") {
				p["source"] = "system-default"
				// reset to the default (what clients send when a parameter is "deleted")
				p["value"] = strOf(p["defaultValue"])
				return nil
			}
			if strOf(p["value"]) == "" {
				return invalid("Configuration '%s' needs a value.", q.ID.Name())
			}
			p["source"] = "user-override"
			setDefault(p, "dataType", "String")
			return nil
		}}))
	reg(pg, pgVersions, []string{"flexibleServers", "administrators"}, arm.KindResourceGroup, "Microsoft Entra administrators (user, group or service principal); needs Entra auth enabled",
		arm.Generic(arm.GenericOptions{Async: true, CreateStatus: 202, Prepare: func(q *arm.Request, body, existing map[string]any) error {
			srv, _ := q.S.LoadResource(q.ID.ParentPath(pg, q.Reg.Types))
			if sub(props(srv), "authConfig")["activeDirectoryAuth"] != "Enabled" {
				return invalid("Microsoft Entra authentication is not enabled on the server (authConfig.activeDirectoryAuth).")
			}
			p := props(body)
			if _, _, ok := q.S.Dir.FindObject(q.Identity.TenantID, q.ID.Name()); !ok {
				return invalid("The principal %s was not found in the tenant.", q.ID.Name())
			}
			switch strOf(p["principalType"]) {
			case "User", "Group", "ServicePrincipal":
			default:
				return invalid("principalType must be User, Group or ServicePrincipal.")
			}
			setDefault(p, "tenantId", q.Identity.TenantID)
			p["objectId"] = q.ID.Name()
			return nil
		}}))
	reg(pg, pgVersions, []string{"checkNameAvailability"}, arm.KindSubscription, "POST: server names are global", checkPG(s))

	// ---------------------------------------------------------------- Cosmos DB
	reg(cosmos, cosmosVersions, []string{"databaseAccounts"}, arm.KindResourceGroup,
		"NoSQL and MongoDB APIs, consistency levels (bounded staleness limits), locations with failover priorities, serverless / free tier, automatic failover, keys and connection strings",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true, CreateStatus: 200, Prepare: prepareCosmos(s),
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				delete(props(b), "_keys")
				return b
			},
			Actions: map[string]arm.Handler{
				"listKeys":              cosmosKeys(false),
				"readonlykeys":          cosmosKeys(true),
				"listConnectionStrings": cosmosConnStrings(s),
				"regenerateKey": func(q *arm.Request) {
					path := q.ID.Path(cosmos, q.Reg.Types)
					a, _ := q.S.LoadResource(path)
					in, _ := q.Body()
					keys := asList(props(a)["_keys"])
					idx := map[string]int{"primary": 0, "secondary": 1, "primaryReadonly": 2, "secondaryReadonly": 3}[strOf(in["keyKind"])]
					keys[idx] = key()
					_ = q.S.SaveResource(a)
					op := q.S.Ops.Start(arm.ScopeSubscription(path), path, "Succeeded", nil)
					q.AsyncHeaders(op, cosmos, strOf(a["location"]))
					q.Empty(202)
				},
			}}))
	// HEAD /providers/Microsoft.DocumentDB/databaseAccountNames/{name}: 200 when taken, 404 when free.
	reg(cosmos, cosmosVersions, []string{"databaseAccountNames"}, arm.KindTenant, "HEAD: account names are global", func(q *arm.Request) {
		if q.R.Method != http.MethodHead && q.R.Method != http.MethodGet {
			q.Error(405, "MethodNotAllowed", "The requested method is not allowed.")
			return
		}
		for _, sb := range s.H.Subscriptions() {
			for _, m := range s.ListAcrossResourceGroups(sb.ID, cosmos+"/databaseAccounts") {
				if strings.EqualFold(strOf(m["name"]), q.ID.Name()) {
					q.Empty(200)
					return
				}
			}
		}
		q.Empty(404)
	})
	child := func(types []string, notes string, prep func(q *arm.Request, body map[string]any) error) {
		reg(cosmos, cosmosVersions, types, arm.KindResourceGroup, notes, arm.Generic(arm.GenericOptions{Async: true, CreateStatus: 202,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				acct, _ := q.S.LoadResource(q.ID.Scope + "/providers/" + cosmos + "/databaseAccounts/" + q.ID.Names[0])
				api := "Sql"
				if strings.EqualFold(strOf(acct["kind"]), "MongoDB") {
					api = "MongoDB"
				}
				if strings.HasPrefix(types[1], "mongodb") && api != "MongoDB" || strings.HasPrefix(types[1], "sql") && api != "Sql" {
					return arm.BadRequest("BadRequest", arm.Errorf("Operation '%s' is not supported for this account's API (%s).", types[len(types)-1], api))
				}
				p := props(body)
				res := sub(p, "resource")
				if strOf(res["id"]) == "" {
					res["id"] = q.ID.Name()
				}
				if opts, _ := p["options"].(map[string]any); opts != nil && (opts["throughput"] != nil || opts["autoscaleSettings"] != nil) && isServerless(acct) {
					return arm.BadRequest("BadRequest", "Setting offer throughput or autopilot on resource is not supported for serverless accounts.")
				}
				if prep != nil {
					return prep(q, body)
				}
				return nil
			}}))
	}
	child([]string{"databaseAccounts", "sqlDatabases"}, "NoSQL databases (shared throughput optional)", nil)
	child([]string{"databaseAccounts", "sqlDatabases", "containers"}, "NoSQL containers: partition key required, indexing policy, TTL, unique keys", func(q *arm.Request, body map[string]any) error {
		res := sub(props(body), "resource")
		pk, _ := res["partitionKey"].(map[string]any)
		if len(asList(pk["paths"])) == 0 {
			return arm.BadRequest("BadRequest", "A partition key path is required, e.g. /merchantId.")
		}
		setDefault(pk, "kind", "Hash")
		setDefault(res, "indexingPolicy", map[string]any{"indexingMode": "consistent", "automatic": true, "includedPaths": []any{map[string]any{"path": "/*"}}, "excludedPaths": []any{map[string]any{"path": "/\"_etag\"/?"}}})
		return nil
	})
	child([]string{"databaseAccounts", "mongodbDatabases"}, "MongoDB databases", nil)
	child([]string{"databaseAccounts", "mongodbDatabases", "collections"}, "MongoDB collections: shard key, indexes (an _id index is required)", func(q *arm.Request, body map[string]any) error {
		res := sub(props(body), "resource")
		if len(asList(res["indexes"])) == 0 {
			res["indexes"] = []any{map[string]any{"key": map[string]any{"keys": []any{"_id"}}}}
		}
		return nil
	})
	for _, parent := range [][]string{{"databaseAccounts", "sqlDatabases"}, {"databaseAccounts", "sqlDatabases", "containers"}, {"databaseAccounts", "mongodbDatabases"}, {"databaseAccounts", "mongodbDatabases", "collections"}} {
		types := append(append([]string{}, parent...), "throughputSettings")
		reg(cosmos, cosmosVersions, types, arm.KindResourceGroup, "throughputSettings/default: manual or autoscale RU/s (not on serverless)", throughput(types))
	}
}

// ------------------------------------------------------------ PostgreSQL

var pgTiers = map[string]bool{"Burstable": true, "GeneralPurpose": true, "MemoryOptimized": true}

func preparePG(s *arm.Server) func(q *arm.Request, body, existing map[string]any) error {
	return func(q *arm.Request, body, existing map[string]any) error {
		name := q.ID.Name()
		if !serverNameRe.MatchString(name) {
			return invalid("Server name '%s' is invalid: use 3-63 lowercase letters, digits and hyphens.", name)
		}
		p := props(body)
		var ep map[string]any
		if existing != nil {
			ep = props(existing)
		}
		if existing == nil && strOf(p["createMode"]) != "PointInTimeRestore" && strOf(p["createMode"]) != "Replica" {
			ac := sub(p, "authConfig")
			setDefault(ac, "passwordAuth", "Enabled")
			setDefault(ac, "activeDirectoryAuth", "Disabled")
			if ac["passwordAuth"] == "Enabled" && (strOf(p["administratorLogin"]) == "" || strOf(p["administratorLoginPassword"]) == "") {
				return invalid("administratorLogin and administratorLoginPassword are required when password authentication is enabled.")
			}
			if l := strings.ToLower(strOf(p["administratorLogin"])); l == "azure_superuser" || l == "admin" || l == "postgres" || l == "root" || strings.HasPrefix(l, "pg_") {
				return invalid("The administrator login '%s' is reserved.", p["administratorLogin"])
			}
			if pw := strOf(p["administratorLoginPassword"]); pw != "" && !strongPassword(pw) {
				return arm.BadRequest("PasswordNotComplex", "The password must be 8-128 characters and contain characters from three of: uppercase, lowercase, digits, non-alphanumeric.")
			}
		}
		if ep != nil {
			for _, k := range []string{"administratorLogin", "fullyQualifiedDomainName"} {
				if ep[k] != nil {
					p[k] = ep[k]
				}
			}
		}
		delete(p, "administratorLoginPassword")
		delete(p, "createMode")
		v := strOf(p["version"])
		if v == "" {
			v = "16"
			p["version"] = v
		}
		switch v {
		case "13", "14", "15", "16", "17":
		default:
			return invalid("PostgreSQL version '%s' is not supported. Supported: 13, 14, 15, 16, 17.", v)
		}
		if ep != nil && strOf(ep["version"]) > v {
			return invalid("Downgrading the major version from %v to %s is not supported.", ep["version"], v)
		}
		p["minorVersion"] = map[string]string{"13": "18", "14": "15", "15": "10", "16": "6", "17": "2"}[v]
		sku, _ := body["sku"].(map[string]any)
		if sku == nil || !pgTiers[strOf(sku["tier"])] || !strings.HasPrefix(strOf(sku["name"]), "Standard_") {
			return invalid("sku must have a tier (Burstable, GeneralPurpose, MemoryOptimized) and a name such as Standard_D2ds_v5.")
		}
		st := sub(p, "storage")
		size, ok := st["storageSizeGB"].(float64)
		if !ok {
			size = 32
			st["storageSizeGB"] = size
		}
		allowed := map[float64]bool{32: true, 64: true, 128: true, 256: true, 512: true, 1024: true, 2048: true, 4096: true, 8192: true, 16384: true, 32767: true}
		if !allowed[size] {
			return invalid("storageSizeGB %v is not a supported size (32, 64, 128 ... 16384, 32767).", size)
		}
		if ep != nil {
			if old, _ := sub(ep, "storage")["storageSizeGB"].(float64); old > size {
				return invalid("Storage size cannot be reduced (from %v GB to %v GB).", old, size)
			}
		}
		setDefault(st, "autoGrow", "Disabled")
		setDefault(st, "tier", "P4")
		bk := sub(p, "backup")
		days, ok := bk["backupRetentionDays"].(float64)
		if !ok {
			days = 7
			bk["backupRetentionDays"] = days
		}
		if days < 7 || days > 35 {
			return invalid("backupRetentionDays must be between 7 and 35.")
		}
		setDefault(bk, "geoRedundantBackup", "Disabled")
		bk["earliestRestoreDate"] = time.Now().UTC().Format(time.RFC3339)
		ha := sub(p, "highAvailability")
		setDefault(ha, "mode", "Disabled")
		if ha["mode"] != "Disabled" && strOf(sku["tier"]) == "Burstable" {
			return invalid("High availability is not supported for the Burstable tier.")
		}
		if ha["mode"] == "ZoneRedundant" && strOf(ha["standbyAvailabilityZone"]) != "" && strOf(ha["standbyAvailabilityZone"]) == strOf(p["availabilityZone"]) {
			return invalid("The standby availability zone must differ from the primary zone for zone-redundant HA.")
		}
		if ha["mode"] != "Disabled" {
			ha["state"] = "Healthy"
		} else {
			ha["state"] = "NotEnabled"
		}
		net := sub(p, "network")
		if sn := strOf(net["delegatedSubnetResourceId"]); sn != "" {
			subnet, ok := s.LoadResource(sn)
			if !ok {
				return invalid("The delegated subnet '%s' was not found.", sn)
			}
			delegated := false
			for _, d := range asList(props(subnet)["delegations"]) {
				dm, _ := d.(map[string]any)
				if strings.EqualFold(strOf(props(dm)["serviceName"]), pg+"/flexibleServers") {
					delegated = true
				}
			}
			if !delegated {
				return arm.BadRequest("SubnetNotDelegated", arm.Errorf("Subnet '%s' must be delegated to Microsoft.DBforPostgreSQL/flexibleServers.", sn[strings.LastIndex(sn, "/")+1:]))
			}
			dz := strOf(net["privateDnsZoneArmResourceId"])
			if dz == "" {
				return invalid("A private DNS zone (privateDnsZoneArmResourceId) is required with a delegated subnet.")
			}
			if !strings.HasSuffix(strings.ToLower(dz), ".postgres.database.azure.com") {
				return arm.BadRequest("InvalidPrivateDnsZone", "The private DNS zone name must end with '.postgres.database.azure.com'.")
			}
			if _, ok := s.LoadResource(dz); !ok {
				return invalid("The private DNS zone '%s' was not found.", dz)
			}
			net["publicNetworkAccess"] = "Disabled"
		} else {
			setDefault(net, "publicNetworkAccess", "Enabled")
		}
		if err := managedidentity.ApplyIdentity(q, body, existing); err != nil {
			return err
		}
		if p["fullyQualifiedDomainName"] == nil {
			p["fullyQualifiedDomainName"] = name + ".postgres." + s.Cfg.Domain
		}
		setDefault(p, "state", "Ready")
		setDefault(p, "availabilityZone", "1")
		setDefault(p, "replicationRole", "Primary")
		setDefault(p, "replicaCapacity", 5)
		setDefault(p, "maintenanceWindow", map[string]any{"customWindow": "Disabled", "dayOfWeek": 0, "startHour": 0, "startMinute": 0})
		return nil
	}
}

func strongPassword(pw string) bool {
	if len(pw) < 8 || len(pw) > 128 {
		return false
	}
	classes := 0
	for _, re := range []string{`[A-Z]`, `[a-z]`, `[0-9]`, `[^A-Za-z0-9]`} {
		if regexp.MustCompile(re).MatchString(pw) {
			classes++
		}
	}
	return classes >= 3
}

func pgPower(state string) arm.Handler {
	return func(q *arm.Request) {
		path := q.ID.Path(pg, q.Reg.Types)
		srv, _ := q.S.LoadResource(path)
		p := props(srv)
		if state == "Stopped" && sub(p, "highAvailability")["mode"] != "Disabled" {
			q.Error(400, "InvalidParameterValue", "Stopping a server with high availability is not supported.")
			return
		}
		if state != "" {
			p["state"] = state
		} else if strOf(p["state"]) == "Stopped" {
			q.Error(400, "InvalidParameterValue", "The server is stopped; start it before restarting.")
			return
		}
		_ = q.S.SaveResource(srv)
		op := q.S.Ops.Start(arm.ScopeSubscription(path), path, "Succeeded", nil)
		q.AsyncHeaders(op, pg, strOf(srv["location"]))
		q.Empty(202)
	}
}

func checkPG(s *arm.Server) arm.Handler {
	return func(q *arm.Request) {
		in, _ := q.Body()
		name := strOf(in["name"])
		if !serverNameRe.MatchString(name) {
			q.JSON(200, map[string]any{"nameAvailable": false, "reason": "Invalid", "message": "Invalid server name."})
			return
		}
		taken := false
		for _, sb := range s.H.Subscriptions() {
			for _, m := range s.ListAcrossResourceGroups(sb.ID, pg+"/flexibleServers") {
				if strings.EqualFold(strOf(m["name"]), name) {
					taken = true
				}
			}
		}
		if taken {
			q.JSON(200, map[string]any{"nameAvailable": false, "reason": "AlreadyExists", "message": "Specified server name is already used."})
			return
		}
		q.JSON(200, map[string]any{"nameAvailable": true})
	}
}

// ------------------------------------------------------------ Cosmos DB

func isServerless(acct map[string]any) bool {
	for _, c := range asList(props(acct)["capabilities"]) {
		cm, _ := c.(map[string]any)
		if strings.EqualFold(strOf(cm["name"]), "EnableServerless") {
			return true
		}
	}
	return false
}

var consistency = map[string]bool{"Eventual": true, "Session": true, "BoundedStaleness": true, "Strong": true, "ConsistentPrefix": true}

func prepareCosmos(s *arm.Server) func(q *arm.Request, body, existing map[string]any) error {
	return func(q *arm.Request, body, existing map[string]any) error {
		name := q.ID.Name()
		if !cosmosNameRe.MatchString(name) {
			return arm.BadRequest("BadRequest", arm.Errorf("Database account name '%s' is invalid: use 3-44 lowercase letters, digits and hyphens.", name))
		}
		if existing == nil {
			for _, sb := range s.H.Subscriptions() {
				for _, m := range s.ListAcrossResourceGroups(sb.ID, cosmos+"/databaseAccounts") {
					if strings.EqualFold(strOf(m["name"]), name) {
						return &arm.APIError{Status: 409, Code: "Conflict", Message: arm.Errorf("Database account %s already exists. Account names are globally unique.", name)}
					}
				}
			}
		}
		kind := strOf(body["kind"])
		if kind == "" {
			kind = "GlobalDocumentDB"
			body["kind"] = kind
		}
		if kind != "GlobalDocumentDB" && kind != "MongoDB" {
			return arm.BadRequest("BadRequest", "kind must be GlobalDocumentDB (NoSQL) or MongoDB.")
		}
		if existing != nil && !strings.EqualFold(strOf(existing["kind"]), kind) {
			return arm.BadRequest("BadRequest", "The API kind of an account cannot be changed.")
		}
		p := props(body)
		p["databaseAccountOfferType"] = "Standard"
		cp := sub(p, "consistencyPolicy")
		setDefault(cp, "defaultConsistencyLevel", "Session")
		if !consistency[strOf(cp["defaultConsistencyLevel"])] {
			return arm.BadRequest("BadRequest", "defaultConsistencyLevel must be Eventual, Session, BoundedStaleness, Strong or ConsistentPrefix.")
		}
		if cp["defaultConsistencyLevel"] == "BoundedStaleness" {
			mi, _ := cp["maxIntervalInSeconds"].(float64)
			ms, _ := cp["maxStalenessPrefix"].(float64)
			if mi < 5 || mi > 86400 || ms < 10 || ms > 2147483647 {
				return arm.BadRequest("BadRequest", "BoundedStaleness needs maxIntervalInSeconds 5-86400 and maxStalenessPrefix 10-2147483647.")
			}
		} else {
			setDefault(cp, "maxIntervalInSeconds", 5)
			setDefault(cp, "maxStalenessPrefix", 100)
		}
		locs := asList(p["locations"])
		if len(locs) == 0 {
			return arm.BadRequest("BadRequest", "At least one location is required.")
		}
		prios := map[float64]bool{}
		var write string
		var out []any
		for _, l := range locs {
			lm, _ := l.(map[string]any)
			pr, _ := lm["failoverPriority"].(float64)
			if prios[pr] {
				return arm.BadRequest("BadRequest", "Failover priorities must be unique.")
			}
			prios[pr] = true
			loc := arm.NormalizeLocation(strOf(lm["locationName"]))
			if pr == 0 {
				write = loc
			}
			lm["id"] = name + "-" + loc
			lm["documentEndpoint"] = "https://" + name + "-" + loc + ".documents." + s.Cfg.Domain + ":443/"
			lm["provisioningState"] = "Succeeded"
			setDefault(lm, "isZoneRedundant", false)
			out = append(out, lm)
		}
		if write == "" {
			return arm.BadRequest("BadRequest", "One location must have failoverPriority 0 (the write region).")
		}
		if isServerless(body) && len(locs) > 1 {
			return arm.BadRequest("BadRequest", "Serverless accounts support a single region.")
		}
		if cp["defaultConsistencyLevel"] == "Strong" && len(locs) > 1 {
			// allowed in Azure, but call it out like the portal does
		}
		p["locations"] = out
		p["writeLocations"] = []any{out[0]}
		p["readLocations"] = out
		p["failoverPolicies"] = out
		if kind == "MongoDB" {
			api := sub(p, "apiProperties")
			setDefault(api, "serverVersion", "4.2")
			switch strOf(api["serverVersion"]) {
			case "3.2", "3.6", "4.0", "4.2", "5.0", "6.0", "7.0":
			default:
				return arm.BadRequest("BadRequest", "apiProperties.serverVersion must be 3.6, 4.0, 4.2, 5.0, 6.0 or 7.0.")
			}
		}
		for k, v := range map[string]any{"enableAutomaticFailover": false, "enableMultipleWriteLocations": false, "isVirtualNetworkFilterEnabled": false, "disableKeyBasedMetadataWriteAccess": false,
			"enableFreeTier": false, "enableAnalyticalStorage": false, "publicNetworkAccess": "Enabled", "disableLocalAuth": false, "minimalTlsVersion": "Tls12", "capabilities": []any{},
			"ipRules": []any{}, "virtualNetworkRules": []any{}, "networkAclBypass": "None", "networkAclBypassResourceIds": []any{}} {
			setDefault(p, k, v)
		}
		setDefault(p, "backupPolicy", map[string]any{"type": "Periodic", "periodicModeProperties": map[string]any{"backupIntervalInMinutes": 240, "backupRetentionIntervalInHours": 8, "backupStorageRedundancy": "Geo"}})
		p["documentEndpoint"] = "https://" + name + ".documents." + s.Cfg.Domain + ":443/"
		if existing != nil {
			ep := props(existing)
			p["_keys"], p["instanceId"] = ep["_keys"], ep["instanceId"]
		} else {
			p["_keys"] = []any{key(), key(), key(), key()}
			p["instanceId"] = entra.NewGUID()
		}
		return managedidentity.ApplyIdentity(q, body, existing)
	}
}

func cosmosKeys(readonly bool) arm.Handler {
	return func(q *arm.Request) {
		a, _ := q.S.LoadResource(q.ID.Path(cosmos, q.Reg.Types))
		if props(a)["disableLocalAuth"] == true && !readonly {
			// Azure still lists keys; requests with them fail. Mirror Azure.
		}
		k := asList(props(a)["_keys"])
		out := map[string]any{"primaryReadonlyMasterKey": k[2], "secondaryReadonlyMasterKey": k[3]}
		if !readonly {
			out["primaryMasterKey"], out["secondaryMasterKey"] = k[0], k[1]
		}
		q.JSON(200, out)
	}
}

func cosmosConnStrings(s *arm.Server) arm.Handler {
	return func(q *arm.Request) {
		a, _ := q.S.LoadResource(q.ID.Path(cosmos, q.Reg.Types))
		name := strOf(a["name"])
		k := asList(props(a)["_keys"])
		var list []any
		if strings.EqualFold(strOf(a["kind"]), "MongoDB") {
			host := name + ".mongo.cosmos." + s.Cfg.Domain + ":10255"
			for i, d := range []string{"Primary MongoDB Connection String", "Secondary MongoDB Connection String", "Primary Read-Only MongoDB Connection String", "Secondary Read-Only MongoDB Connection String"} {
				list = append(list, map[string]any{"description": d, "keyKind": []string{"Primary", "Secondary", "PrimaryReadonly", "SecondaryReadonly"}[i], "type": "MongoDB",
					"connectionString": "mongodb://" + name + ":" + url.QueryEscape(strOf(k[i])) + "@" + host + "/?ssl=true&replicaSet=globaldb&retrywrites=false&maxIdleTimeMS=120000&appName=@" + name + "@"})
			}
		} else {
			ep := strOf(props(a)["documentEndpoint"])
			for i, d := range []string{"Primary SQL Connection String", "Secondary SQL Connection String", "Primary Read-Only SQL Connection String", "Secondary Read-Only SQL Connection String"} {
				list = append(list, map[string]any{"description": d, "keyKind": []string{"Primary", "Secondary", "PrimaryReadonly", "SecondaryReadonly"}[i], "type": "Sql",
					"connectionString": "AccountEndpoint=" + ep + ";AccountKey=" + strOf(k[i]) + ";"})
			}
		}
		q.JSON(200, map[string]any{"connectionStrings": list})
	}
}

func throughput(types []string) arm.Handler {
	return func(q *arm.Request) {
		if q.ID.Name() != "default" {
			q.Error(404, "NotFound", "Only throughputSettings/default exists.")
			return
		}
		acct, _ := q.S.LoadResource(q.ID.Scope + "/providers/" + cosmos + "/databaseAccounts/" + q.ID.Names[0])
		parentPath := q.ID.ParentPath(cosmos, types)
		parent, ok := q.S.LoadResource(parentPath)
		if !ok {
			q.NotFound()
			return
		}
		if isServerless(acct) {
			q.Error(400, "BadRequest", "Reading or replacing offers is not supported for serverless accounts.")
			return
		}
		opts, _ := props(parent)["options"].(map[string]any)
		if q.R.Method == http.MethodPut {
			in, _ := q.Body()
			res := sub(props(in), "resource")
			if opts == nil {
				opts = map[string]any{}
				props(parent)["options"] = opts
			}
			if as, ok := res["autoscaleSettings"].(map[string]any); ok {
				opts["autoscaleSettings"] = map[string]any{"maxThroughput": as["maxThroughput"]}
				delete(opts, "throughput")
			} else if t, ok := res["throughput"]; ok {
				opts["throughput"] = t
				delete(opts, "autoscaleSettings")
			}
			_ = q.S.SaveResource(parent)
		}
		if opts == nil || (opts["throughput"] == nil && opts["autoscaleSettings"] == nil) {
			q.Error(404, "NotFound", "Throughput is not configured at this level (it is shared or set elsewhere).")
			return
		}
		res := map[string]any{"minimumThroughput": "400"}
		if as, ok := opts["autoscaleSettings"].(map[string]any); ok {
			res["autoscaleSettings"] = as
			res["throughput"] = 400
		} else {
			res["throughput"] = opts["throughput"]
		}
		q.JSON(200, map[string]any{"id": parentPath + "/throughputSettings/default", "name": "default", "type": strings.Join(append([]string{cosmos}, types...), "/"),
			"properties": map[string]any{"resource": res}})
	}
}
