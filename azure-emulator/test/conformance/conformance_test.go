// Package conformance runs the per-service self-test suite
// (internal/selftest) as one Go subtest per coverage-matrix row, so
// `go test -json` / JUnit reports list every emulated service.
package conformance

import (
	"testing"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/admin"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/app"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/selftest"
)

func TestServices(t *testing.T) {
	cfg := config.Config{Domain: "cloudtyke.local", PageSize: 100, DocsURL: "docs"}
	a, done, err := app.NewIsolated(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(done)
	runner := selftest.NewRunner(a.Handler, cfg.Domain)
	runner.Poll = 5 * time.Second
	services := admin.Services(app.Registry())

	for _, svc := range services {
		t.Run(svc.Section+"/"+svc.ID, func(t *testing.T) {
			res := runner.RunOne(svc)
			for _, c := range res.Calls {
				t.Logf("%3d %s %s %s", c.Status, c.Method, c.URL, c.Error)
			}
			if res.Status != selftest.Pass {
				t.Fatalf("%s: %s", res.Status, res.Error)
			}
		})
	}
	t.Run("NoStaleChecks", func(t *testing.T) {
		if s := selftest.StaleChecks(services); len(s) > 0 {
			t.Fatalf("checks without a coverage-matrix row: %v", s)
		}
	})
}
