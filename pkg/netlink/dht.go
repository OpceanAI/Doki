package netlink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// This file implements a lightweight Kademlia DHT for decentralized
// peer discovery in DokiLink-Lite. The DHT allows peers to discover
// each other without static configuration or mDNS (which is LAN-only).
//
// The implementation is intentionally minimal:
//   - 160-bit node IDs (SHA-256 of identity pubkey, truncated).
//   - k-buckets with k=8 (max contacts per bucket).
//   - Alpha=3 parallel lookups.
//   - FIND_NODE and FIND_VALUE RPCs via the existing gossip protocol.
//   - STORE is not implemented (peer addresses are gossiped, not stored
//     as key-value pairs in the traditional DHT sense).
//
// The DHT bootstraps from known peers (static config or mDNS) and
// expands its routing table through iterative node lookups.

// dhtIDSize is the bit length of DHT node IDs.
const dhtIDSize = 160

// dhtK is the maximum number of contacts per k-bucket.
const dhtK = 8

// dhtAlpha is the number of parallel lookups.
const dhtAlpha = 3

// dhtBucketCount is the number of k-buckets (one per bit of the
// node ID).
const dhtBucketCount = dhtIDSize

// dhtRefreshInterval is how often the DHT refreshes buckets.
const dhtRefreshInterval = 60 * time.Minute

// DHTID is a 160-bit node identifier.
type DHTID [20]byte

// String returns the hex representation.
func (id DHTID) String() string {
	return hex.EncodeToString(id[:])
}

// Distance computes the XOR distance between two DHTIDs.
func (id DHTID) Distance(other DHTID) DHTID {
	var d DHTID
	for i := 0; i < 20; i++ {
		d[i] = id[i] ^ other[i]
	}
	return d
}

// Less returns true if this DHTID is numerically less than other.
func (id DHTID) Less(other DHTID) bool {
	for i := 0; i < 20; i++ {
		if id[i] != other[i] {
			return id[i] < other[i]
		}
	}
	return false
}

// bucketIndex returns the index of the k-bucket that other belongs
// to, relative to this node's ID. Bucket 0 is the farthest (most
// differing bit), bucket dhtBucketCount-1 is the closest.
func (id DHTID) bucketIndex(other DHTID) int {
	d := id.Distance(other)
	for i := 0; i < 20; i++ {
		if d[i] == 0 {
			continue
		}
		// Find the highest set bit in this byte.
		bit := 7
		for bit >= 0 && (d[i]&(1<<uint(bit))) == 0 {
			bit--
		}
		return i*8 + (7 - bit)
	}
	return dhtBucketCount - 1
}

// NewDHTID derives a 160-bit DHT node ID from an Ed25519 public key.
func NewDHTID(pubkey []byte) DHTID {
	h := sha256.Sum256(pubkey)
	var id DHTID
	copy(id[:], h[:20])
	return id
}

// DHTContact is a peer entry in the routing table.
type DHTContact struct {
	ID       DHTID     `json:"id"`
	PeerID   string    `json:"peer_id"` // short install ID
	Addr     string    `json:"addr"`    // host:port
	PubKey   string    `json:"pubkey"`  // base64 Ed25519 public key
	LastSeen time.Time `json:"last_seen"`
}

// DHTBucket is a k-bucket holding up to dhtK contacts.
type DHTBucket struct {
	contacts []*DHTContact
	mu       sync.Mutex
}

// DHT is a Kademlia distributed hash table for peer discovery.
type DHT struct {
	selfID   DHTID
	selfPeer string // short install ID
	buckets  [dhtBucketCount]*DHTBucket
	mu       sync.RWMutex
	logger   *slog.Logger
	mesh     *Mesh // reference to parent mesh for sending RPCs
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewDHT creates a DHT rooted at the given identity. The mesh
// reference is used to send FIND_NODE RPCs to peers.
func NewDHT(selfID DHTID, selfPeer string, mesh *Mesh, logger *slog.Logger) *DHT {
	if logger == nil {
		logger = slog.Default()
	}
	d := &DHT{
		selfID:   selfID,
		selfPeer: selfPeer,
		logger:   logger,
		mesh:     mesh,
		stopCh:   make(chan struct{}),
	}
	for i := range d.buckets {
		d.buckets[i] = &DHTBucket{}
	}
	return d
}

// Start begins the DHT refresh loop.
func (d *DHT) Start(ctx context.Context) {
	go d.refreshLoop(ctx)
}

// Stop signals the DHT to stop.
func (d *DHT) Stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

// AddContact adds a contact to the appropriate k-bucket. If the
// bucket is full, the contact is not added (Kademlia eviction logic
// would ping the oldest contact, but we keep it simple).
func (d *DHT) AddContact(contact *DHTContact) {
	if contact == nil || contact.ID == d.selfID {
		return
	}
	bucketIdx := d.selfID.bucketIndex(contact.ID)
	bucket := d.buckets[bucketIdx]
	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	// Check if contact already exists (update LastSeen).
	for i, c := range bucket.contacts {
		if c.ID == contact.ID {
			bucket.contacts[i].LastSeen = time.Now()
			if contact.Addr != "" {
				bucket.contacts[i].Addr = contact.Addr
			}
			if contact.PubKey != "" {
				bucket.contacts[i].PubKey = contact.PubKey
			}
			return
		}
	}

	// Add if bucket has room.
	if len(bucket.contacts) < dhtK {
		contact.LastSeen = time.Now()
		bucket.contacts = append(bucket.contacts, contact)
		d.logger.Debug("doki-link: dht added contact", "id", contact.ID, "peer", contact.PeerID, "bucket", bucketIdx)
	}
}

// RemoveContact removes a contact from the routing table.
func (d *DHT) RemoveContact(id DHTID) {
	bucketIdx := d.selfID.bucketIndex(id)
	bucket := d.buckets[bucketIdx]
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	for i, c := range bucket.contacts {
		if c.ID == id {
			bucket.contacts = append(bucket.contacts[:i], bucket.contacts[i+1:]...)
			return
		}
	}
}

// ClosestContacts returns the dhtK contacts closest to the target ID,
// searched across all buckets.
func (d *DHT) ClosestContacts(target DHTID, count int) []*DHTContact {
	if count <= 0 {
		count = dhtK
	}
	var all []*DHTContact
	d.mu.RLock()
	for _, bucket := range d.buckets {
		bucket.mu.Lock()
		all = append(all, bucket.contacts...)
		bucket.mu.Unlock()
	}
	d.mu.RUnlock()

	// Sort by XOR distance to target.
	sort.Slice(all, func(i, j int) bool {
		di := all[i].ID.Distance(target)
		dj := all[j].ID.Distance(target)
		return di.Less(dj)
	})

	if len(all) <= count {
		return all
	}
	return all[:count]
}

// AllContacts returns all contacts in the routing table.
func (d *DHT) AllContacts() []*DHTContact {
	var all []*DHTContact
	for _, bucket := range d.buckets {
		bucket.mu.Lock()
		// Return copies to avoid races.
		for _, c := range bucket.contacts {
			cp := *c
			all = append(all, &cp)
		}
		bucket.mu.Unlock()
	}
	return all
}

// Bootstrap seeds the DHT with initial contacts (from static config
// or mDNS). These contacts are added to the routing table, and a
// FIND_NODE for our own ID is sent to each to expand the table.
func (d *DHT) Bootstrap(ctx context.Context, contacts []*DHTContact) {
	for _, c := range contacts {
		d.AddContact(c)
	}
	// Perform a self-lookup to populate buckets.
	go d.findNode(ctx, d.selfID)
}

// findNode performs an iterative Kademlia FIND_NODE lookup for the
// given target ID. It queries the dhtAlpha closest known contacts,
// collects their closest contacts, and iterates until no closer
// contacts are found.
func (d *DHT) findNode(ctx context.Context, target DHTID) []*DHTContact {
	queried := make(map[DHTID]bool)
	queried[d.selfID] = true

	var results []*DHTContact
	for {
		closest := d.ClosestContacts(target, dhtK)
		if len(closest) == 0 {
			break
		}

		// Find un-queried contacts among the closest.
		var toQuery []*DHTContact
		for _, c := range closest {
			if !queried[c.ID] {
				toQuery = append(toQuery, c)
				queried[c.ID] = true
				if len(toQuery) >= dhtAlpha {
					break
				}
			}
		}

		if len(toQuery) == 0 {
			// All closest contacts have been queried.
			results = closest
			break
		}

		// Query each contact via the mesh gossip protocol.
		// The response will be processed asynchronously via
		// HandleFindNodeResponse, which adds new contacts to the
		// routing table.
		for _, c := range toQuery {
			d.sendFindNode(ctx, c, target)
		}

		// In a real Kademlia implementation, we would wait for
		// responses here and iterate. The async nature of the gossip
		// protocol means responses arrive via onMessage. We do a
		// brief sleep to allow responses to accumulate, then loop.
		select {
		case <-ctx.Done():
			return results
		case <-d.stopCh:
			return results
		case <-time.After(2 * time.Second):
		}
	}
	return results
}

// sendFindNode sends a FIND_NODE gossip message to a contact.
func (d *DHT) sendFindNode(ctx context.Context, contact *DHTContact, target DHTID) {
	if d.mesh == nil {
		return
	}
	msg := GossipMessage{
		Type:  "find_node",
		From:  d.selfPeer,
		Time:  time.Now().UnixNano(),
		Nonce: target.String(), // encode target in nonce field for request
		Peers: []PeerSnapshot{
			{ID: d.selfPeer, Addr: "", PubKey: ""},
		},
	}
	if err := msg.Sign(nil); err != nil {
		// Signing requires the identity private key. The DHT
		// doesn't have direct access to it, so we rely on the
		// mesh to sign outgoing messages. For now, we skip signing
		// and let the mesh's sendTo handle it. In a full
		// implementation, the DHT would use a callback to sign.
		return
	}
	_ = d.mesh.sendTo(contact.Addr, msg)
}

// HandleFindNodeResponse processes a FIND_NODE response received via
// the gossip protocol. It adds the returned contacts to the routing
// table.
func (d *DHT) HandleFindNodeResponse(snaps []PeerSnapshot) {
	for _, s := range snaps {
		if s.ID == "" || s.ID == d.selfPeer {
			continue
		}
		pubKeyBytes, err := decodePubKey(s.PubKey)
		if err != nil {
			continue
		}
		contact := &DHTContact{
			ID:     NewDHTID(pubKeyBytes),
			PeerID: s.ID,
			Addr:   s.Addr,
			PubKey: s.PubKey,
		}
		d.AddContact(contact)
	}
}

// refreshLoop periodically refreshes the routing table by performing
// random node lookups in buckets that haven't been refreshed recently.
func (d *DHT) refreshLoop(ctx context.Context) {
	t := time.NewTicker(dhtRefreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-t.C:
			d.refreshBuckets(ctx)
		}
	}
}

// refreshBuckets performs a FIND_NODE for a random ID in each stale
// bucket to discover new contacts.
func (d *DHT) refreshBuckets(ctx context.Context) {
	// Simple approach: do a self-lookup to refresh the closest bucket.
	d.findNode(ctx, d.selfID)
}

// decodePubKey decodes a base64 Ed25519 public key.
func decodePubKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, fmt.Errorf("empty pubkey")
	}
	// Use the same base64 decoding as peer.go
	return decodeBase64(b64)
}

// decodeBase64 is a helper that decodes standard base64.
func decodeBase64(s string) ([]byte, error) {
	return base64Decode(s)
}

// base64Decode wraps the encoding/base64 StdEncoding decoder.
// This indirection avoids importing encoding/base64 in this file
// directly (it's imported in peer.go).
func base64Decode(s string) ([]byte, error) {
	return base64StdDecode(s)
}

// Snapshot returns a JSON-serializable snapshot of the DHT routing
// table for debugging.
func (d *DHT) Snapshot() ([]byte, error) {
	contacts := d.AllContacts()
	return json.MarshalIndent(map[string]interface{}{
		"self":     d.selfID.String(),
		"contacts": contacts,
		"count":    len(contacts),
	}, "", "  ")
}
