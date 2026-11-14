package arm

import (
	"strings"
	"testing"
)

func TestParseProviderPath(t *testing.T) {
	seg := strings.Split("subscriptions/s/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v/subnets/a/providers/Microsoft.Authorization/roleAssignments/x", "/")
	id, ok := ParseProviderPath(seg)
	if !ok {
		t.Fatal("not parsed")
	}
	if id.Namespace != "Microsoft.Authorization" || id.TypeKey() != "microsoft.authorization/roleassignments" || id.Name() != "x" {
		t.Fatalf("bad parse %+v", id)
	}
	if id.Scope != "/subscriptions/s/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v/subnets/a" {
		t.Fatalf("bad scope %s", id.Scope)
	}
	coll, _ := ParseProviderPath(strings.Split("subscriptions/s/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v/subnets", "/"))
	if !coll.IsCollection() || coll.ParentPath("Microsoft.Network", []string{"virtualNetworks", "subnets"}) != "/subscriptions/s/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/v" {
		t.Fatalf("bad collection parse %+v", coll)
	}
}

func TestMergePatch(t *testing.T) {
	dst := map[string]any{"tags": map[string]any{"a": "1"}, "properties": map[string]any{"x": 1.0, "y": 2.0}}
	mergePatch(dst, map[string]any{"tags": map[string]any{"b": "2"}, "properties": map[string]any{"y": nil}})
	if tags := dst["tags"].(map[string]any); len(tags) != 1 || tags["b"] != "2" {
		t.Fatalf("tags should be replaced: %v", tags)
	}
	if p := dst["properties"].(map[string]any); p["x"] != 1.0 || p["y"] != nil {
		t.Fatalf("properties merge wrong: %v", p)
	}
}
