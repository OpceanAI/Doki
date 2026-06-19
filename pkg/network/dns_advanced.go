//go:build linux

package network

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type AdvancedDNS struct {
	srvRecords   []SRVRecord
	dnssec       *DNSSECValidator
	cache        *PersistentCache
	domains      *DomainResolver
}

type SRVRecord struct {
	Name     string `json:"name"`
	Priority uint16 `json:"priority"`
	Weight   uint16 `json:"weight"`
	Port     uint16 `json:"port"`
	Target   string `json:"target"`
	TTL      uint32 `json:"ttl"`
}

type DNSSECConfig struct {
	Enabled    bool     `json:"enabled"`
	Anchors    []string `json:"anchors"`
	Validate   bool     `json:"validate"`
}

type CacheConfig struct {
	Persistent  bool   `json:"persistent"`
	Path        string `json:"path"`
	MaxEntries  int    `json:"max_entries"`
	PositiveTTL string `json:"positive_ttl"`
	NegativeTTL string `json:"negative_ttl"`
}

type DomainRule struct {
	Pattern string `json:"pattern"`
	Target  string `json:"target"`
	TTL     uint32 `json:"ttl"`
}

type DNSSECValidator struct {
	enabled      bool
	trustAnchors []string
}

type PersistentCache struct {
	db         *sql.DB
	maxEntries int
	positiveTTL time.Duration
	negativeTTL time.Duration
	mu         sync.RWMutex
}

type DomainResolver struct {
	rules []DomainRule
	mu    sync.RWMutex
}

func NewAdvancedDNS() *AdvancedDNS {
	return &AdvancedDNS{}
}

func (a *AdvancedDNS) ConfigureSRV(records []SRVRecord) {
	a.srvRecords = records
}

func (a *AdvancedDNS) ConfigureDNSSEC(cfg DNSSECConfig) {
	if cfg.Enabled {
		a.dnssec = &DNSSECValidator{
			enabled:      true,
			trustAnchors: cfg.Anchors,
		}
	}
}

func (a *AdvancedDNS) ConfigureCache(cfg CacheConfig) error {
	if !cfg.Persistent || cfg.Path == "" {
		return nil
	}

	posTTL, _ := time.ParseDuration(cfg.PositiveTTL)
	if posTTL == 0 {
		posTTL = time.Hour
	}
	negTTL, _ := time.ParseDuration(cfg.NegativeTTL)
	if negTTL == 0 {
		negTTL = 5 * time.Minute
	}
	maxEntries := cfg.MaxEntries
	if maxEntries == 0 {
		maxEntries = 10000
	}

	a.cache = &PersistentCache{
		maxEntries:  maxEntries,
		positiveTTL: posTTL,
		negativeTTL: negTTL,
	}

	return a.cache.init(cfg.Path)
}

func (a *AdvancedDNS) ConfigureDomains(rules []DomainRule) {
	a.domains = &DomainResolver{rules: rules}
}

func (a *AdvancedDNS) HandleSRV(name string) []dns.RR {
	var records []dns.RR
	for _, srv := range a.srvRecords {
		if strings.EqualFold(srv.Name, name) {
			records = append(records, &dns.SRV{
				Hdr: dns.RR_Header{
					Name:   srv.Name,
					Rrtype: dns.TypeSRV,
					Class:  dns.ClassINET,
					Ttl:    srv.TTL,
				},
				Priority: srv.Priority,
				Weight:   srv.Weight,
				Port:     srv.Port,
				Target:   srv.Target,
			})
		}
	}
	return records
}

func (c *PersistentCache) init(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("open cache db: %w", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS cache (
			name TEXT NOT NULL,
			type INTEGER NOT NULL,
			answer BLOB,
			expiry INTEGER NOT NULL,
			created INTEGER NOT NULL,
			PRIMARY KEY (name, type)
		);
		CREATE INDEX IF NOT EXISTS idx_expiry ON cache(expiry);
	`)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("init cache schema: %w", err)
	}

	_, _ = db.Exec("DELETE FROM cache WHERE expiry < ?", time.Now().Unix())

	c.db = db
	return nil
}

func (c *PersistentCache) Get(name string, qtype uint16) (*dns.Msg, bool) {
	if c == nil || c.db == nil {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var answer []byte
	err := c.db.QueryRow(
		"SELECT answer FROM cache WHERE name = ? AND type = ? AND expiry > ?",
		name, qtype, time.Now().Unix(),
	).Scan(&answer)
	if err != nil {
		return nil, false
	}

	msg := new(dns.Msg)
	if err := msg.Unpack(answer); err != nil {
		return nil, false
	}
	return msg, true
}

func (c *PersistentCache) Put(name string, qtype uint16, msg *dns.Msg, ttl time.Duration) {
	if c == nil || c.db == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	answer, err := msg.Pack()
	if err != nil {
		return
	}

	expiry := time.Now().Add(ttl).Unix()
	_, _ = c.db.Exec(
		"INSERT OR REPLACE INTO cache (name, type, answer, expiry, created) VALUES (?, ?, ?, ?, ?)",
		name, qtype, answer, expiry, time.Now().Unix(),
	)

	_, _ = c.db.Exec("DELETE FROM cache WHERE rowid NOT IN (SELECT rowid FROM cache ORDER BY created DESC LIMIT ?)", c.maxEntries)
}

func (c *PersistentCache) Close() {
	if c != nil && c.db != nil {
		_ = c.db.Close()
	}
}

func (r *DomainResolver) Resolve(name string, qtype uint16) ([]net.IP, bool) {
	if r == nil {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if matchPattern(rule.Pattern, name) {
			ips, err := net.LookupIP(rule.Target)
			if err != nil {
				return nil, false
			}
			return ips, true
		}
	}
	return nil, false
}

func matchPattern(pattern, name string) bool {
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:]
		return strings.HasSuffix(name, suffix)
	}
	return strings.EqualFold(pattern, name)
}
