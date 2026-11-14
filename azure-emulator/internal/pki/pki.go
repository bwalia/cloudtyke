// Package pki creates and persists the emulator's local certificate
// authority and the HTTPS server certificate signed by it.
//
// The CA never leaves the user's machine. It is exported to
// CertExportDir/ca.pem so tools (Terraform, az, SDKs, curl) can trust it.
package pki

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

// Bundle is the loaded CA and server certificate.
type Bundle struct {
	CACert    *x509.Certificate
	CAPEM     []byte
	ServerTLS tls.Certificate
}

const (
	caCertFile  = "ca.pem"
	caKeyFile   = "ca-key.pem"
	srvCertFile = "server.pem"
	srvKeyFile  = "server-key.pem"
	// Apple platforms refuse server certs valid for more than 825 days even
	// for user-trusted roots; stay well under that.
	serverValidity = 397 * 24 * time.Hour
)

// LoadOrCreate loads the CA from dir (creating it on first start) and makes
// sure a server certificate covering hosts exists and is not near expiry.
// If exportDir is non-empty the CA certificate is copied there as ca.pem.
func LoadOrCreate(dir, exportDir string, hosts []string) (*Bundle, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	caCert, caKey, caPEM, err := loadOrCreateCA(dir)
	if err != nil {
		return nil, fmt.Errorf("CA: %w", err)
	}
	srv, err := loadOrCreateServer(dir, caCert, caKey, hosts)
	if err != nil {
		return nil, fmt.Errorf("server certificate: %w", err)
	}
	if exportDir != "" {
		if err := os.MkdirAll(exportDir, 0o755); err == nil {
			_ = os.WriteFile(filepath.Join(exportDir, caCertFile), caPEM, 0o644)
		}
	}
	return &Bundle{CACert: caCert, CAPEM: caPEM, ServerTLS: srv}, nil
}

func loadOrCreateCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	certPath, keyPath := filepath.Join(dir, caCertFile), filepath.Join(dir, caKeyFile)
	certPEM, err1 := os.ReadFile(certPath)
	keyPEM, err2 := os.ReadFile(keyPath)
	if err1 == nil && err2 == nil {
		cert, key, err := parsePair(certPEM, keyPEM)
		if err == nil && time.Until(cert.NotAfter) > 30*24*time.Hour {
			return cert, key, certPEM, nil
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	host, _ := os.Hostname()
	tmpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{Organization: []string{"CloudTyke local development CA"}, CommonName: "CloudTyke Local CA (" + host + ")"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, err
	}
	cert, _ := x509.ParseCertificate(der)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	kb, _ := x509.MarshalECPrivateKey(key)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, nil, nil, err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, nil, nil, err
	}
	// a new CA invalidates any existing server cert
	_ = os.Remove(filepath.Join(dir, srvCertFile))
	return cert, key, certPEM, nil
}

func loadOrCreateServer(dir string, ca *x509.Certificate, caKey *ecdsa.PrivateKey, hosts []string) (tls.Certificate, error) {
	certPath, keyPath := filepath.Join(dir, srvCertFile), filepath.Join(dir, srvKeyFile)
	certPEM, err1 := os.ReadFile(certPath)
	keyPEM, err2 := os.ReadFile(keyPath)
	if err1 == nil && err2 == nil {
		cert, _, err := parsePair(certPEM, keyPEM)
		if err == nil && time.Until(cert.NotAfter) > 30*24*time.Hour && sameHosts(cert, hosts) && cert.CheckSignatureFrom(ca) == nil {
			return tls.X509KeyPair(append(certPEM, pemOf(ca)...), keyPEM)
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{Organization: []string{"CloudTyke emulator"}, CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(serverValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	kb, _ := x509.MarshalECPrivateKey(key)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(append(certPEM, pemOf(ca)...), keyPEM)
}

func pemOf(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

func parsePair(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, nil, errors.New("invalid PEM")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func sameHosts(cert *x509.Certificate, hosts []string) bool {
	have := append([]string{}, cert.DNSNames...)
	for _, ip := range cert.IPAddresses {
		have = append(have, ip.String())
	}
	want := make([]string, 0, len(hosts))
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			want = append(want, ip.String())
		} else {
			want = append(want, h)
		}
	}
	sort.Strings(have)
	sort.Strings(want)
	return slices.Equal(slices.Compact(have), slices.Compact(want))
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 126))
	return n
}

// VerifyChain is a helper for tests: checks the server cert is valid for host.
func (b *Bundle) VerifyChain(host string) error {
	leaf, err := x509.ParseCertificate(b.ServerTLS.Certificate[0])
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	pool.AddCert(b.CACert)
	_, err = leaf.Verify(x509.VerifyOptions{DNSName: host, Roots: pool})
	return err
}

// Equal reports whether the exported CA matches pemBytes (used by `tyke trust`).
func (b *Bundle) Equal(pemBytes []byte) bool {
	return bytes.Equal(bytes.TrimSpace(b.CAPEM), bytes.TrimSpace(pemBytes))
}
