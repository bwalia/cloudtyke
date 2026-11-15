package selftest

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/admin"
)

// Check exercises one coverage-matrix row.
type Check struct {
	Title string // what the check proves, shown in reports
	Run   func(c *C)
}

// Status of a service result.
const (
	Pass    = "pass"
	Fail    = "fail"
	Missing = "missing" // coverage row without a check
	Stale   = "stale"   // check without a coverage row
)

// Result is the outcome for one service.
type Result struct {
	Section    string `json:"section"`
	Service    string `json:"service"`
	Title      string `json:"title"`
	Notes      string `json:"notes,omitempty"` // coverage-matrix notes
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Calls      []Call `json:"calls"`
}

// Report is a full run.
type Report struct {
	Started    time.Time `json:"started"`
	DurationMs int64     `json:"durationMs"`
	Passed     int       `json:"passed"`
	Failed     int       `json:"failed"`
	Results    []Result  `json:"results"`
}

// Runner runs checks against one emulator handler.
type Runner struct {
	Handler http.Handler
	Domain  string
	Poll    time.Duration
	seq     atomic.Int64
	toks    map[string]string
}

// NewRunner creates a runner for a handler serving the given domain.
func NewRunner(h http.Handler, domain string) *Runner {
	return &Runner{Handler: h, Domain: domain, Poll: 10 * time.Second, toks: map[string]string{}}
}

// RunOne runs the check for a service.
func (r *Runner) RunOne(svc admin.Service) Result {
	res := Result{Section: svc.Section, Service: svc.ID, Notes: svc.Notes}
	chk, ok := Checks[svc.ID]
	if !ok {
		res.Status = Missing
		res.Error = "no self-test check for this coverage-matrix row: add one to internal/selftest/checks.go"
		return res
	}
	res.Title = chk.Title
	c := &C{h: r.Handler, domain: r.Domain, toks: r.toks, seq: &r.seq, Poll: r.Poll}
	start := time.Now()
	func() {
		defer func() {
			if v := recover(); v != nil {
				res.Status = Fail
				if f, ok := v.(failure); ok {
					res.Error = string(f)
				} else {
					res.Error = fmt.Sprintf("panic: %v\n%s", v, debug.Stack())
				}
			}
		}()
		chk.Run(c)
		res.Status = Pass
	}()
	res.DurationMs = time.Since(start).Milliseconds()
	res.Calls = c.calls
	return res
}

// Run runs every service (optionally only IDs matching filter, a
// case-insensitive substring) and reports stale checks.
func (r *Runner) Run(services []admin.Service, filter string) Report {
	rep := Report{Started: time.Now().UTC()}
	known := map[string]bool{}
	for _, s := range services {
		known[s.ID] = true
		if filter != "" && !strings.Contains(strings.ToLower(s.Section+"/"+s.ID), strings.ToLower(filter)) {
			continue
		}
		rep.add(r.RunOne(s))
	}
	if filter == "" {
		for _, id := range StaleChecks(services) {
			rep.add(Result{Section: "?", Service: id, Status: Stale, Error: "check has no matching coverage-matrix row (renamed or removed type?)"})
		}
	}
	rep.DurationMs = time.Since(rep.Started).Milliseconds()
	return rep
}

func (rep *Report) add(res Result) {
	if res.Status == Pass {
		rep.Passed++
	} else {
		rep.Failed++
	}
	rep.Results = append(rep.Results, res)
}

// StaleChecks lists check IDs with no coverage-matrix row.
func StaleChecks(services []admin.Service) []string {
	known := map[string]bool{}
	for _, s := range services {
		known[s.ID] = true
	}
	var out []string
	for id := range Checks {
		if !known[id] {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}
