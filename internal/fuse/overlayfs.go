//go:build linux
// +build linux

package fuse

import (
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// OverlayFS implements a rootless overlay filesystem using fuse-overlayfs.
type OverlayFS struct {
	workDir string
}

// NewOverlayFS creates a new FUSE overlay filesystem manager.
func NewOverlayFS(workDir string) *OverlayFS {
	return &OverlayFS{workDir: workDir}
}

// Mount mounts an overlay filesystem at the target path.
func (o *OverlayFS) Mount(lowerDirs []string, upperDir, target string) error {
	if err := common.EnsureDir(target); err != nil {
		return fmt.Errorf("ensure mount target: %w", err)
	}

	if err := common.EnsureDir(upperDir); err != nil {
		return fmt.Errorf("ensure upper dir: %w", err)
	}

	workDir := filepath.Join(o.workDir, "work")
	if err := common.EnsureDir(workDir); err != nil {
		return fmt.Errorf("ensure work dir: %w", err)
	}

	if IsFuseOverlayfsAvailable() {
		return o.mountFuseOverlayFS(lowerDirs, upperDir, workDir, target)
	}

	return o.mountNativeOverlay(lowerDirs, upperDir, workDir, target)
}

func (o *OverlayFS) mountFuseOverlayFS(lowerDirs []string, upperDir, workDir, target string) error {
	args := []string{"-o"}

	lowerStr := strings.Join(lowerDirs, ":")
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerStr, upperDir, workDir)
	args = append(args, opts, target)

	cmd := exec.Command("fuse-overlayfs", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("fuse-overlayfs start: %w", err)
	}
	// fuse-overlayfs daemonizes or stays in foreground — run in background
	go func() {
		cmd.Wait()
	}()

	// Give it a moment to mount
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (o *OverlayFS) mountNativeOverlay(lowerDirs []string, upperDir, workDir, target string) error {
	lowerStr := strings.Join(lowerDirs, ":")
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerStr, upperDir, workDir)

	return syscall.Mount("overlay", target, "overlay", 0, opts)
}

// Unmount unmounts a FUSE overlay filesystem.
func (o *OverlayFS) Unmount(target string) error {
	// Try fusermount first.
	cmd := exec.Command("fusermount", "-u", target)
	if err := cmd.Run(); err != nil {
		// Fall back to umount.
		cmd = exec.Command("umount", target)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("unmount %s: %w", target, err)
		}
	}
	return nil
}

// UnmountForce force-unmounts a mount point.
func (o *OverlayFS) UnmountForce(target string) error {
	cmd := exec.Command("fusermount", "-uz", target)
	_ = cmd.Run()

	return syscall.Unmount(target, syscall.MNT_DETACH)
}

// IsFuseOverlayfsAvailable checks if fuse-overlayfs is installed.
func IsFuseOverlayfsAvailable() bool {
	_, err := exec.LookPath("fuse-overlayfs")
	return err == nil
}

// IsFusermountAvailable checks if fusermount is available.
func IsFusermountAvailable() bool {
	_, err := exec.LookPath("fusermount")
	return err == nil
}

// PrepareRootfs prepares a rootfs directory with proper permissions.
func PrepareRootfs(rootfs string, files map[string]string, user ...string) error {
	for path, content := range files {
		fullPath := filepath.Join(rootfs, path)

		if err := common.EnsureDir(filepath.Dir(fullPath)); err != nil {
			return err
		}

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return err
		}
	}

	// Create essential device nodes if we have CAP_MKNOD.
	mknod := func(path string, mode uint32, dev int) error {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(rootfs, path)), 0755); err != nil {
			return err
		}
		return syscall.Mknod(filepath.Join(rootfs, path), mode, dev)
	}

	if os.Geteuid() == 0 {
		devices := map[string][2]uint32{
			"dev/null":    {syscall.S_IFCHR | 0666, 0x0103},
			"dev/zero":    {syscall.S_IFCHR | 0666, 0x0105},
			"dev/random":  {syscall.S_IFCHR | 0666, 0x0108},
			"dev/urandom": {syscall.S_IFCHR | 0666, 0x0109},
			"dev/tty":     {syscall.S_IFCHR | 0666, 0x0500},
			"dev/console": {syscall.S_IFCHR | 0600, 0x0501},
			"dev/ptmx":    {syscall.S_IFCHR | 0666, 0x0502},
		}

		for dev, info := range devices {
			mknod(dev, info[0], int(info[1]))
		}

		// Create symlinks.
		symlinks := map[string]string{
			"dev/fd":     "/proc/self/fd",
			"dev/stdin":  "/proc/self/fd/0",
			"dev/stdout": "/proc/self/fd/1",
			"dev/stderr": "/proc/self/fd/2",
		}

		for link, target := range symlinks {
			os.Remove(filepath.Join(rootfs, link))
			if err := os.Symlink(target, filepath.Join(rootfs, link)); err != nil {
				return fmt.Errorf("create symlink %s -> %s: %w", link, target, err)
			}
		}
	}

	// Inject configured user into /etc/passwd and /etc/group.
	if len(user) > 0 {
		injectUser(rootfs, user[0])
	}

	return nil
}

// injectUser writes a user entry into /etc/passwd and /etc/group inside rootfs.
func injectUser(rootfsDir, user string) error {
	if user == "" {
		return nil
	}

	passwdPath := filepath.Join(rootfsDir, "etc", "passwd")
	groupPath := filepath.Join(rootfsDir, "etc", "group")

	os.MkdirAll(filepath.Dir(passwdPath), 0755)

	parts := strings.SplitN(user, ":", 2)
	name := parts[0]
	uid := 1000
	gid := 1000

	if u, err := strconv.Atoi(name); err == nil {
		uid = u
		name = fmt.Sprintf("user%d", uid)
	}
	if len(parts) > 1 {
		if g, err := strconv.Atoi(parts[1]); err == nil {
			gid = g
		}
	}

	// Only add if user doesn't already exist
	if data, err := os.ReadFile(passwdPath); err == nil {
		if strings.Contains(string(data), ":"+name+":") {
			return nil
		}
	}

	f, err := os.OpenFile(passwdPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open passwd: %w", err)
	}
	defer f.Close()

	homeDir := "/home/" + name
	os.MkdirAll(filepath.Join(rootfsDir, homeDir), 0755)

	entry := fmt.Sprintf("%s:x:%d:%d:%s:%s:/bin/sh\n", name, uid, gid, name, homeDir)
	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("write passwd: %w", err)
	}

	gf, err := os.OpenFile(groupPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer gf.Close()
		gf.WriteString(fmt.Sprintf("%s:x:%d:\n", name, gid))
	}

	return nil
}

// GenerateResolvConf generates a resolv.conf file.
// If internalDNS is provided, it is added as the first nameserver
// (containers use the internal DNS server for container name resolution).
// Port is stripped from nameserver addresses — resolv.conf does not support
// port-qualified nameservers.
func GenerateResolvConf(dnsServers []string, searchDomains []string, options []string, internalDNS ...string) string {
	var sb strings.Builder

	// Internal DNS server first (for container name resolution).
	if len(internalDNS) > 0 && internalDNS[0] != "" {
		host, _, err := net.SplitHostPort(internalDNS[0])
		if err != nil {
			// No port present, use as-is
			host = internalDNS[0]
		}
		sb.WriteString(fmt.Sprintf("nameserver %s\n", host))
	}

	for _, dns := range dnsServers {
		host, _, err := net.SplitHostPort(dns)
		if err != nil {
			host = dns
		}
		sb.WriteString(fmt.Sprintf("nameserver %s\n", host))
	}

	if len(searchDomains) > 0 {
		sb.WriteString(fmt.Sprintf("search %s\n", strings.Join(searchDomains, " ")))
	}

	if len(options) > 0 {
		sb.WriteString(fmt.Sprintf("options %s\n", strings.Join(options, " ")))
	} else if len(internalDNS) > 0 && internalDNS[0] != "" {
		// Default ndots:0 so single-label container names resolve without dots.
		sb.WriteString("options ndots:0\n")
	}

	return sb.String()
}

// GenerateHosts generates a /etc/hosts file.
func GenerateHosts(hostname string, extraHosts map[string]string) string {
	var sb strings.Builder
	sb.WriteString("127.0.0.1 localhost\n")
	sb.WriteString("::1 localhost ip6-localhost ip6-loopback\n")
	sb.WriteString("fe00::0 ip6-localnet\n")
	sb.WriteString("ff00::0 ip6-mcastprefix\n")
	sb.WriteString("ff02::1 ip6-allnodes\n")
	sb.WriteString("ff02::2 ip6-allrouters\n")

	if hostname != "" {
		sb.WriteString(fmt.Sprintf("127.0.0.1 %s\n", hostname))
		sb.WriteString(fmt.Sprintf("::1 %s\n", hostname))
	}

	for host, ip := range extraHosts {
		sb.WriteString(fmt.Sprintf("%s %s\n", ip, host))
	}

	return sb.String()
}

// GenerateHostname generates a hostname file.
func GenerateHostname(hostname string) string {
	return hostname + "\n"
}

// CopyDir recursively copies a directory.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		targetPath := filepath.Join(dst, relPath)

		// C2: Preserve symlinks instead of reading their content.
		if d.Type()&fs.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			os.Remove(targetPath)
			return os.Symlink(linkTarget, targetPath)
		}

		if d.IsDir() {
			return common.EnsureDir(targetPath)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, data, info.Mode())
	})
}

// BindMount creates a bind mount.
func BindMount(source, target string, readOnly bool) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	flags := syscall.MS_BIND
	if readOnly {
		flags |= syscall.MS_RDONLY
	}
	return syscall.Mount(source, target, "", uintptr(flags), "")
}

// TmpfsMount creates a tmpfs mount.
func TmpfsMount(target string, sizeBytes int64, mode uint32) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}

	opts := []string{}
	if sizeBytes > 0 {
		opts = append(opts, fmt.Sprintf("size=%d", sizeBytes))
	}
	if mode > 0 {
		opts = append(opts, fmt.Sprintf("mode=%o", mode))
	}
	optStr := strings.Join(opts, ",")

	return syscall.Mount("tmpfs", target, "tmpfs", 0, optStr)
}

// ProcMount mounts /proc inside a container.
func ProcMount(target string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	return syscall.Mount("proc", target, "proc", 0, "")
}

// SysMount mounts /sys inside a container.
func SysMount(target string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	return syscall.Mount("none", target, "sysfs", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
}

// DevMount mounts /dev inside a container (tmpfs-based).
func DevMount(target string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	return syscall.Mount("tmpfs", target, "tmpfs", syscall.MS_NOSUID|syscall.MS_NOEXEC, "size=65536k,mode=755")
}

// DevPtsMount mounts /dev/pts inside a container.
func DevPtsMount(target string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}

	if err := syscall.Mount("devpts", target, "devpts", syscall.MS_NOSUID|syscall.MS_NOEXEC, "newinstance,ptmxmode=0666,mode=0620,gid=5"); err != nil {
		return fmt.Errorf("mount devpts: %w", err)
	}

	return nil
}

// ShmMount mounts /dev/shm inside a container.
func ShmMount(target string, sizeBytes int64) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	opts := fmt.Sprintf("size=%d,nosuid,nodev,noexec", sizeBytes)
	return syscall.Mount("none", target, "tmpfs", syscall.MS_NOSUID|syscall.MS_NOEXEC, opts)
}

// MqueueMount mounts /dev/mqueue inside a container.
func MqueueMount(target string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	return syscall.Mount("mqueue", target, "mqueue", 0, "")
}

// EnsureMountedDir ensures a directory is mounted.
func EnsureMountedDir(source, target, fstype string, flags uintptr, data string) error {
	if err := common.EnsureDir(target); err != nil {
		return err
	}
	return syscall.Mount(source, target, fstype, flags, data)
}

// CleanupMounts unmounts all mounts recursively from a path.
func CleanupMounts(path string) error {
	// Find all mounts inside the path and unmount them.
	cmd := exec.Command("findmnt", "--list", "--noheadings", "--submounts", "--output", "TARGET", path)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	mounts := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Reverse order to unmount from deepest first.
	for i := len(mounts) - 1; i >= 0; i-- {
		mnt := strings.TrimSpace(mounts[i])
		if mnt != "" {
			syscall.Unmount(mnt, syscall.MNT_DETACH)
		}
	}

	return nil
}
