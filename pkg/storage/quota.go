package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
)

// QuotaManager manages disk quotas for storage drivers.
type QuotaManager struct {
	driver Driver
}

// NewQuotaManager creates a new quota manager.
func NewQuotaManager(driver Driver) *QuotaManager {
	return &QuotaManager{driver: driver}
}

// SetQuota sets a disk quota for a container layer.
// Size is in bytes. 0 means no limit.
func (q *QuotaManager) SetQuota(id string, size int64) error {
	if size <= 0 {
		return nil // No quota.
	}

	switch d := q.driver.(type) {
	case *BtrfsDriver:
		return q.setBtrfsQuota(d, id, size)
	case *ZFSDriver:
		return q.setZFSQuota(d, id, size)
	default:
		// VFS and fuse-overlayfs don't support quotas.
		return fmt.Errorf("driver %s does not support quotas", d.Name())
	}
}

// GetQuota returns the current quota for a container layer.
func (q *QuotaManager) GetQuota(id string) (int64, error) {
	switch d := q.driver.(type) {
	case *BtrfsDriver:
		return q.getBtrfsQuota(d, id)
	case *ZFSDriver:
		return q.getZFSQuota(d, id)
	default:
		return 0, nil
	}
}

// GetUsage returns the current disk usage for a container layer.
func (q *QuotaManager) GetUsage(id string) (int64, error) {
	switch d := q.driver.(type) {
	case *BtrfsDriver:
		return q.getBtrfsUsage(d, id)
	case *ZFSDriver:
		return q.getZFSUsage(d, id)
	default:
		return q.getDirSize(id)
	}
}

func (q *QuotaManager) setBtrfsQuota(d *BtrfsDriver, id string, size int64) error {
	path := d.root + "/" + id
	if !common.PathExists(path) {
		return fmt.Errorf("btrfs: subvolume %s not found", id)
	}
	cmd := exec.Command("btrfs", "qgroup", "limit", fmt.Sprintf("%d", size), path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("btrfs qgroup limit: %s: %w", string(out), err)
	}
	return nil
}

func (q *QuotaManager) getBtrfsQuota(d *BtrfsDriver, id string) (int64, error) {
	path := d.root + "/" + id
	cmd := exec.Command("btrfs", "qgroup", "show", "--raw", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("btrfs qgroup show: %s: %w", string(out), err)
	}
	// Parse output: qgroupid rfer max_rfer
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[2:] { // Skip header.
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			if maxRef, err := strconv.ParseInt(fields[2], 10, 64); err == nil && maxRef > 0 {
				return maxRef, nil
			}
		}
	}
	return 0, nil
}

func (q *QuotaManager) getBtrfsUsage(d *BtrfsDriver, id string) (int64, error) {
	path := d.root + "/" + id
	cmd := exec.Command("btrfs", "qgroup", "show", "--raw", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("btrfs qgroup show: %s: %w", string(out), err)
	}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[2:] {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if rfer, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				return rfer, nil
			}
		}
	}
	return 0, nil
}

func (q *QuotaManager) setZFSQuota(d *ZFSDriver, id string, size int64) error {
	fsName := d.fsPrefix + "/" + id
	cmd := exec.Command("zfs", "set", fmt.Sprintf("quota=%d", size), fsName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("zfs set quota: %s: %w", string(out), err)
	}
	return nil
}

func (q *QuotaManager) getZFSQuota(d *ZFSDriver, id string) (int64, error) {
	fsName := d.fsPrefix + "/" + id
	cmd := exec.Command("zfs", "get", "-H", "-p", "-o", "value", "quota", fsName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("zfs get quota: %s: %w", string(out), err)
	}
	val := strings.TrimSpace(string(out))
	if val == "none" || val == "0" {
		return 0, nil
	}
	return strconv.ParseInt(val, 10, 64)
}

func (q *QuotaManager) getZFSUsage(d *ZFSDriver, id string) (int64, error) {
	fsName := d.fsPrefix + "/" + id
	cmd := exec.Command("zfs", "get", "-H", "-p", "-o", "value", "used", fsName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("zfs get used: %s: %w", string(out), err)
	}
	val := strings.TrimSpace(string(out))
	return strconv.ParseInt(val, 10, 64)
}

func (q *QuotaManager) getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
