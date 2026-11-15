package containerservice

import (
	"net/http/httptest"
	"testing"
)

func TestKubeDataAction(t *testing.T) {
	cases := []struct{ method, path, action, ns string }{
		{"GET", "/api/v1/namespaces", "Microsoft.ContainerService/managedClusters/namespaces/read", ""},
		{"POST", "/api/v1/namespaces", "Microsoft.ContainerService/managedClusters/namespaces/write", ""},
		{"GET", "/api/v1/namespaces/payments", "Microsoft.ContainerService/managedClusters/namespaces/read", "payments"},
		{"GET", "/api/v1/namespaces/payments/pods", "Microsoft.ContainerService/managedClusters/pods/read", "payments"},
		{"GET", "/api/v1/namespaces/payments/pods/gw-1/log", "Microsoft.ContainerService/managedClusters/pods/read", "payments"},
		{"PATCH", "/apis/apps/v1/namespaces/payments/deployments/payment-gateway", "Microsoft.ContainerService/managedClusters/apps/deployments/write", "payments"},
		{"DELETE", "/api/v1/namespaces/risk/secrets/x", "Microsoft.ContainerService/managedClusters/secrets/delete", "risk"},
		{"GET", "/api/v1/nodes", "Microsoft.ContainerService/managedClusters/nodes/read", ""},
		{"GET", "/apis/rbac.authorization.k8s.io/v1/clusterroles", "Microsoft.ContainerService/managedClusters/rbac.authorization.k8s.io/clusterroles/read", ""},
	}
	for _, c := range cases {
		action, ns := kubeDataAction(httptest.NewRequest(c.method, c.path, nil))
		if action != c.action || ns != c.ns {
			t.Errorf("%s %s = %s (ns %q), want %s (ns %q)", c.method, c.path, action, ns, c.action, c.ns)
		}
	}
}
