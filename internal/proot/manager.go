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

// findProotBinary locates the best available proot binary.
// Searches for doki-proot first, falls back to system proot.
func findProotBinary() string {
	exe, _ := os.Executable()
	candidates := []string{
		filepath.Join(filepath.Dir(exe), "doki-proot"),
		"doki-proot",
	}
	for _, c := range candidates {
		if common.PathExists(c) {
			return c
		}
	}
	return "proot"
}

// IsAvailable checks if proot (or doki-proot) is available on the system.
func IsAvailable() bool {
	if common.PathExists("doki-proot") {
		return true
	}
	p, err := exec.LookPath("proot")
	if err != nil {
		return false
	}
	return p != ""
}

// IsTermuxProot checks if the proot binary is the Termux-specific build.
func IsTermuxProot() bool {
	p, _ := exec.LookPath("proot")
	return strings.Contains(p, "/data/data/com.termux")
}

// buildProotBaseArgs returns the common proot arguments for all execution methods.
func buildProotBaseArgs(rootfs string) []string {
	args := []string{
		"-r", rootfs,
		"-b", "/proc",
		"-b", "/proc/self/fd:/dev/fd",
		"-b", "/sys",
		"-b", "/dev",
		"-b", "/dev/urandom:/dev/random",
		"--kill-on-exit",
		"--link2symlink",
		"--kernel-release=6.17.0-PRoot-Distro",
	}

	selinuxTarget := filepath.Join(rootfs, "sys", "fs", "selinux")
	os.MkdirAll(selinuxTarget, 0755)
	args = append(args, "-b", selinuxTarget+":/sys/fs/selinux")

	return args
}

// appendAndroidBinds appends Android-specific bind mounts to proot args.
func appendAndroidBinds(args []string) []string {
	for _, dir := range []string{
		"/apex", "/system", "/vendor",
		"/storage", "/sdcard",
		"/data/data/com.termux/files",
	} {
		if _, err := os.Stat(dir); err == nil {
			args = append(args, "-b", dir)
		}
	}
	if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
		args = append(args, "-b", "/data/data/com.termux/files/usr")
	}
	if _, err := os.Stat("/linkerconfig/ld.config.txt"); err == nil {
		args = append(args, "-b", "/linkerconfig/ld.config.txt")
	}
	if home := os.Getenv("HOME"); home != "" {
		args = append(args, "-b", home)
	}
	return args
}

// buildProotEnv builds the environment slice for proot execution.
func buildProotEnv(userEnv []string) []string {
	env := os.Environ()
	env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/")
	env = append(env, "LD_LIBRARY_PATH=/usr/lib:/lib:/usr/local/lib")
	for _, e := range userEnv {
		env = append(env, e)
	}
	return env
}

// Exec executes a command in a proot-based environment.
func (m *Manager) Exec(rootfs string, args []string, env []string, workDir string) error {
	prootArgs := buildProotBaseArgs(rootfs)
	prootArgs = appendAndroidBinds(prootArgs)

	if workDir != "" {
		prootArgs = append(prootArgs, "-w", workDir)
	}
	prootArgs = append(prootArgs, args...)

	prootBin := findProotBinary()
	cmd := exec.Command(prootBin, prootArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = buildProotEnv(env)

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
	prootArgs := buildProotBaseArgs(rootfs)
	prootArgs = appendAndroidBinds(prootArgs)
	prootArgs = append(prootArgs, args...)

	prootBin := findProotBinary()
	cmd := exec.Command(prootBin, prootArgs...)
	cmd.Env = buildProotEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("proot command failed: %w\n%s", err, string(output))
	}
	return string(output), nil
}
