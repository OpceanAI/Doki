package netlink

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestNewIdentity_GenerateAndReload(t *testing.T) {
	dir := t.TempDir()

	// First call: generates material.
	id1, err := NewIdentity(dir)
	if err != nil {
		t.Fatalf("NewIdentity (first): %v", err)
	}
	if id1.InstallID() == "" {
		t.Fatal("InstallID is empty")
	}
	pub1 := id1.PublicKey()
	if len(pub1) == 0 {
		t.Fatal("PublicKey is empty")
	}
	short1 := id1.InstallID()
	if id1.PublicKeyPEM() == "" {
		t.Fatal("PublicKeyPEM is empty")
	}

	// Second call: should reload the same identity.
	id2, err := NewIdentity(dir)
	if err != nil {
		t.Fatalf("NewIdentity (reload): %v", err)
	}
	if id2.InstallID() != short1 {
		t.Errorf("InstallID changed across reload: %q vs %q", id2.InstallID(), short1)
	}
	if string(id2.PublicKey()) != string(pub1) {
		t.Error("PublicKey changed across reload")
	}

	// Sign / verify roundtrip.
	msg := []byte("hello mesh")
	sig := id1.Sign(msg)
	if !id1.Verify(msg, sig) {
		t.Error("Verify failed for own signature")
	}
	if id1.Verify([]byte("tampered"), sig) {
		t.Error("Verify accepted tampered message")
	}
}

func TestIdentity_CAValid(t *testing.T) {
	dir := t.TempDir()
	id, err := NewIdentity(dir)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	caPEM := id.CACertPEM()
	if len(caPEM) == 0 {
		t.Fatal("CACertPEM is empty")
	}
	block, _ := pem.Decode(caPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatalf("invalid CA PEM block: %+v", block)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}
	if !cert.IsCA {
		t.Error("CA cert is not marked IsCA")
	}
	// NotAfter should be ~1 year in the future (allow 30d slack).
	lifeDays := cert.NotAfter.Sub(cert.NotBefore).Hours() / 24
	if lifeDays < 300 || lifeDays > 400 {
		t.Errorf("CA lifetime = %.1f days, want ~365", lifeDays)
	}
}

func TestIdentity_IssueLinkCert(t *testing.T) {
	dir := t.TempDir()
	id, err := NewIdentity(dir)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	link, err := id.IssueLinkCert("test-peer")
	if err != nil {
		t.Fatalf("IssueLinkCert: %v", err)
	}
	if link.Leaf == nil {
		t.Fatal("leaf is nil")
	}
	if link.Leaf.Subject.CommonName != "test-peer" {
		t.Errorf("CN = %q, want test-peer", link.Leaf.Subject.CommonName)
	}
	if link.Leaf.IsCA {
		t.Error("link cert is marked CA (should be leaf)")
	}
	// Lifetime should be ~90 days.
	lifeDays := link.Leaf.NotAfter.Sub(link.Leaf.NotBefore).Hours() / 24
	if lifeDays < 80 || lifeDays > 95 {
		t.Errorf("link cert lifetime = %.1f days, want ~90", lifeDays)
	}
	// Verify it chains to the CA.
	pool := x509.NewCertPool()
	pool.AddCert(id.caCert)
	if _, err := link.Leaf.Verify(x509.VerifyOptions{
		Roots: pool,
		KeyUsages: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth,
		},
	}); err != nil {
		t.Errorf("link cert does not chain to CA: %v", err)
	}
}

func TestIdentity_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewIdentity(dir); err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	// Private key files must be 0600 (or stricter).
	for _, name := range []string{"id_ed25519", "ca.key"} {
		path := filepath.Join(dir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Errorf("stat %s: %v", name, err)
			continue
		}
		mode := fi.Mode().Perm()
		if mode&0o077 != 0 {
			t.Errorf("%s has mode %o, want no group/world bits", name, mode)
		}
	}
}

func TestShortID_Deterministic(t *testing.T) {
	pub := make([]byte, 32)
	for i := range pub {
		pub[i] = byte(i)
	}
	a := shortID(pub)
	b := shortID(pub)
	if a != b {
		t.Errorf("shortID not deterministic: %q vs %q", a, b)
	}
	if len(a) == 0 || len(a) > 14 {
		t.Errorf("shortID length = %d, want 1..14", len(a))
	}
}
