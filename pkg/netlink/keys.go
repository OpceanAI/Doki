package netlink

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Identity holds the per-Doki-install cryptographic identity used by the
// mesh layer. It consists of:
//
//   - An Ed25519 keypair (identity), used to sign mesh messages
//     (HELLO, ADVERTISE, REVOKE, BYE). The private key never leaves the
//     host. The public key is broadcast as the peer "fingerprint".
//   - A self-signed ECDSA P-256 certificate authority (CA), used to mint
//     per-link TLS certificates on demand. The CA private key never
//     leaves the host.
//   - Zero or more per-link ECDSA certificates, lazily generated and
//     cached on disk at keys/<id>.crt / keys/<id>.key.
//
// The Identity is loaded (or generated) at daemon startup. Key files are
// stored with 0600 permissions. Loading is safe to call multiple times;
// generation is gated by sync.Once.
type Identity struct {
	root      string // $DOKI_ROOT/keys
	installID string // short identifier, derived from the Ed25519 pubkey

	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey

	identity ed25519.PrivateKey // 64 bytes (seed + pub)
	idPub    ed25519.PublicKey  // 32 bytes

	once sync.Once
	err  error
}

// ShortID returns the short install identifier derived from the
// Ed25519 public key. Stable for the lifetime of the keys on disk.
func (id *Identity) ShortID() string {
	return id.installID
}

// PrivateKey returns the Ed25519 private key bytes (64). Callers must
// not mutate the returned slice. Only used for signing in this
// package's mesh code.
func (id *Identity) PrivateKey() ed25519.PrivateKey {
	return id.identity
}

// CACert returns the self-signed CA certificate.
func (id *Identity) CACert() *x509.Certificate {
	return id.caCert
}

// NewIdentity returns the Doki install identity, loading from disk or
// generating fresh material if absent. root is the keys directory
// (typically $DOKI_ROOT/keys).
func NewIdentity(root string) (*Identity, error) {
	id := &Identity{root: root}
	id.once.Do(func() {
		id.err = id.loadOrGenerate()
	})
	if id.err != nil {
		return nil, id.err
	}
	return id, nil
}

// InstallID returns a short, stable, URL-safe identifier derived from the
// Ed25519 public key (first 16 chars of base32). It is safe to log.
func (id *Identity) InstallID() string {
	return id.installID
}

// PublicKey returns the raw Ed25519 public key bytes.
func (id *Identity) PublicKey() ed25519.PublicKey {
	return id.idPub
}

// PublicKeyPEM returns the Ed25519 public key in PEM format for export.
func (id *Identity) PublicKeyPEM() string {
	pubBytes, err := x509.MarshalPKIXPublicKey(id.idPub)
	if err != nil {
		return ""
	}
	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}
	return string(pem.EncodeToMemory(block))
}

// Sign returns the Ed25519 signature of msg.
func (id *Identity) Sign(msg []byte) []byte {
	return ed25519.Sign(id.identity, msg)
}

// Verify verifies an Ed25519 signature made by this identity. Used in
// tests; mesh code calls ed25519.Verify directly with the remote peer's
// public key.
func (id *Identity) Verify(msg, sig []byte) bool {
	return ed25519.Verify(id.idPub, msg, sig)
}

// CACertPEM returns the CA certificate in PEM form, suitable for use as
// tls.Config.RootCAs or for inclusion in a tls.Config.Certificates pair.
func (id *Identity) CACertPEM() []byte {
	if id.caCert == nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: id.caCert.Raw,
	})
}

// CAKeyPEM returns the CA private key in PEM form. NEVER ship this over
// the wire; it is exposed here only for the local TLS wrapper to load
// into crypto/tls.
func (id *Identity) CAKeyPEM() []byte {
	if id.caKey == nil {
		return nil
	}
	der, err := x509.MarshalECPrivateKey(id.caKey)
	if err != nil {
		return nil
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: der,
	})
}

// IssueLinkCert generates a fresh ECDSA P-256 leaf certificate signed by
// the CA. commonName is the link/peer identifier (e.g. the remote
// peer's install ID or a container name).
//
// The returned LoadedCert is the minimal subset of tls.Certificate we
// need: a parsed leaf plus DER-encoded key. The TLS wrapper
// (crypto.go) turns it into a real crypto/tls.Certificate.
func (id *Identity) IssueLinkCert(commonName string) (LoadedCert, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return LoadedCert{}, fmt.Errorf("generate link key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return LoadedCert{}, fmt.Errorf("generate serial: %w", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-1 * time.Hour),
		NotAfter:     now.Add(90 * 24 * time.Hour), // 90 days
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{commonName}, // SAN required since Go 1.15
	}
	der, err := x509.CreateCertificate(rand.Reader, template, id.caCert, &key.PublicKey, id.caKey)
	if err != nil {
		return LoadedCert{}, fmt.Errorf("create link cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return LoadedCert{}, fmt.Errorf("parse link cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return LoadedCert{}, fmt.Errorf("marshal link key: %w", err)
	}
	return LoadedCert{Leaf: cert, KeyDER: keyDER}, nil
}

// IssueLinkCertForContainer is a convenience wrapper for the typical
// "generate a cert for a container's exposed port" use case.
func (id *Identity) IssueLinkCertForContainer(containerID string) (LoadedCert, error) {
	return id.IssueLinkCert("doki-link:" + containerID)
}

// LoadedCert is the minimal subset of tls.Certificate we need: a parsed
// leaf certificate plus the matching private key in DER. We avoid
// importing crypto/tls here to keep the keys file portable; the TLS
// wrapper (crypto.go) does the conversion.
type LoadedCert struct {
	Leaf   *x509.Certificate
	KeyDER []byte
}

// loadOrGenerate brings the identity into memory. It tries to load
// existing material from disk; on failure, it generates a fresh
// identity + CA and writes them to disk with 0600 permissions.
func (id *Identity) loadOrGenerate() error {
	if err := os.MkdirAll(id.root, 0700); err != nil {
		return fmt.Errorf("create keys dir: %w", err)
	}

	// 1. Ed25519 identity.
	identityPath := filepath.Join(id.root, "id_ed25519")
	identity, idPub, err := loadOrCreateEd25519(identityPath)
	if err != nil {
		return err
	}
	id.identity = identity
	id.idPub = idPub
	id.installID = shortID(idPub)

	// 2. CA certificate + key.
	caCertPath := filepath.Join(id.root, "ca.crt")
	caKeyPath := filepath.Join(id.root, "ca.key")
	caCert, caKey, err := loadOrCreateCA(caCertPath, caKeyPath, id.installID)
	if err != nil {
		return err
	}
	id.caCert = caCert
	id.caKey = caKey

	return nil
}

func loadOrCreateEd25519(path string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	if data, err := os.ReadFile(path); err == nil {
		if len(data) == ed25519.PrivateKeySize {
			priv := ed25519.PrivateKey(data)
			return priv, priv.Public().(ed25519.PublicKey), nil
		}
		// Legacy PEM form, parse and rewrite.
		block, _ := pem.Decode(data)
		if block != nil {
			if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				if priv, ok := key.(ed25519.PrivateKey); ok {
					return priv, priv.Public().(ed25519.PublicKey), nil
				}
			}
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519: %w", err)
	}
	if err := os.WriteFile(path, priv, 0600); err != nil {
		return nil, nil, fmt.Errorf("write identity: %w", err)
	}
	return priv, pub, nil
}

func loadOrCreateCA(certPath, keyPath, installID string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	// Try to load.
	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	if certErr == nil && keyErr == nil {
		cb, _ := pem.Decode(certPEM)
		kb, _ := pem.Decode(keyPEM)
		if cb != nil && cb.Type == "CERTIFICATE" && kb != nil && kb.Type == "EC PRIVATE KEY" {
			cert, err1 := x509.ParseCertificate(cb.Bytes)
			key, err2 := x509.ParseECPrivateKey(kb.Bytes)
			if err1 == nil && err2 == nil {
				// Validate the CA is still valid.
				if time.Now().Before(cert.NotAfter) {
					return cert, key, nil
				}
			}
		}
	}

	// Generate.
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("CA serial: %w", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Doki"},
			CommonName:   "DokiLink CA " + installID,
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA cert: %w", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal CA key: %w", err)
	}
	certPEMOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	keyPEMOut := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEMOut, 0644); err != nil {
		return nil, nil, fmt.Errorf("write CA cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEMOut, 0600); err != nil {
		return nil, nil, fmt.Errorf("write CA key: %w", err)
	}
	return cert, priv, nil
}

// shortID returns a short, URL-safe identifier derived from the leading
// 8 bytes of pub. Used for logs and the install ID.
func shortID(pub ed25519.PublicKey) string {
	if len(pub) < 8 {
		return fmt.Sprintf("%x", []byte(pub))
	}
	// base32 is URL-safe and 8 bytes -> 14 chars; we take 12 for compactness.
	const alphabet = "abcdefghijklmnopqrstuvwxyz234567"
	var out [12]byte
	v := uint64(0)
	bits := 0
	idx := 0
	for _, b := range pub[:8] {
		v = (v << 8) | uint64(b)
		bits += 8
		for bits >= 5 && idx < 12 {
			bits -= 5
			out[idx] = alphabet[(v>>uint(bits))&0x1f]
			idx++
		}
	}
	return string(out[:idx])
}

// PeerKeyFile returns the on-disk path for a peer's pinned public key.
// Used by the mesh layer for trust-on-first-use storage.
func (id *Identity) PeerKeyFile(peerID string) string {
	return filepath.Join(id.root, "peers", peerID+".pub.pem")
}

// ErrIdentityNotLoaded is returned by NewIdentity when generation failed.
var ErrIdentityNotLoaded = errors.New("netlink: identity not loaded")
