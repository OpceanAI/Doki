package netlink

import (
	"crypto/ed25519"
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestStaticPeers_AddAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")
	sp, err := NewStaticPeers(path)
	if err != nil {
		t.Fatalf("NewStaticPeers: %v", err)
	}
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	p := &Peer{
		ID:     "peer-a",
		Addr:   "192.168.1.10:7432",
		PubKey: base64.StdEncoding.EncodeToString(pub),
	}
	if err := sp.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	got := sp.Get("peer-a")
	if got == nil {
		t.Fatal("Get returned nil after Add")
	}
	if got.Addr != "192.168.1.10:7432" {
		t.Errorf("addr = %q", got.Addr)
	}
}

func TestStaticPeers_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	sp, _ := NewStaticPeers(filepath.Join(dir, "p.json"))
	if err := sp.Add(&Peer{ID: ""}); err == nil {
		t.Error("empty id should fail")
	}
	if err := sp.Add(&Peer{ID: "x", PubKey: ""}); err == nil {
		t.Error("empty pubkey should fail")
	}
	if err := sp.Add(&Peer{ID: "x", PubKey: "abc", Addr: ""}); err == nil {
		t.Error("empty addr should fail")
	}
}

func TestStaticPeers_DuplicateIDDifferentKey(t *testing.T) {
	dir := t.TempDir()
	sp, _ := NewStaticPeers(filepath.Join(dir, "p.json"))
	_, p1, _ := ed25519.GenerateKey(nil)
	_, p2, _ := ed25519.GenerateKey(nil)
	a := &Peer{ID: "p", Addr: "h:1", PubKey: base64.StdEncoding.EncodeToString(p1.Public().(ed25519.PublicKey))}
	b := &Peer{ID: "p", Addr: "h:2", PubKey: base64.StdEncoding.EncodeToString(p2.Public().(ed25519.PublicKey))}
	if err := sp.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := sp.Add(b); err == nil {
		t.Error("expected conflict on different pubkey")
	}
}

func TestStaticPeers_DuplicateIDSameKeyUpdatesAddr(t *testing.T) {
	dir := t.TempDir()
	sp, _ := NewStaticPeers(filepath.Join(dir, "p.json"))
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	a := &Peer{ID: "p", Addr: "h:1", PubKey: pub}
	b := &Peer{ID: "p", Addr: "h:2", PubKey: pub}
	if err := sp.Add(a); err != nil {
		t.Fatal(err)
	}
	if err := sp.Add(b); err != nil {
		t.Fatalf("expected addr update, got: %v", err)
	}
	if got := sp.Get("p"); got == nil || got.Addr != "h:2" {
		t.Errorf("addr not updated: %+v", got)
	}
}

func TestStaticPeers_PersistsToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")
	sp, _ := NewStaticPeers(path)
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	if err := sp.Add(&Peer{ID: "p", Addr: "h:1", PubKey: base64.StdEncoding.EncodeToString(pub)}); err != nil {
		t.Fatal(err)
	}

	sp2, err := NewStaticPeers(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if sp2.Get("p") == nil {
		t.Error("peer not persisted")
	}
}

func TestStaticPeers_Remove(t *testing.T) {
	dir := t.TempDir()
	sp, _ := NewStaticPeers(filepath.Join(dir, "p.json"))
	_, priv, _ := ed25519.GenerateKey(nil)
	sp.Add(&Peer{ID: "p", Addr: "h:1", PubKey: base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))})
	if err := sp.Remove("p"); err != nil {
		t.Fatal(err)
	}
	if sp.Get("p") != nil {
		t.Error("peer should be removed")
	}
}

func TestStaticPeers_List(t *testing.T) {
	dir := t.TempDir()
	sp, _ := NewStaticPeers(filepath.Join(dir, "p.json"))
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	sp.Add(&Peer{ID: "a", Addr: "h:1", PubKey: pub})
	sp.Add(&Peer{ID: "b", Addr: "h:2", PubKey: pub})
	if got := sp.List(); len(got) != 2 {
		t.Errorf("list len = %d, want 2", len(got))
	}
}
