//go:build !darwin

package macos

type VZBackend struct{}
type QEMUBackend struct{}

func NewVZBackend() *VZBackend {
	return nil
}

func NewQEMUBackend() *QEMUBackend {
	return nil
}

func (b *VZBackend) Name() string         { return "vz" }
func (b *VZBackend) Available() bool       { return false }
func (b *VZBackend) MinVersion() string    { return "N/A" }
func (b *VZBackend) CreateVM(cfg *VMConfig) error { return nil }
func (b *VZBackend) StartVM(id string) error { return nil }
func (b *VZBackend) StopVM(id string, timeoutSec int) error { return nil }
func (b *VZBackend) DeleteVM(id string) error { return nil }
func (b *VZBackend) VMStatus(id string) (string, error) { return "", nil }
func (b *VZBackend) ShareHostDir(hostPath, guestPath, tag string, readOnly bool) error { return nil }
func (b *VZBackend) UnshareHostDir(tag string) error { return nil }
func (b *VZBackend) ForwardPort(hostPort, guestPort int, proto string) error { return nil }
func (b *VZBackend) RemoveForwardPort(hostPort int, proto string) error { return nil }
func (b *VZBackend) Stats(id string) (*VMStats, error) { return nil, nil }

func (b *QEMUBackend) Name() string         { return "qemu" }
func (b *QEMUBackend) Available() bool       { return false }
func (b *QEMUBackend) MinVersion() string    { return "N/A" }
func (b *QEMUBackend) CreateVM(cfg *VMConfig) error { return nil }
func (b *QEMUBackend) StartVM(id string) error { return nil }
func (b *QEMUBackend) StopVM(id string, timeoutSec int) error { return nil }
func (b *QEMUBackend) DeleteVM(id string) error { return nil }
func (b *QEMUBackend) VMStatus(id string) (string, error) { return "", nil }
func (b *QEMUBackend) ShareHostDir(hostPath, guestPath, tag string, readOnly bool) error { return nil }
func (b *QEMUBackend) UnshareHostDir(tag string) error { return nil }
func (b *QEMUBackend) ForwardPort(hostPort, guestPort int, proto string) error { return nil }
func (b *QEMUBackend) RemoveForwardPort(hostPort int, proto string) error { return nil }
func (b *QEMUBackend) Stats(id string) (*VMStats, error) { return nil, nil }
