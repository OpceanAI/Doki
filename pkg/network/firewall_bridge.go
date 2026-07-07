// Package network provides a unified firewall interface via the
// `firewall` sub-package. This file re-exports the new Manager type
// for callers that want auto-detection between nftables and iptables.
package network

import (
	"context"

	"github.com/OpceanAI/Doki/pkg/network/firewall"
)

// NewAutoFirewallManager returns a firewall.Manager that auto-detects
// nftables first, then iptables. It is the recommended way to construct
// the firewall in 2026: nftables is preferred on every modern distro
// (Fedora 40+, Ubuntu 24.04+, RHEL 10+, openSUSE Leap 16+).
func NewAutoFirewallManager() (firewall.Manager, error) {
	return firewall.Open(context.Background(), firewall.Options{
		TableName: "doki",
		IPv6:      true,
	})
}

// NewFirewallManagerWithBackend returns a manager that forces a specific
// backend. Pass firewall.BackendNFTables or firewall.BackendIPTables.
func NewFirewallManagerWithBackend(b firewall.Backend) (firewall.Manager, error) {
	return firewall.Open(context.Background(), firewall.Options{
		Preferred: b,
		TableName: "doki",
		IPv6:      true,
	})
}
