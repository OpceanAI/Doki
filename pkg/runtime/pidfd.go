//go:build linux

package runtime

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

type PidfdMonitor struct {
	pidfd int
	pid   int
	mu    sync.Mutex
	closed bool
}

func NewPidfdMonitor(pid int) (*PidfdMonitor, error) {
	fd, err := pidfdOpen(pid, 0)
	if err != nil {
		return nil, fmt.Errorf("pidfd_open(%d): %w", pid, err)
	}

	return &PidfdMonitor{
		pidfd: fd,
		pid:   pid,
	}, nil
}

func pidfdOpen(pid int, flags uint) (int, error) {
	fd, _, errno := unix.Syscall(unix.SYS_PIDFD_OPEN, uintptr(pid), uintptr(flags), 0)
	if errno != 0 {
		return 0, errno
	}
	return int(fd), nil
}

func (m *PidfdMonitor) Wait() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, fmt.Errorf("monitor already closed")
	}

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return 0, fmt.Errorf("epoll_create1: %w", err)
	}
	defer func() { _ = unix.Close(epfd) }()

	event := unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(m.pidfd),
	}
	if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, m.pidfd, &event); err != nil {
		return 0, fmt.Errorf("epoll_ctl: %w", err)
	}

	events := make([]unix.EpollEvent, 1)
	for {
		n, err := unix.EpollWait(epfd, events, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return 0, fmt.Errorf("epoll_wait: %w", err)
		}
		if n > 0 {
			break
		}
	}

	var status unix.WaitStatus
	err = unix.Waitid(unix.P_PIDFD, m.pidfd, nil, unix.WEXITED, nil)
	if err != nil {
		return 0, fmt.Errorf("waitid: %w", err)
	}

	_ = status
	exitCode := m.getExitCode()

	m.close()
	return exitCode, nil
}

func (m *PidfdMonitor) getExitCode() int {
	var siginfo [128]byte
	ptr := unsafe.Pointer(&siginfo[0])
	_, _, errno := unix.Syscall6(247,
		uintptr(unix.P_PIDFD),
		uintptr(m.pidfd),
		uintptr(ptr),
		uintptr(unix.WEXITED),
		0, 0)
	if errno != 0 {
		return -1
	}
	return int(siginfo[16])
}

func (m *PidfdMonitor) Signal(sig syscall.Signal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("monitor already closed")
	}

	return pidfdSendSignal(m.pidfd, sig)
}

func pidfdSendSignal(pidfd int, sig syscall.Signal) error {
	_, _, errno := unix.Syscall(424,
		uintptr(pidfd),
		uintptr(sig),
		0)
	if errno != 0 {
		return errno
	}
	return nil
}

func (m *PidfdMonitor) PidFD() int {
	return m.pidfd
}

func (m *PidfdMonitor) PID() int {
	return m.pid
}

func (m *PidfdMonitor) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.close()
	return nil
}

func (m *PidfdMonitor) close() {
	if !m.closed {
		_ = unix.Close(m.pidfd)
		m.closed = true
	}
}

func PidfdAvailable() bool {
	fd, err := pidfdOpen(os.Getpid(), 0)
	if err != nil {
		return false
	}
	_ = unix.Close(fd)
	return true
}

type PidfdProcessGroup struct {
	monitors map[int]*PidfdMonitor
	mu       sync.RWMutex
}

func NewPidfdProcessGroup() *PidfdProcessGroup {
	return &PidfdProcessGroup{
		monitors: make(map[int]*PidfdMonitor),
	}
}

func (pg *PidfdProcessGroup) Track(pid int) error {
	mon, err := NewPidfdMonitor(pid)
	if err != nil {
		return err
	}

	pg.mu.Lock()
	pg.monitors[pid] = mon
	pg.mu.Unlock()

	return nil
}

func (pg *PidfdProcessGroup) Signal(pid int, sig syscall.Signal) error {
	pg.mu.RLock()
	mon, ok := pg.monitors[pid]
	pg.mu.RUnlock()

	if !ok {
		return fmt.Errorf("pid %d not tracked", pid)
	}

	return mon.Signal(sig)
}

func (pg *PidfdProcessGroup) SignalAll(sig syscall.Signal) []error {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	var errs []error
	for pid, mon := range pg.monitors {
		if err := mon.Signal(sig); err != nil {
			errs = append(errs, fmt.Errorf("pid %d: %w", pid, err))
		}
	}
	return errs
}

func (pg *PidfdProcessGroup) Remove(pid int) {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	if mon, ok := pg.monitors[pid]; ok {
		_ = mon.Close()
		delete(pg.monitors, pid)
	}
}

func (pg *PidfdProcessGroup) CloseAll() {
	pg.mu.Lock()
	defer pg.mu.Unlock()

	for pid, mon := range pg.monitors {
		_ = mon.Close()
		delete(pg.monitors, pid)
	}
}

func (pg *PidfdProcessGroup) Tracked() []int {
	pg.mu.RLock()
	defer pg.mu.RUnlock()

	pids := make([]int, 0, len(pg.monitors))
	for pid := range pg.monitors {
		pids = append(pids, pid)
	}
	return pids
}
