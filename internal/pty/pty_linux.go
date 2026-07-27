//go:build linux

// Package pty provides a minimal pseudo-terminal implementation built directly
// on Linux ioctls via golang.org/x/sys/unix, with zero new dependencies
// (creack/pty is intentionally NOT used — it is not vendored).
package pty

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// Open allocates a new pseudo-terminal pair and returns the master (ptmx) and
// the slave (pts) as *os.File. The caller wires the slave to a child process's
// stdin/stdout/stderr and reads/writes the master.
func Open() (master, slave *os.File, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("open /dev/ptmx: %w", err)
	}
	// Unlock the slave (TIOCSPTLCK = 0).
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = m.Close()
		return nil, nil, fmt.Errorf("unlockpt: %w", err)
	}
	// Resolve the slave pts number (TIOCGPTN).
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = m.Close()
		return nil, nil, fmt.Errorf("ptsname: %w", err)
	}
	sname := fmt.Sprintf("/dev/pts/%d", n)
	s, err := os.OpenFile(sname, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = m.Close()
		return nil, nil, fmt.Errorf("open %s: %w", sname, err)
	}
	return m, s, nil
}

// Setsize sets the window size of the pty referenced by the master file.
func Setsize(master *os.File, rows, cols uint16) error {
	return unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{
		Row: rows,
		Col: cols,
	})
}
