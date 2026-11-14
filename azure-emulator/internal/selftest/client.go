// Package selftest is the per-service conformance suite. Each row of the
// coverage matrix (admin.Services) has one check that drives the emulator
// over its public HTTP API. The same checks run in two places:
//
//   - go test ./test/conformance (one subtest per service, JUnit/HTML report)
//   - GET /_cloudtyke/selftest (live page for demos, against a throwaway
//     in-memory instance so the caller's state is never touched)
package selftest

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// Seed constants from internal/seed/default.yaml. The checks always run
// against the default seed.
const (
	Tenant      = "7c10d7e4-0000-4000-8000-000000000001"
	SubSandbox  = "5b5c0000-0000-4000-8000-000000000003"
	SubConn     = "5b5c0000-0000-4000-8000-000000000001"
	BootAppID   = "a99c0000-0000-4000-8000-000000000001"
	BootSecret  = "cloudtyke-bootstrap-secret"
	BootSPID    = "0b100000-0000-4000-8000-000000000001"
	ReaderAppID = "a99c0000-0000-4000-8000-000000000002"
	ReaderSec   = "cloudtyke-reader-secret"
	ReaderSPID  = "0b100000-0000-4000-8000-000000000002"
	AdaUserID   = "0c100000-0000-4000-8000-000000000001"
	GraphSPID   = "0e100000-0000-4000-8000-00000000c0de"
	GraphAppID  = "00000003-0000-0000-c000-000000000000"

	RoleOwner       = "8e3af657-a8ff-443c-a75c-2fe8c4bcb635"
	RoleContributor = "b24988ac-6180-42a0-ab88-20f7382dd24c"
	RoleReader      = "acdd72a7-3385-48ef-bd42-f606fba81ae7"
)

// Call is one HTTP exchange made by a check, kept for the report and tour.
type Call struct {
	Method string `json:"method"`
	URL    string `json:"url"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"` // error.code from the body, if any
	// Step is the plain-English purpose of the call ("Reader cannot write")
	// and Expect what the check required ("403 AuthorizationFailed").
	Step   string `json:"step,omitempty"`
	Expect string `json:"expect,omitempty"`
	// Verified lists further assertions made on this response.
	Verified []string `json:"verified,omitempty"`
	Request  string   `json:"request,omitempty"`
	Response string   `json:"response,omitempty"`
}

// Resp is a decoded response.
type Resp struct {
	Code   int
	Header http.Header
	Body   map[string]any
	Raw    []byte
	idx    int // index into C.calls
}

// Err returns error.code (ARM / Graph) or error (OAuth).
func (r Resp) Err() string {
	switch e := r.Body["error"].(type) {
	case map[string]any:
		s, _ := e["code"].(string)
		return s
	case string:
		return e
	}
	return ""
}

// Str walks a dotted path ("properties.principalType") and returns a string.
func (r Resp) Str(path string) string { s, _ := dig(r.Body, path).(string); return s }

// Value returns the "value" array of a list response.
func (r Resp) Value() []map[string]any {
	arr, _ := r.Body["value"].([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func dig(v any, path string) any {
	for _, p := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[p]
	}
	return v
}

// failure is raised (via panic) by C.Fatalf and recovered by the runner.
type failure string

// C is the context handed to each check.
type C struct {
	h      http.Handler
	domain string
	calls  []Call
	toks   map[string]string
	seq    *atomic.Int64
	// Poll is how long async operations are polled before giving up.
	Poll time.Duration
}

// Fatalf fails the check.
func (c *C) Fatalf(format string, args ...any) { panic(failure(fmt.Sprintf(format, args...))) }

// Expect fails the check unless ok. Passing assertions are recorded on the
// most recent call, labelled with the message up to its "got ..." part.
func (c *C) Expect(ok bool, format string, args ...any) {
	if !ok {
		c.Fatalf(format, args...)
	}
	if n := len(c.calls); n > 0 {
		if l := assertLabel(format); l != "" {
			c.calls[n-1].Verified = append(c.calls[n-1].Verified, l)
		}
	}
}

func assertLabel(format string) string {
	cut := len(format)
	for _, sep := range []string{", got", ", saw", ": got", ":", " %"} {
		if i := strings.Index(format, sep); i >= 0 && i < cut {
			cut = i
		}
	}
	l := strings.TrimSpace(format[:cut])
	if strings.HasPrefix(l, "unexpected") || l == "" {
		return ""
	}
	return l
}

// Status fails unless the response has the given status (and, if code is
// non-empty, the given error code).
func (c *C) Status(r Resp, want int, code, what string) Resp {
	if r.idx < len(c.calls) {
		c.calls[r.idx].Step = what
		c.calls[r.idx].Expect = strings.TrimSpace(fmt.Sprintf("%d %s", want, code))
	}
	if r.Code != want || (code != "" && r.Err() != code) {
		c.Fatalf("%s: want %d %s, got %d %s %s", what, want, code, r.Code, r.Err(), short(r.Raw))
	}
	return r
}

func short(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// StatusIn is Status for checks that accept any of several status codes.
func (c *C) StatusIn(r Resp, what string, want ...int) Resp {
	var exp []string
	for _, w := range want {
		exp = append(exp, fmt.Sprint(w))
	}
	if r.idx < len(c.calls) {
		c.calls[r.idx].Step = what
		c.calls[r.idx].Expect = strings.Join(exp, " or ")
	}
	for _, w := range want {
		if r.Code == w {
			return r
		}
	}
	c.Fatalf("%s: want %s, got %d %s %s", what, strings.Join(exp, " or "), r.Code, r.Err(), short(r.Raw))
	return r
}

// Name returns a unique name with the given prefix.
func (c *C) Name(prefix string) string {
	return fmt.Sprintf("%s-st%d-%d", prefix, time.Now().UnixNano()%100000, c.seq.Add(1))
}

// URL builds https://<sub>.<domain><path>.
func (c *C) URL(sub, path string) string { return "https://" + sub + "." + c.domain + path }

// ARM is the management endpoint URL for a path (api-version appended).
func (c *C) ARM(path, apiVersion string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return c.URL("management", path+sep+"api-version="+apiVersion)
}

// Graph is a Microsoft Graph v1.0 URL.
func (c *C) Graph(path string) string { return c.URL("graph", "/v1.0"+path) }

// Do performs a request against the emulator handler in-process.
func (c *C) Do(method, u, tok string, body any) Resp {
	var rdr io.Reader
	var reqText string
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
		reqText = pretty(b)
	}
	req := httptest.NewRequest(method, u, rdr)
	req.Header.Set("Content-Type", "application/json")
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return c.serve(req, reqText)
}

// Form posts an x-www-form-urlencoded body.
func (c *C) Form(u string, form url.Values) Resp {
	req := httptest.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var shown []string
	for _, k := range []string{"grant_type", "client_id", "client_secret", "client_assertion_type", "client_assertion", "scope", "resource"} {
		if v := form.Get(k); v != "" {
			if k == "client_secret" || k == "client_assertion" {
				v = "***"
			}
			shown = append(shown, k+"="+v)
		}
	}
	return c.serve(req, strings.Join(shown, "\n"))
}

func (c *C) serve(req *http.Request, reqText string) Resp {
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	res := rec.Result()
	raw, _ := io.ReadAll(res.Body)
	r := Resp{Code: res.StatusCode, Header: res.Header, Raw: raw, idx: len(c.calls)}
	_ = json.Unmarshal(raw, &r.Body)
	path := req.URL.Path
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}
	resp := pretty(raw)
	if loc := res.Header.Get("Location"); loc != "" {
		resp = strings.TrimSpace("Location: " + loc + "\n" + resp)
	}
	c.calls = append(c.calls, Call{Method: req.Method, URL: req.Host + path, Status: r.Code, Error: r.Err(), Request: reqText, Response: resp})
	return r
}

// pretty indents JSON, shortens long strings (tokens, keys) and caps the size
// so reports stay readable.
func pretty(raw []byte) string {
	var v any
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	if json.Unmarshal(raw, &v) != nil {
		return capLen(string(raw), 2000)
	}
	v = shorten(v)
	b, _ := json.MarshalIndent(v, "", "  ")
	return capLen(string(b), 4000)
}

func shorten(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			x[k] = shorten(e)
		}
	case []any:
		for i, e := range x {
			x[i] = shorten(e)
		}
		if len(x) > 8 {
			return append(x[:8], fmt.Sprintf("… %d more", len(x)-8))
		}
	case string:
		if len(x) > 120 {
			return x[:60] + "…" + x[len(x)-20:]
		}
	}
	return v
}

func capLen(s string, n int) string {
	if len(s) > n {
		return s[:n] + "\n… (truncated)"
	}
	return s
}

// Token gets a client_credentials token for resource ("management" or "graph").
func (c *C) Token(clientID, secret, sub string) string {
	key := clientID + "|" + secret + "|" + sub
	if t, ok := c.toks[key]; ok {
		return t
	}
	resource := c.URL(sub, "")
	if sub == "management" {
		resource += "/"
	}
	r := c.Form(c.URL("login", "/"+Tenant+"/oauth2/v2.0/token"), url.Values{
		"client_id": {clientID}, "client_secret": {secret}, "grant_type": {"client_credentials"}, "scope": {resource + "/.default"},
	})
	who := map[string]string{BootAppID: "cloudtyke-bootstrap (Owner, Global Admin)", ReaderAppID: "cloudtyke-reader (Reader)"}[clientID]
	if who == "" {
		who = "app " + clientID
	}
	api := map[string]string{"management": "ARM", "graph": "Microsoft Graph"}[sub]
	c.Status(r, 200, "", "get a token for "+api+" as "+who)
	t, _ := r.Body["access_token"].(string)
	c.toks[key] = t
	return t
}

// Boot / Reader tokens for ARM ("management") or Graph ("graph").
func (c *C) Boot(sub string) string   { return c.Token(BootAppID, BootSecret, sub) }
func (c *C) Reader(sub string) string { return c.Token(ReaderAppID, ReaderSec, sub) }

// Claims decodes a JWT payload without verifying it.
func Claims(tok string) map[string]any {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

// WaitAsync polls an ARM Location / Azure-AsyncOperation URL until it stops
// returning 202, and returns the final response.
func (c *C) WaitAsync(loc, tok string) Resp {
	deadline := time.Now().Add(c.Poll)
	for {
		r := c.Do(http.MethodGet, loc, tok, nil)
		if r.Code != 202 || time.Now().After(deadline) {
			return r
		}
		c.calls[r.idx].Step = "poll async operation (still running)"
		time.Sleep(25 * time.Millisecond)
	}
}

// HasID reports whether a list contains an item whose field equals want
// (case-insensitive).
func HasID(items []map[string]any, field, want string) bool {
	for _, it := range items {
		if s, _ := it[field].(string); strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
