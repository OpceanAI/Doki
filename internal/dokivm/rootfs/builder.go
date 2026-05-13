package rootfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Builder builds rootfs images for microVMs from OCI layers.
type Builder struct {
	workDir string
	mkfs    string
}

// NewBuilder creates a rootfs builder.
func NewBuilder(workDir string) *Builder {
	mkfs, _ := exec.LookPath("mkfs.ext4")
	if mkfs == "" {
		mkfs, _ = exec.LookPath("mke2fs")
	}
	return &Builder{
		workDir: workDir,
		mkfs:    mkfs,
	}
}

// BuildRootfs builds an ext4 rootfs image from extracted OCI layers.
func (b *Builder) BuildRootfs(vmID string, rootfsDir string, sizeMB int) (string, error) {
	if b.mkfs == "" {
		return "", fmt.Errorf("mkfs.ext4 not found")
	}
	if sizeMB <= 0 {
		sizeMB = 256
	}

	vmDir := filepath.Join(b.workDir, vmID)
	common.EnsureDir(vmDir)

	rootfsPath := filepath.Join(vmDir, "rootfs.ext4")

	// Check if source rootfs exists and has content.
	entries, _ := os.ReadDir(rootfsDir)
	if len(entries) == 0 {
		return rootfsPath, nil
	}

	// Create sparse file.
	sizeStr := fmt.Sprintf("%dM", sizeMB)
	cmd := exec.Command("truncate", "-s", sizeStr, rootfsPath)
	if err := cmd.Run(); err != nil {
		// Fallback: use dd.
		cmd = exec.Command("dd", "if=/dev/zero", "of="+rootfsPath, "bs=1M", "count="+fmt.Sprintf("%d", sizeMB))
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("create sparse file: %w", err)
		}
	}

	// Format as ext4.
	cmd = exec.Command(b.mkfs, "-F", "-d", rootfsDir, rootfsPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mkfs.ext4: %w\n%s", err, string(out))
	}

	return rootfsPath, nil
}

// BuildMinimalRootfs builds a minimal rootfs with just the OCI layers
// and the doki-init binary injected.
// Searches for doki-init-rust first, falls back to doki-init (Go).
func (b *Builder) BuildMinimalRootfs(vmID string, layersDir string, dokiInitPath string) (string, error) {
	vmDir := filepath.Join(b.workDir, vmID)
	common.EnsureDir(vmDir)

	stagingDir := filepath.Join(vmDir, "staging")
	common.EnsureDir(stagingDir)

	// Create essential directories.
	for _, dir := range []string{"bin", "sbin", "dev", "proc", "sys", "tmp", "etc", "var", "run"} {
		common.EnsureDir(filepath.Join(stagingDir, dir))
	}

	// Find doki-init binary: try doki-init-rust first, then doki-init (Go).
	initPath := b.findDokiInit(dokiInitPath)
	if initPath != "" {
		data, err := os.ReadFile(initPath)
		if err != nil {
			return "", fmt.Errorf("read doki-init: %w", err)
		}
		if err := os.WriteFile(filepath.Join(stagingDir, "sbin", "init"), data, 0755); err != nil {
			return "", fmt.Errorf("write doki-init: %w", err)
		}
	}

	rootfsPath := filepath.Join(vmDir, "rootfs.ext4")
	cmd := exec.Command(b.mkfs, "-F", "-d", stagingDir, rootfsPath, "256M")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mkfs.ext4: %w\n%s", err, string(out))
	}

	return rootfsPath, nil
}

// KernelManager manages kernel images for microVMs.
type KernelManager struct {
	kernelDir string
}

// NewKernelManager creates a kernel manager.
func NewKernelManager(kernelDir string) *KernelManager {
	return &KernelManager{kernelDir: kernelDir}
}

// GetKernelPath returns the path to the kernel for the current architecture.
func (k *KernelManager) GetKernelPath() (string, error) {
	// Check for architecture-specific kernel.
	arch := os.Getenv("DOKI_KERNEL")
	if arch != "" && common.PathExists(arch) {
		return arch, nil
	}

	candidates := []string{
		filepath.Join(k.kernelDir, "vmlinux-arm64"),
		filepath.Join(k.kernelDir, "vmlinux-x86_64"),
		filepath.Join(k.kernelDir, "vmlinux.bin"),
		"/var/lib/doki/kernels/vmlinux-arm64",
		"/var/lib/doki/kernels/vmlinux-x86_64",
		"/var/lib/doki/kernels/vmlinux.bin",
	}

	for _, c := range candidates {
		if common.PathExists(c) {
			return c, nil
		}
	}

	return "", fmt.Errorf("no kernel found in %s (set DOKI_KERNEL env var)", k.kernelDir)
}

// HasKernel checks if a kernel is available.
func (k *KernelManager) HasKernel() bool {
	_, err := k.GetKernelPath()
	return err == nil
}

// findDokiInit locates the doki-init binary.
// Searches doki-init-rust first, then doki-init (Go fallback).
func (b *Builder) findDokiInit(explicitPath string) string {
	if explicitPath != "" && common.PathExists(explicitPath) {
		return explicitPath
	}
	candidates := []string{
		filepath.Join(b.workDir, "doki-init-rust"),
		filepath.Join(b.workDir, "doki-init"),
		"doki-init-rust",
		"doki-init",
	}
	for _, c := range candidates {
		if common.PathExists(c) {
			return c
		}
	}
	return ""
}
