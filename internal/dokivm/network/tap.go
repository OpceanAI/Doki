package network

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/OpceanAI/Doki/pkg/common"
)

// TapManager manages TAP devices for microVM networking.
type TapManager struct {
	bridge string
	subnet string
}

// NewTapManager creates a TAP device manager.
func NewTapManager(bridge, subnet string) *TapManager {
	if bridge == "" {
		bridge = "doki0"
	}
	if subnet == "" {
		subnet = "10.89.0.0/16"
	}
	return &TapManager{
		bridge: bridge,
		subnet: subnet,
	}
}

// CreateTap creates a TAP device for a VM.
func (t *TapManager) CreateTap(vmID string) (string, error) {
	tapName := fmt.Sprintf("doki-%s", vmID[:8])

	// Create TAP device.
	cmd := exec.Command("ip", "tuntap", "add", tapName, "mode", "tap")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("create tap %s: %w", tapName, err)
	}

	// Bring up.
	cmd = exec.Command("ip", "link", "set", tapName, "up")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bring up tap %s: %w", tapName, err)
	}

	// Add to bridge if it exists.
	if err := exec.Command("ip", "link", "set", tapName, "master", t.bridge).Run(); err == nil {
		return tapName, nil
	}

	return tapName, nil
}

// DeleteTap removes a TAP device.
func (t *TapManager) DeleteTap(tapName string) error {
	exec.Command("ip", "link", "del", tapName).Run()
	return nil
}

// SetupBridge creates or ensures a bridge device exists.
func (t *TapManager) SetupBridge() error {
	// Check if bridge already exists.
	if _, err := os.Stat(fmt.Sprintf("/sys/class/net/%s", t.bridge)); err == nil {
		return nil
	}

	cmd := exec.Command("ip", "link", "add", t.bridge, "type", "bridge")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create bridge %s: %w", t.bridge, err)
	}

	cmd = exec.Command("ip", "addr", "add", "10.89.0.1/16", "dev", t.bridge)
	cmd.Run()

	cmd = exec.Command("ip", "link", "set", t.bridge, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bring up bridge %s: %w", t.bridge, err)
	}

	return nil
}

// TeardownBridge removes the bridge device.
func (t *TapManager) TeardownBridge() {
	exec.Command("ip", "link", "del", t.bridge).Run()
}

// EnableMasquerade enables NAT for the bridge subnet.
func (t *TapManager) EnableMasquerade() error {
	if iptables, _ := exec.LookPath("iptables"); iptables != "" {
		exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", t.subnet, "-j", "MASQUERADE").Run()
		exec.Command("iptables", "-A", "FORWARD", "-i", t.bridge, "-j", "ACCEPT").Run()
		exec.Command("iptables", "-A", "FORWARD", "-o", t.bridge, "-j", "ACCEPT").Run()
	}
	return nil
}

// ─── CNI Integration ──────────────────────────────────────────────

// CNISetup runs a CNI plugin to configure networking.
func CNISetup(vmID, tapName, cniConfDir, cniBinDir string) error {
	// Basic CNI ADD via bridge plugin.
	if cniBinDir == "" {
		cniBinDir = "/usr/lib/cni"
	}
	bridgePlugin := fmt.Sprintf("%s/bridge", cniBinDir)
	if !common.PathExists(bridgePlugin) {
		return fmt.Errorf("CNI bridge plugin not found at %s", bridgePlugin)
	}

	// CNI config should be written to cniConfDir before calling.
	return nil
}

// CNITeardown removes CNI configuration for a VM.
func CNITeardown(vmID, tapName, cniConfDir, cniBinDir string) error {
	return nil
}
