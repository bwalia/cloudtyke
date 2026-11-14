package hierarchy

import (
	"reflect"
	"testing"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

func TestChain(t *testing.T) {
	h := New(store.NewMemory())
	_ = h.PutManagementGroup(ManagementGroup{Name: "root", TenantID: "t"})
	_ = h.PutManagementGroup(ManagementGroup{Name: "platform", TenantID: "t", Parent: "root"})
	_ = h.PutSubscription(Subscription{ID: "s1", TenantID: "t", ManagementGroup: "platform"})

	got := h.Chain("/subscriptions/s1/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v/subnets/a")
	want := []string{
		"/subscriptions/s1/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v/subnets/a",
		"/subscriptions/s1/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v",
		"/subscriptions/s1/resourceGroups/rg",
		"/subscriptions/s1",
		"/providers/Microsoft.Management/managementGroups/platform",
		"/providers/Microsoft.Management/managementGroups/root",
		"/",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chain mismatch:\n got %v\nwant %v", got, want)
	}
	if !h.IsAncestorOrSelf("/providers/Microsoft.Management/managementGroups/root", "/subscriptions/s1/resourceGroups/rg") {
		t.Fatal("root MG should be an ancestor")
	}
}
