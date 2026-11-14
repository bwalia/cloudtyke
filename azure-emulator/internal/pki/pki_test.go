package pki

import "testing"

func TestLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	hosts := []string{"cloudtyke.local", "*.cloudtyke.local", "*.vault.cloudtyke.local", "localhost", "127.0.0.1"}
	b, err := LoadOrCreate(dir, "", hosts)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []string{"management.cloudtyke.local", "myvault.vault.cloudtyke.local", "localhost"} {
		if err := b.VerifyChain(h); err != nil {
			t.Errorf("%s: %v", h, err)
		}
	}
	// reload keeps the same CA
	b2, err := LoadOrCreate(dir, "", hosts)
	if err != nil {
		t.Fatal(err)
	}
	if !b.Equal(b2.CAPEM) {
		t.Fatal("CA changed on reload")
	}
}
