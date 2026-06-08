package netlink

import "time"

// MDNSEntry describes a peer found via mDNS. The struct lives in a
// build-tag-neutral file so that the stub and full implementations
// share the same return type.
type MDNSEntry struct {
	ID     string    `json:"id"`
	Addr   string    `json:"addr"`
	Port   int       `json:"port"`
	PubKey string    `json:"pubkey"`
	Source string    `json:"source"`
	Seen   time.Time `json:"seen"`
}
