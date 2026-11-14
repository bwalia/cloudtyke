// Command emulator runs the CloudTyke Azure emulator.
//
//	emulator            serve (default)
//	emulator healthcheck  probe /_cloudtyke/health (used by Docker HEALTHCHECK)
//	emulator coverage     print docs/coverage.md
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bwalia/cloudtyke/azure-emulator/internal/admin"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/app"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/config"
	"github.com/bwalia/cloudtyke/azure-emulator/internal/version"
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "serve":
		os.Exit(serve())
	case "healthcheck":
		os.Exit(healthcheck())
	case "coverage":
		fmt.Print(admin.CoverageMarkdown(app.Registry()))
	case "version":
		fmt.Println(version.Version, version.Commit, version.Date)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (serve|healthcheck|coverage|version)\n", cmd)
		os.Exit(2)
	}
}

func serve() int {
	cfg := config.FromEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	a, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("startup failed", "err", err)
		return 1
	}
	defer a.Store.Close()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           a.Handler,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{a.PKI.ServerTLS}},
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelDebug),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	logger.Info("CloudTyke Azure emulator listening",
		"version", version.Version, "listen", cfg.Listen, "domain", cfg.Domain, "ephemeral", cfg.Ephemeral,
		"metadata_host", cfg.Host(config.SubManagement), "ca", filepath.Join(cfg.CertExportDir, "ca.pem"))
	logger.Warn("local development emulator: not affiliated with Microsoft; never expose this endpoint to untrusted networks")
	if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		logger.Error("server failed", "err", err)
		return 1
	}
	return 0
}

func healthcheck() int {
	cfg := config.FromEnv()
	pool := x509.NewCertPool()
	if b, err := os.ReadFile(filepath.Join(cfg.DataDir, "certs", "ca.pem")); err == nil {
		pool.AppendCertsFromPEM(b)
	}
	port := "443"
	if _, p, err := splitPort(cfg.Listen); err == nil && p != "" {
		port = p
	}
	c := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost"}}}
	resp, err := c.Get("https://127.0.0.1:" + port + "/_cloudtyke/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return 1
	}
	return 0
}

func splitPort(addr string) (string, string, error) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return addr, "", fmt.Errorf("no port")
}
