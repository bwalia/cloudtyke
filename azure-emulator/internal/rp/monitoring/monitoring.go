// Package monitoring implements Microsoft.OperationalInsights workspaces
// (Log Analytics), used by AKS Container insights, diagnostic settings and
// landing-zone management subscriptions.
package monitoring

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/managedidentity"
)

const ns = "Microsoft.OperationalInsights"

// WorkspaceType is the full type of a Log Analytics workspace.
const WorkspaceType = ns + "/workspaces"

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{2,61}[A-Za-z0-9]$`)

var skus = map[string]bool{"PerGB2018": true, "Free": true, "Standalone": true, "PerNode": true, "Standard": true, "Premium": true, "CapacityReservation": true, "LACluster": true}

// Register wires the RP.
func Register(s *arm.Server) {
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: ns, Types: []string{"workspaces"}, APIVersions: []string{"2025-02-01", "2023-09-01", "2022-10-01", "2021-12-01-preview", "2020-08-01"},
		Scopes: []arm.ScopeKind{arm.KindResourceGroup}, Phase: 2,
		Notes: "customerId (workspace id), SKU and retention (30-730 days) validation, daily cap, sharedKeys and regenerateSharedKey actions",
		Handler: arm.Generic(arm.GenericOptions{Tracked: true, Async: true,
			Prepare: prepare,
			Actions: map[string]arm.Handler{
				"sharedKeys": func(q *arm.Request) {
					m, _ := q.S.LoadResource(q.ID.Path(ns, q.Reg.Types))
					p := props(m)
					q.JSON(200, map[string]any{"primarySharedKey": p["_primaryKey"], "secondarySharedKey": p["_secondaryKey"]})
				},
				"regenerateSharedKey": func(q *arm.Request) {
					path := q.ID.Path(ns, q.Reg.Types)
					m, _ := q.S.LoadResource(path)
					p := props(m)
					p["_primaryKey"], p["_secondaryKey"] = key(), key()
					_ = q.S.SaveResource(m)
					q.JSON(200, map[string]any{"primarySharedKey": p["_primaryKey"], "secondarySharedKey": p["_secondaryKey"]})
				},
			},
			Present: func(q *arm.Request, b map[string]any) map[string]any {
				b = arm.DeepCopy(b)
				p := props(b)
				delete(p, "_primaryKey")
				delete(p, "_secondaryKey")
				return b
			},
		}),
	})
	s.Registry.Register(&arm.TypeRegistration{
		Namespace: ns, Types: []string{"deletedWorkspaces"}, APIVersions: []string{"2025-07-01", "2025-02-01", "2023-09-01", "2022-10-01", "2020-08-01"},
		Scopes: []arm.ScopeKind{arm.KindSubscription, arm.KindResourceGroup}, Phase: 2,
		Notes: "list soft-deleted workspaces (deletes are permanent in the emulator, so the list is empty)",
		Handler: func(q *arm.Request) {
			if q.R.Method != "GET" {
				q.Error(405, "MethodNotAllowed", "Use GET.")
				return
			}
			q.JSON(200, map[string]any{"value": []any{}})
		},
	})
	s.OnDeleted(WorkspaceType, func(b map[string]any) { managedidentity.CleanupSystemIdentity(s, b) })
}

func strOf(v any) string { s, _ := v.(string); return s }

func props(b map[string]any) map[string]any {
	p, _ := b["properties"].(map[string]any)
	if p == nil {
		p = map[string]any{}
		b["properties"] = p
	}
	return p
}

func key() string {
	b := make([]byte, 64)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func num(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	}
	return 0, false
}

func prepare(q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	if !nameRe.MatchString(name) {
		return arm.BadRequest("InvalidWorkspaceName", arm.Errorf("Workspace name '%s' is invalid. It must be 4-63 characters of letters, digits and '-', and start and end with a letter or digit.", name))
	}
	p := props(body)
	sku, _ := p["sku"].(map[string]any)
	if sku == nil {
		sku = map[string]any{"name": "PerGB2018"}
		p["sku"] = sku
	}
	if !skus[strOf(sku["name"])] {
		return arm.BadRequest("InvalidParameter", arm.Errorf("The SKU '%v' is not valid. Use PerGB2018 for new workspaces.", sku["name"]))
	}
	if r, ok := num(p["retentionInDays"]); ok {
		if r != -1 && (r < 30 || r > 730) && strOf(sku["name"]) != "Free" {
			return arm.BadRequest("InvalidParameter", arm.Errorf("Retention of %v days is not valid. It must be between 30 and 730 days.", r))
		}
	} else {
		p["retentionInDays"] = 30
	}
	if p["workspaceCapping"] == nil {
		p["workspaceCapping"] = map[string]any{"dailyQuotaGb": -1, "quotaNextResetTime": time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02T15:04:05Z"), "dataIngestionStatus": "RespectQuota"}
	}
	for k, v := range map[string]any{"publicNetworkAccessForIngestion": "Enabled", "publicNetworkAccessForQuery": "Enabled"} {
		if strOf(p[k]) == "" {
			p[k] = v
		}
	}
	if p["features"] == nil {
		p["features"] = map[string]any{"enableLogAccessUsingOnlyResourcePermissions": true, "legacy": 0, "searchVersion": 1}
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.0000000Z")
	if existing != nil {
		ep := props(existing)
		for _, k := range []string{"customerId", "createdDate", "_primaryKey", "_secondaryKey"} {
			p[k] = ep[k]
		}
	} else {
		p["customerId"] = entra.NewGUID()
		p["createdDate"] = now
		p["_primaryKey"], p["_secondaryKey"] = key(), key()
	}
	p["modifiedDate"] = now
	return managedidentity.ApplyIdentity(q, body, existing)
}
