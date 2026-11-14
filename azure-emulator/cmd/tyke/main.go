// Command tyke is the CloudTyke CLI.
//
//	tyke azure up|down|status|logs|reset|seed
//	tyke azure env [--shell]         print ARM_* / AZURE_* exports
//	tyke azure trust [--install]     export the local CA (and optionally trust it)
//	tyke azure hosts                 print /etc/hosts lines
//	tyke azure requests [-n 20]      recent API calls seen by the emulator
//	tyke azure snapshot save|restore|list <name>
//	tyke azure chaos get|set|clear [--throttle 0.2 --errors 0.1 --latency 200 --async-delay 5]
//	tyke azure labs ...              (phase 6)
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 3 || os.Args[1] != "azure" {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[2], os.Args[3:]
	var err error
	switch cmd {
	case "up":
		err = compose(append([]string{"up", "-d", "--build", "--wait"}, args...)...)
		if err == nil {
			err = exportCA(false)
			fmt.Println("\nCloudTyke Azure emulator is up. Next:\n  eval \"$(tyke azure env)\"\n  tyke azure trust --install   # or use: docker compose run --rm toolbox")
		}
	case "down":
		err = compose(append([]string{"down"}, args...)...)
	case "logs":
		err = compose(append([]string{"logs", "-f", "emulator"}, args...)...)
	case "status":
		err = status()
	case "reset":
		err = adminPost("/reset", nil)
	case "seed":
		err = adminPost("/seed", nil)
	case "env":
		err = env(args)
	case "trust":
		fs := flag.NewFlagSet("trust", flag.ExitOnError)
		install := fs.Bool("install", false, "add the CA to the OS trust store (asks for your password)")
		_ = fs.Parse(args)
		err = exportCA(*install)
	case "hosts":
		hosts()
	case "requests":
		fs := flag.NewFlagSet("requests", flag.ExitOnError)
		n := fs.Int("n", 20, "number of requests")
		_ = fs.Parse(args)
		err = requests(*n)
	case "snapshot":
		err = snapshot(args)
	case "chaos":
		err = chaos(args)
	case "labs":
		err = fmt.Errorf("labs arrive in phase 6 (see docs/roadmap.md)")
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "tyke:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: tyke azure <command>

  up | down | status | logs        manage the emulator containers
  reset | seed                     wipe state and re-seed / re-apply seed.yaml
  env                              print ARM_* and AZURE_* exports (eval "$(tyke azure env)")
  trust [--install]                write ./certs/ca.pem and optionally trust it
  hosts                            print /etc/hosts entries
  requests [-n N]                  show recent API calls (what Terraform actually called)
  snapshot save|restore|list NAME  save / restore emulator state
  chaos get|set|clear              fault injection (--throttle --errors --latency --async-delay)

environment: CLOUDTYKE_URL (default https://localhost), CLOUDTYKE_DIR (azure-emulator dir), CLOUDTYKE_ADMIN_TOKEN`)
}

func baseURL() string {
	if u := os.Getenv("CLOUDTYKE_URL"); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return "https://localhost"
}

func projectDir() string {
	if d := os.Getenv("CLOUDTYKE_DIR"); d != "" {
		return d
	}
	wd, _ := os.Getwd()
	for d := wd; d != "/" && d != "."; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "docker-compose.yml")); err == nil {
			if _, err := os.Stat(filepath.Join(d, "cmd", "emulator")); err == nil {
				return d
			}
		}
		if _, err := os.Stat(filepath.Join(d, "azure-emulator", "docker-compose.yml")); err == nil {
			return filepath.Join(d, "azure-emulator")
		}
	}
	return wd
}

func compose(args ...string) error {
	c := exec.Command("docker", append([]string{"compose"}, args...)...)
	c.Dir = projectDir()
	c.Stdout, c.Stderr, c.Stdin = os.Stdout, os.Stderr, os.Stdin
	return c.Run()
}

func client() *http.Client {
	pool, _ := x509.SystemCertPool()
	if pool == nil {
		pool = x509.NewCertPool()
	}
	if b, err := os.ReadFile(filepath.Join(projectDir(), "certs", "ca.pem")); err == nil {
		pool.AppendCertsFromPEM(b)
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
}

// insecureClient is only used to bootstrap: download the CA itself.
func insecureClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec
}

func adminReq(method, path string, body any) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, baseURL()+"/_cloudtyke"+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if t := os.Getenv("CLOUDTYKE_ADMIN_TOKEN"); t != "" {
		req.Header.Set("X-CloudTyke-Admin-Token", t)
	}
	resp, err := client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w (is the emulator running and certs/ca.pem exported? try: tyke azure trust)", err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func adminPost(path string, body any) error {
	out, err := adminReq(http.MethodPost, path, body)
	if err == nil {
		fmt.Println(strings.TrimSpace(string(out)))
	}
	return err
}

func status() error {
	out, err := adminReq(http.MethodGet, "/health", nil)
	if err != nil {
		return err
	}
	fmt.Println("emulator:", strings.TrimSpace(string(out)))
	return compose("ps")
}

func env(args []string) error {
	out, err := adminReq(http.MethodGet, "/env", nil)
	if err != nil {
		return err
	}
	var v struct {
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return err
	}
	keys := make([]string, 0, len(v.Env))
	for k := range v.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ca, _ := filepath.Abs(filepath.Join(projectDir(), "certs", "ca.pem"))
	for _, k := range keys {
		fmt.Printf("export %s=%q\n", k, v.Env[k])
	}
	fmt.Printf("export SSL_CERT_FILE=%q        # Linux Go tools, curl, openssl\n", ca)
	fmt.Printf("export REQUESTS_CA_BUNDLE=%q   # Python / az CLI\n", ca)
	fmt.Printf("export NODE_EXTRA_CA_CERTS=%q  # Node.js SDKs\n", ca)
	return nil
}

func exportCA(install bool) error {
	resp, err := insecureClient().Get(baseURL() + "/_cloudtyke/ca.pem")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	pemBytes, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(pemBytes, []byte("BEGIN CERTIFICATE")) {
		return fmt.Errorf("unexpected CA response")
	}
	dir := filepath.Join(projectDir(), "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(path, pemBytes, 0o644); err != nil {
		return err
	}
	fmt.Println("CA written to", path)
	if !install {
		fmt.Println("Trust it with: tyke azure trust --install   (Go tools on macOS ignore SSL_CERT_FILE)")
		return nil
	}
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("security", "add-trusted-cert", "-r", "trustRoot", "-k", filepath.Join(os.Getenv("HOME"), "Library/Keychains/login.keychain-db"), path)
	case "linux":
		c = exec.Command("sh", "-c", "sudo cp '"+path+"' /usr/local/share/ca-certificates/cloudtyke-ca.crt && sudo update-ca-certificates")
	case "windows":
		c = exec.Command("certutil", "-user", "-addstore", "Root", path)
	default:
		return fmt.Errorf("install the CA manually on %s", runtime.GOOS)
	}
	c.Stdout, c.Stderr, c.Stdin = os.Stdout, os.Stderr, os.Stdin
	return c.Run()
}

func hosts() {
	d := os.Getenv("CLOUDTYKE_DOMAIN")
	if d == "" {
		d = "cloudtyke.local"
	}
	fmt.Printf("# CloudTyke Azure emulator (add to /etc/hosts or C:\\Windows\\System32\\drivers\\etc\\hosts)\n")
	fmt.Printf("127.0.0.1 %s management.%s login.%s graph.%s\n", d, d, d, d)
}

func requests(n int) error {
	out, err := adminReq(http.MethodGet, fmt.Sprintf("/requests?limit=%d", n), nil)
	if err != nil {
		return err
	}
	var v struct {
		Value []struct {
			Time       time.Time `json:"time"`
			Method     string    `json:"method"`
			Service    string    `json:"service"`
			Path       string    `json:"path"`
			APIVersion string    `json:"apiVersion"`
			Status     int       `json:"status"`
			DurationMs int64     `json:"durationMs"`
			Caller     string    `json:"caller"`
		} `json:"value"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return err
	}
	for i := len(v.Value) - 1; i >= 0; i-- {
		e := v.Value[i]
		fmt.Printf("%s %-6s %-5s %3d %4dms %s %s\n", e.Time.Local().Format("15:04:05"), e.Service, e.Method, e.Status, e.DurationMs, e.Path, e.APIVersion)
	}
	return nil
}

func snapshot(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: tyke azure snapshot save|restore|list|delete NAME")
	}
	switch args[0] {
	case "list":
		out, err := adminReq(http.MethodGet, "/snapshots", nil)
		fmt.Println(strings.TrimSpace(string(out)))
		return err
	case "save", "restore", "delete":
		if len(args) < 2 {
			return fmt.Errorf("snapshot name required")
		}
		switch args[0] {
		case "save":
			return adminPost("/snapshots/"+args[1], nil)
		case "restore":
			return adminPost("/snapshots/"+args[1]+"/restore", nil)
		default:
			_, err := adminReq(http.MethodDelete, "/snapshots/"+args[1], nil)
			return err
		}
	}
	return fmt.Errorf("unknown snapshot command %q", args[0])
}

func chaos(args []string) error {
	if len(args) == 0 || args[0] == "get" {
		out, err := adminReq(http.MethodGet, "/chaos", nil)
		fmt.Println(strings.TrimSpace(string(out)))
		return err
	}
	switch args[0] {
	case "clear":
		_, err := adminReq(http.MethodDelete, "/chaos", nil)
		return err
	case "set":
		fs := flag.NewFlagSet("chaos set", flag.ExitOnError)
		thr := fs.Float64("throttle", 0, "probability of 429 responses (0..1)")
		errs := fs.Float64("errors", 0, "probability of 5xx responses (0..1)")
		lat := fs.Int("latency", 0, "added latency in ms")
		delay := fs.Float64("async-delay", 0, "seconds async operations stay InProgress")
		writes := fs.Bool("writes-only", false, "only affect write requests")
		_ = fs.Parse(args[1:])
		return adminPost("/chaos", map[string]any{"throttleRate": *thr, "errorRate": *errs, "latencyMs": *lat, "asyncDelaySeconds": *delay, "writesOnly": *writes})
	case "drift":
		return fmt.Errorf("drift injection arrives with the labs in phase 6")
	}
	return fmt.Errorf("unknown chaos command %q", args[0])
}
