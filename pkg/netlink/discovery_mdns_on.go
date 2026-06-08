//go:build netlink_mdns

package netlink

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

// MDNSService announces this Doki instance and discovers other Doki
// peers on the LAN. Only enabled with `-tags netlink_mdns`; the default
// build does not pull in github.com/hashicorp/mdns.
//
// TXT records encode the peer's short id and public key (base64), so
// that listeners can call TrustStore.Trust immediately on first contact.
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
		"v=0.9.3",
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
	return nil
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
				if e.Port == m.port {
					continue
				}
				id, pub := parseTXT(e.InfoFields)
				if id == "" {
					continue
				}
				key := id
				host := ""
				if e.AddrV4 != nil {
					host = e.AddrV4.String()
				} else if e.AddrV6 != nil {
					host = e.AddrV6.String()
				}
				m.entries[key] = &MDNSEntry{
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

func parseTXT(fields []string) (id, pub string) {
	for _, f := range fields {
		if len(f) > 3 && f[:3] == "id=" {
			id = f[3:]
		} else if len(f) > 4 && f[:4] == "pub=" {
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
