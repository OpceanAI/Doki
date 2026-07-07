// Package firewall provides a backend abstraction for port-mapping
// and NAT rules that auto-detects nftables vs iptables at runtime.
//
// On Linux in 2026, modern distributions (Fedora 40+, Ubuntu 24.04+,
// RHEL 10, openSUSE Leap 16+) ship nftables only and no longer provide
// iptables-legacy. Older systems (Debian 11, Ubuntu 22.04) provide
// iptables-legacy. This shim probes both and picks the first one that
// is functional, with a hard preference for nftables when available.
package firewall

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// Backend identifies which underlying firewall tool is in use.
type Backend string

const (
	BackendNFTables   Backend = "nftables"
	BackendIPTables   Backend = "iptables"
	BackendUnavailable Backend = "unavailable"
)

// ErrNoBackend is returned when neither nft nor iptables is available.
var ErrNoBackend = errors.New("no firewall backend available (nft or iptables)")

// Rule represents a single port-forward rule: publish hostPort to
// containerIP:containerPort with a protocol.
type Rule struct {
	HostIP        string
	HostPort      uint16
	ContainerIP   string
	ContainerPort uint16
	Protocol      string // "tcp" or "udp"
	ContainerID   string // for tracking / removal
}

// Manager is the firewall backend abstraction. Implementations must be
// safe for concurrent use.
type Manager interface {
	Backend() Backend
	Apply(ctx context.Context, rule Rule) error
	Remove(ctx context.Context, rule Rule) error
	List(ctx context.Context) ([]Rule, error)
	Reload(ctx context.Context) error
	Close() error
}

// Detect returns the preferred backend for this host, probing nftables
// first, then iptables. It is safe to call before Open.
func Detect(ctx context.Context) Backend {
	if _, err := exec.LookPath("nft"); err == nil {
		cmd := exec.CommandContext(ctx, "nft", "--version")
		if out, err := cmd.CombinedOutput(); err == nil &&
			strings.Contains(string(out), "nftables") {
			return BackendNFTables
		}
	}
	for _, candidate := range []string{"iptables", "iptables-legacy"} {
		if _, err := exec.LookPath(candidate); err == nil {
			cmd := exec.CommandContext(ctx, candidate, "--version")
			if out, err := cmd.CombinedOutput(); err == nil &&
				strings.Contains(strings.ToLower(string(out)), "iptables") {
				return BackendIPTables
			}
		}
	}
	return BackendUnavailable
}

// Open selects the appropriate backend and returns a ready Manager.
// The decision is made once at Open time; the manager caches it.
//
// The chain name and table name are configurable so multiple Doki
// instances on the same host can coexist without stomping on each
// other's rules.
func Open(ctx context.Context, opts Options) (Manager, error) {
	opts = opts.normalized()
	backend := opts.Preferred
	if backend == "" {
		backend = Detect(ctx)
	}
	switch backend {
	case BackendNFTables:
		return newNFTables(ctx, opts)
	case BackendIPTables:
		return newIPTables(ctx, opts)
	}
	return nil, ErrNoBackend
}

// Options configures the firewall manager.
type Options struct {
	// Preferred forces a specific backend. Empty = auto-detect.
	Preferred Backend
	// TableName is the nftables table or iptables chain prefix.
	// Default: "doki".
	TableName string
	// Family is "inet" (nftables) or "ipv4"/"ipv6" (iptables).
	// Default: "inet" for nftables, "ipv4" for iptables.
	Family string
	// IPv6 enables the ipv6 backend as well. Default: true.
	IPv6 bool
}

func (o Options) normalized() Options {
	if o.TableName == "" {
		o.TableName = "doki"
	}
	if o.Family == "" {
		o.Family = "inet"
	}
	return o
}

// ─── shared helpers ─────────────────────────────────────────────────

// execCmd is a small helper that runs a command and returns
// (stdout, stderr, err). It is used by both backends.
func execCmd(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	stdoutBuf := &strings.Builder{}
	stderrBuf := &strings.Builder{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	err := cmd.Run()
	return []byte(stdoutBuf.String()), []byte(stderrBuf.String()), err
}

// ruleKey produces a stable identifier for a rule so we can match
// "remove this" with "add this" even if order changes.
func ruleKey(r Rule) string {
	return fmt.Sprintf("%s|%s:%d->%s:%d|%s",
		r.Protocol, r.HostIP, r.HostPort, r.ContainerIP, r.ContainerPort, r.ContainerID)
}

// onceInit ensures initialization happens once.
type onceInit struct {
	sync.Once
	err error
	mgr Manager
}
