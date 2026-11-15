package selftest

import (
	"encoding/json"
	"html/template"
	"net/http"
	"sync"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/admin"
)

// Factory builds a fresh emulator handler for one run and a cleanup func.
type Factory func() (http.Handler, func(), error)

// Handler serves /_cloudtyke/selftest. Every request runs the suite against
// a new instance from factory, so the live emulator's state is untouched.
//
//	GET /_cloudtyke/selftest                 HTML report (runs the suite)
//	GET /_cloudtyke/selftest?format=json     JSON report
//	GET /_cloudtyke/selftest?service=graph   only services matching "graph"
//	GET /_cloudtyke/selftest?tour=1          step-by-step tour, one service per slide
func Handler(domain string, services []admin.Service, factory Factory) http.Handler {
	var mu sync.Mutex // one run at a time
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		h, done, err := factory()
		if err != nil {
			http.Error(w, "starting isolated emulator: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer done()
		filter := r.URL.Query().Get("service")
		rep := NewRunner(h, domain).Run(services, filter)
		status := http.StatusOK
		if rep.Failed > 0 {
			status = http.StatusExpectationFailed // 417: lets curl -f / CI fail on red
		}
		if r.URL.Query().Get("format") == "json" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			_ = enc.Encode(rep)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		if r.URL.Query().Get("tour") != "" {
			_ = tourPage.Execute(w, map[string]any{"R": rep})
			return
		}
		_ = page.Execute(w, map[string]any{"R": rep, "Filter": filter, "Sections": group(rep)})
	})
}

type section struct {
	Name    string
	Results []Result
	Passed  int
	Total   int
}

func group(rep Report) []section {
	var out []section
	idx := map[string]int{}
	for _, res := range rep.Results {
		i, ok := idx[res.Section]
		if !ok {
			i = len(out)
			idx[res.Section] = i
			out = append(out, section{Name: res.Section})
		}
		out[i].Results = append(out[i].Results, res)
		out[i].Total++
		if res.Status == Pass {
			out[i].Passed++
		}
	}
	return out
}

var sectionNames = map[string]string{"ARM": "Azure Resource Manager", "Token": "Entra ID token service", "Graph": "Microsoft Graph", "?": "Stale checks"}

var page = template.Must(template.New("selftest").Funcs(template.FuncMap{
	"title": func(s string) string {
		if n, ok := sectionNames[s]; ok {
			return n
		}
		return s
	},
}).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CloudTyke self-test</title>
<style>
:root{--bg:#f6f7f9;--card:#fff;--fg:#1d2330;--muted:#5d6678;--line:#e3e6ec;--ok:#1a7f37;--okbg:#e6f4ea;--bad:#cf222e;--badbg:#fdecee;--warn:#9a6700;--warnbg:#fff8c5}
@media (prefers-color-scheme:dark){:root{--bg:#0f1218;--card:#171b23;--fg:#e6e9ef;--muted:#9aa3b2;--line:#2a303b;--ok:#4ac26b;--okbg:#12261a;--bad:#ff7b72;--badbg:#2d1215;--warn:#d29922;--warnbg:#2b2111}}
*{box-sizing:border-box}body{margin:0;font:14px/1.5 system-ui,-apple-system,Segoe UI,sans-serif;background:var(--bg);color:var(--fg)}
main{max-width:1100px;margin:0 auto;padding:28px 20px 60px}
h1{font-size:22px;margin:0 0 4px}.sub{color:var(--muted);margin:0 0 20px}
.bar{display:flex;gap:12px;flex-wrap:wrap;align-items:center;margin-bottom:22px}
.tile{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:12px 16px;min-width:130px}
.tile b{display:block;font-size:24px;font-variant-numeric:tabular-nums}.tile span{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.04em}
.ok{color:var(--ok)}.bad{color:var(--bad)}
.btn.alt{margin-left:0;background:var(--card);color:var(--fg);border:1px solid var(--line)}.btn{margin-left:auto;background:var(--fg);color:var(--bg);border:0;border-radius:8px;padding:10px 16px;font-weight:600;text-decoration:none}
form{display:flex;gap:8px}input{padding:9px 10px;border:1px solid var(--line);border-radius:8px;background:var(--card);color:var(--fg)}
section{background:var(--card);border:1px solid var(--line);border-radius:10px;margin-bottom:18px;overflow:hidden}
section h2{font-size:15px;margin:0;padding:12px 16px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between}
details{border-top:1px solid var(--line)}details:first-of-type{border-top:0}
summary{display:grid;grid-template-columns:70px minmax(290px,1.1fr) 2fr 70px;gap:12px;align-items:center;padding:10px 16px;cursor:pointer;list-style:none}
summary::-webkit-details-marker{display:none}
.pill{font-size:12px;font-weight:600;border-radius:999px;padding:2px 8px;text-align:center}
.pill.pass{background:var(--okbg);color:var(--ok)}.pill.fail{background:var(--badbg);color:var(--bad)}.pill.missing,.pill.stale{background:var(--warnbg);color:var(--warn)}
code{font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace}.muted{color:var(--muted)}.num{text-align:right;font-variant-numeric:tabular-nums;color:var(--muted)}
.body{padding:4px 16px 14px 98px}.err{background:var(--badbg);color:var(--bad);padding:10px 12px;border-radius:8px;white-space:pre-wrap;margin:6px 0 10px}
table{border-collapse:collapse;width:100%}td{padding:3px 8px 3px 0;vertical-align:top;border-bottom:1px dashed var(--line)}td.s{width:48px;font-variant-numeric:tabular-nums}
</style></head><body><main>
<h1>CloudTyke self-test</h1>
<p class="sub">Every service in the coverage matrix, exercised over its real HTTP API against a fresh in-memory emulator. Your emulator's state is not touched.</p>
<div class="bar">
 <div class="tile"><b class="{{if .R.Failed}}bad{{else}}ok{{end}}">{{if .R.Failed}}FAILED{{else}}PASSED{{end}}</b><span>result</span></div>
 <div class="tile"><b class="ok">{{.R.Passed}}</b><span>services passed</span></div>
 <div class="tile"><b class="{{if .R.Failed}}bad{{end}}">{{.R.Failed}}</b><span>failed</span></div>
 <div class="tile"><b>{{.R.DurationMs}} ms</b><span>{{.R.Started.Format "15:04:05 UTC"}}</span></div>
 <form method="get"><input name="service" placeholder="filter, e.g. graph" value="{{.Filter}}"></form>
 <a class="btn" href="?tour=1{{if .Filter}}&amp;service={{.Filter}}{{end}}">Take the tour</a>
 <a class="btn alt" href="?{{if .Filter}}service={{.Filter}}{{end}}">Run again</a>
</div>
{{range .Sections}}<section><h2><span>{{title .Name}}</span><span class="muted">{{.Passed}}/{{.Total}}</span></h2>
{{range .Results}}<details{{if ne .Status "pass"}} open{{end}}><summary>
<span class="pill {{.Status}}">{{.Status}}</span><code>{{.Service}}</code><span class="muted">{{.Title}}</span><span class="num">{{len .Calls}} call{{if ne (len .Calls) 1}}s{{end}}</span></summary>
<div class="body">{{if .Error}}<div class="err">{{.Error}}</div>{{end}}
{{if .Calls}}<table>{{range .Calls}}<tr><td class="s {{if ge .Status 500}}bad{{end}}">{{.Status}}</td><td>{{if .Step}}<b>{{.Step}}</b><br>{{end}}<code>{{.Method}} {{.URL}}</code>{{if .Error}} <span class="muted">{{.Error}}</span>{{end}}</td></tr>{{end}}</table>{{end}}
<p class="muted">{{.DurationMs}} ms</p></div></details>{{end}}
</section>{{end}}
<p class="muted">JSON: <code>?format=json</code> · returns HTTP 417 when anything fails.</p>
</main></body></html>`))
