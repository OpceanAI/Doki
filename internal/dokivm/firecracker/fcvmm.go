package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
)

// VMM implements the VMM interface using AWS Firecracker.
type VMM struct {
	mu     sync.RWMutex
	vms    map[string]*dokivm.MicroVM
	cfg    *dokivm.VMMConfig
	binary string
	jailer string
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
	}, nil
}

func init() {
	dokivm.RegisterBackend("firecracker", func(cfg *dokivm.VMMConfig) (dokivm.VMM, error) {
		return New(cfg)
	})
}

func (v *VMM) Name() string { return "firecracker" }

func (v *VMM) Create(ctx context.Context, vmCfg *dokivm.VMConfig) (*dokivm.MicroVM, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	vm := &dokivm.MicroVM{
		ID:        vmCfg.ID,
		VMM:       v,
		State:     dokivm.VMStateCreated,
		CreatedAt: time.Now(),
	}
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
	os.MkdirAll(workDir, 0755)
	apiSock := filepath.Join(workDir, "api.sock")
	os.Remove(apiSock)

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

	v.configureMachine(client)
	v.configureBootSource(client, vmID)
	v.configureDrives(client, vmID)
	v.configureNetwork(client, vmID)

	// Start the VM.
	v.sendAction(client, "InstanceStart")

	vm.State = dokivm.VMStateRunning
	vm.StartedAt = time.Now()

	// Monitor.
	go func() {
		cmd.Wait()
		v.mu.Lock()
		if vm, ok := v.vms[vmID]; ok {
			vm.State = dokivm.VMStateStopped
		}
		v.mu.Unlock()
	}()

	return nil
}

func (v *VMM) configureMachine(client *http.Client) {
	body := map[string]interface{}{
		"vcpu_count":  1,
		"mem_size_mib": 128,
		"cpu_template": "T2",
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "http://unix/machine-config", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

func (v *VMM) configureBootSource(client *http.Client, vmID string) {
	body := map[string]interface{}{
		"kernel_image_path": filepath.Join(v.cfg.KernelPath),
		"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off nomodules ro quiet doki.init=1",
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "http://unix/boot-source", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

func (v *VMM) configureDrives(client *http.Client, vmID string) {
	rootfsPath := filepath.Join(v.cfg.WorkDir, vmID, "rootfs.ext4")
	if _, err := os.Stat(rootfsPath); err != nil {
		return
	}
	body := map[string]interface{}{
		"drive_id":      "rootfs",
		"path_on_host":  rootfsPath,
		"is_root_device": true,
		"is_read_only":   false,
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "http://unix/drives/rootfs", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

func (v *VMM) configureNetwork(client *http.Client, vmID string) {
	tapName := fmt.Sprintf("doki-fc-%s", vmID[:8])
	body := map[string]interface{}{
		"iface_id":     "eth0",
		"host_dev_name": tapName,
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "http://unix/network-interfaces/eth0", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

func (v *VMM) sendAction(client *http.Client, action string) {
	body := map[string]interface{}{
		"action_type": action,
	}
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("PUT", "http://unix/actions", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
}

func (v *VMM) Stop(ctx context.Context, vmID string, timeout time.Duration) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, _ := os.FindProcess(vm.PID)
	proc.Signal(syscall.SIGTERM)
	select {
	case <-time.After(timeout):
		proc.Signal(syscall.SIGKILL)
	case <-ctx.Done():
		return ctx.Err()
	}
	vm.State = dokivm.VMStateStopped
	return nil
}

func (v *VMM) Kill(ctx context.Context, vmID string) error {
	v.mu.RLock()
	vm, ok := v.vms[vmID]
	v.mu.RUnlock()
	if !ok || vm.PID == 0 {
		return nil
	}
	proc, _ := os.FindProcess(vm.PID)
	proc.Signal(syscall.SIGKILL)
	vm.State = dokivm.VMStateStopped
	return nil
}

func (v *VMM) State(ctx context.Context, vmID string) (dokivm.VMState, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	vm, ok := v.vms[vmID]
	if !ok {
		return "", fmt.Errorf("vm %s not found", vmID)
	}
	return vm.State, nil
}

func (v *VMM) Exec(ctx context.Context, vmID string, cmd []string, env []string, tty bool) error {
	return fmt.Errorf("exec via vsock not implemented")
}
func (v *VMM) Attach(ctx context.Context, vmID string) error                             { return nil }
func (v *VMM) Logs(ctx context.Context, vmID string) (io.Reader, error)                 { return nil, nil }
func (v *VMM) Stats(ctx context.Context, vmID string) (*dokivm.VMStats, error)          { return &dokivm.VMStats{}, nil }
func (v *VMM) Cleanup(ctx context.Context, vmID string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.vms, vmID)
	os.RemoveAll(filepath.Join(v.cfg.WorkDir, vmID))
	return nil
}

// IsAvailable checks if Firecracker is available.
func IsAvailable() bool {
	_, err := exec.LookPath("firecracker")
	return err == nil
}

var _ dokivm.VMM = (*VMM)(nil)
