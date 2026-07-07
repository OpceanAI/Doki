// iptables backend implementation for the firewall Manager.
//
// This is the legacy path used by older distros (Debian 11, Ubuntu 22.04
// with iptables-legacy) and by hosts where nft is not available.
package firewall

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type iptManager struct {
	mu      sync.Mutex
	bin     string // "iptables" or "iptables-legacy"
	chain   string // "DOKI"
	ipv6    bool
	ip6Bin  string
	ip6Chain string
}

func newIPTables(ctx context.Context, opts Options) (Manager, error) {
	bin, err := pickIptables(ctx)
	if err != nil {
		return nil, err
	}
	m := &iptManager{
		bin:      bin,
		chain:    strings.ToUpper(opts.TableName),
		ipv6:     opts.IPv6,
		ip6Bin:   "ip6tables",
		ip6Chain: strings.ToUpper(opts.TableName) + "_6",
	}
	if err := m.bootstrap(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func pickIptables(ctx context.Context) (string, error) {
	for _, candidate := range []string{"iptables", "iptables-legacy", "iptables-nft"} {
		if _, err := exec.LookPath(candidate); err != nil {
			continue
		}
		out, _, err := execCmd(ctx, candidate, "--version")
		if err != nil {
			continue
		}
		// iptables-nft still works for our purposes; we use the same
		// iptables-style commands. Prefer legacy if both present.
		if strings.Contains(strings.ToLower(string(out)), "iptables") {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no usable iptables binary found")
}

func (m *iptManager) Backend() Backend { return BackendIPTables }

func (m *iptManager) bootstrap(ctx context.Context) error {
	for _, target := range m.targets() {
		if err := m.ensureChain(ctx, target.bin, target.chain); err != nil {
			return err
		}
	}
	return nil
}

type iptTarget struct {
	bin   string
	chain string
}

func (m *iptManager) targets() []iptTarget {
	out := []iptTarget{{bin: m.bin, chain: m.chain}}
	if m.ipv6 {
		// ip6tables may not be present; we only add it if available.
		if _, err := exec.LookPath(m.ip6Bin); err == nil {
			out = append(out, iptTarget{bin: m.ip6Bin, chain: m.ip6Chain})
		}
	}
	return out
}

func (m *iptManager) ensureChain(ctx context.Context, bin, chain string) error {
	// Create chain if missing.
	if _, _, err := execCmd(ctx, bin, "-t", "nat", "-L", chain); err != nil {
		_, _, err := execCmd(ctx, bin, "-t", "nat", "-N", chain)
		if err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("create chain %s: %w", chain, err)
		}
	}
	// Hook it into PREROUTING/OUTPUT/POSTROUTING if not already.
	for _, hook := range []string{"PREROUTING", "OUTPUT"} {
		_, _, _ = execCmd(ctx, bin, "-t", "nat", "-A", hook, "-j", chain)
	}
	return nil
}

func (m *iptManager) Apply(ctx context.Context, r Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bin, chain := m.bin, m.chain
	if isV6(r.HostIP) || isV6(r.ContainerIP) {
		if !m.ipv6 {
			return fmt.Errorf("ipv6 not enabled")
		}
		bin, chain = m.ip6Bin, m.ip6Chain
	}
	proto := r.Protocol
	if proto == "" {
		proto = "tcp"
	}
	daddr := r.HostIP
	if daddr == "" {
		daddr = "0.0.0.0/0"
	}
	dnatArgs := []string{
		"-t", "nat", "-A", chain,
		"-p", proto,
		"-d", daddr,
		"--dport", fmt.Sprint(r.HostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", r.ContainerIP, r.ContainerPort),
		"-m", "comment", "--comment", fmt.Sprintf("doki:%s", ruleKey(r)),
	}
	if _, _, err := execCmd(ctx, bin, dnatArgs...); err != nil {
		return fmt.Errorf("iptables dnat: %w", err)
	}
	masqArgs := []string{
		"-t", "nat", "-A", "POSTROUTING",
		"-d", r.ContainerIP,
		"-p", proto,
		"--dport", fmt.Sprint(r.ContainerPort),
		"-j", "MASQUERADE",
		"-m", "comment", "--comment", fmt.Sprintf("doki:%s", ruleKey(r)),
	}
	if _, _, err := execCmd(ctx, bin, masqArgs...); err != nil {
		return fmt.Errorf("iptables masq: %w", err)
	}
	return nil
}

func (m *iptManager) Remove(ctx context.Context, r Rule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bin, chain := m.bin, m.chain
	if isV6(r.HostIP) || isV6(r.ContainerIP) {
		if !m.ipv6 {
			return nil
		}
		bin, chain = m.ip6Bin, m.ip6Chain
	}
	// Delete DNAT rule.
	dnatArgs := []string{
		"-t", "nat", "-D", chain,
		"-p", r.Protocol,
		"-d", r.HostIP,
		"--dport", fmt.Sprint(r.HostPort),
		"-j", "DNAT",
		"--to-destination", fmt.Sprintf("%s:%d", r.ContainerIP, r.ContainerPort),
	}
	if r.HostIP == "" {
		dnatArgs[6] = "0.0.0.0/0"
	}
	_, _, _ = execCmd(ctx, bin, dnatArgs...) // idempotent; ignore err

	// Delete MASQUERADE rule.
	masqArgs := []string{
		"-t", "nat", "-D", "POSTROUTING",
		"-d", r.ContainerIP,
		"-p", r.Protocol,
		"--dport", fmt.Sprint(r.ContainerPort),
		"-j", "MASQUERADE",
	}
	_, _, _ = execCmd(ctx, bin, masqArgs...) // idempotent; ignore err
	return nil
}

func (m *iptManager) List(ctx context.Context) ([]Rule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Rule
	for _, t := range m.targets() {
		buf, _, err := execCmd(ctx, t.bin, "-t", "nat", "-S", t.chain)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(buf), "\n") {
			idx := strings.Index(line, "--comment")
			if idx < 0 {
				continue
			}
			c := strings.TrimSpace(line[idx+len("--comment"):])
			c = strings.Trim(c, `"`)
			if !strings.HasPrefix(c, "doki:") {
				continue
			}
			out = append(out, Rule{ContainerID: strings.TrimPrefix(c, "doki:")})
		}
	}
	return out, nil
}

func (m *iptManager) Reload(ctx context.Context) error {
	return m.bootstrap(ctx)
}

func (m *iptManager) Close() error {
	return nil
}
