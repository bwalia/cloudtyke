package containerservice

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
)

// Backend runs what kubectl talks to for each cluster.
type Backend interface {
	// Name is "stub" or "k3s".
	Name() string
	// Ensure makes the cluster's API server match the stored state (create,
	// stop/start, pool changes). It must be quick; slow work runs async.
	Ensure(cluster map[string]any, pools []map[string]any)
	// Delete tears the cluster's API server down.
	Delete(clusterID string)
	// Handler serves the Kubernetes API for an authenticated request.
	Handler(cluster map[string]any, pools []map[string]any) http.Handler
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ------------------------------------------------------------ credentials

func (rp *RP) credentials(q *arm.Request, kind string) {
	c, _ := q.S.LoadResource(q.ID.Path(ns, q.Reg.Types))
	p := props(c)
	if kind != "clusterUser" && p["disableLocalAccounts"] == true {
		q.Error(400, "BadRequest", "Getting static credential is not allowed because this cluster is set to disable local accounts.")
		return
	}
	format := strings.ToLower(q.R.URL.Query().Get("format"))
	cfg := rp.kubeconfig(c, kind, format)
	q.JSON(200, map[string]any{"kubeconfigs": []any{map[string]any{"name": kind, "value": base64.StdEncoding.EncodeToString([]byte(cfg))}}})
}

// kubeconfig renders a kubeconfig for the cluster. AAD clusters give users
// an exec (kubelogin) config like AKS; admin / local accounts get a token.
func (rp *RP) kubeconfig(c map[string]any, kind, format string) string {
	p := props(c)
	name := strOf(c["name"])
	server := "https://" + strOf(p["fqdn"]) + ":443"
	ca := base64.StdEncoding.EncodeToString(rp.caPEM())
	user := kind + "_" + rgOf(strOf(c["id"])) + "_" + name
	ctx := name
	if kind == "clusterAdmin" {
		ctx = name + "-admin"
	}
	var auth string
	aad, _ := p["aadProfile"].(map[string]any)
	switch {
	case kind == "clusterUser" && aad != nil && format != "azure":
		auth = fmt.Sprintf(`    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: kubelogin
      args:
      - get-token
      - --login
      - devicecode
      - --server-id
      - 6dae42f8-4368-4678-94ff-3960e28e3630
      - --client-id
      - 80faf920-1908-4b52-b5ef-a8e7bedfc67a
      - --tenant-id
      - %s
      - --environment
      - AzurePublicCloud
      env: null
      installHint: |
        kubelogin is not installed which is required to connect to AAD enabled cluster.
        To learn more, please go to https://aka.ms/aks/kubelogin
      provideClusterInfo: false
      interactiveMode: IfNeeded
    # CloudTyke: AAD sign-in is not emulated; use the admin credential
    # (az aks get-credentials --admin) or listClusterAdminCredential instead.
`, strOf(aad["tenantID"]))
	case kind == "clusterAdmin":
		auth = "    token: " + strOf(p["_adminToken"]) + "\n"
	case kind == "clusterMonitoringUser":
		auth = "    token: " + strOf(p["_monitorToken"]) + "\n"
	default:
		auth = "    token: " + strOf(p["_userToken"]) + "\n"
	}
	return fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
preferences: {}
users:
- name: %s
  user:
%s`, ca, server, name, name, user, ctx, ctx, user, auth)
}

func rgOf(id string) string {
	parts := strings.Split(id, "/")
	for i, p := range parts {
		if strings.EqualFold(p, "resourceGroups") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (rp *RP) caPEM() []byte { return rp.CAPEM }

func (rp *RP) rotate(q *arm.Request) {
	path := q.ID.Path(ns, q.Reg.Types)
	c, _ := q.S.LoadResource(path)
	p := props(c)
	for _, k := range []string{"_adminToken", "_userToken", "_monitorToken"} {
		p[k] = newToken()
	}
	_ = q.S.SaveResource(c)
	op := q.S.Ops.Start(arm.ScopeSubscription(path), path, "Succeeded", nil)
	q.AsyncHeaders(op, ns, strOf(c["location"]))
	q.Empty(202)
}

func (rp *RP) power(q *arm.Request, code string) {
	path := q.ID.Path(ns, q.Reg.Types)
	c, _ := q.S.LoadResource(path)
	p := props(c)
	p["powerState"] = map[string]any{"code": code}
	_ = q.S.SaveResource(c)
	for _, pool := range q.S.Children(path + "/agentPools") {
		props(pool)["powerState"] = map[string]any{"code": code}
		_ = q.S.SaveResource(pool)
	}
	rp.syncBackend(path)
	op := q.S.Ops.Start(arm.ScopeSubscription(path), path, "Succeeded", nil)
	q.AsyncHeaders(op, ns, strOf(c["location"]))
	q.Empty(202)
}

// ------------------------------------------------------------ upgrade profiles

func upgradesList(v string) []any {
	out := []any{}
	for _, u := range upgradesFrom(v) {
		out = append(out, map[string]any{"kubernetesVersion": u, "isPreview": false})
	}
	return out
}

func (rp *RP) upgradeProfile(q *arm.Request) {
	if q.R.Method != http.MethodGet || q.ID.Name() != "default" {
		q.Error(404, "NotFound", "Only GET .../upgradeProfiles/default is supported.")
		return
	}
	cluster := q.ID.ParentPath(ns, q.Reg.Types)
	c, ok := q.S.LoadResource(cluster)
	if !ok {
		q.NotFound()
		return
	}
	p := props(c)
	var poolProfiles []any
	for _, pool := range q.S.Children(cluster + "/agentPools") {
		pp := props(pool)
		poolProfiles = append(poolProfiles, map[string]any{"name": pool["name"], "kubernetesVersion": pp["currentOrchestratorVersion"], "osType": pp["osType"], "upgrades": upgradesList(strOf(pp["currentOrchestratorVersion"]))})
	}
	q.JSON(200, map[string]any{
		"id": cluster + "/upgradeprofiles/default", "name": "default", "type": ns + "/managedClusters/upgradeprofiles",
		"properties": map[string]any{
			"controlPlaneProfile": map[string]any{"kubernetesVersion": p["currentKubernetesVersion"], "name": nil, "osType": "Linux", "upgrades": upgradesList(strOf(p["currentKubernetesVersion"]))},
			"agentPoolProfiles":   poolProfiles,
		},
	})
}

func (rp *RP) poolUpgradeProfile(q *arm.Request) {
	if q.R.Method != http.MethodGet || q.ID.Name() != "default" {
		q.Error(404, "NotFound", "Only GET .../upgradeProfiles/default is supported.")
		return
	}
	poolID := q.ID.ParentPath(ns, q.Reg.Types)
	pool, ok := q.S.LoadResource(poolID)
	if !ok {
		q.NotFound()
		return
	}
	pp := props(pool)
	q.JSON(200, map[string]any{
		"id": poolID + "/upgradeProfiles/default", "name": "default", "type": ns + "/managedClusters/agentPools/upgradeProfiles",
		"properties": map[string]any{"kubernetesVersion": pp["currentOrchestratorVersion"], "osType": pp["osType"], "upgrades": upgradesList(strOf(pp["currentOrchestratorVersion"])), "latestNodeImageVersion": pp["nodeImageVersion"]},
	})
}

// ------------------------------------------------------------ versions

func (rp *RP) kubernetesVersions(q *arm.Request) {
	if q.R.Method != http.MethodGet {
		q.Error(405, "MethodNotAllowed", "Use GET.")
		return
	}
	var values []any
	for _, v := range Versions {
		patches := map[string]any{}
		for _, p := range v.Patches {
			ups := []any{}
			for _, u := range upgradesFrom(p) {
				ups = append(ups, u)
			}
			patches[p] = map[string]any{"upgrades": ups}
		}
		plans := []any{"KubernetesOfficial"}
		if v.LTS {
			plans = append(plans, "AKSLongTermSupport")
		}
		values = append(values, map[string]any{"version": v.Minor, "isDefault": v.Default, "isPreview": false,
			"capabilities": map[string]any{"supportPlan": plans}, "patchVersions": patches})
	}
	q.JSON(200, map[string]any{"values": values})
}

func (rp *RP) orchestrators(q *arm.Request) {
	var list []any
	for _, v := range Versions {
		for _, p := range v.Patches {
			ups := []any{}
			for _, u := range upgradesFrom(p) {
				ups = append(ups, map[string]any{"orchestratorType": "Kubernetes", "orchestratorVersion": u})
			}
			list = append(list, map[string]any{"orchestratorType": "Kubernetes", "orchestratorVersion": p, "default": p == DefaultVersion(), "isPreview": false, "upgrades": ups})
		}
	}
	loc := ""
	if len(q.ID.Names) > 0 {
		loc = q.ID.Names[0]
	}
	q.JSON(200, map[string]any{"id": "/subscriptions/" + q.SubscriptionID() + "/providers/" + ns + "/locations/" + loc + "/orchestrators", "name": "default", "type": ns + "/locations/orchestrators",
		"properties": map[string]any{"orchestrators": list}})
}
