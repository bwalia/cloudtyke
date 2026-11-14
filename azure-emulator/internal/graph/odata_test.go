package graph

import "testing"

func TestParseFilter(t *testing.T) {
	obj := map[string]any{
		"displayName": "Platform Engineers", "appId": "abc", "securityEnabled": true,
		"identifierUris": []any{"api://one", "api://two"}, "mail": nil,
	}
	cases := map[string]bool{
		"displayName eq 'platform engineers'":                              true,
		"displayName eq 'nope'":                                            false,
		"displayName ne 'nope'":                                            true,
		"startswith(displayName,'Plat')":                                   true,
		"appId eq 'abc' and securityEnabled eq true":                       true,
		"appId eq 'x' or securityEnabled eq true":                          true,
		"identifierUris/any(x:x eq 'api://two')":                           true,
		"identifierUris/any(x:x eq 'api://three')":                         false,
		"appId in ('x','abc')":                                             true,
		"mail eq null":                                                     true,
		"not(displayName eq 'Platform Engineers')":                         false,
		"(appId eq 'abc') and (displayName eq 'O''Brien' or true eq true)": false,
	}
	for expr, want := range cases {
		f, err := parseFilter(expr)
		if err != nil {
			t.Errorf("%s: %v", expr, err)
			continue
		}
		if got := f(obj); got != want {
			t.Errorf("%s = %v, want %v", expr, got, want)
		}
	}
	if _, err := parseFilter("displayName gt 'a'"); err == nil {
		t.Error("expected unsupported operator error")
	}
}

func TestPasswordPolicy(t *testing.T) {
	for pw, want := range map[string]bool{"short1!": false, "alllowercase": false, "Abcdefg1": true, "Passw0rd!": true, "ABCDEFGH12": false} {
		if got := passwordOK(pw); got != want {
			t.Errorf("passwordOK(%q)=%v want %v", pw, got, want)
		}
	}
}
