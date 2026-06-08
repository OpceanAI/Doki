package netlink

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StaticPeersConfig is the on-disk representation of manually
// configured peers. The file is $DOKI_ROOT/mesh/peers.json and can be
// edited by hand or by `doki link add <name> <addr> --pub <b64>`.
type StaticPeersConfig struct {
	Peers []*Peer `json:"peers"`
}

// StaticPeers is an in-memory view of $DOKI_ROOT/mesh/peers.json with
// safe concurrent access and atomic file writes.
type StaticPeers struct {
	path string
	mu   sync.RWMutex
	peers map[string]*Peer
}

// NewStaticPeers loads (or creates) the static peers file at path.
func NewStaticPeers(path string) (*StaticPeers, error) {
	if path == "" {
		return nil, errors.New("static peers: empty path")
	}
	sp := &StaticPeers{
		path:  path,
		peers: make(map[string]*Peer),
	}
	if err := sp.load(); err != nil {
		return nil, err
	}
	return sp, nil
}

func (sp *StaticPeers) load() error {
	data, err := os.ReadFile(sp.path)
	if err != nil {
		if os.IsNotExist(err) {
			return sp.save()
		}
		return err
	}
	cfg := StaticPeersConfig{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("static peers: %s: %w", sp.path, err)
	}
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for _, p := range cfg.Peers {
		if p == nil || p.ID == "" {
			continue
		}
		sp.peers[p.ID] = p
	}
	return nil
}

func (sp *StaticPeers) save() error {
	if err := os.MkdirAll(filepath.Dir(sp.path), 0755); err != nil {
		return err
	}
	sp.mu.RLock()
	out := StaticPeersConfig{Peers: make([]*Peer, 0, len(sp.peers))}
	for _, p := range sp.peers {
		out.Peers = append(out.Peers, p)
	}
	sp.mu.RUnlock()
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return err
	}
	tmp := sp.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, sp.path)
}

// List returns a copy of the static peer list.
func (sp *StaticPeers) List() []*Peer {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	out := make([]*Peer, 0, len(sp.peers))
	for _, p := range sp.peers {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

// Get returns a single peer by ID, or nil.
func (sp *StaticPeers) Get(id string) *Peer {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	if p, ok := sp.peers[id]; ok {
		cp := *p
		return &cp
	}
	return nil
}

// Add inserts or updates a peer, normalising fields, and persists the
// file.
func (sp *StaticPeers) Add(p *Peer) error {
	if p == nil {
		return errors.New("static peers: nil peer")
	}
	if p.ID == "" {
		return errors.New("static peers: empty id")
	}
	if p.PubKey == "" {
		return errors.New("static peers: empty pubkey")
	}
	if p.Addr == "" {
		return errors.New("static peers: empty addr")
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	sp.mu.Lock()
	if existing, ok := sp.peers[p.ID]; ok {
		if existing.PubKey != p.PubKey {
			sp.mu.Unlock()
			return fmt.Errorf("static peers: id %q already exists with a different pubkey", p.ID)
		}
		existing.Addr = p.Addr
		existing.Name = p.Name
		if p.CACert != "" {
			existing.CACert = p.CACert
		}
	} else {
		sp.peers[p.ID] = p
	}
	sp.mu.Unlock()
	return sp.save()
}

// Remove deletes a peer by ID and persists the file.
func (sp *StaticPeers) Remove(id string) error {
	sp.mu.Lock()
	delete(sp.peers, id)
	sp.mu.Unlock()
	return sp.save()
}

// Touch updates LastSeen to time.Now() for the given peer ID.
func (sp *StaticPeers) Touch(id string) {
	sp.mu.Lock()
	if p, ok := sp.peers[id]; ok {
		p.LastSeen = time.Now()
	}
	sp.mu.Unlock()
}
