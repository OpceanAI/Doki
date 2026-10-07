//go:build !linux

package seccomp

import "fmt"

// ErrUnsupported is returned by every enforcement entry point on non-Linux
// kernels. Enforcement is reported as unavailable rather than pretended.
var ErrUnsupported = fmt.Errorf("seccomp enforcement is only implemented on Linux")

// Applied always reports false on non-Linux kernels: no filter is ever
// installed there, so callers must not claim seccomp is active.
func Applied() bool { return false }

// Supported always reports false on non-Linux kernels.
func Supported() bool { return false }

// ApplyToSelf is not implemented outside Linux and always fails honestly.
func ApplyToSelf(*Profile) error { return ErrUnsupported }
