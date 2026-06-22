package netlink

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestTrustStore_TrustAndPubKey(t *testing.T) {
	dir := t.TempDir()
	ts, err := NewTrustStore(dir)
	if err != nil {
		t.Fatalf("NewTrustStore: %v", err)
	}
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	id := "abcd1234efgh"
	if err := ts.Trust(id, pub, nil); err != nil {
		t.Fatalf("Trust: %v", err)
	}
	if !ts.Trusted(id) {
		t.Error("peer should be trusted")
	}
	got, err := ts.PubKey(id)
	if err != nil {
		t.Fatalf("PubKey: %v", err)
	}
	if subtle.ConstantTimeCompare(got, pub) != 1 {
		t.Error("PubKey round-trip mismatch")
	}
}

func TestTrustStore_TrustTwiceSameKeyOK(t *testing.T) {
	dir := t.TempDir()
	ts, _ := NewTrustStore(dir)
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	if err := ts.Trust("p1", pub, nil); err != nil {
		t.Fatal(err)
	}
	if err := ts.Trust("p1", pub, nil); err != nil {
		t.Errorf("second trust with same key should be no-op, got: %v", err)
	}
}

func TestTrustStore_TrustConflict(t *testing.T) {
	dir := t.TempDir()
	ts, _ := NewTrustStore(dir)
	_, priv1, _ := ed25519.GenerateKey(nil)
	_, priv2, _ := ed25519.GenerateKey(nil)
	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)
	if err := ts.Trust("p1", pub1, nil); err != nil {
		t.Fatal(err)
	}
	if err := ts.Trust("p1", pub2, nil); err == nil {
		t.Error("expected conflict on different pubkey")
	}
}

func TestTrustStore_PersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	ts, _ := NewTrustStore(dir)
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	if err := ts.Trust("peer-x", pub, nil); err != nil {
		t.Fatal(err)
	}

	// New instance should load from disk.
	ts2, err := NewTrustStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ts2.Load(); err != nil {
		t.Fatal(err)
	}
	if !ts2.Trusted("peer-x") {
		t.Error("peer-x should have been loaded from disk")
	}
	got, err := ts2.PubKey("peer-x")
	if err != nil {
		t.Fatal(err)
	}
	if subtle.ConstantTimeCompare(got, pub) != 1 {
		t.Error("loaded pubkey mismatch")
	}
}

func TestTrustStore_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	ts, _ := NewTrustStore(dir)
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	if err := ts.Trust("p9", pub, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "p9.pub.pem")
	info, err := fileInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	// The file must not be world-writable; 0600 or 0644 are both OK
	// depending on umask (umask 0077 yields 0600, umask 0022 yields 0644).
	if mode&0o002 != 0 {
		t.Errorf("pubkey file is world-writable: %o", mode)
	}
	if mode&0o077 > 0o077 {
		// i.e. group + other bits all zero is acceptable; 0600 passes.
	}
}

func TestPeer_FingerprintAndPubKey(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	p := &Peer{
		ID:     "abc",
		PubKey: base64.StdEncoding.EncodeToString(pub),
	}
	got, err := p.pubKeyBytes()
	if err != nil {
		t.Fatalf("pubKeyBytes: %v", err)
	}
	if subtle.ConstantTimeCompare(got, pub) != 1 {
		t.Error("pubKeyBytes round-trip")
	}
	if p.fingerprint() != "abc" {
		t.Errorf("fingerprint = %q, want abc", p.fingerprint())
	}
}

func TestPeer_InvalidPubKey(t *testing.T) {
	p := &Peer{PubKey: "not-base64!!!"}
	if _, err := p.pubKeyBytes(); err == nil {
		t.Error("expected error for invalid pubkey")
	}
}

// fileInfo is a tiny helper that hides os.Stat for tests.
func fileInfo(path string) (os.FileInfo, error) {
	return os.Stat(path)
}
