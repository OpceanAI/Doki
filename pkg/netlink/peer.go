package netlink

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Peer represents a remote Doki instance participating in the mesh.
// Each peer is uniquely identified by its short install ID (the first
// 12 chars of base32 of its Ed25519 public key).
type Peer struct {
	ID       string             `json:"id"`        // short install id
	Name     string             `json:"name"`      // human-friendly (defaults to ID)
	Addr     string             `json:"addr"`      // host:port the peer listens on
	PubKey   string             `json:"pubkey"`    // base64 of Ed25519 public key
	CACert   string             `json:"ca_cert"`   // PEM-encoded CA cert
	LastSeen time.Time          `json:"last_seen"`
	Containers []RemoteContainer `json:"containers,omitempty"`
}

// RemoteContainer describes a container running on a peer that we can
// reach through the mesh.
type RemoteContainer struct {
	Name     string `json:"name"`
	ID       string `json:"id"`
	IP       string `json:"ip"`
	Port     uint16 `json:"port"`
	Proto    string `json:"proto"`
	HostPort uint16 `json:"host_port,omitempty"`
}

// pubKeyBytes parses and returns the Ed25519 public key bytes.
func (p *Peer) pubKeyBytes() (ed25519.PublicKey, error) {
	if len(p.PubKey) == 0 {
		return nil, errors.New("peer: empty pubkey")
	}
	raw, err := base64.StdEncoding.DecodeString(p.PubKey)
	if err != nil {
		return nil, fmt.Errorf("peer: decode pubkey: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("peer: pubkey size = %d, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// fingerprint returns a short, log-safe identifier for the peer.
func (p *Peer) fingerprint() string {
	if p.ID != "" {
		return p.ID
	}
	if len(p.PubKey) >= 8 {
		return p.PubKey[:8]
	}
	return "unknown"
}

// TrustStore persists known peer public keys and CA certs. Trust is
// established on first contact (TOFU) and then verified on every
// subsequent message.
type TrustStore struct {
	root    string // typically $DOKI_ROOT/keys/peers
	mu      sync.RWMutex
	trusted map[string]*trustedPeer
}

type trustedPeer struct {
	PubKey   ed25519.PublicKey
	CACert   *x509.Certificate
	FirstSeen time.Time
}

// NewTrustStore creates a TrustStore rooted at dir. The directory is
// created if it does not exist.
func NewTrustStore(dir string) (*TrustStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("trust store: mkdir: %w", err)
	}
	return &TrustStore{
		root:    dir,
		trusted: make(map[string]*trustedPeer),
	}, nil
}

// Trust records a peer's public key and (optionally) CA cert. If the
// peer is already trusted, the existing record is returned unless the
// new pubkey differs (in which case an error is returned to surface the
// potential MITM or key-rotation mismatch).
func (ts *TrustStore) Trust(peerID string, pub ed25519.PublicKey, ca *x509.Certificate) error {
	if peerID == "" {
		return errors.New("trust store: empty peer id")
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("trust store: pub size = %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if existing, ok := ts.trusted[peerID]; ok {
		if !bytesEqual(existing.PubKey, pub) {
			return fmt.Errorf("trust store: peer %q pubkey mismatch (TOFU collision)", peerID)
		}
		return nil
	}
	ts.trusted[peerID] = &trustedPeer{
		PubKey:    pub,
		CACert:    ca,
		FirstSeen: time.Now(),
	}
	return ts.persistUnlocked(peerID, pub, ca)
}

// Trusted returns true if peerID has been recorded in the store.
func (ts *TrustStore) Trusted(peerID string) bool {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	_, ok := ts.trusted[peerID]
	return ok
}

// PubKey returns the trusted public key for peerID, or an error.
func (ts *TrustStore) PubKey(peerID string) (ed25519.PublicKey, error) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	tp, ok := ts.trusted[peerID]
	if !ok {
		return nil, fmt.Errorf("trust store: peer %q not trusted", peerID)
	}
	return tp.PubKey, nil
}

// CACert returns the trusted CA cert for peerID, or nil if none was
// stored.
func (ts *TrustStore) CACert(peerID string) *x509.Certificate {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	tp, ok := ts.trusted[peerID]
	if !ok {
		return nil
	}
	return tp.CACert
}

// ListIDs returns all trusted peer IDs.
func (ts *TrustStore) ListIDs() []string {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	out := make([]string, 0, len(ts.trusted))
	for id := range ts.trusted {
		out = append(out, id)
	}
	return out
}

// Load reads all .pub.pem and .ca.pem files from the trust store
// directory. Called at startup.
func (ts *TrustStore) Load() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	entries, err := os.ReadDir(ts.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".pem" {
			continue
		}
		base := e.Name()[:len(e.Name())-len(".pem")]
		parts := splitLast(base, ".")
		if len(parts) != 2 {
			continue
		}
		peerID, kind := parts[0], parts[1]
		path := filepath.Join(ts.root, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		tp, ok := ts.trusted[peerID]
		if !ok {
			tp = &trustedPeer{}
			ts.trusted[peerID] = tp
		}
		switch kind {
		case "pub":
			block, _ := pem.Decode(data)
			if block == nil {
				continue
			}
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				continue
			}
			if edPub, ok := pub.(ed25519.PublicKey); ok {
				tp.PubKey = edPub
			}
		case "ca":
			block, _ := pem.Decode(data)
			if block == nil {
				continue
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil {
				tp.CACert = cert
			}
		}
	}
	return nil
}

func (ts *TrustStore) persistUnlocked(peerID string, pub ed25519.PublicKey, ca *x509.Certificate) error {
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if err := os.WriteFile(filepath.Join(ts.root, peerID+".pub.pem"), pubPEM, 0600); err != nil {
		return err
	}
	if ca != nil {
		caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
		if err := os.WriteFile(filepath.Join(ts.root, peerID+".ca.pem"), caPEM, 0600); err != nil {
			return err
		}
	}
	return nil
}

func splitLast(s, sep string) []string {
	i := len(s) - 1
	for i >= 0 {
		if string(s[i]) == sep {
			return []string{s[:i], s[i+1:]}
		}
		i--
	}
	return []string{s}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
