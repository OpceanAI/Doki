// Package firecracker provides Firecracker-based VM backend.
package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
	"github.com/OpceanAI/Doki/internal/dokivm/network"
)

// VMM implements the VMM interface using AWS Firecracker.
type VMM struct {
	mu     sync.RWMutex
	vms    map[string]*dokivm.MicroVM
	cfg    *dokivm.VMMConfig
	binary string
	jailer string
	vmCfg  *dokivm.VMConfig
	tapMgr *network.TapManager
}

func New(cfg *dokivm.VMMConfig) (*VMM, error) {
	binary, err := exec.LookPath("firecracker")
	if err != nil {
		return nil, fmt.Errorf("firecracker not found: %w", err)
	}
	jailer, _ := exec.LookPath("jailer")
	return &VMM{
		vms:    make(map[string]*dokivm.MicroVM),
		cfg:    cfg,
		binary: binary,
		jailer: jailer,
		tapMgr: network.NewTapManager("", ""),
	}, nil
}

func init() {
	dokivm.RegisterBackend("firecracker", func(cfg *dokivm.VMMConfig) (dokivm.VMM, error) {
		return New(cfg)
	})
}

func (v *VMM) Name() string { return "firecracker" }

func (v *VMM) Create(_ context.Context, vmCfg *dokivm.VMConfig) (*dokivm.MicroVM, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	vm := &dokivm.MicroVM{
		ID:        vmCfg.ID,
		VMM:       v,
		State:     dokivm.VMStateCreated,
		CreatedAt: time.Now(),
	}
	v.vmCfg = vmCfg
	v.vms[vm.ID] = vm
	return vm, nil
}

func (v *VMM) Start(ctx context.Context, vmID string) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok {
		return fmt.Errorf("vm %s not found", vmID)
	}

	// Create API socket path.
	workDir := filepath.Join(v.cfg.WorkDir, vmID)
	_ = os.MkdirAll(workDir, 0755)
	apiSock := filepath.Join(workDir, "api.sock")
	_ = os.Remove(apiSock)

	// Start Firecracker.
	cmd := exec.CommandContext(ctx, v.binary, "--api-sock", apiSock)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("firecracker start: %w", err)
	}
	vm.PID = cmd.Process.Pid

	// Wait for API socket.
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(apiSock); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Configure VM via HTTP API.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", apiSock)
			},
		},
		Timeout: 10 * time.Second,
	}

	if err := v.configureMachine(client); err != nil {
		return fmt.Errorf("configure machine: %w", err)
	}
	if err := v.configureBootSource(client, vmID); err != nil {
		return fmt.Errorf("configure boot source: %w", err)
	}
	if err := v.configureDrives(client, vmID); err != nil {
		return fmt.Errorf("configure drives: %w", err)
	}
	if err := v.configureNetwork(client, vmID); err != nil {
		return fmt.Errorf("configure network: %w", err)
	}

	// Start the VM.
	if err := v.sendAction(client, "InstanceStart"); err != nil {
		return fmt.Errorf("start VM: %w", err)
	}

	vm.State = dokivm.VMStateRunning
	vm.StartedAt = time.Now()

	// Monitor.
	go func() {
		_ = cmd.Wait()
		v.mu.Lock()
		if vm, ok := v.vms[vmID]; ok {
			vm.State = dokivm.VMStateStopped
		}
		v.mu.Unlock()
	}()

	return nil
}

func (v *VMM) configureMachine(client *http.Client) error {
	vcpus := 1
	if v.vmCfg != nil && v.vmCfg.CPUs > 0 {
		vcpus = v.vmCfg.CPUs
	}
	memMib := 128
	if v.vmCfg != nil && v.vmCfg.Memory > 0 {
		memMib = v.vmCfg.Memory
	}
	cpuTemplate := "None"
	switch runtime.GOARCH {
	case "arm64":
		cpuTemplate = "T2"
	case "amd64":
		cpuTemplate = "C3"
	}
	body := map[string]interface{}{
		"vcpu_count":   vcpus,
		"mem_size_mib": memMib,
		"cpu_template": cpuTemplate,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal machine config: %w", err)
	}
	req, err := http.NewRequest("PUT", "http://unix/machine-config", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create machine config request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send machine config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("machine config: status %d", resp.StatusCode)
	}
	return nil
}

func (v *VMM) configureBootSource(client *http.Client, _ string) error {
	body := map[string]interface{}{
		"kernel_image_path": filepath.Join(v.cfg.KernelPath),
		"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off nomodules ro quiet doki.init=1",
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal boot source: %w", err)
	}
	req, err := http.NewRequest("PUT", "http://unix/boot-source", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create boot source request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send boot source: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("boot source: status %d", resp.StatusCode)
	}
	return nil
}

func (v *VMM) configureDrives(client *http.Client, vmID string) error {
	rootfsPath := filepath.Join(v.cfg.WorkDir, vmID, "rootfs.ext4")
	if _, err := os.Stat(rootfsPath); err != nil {
		return nil // no rootfs to configure
	}
	body := map[string]interface{}{
		"drive_id":       "rootfs",
		"path_on_host":   rootfsPath,
		"is_root_device": true,
		"is_read_only":   false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal drive config: %w", err)
	}
	req, err := http.NewRequest("PUT", "http://unix/drives/rootfs", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create drive config request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send drive config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("drive config: status %d", resp.StatusCode)
	}
	return nil
}

func (v *VMM) configureNetwork(client *http.Client, vmID string) error {
	tapName, err := v.tapMgr.CreateTap(vmID)
	if err != nil {
		return fmt.Errorf("create tap device: %w", err)
	}
	body := map[string]interface{}{
		"iface_id":      "eth0",
		"host_dev_name": tapName,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal network config: %w", err)
	}
	req, err := http.NewRequest("PUT", "http://unix/network-interfaces/eth0", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create network config request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send network config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("network config: status %d", resp.StatusCode)
	}
	return nil
}

func (v *VMM) sendAction(client *http.Client, action string) error {
	body := map[string]interface{}{
		"action_type": action,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal action: %w", err)
	}
	req, err := http.NewRequest("PUT", "http://unix/actions", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create action request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send action: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("action %s: status %d", action, resp.StatusCode)
	}
	return nil
}

func (v *VMM) Stop(ctx context.Context, vmID string, timeout time.Duration) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, err := os.FindProcess(vm.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", vm.PID, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		slog.Warn("firecracker: SIGTERM failed", "pid", vm.PID, "err", err)
	}
	select {
	case <-time.After(timeout):
		_ = proc.Signal(syscall.SIGKILL)
	case <-ctx.Done():
		return ctx.Err()
	}
	v.mu.Lock()
	vm.State = dokivm.VMStateStopped
	v.mu.Unlock()
	return nil
}

func (v *VMM) Kill(_ context.Context, vmID string) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, err := os.FindProcess(vm.PID)
	if err != nil {
		return fmt.Errorf("find process %d: %w", vm.PID, err)
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		slog.Warn("firecracker: SIGKILL failed", "pid", vm.PID, "err", err)
	}
	v.mu.Lock()
	vm.State = dokivm.VMStateStopped
	v.mu.Unlock()
	return nil
}

func (v *VMM) State(_ context.Context, vmID string) (dokivm.VMState, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	vm, ok := v.vms[vmID]
	if !ok {
		return "", fmt.Errorf("vm %s not found", vmID)
	}
	return vm.State, nil
}

func (v *VMM) Exec(_ context.Context, _ string, _ []string, _ []string, _ bool) error {
	return fmt.Errorf("exec via vsock not implemented")
}
func (v *VMM) Attach(_ context.Context, _ string) error            { return nil }
func (v *VMM) Logs(_ context.Context, _ string) (io.Reader, error) { return nil, nil }
func (v *VMM) Stats(_ context.Context, _ string) (*dokivm.VMStats, error) {
	return &dokivm.VMStats{}, nil
}
func (v *VMM) Cleanup(_ context.Context, vmID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vms, vmID)
	_ = os.RemoveAll(filepath.Join(v.cfg.WorkDir, vmID))
	return nil
}

// IsAvailable checks if Firecracker is available.
func IsAvailable() bool {
	_, err := exec.LookPath("firecracker")
	return err == nil
}

var _ dokivm.VMM = (*VMM)(nil)
