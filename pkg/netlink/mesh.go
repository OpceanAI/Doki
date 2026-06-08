package netlink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// GossipMessage is the wire format for inter-peer gossip. Every message
// is JSON, ed25519-signed, and capped at 4 KiB. Larger messages are
// truncated by the sender.
type GossipMessage struct {
	Type      string         `json:"type"` // "hello", "peer", "container"
	From      string         `json:"from"` // sender short id
	Time      int64          `json:"time"` // unix nanos
	Nonce     string         `json:"nonce,omitempty"`
	Peers     []PeerSnapshot `json:"peers,omitempty"`
	Container *RemoteContainer `json:"container,omitempty"`
	Signature string         `json:"sig"` // base64 ed25519 sig over canonical JSON
}

// PeerSnapshot is a compact, wire-friendly peer record. It is what
// gets gossiped; full Peer objects (with .Containers) live only in
// local memory.
type PeerSnapshot struct {
	ID     string `json:"id"`
	Addr   string `json:"addr"`
	PubKey string `json:"pubkey"`
}

// Verify checks that the message was signed by the holder of pub.
func (m *GossipMessage) Verify(pub []byte) error {
	body, err := canonicalJSON(m)
	if err != nil {
		return err
	}
	return verifyEd25519(pub, body, m.Signature)
}

// Sign signs the message in-place using priv. The signature is base64.
func (m *GossipMessage) Sign(priv []byte) error {
	body, err := canonicalJSON(m)
	if err != nil {
		return err
	}
	sig, err := signEd25519(priv, body)
	if err != nil {
		return err
	}
	m.Signature = sig
	return nil
}

// MaxGossipMessageBytes is the on-wire size cap.
const MaxGossipMessageBytes = 4 * 1024

// Mesh wires together identity, trust, discovery, and a TCP listener
// that exchanges gossip with peers. It is the single entry point for
// the daemon to start participating in a DokiLink-Lite mesh.
type Mesh struct {
	identity   *Identity
	trust      *TrustStore
	static     *StaticPeers
	mdns       *MDNSService
	listenAddr string
	logger     *slog.Logger

	mu        sync.RWMutex
	peers     map[string]*Peer      // id -> latest
	conns     map[string]chan GossipMessage
	started   bool
	gossipQ   chan GossipMessage
	listener  *meshListener
}

// MeshConfig configures a Mesh.
type MeshConfig struct {
	Identity   *Identity
	Trust      *TrustStore
	Static     *StaticPeers
	ListenAddr string // host:port to listen for gossip, e.g. ":7432"
	Logger     *slog.Logger
	// EnableMDNS requires the netlink_mdns build tag. With the default
	// build the mDNS service is a no-op stub.
	EnableMDNS bool
}

// NewMesh constructs a Mesh. The identity is required; everything else
// is optional (defaulted). The mesh does not listen until Start().
func NewMesh(cfg MeshConfig) (*Mesh, error) {
	if cfg.Identity == nil {
		return nil, errors.New("mesh: nil identity")
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":7432"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	m := &Mesh{
		identity:   cfg.Identity,
		trust:      cfg.Trust,
		static:     cfg.Static,
		listenAddr: cfg.ListenAddr,
		logger:     cfg.Logger,
		peers:      make(map[string]*Peer),
		conns:      make(map[string]chan GossipMessage),
		gossipQ:    make(chan GossipMessage, 64),
	}
	if cfg.EnableMDNS {
		_, port, _ := splitHostPort(cfg.ListenAddr)
		m.mdns = NewMDNSService(cfg.Identity, port, cfg.Logger)
	}
	// Seed peers from static config.
	if cfg.Static != nil {
		for _, p := range cfg.Static.List() {
			cp := *p
			m.peers[p.ID] = &cp
		}
	}
	return m, nil
}

// Peers returns a snapshot of currently known peers.
func (m *Mesh) Peers() []*Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Peer, 0, len(m.peers))
	for _, p := range m.peers {
		cp := *p
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Peer returns a single peer by ID.
func (m *Mesh) Peer(id string) *Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.peers[id]; ok {
		cp := *p
		return &cp
	}
	return nil
}

// Start opens the gossip listener and begins the gossip loop. The
// returned context is cancelled on Stop.
func (m *Mesh) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return errors.New("mesh: already started")
	}
	m.started = true
	m.mu.Unlock()

	ln, err := newMeshListener(m.listenAddr, m.identity, m.logger, m.onMessage)
	if err != nil {
		return err
	}
	m.listener = ln
	go ln.acceptLoop(ctx)

	if m.mdns != nil {
		if err := m.mdns.Start(ctx); err != nil {
			m.logger.Warn("doki-link: mdns start failed", "err", err)
		}
	}

	go m.gossipLoop(ctx)
	go m.staticRefreshLoop(ctx)
	return nil
}

// Stop closes the listener, mDNS, and stops the gossip loop.
func (m *Mesh) Stop() error {
	m.mu.Lock()
	m.started = false
	m.mu.Unlock()
	if m.listener != nil {
		m.listener.close()
	}
	if m.mdns != nil {
		m.mdns.Stop()
	}
	return nil
}

// AnnounceContainer publishes a container descriptor to all peers.
func (m *Mesh) AnnounceContainer(c RemoteContainer) error {
	msg := GossipMessage{
		Type:      "container",
		From:      m.identity.ShortID(),
		Time:      time.Now().UnixNano(),
		Container: &c,
	}
	if err := msg.Sign(m.identity.PrivateKey()); err != nil {
		return err
	}
	return m.broadcast(msg)
}

// SendHello sends a hello message to one peer, returning any error.
func (m *Mesh) SendHello(addr string) error {
	msg := GossipMessage{
		Type: "hello",
		From: m.identity.ShortID(),
		Time: time.Now().UnixNano(),
	}
	if err := msg.Sign(m.identity.PrivateKey()); err != nil {
		return err
	}
	return m.sendTo(addr, msg)
}

func (m *Mesh) broadcast(msg GossipMessage) error {
	m.mu.RLock()
	addrs := make([]string, 0, len(m.peers))
	for _, p := range m.peers {
		if p.Addr != "" {
			addrs = append(addrs, p.Addr)
		}
	}
	m.mu.RUnlock()
	var lastErr error
	for _, a := range addrs {
		if err := m.sendTo(a, msg); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (m *Mesh) sendTo(addr string, msg GossipMessage) error {
	if m.listener == nil {
		return errors.New("mesh: not started")
	}
	return m.listener.send(addr, msg)
}

func (m *Mesh) gossipLoop(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.tick()
		}
	}
}

func (m *Mesh) tick() {
	// Build a peer snapshot from current registry, then gossip.
	m.mu.RLock()
	snaps := make([]PeerSnapshot, 0, len(m.peers))
	for _, p := range m.peers {
		snaps = append(snaps, PeerSnapshot{ID: p.ID, Addr: p.Addr, PubKey: p.PubKey})
	}
	m.mu.RUnlock()
	if len(snaps) == 0 {
		return
	}
	msg := GossipMessage{
		Type:  "peer",
		From:  m.identity.ShortID(),
		Time:  time.Now().UnixNano(),
		Peers: snaps,
	}
	if err := msg.Sign(m.identity.PrivateKey()); err != nil {
		m.logger.Warn("doki-link: sign gossip", "err", err)
		return
	}
	if err := m.broadcast(msg); err != nil {
		m.logger.Debug("doki-link: gossip", "err", err)
	}
}

func (m *Mesh) staticRefreshLoop(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.refreshFromStatic()
		}
	}
}

func (m *Mesh) refreshFromStatic() {
	if m.static == nil {
		return
	}
	for _, p := range m.static.List() {
		cp := *p
		m.mu.Lock()
		m.peers[p.ID] = &cp
		m.mu.Unlock()
	}
}

// onMessage is called by the listener when a gossip message arrives.
// It verifies the signature, applies the changes locally, and merges
// the peer's view of the world.
func (m *Mesh) onMessage(msg GossipMessage) {
	if msg.From == "" || msg.From == m.identity.ShortID() {
		return
	}
	m.mu.RLock()
	p, ok := m.peers[msg.From]
	if !ok {
		m.mu.RUnlock()
		m.logger.Debug("doki-link: gossip from unknown peer", "id", msg.From)
		return
	}
	pub, err := p.pubKeyBytes()
	m.mu.RUnlock()
	if err != nil {
		m.logger.Warn("doki-link: bad pubkey on peer", "id", msg.From, "err", err)
		return
	}
	if err := msg.Verify(pub); err != nil {
		m.logger.Warn("doki-link: gossip signature invalid", "id", msg.From, "err", err)
		return
	}
	// Hold write lock for mutations
	m.mu.Lock()
	if p.LastSeen.IsZero() || time.Since(p.LastSeen) > 5*time.Minute {
		p.LastSeen = time.Now()
	}
	switch msg.Type {
	case "hello", "peer":
		m.mu.Unlock()
		m.mergePeers(msg.Peers)
	case "container":
		if msg.Container != nil {
			p.Containers = appendUniqueContainer(p.Containers, *msg.Container)
		}
		m.mu.Unlock()
	default:
		m.mu.Unlock()
	}
}

func (m *Mesh) mergePeers(snaps []PeerSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range snaps {
		if s.ID == m.identity.ShortID() {
			continue
		}
		if existing, ok := m.peers[s.ID]; ok {
			if existing.Addr == "" && s.Addr != "" {
				existing.Addr = s.Addr
			}
			if existing.PubKey == "" && s.PubKey != "" {
				existing.PubKey = s.PubKey
			}
			continue
		}
		m.peers[s.ID] = &Peer{
			ID:     s.ID,
			Name:   s.ID,
			Addr:   s.Addr,
			PubKey: s.PubKey,
		}
	}
}

func appendUniqueContainer(list []RemoteContainer, c RemoteContainer) []RemoteContainer {
	for _, x := range list {
		if x.ID == c.ID && x.Port == c.Port && x.Proto == c.Proto {
			return list
		}
	}
	return append(list, c)
}

func canonicalJSON(v interface{}) ([]byte, error) {
	// Marshal without the signature field by wrapping.
	switch msg := v.(type) {
	case *GossipMessage:
		copy := *msg
		copy.Signature = ""
		return json.Marshal(&copy)
	case GossipMessage:
		msg.Signature = ""
		return json.Marshal(&msg)
	}
	return nil, fmt.Errorf("canonicalJSON: unsupported type %T", v)
}

func splitHostPort(addr string) (host string, port int, err error) {
	if addr == "" {
		return "", 0, errors.New("empty addr")
	}
	if addr[0] == ':' {
		fmt.Sscanf(addr[1:], "%d", &port)
		return "", port, nil
	}
	var p int
	i := len(addr) - 1
	for i >= 0 && addr[i] != ':' {
		i--
	}
	if i < 0 {
		return addr, 0, nil
	}
	fmt.Sscanf(addr[i+1:], "%d", &p)
	return addr[:i], p, nil
}
