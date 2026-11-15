package containerservice

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// KubeAPI routes requests for cluster host names (the fqdn in each
// kubeconfig) to the cluster's backend after checking its bearer token.
func (rp *RP) KubeAPI(host string) (http.Handler, bool) {
	c, ok := rp.clusterByFQDN(host)
	if !ok {
		return nil, false
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := props(c)
		if ps, _ := p["powerState"].(map[string]any); strOf(ps["code"]) == "Stopped" {
			kubeStatus(w, 503, "ServiceUnavailable", "the cluster is stopped (az aks start to start it)")
			return
		}
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		role := ""
		for k, v := range map[string]string{"_adminToken": "admin", "_userToken": "user", "_monitorToken": "monitor"} {
			if t := strOf(p[k]); t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(tok)) == 1 {
				role = v
			}
		}
		if role == "" {
			kubeStatus(w, 401, "Unauthorized", "Unauthorized")
			return
		}
		if role == "monitor" && r.Method != http.MethodGet {
			kubeStatus(w, 403, "Forbidden", "the monitoring user can only read")
			return
		}
		r.Header.Del("Authorization")
		rp.backend.Handler(c, rp.s.Children(strOf(c["id"])+"/agentPools")).ServeHTTP(w, r)
	}), true
}

func (rp *RP) clusterByFQDN(host string) (map[string]any, bool) {
	host = strings.ToLower(host)
	var found map[string]any
	_ = rp.s.Store.Scan(store.BucketARM, "/subscriptions/", func(k string, v []byte) bool {
		if !strings.Contains(k, "/providers/microsoft.containerservice/managedclusters/") || strings.Count(k[strings.Index(k, "/managedclusters/")+17:], "/") > 0 {
			return true
		}
		if !strings.Contains(strings.ToLower(string(v)), `"`+host+`"`) {
			return true
		}
		var m map[string]any
		if json.Unmarshal(v, &m) == nil && strings.EqualFold(strOf(props(m)["fqdn"]), host) {
			found = m
			return false
		}
		return true
	})
	return found, found != nil
}

func kubeStatus(w http.ResponseWriter, code int, reason, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"kind": "Status", "apiVersion": "v1", "metadata": map[string]any{}, "status": "Failure", "message": msg, "reason": reason, "code": code})
}

// ------------------------------------------------------------ stub backend

// StubBackend is a read-only Kubernetes API that reflects the AKS model:
// one Node per pool instance (with AKS labels), the system namespaces and
// the add-on pods. Enough for kubectl get / describe-style demos without
// running Kubernetes.
type StubBackend struct{}

// Name implements Backend.
func (StubBackend) Name() string { return "stub" }

// Ensure implements Backend.
func (StubBackend) Ensure(map[string]any, []map[string]any) {}

// Delete implements Backend.
func (StubBackend) Delete(string) {}

// Handler implements Backend.
func (StubBackend) Handler(c map[string]any, pools []map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := props(c)
		version := strOf(p["currentKubernetesVersion"])
		path := strings.TrimSuffix(r.URL.Path, "/")
		table := strings.Contains(r.Header.Get("Accept"), "as=Table")
		created := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
		if sd, ok := c["systemData"].(map[string]any); ok && strOf(sd["createdAt"]) != "" {
			created = strOf(sd["createdAt"])
		}
		switch {
		case path == "/version":
			writeJSON(w, map[string]any{"major": "1", "minor": fmt.Sprint(minorOf(version)), "gitVersion": "v" + version, "gitCommit": "cloudtyke", "gitTreeState": "clean",
				"buildDate": created, "goVersion": "go1.24", "compiler": "gc", "platform": "linux/amd64"})
		case path == "/api":
			writeJSON(w, map[string]any{"kind": "APIVersions", "versions": []any{"v1"}, "serverAddressByClientCIDRs": []any{map[string]any{"clientCIDR": "0.0.0.0/0", "serverAddress": strOf(p["fqdn"]) + ":443"}}})
		case path == "/apis":
			writeJSON(w, map[string]any{"kind": "APIGroupList", "apiVersion": "v1", "groups": []any{}})
		case path == "/api/v1":
			res := []any{}
			for _, n := range []struct {
				name, kind string
				namespaced bool
			}{{"nodes", "Node", false}, {"namespaces", "Namespace", false}, {"pods", "Pod", true}, {"services", "Service", true}} {
				res = append(res, map[string]any{"name": n.name, "singularName": strings.ToLower(n.kind), "namespaced": n.namespaced, "kind": n.kind, "verbs": []any{"get", "list"}})
			}
			writeJSON(w, map[string]any{"kind": "APIResourceList", "groupVersion": "v1", "resources": res})
		case path == "/api/v1/nodes":
			nodes := stubNodes(c, pools, created)
			if table {
				writeTable(w, []string{"Name", "Status", "Roles", "Age", "Version"}, nodes, func(n map[string]any) []any {
					st := n["status"].(map[string]any)
					return []any{n["metadata"].(map[string]any)["name"], st["_ready"], "<none>", age(created), "v" + version}
				})
				return
			}
			writeJSON(w, map[string]any{"kind": "NodeList", "apiVersion": "v1", "metadata": map[string]any{}, "items": nodes})
		case path == "/api/v1/namespaces":
			var items []map[string]any
			for _, n := range []string{"default", "kube-node-lease", "kube-public", "kube-system", "gatekeeper-system"} {
				items = append(items, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": n, "creationTimestamp": created}, "status": map[string]any{"phase": "Active"}})
			}
			if table {
				writeTable(w, []string{"Name", "Status", "Age"}, items, func(n map[string]any) []any {
					return []any{n["metadata"].(map[string]any)["name"], "Active", age(created)}
				})
				return
			}
			writeJSON(w, map[string]any{"kind": "NamespaceList", "apiVersion": "v1", "metadata": map[string]any{}, "items": items})
		case path == "/api/v1/pods" || strings.HasPrefix(path, "/api/v1/namespaces/") && strings.HasSuffix(path, "/pods"):
			nsName := ""
			if strings.HasPrefix(path, "/api/v1/namespaces/") {
				nsName = strings.Split(path, "/")[4]
			}
			pods := stubPods(c, created, nsName)
			if table {
				writeTable(w, []string{"Name", "Ready", "Status", "Restarts", "Age"}, pods, func(n map[string]any) []any {
					return []any{n["metadata"].(map[string]any)["name"], "1/1", "Running", 0, age(created)}
				})
				return
			}
			writeJSON(w, map[string]any{"kind": "PodList", "apiVersion": "v1", "metadata": map[string]any{}, "items": pods})
		case strings.HasPrefix(path, "/api/v1/"):
			if r.Method != http.MethodGet {
				kubeStatus(w, 405, "MethodNotAllowed", "the CloudTyke API stub is read-only; start the emulator with CLOUDTYKE_AKS_BACKEND=k3s for a real cluster")
				return
			}
			writeJSON(w, map[string]any{"kind": "List", "apiVersion": "v1", "metadata": map[string]any{}, "items": []any{}})
		default:
			kubeStatus(w, 404, "NotFound", "the server could not find the requested resource (CloudTyke API stub: set CLOUDTYKE_AKS_BACKEND=k3s for a full Kubernetes API)")
		}
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeTable(w http.ResponseWriter, cols []string, items []map[string]any, cells func(map[string]any) []any) {
	var defs, rows []any
	for _, c := range cols {
		defs = append(defs, map[string]any{"name": c, "type": "string", "format": map[bool]string{true: "name", false: ""}[c == "Name"], "description": c, "priority": 0})
	}
	for _, it := range items {
		meta := it["metadata"].(map[string]any)
		rows = append(rows, map[string]any{"cells": cells(it), "object": map[string]any{"kind": "PartialObjectMetadata", "apiVersion": "meta.k8s.io/v1", "metadata": meta}})
	}
	writeJSON(w, map[string]any{"kind": "Table", "apiVersion": "meta.k8s.io/v1", "metadata": map[string]any{}, "columnDefinitions": defs, "rows": nonNil(rows)})
}

func nonNil(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

func age(created string) string {
	t, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return "1h"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func stubNodes(c map[string]any, pools []map[string]any, created string) []map[string]any {
	var out []map[string]any
	id := strOf(c["id"])
	h := shortHash(id)
	sort.SliceStable(pools, func(i, j int) bool { return strOf(pools[i]["name"]) < strOf(pools[j]["name"]) })
	for _, pool := range pools {
		pp := props(pool)
		n, _ := num(pp["count"])
		zones := strListOf(pp["availabilityZones"])
		for i := 0; i < n; i++ {
			name := fmt.Sprintf("aks-%s-%s-vmss%06x", strOf(pool["name"]), h, i)
			labels := map[string]any{
				"agentpool": pool["name"], "kubernetes.azure.com/agentpool": pool["name"], "kubernetes.azure.com/mode": strings.ToLower(strOf(pp["mode"])),
				"kubernetes.azure.com/os-sku": pp["osSKU"], "kubernetes.azure.com/cluster": props(c)["nodeResourceGroup"],
				"kubernetes.io/os": strings.ToLower(strOf(pp["osType"])), "kubernetes.io/hostname": name, "kubernetes.io/role": "agent",
				"node.kubernetes.io/instance-type": pp["vmSize"], "topology.kubernetes.io/region": c["location"],
			}
			if len(zones) > 0 {
				labels["topology.kubernetes.io/zone"] = fmt.Sprintf("%s-%s", c["location"], zones[i%len(zones)])
			}
			if nl, ok := pp["nodeLabels"].(map[string]any); ok {
				for k, v := range nl {
					labels[k] = v
				}
			}
			out = append(out, map[string]any{
				"apiVersion": "v1", "kind": "Node",
				"metadata": map[string]any{"name": name, "labels": labels, "creationTimestamp": created},
				"spec":     map[string]any{"providerID": "azure://" + strings.ToLower(id) + "/" + name, "taints": taints(pp["nodeTaints"])},
				"status": map[string]any{"_ready": "Ready",
					"conditions": []any{map[string]any{"type": "Ready", "status": "True", "reason": "KubeletReady", "message": "kubelet is posting ready status"}},
					"nodeInfo":   map[string]any{"kubeletVersion": "v" + strOf(pp["currentOrchestratorVersion"]), "osImage": strOf(pp["nodeImageVersion"]), "containerRuntimeVersion": "containerd://1.7.27-1", "operatingSystem": strings.ToLower(strOf(pp["osType"])), "architecture": "amd64"},
					"capacity":   map[string]any{"pods": fmt.Sprint(pp["maxPods"])}},
			})
		}
	}
	return out
}

func strListOf(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			out = append(out, fmt.Sprint(x))
		}
	}
	return out
}

func taints(v any) []any {
	out := []any{}
	for _, t := range strListOf(v) {
		kv, effect, _ := strings.Cut(t, ":")
		k, val, _ := strings.Cut(kv, "=")
		out = append(out, map[string]any{"key": k, "value": val, "effect": effect})
	}
	return out
}

func stubPods(c map[string]any, created, nsName string) []map[string]any {
	p := props(c)
	type pod struct{ ns, name string }
	list := []pod{{"kube-system", "coredns-6f776c8fb5-8xk2p"}, {"kube-system", "coredns-6f776c8fb5-q7w9d"}, {"kube-system", "coredns-autoscaler-5b9c6c4f8-2vlhz"},
		{"kube-system", "konnectivity-agent-7d8f6b9b4-4tq7n"}, {"kube-system", "metrics-server-7c8f6b66d8-5kq2x"}, {"kube-system", "kube-proxy-x8b2k"},
		{"kube-system", "csi-azuredisk-node-9c7lq"}, {"kube-system", "csi-azurefile-node-f4m2s"}, {"kube-system", "cloud-node-manager-hz5r7"}}
	if np, _ := p["networkProfile"].(map[string]any); strOf(np["networkPlugin"]) == "azure" {
		list = append(list, pod{"kube-system", "azure-cns-6kqtw"}, pod{"kube-system", "azure-ip-masq-agent-v2c8j"})
	}
	if addons, _ := p["addonProfiles"].(map[string]any); addons != nil {
		if a, _ := addons["omsagent"].(map[string]any); a != nil && a["enabled"] == true {
			list = append(list, pod{"kube-system", "ama-logs-rs-6f9b7d5c4-p2k8d"}, pod{"kube-system", "ama-logs-7xq4m"})
		}
		if a, _ := addons["azurepolicy"].(map[string]any); a != nil && a["enabled"] == true {
			list = append(list, pod{"kube-system", "azure-policy-5c8d7b6f9-jx2lp"}, pod{"gatekeeper-system", "gatekeeper-controller-7b9c6d5f8-wq4zk"})
		}
		if a, _ := addons["azureKeyvaultSecretsProvider"].(map[string]any); a != nil && a["enabled"] == true {
			list = append(list, pod{"kube-system", "aks-secrets-store-csi-driver-8m2xq"}, pod{"kube-system", "aks-secrets-store-provider-azure-h6k9t"})
		}
	}
	if sec, _ := p["securityProfile"].(map[string]any); sec != nil {
		if wi, _ := sec["workloadIdentity"].(map[string]any); wi != nil && wi["enabled"] == true {
			list = append(list, pod{"kube-system", "azure-wi-webhook-controller-manager-7f9c8d6b5-r2t8v"})
		}
	}
	var out []map[string]any
	for _, x := range list {
		if nsName != "" && x.ns != nsName {
			continue
		}
		out = append(out, map[string]any{"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": x.name, "namespace": x.ns, "creationTimestamp": created},
			"status":   map[string]any{"phase": "Running"}})
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out
}
