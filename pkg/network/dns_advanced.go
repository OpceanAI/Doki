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

// AdvancedDNS provides SRV resolution, DNSSEC validation, persistent caching, and domain rules.
type AdvancedDNS struct {
	srvRecords []SRVRecord
	dnssec     *DNSSECValidator
	cache      *PersistentCache
	domains    *DomainResolver
}

// SRVRecord represents a DNS SRV record.
type SRVRecord struct {
	Name     string `json:"name"`
	Priority uint16 `json:"priority"`
	Weight   uint16 `json:"weight"`
	Port     uint16 `json:"port"`
	Target   string `json:"target"`
	TTL      uint32 `json:"ttl"`
}

// DNSSECConfig holds DNSSEC validation settings.
type DNSSECConfig struct {
	Enabled  bool     `json:"enabled"`
	Anchors  []string `json:"anchors"`
	Validate bool     `json:"validate"`
}

// CacheConfig holds persistent DNS cache settings.
type CacheConfig struct {
	Persistent  bool   `json:"persistent"`
	Path        string `json:"path"`
	MaxEntries  int    `json:"max_entries"`
	PositiveTTL string `json:"positive_ttl"`
	NegativeTTL string `json:"negative_ttl"`
}

// DomainRule defines a pattern-based domain resolution rule.
type DomainRule struct {
	Pattern string `json:"pattern"`
	Target  string `json:"target"`
	TTL     uint32 `json:"ttl"`
}

// DNSSECValidator validates DNS responses using DNSSEC trust anchors.
type DNSSECValidator struct {
	enabled      bool
	trustAnchors []string
}

// PersistentCache stores DNS responses in a SQLite database.
type PersistentCache struct {
	db          *sql.DB
	maxEntries  int
	positiveTTL time.Duration
	negativeTTL time.Duration
	mu          sync.RWMutex
}

// DomainResolver resolves domain names using pattern-based rules.
type DomainResolver struct {
	rules []DomainRule
	mu    sync.RWMutex
}

// NewAdvancedDNS creates a new AdvancedDNS instance.
func NewAdvancedDNS() *AdvancedDNS {
	return &AdvancedDNS{}
}

// ConfigureSRV sets the SRV records for the advanced DNS resolver.
func (a *AdvancedDNS) ConfigureSRV(records []SRVRecord) {
	a.srvRecords = records
}

// ConfigureDNSSEC enables DNSSEC validation with the given configuration.
func (a *AdvancedDNS) ConfigureDNSSEC(cfg DNSSECConfig) {
	if cfg.Enabled {
		a.dnssec = &DNSSECValidator{
			enabled:      true,
			trustAnchors: cfg.Anchors,
		}
	}
}

// ConfigureCache initializes the persistent DNS cache with the given configuration.
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

// ConfigureDomains sets domain resolution rules.
func (a *AdvancedDNS) ConfigureDomains(rules []DomainRule) {
	a.domains = &DomainResolver{rules: rules}
}

// HandleSRV returns matching SRV DNS resource records for a given name.
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

// Get retrieves a cached DNS response for a name and query type.
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

// Put stores a DNS response in the persistent cache.
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

// Close releases the database connection held by the cache.
func (c *PersistentCache) Close() {
	if c != nil && c.db != nil {
		_ = c.db.Close()
	}
}

// Resolve resolves a domain name using configured domain rules.
func (r *DomainResolver) Resolve(name string, _ uint16) ([]net.IP, bool) {
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
