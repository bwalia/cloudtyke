// Package containerservice emulates Azure Kubernetes Service
// (Microsoft.ContainerService): managed clusters, node pools, credentials,
// upgrades, maintenance configurations, the OIDC issuer and workload
// identity. Clusters are backed by a node resource group (MC_...) holding the
// kubelet / add-on identities, outbound public IP, NSG, route table and, when
// no subnet is given, an AKS-managed VNet, all as real emulated resources.
//
// Every cluster answers kubectl: either a built-in API stub (default) or,
// with CLOUDTYKE_AKS_BACKEND=k3s, a real k3s container per cluster.
package containerservice

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/arm"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/entra"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/managedidentity"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/monitoring"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/rp/resources"
)

const ns = "Microsoft.ContainerService"

// ClusterType is the full type of an AKS cluster.
const ClusterType = ns + "/managedClusters"

var apiVersions = []string{"2025-08-01", "2025-07-01", "2025-05-01", "2025-03-01", "2025-02-01", "2025-01-01", "2024-10-01", "2024-09-01", "2024-08-01", "2024-07-01", "2024-05-01", "2024-02-01", "2023-11-01", "2023-08-01"}

var (
	clusterNameRe = regexp.MustCompile(`^[a-zA-Z0-9]$|^[a-zA-Z0-9][-_a-zA-Z0-9]{0,61}[a-zA-Z0-9]$`)
	dnsPrefixRe   = regexp.MustCompile(`^[a-zA-Z0-9]$|^[a-zA-Z0-9][-a-zA-Z0-9]{0,52}[a-zA-Z0-9]$`)
	linuxPoolRe   = regexp.MustCompile(`^[a-z][a-z0-9]{0,11}$`)
	windowsPoolRe = regexp.MustCompile(`^[a-z][a-z0-9]{0,5}$`)
)

// RP is the AKS resource provider.
type RP struct {
	s       *arm.Server
	backend Backend
	pools   arm.Inline
	// CAPEM is the emulator CA, embedded in kubeconfigs.
	CAPEM []byte
}

// Register wires the RP. The backend decides what answers kubectl.
func Register(s *arm.Server, backend Backend) *RP {
	rp := &RP{s: s, backend: backend}
	rp.pools = arm.Inline{Prop: "agentPoolProfiles", ChildType: "agentPools", Flat: true, KeepMissing: true, Validate: rp.validatePool}
	reg := func(types []string, scope arm.ScopeKind, notes string, h arm.Handler) {
		s.Registry.Register(&arm.TypeRegistration{Namespace: ns, Types: types, APIVersions: apiVersions, Scopes: []arm.ScopeKind{scope}, Phase: 4, Notes: notes, Handler: h})
	}
	reg([]string{"managedClusters"}, arm.KindResourceGroup,
		"Kubernetes versions + one-minor upgrades, system/user pools, managed or user-assigned identity, kubelet identity, node resource group (MC_*) with real resources, kubenet / Azure CNI (overlay) / BYO subnet with CIDR overlap checks, UDR egress, AAD + Azure RBAC, OIDC issuer + workload identity, Container insights, credentials, stop/start, kubectl via API stub or k3s",
		arm.Generic(arm.GenericOptions{Tracked: true, Async: true,
			Prepare:    rp.prepareCluster,
			AfterWrite: rp.afterWrite,
			Present:    func(q *arm.Request, b map[string]any) map[string]any { return rp.present(b) },
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				return nil
			},
			Actions: map[string]arm.Handler{
				"listClusterUserCredential":           func(q *arm.Request) { rp.credentials(q, "clusterUser") },
				"listClusterAdminCredential":          func(q *arm.Request) { rp.credentials(q, "clusterAdmin") },
				"listClusterMonitoringUserCredential": func(q *arm.Request) { rp.credentials(q, "clusterMonitoringUser") },
				"rotateClusterCertificates":           rp.rotate,
				"stop":                                func(q *arm.Request) { rp.power(q, "Stopped") },
				"start":                               func(q *arm.Request) { rp.power(q, "Running") },
				"abort":                               func(q *arm.Request) { q.Empty(204) },
			},
		}))
	reg([]string{"managedClusters", "agentPools"}, arm.KindResourceGroup,
		"node pools: VM size, count / autoscaler bounds, System vs User mode (last System pool protected), zones, labels, taints, orchestrator version within 3 minors of the control plane, subnet must exist",
		arm.Generic(arm.GenericOptions{Async: true,
			Prepare: func(q *arm.Request, body, existing map[string]any) error {
				cluster := q.ID.ParentPath(ns, q.Reg.Types)
				c, _ := q.S.LoadResource(cluster)
				body["properties"] = flattenPool(props(body))
				return rp.validatePool(q, cluster, props(c), body, rp.pools.Siblings(q.S, cluster, strOf(body["name"])))
			},
			AfterWrite: func(q *arm.Request, body map[string]any) { rp.syncBackend(q.ID.ParentPath(ns, q.Reg.Types)) },
			OnDelete: func(q *arm.Request, existing map[string]any) error {
				if strOf(props(existing)["mode"]) != "System" {
					return nil
				}
				for _, sib := range rp.pools.Siblings(q.S, q.ID.ParentPath(ns, q.Reg.Types), strOf(existing["name"])) {
					if strOf(props(sib)["mode"]) == "System" {
						return nil
					}
				}
				return &arm.APIError{Status: 400, Code: "OperationNotAllowed", Message: "There has to be at least one system agent pool. Cannot delete the last system agent pool."}
			},
		}))
	reg([]string{"managedClusters", "maintenanceConfigurations"}, arm.KindResourceGroup,
		"planned maintenance windows: default, aksManagedAutoUpgradeSchedule, aksManagedNodeOSUpgradeSchedule",
		arm.Generic(arm.GenericOptions{Prepare: func(q *arm.Request, body, existing map[string]any) error {
			switch q.ID.Name() {
			case "default", "aksManagedAutoUpgradeSchedule", "aksManagedNodeOSUpgradeSchedule":
				return nil
			}
			return arm.BadRequest("InvalidParameter", arm.Errorf("Maintenance configuration name '%s' is not valid. Allowed: default, aksManagedAutoUpgradeSchedule, aksManagedNodeOSUpgradeSchedule.", q.ID.Name()))
		}}))
	reg([]string{"managedClusters", "upgradeProfiles"}, arm.KindResourceGroup, "GET .../upgradeProfiles/default: available control-plane and node pool upgrades", rp.upgradeProfile)
	reg([]string{"managedClusters", "agentPools", "upgradeProfiles"}, arm.KindResourceGroup, "GET .../agentPools/{pool}/upgradeProfiles/default", rp.poolUpgradeProfile)
	reg([]string{"locations", "kubernetesVersions"}, arm.KindSubscription, "az aks get-versions: supported minors, patches, default, LTS and upgrade paths", rp.kubernetesVersions)
	reg([]string{"locations", "orchestrators"}, arm.KindSubscription, "legacy orchestrator version list", rp.orchestrators)

	s.OnDeleted(ClusterType, rp.cleanup)
	return rp
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

func num(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	}
	return 0, false
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(strings.ToLower(s)))
	return hex.EncodeToString(h[:])[:8]
}

func invalid(msg string, args ...any) error {
	return arm.BadRequest("InvalidParameter", arm.Errorf(msg, args...))
}

// flattenPool accepts an agent pool body in either REST shape.
func flattenPool(p map[string]any) map[string]any { return p }

// ------------------------------------------------------------ cluster PUT

func (rp *RP) prepareCluster(q *arm.Request, body, existing map[string]any) error {
	name := q.ID.Name()
	id := q.ID.Path(ns, q.Reg.Types)
	if !clusterNameRe.MatchString(name) {
		return invalid("The value of parameter name is invalid: '%s'. It must be 1-63 characters of letters, digits, '-' and '_', starting and ending with a letter or digit.", name)
	}
	p := props(body)
	var ep map[string]any
	if existing != nil {
		ep = props(existing)
	}
	loc := strOf(body["location"])

	// immutable properties
	if ep != nil {
		for _, k := range []string{"dnsPrefix", "nodeResourceGroup", "fqdnSubdomain"} {
			if v := strOf(p[k]); v != "" && !strings.EqualFold(v, strOf(ep[k])) {
				return &arm.APIError{Status: 400, Code: "PropertyChangeNotAllowed", Message: arm.Errorf("Changing property '%s' is not allowed.", k)}
			}
		}
		for _, k := range []string{"dnsPrefix", "nodeResourceGroup", "fqdn", "azurePortalFQDN", "privateFQDN", "_adminToken", "_userToken", "_monitorToken", "_uid", "_k3s"} {
			if ep[k] != nil {
				p[k] = ep[k]
			}
		}
	}

	// platform-created state survives updates that don't echo it back
	if ep != nil {
		if p["identityProfile"] == nil && ep["identityProfile"] != nil {
			p["identityProfile"] = ep["identityProfile"]
		}
		oldAddons, _ := ep["addonProfiles"].(map[string]any)
		if newAddons, _ := p["addonProfiles"].(map[string]any); newAddons != nil {
			for k, v := range newAddons {
				m, _ := v.(map[string]any)
				key := addonKeys[k]
				if key == "" {
					key = k
				}
				if old, _ := oldAddons[key].(map[string]any); m != nil && old != nil && m["identity"] == nil && m["enabled"] == true {
					m["identity"] = old["identity"]
				}
			}
		} else if p["addonProfiles"] == nil && oldAddons != nil {
			p["addonProfiles"] = oldAddons
		}
	}

	// DNS prefix
	if strOf(p["dnsPrefix"]) == "" && strOf(p["fqdnSubdomain"]) == "" {
		return invalid("DnsPrefix or FqdnSubdomain must be specified.")
	}
	if dp := strOf(p["dnsPrefix"]); dp != "" && !dnsPrefixRe.MatchString(dp) {
		return invalid("DNSPrefix '%s' is invalid. It must contain between 1 and 54 characters, start and end with a letter or digit, and contain only letters, digits and '-'.", dp)
	}

	// Kubernetes version and upgrades
	want, err := resolveVersion(strOf(p["kubernetesVersion"]))
	if err != nil {
		return invalid("%s", err.Error())
	}
	if ep != nil {
		if strOf(p["kubernetesVersion"]) == "" {
			want = strOf(ep["currentKubernetesVersion"])
		}
		if err := checkUpgrade(strOf(ep["currentKubernetesVersion"]), want); err != nil {
			return &arm.APIError{Status: 400, Code: "UpgradeFailed", Message: err.Error()}
		}
	}
	if strOf(p["kubernetesVersion"]) == "" {
		p["kubernetesVersion"] = want
	}
	p["currentKubernetesVersion"] = want

	// identity: managed identity or service principal
	if body["identity"] == nil && p["servicePrincipalProfile"] == nil {
		return invalid("Either a managed identity (identity) or a service principal (properties.servicePrincipalProfile) is required.")
	}
	if body["identity"] != nil {
		if err := managedidentity.ApplyIdentity(q, body, existing); err != nil {
			return err
		}
		p["servicePrincipalProfile"] = map[string]any{"clientId": "msi"}
	} else {
		spp := sub(p, "servicePrincipalProfile")
		if cid := strOf(spp["clientId"]); cid != "msi" {
			if _, ok := q.S.Dir.FindBy(q.Identity.TenantID, entra.ServicePrincipals, "appId", cid); !ok {
				return &arm.APIError{Status: 400, Code: "ServicePrincipalNotFound", Message: arm.Errorf("Service principal clientID: %s not found in Active Directory tenant %s, Please see https://aka.ms/aks-sp-help for more details.", cid, q.Identity.TenantID)}
			}
		}
		delete(spp, "secret")
	}

	// node resource group
	rgName := strOf(p["nodeResourceGroup"])
	if rgName == "" {
		rgName = "MC_" + q.ID.Scope[strings.LastIndex(q.ID.Scope, "/")+1:] + "_" + name + "_" + loc
		if len(rgName) > 80 {
			rgName = rgName[:80]
		}
		p["nodeResourceGroup"] = rgName
	}

	// network
	if err := rp.prepareNetwork(q, id, p, ep); err != nil {
		return err
	}

	// agent pools: on create at least one System pool is required
	if existing == nil {
		list, _ := p["agentPoolProfiles"].([]any)
		if len(list) == 0 {
			return invalid("The value of parameter agentPoolProfiles is invalid: at least one agent pool is required.")
		}
		hasSystem := false
		for i, it := range list {
			m, _ := it.(map[string]any)
			if m == nil {
				continue
			}
			if strOf(m["mode"]) == "" && i == 0 {
				m["mode"] = "System"
			}
			if strOf(m["mode"]) == "System" {
				hasSystem = true
			}
		}
		if !hasSystem {
			return &arm.APIError{Status: 400, Code: "MustDefineAtLeastOneSystemPool", Message: "Must define at least one system pool."}
		}
	}
	if err := rp.pools.Sync(q, id, ClusterType, p); err != nil {
		return err
	}

	// AAD, local accounts, RBAC
	setDefault(p, "enableRBAC", true)
	if p["enableRBAC"] == false && p["aadProfile"] != nil {
		return invalid("Kubernetes RBAC must be enabled when AAD integration is used.")
	}
	if aad, ok := p["aadProfile"].(map[string]any); ok && aad != nil {
		if aad["managed"] != true {
			return invalid("Only AKS-managed Azure AD integration (aadProfile.managed=true) is supported; the legacy AAD integration is retired.")
		}
		setDefault(aad, "tenantID", q.Identity.TenantID)
		setDefault(aad, "enableAzureRBAC", false)
		for _, gid := range entra.StringList(aad["adminGroupObjectIDs"]) {
			if _, ok := q.S.Dir.Get(q.Identity.TenantID, entra.Groups, gid); !ok {
				return invalid("The admin group object ID '%s' was not found in tenant %s.", gid, q.Identity.TenantID)
			}
		}
	}
	setDefault(p, "disableLocalAccounts", false)
	if p["disableLocalAccounts"] == true && p["aadProfile"] == nil {
		return &arm.APIError{Status: 400, Code: "BadRequest", Message: "Local accounts can only be disabled on clusters with AKS-managed Azure AD integration."}
	}

	// OIDC issuer and workload identity
	oidc := sub(p, "oidcIssuerProfile")
	setDefault(oidc, "enabled", false)
	if ep != nil {
		if eo, _ := ep["oidcIssuerProfile"].(map[string]any); eo != nil && eo["enabled"] == true && oidc["enabled"] != true {
			return &arm.APIError{Status: 400, Code: "OperationNotAllowed", Message: "The OIDC issuer cannot be disabled once enabled."}
		}
	}
	if p["_uid"] == nil {
		p["_uid"] = entra.NewGUID()
	}
	if oidc["enabled"] == true {
		oidc["issuerURL"] = q.S.Cfg.URL(config.SubOIDC) + "/" + q.Identity.TenantID + "/" + strOf(p["_uid"]) + "/"
	} else {
		delete(oidc, "issuerURL")
	}
	sec := sub(p, "securityProfile")
	if wi, _ := sec["workloadIdentity"].(map[string]any); wi != nil && wi["enabled"] == true && oidc["enabled"] != true {
		return invalid("Enabling workload identity requires the OIDC issuer: set oidcIssuerProfile.enabled=true.")
	}

	// add-ons
	if err := rp.prepareAddons(q, p); err != nil {
		return err
	}

	// API server access
	if acc, ok := p["apiServerAccessProfile"].(map[string]any); ok && acc != nil {
		for _, r := range entra.StringList(acc["authorizedIPRanges"]) {
			if _, err := netip.ParsePrefix(r); err != nil {
				if _, err2 := netip.ParseAddr(r); err2 != nil {
					return invalid("Authorized IP range '%s' is not a valid IP address or CIDR.", r)
				}
			}
		}
		if acc["enablePrivateCluster"] == true && len(entra.StringList(acc["authorizedIPRanges"])) > 0 {
			return invalid("Authorized IP ranges are not supported on private clusters.")
		}
	}

	// SKU, upgrades, storage, misc defaults
	if body["sku"] == nil {
		body["sku"] = map[string]any{"name": "Base", "tier": "Free"}
	}
	sku := body["sku"].(map[string]any)
	setDefault(sku, "name", "Base")
	setDefault(sku, "tier", "Free")
	switch strOf(sku["tier"]) {
	case "Free", "Standard", "Premium":
	default:
		return invalid("The SKU tier '%v' is not valid. Allowed: Free, Standard, Premium.", sku["tier"])
	}
	setDefault(p, "supportPlan", "KubernetesOfficial")
	if strOf(p["supportPlan"]) == "AKSLongTermSupport" && strOf(sku["tier"]) != "Premium" {
		return invalid("Long term support (AKSLongTermSupport) requires the Premium tier.")
	}
	au := sub(p, "autoUpgradeProfile")
	setDefault(au, "nodeOSUpgradeChannel", "NodeImage")
	if c := strOf(au["upgradeChannel"]); c != "" {
		switch c {
		case "rapid", "stable", "patch", "node-image", "none":
		default:
			return invalid("Auto upgrade channel '%s' is not valid. Allowed: rapid, stable, patch, node-image, none.", c)
		}
	}
	st := sub(p, "storageProfile")
	setDefault(st, "diskCSIDriver", map[string]any{"enabled": true})
	setDefault(st, "fileCSIDriver", map[string]any{"enabled": true})
	setDefault(st, "snapshotController", map[string]any{"enabled": true})
	setDefault(p, "maxAgentPools", 100)
	setDefault(p, "powerState", map[string]any{"code": "Running"})
	setDefault(p, "workloadAutoScalerProfile", map[string]any{})
	setDefault(p, "metricsProfile", map[string]any{"costAnalysis": map[string]any{"enabled": false}})
	setDefault(p, "bootstrapProfile", map[string]any{"artifactSource": "Direct"})
	npp := sub(p, "nodeProvisioningProfile")
	setDefault(npp, "mode", "Manual")
	setDefault(npp, "defaultNodePools", "Auto")
	if m := strOf(npp["mode"]); m != "Manual" && m != "Auto" {
		return invalid("nodeProvisioningProfile.mode '%s' is not valid. Allowed: Manual, Auto.", m)
	}
	if win, ok := p["windowsProfile"].(map[string]any); ok && win != nil {
		if strOf(win["adminUsername"]) == "" {
			return invalid("windowsProfile.adminUsername is required.")
		}
		delete(win, "adminPassword")
	}

	// endpoints
	if p["fqdn"] == nil {
		prefix := strOf(p["dnsPrefix"])
		if prefix == "" {
			prefix = strOf(p["fqdnSubdomain"])
		}
		h := shortHash(id)
		p["fqdn"] = strings.ToLower(prefix) + "-" + h + "." + q.S.Cfg.Domain
		p["azurePortalFQDN"] = strings.ToLower(prefix) + "-" + h + "-portal." + q.S.Cfg.Domain
	}
	if acc, _ := p["apiServerAccessProfile"].(map[string]any); acc != nil && acc["enablePrivateCluster"] == true {
		p["privateFQDN"] = p["fqdn"]
	}
	for _, k := range []string{"_adminToken", "_userToken", "_monitorToken"} {
		if p[k] == nil {
			p[k] = newToken()
		}
	}
	p["resourceUID"] = strOf(p["_uid"])[:24]
	return nil
}

// prepareNetwork applies the network profile defaults and checks address
// ranges against the node subnet's VNet like AKS does.
func (rp *RP) prepareNetwork(q *arm.Request, id string, p, ep map[string]any) error {
	np := sub(p, "networkProfile")
	if ep != nil {
		old := sub(ep, "networkProfile")
		for _, k := range []string{"networkPlugin", "serviceCidr", "dnsServiceIP", "podCidr", "networkPluginMode", "outboundType"} {
			if v := strOf(np[k]); v != "" && !strings.EqualFold(v, strOf(old[k])) && k != "outboundType" {
				return &arm.APIError{Status: 400, Code: "PropertyChangeNotAllowed", Message: arm.Errorf("Changing property 'networkProfile.%s' is not allowed.", k)}
			}
		}
		for k, v := range old {
			if _, ok := np[k]; !ok {
				np[k] = v
			}
		}
		if lb, _ := np["loadBalancerProfile"].(map[string]any); lb != nil && lb["effectiveOutboundIPs"] == nil {
			if olb, _ := old["loadBalancerProfile"].(map[string]any); olb != nil {
				lb["effectiveOutboundIPs"] = olb["effectiveOutboundIPs"]
			}
		}
	}
	plugin := strOf(np["networkPlugin"])
	if plugin == "" {
		plugin = "azure"
		np["networkPlugin"] = plugin
		setDefault(np, "networkPluginMode", "overlay")
	}
	switch plugin {
	case "azure", "kubenet", "none":
	default:
		return invalid("Network plugin '%s' is not valid. Allowed: azure, kubenet, none.", plugin)
	}
	overlay := strings.EqualFold(strOf(np["networkPluginMode"]), "overlay")
	if plugin == "kubenet" || overlay {
		setDefault(np, "podCidr", "10.244.0.0/16")
	} else if strOf(np["podCidr"]) != "" {
		return invalid("podCidr can only be set with kubenet or Azure CNI overlay (networkPluginMode=overlay).")
	}
	if pol := strOf(np["networkPolicy"]); pol != "" {
		switch pol {
		case "calico", "azure", "cilium", "none":
		default:
			return invalid("Network policy '%s' is not valid. Allowed: azure, calico, cilium, none.", pol)
		}
		if pol == "azure" && plugin != "azure" {
			return invalid("Network policy 'azure' requires network plugin 'azure'.")
		}
		if pol == "cilium" && strOf(np["networkDataplane"]) != "cilium" {
			return invalid("Network policy 'cilium' requires networkDataplane 'cilium'.")
		}
	}
	setDefault(np, "serviceCidr", "10.0.0.0/16")
	setDefault(np, "dnsServiceIP", "10.0.0.10")
	setDefault(np, "outboundType", "loadBalancer")
	setDefault(np, "loadBalancerSku", "standard")
	setDefault(np, "ipFamilies", []any{"IPv4"})
	np["serviceCidrs"] = []any{np["serviceCidr"]}
	if np["podCidr"] != nil {
		np["podCidrs"] = []any{np["podCidr"]}
	}
	svc, err := netip.ParsePrefix(strOf(np["serviceCidr"]))
	if err != nil {
		return invalid("ServiceCidr '%v' is not a valid CIDR.", np["serviceCidr"])
	}
	dns, err := netip.ParseAddr(strOf(np["dnsServiceIP"]))
	if err != nil || !svc.Contains(dns) {
		return invalid("DNSServiceIP %v is not within ServiceCidr %s.", np["dnsServiceIP"], svc)
	}
	if pc := strOf(np["podCidr"]); pc != "" {
		pod, err := netip.ParsePrefix(pc)
		if err != nil {
			return invalid("PodCidr '%s' is not a valid CIDR.", pc)
		}
		if pod.Overlaps(svc) {
			return invalid("The PodCidr %s overlaps with the ServiceCidr %s.", pod, svc)
		}
	}
	// compare with every subnet referenced by the pools in this request or stored
	for _, sn := range rp.poolSubnets(id, p) {
		m, ok := q.S.LoadResource(sn)
		if !ok {
			continue // reported by pool validation
		}
		vnetID := sn[:strings.Index(strings.ToLower(sn), "/subnets/")]
		vnet, _ := q.S.LoadResource(vnetID)
		for _, s := range vnetPrefixes(vnet) {
			vp, err := netip.ParsePrefix(s)
			if err != nil {
				continue
			}
			if vp.Overlaps(svc) {
				return &arm.APIError{Status: 400, Code: "ServiceCidrOverlapExistingSubnetsCidr", Message: arm.Errorf("The specified service CIDR %s is conflicted with an existing subnet CIDR %s", svc, s)}
			}
			if pc := strOf(np["podCidr"]); pc != "" {
				if pod, _ := netip.ParsePrefix(pc); pod.Overlaps(vp) {
					return invalid("The PodCidr %s overlaps with the virtual network address space %s used by the cluster's subnet.", pod, s)
				}
			}
		}
		if strOf(np["outboundType"]) == "userDefinedRouting" {
			rt := props(m)["routeTable"]
			if rt == nil {
				return &arm.APIError{Status: 400, Code: "ExistingRouteTableNotAssociatedWithSubnet", Message: arm.Errorf("An existing route table has not been associated with subnet %s. Please update the route table association", sn)}
			}
		}
	}
	switch strOf(np["outboundType"]) {
	case "loadBalancer":
		lb := sub(np, "loadBalancerProfile")
		if lb["managedOutboundIPs"] == nil && lb["outboundIPs"] == nil && lb["outboundIPPrefixes"] == nil {
			lb["managedOutboundIPs"] = map[string]any{"count": 1}
		}
		setDefault(lb, "backendPoolType", "nodeIPConfiguration")
	case "userDefinedRouting", "managedNATGateway", "userAssignedNATGateway", "none":
		if len(rp.poolSubnets(id, p)) == 0 && strOf(np["outboundType"]) == "userDefinedRouting" {
			return invalid("outboundType userDefinedRouting requires node pools in an existing subnet (vnetSubnetID) with a route table.")
		}
	default:
		return invalid("Outbound type '%v' is not valid.", np["outboundType"])
	}
	return nil
}

func vnetPrefixes(vnet map[string]any) []string {
	as, _ := props(vnet)["addressSpace"].(map[string]any)
	return entra.StringList(as["addressPrefixes"])
}

// poolSubnets returns the distinct vnetSubnetIDs used by the cluster's pools.
func (rp *RP) poolSubnets(id string, p map[string]any) []string {
	set := map[string]bool{}
	if list, ok := p["agentPoolProfiles"].([]any); ok {
		for _, it := range list {
			if m, _ := it.(map[string]any); m != nil && strOf(m["vnetSubnetID"]) != "" {
				set[strOf(m["vnetSubnetID"])] = true
			}
		}
	}
	for _, c := range rp.s.Children(id + "/agentPools") {
		if v := strOf(props(c)["vnetSubnetID"]); v != "" {
			set[v] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var addonKeys = map[string]string{
	"omsagent": "omsagent", "omsAgent": "omsagent",
	"azurepolicy": "azurepolicy", "azurePolicy": "azurepolicy",
	"azureKeyvaultSecretsProvider": "azureKeyvaultSecretsProvider", "azurekeyvaultsecretsprovider": "azureKeyvaultSecretsProvider",
	"ingressApplicationGateway": "ingressApplicationGateway", "ingressapplicationgateway": "ingressApplicationGateway",
	"httpApplicationRouting": "httpApplicationRouting", "httpapplicationrouting": "httpApplicationRouting",
	"openServiceMesh": "openServiceMesh", "aciConnectorLinux": "aciConnectorLinux",
}

func (rp *RP) prepareAddons(q *arm.Request, p map[string]any) error {
	in, _ := p["addonProfiles"].(map[string]any)
	out := map[string]any{}
	for k, v := range in {
		key := addonKeys[k]
		if key == "" {
			key = k
		}
		m, _ := v.(map[string]any)
		if m == nil {
			continue
		}
		cfg, _ := m["config"].(map[string]any)
		if key == "httpApplicationRouting" && m["enabled"] == true {
			return invalid("The HTTP application routing add-on is retired. Use the application routing add-on (ingressProfile.webAppRouting) instead.")
		}
		if key == "omsagent" && m["enabled"] == true {
			ws := strOf(cfg["logAnalyticsWorkspaceResourceID"])
			if ws == "" {
				return invalid("addonProfiles.omsagent.config.logAnalyticsWorkspaceResourceID is required when Container insights is enabled.")
			}
			if w, ok := q.S.LoadResource(ws); !ok || !strings.EqualFold(strOf(w["type"]), monitoring.WorkspaceType) {
				return invalid("The Log Analytics workspace '%s' was not found.", ws)
			}
		}
		if key == "azureKeyvaultSecretsProvider" && cfg != nil {
			setDefault(cfg, "enableSecretRotation", "false")
			setDefault(cfg, "rotationPollInterval", "2m")
		}
		out[key] = m
	}
	if len(out) > 0 || in != nil {
		p["addonProfiles"] = out
	}
	return nil
}

// ------------------------------------------------------------ node pools

func (rp *RP) validatePool(q *arm.Request, clusterPath string, cp, child map[string]any, siblings []map[string]any) error {
	pp := props(child)
	name := strOf(child["name"])
	setDefault(pp, "osType", "Linux")
	windows := strOf(pp["osType"]) == "Windows"
	if windows {
		if !windowsPoolRe.MatchString(name) {
			return invalid("Windows agent pool name '%s' is invalid: it must be 1-6 lowercase letters or digits, starting with a letter.", name)
		}
	} else if !linuxPoolRe.MatchString(name) {
		return invalid("Agent pool name '%s' is invalid: it must be 1-12 lowercase letters or digits, starting with a letter.", name)
	}
	setDefault(pp, "mode", "User")
	mode := strOf(pp["mode"])
	if mode != "System" && mode != "User" && mode != "Gateway" {
		return invalid("Agent pool mode '%s' is not valid. Allowed: System, User.", mode)
	}
	if windows && mode == "System" {
		return invalid("Windows agent pools cannot be System pools.")
	}
	setDefault(pp, "vmSize", "Standard_DS2_v2")
	if !strings.HasPrefix(strOf(pp["vmSize"]), "Standard_") {
		return invalid("VM size '%v' is not valid. Use an Azure size such as Standard_D4s_v5.", pp["vmSize"])
	}
	auto := pp["enableAutoScaling"] == true
	setDefault(pp, "enableAutoScaling", false)
	count, hasCount := num(pp["count"])
	if !hasCount {
		count = 1
		if mn, ok := num(pp["minCount"]); auto && ok {
			count = mn
		}
		pp["count"] = count
	}
	if count < 0 || count > 1000 {
		return invalid("Agent pool %s count %d is out of range 0-1000.", name, count)
	}
	if mode == "System" && count < 1 {
		return invalid("System agent pool %s must have at least 1 node.", name)
	}
	if auto {
		mn, ok1 := num(pp["minCount"])
		mx, ok2 := num(pp["maxCount"])
		if !ok1 || !ok2 {
			return invalid("Agent pool %s: minCount and maxCount are required when enableAutoScaling is true.", name)
		}
		if mn > mx {
			return invalid("Agent pool %s: minCount %d must be less than or equal to maxCount %d.", name, mn, mx)
		}
		if count < mn || count > mx {
			return invalid("Agent pool %s: count %d must be between minCount %d and maxCount %d.", name, count, mn, mx)
		}
	} else if pp["minCount"] != nil || pp["maxCount"] != nil {
		if _, ok := num(pp["minCount"]); ok {
			return invalid("Agent pool %s: minCount and maxCount can only be set when enableAutoScaling is true.", name)
		}
	}
	for _, z := range entra.StringList(pp["availabilityZones"]) {
		if z != "1" && z != "2" && z != "3" {
			return invalid("Availability zone '%s' is not valid. Allowed: 1, 2, 3.", z)
		}
	}
	for _, f := range []string{"vnetSubnetID", "podSubnetID"} {
		if sn := strOf(pp[f]); sn != "" {
			m, ok := q.S.LoadResource(sn)
			if !ok || !strings.EqualFold(strOf(m["type"]), "Microsoft.Network/virtualNetworks/subnets") {
				return invalid("The subnet '%s' (%s of agent pool %s) was not found.", sn, f, name)
			}
		}
	}
	// orchestrator version: within 3 minors of the control plane, never newer
	cpVersion := strOf(cp["currentKubernetesVersion"])
	if cpVersion == "" {
		cpVersion, _ = resolveVersion(strOf(cp["kubernetesVersion"]))
	}
	ov := strOf(pp["orchestratorVersion"])
	var cur string
	if ov == "" {
		cur = cpVersion
		if old, ok := q.S.LoadResource(strOf(child["id"])); ok {
			cur = strOf(props(old)["currentOrchestratorVersion"])
		}
	} else {
		v, err := resolveVersion(ov)
		if err != nil {
			return invalid("%s", err.Error())
		}
		cur = v
	}
	if cmpVersion(cur, cpVersion) > 0 {
		return invalid("Node pool %s version %s can't be greater than the control plane version %s.", name, cur, cpVersion)
	}
	if minorOf(cpVersion)-minorOf(cur) > 3 {
		return invalid("Node pool %s version %s is more than three minor versions behind the control plane version %s.", name, cur, cpVersion)
	}
	if old, ok := q.S.LoadResource(strOf(child["id"])); ok {
		if err := checkUpgrade(strOf(props(old)["currentOrchestratorVersion"]), cur); err != nil {
			return &arm.APIError{Status: 400, Code: "UpgradeFailed", Message: err.Error()}
		}
		op := props(old)
		for _, k := range []string{"vmSize", "osType", "vnetSubnetID", "osDiskSizeGB"} {
			if op[k] != nil && pp[k] != nil && strOf(op[k]) != strOf(pp[k]) && !(k == "osDiskSizeGB") {
				return &arm.APIError{Status: 400, Code: "PropertyChangeNotAllowed", Message: arm.Errorf("Changing property '%s' of agent pool %s is not allowed. Create a new node pool instead.", k, name)}
			}
		}
	}
	if ov == "" {
		pp["orchestratorVersion"] = cur
	}
	pp["currentOrchestratorVersion"] = cur
	setDefault(pp, "osSKU", map[bool]string{true: "Windows2022", false: "Ubuntu"}[windows])
	setDefault(pp, "osDiskSizeGB", 128)
	setDefault(pp, "osDiskType", "Managed")
	setDefault(pp, "kubeletDiskType", "OS")
	setDefault(pp, "type", "VirtualMachineScaleSets")
	np := sub(cp, "networkProfile")
	maxPods := 30
	if strOf(np["networkPlugin"]) == "kubenet" || strings.EqualFold(strOf(np["networkPluginMode"]), "overlay") {
		maxPods = 250
		if strOf(np["networkPlugin"]) == "kubenet" {
			maxPods = 110
		}
	}
	setDefault(pp, "maxPods", maxPods)
	if mp, _ := num(pp["maxPods"]); mp < 10 || mp > 250 {
		return invalid("Agent pool %s maxPods %d is out of range 10-250.", name, mp)
	}
	setDefault(pp, "enableNodePublicIP", false)
	setDefault(pp, "enableEncryptionAtHost", false)
	setDefault(pp, "enableUltraSSD", false)
	setDefault(pp, "enableFIPS", false)
	setDefault(pp, "scaleDownMode", "Delete")
	setDefault(pp, "nodeLabels", map[string]any{})
	setDefault(pp, "nodeTaints", []any{})
	for _, t := range entra.StringList(pp["nodeTaints"]) {
		if !strings.Contains(t, "=") && !strings.Contains(t, ":") {
			return invalid("Node taint '%s' is not valid. Use key=value:Effect.", t)
		}
	}
	pp["nodeImageVersion"] = imageVersion(strOf(pp["osSKU"]), strOf(pp["osType"]))
	setDefault(pp, "powerState", map[string]any{"code": "Running"})
	if ps, _ := cp["powerState"].(map[string]any); ps != nil && strOf(ps["code"]) == "Stopped" {
		pp["powerState"] = map[string]any{"code": "Stopped"}
	}
	pp["provisioningState"] = "Succeeded"
	_ = siblings
	return nil
}

// ------------------------------------------------------------ node resource group

func (rp *RP) afterWrite(q *arm.Request, body map[string]any) {
	p := props(body)
	id := strOf(body["id"])
	sub := arm.ScopeSubscription(id)
	loc := strOf(body["location"])
	rgName := strOf(p["nodeResourceGroup"])
	tags := map[string]any{"aks-managed-cluster-name": body["name"], "aks-managed-cluster-rg": arm.ScopeResourceGroup(id)}
	rgID, err := resources.PutManagedResourceGroup(rp.s, sub, rgName, loc, id, nil)
	if err != nil {
		return
	}
	changed := false
	tid := q.Identity.TenantID
	// kubelet identity
	ip := sub2(p, "identityProfile")
	if ip["kubeletidentity"] == nil {
		ip["kubeletidentity"] = managedidentity.Ref(managedidentity.CreateUAI(rp.s, tid, rgID, strOf(body["name"])+"-agentpool", loc, tags))
		changed = true
	}
	// add-on identities
	if addons, ok := p["addonProfiles"].(map[string]any); ok {
		for key, v := range addons {
			m, _ := v.(map[string]any)
			if m == nil {
				continue
			}
			if m["enabled"] != true {
				delete(m, "identity")
				continue
			}
			switch key {
			case "omsagent", "azurepolicy", "azureKeyvaultSecretsProvider", "ingressApplicationGateway":
				if m["identity"] == nil {
					m["identity"] = managedidentity.Ref(managedidentity.CreateUAI(rp.s, tid, rgID, strings.ToLower(key)+"-"+strOf(body["name"]), loc, tags))
					changed = true
				}
			}
		}
	}
	// outbound public IP, NSG, route table, AKS-managed VNet
	h := shortHash(id)
	np := sub2(p, "networkProfile")
	if strOf(np["outboundType"]) == "loadBalancer" {
		lb := sub2(np, "loadBalancerProfile")
		if lb["effectiveOutboundIPs"] == nil {
			pipID := rgID + "/providers/Microsoft.Network/publicIPAddresses/" + entra.NewGUID()
			_ = rp.s.SaveResource(map[string]any{"id": pipID, "name": pipID[strings.LastIndex(pipID, "/")+1:], "type": "Microsoft.Network/publicIPAddresses",
				"location": loc, "tags": tags, "sku": map[string]any{"name": "Standard", "tier": "Regional"}, "etag": arm.NewETag(),
				"properties": map[string]any{"provisioningState": "Succeeded", "publicIPAllocationMethod": "Static", "publicIPAddressVersion": "IPv4",
					"ipAddress": fakeIP(pipID), "idleTimeoutInMinutes": 30, "ipTags": []any{}, "resourceGuid": entra.NewGUID()}})
			lb["effectiveOutboundIPs"] = []any{map[string]any{"id": pipID}}
			changed = true
		}
	}
	nsgID := rgID + "/providers/Microsoft.Network/networkSecurityGroups/aks-agentpool-" + h + "-nsg"
	if _, ok := rp.s.LoadResource(nsgID); !ok {
		_ = rp.s.SaveResource(map[string]any{"id": nsgID, "name": "aks-agentpool-" + h + "-nsg", "type": "Microsoft.Network/networkSecurityGroups", "location": loc, "tags": tags,
			"etag": arm.NewETag(), "properties": map[string]any{"provisioningState": "Succeeded", "resourceGuid": entra.NewGUID()}})
	}
	if strOf(np["networkPlugin"]) == "kubenet" {
		rtID := rgID + "/providers/Microsoft.Network/routeTables/aks-agentpool-" + h + "-routetable"
		if _, ok := rp.s.LoadResource(rtID); !ok {
			_ = rp.s.SaveResource(map[string]any{"id": rtID, "name": "aks-agentpool-" + h + "-routetable", "type": "Microsoft.Network/routeTables", "location": loc, "tags": tags,
				"etag": arm.NewETag(), "properties": map[string]any{"provisioningState": "Succeeded", "disableBgpRoutePropagation": false}})
		}
	}
	if len(rp.poolSubnets(id, p)) == 0 {
		vnetID := rgID + "/providers/Microsoft.Network/virtualNetworks/aks-vnet-" + h
		if _, ok := rp.s.LoadResource(vnetID); !ok {
			_ = rp.s.SaveResource(map[string]any{"id": vnetID, "name": "aks-vnet-" + h, "type": "Microsoft.Network/virtualNetworks", "location": loc, "tags": tags,
				"etag": arm.NewETag(), "properties": map[string]any{"provisioningState": "Succeeded", "addressSpace": map[string]any{"addressPrefixes": []any{"10.224.0.0/12"}},
					"enableDdosProtection": false, "resourceGuid": entra.NewGUID()}})
			_ = rp.s.SaveResource(map[string]any{"id": vnetID + "/subnets/aks-subnet", "name": "aks-subnet", "type": "Microsoft.Network/virtualNetworks/subnets", "etag": arm.NewETag(),
				"properties": map[string]any{"provisioningState": "Succeeded", "addressPrefix": "10.224.0.0/16", "networkSecurityGroup": map[string]any{"id": nsgID},
					"privateEndpointNetworkPolicies": "Disabled", "privateLinkServiceNetworkPolicies": "Enabled", "delegations": []any{}}})
		}
	}
	if changed {
		_ = rp.s.SaveResource(body)
	}
	rp.syncBackend(id)
}

func sub2(m map[string]any, k string) map[string]any { return sub(m, k) }

func fakeIP(id string) string {
	h := sha256.Sum256([]byte(strings.ToLower(id)))
	return "20." + itoa(int(h[0])) + "." + itoa(int(h[1])) + "." + itoa(1+int(h[2])%254)
}

func itoa(i int) string {
	const d = "0123456789"
	if i < 10 {
		return string(d[i])
	}
	return itoa(i/10) + string(d[i%10])
}

func (rp *RP) cleanup(body map[string]any) {
	p := props(body)
	id := strOf(body["id"])
	if rg := strOf(p["nodeResourceGroup"]); rg != "" {
		rp.s.DeleteTree("/subscriptions/" + arm.ScopeSubscription(id) + "/resourceGroups/" + rg)
	}
	managedidentity.CleanupSystemIdentity(rp.s, body)
	if rp.backend != nil {
		rp.backend.Delete(id)
	}
}

// ------------------------------------------------------------ GET shape

func (rp *RP) present(b map[string]any) map[string]any {
	b = arm.DeepCopy(b)
	p := props(b)
	rp.pools.Present(rp.s, strOf(b["id"]), p, nil)
	for _, k := range []string{"_adminToken", "_userToken", "_monitorToken", "_uid", "_k3s"} {
		delete(p, k)
	}
	if list, ok := p["agentPoolProfiles"].([]any); ok {
		sort.SliceStable(list, func(i, j int) bool {
			a, _ := list[i].(map[string]any)
			c, _ := list[j].(map[string]any)
			return strOf(a["mode"]) == "System" && strOf(c["mode"]) != "System"
		})
	}
	return b
}

// Cluster returns a stored cluster by id.
func (rp *RP) Cluster(id string) (map[string]any, bool) {
	m, ok := rp.s.LoadResource(id)
	if !ok || !strings.EqualFold(strOf(m["type"]), ClusterType) {
		return nil, false
	}
	return m, true
}

func (rp *RP) syncBackend(id string) {
	if rp.backend == nil {
		return
	}
	if c, ok := rp.Cluster(id); ok {
		rp.backend.Ensure(c, rp.s.Children(id+"/agentPools"))
	}
}
