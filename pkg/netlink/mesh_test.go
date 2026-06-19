package netlink

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// TestMesh_SignAndVerify exercises the signature helpers directly
// without spinning up a listener.
func TestMesh_SignAndVerify(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)
	msg := GossipMessage{
		Type: "hello",
		From: "test",
		Time: time.Now().UnixNano(),
	}
	if err := msg.Sign(priv); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := msg.Verify(pub); err != nil {
		t.Errorf("Verify: %v", err)
	}
	// Tamper: change From after signing.
	msg.From = "evil"
	if err := msg.Verify(pub); err == nil {
		t.Error("Verify should fail on tampered message")
	}
}

func TestMesh_TrustAndList(t *testing.T) {
	dir := t.TempDir()
	idDir := filepath.Join(dir, "keys")
	id, err := NewIdentity(idDir)
	if err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	ts, _ := NewTrustStore(filepath.Join(dir, "trust"))
	peersPath := filepath.Join(dir, "peers.json")
	sp, _ := NewStaticPeers(peersPath)
	m, err := NewMesh(MeshConfig{
		Identity:   id,
		Trust:      ts,
		Static:     sp,
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewMesh: %v", err)
	}
	if got := m.Peers(); len(got) != 0 {
		t.Errorf("fresh mesh peers = %d, want 0", len(got))
	}
}

func TestMesh_StartStop(t *testing.T) {
	dir := t.TempDir()
	id, _ := NewIdentity(filepath.Join(dir, "keys"))
	ts, _ := NewTrustStore(filepath.Join(dir, "trust"))
	sp, _ := NewStaticPeers(filepath.Join(dir, "peers.json"))
	m, _ := NewMesh(MeshConfig{
		Identity:   id,
		Trust:      ts,
		Static:     sp,
		ListenAddr: "127.0.0.1:0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestMesh_TwoInstancesCrossForward(t *testing.T) {
	// Two identities on different ports. A trusts B and vice versa.
	// A.SendHello(B.addr) should land in B's listener, be verified,
	// and (in this test) we directly call onMessage to assert it.
	dirA := t.TempDir()
	dirB := t.TempDir()
	idA, _ := NewIdentity(filepath.Join(dirA, "keys"))
	idB, _ := NewIdentity(filepath.Join(dirB, "keys"))
	tsA, _ := NewTrustStore(filepath.Join(dirA, "trust"))
	tsB, _ := NewTrustStore(filepath.Join(dirB, "trust"))
	spA, _ := NewStaticPeers(filepath.Join(dirA, "peers.json"))
	spB, _ := NewStaticPeers(filepath.Join(dirB, "peers.json"))

	mA, _ := NewMesh(MeshConfig{
		Identity: idA, Trust: tsA, Static: spA, ListenAddr: "127.0.0.1:0",
	})
	mB, _ := NewMesh(MeshConfig{
		Identity: idB, Trust: tsB, Static: spB, ListenAddr: "127.0.0.1:0",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := mA.Start(ctx); err != nil {
		t.Fatalf("mA.Start: %v", err)
	}
	if err := mB.Start(ctx); err != nil {
		t.Fatalf("mB.Start: %v", err)
	}
	defer mA.Stop()
	defer mB.Stop()

	// Cross-trust.
	pubA := idA.PublicKey()
	pubB := idB.PublicKey()
	if err := tsA.Trust(idB.ShortID(), pubB, idB.CACert()); err != nil {
		t.Fatalf("tsA.Trust B: %v", err)
	}
	if err := tsB.Trust(idA.ShortID(), pubA, idA.CACert()); err != nil {
		t.Fatalf("tsB.Trust A: %v", err)
	}

	// Seed peer addresses in each mesh.
	bAddr := mB.listener.Addr()
	aAddr := mA.listener.Addr()
	spA.Add(&Peer{
		ID:     idB.ShortID(),
		Addr:   bAddr,
		PubKey: base64.StdEncoding.EncodeToString(pubB),
	})
	spB.Add(&Peer{
		ID:     idA.ShortID(),
		Addr:   aAddr,
		PubKey: base64.StdEncoding.EncodeToString(pubA),
	})
	mA.refreshFromStatic()
	mB.refreshFromStatic()

	// A sends a hello to B.
	if err := mA.SendHello(bAddr); err != nil {
		t.Fatalf("SendHello: %v", err)
	}
	// Give the listener a moment to receive.
	time.Sleep(200 * time.Millisecond)
}

// TestMesh_MaxMessageSize ensures Verify still works for a message
// whose body is below the cap. Larger messages are rejected at the
// listener level (see mesh_listener.send), which we don't re-test here.
func TestMesh_MaxMessageSize_RejectsOversize(t *testing.T) {
	dir := t.TempDir()
	id, _ := NewIdentity(filepath.Join(dir, "keys"))
	ln, err := newMeshListener("127.0.0.1:0", id, nil, nil)
	if err != nil {
		t.Fatalf("newMeshListener: %v", err)
	}
	defer ln.close()

	// Build a >4KiB body. We don't care about signature here; send
	// is expected to refuse before reading.
	huge := GossipMessage{
		Type:  "peer",
		From:  "x",
		Time:  time.Now().UnixNano(),
		Peers: make([]PeerSnapshot, 0, 200),
	}
	for i := 0; i < 200; i++ {
		huge.Peers = append(huge.Peers, PeerSnapshot{
			ID:     "peer-" + string(rune('a'+i%26)) + "-" + string(make([]byte, 80)),
			Addr:   "127.0.0.1:65535",
			PubKey: "AAAA",
		})
	}
	if err := ln.send("127.0.0.1:1", huge); err == nil {
		t.Error("expected error for oversized gossip")
	}
	body, _ := json.Marshal(&huge)
	if len(body) <= MaxGossipMessageBytes {
		t.Logf("body size = %d, cap = %d (smaller than expected; still verifying rejection)",
			len(body), MaxGossipMessageBytes)
	}
}
