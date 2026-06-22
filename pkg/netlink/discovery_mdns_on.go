//go:build netlink_mdns

package netlink

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/hashicorp/mdns"
)

// mdnsEntryExpiry is the time-to-live for mDNS entries. Entries that
// have not been refreshed (re-announced) within this window are
// evicted by the cleanup loop. This keeps the peer list fresh and
// removes peers that have left the LAN.
const mdnsEntryExpiry = 90 * time.Second

// mdnsCleanupInterval is how often the cleanup loop runs to evict
// expired entries.
const mdnsCleanupInterval = 30 * time.Second

// MDNSService announces this Doki instance and discovers other Doki
// peers on the LAN. Only enabled with `-tags netlink_mdns`; the default
// build does not pull in github.com/hashicorp/mdns.
//
// TXT records encode the peer's short id and public key (base64), so
// that listeners can call TrustStore.Trust immediately on first contact.
//
// Entries expire after mdnsEntryExpiry (90 seconds) if not refreshed.
// A periodic cleanup loop evicts stale entries to keep the peer list
// accurate.
type MDNSService struct {
	id       *Identity
	port     int
	logger   *slog.Logger
	server   *mdns.Server
	entries  map[string]*MDNSEntry
	mu       sync.RWMutex
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewMDNSService returns a service bound to the given identity. The
// service does not start until Start() is called.
func NewMDNSService(identity *Identity, port int, logger *slog.Logger) *MDNSService {
	if logger == nil {
		logger = slog.Default()
	}
	return &MDNSService{
		id:      identity,
		port:    port,
		logger:  logger,
		entries: make(map[string]*MDNSEntry),
		stopCh:  make(chan struct{}),
	}
}

// serviceType is the well-known mDNS service name for DokiLink.
const serviceType = "_doki-link._tcp"

// Start begins announcing this peer and browsing for others.
func (m *MDNSService) Start(ctx context.Context) error {
	if m.id == nil {
		return errors.New("mdns: nil identity")
	}
	pub := m.id.idPub
	pubB64 := base64.StdEncoding.EncodeToString(pub)
	txt := []string{
		"id=" + m.id.installID,
		"pub=" + pubB64,
		"v=" + mdnsVersionTag(),
	}
	iface, err := mdns.NewMDNSService(
		"doki-"+m.id.installID,
		serviceType,
		"",
		"",
		m.port,
		nil,
		txt,
	)
	if err != nil {
		return err
	}
	srv, err := mdns.NewServer(&mdns.Config{Zone: iface})
	if err != nil {
		return err
	}
	m.server = srv

	// Browser: poll every 5s for new entries.
	go m.browseLoop(ctx)
	// Cleanup: evict expired entries every 30s.
	go m.cleanupLoop(ctx)
	return nil
}

// mdnsVersionTag returns the version string for mDNS TXT records.
func mdnsVersionTag() string {
	return common.DokiVersion
}

func (m *MDNSService) browseLoop(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-t.C:
			entriesCh := make(chan *mdns.ServiceEntry, 10)
			go func() {
				params := &mdns.QueryParam{
					Service:             serviceType,
					Domain:              "",
					Timeout:             3 * time.Second,
					Entries:             entriesCh,
					WantUnicastResponse: true,
				}
				mdns.Query(params)
			}()
			var entries []*mdns.ServiceEntry
			timeout := time.After(4 * time.Second)
		collect:
			for {
				select {
				case e, ok := <-entriesCh:
					if !ok {
						break collect
					}
					entries = append(entries, e)
				case <-timeout:
					break collect
				}
			}
			if len(entries) == 0 {
				continue
			}
			m.mu.Lock()
			for _, e := range entries {
				id, pub := parseTXT(e.InfoFields)
				if id == "" {
					continue
				}
				// Self-filter by install ID, not by port. Two
				// instances on the same host with different ports
				// would be incorrectly filtered by port, and a
				// different peer on the same port would be
				// incorrectly dropped.
				if id == m.id.installID {
					continue
				}
				host := ""
				if e.AddrV4 != nil {
					host = e.AddrV4.String()
				} else if e.AddrV6 != nil {
					host = e.AddrV6.String()
				}
				// Reject entries with no usable address.
				if host == "" {
					m.logger.Debug("doki-link: mdns entry with no address", "id", id)
					continue
				}
				// Refresh or create the entry, updating Seen time.
				m.entries[id] = &MDNSEntry{
					ID:     id,
					Addr:   host,
					Port:   e.Port,
					PubKey: pub,
					Source: "mdns",
					Seen:   time.Now(),
				}
			}
			m.mu.Unlock()
		}
	}
}

// cleanupLoop periodically evicts mDNS entries that have not been
// refreshed within mdnsEntryExpiry. This implements the 90-second
// expiry documented in the release notes.
func (m *MDNSService) cleanupLoop(ctx context.Context) {
	t := time.NewTicker(mdnsCleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-t.C:
			m.evictExpired()
		}
	}
}

// evictExpired removes entries where time.Since(Seen) > mdnsEntryExpiry.
func (m *MDNSService) evictExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for id, entry := range m.entries {
		if now.Sub(entry.Seen) > mdnsEntryExpiry {
			m.logger.Debug("doki-link: mdns entry expired", "id", id, "age", now.Sub(entry.Seen))
			delete(m.entries, id)
		}
	}
}

func parseTXT(fields []string) (id, pub string) {
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, "id="):
			id = f[3:]
		case strings.HasPrefix(f, "pub="):
			pub = f[4:]
		}
	}
	return
}

// Stop releases mDNS resources.
func (m *MDNSService) Stop() error {
	var err error
	m.stopOnce.Do(func() {
		close(m.stopCh)
		if m.server != nil {
			m.server.Shutdown()
		}
	})
	return err
}

// Entries returns a snapshot of all peers discovered via mDNS.
func (m *MDNSService) Entries() []MDNSEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MDNSEntry, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, *e)
	}
	return out
}
