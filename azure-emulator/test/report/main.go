// Command report turns `go test -json` output and Terraform e2e logs into a
// test report: JUnit XML (CI), a standalone HTML page, a Markdown summary
// (GitHub step summary) and a terminal table. It exits 1 if anything failed.
//
//	go run ./test/report -in reports/go-test.json \
//	    -e2e OpenTofu=reports/e2e-tofu.log -e2e Terraform=reports/e2e-terraform.log -out reports
package main

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/selftest"
)

const servicePrefix = "TestServices/"

type event struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

// Case is one leaf test (or e2e example).
type Case struct {
	Suite   string // Go package or "e2e/<tool>"
	Name    string
	Status  string // pass, fail, skip
	Seconds float64
	Output  string
}

// Service is a conformance result for one coverage-matrix row.
type Service struct {
	Section, ID, Title, Status string
	Seconds                    float64
	Output                     string
}

// Group summarises a set of cases.
type Group struct {
	Name                string
	Passed, Failed, All int
	Cases               []Case
	Note                string
}

type report struct {
	Generated time.Time
	Services  []Service
	Sections  []Group // per-section service counts
	Go        []Group // per-package unit / integration results
	E2E       []Group
	Failed    int
	Total     int
}

type e2eFlag []string

func (e *e2eFlag) String() string     { return strings.Join(*e, ",") }
func (e *e2eFlag) Set(v string) error { *e = append(*e, v); return nil }

func main() {
	in := flag.String("in", "reports/go-test.json", "go test -json output (empty: none)")
	out := flag.String("out", "reports", "output directory")
	var e2e e2eFlag
	flag.Var(&e2e, "e2e", "Name=path/to/run.sh.log (repeatable; missing files are reported as not run)")
	flag.Parse()

	cases, err := readGoTest(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		os.Exit(2)
	}
	r := build(cases)
	for _, spec := range e2e {
		name, path, _ := strings.Cut(spec, "=")
		r.E2E = append(r.E2E, readE2E(name, path))
	}
	for _, g := range append(append([]Group{}, r.Go...), r.E2E...) {
		r.Failed += g.Failed
		r.Total += g.All
	}

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		os.Exit(2)
	}
	must(writeJUnit(filepath.Join(*out, "junit.xml"), r))
	must(writeHTML(filepath.Join(*out, "index.html"), r))
	must(os.WriteFile(filepath.Join(*out, "summary.md"), []byte(markdown(r)), 0o644))
	fmt.Print(terminal(r))
	fmt.Printf("\nReport: %s  (JUnit: %s)\n", filepath.Join(*out, "index.html"), filepath.Join(*out, "junit.xml"))
	if r.Failed > 0 {
		os.Exit(1)
	}
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		os.Exit(2)
	}
}

// ------------------------------------------------------------------ inputs

func readGoTest(path string) ([]Case, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	type key struct{ pkg, test string }
	out := map[key]*strings.Builder{}
	res := map[key]*Case{}
	var order []key
	parents := map[key]bool{}
	pkgFail := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var e event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.Test == "" {
			// package-level: build failures, panics outside tests
			if e.Action == "output" {
				pkgFail[e.Package] += e.Output
			}
			if e.Action == "fail" {
				k := key{e.Package, ""}
				if res[k] == nil {
					res[k] = &Case{Suite: e.Package, Name: "(package)", Status: "fail", Output: pkgFail[e.Package]}
					order = append(order, k)
				}
			}
			continue
		}
		k := key{e.Package, e.Test}
		if i := strings.LastIndex(e.Test, "/"); i > 0 {
			parents[key{e.Package, e.Test[:i]}] = true
		}
		switch e.Action {
		case "run":
			if res[k] == nil {
				res[k] = &Case{Suite: e.Package, Name: e.Test}
				out[k] = &strings.Builder{}
				order = append(order, k)
			}
		case "output":
			if out[k] != nil {
				out[k].WriteString(e.Output)
			}
		case "pass", "fail", "skip":
			if res[k] == nil {
				res[k] = &Case{Suite: e.Package, Name: e.Test}
				order = append(order, k)
			}
			res[k].Status, res[k].Seconds = e.Action, e.Elapsed
		}
	}
	var cases []Case
	for _, k := range order {
		if parents[k] {
			continue // only leaf tests are reported
		}
		c := *res[k]
		if out[k] != nil {
			c.Output = out[k].String()
		}
		if c.Status == "" {
			c.Status = "fail" // started but never finished: timeout / crash
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

var e2eExample = regexp.MustCompile(`^=== \[\w+\] (\S+)`)
var e2eFailed = regexp.MustCompile(`^!!! (\S+) failed`)

func readE2E(name, path string) Group {
	g := Group{Name: "Terraform e2e (" + name + ")"}
	b, err := os.ReadFile(path)
	if err != nil {
		g.Note = "not run"
		return g
	}
	var cur string
	idx := map[string]int{}
	for _, line := range strings.Split(string(b), "\n") {
		if m := e2eExample.FindStringSubmatch(line); m != nil {
			cur = m[1]
			idx[cur] = len(g.Cases)
			g.Cases = append(g.Cases, Case{Suite: "e2e/" + name, Name: cur, Status: "pass"})
			continue
		}
		if m := e2eFailed.FindStringSubmatch(line); m != nil {
			if i, ok := idx[m[1]]; ok {
				g.Cases[i].Status = "fail"
			}
			continue
		}
		if cur != "" {
			c := &g.Cases[idx[cur]]
			c.Output += line + "\n"
		}
	}
	if len(g.Cases) == 0 {
		g.Cases = append(g.Cases, Case{Suite: "e2e/" + name, Name: "(run)", Status: "fail", Output: string(b)})
	}
	for _, c := range g.Cases {
		g.count(c.Status)
	}
	return g
}

func (g *Group) count(status string) {
	g.All++
	switch status {
	case "pass":
		g.Passed++
	case "fail":
		g.Failed++
	}
}

func build(cases []Case) *report {
	r := &report{Generated: time.Now()}
	pkgs := map[string]*Group{}
	var pkgOrder []string
	secIdx := map[string]int{}
	for _, c := range cases {
		if strings.HasPrefix(c.Name, servicePrefix) && c.Name != servicePrefix+"NoStaleChecks" {
			sec, id, _ := strings.Cut(strings.TrimPrefix(c.Name, servicePrefix), "/")
			svc := Service{Section: sec, ID: id, Status: c.Status, Seconds: c.Seconds, Output: c.Output}
			if chk, ok := selftest.Checks[id]; ok {
				svc.Title = chk.Title
			}
			r.Services = append(r.Services, svc)
			i, ok := secIdx[sec]
			if !ok {
				i = len(r.Sections)
				secIdx[sec] = i
				r.Sections = append(r.Sections, Group{Name: sec})
			}
			r.Sections[i].count(c.Status)
		}
		g := pkgs[c.Suite]
		if g == nil {
			g = &Group{Name: c.Suite}
			pkgs[c.Suite] = g
			pkgOrder = append(pkgOrder, c.Suite)
		}
		g.Cases = append(g.Cases, c)
		g.count(c.Status)
	}
	sort.Strings(pkgOrder)
	for _, p := range pkgOrder {
		r.Go = append(r.Go, *pkgs[p])
	}
	return r
}

// ------------------------------------------------------------------ outputs

func icon(status string) string {
	switch status {
	case "pass":
		return "✅"
	case "skip":
		return "⏭️"
	}
	return "❌"
}

func short(pkg string) string {
	return strings.TrimPrefix(pkg, "github.com/bwalia/cloudtyke/azure-emulator/")
}

func terminal(r *report) string {
	var b strings.Builder
	b.WriteString("\nCloudTyke test report\n=====================\n")
	if len(r.Services) > 0 {
		b.WriteString("\nServices (one conformance check per coverage-matrix row)\n")
	}
	sec := ""
	for _, s := range r.Services {
		if s.Section != sec {
			sec = s.Section
			fmt.Fprintf(&b, "  %s\n", sec)
		}
		fmt.Fprintf(&b, "    %s %-42s %s\n", icon(s.Status), s.ID, s.Title)
	}
	b.WriteString("\nSuites\n")
	for _, g := range r.Go {
		fmt.Fprintf(&b, "  %s %-44s %3d/%-3d passed\n", groupIcon(g), short(g.Name), g.Passed, g.All)
	}
	for _, g := range r.E2E {
		if g.Note != "" {
			fmt.Fprintf(&b, "  ⏭️ %-44s %s\n", g.Name, g.Note)
			continue
		}
		fmt.Fprintf(&b, "  %s %-44s %3d/%-3d examples\n", groupIcon(g), g.Name, g.Passed, g.All)
	}
	fmt.Fprintf(&b, "\n%s %d/%d tests passed\n", map[bool]string{true: "❌ FAILED:", false: "✅ PASSED:"}[r.Failed > 0], r.Total-r.Failed, r.Total)
	return b.String()
}

func groupIcon(g Group) string {
	if g.Failed > 0 {
		return "❌"
	}
	return "✅"
}

func markdown(r *report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## CloudTyke tests: %s %d/%d passed\n\n", map[bool]string{true: "❌", false: "✅"}[r.Failed > 0], r.Total-r.Failed, r.Total)
	if len(r.Services) > 0 {
		b.WriteString("### Services\n\n| | Section | Service | What is checked |\n|---|---|---|---|\n")
		for _, s := range r.Services {
			fmt.Fprintf(&b, "| %s | %s | `%s` | %s |\n", icon(s.Status), s.Section, s.ID, s.Title)
		}
	}
	b.WriteString("\n### Suites\n\n| | Suite | Passed |\n|---|---|---|\n")
	for _, g := range r.Go {
		fmt.Fprintf(&b, "| %s | `%s` | %d/%d |\n", groupIcon(g), short(g.Name), g.Passed, g.All)
	}
	for _, g := range r.E2E {
		if g.Note != "" {
			fmt.Fprintf(&b, "| ⏭️ | %s | %s |\n", g.Name, g.Note)
		} else {
			fmt.Fprintf(&b, "| %s | %s | %d/%d |\n", groupIcon(g), g.Name, g.Passed, g.All)
		}
	}
	return b.String()
}

type junitSuites struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []junitSuite `xml:"testsuite"`
}
type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Cases    []junitCase `xml:"testcase"`
}
type junitCase struct {
	Class   string     `xml:"classname,attr"`
	Name    string     `xml:"name,attr"`
	Time    string     `xml:"time,attr"`
	Failure *junitText `xml:"failure,omitempty"`
	Skipped *junitText `xml:"skipped,omitempty"`
	Output  string     `xml:"system-out,omitempty"`
}
type junitText struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func writeJUnit(path string, r *report) error {
	var js junitSuites
	for _, g := range append(append([]Group{}, r.Go...), r.E2E...) {
		s := junitSuite{Name: short(g.Name), Tests: g.All, Failures: g.Failed}
		for _, c := range g.Cases {
			jc := junitCase{Class: short(c.Suite), Name: c.Name, Time: fmt.Sprintf("%.3f", c.Seconds)}
			switch c.Status {
			case "fail":
				jc.Failure = &junitText{Message: "failed", Body: c.Output}
			case "skip":
				s.Skipped++
				jc.Skipped = &junitText{Message: "skipped"}
			}
			s.Cases = append(s.Cases, jc)
		}
		js.Suites = append(js.Suites, s)
	}
	b, err := xml.MarshalIndent(js, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(xml.Header), b...), 0o644)
}

func writeHTML(path string, r *report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return page.Execute(f, r)
}

var page = template.Must(template.New("report").Funcs(template.FuncMap{
	"short": short,
	"pct": func(p, all int) int {
		if all == 0 {
			return 0
		}
		return p * 100 / all
	},
	"sec": func(s string) string {
		return map[string]string{"ARM": "Azure Resource Manager", "Token": "Entra ID token service", "Graph": "Microsoft Graph"}[s]
	},
}).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>CloudTyke test report</title>
<style>
:root{--bg:#f6f7f9;--card:#fff;--fg:#1d2330;--muted:#5d6678;--line:#e3e6ec;--ok:#1a7f37;--okbg:#e6f4ea;--bad:#cf222e;--badbg:#fdecee;--skip:#9a6700;--skipbg:#fff8c5}
@media (prefers-color-scheme:dark){:root{--bg:#0f1218;--card:#171b23;--fg:#e6e9ef;--muted:#9aa3b2;--line:#2a303b;--ok:#4ac26b;--okbg:#12261a;--bad:#ff7b72;--badbg:#2d1215;--skip:#d29922;--skipbg:#2b2111}}
*{box-sizing:border-box}body{margin:0;font:14px/1.5 system-ui,-apple-system,Segoe UI,sans-serif;background:var(--bg);color:var(--fg)}
main{max-width:1100px;margin:0 auto;padding:28px 20px 60px}h1{font-size:22px;margin:0 0 4px}h2{font-size:17px;margin:28px 0 10px}
.sub{color:var(--muted);margin:0 0 20px}.bar{display:flex;gap:12px;flex-wrap:wrap}
.tile{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:12px 16px;min-width:150px}
.tile b{display:block;font-size:24px;font-variant-numeric:tabular-nums}.tile span{color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.04em}
.ok{color:var(--ok)}.bad{color:var(--bad)}
.card{background:var(--card);border:1px solid var(--line);border-radius:10px;overflow:hidden;margin-bottom:14px}
.card h3{font-size:14px;margin:0;padding:10px 16px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between}
details{border-top:1px solid var(--line)}details:first-of-type{border-top:0}
summary{display:grid;grid-template-columns:64px minmax(290px,1.1fr) 2fr 70px;gap:12px;align-items:center;padding:9px 16px;cursor:pointer;list-style:none}
summary::-webkit-details-marker{display:none}
.pill{font-size:12px;font-weight:600;border-radius:999px;padding:2px 8px;text-align:center}
.pass{background:var(--okbg);color:var(--ok)}.fail{background:var(--badbg);color:var(--bad)}.skip{background:var(--skipbg);color:var(--skip)}
code{font:12.5px ui-monospace,SFMono-Regular,Menlo,monospace}.muted{color:var(--muted)}.num{text-align:right;color:var(--muted);font-variant-numeric:tabular-nums}
pre{margin:0;padding:10px 16px 14px;max-height:420px;overflow:auto;font:12px/1.45 ui-monospace,Menlo,monospace;white-space:pre-wrap;color:var(--muted)}
.meter{height:6px;background:var(--badbg);border-radius:3px;overflow:hidden;margin-top:8px}.meter i{display:block;height:100%;background:var(--ok)}
table{border-collapse:collapse;width:100%}td{padding:8px 16px;border-top:1px solid var(--line)}tr:first-child td{border-top:0}
</style></head><body><main>
<h1>CloudTyke test report</h1>
<p class="sub">Generated {{.Generated.Format "2006-01-02 15:04:05 MST"}}</p>
<div class="bar">
 <div class="tile"><b class="{{if .Failed}}bad{{else}}ok{{end}}">{{if .Failed}}FAILED{{else}}PASSED{{end}}</b><span>overall</span></div>
 <div class="tile"><b>{{len .Services}}</b><span>services checked</span></div>
 {{range .Sections}}<div class="tile"><b class="{{if .Failed}}bad{{end}}">{{.Passed}}/{{.All}}</b><span>{{.Name}} services</span><div class="meter"><i style="width:{{pct .Passed .All}}%"></i></div></div>{{end}}
 <div class="tile"><b class="{{if .Failed}}bad{{end}}">{{.Failed}}</b><span>failed of {{.Total}} tests</span></div>
</div>

{{if .Services}}<h2>Services</h2>{{end}}
{{$svcs := .Services}}{{range .Sections}}{{$s := .Name}}<div class="card"><h3><span>{{sec $s}}</span><span class="muted">{{.Passed}}/{{.All}}</span></h3>
{{range $svcs}}{{if eq .Section $s}}<details{{if ne .Status "pass"}} open{{end}}><summary><span class="pill {{.Status}}">{{.Status}}</span><code>{{.ID}}</code><span class="muted">{{.Title}}</span><span class="num">{{printf "%.2f" .Seconds}}s</span></summary><pre>{{.Output}}</pre></details>{{end}}{{end}}
</div>{{end}}

<h2>Terraform end-to-end</h2>
<div class="card"><table>{{range .E2E}}<tr><td>{{if .Note}}<span class="pill skip">{{.Note}}</span>{{else if .Failed}}<span class="pill fail">fail</span>{{else}}<span class="pill pass">pass</span>{{end}}</td><td>{{.Name}}</td><td class="num">{{if not .Note}}{{.Passed}}/{{.All}} examples{{end}}</td></tr>{{else}}<tr><td class="muted">not run (make test-all without SKIP_E2E runs both OpenTofu and Terraform)</td></tr>{{end}}</table>
{{range .E2E}}{{range .Cases}}<details{{if eq .Status "fail"}} open{{end}}><summary><span class="pill {{.Status}}">{{.Status}}</span><code>{{.Suite}} / {{.Name}}</code><span></span><span></span></summary><pre>{{.Output}}</pre></details>{{end}}{{end}}</div>

{{if .Go}}<h2>Unit and integration suites</h2>{{end}}
{{range .Go}}<div class="card"><h3><span><code>{{short .Name}}</code></span><span class="{{if .Failed}}bad{{else}}muted{{end}}">{{.Passed}}/{{.All}}</span></h3>
{{range .Cases}}<details{{if eq .Status "fail"}} open{{end}}><summary><span class="pill {{.Status}}">{{.Status}}</span><code>{{.Name}}</code><span></span><span class="num">{{printf "%.2f" .Seconds}}s</span></summary><pre>{{.Output}}</pre></details>{{end}}
</div>{{end}}
</main></body></html>`))
