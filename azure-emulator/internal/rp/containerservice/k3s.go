package containerservice

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
)

// K3sBackend runs one k3s container per AKS cluster through the Docker
// Engine API (unix socket). The emulator proxies kubectl traffic for the
// cluster fqdn to the container with the k3s admin certificate, after
// checking the AKS credential token, so kubeconfigs from
// listClusterAdminCredential drive a real Kubernetes API.
//
// k3s is started with the cluster's OIDC issuer URL as service account
// issuer, so projected service account tokens validate against
// oidc.<domain> (workload identity).
type K3sBackend struct {
	cfg     config.Config
	log     *slog.Logger
	docker  *http.Client
	image   string
	network string

	mu       sync.Mutex
	clusters map[string]*k3sCluster // by cluster id (lower-case)
}

type k3sCluster struct {
	name      string // container name
	ready     bool
	starting  bool
	err       string
	transport *http.Transport
}

// NewK3sBackend connects to Docker (DOCKER_HOST or /var/run/docker.sock).
func NewK3sBackend(cfg config.Config, logger *slog.Logger) (Backend, error) {
	sock := "/var/run/docker.sock"
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		sock = strings.TrimPrefix(h, "unix://")
	}
	if _, err := os.Stat(sock); err != nil {
		return nil, fmt.Errorf("docker socket %s not available (mount it into the emulator container): %w", sock, err)
	}
	b := &K3sBackend{
		cfg: cfg, log: logger, clusters: map[string]*k3sCluster{},
		image:   envOr("CLOUDTYKE_K3S_IMAGE", "rancher/k3s:v1.33.5-k3s1"),
		network: envOr("CLOUDTYKE_DOCKER_NETWORK", "cloudtyke-azure_cloudtyke"),
		docker: &http.Client{Timeout: 10 * time.Minute, Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		}},
	}
	if _, err := b.call("GET", "/_ping", nil, nil); err != nil {
		return nil, fmt.Errorf("docker API not reachable: %w", err)
	}
	return b, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Name implements Backend.
func (b *K3sBackend) Name() string { return "k3s" }

func containerName(clusterID string) string { return "cloudtyke-aks-" + shortHash(clusterID) }

func (b *K3sBackend) state(id string) *k3sCluster {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := strings.ToLower(id)
	c := b.clusters[k]
	if c == nil {
		c = &k3sCluster{name: containerName(id)}
		b.clusters[k] = c
	}
	return c
}

// Ensure implements Backend: create/start/stop the container to match the cluster.
func (b *K3sBackend) Ensure(cluster map[string]any, _ []map[string]any) {
	id := strOf(cluster["id"])
	p := props(cluster)
	st := b.state(id)
	stopped := false
	if ps, _ := p["powerState"].(map[string]any); strOf(ps["code"]) == "Stopped" {
		stopped = true
	}
	issuer := ""
	if o, _ := p["oidcIssuerProfile"].(map[string]any); o != nil && o["enabled"] == true {
		issuer = strOf(o["issuerURL"])
	}
	b.mu.Lock()
	if st.starting {
		b.mu.Unlock()
		return
	}
	st.starting = true
	b.mu.Unlock()
	go func() {
		defer func() { b.mu.Lock(); st.starting = false; b.mu.Unlock() }()
		if stopped {
			_, _ = b.call("POST", "/containers/"+st.name+"/stop?t=5", nil, nil)
			b.mu.Lock()
			st.ready = false
			b.mu.Unlock()
			return
		}
		if err := b.run(st, cluster, issuer); err != nil {
			b.log.Error("k3s cluster failed to start", "cluster", id, "err", err)
			b.mu.Lock()
			st.err = err.Error()
			b.mu.Unlock()
		}
	}()
}

func (b *K3sBackend) run(st *k3sCluster, cluster map[string]any, issuer string) error {
	info, err := b.call("GET", "/containers/"+st.name+"/json", nil, nil)
	if err != nil && !isNotFound(err) {
		return err
	}
	if isNotFound(err) {
		args := []string{"server", "--disable=traefik", "--disable=servicelb", "--tls-san=" + st.name,
			"--node-name=aks-system-" + shortHash(strOf(cluster["id"])) + "-vmss000000",
			"--node-label=agentpool=system", "--node-label=kubernetes.azure.com/agentpool=system",
			"--node-label=kubernetes.azure.com/cluster=" + strOf(props(cluster)["nodeResourceGroup"]),
			"--write-kubeconfig-mode=644"}
		if issuer != "" {
			args = append(args, "--kube-apiserver-arg=service-account-issuer="+issuer, "--kube-apiserver-arg=service-account-jwks-uri="+issuer+"openid/v1/jwks")
		}
		body := map[string]any{
			"Image": b.image, "Cmd": args, "Hostname": st.name,
			"Labels":     map[string]string{"cloudtyke.aks/cluster": strOf(cluster["id"]), "cloudtyke.managed": "true"},
			"HostConfig": map[string]any{"Privileged": true, "Tmpfs": map[string]string{"/run": "", "/var/run": ""}, "NetworkMode": b.network},
		}
		if _, err := b.call("POST", "/containers/create?name="+st.name, body, nil); err != nil {
			if !isNotFound(err) {
				return err
			}
			b.log.Info("pulling k3s image", "image", b.image)
			ref, tag, _ := strings.Cut(b.image, ":")
			if _, err := b.call("POST", "/images/create?fromImage="+url.QueryEscape(ref)+"&tag="+url.QueryEscape(tag), nil, nil); err != nil {
				return fmt.Errorf("pulling %s: %w", b.image, err)
			}
			if _, err := b.call("POST", "/containers/create?name="+st.name, body, nil); err != nil {
				return err
			}
		}
	} else if s, _ := info["State"].(map[string]any); s != nil && s["Running"] == true && st.transport != nil {
		return nil
	}
	if _, err := b.call("POST", "/containers/"+st.name+"/start", nil, nil); err != nil && !strings.Contains(err.Error(), "304") {
		return err
	}
	// wait for the kubeconfig, then for the API server
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		if tr, err := b.transport(st.name); err == nil {
			cl := &http.Client{Transport: tr, Timeout: 5 * time.Second}
			if resp, err := cl.Get("https://" + st.name + ":6443/readyz"); err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					b.mu.Lock()
					st.transport, st.ready, st.err = tr, true, ""
					b.mu.Unlock()
					b.log.Info("k3s cluster ready", "container", st.name)
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return errors.New("timed out waiting for the k3s API server")
}

// transport builds an mTLS transport from the container's admin kubeconfig.
func (b *K3sBackend) transport(name string) (*http.Transport, error) {
	raw, err := b.callRaw("GET", "/containers/"+name+"/archive?path=/etc/rancher/k3s/k3s.yaml", nil)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	if _, err := tr.Next(); err != nil {
		return nil, err
	}
	yml, err := io.ReadAll(tr)
	if err != nil {
		return nil, err
	}
	field := func(k string) []byte {
		for _, line := range strings.Split(string(yml), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, k+":") {
				v, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(line, k+":")))
				return v
			}
		}
		return nil
	}
	cert, err := tls.X509KeyPair(field("client-certificate-data"), field("client-key-data"))
	if err != nil {
		return nil, fmt.Errorf("k3s admin credentials: %w", err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(field("certificate-authority-data"))
	return &http.Transport{TLSClientConfig: &tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: pool, ServerName: name, MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: false}, nil
}

// Delete implements Backend.
func (b *K3sBackend) Delete(clusterID string) {
	name := containerName(clusterID)
	go func() { _, _ = b.call("DELETE", "/containers/"+name+"?force=true&v=true", nil, nil) }()
	b.mu.Lock()
	delete(b.clusters, strings.ToLower(clusterID))
	b.mu.Unlock()
}

// Handler implements Backend: proxy to k3s, or 503 while it starts.
func (b *K3sBackend) Handler(cluster map[string]any, _ []map[string]any) http.Handler {
	st := b.state(strOf(cluster["id"]))
	b.mu.Lock()
	ready, errMsg, tr := st.ready, st.err, st.transport
	b.mu.Unlock()
	if !ready {
		if tr == nil && !st.starting {
			b.Ensure(cluster, nil) // emulator restarted: reattach or recreate
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			msg := "the cluster's k3s API server is starting; retry in a few seconds"
			if errMsg != "" {
				msg = "the cluster's k3s API server failed to start: " + errMsg
			}
			w.Header().Set("Retry-After", "5")
			kubeStatus(w, 503, "ServiceUnavailable", msg)
		})
	}
	target := &url.URL{Scheme: "https", Host: st.name + ":6443"}
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = tr
	rp.FlushInterval = -1 // stream watches and logs
	return rp
}

// JWKS implements JWKSProvider.
func (b *K3sBackend) JWKS(clusterID string) (json.RawMessage, bool) {
	st := b.state(clusterID)
	b.mu.Lock()
	tr := st.transport
	b.mu.Unlock()
	if tr == nil {
		return nil, false
	}
	resp, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get("https://" + st.name + ":6443/openid/v1/jwks")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return raw, err == nil && resp.StatusCode == 200
}

// ------------------------------------------------------------ docker API

type dockerError struct {
	status int
	msg    string
}

func (e *dockerError) Error() string { return fmt.Sprintf("docker %d: %s", e.status, e.msg) }

func isNotFound(err error) bool {
	var de *dockerError
	return errors.As(err, &de) && de.status == 404
}

func (b *K3sBackend) callRaw(method, path string, body any) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, "http://docker"+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.docker.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 304 {
		return out, nil
	}
	if resp.StatusCode >= 300 {
		var m struct{ Message string }
		_ = json.Unmarshal(out, &m)
		return nil, &dockerError{status: resp.StatusCode, msg: m.Message}
	}
	return out, nil
}

func (b *K3sBackend) call(method, path string, body any, _ any) (map[string]any, error) {
	raw, err := b.callRaw(method, path, body)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if len(raw) > 0 && raw[0] == '{' {
		_ = json.Unmarshal(raw, &m)
	}
	return m, nil
}
