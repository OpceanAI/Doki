package proot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Manager provides proot-based container support for Android kernels
// that lack full user namespace support.
type Manager struct {
	rootfsDir string
}

// NewManager creates a new proot manager.
func NewManager(rootfsDir string) *Manager {
	return &Manager{rootfsDir: rootfsDir}
}

// IsAvailable checks if proot is available on the system.
func IsAvailable() bool {
	_, err := exec.LookPath("proot")
	return err == nil
}

// Exec executes a command in a proot-based environment.
func (m *Manager) Exec(rootfs string, args []string, env []string, workDir string) error {
	prootArgs := []string{
		"-r", rootfs,
		"-b", "/proc",
		"-b", "/sys",
		"-b", "/dev",
	}

	// Bind /data for Termux compatibility.
	if _, err := os.Stat("/data"); err == nil {
		prootArgs = append(prootArgs, "-b", "/data")
	}

	// Bind /system for Android compatibility.
	if _, err := os.Stat("/system"); err == nil {
		prootArgs = append(prootArgs, "-b", "/system")
	}

	// Bind /vendor if available.
	if _, err := os.Stat("/vendor"); err == nil {
		prootArgs = append(prootArgs, "-b", "/vendor")
	}

	// Set working directory.
	if workDir != "" {
		prootArgs = append(prootArgs, "-w", workDir)
	}

	// Kill all processes on exit.
	prootArgs = append(prootArgs, "--kill-on-exit")

	// Pass environment.
	for _, e := range env {
		prootArgs = append(prootArgs, fmt.Sprintf("--env=%s", e))
	}

	// Append user command.
	prootArgs = append(prootArgs, args...)

	cmd := exec.Command("proot", prootArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd.Run()
}

// PrepareRootfs prepares a rootfs directory for proot usage.
func (m *Manager) PrepareRootfs(rootfs string, bindMounts map[string]string) error {
	if err := common.EnsureDir(rootfs); err != nil {
		return fmt.Errorf("create rootfs dir: %w", err)
	}

	// Create essential directories.
	dirs := []string{
		"proc", "sys", "dev", "tmp", "run",
		"etc", "var", "home", "root", "opt",
		"usr/bin", "usr/lib", "usr/share",
	}

	for _, dir := range dirs {
		if err := common.EnsureDir(filepath.Join(rootfs, dir)); err != nil {
			return err
		}
	}

	// Create symlinks.
	symlinks := map[string]string{
		filepath.Join(rootfs, "usr/bin/env"): "/bin/env",
	}

	for link, target := range symlinks {
		os.Remove(link)
		os.Symlink(target, link)
	}

	return nil
}

// ExtractLayer extracts a container layer to the rootfs.
func (m *Manager) ExtractLayer(rootfs, layerPath string) error {
	cmd := exec.Command("tar", "-xf", layerPath, "-C", rootfs, "--no-same-owner")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CopyFile copies a file into the proot root filesystem.
func (m *Manager) CopyFile(rootfs, src, dest string, mode os.FileMode) error {
	target := filepath.Join(rootfs, dest)

	if err := common.EnsureDir(filepath.Dir(target)); err != nil {
		return err
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(target, data, mode)
}

// WriteFile writes content to a file inside the proot root filesystem.
func (m *Manager) WriteFile(rootfs, dest, content string, mode os.FileMode) error {
	target := filepath.Join(rootfs, dest)

	if err := common.EnsureDir(filepath.Dir(target)); err != nil {
		return err
	}

	return os.WriteFile(target, []byte(content), mode)
}

// ReadFile reads a file from inside the proot root filesystem.
func (m *Manager) ReadFile(rootfs, path string) (string, error) {
	data, err := os.ReadFile(filepath.Join(rootfs, path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ResolvePath resolves a path relative to the proot root filesystem.
func (m *Manager) ResolvePath(rootfs, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Join(rootfs, path)
	}
	return filepath.Join(rootfs, path)
}

// SetupEnvironment prepares the environment for proot execution.
func SetupEnvironment(env []string) []string {
	prootEnv := []string{}

	for _, e := range env {
		// Filter out potentially dangerous variables.
		if strings.HasPrefix(e, "LD_PRELOAD=") ||
			strings.HasPrefix(e, "LD_LIBRARY_PATH=") {
			continue
		}
		prootEnv = append(prootEnv, e)
	}

	return prootEnv
}

// CanUseNamespaces checks if user namespaces are available on the system.
func CanUseNamespaces() bool {
	// Check /proc/sys/kernel/unprivileged_userns_clone.
	data, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err == nil {
		return strings.TrimSpace(string(data)) == "1"
	}

	// Fallback: try to create a user namespace.
	cmd := exec.Command("unshare", "-U", "true")
	err = cmd.Run()
	return err == nil
}

// ShouldUseProot determines whether proot fallback should be used.
func ShouldUseProot() bool {
	if !IsAvailable() {
		return false
	}

	// If can use namespaces, prefer them.
	if CanUseNamespaces() {
		return false
	}

	// On Android, use proot as fallback.
	for _, path := range []string{"/system/build.prop", "/system/bin", "/vendor"} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
}

// RunCommand runs a command inside a proot environment and captures output.
func RunCommand(rootfs string, args []string, env []string) (string, error) {
	prootArgs := []string{
		"-r", rootfs,
		"-b", "/proc",
		"-b", "/sys",
		"-b", "/dev",
	}

	for _, e := range env {
		prootArgs = append(prootArgs, fmt.Sprintf("--env=%s", e))
	}

	prootArgs = append(prootArgs, args...)

	cmd := exec.Command("proot", prootArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("proot command failed: %w\n%s", err, string(output))
	}

	return string(output), nil
}
