//go:build !netlink_mdns

package netlink

import (
	"context"
	"log/slog"
	"sync"
)

// MDNSService is the build-tag stub for installations built without
// the netlink_mdns tag. All methods are no-ops.
type MDNSService struct{}

// NewMDNSService returns a disabled service. The full implementation is
// gated behind the `netlink_mdns` build tag and depends on
// github.com/hashicorp/mdns.
func NewMDNSService(_ *Identity, _ int, _ *slog.Logger) *MDNSService {
	return &MDNSService{}
}

// Start is a no-op when mDNS is disabled at build time.
func (m *MDNSService) Start(_ context.Context) error { return nil }

// Stop is a no-op when mDNS is disabled at build time.
func (m *MDNSService) Stop() error { return nil }

// Entries always returns an empty slice.
func (m *MDNSService) Entries() []MDNSEntry { return nil }

// _ guards the unused mutex import in stub mode.
var _ sync.Mutex
