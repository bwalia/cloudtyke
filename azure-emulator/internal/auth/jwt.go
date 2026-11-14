// Package auth implements the emulated Entra ID token service (OAuth2 /
// OIDC endpoints, RS256 JWT issuance, JWKS) and bearer-token validation
// for the ARM and Graph front ends.
package auth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Signer issues and verifies RS256 JWTs with a persistent key.
type Signer struct {
	key  *rsa.PrivateKey
	cert *x509.Certificate
	kid  string
}

// LoadOrCreateSigner loads the token signing key from dir (creating it if needed).
func LoadOrCreateSigner(dir string) (*Signer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "token-signing-key.pem")
	var key *rsa.PrivateKey
	if b, err := os.ReadFile(path); err == nil {
		if blk, _ := pem.Decode(b); blk != nil {
			if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
				key = k
			}
		}
	}
	if key == nil {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		key = k
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
		if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
			return nil, err
		}
	}
	return newSigner(key)
}

// NewEphemeralSigner creates an in-memory signer (tests, --ephemeral).
func NewEphemeralSigner() (*Signer, error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return newSigner(k)
}

func newSigner(key *rsa.PrivateKey) (*Signer, error) {
	// A self-signed certificate is published in the JWKS (x5c) because some
	// client libraries only validate via the certificate chain.
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "CloudTyke token signing"},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Now().AddDate(20, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, _ := x509.ParseCertificate(der)
	sum := sha1.Sum(der)
	return &Signer{key: key, cert: cert, kid: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

// KeyID returns the kid / x5t of the signing key.
func (s *Signer) KeyID() string { return s.kid }

// Sign produces a compact RS256 JWT for the claims.
func (s *Signer) Sign(claims map[string]any) (string, error) {
	hdr := map[string]any{"typ": "JWT", "alg": "RS256", "kid": s.kid, "x5t": s.kid}
	hb, _ := json.Marshal(hdr)
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signing := b64(hb) + "." + b64(cb)
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64(sig), nil
}

// Errors returned by Verify.
var (
	ErrMalformed = errors.New("malformed token")
	ErrSignature = errors.New("invalid signature")
	ErrExpired   = errors.New("token expired")
)

// Verify checks signature and expiry and returns the claims.
func (s *Signer) Verify(token string) (map[string]any, error) {
	return VerifyWithKey(token, &s.key.PublicKey, time.Now())
}

// VerifyWithKey verifies an RS256 JWT against an RSA public key.
func VerifyWithKey(token string, pub *rsa.PublicKey, now time.Time) (map[string]any, error) {
	claims, hdr, sig, signing, err := Split(token)
	if err != nil {
		return nil, err
	}
	if alg, _ := hdr["alg"].(string); alg != "RS256" {
		return nil, fmt.Errorf("%w: unsupported alg %q", ErrMalformed, alg)
	}
	h := sha256.Sum256([]byte(signing))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, h[:], sig); err != nil {
		return nil, ErrSignature
	}
	if exp, ok := claims["exp"].(float64); ok && now.Unix() > int64(exp)+60 {
		return claims, ErrExpired
	}
	if nbf, ok := claims["nbf"].(float64); ok && now.Unix()+60 < int64(nbf) {
		return claims, fmt.Errorf("%w: token not yet valid", ErrMalformed)
	}
	return claims, nil
}

// Split decodes a JWT without verifying it.
func Split(token string) (claims, header map[string]any, sig []byte, signing string, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, nil, "", ErrMalformed
	}
	hb, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	cb, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	sig, err3 := base64.RawURLEncoding.DecodeString(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, nil, nil, "", ErrMalformed
	}
	if json.Unmarshal(hb, &header) != nil || json.Unmarshal(cb, &claims) != nil {
		return nil, nil, nil, "", ErrMalformed
	}
	return claims, header, sig, parts[0] + "." + parts[1], nil
}

// JWKS returns the JSON Web Key Set document.
func (s *Signer) JWKS() map[string]any {
	pub := s.key.PublicKey
	return map[string]any{"keys": []any{map[string]any{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": s.kid,
		"x5t": s.kid,
		"n":   b64(pub.N.Bytes()),
		"e":   b64(big.NewInt(int64(pub.E)).Bytes()),
		"x5c": []string{base64.StdEncoding.EncodeToString(s.cert.Raw)},
	}}}
}

// PublicKey exposes the verification key.
func (s *Signer) PublicKey() *rsa.PublicKey { return &s.key.PublicKey }

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// ParseJWK turns a JWK (RSA) into a public key.
func ParseJWK(k map[string]any) (*rsa.PublicKey, error) {
	n, _ := k["n"].(string)
	e, _ := k["e"].(string)
	nb, err1 := base64.RawURLEncoding.DecodeString(n)
	eb, err2 := base64.RawURLEncoding.DecodeString(e)
	if err1 != nil || err2 != nil || len(nb) == 0 {
		return nil, errors.New("invalid JWK")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}, nil
}
