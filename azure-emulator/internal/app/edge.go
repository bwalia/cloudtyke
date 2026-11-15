package app

import (
	"strings"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/store"
)

// dataPlaneExists maps a data-plane host to the ARM resource behind it and
// reports whether that resource exists:
//
//	<vault>.vault.<d>                              -> Microsoft.KeyVault/vaults/<vault>
//	<account>.{blob,file,queue,table,dfs}.storage.<d> -> Microsoft.Storage/storageAccounts/<account>
//	<registry>.azurecr.<d>                         -> Microsoft.ContainerRegistry/registries/<registry>
func dataPlaneExists(cfg config.Config, st store.Store) func(host string) bool {
	type plane struct{ suffix, typ string }
	planes := []plane{{"." + cfg.Host(config.SubVault), "microsoft.keyvault/vaults"}}
	for _, svc := range []string{"blob", "file", "queue", "table", "dfs"} {
		planes = append(planes, plane{"." + svc + "." + cfg.Host(config.SubStorage), "microsoft.storage/storageaccounts"})
	}
	planes = append(planes, plane{"." + cfg.Host(config.SubACR), "microsoft.containerregistry/registries"})
	return func(host string) bool {
		host = strings.ToLower(host)
		for _, p := range planes {
			name, ok := strings.CutSuffix(host, p.suffix)
			if !ok || name == "" || strings.Contains(name, ".") {
				continue
			}
			want := "/providers/" + p.typ + "/" + name
			found := false
			_ = st.Scan(store.BucketARM, "/subscriptions/", func(k string, _ []byte) bool {
				if strings.HasSuffix(strings.ToLower(k), want) {
					found = true
					return false
				}
				return true
			})
			return found
		}
		return false
	}
}
