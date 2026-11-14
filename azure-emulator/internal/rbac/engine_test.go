package rbac

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		pattern, action string
		want            bool
	}{
		{"*", "Microsoft.Compute/virtualMachines/write", true},
		{"*/read", "Microsoft.Network/virtualNetworks/read", true},
		{"*/read", "Microsoft.Network/virtualNetworks/write", false},
		{"Microsoft.Authorization/*/Write", "Microsoft.Authorization/roleAssignments/write", true},
		{"Microsoft.Authorization/*/Delete", "Microsoft.Authorization/roleAssignments/write", false},
		{"Microsoft.KeyVault/vaults/secrets/*", "Microsoft.KeyVault/vaults/secrets/getSecret/action", true},
		{"Microsoft.Network/*", "microsoft.network/virtualnetworks/subnets/join/action", true},
		{"Microsoft.Resources/subscriptions/resourceGroups/read", "Microsoft.Resources/subscriptions/resourceGroups/write", false},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.action); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.action, got, c.want)
		}
	}
}

func TestContributorCannotAssignRoles(t *testing.T) {
	c := BuiltinByID["b24988ac-6180-42a0-ab88-20f7382dd24c"]
	if !Permits(c, "Microsoft.Network/virtualNetworks/write", false) {
		t.Fatal("contributor should write vnets")
	}
	if Permits(c, "Microsoft.Authorization/roleAssignments/write", false) {
		t.Fatal("contributor must not write role assignments")
	}
	if Permits(c, "Microsoft.KeyVault/vaults/secrets/getSecret/action", true) {
		t.Fatal("contributor has no data actions")
	}
}

func TestBuiltinGUIDsUnique(t *testing.T) {
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, r := range BuiltinRoles {
		if seen[r.ID] || names[r.RoleName] {
			t.Fatalf("duplicate built-in role %s %s", r.ID, r.RoleName)
		}
		seen[r.ID], names[r.RoleName] = true, true
	}
}
