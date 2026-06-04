package proot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// FindProotBinary locates the best available proot binary and verifies it exists.
// Searches for doki-proot first, then falls back to system proot. Returns the empty
// string when no usable proot binary is found, so callers can surface a clear error
// instead of "executable not found" at exec.Cmd time.
func FindProotBinary() string {
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
	if p, err := exec.LookPath("proot"); err == nil {
		return p
	}
	return ""
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

// ShouldUseProot returns true when proot should be the preferred execution mode.
// Typically on Android/Termux where user namespaces are not available.
func ShouldUseProot() bool {
	if _, err := os.Stat("/system/bin/adb"); err == nil {
		return true
	}
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		return true
	}
	return false
}

// IsTermuxProot checks if the proot binary is the Termux-specific build.
func IsTermuxProot() bool {
	p, err := exec.LookPath("proot")
	if err != nil {
		return false
	}
	return strings.Contains(p, "/data/data/com.termux")
}

// DetectKernelRelease reads the actual kernel release from /proc.
func DetectKernelRelease() string {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err == nil && len(data) > 2 {
		return strings.TrimSpace(string(data))
	}
	return "6.17.0-PRoot-Distro"
}

// BuildProotBaseArgs returns the common proot arguments for all execution methods.
// If uid/gid are >= 0, adds -i flag to impersonate that user inside proot.
func BuildProotBaseArgs(rootfs string, uid, gid int) []string {
	args := []string{
		"-r", rootfs,
		"-b", "/proc",
		"-b", "/proc/self/fd:/dev/fd",
		"-b", "/sys",
		"-b", "/dev",
		"-b", "/dev/urandom:/dev/random",
		"--kill-on-exit",
		"--link2symlink",
		"--kernel-release=" + DetectKernelRelease(),
	}

	if uid >= 0 && gid >= 0 {
		args = append(args, "-i", fmt.Sprintf("%d:%d", uid, gid))
	}

	selinuxTarget := filepath.Join(rootfs, "sys", "fs", "selinux")
	os.MkdirAll(selinuxTarget, 0755)
	args = append(args, "-b", selinuxTarget+":/sys/fs/selinux")

	return args
}

// AppendAndroidBinds appends Android-specific bind mounts to proot args.
func AppendAndroidBinds(args []string) []string {
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
	env := common.StripHostEnv(os.Environ())
	env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/")
	env = append(env, "LD_LIBRARY_PATH=/usr/lib:/lib:/usr/local/lib")
	for _, e := range userEnv {
		env = append(env, e)
	}
	return env
}

// Exec executes a command in a proot-based environment.
func (m *Manager) Exec(rootfs string, args []string, env []string, workDir string) (string, error) {
	prootArgs := BuildProotBaseArgs(rootfs, -1, -1)

	// AppendAndroidBinds adds Android-specific mount points.
	prootArgs = AppendAndroidBinds(prootArgs)
	prootArgs = append(prootArgs, args...)

	prootBin := FindProotBinary()
	cmd := exec.Command(prootBin, prootArgs...)
	cmd.Env = buildProotEnv(env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("proot command failed: %w\n%s", err, string(output))
	}
	return string(output), nil
}
