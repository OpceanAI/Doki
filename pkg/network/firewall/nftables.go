// nftables backend implementation for the firewall Manager.
package firewall

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// nftManager implements Manager against the nft CLI.
//
// Layout (table inet doki):
//
//	chain prerouting { type nat hook prerouting priority -100; policy accept; }
//	chain output     { type nat hook output     priority -100; policy accept; }
//	chain postrouting{ type nat hook postrouting priority  100; policy accept; }
//
// Rules carry a comment with the container ID and host port so
// "list" and "remove" are fast.
type nftManager struct {
	mu       sync.Mutex
	table    string
	family   string
	ipv6     bool
	ip6Table string // separate inet6 table for IPv6 rules
}

func newNFTables(ctx context.Context, opts Options) (Manager, error) {
	m := &nftManager{
		table:    opts.TableName,
		family:   opts.Family,
		ipv6:     opts.IPv6,
		ip6Table: opts.TableName + "_6",
	}
	if err := m.bootstrap(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *nftManager) Backend() Backend { return BackendNFTables }

// bootstrap creates the table + chains idempotently.
func (m *nftManager) bootstrap(ctx context.Context) error {
	specs := [][]string{
		{"add", "table", m.family, m.table},
		{"add", "chain", m.family, m.table, "prerouting",
			"{", "type", "nat", "hook", "prerouting", "priority", "-100", ";", "policy", "accept", ";", "}"},
		{"add", "chain", m.family, m.table, "output",
			"{", "type", "nat", "hook", "output", "priority", "-100", ";", "policy", "accept", ";", "}"},
		{"add", "chain", m.family, m.table, "postrouting",
			"{", "type", "nat", "hook", "postrouting", "priority", "100", ";", "policy", "accept", ";", "}"},
	}
	for _, args := range specs {
		_, _, err := execCmd(ctx, "nft", args...)
		if err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("nft bootstrap %v: %w", args, err)
		}
	}
	if m.ipv6 {
		ipv6Specs := [][]string{
			{"add", "table", "ip6", m.ip6Table},
			{"add", "chain", "ip6", m.ip6Table, "prerouting",
				"{", "type", "nat", "hook", "prerouting", "priority", "-100", ";", "policy", "accept", ";", "}"},
			{"add", "chain", "ip6", m.ip6Table, "output",
				"{", "type", "nat", "hook", "output", "priority", "-100", ";", "policy", "accept", ";", "}"},
			{"add", "chain", "ip6", m.ip6Table, "postrouting",
				"{", "type", "nat", "hook", "postrouting", "priority", "100", ";", "policy", "accept", ";", "}"},
		}
		for _, args := range ipv6Specs {
			_, _, err := execCmd(ctx, "nft", args...)
			if err != nil && !isAlreadyExists(err) {
				return fmt.Errorf("nft bootstrap ipv6 %v: %w", args, err)
			}
		}
	}
	return nil
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "File exists")
}

func (m *nftManager) Apply(ctx context.Context, r Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	family, table := m.family, m.table
	if isV6(r.HostIP) || isV6(r.ContainerIP) {
		if !m.ipv6 {
			return fmt.Errorf("ipv6 not enabled in firewall manager")
		}
		family, table = "ip6", m.ip6Table
	}

	proto := r.Protocol
	if proto == "" {
		proto = "tcp"
	}

	dnatExpr := fmt.Sprintf("dnat to %s:%d", r.ContainerIP, r.ContainerPort)
	args := []string{"add", "rule", family, table, "prerouting"}
	// When a specific host IP is published, match it; when it is empty the
	// publish applies to any destination, so the `ip daddr` clause must be
	// omitted entirely — emitting `ip daddr ""` produced a malformed rule that
	// nft rejected (or that matched nothing).
	if r.HostIP != "" {
		args = append(args, "ip", "daddr", r.HostIP)
	}
	args = append(args,
		protoL4(proto), "dport", fmt.Sprint(r.HostPort),
		dnatExpr,
		"comment", fmt.Sprintf("doki:%s", ruleKey(r)),
	)
	_, _, err := execCmd(ctx, "nft", args...)
	if err != nil {
		return fmt.Errorf("nft dnat: %w", err)
	}

	// Also add an output-chain rule so containers publishing on the
	// docker host's own IP get dnatted (for localhost-published ports).
	if r.HostIP == "" || r.HostIP == "127.0.0.1" || r.HostIP == "::1" {
		args2 := []string{
			"add", "rule", family, table, "output",
			"ip", "daddr", "127.0.0.1", protoL4(proto), "dport", fmt.Sprint(r.HostPort),
			dnatExpr,
			"comment", fmt.Sprintf("doki:%s", ruleKey(r)),
		}
		_, _, err := execCmd(ctx, "nft", args2...)
		if err != nil {
			return fmt.Errorf("nft dnat output: %w", err)
		}
	}

	// Masquerade for return traffic.
	masqArgs := []string{
		"add", "rule", family, table, "postrouting",
		"ip", "daddr", r.ContainerIP, protoL4(proto), "dport", fmt.Sprint(r.ContainerPort),
		"masquerade",
		"comment", fmt.Sprintf("doki:%s", ruleKey(r)),
	}
	_, _, err = execCmd(ctx, "nft", masqArgs...)
	if err != nil {
		return fmt.Errorf("nft masq: %w", err)
	}

	return nil
}

func protoL4(proto string) string {
	switch strings.ToLower(proto) {
	case "udp":
		return "udp"
	case "sctp":
		return "sctp"
	default:
		return "tcp"
	}
}

func isV6(ip string) bool {
	return strings.Contains(ip, ":")
}

func (m *nftManager) Remove(ctx context.Context, r Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// nft does not support deleting by comment, so we list rules in
	// handle order and find the matching handles.
	family, table := m.family, m.table
	if isV6(r.HostIP) || isV6(r.ContainerIP) {
		if !m.ipv6 {
			return nil
		}
		family, table = "ip6", m.ip6Table
	}
	return m.removeByComment(ctx, family, table, r)
}

func (m *nftManager) removeByComment(ctx context.Context, family, table string, r Rule) error {
	for _, chain := range []string{"prerouting", "output", "postrouting"} {
		// # @nft: handle=N is not part of stable output, so we use
		// `-a` flag to print handles and grep for our comment.
		out, _, err := execCmd(ctx, "nft", "-a", "list", "chain", family, table, chain)
		if err != nil {
			continue
		}
		comment := fmt.Sprintf("doki:%s", ruleKey(r))
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.Contains(line, comment) {
				continue
			}
			idx := strings.Index(line, "handle")
			if idx < 0 {
				continue
			}
			handle := strings.TrimSpace(line[idx+len("handle"):])
			fields := strings.Fields(handle)
			if len(fields) == 0 {
				continue
			}
			h := fields[0]
			_, _, err := execCmd(ctx, "nft", "delete", "rule", family, table, chain, "handle", h)
			if err != nil && !isAlreadyExists(err) {
				return fmt.Errorf("nft delete handle %s: %w", h, err)
			}
		}
	}
	return nil
}

func (m *nftManager) List(ctx context.Context) ([]Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Rule
	for _, ft := range []struct{ family, table string }{
		{m.family, m.table},
		{"ip6", m.ip6Table},
	} {
		if ft.family == "ip6" && !m.ipv6 {
			continue
		}
		for _, chain := range []string{"prerouting", "output", "postrouting"} {
			buf, _, err := execCmd(ctx, "nft", "list", "chain", ft.family, ft.table, chain)
			if err != nil {
				continue
			}
			// We do not try to fully parse the rule grammar; instead
			// we extract "comment" lines and re-derive the Rule via
			// the ruleKey() that was embedded. If parsing fails, we
			// skip — listing is best-effort.
			for _, line := range strings.Split(string(buf), "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "comment") {
					continue
				}
				idx := strings.Index(line, `"doki:`)
				if idx < 0 {
					continue
				}
				end := strings.Index(line[idx+7:], `"`)
				if end < 0 {
					continue
				}
				out = append(out, Rule{ContainerID: line[idx+7 : idx+7+end]})
			}
		}
	}
	return out, nil
}

func (m *nftManager) Reload(ctx context.Context) error {
	// Re-apply bootstrap (idempotent). This is the "firewall-reload"
	// behavior of netavark: ensure the table still exists after the
	// host firewall is flushed.
	return m.bootstrap(ctx)
}

func (m *nftManager) Close() error {
	return nil
}
