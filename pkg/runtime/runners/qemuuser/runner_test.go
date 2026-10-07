package qemuuser

import (
	"os"
	"strings"
	"testing"
)

func TestArchMap(t *testing.T) {
	cases := map[string]string{
		"amd64": "x86_64", "386": "i386", "arm64": "aarch64", "arm": "arm",
	}
	for in, want := range cases {
		if got := archMap(in); got != want {
			t.Errorf("archMap(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildQEMUArgsUsesQEMU(t *testing.T) {
	args := buildQEMUArgs("/rootfs", "aarch64", "/bin/sh", []string{"-c", "hi"}, []string{"LD_LIBRARY_PATH=/usr/lib"})
	joined := strings.Join(args, " ")
	for _, want := range []string{"-L", "/rootfs", "-cpu", "-0", "/bin/sh"} {
		if !strings.Contains(joined, want) {
			t.Errorf("buildQEMUArgs missing %q in %q", want, joined)
		}
	}
	if !strings.Contains(joined, "LD_LIBRARY_PATH") {
		t.Errorf("buildQEMUArgs should pass -E LD_LIBRARY_PATH, got %q", joined)
	}
}

func TestBuildQEMUArgsArgv0(t *testing.T) {
	args := buildQEMUArgs("/rootfs", "x86_64", "/bin/echo", []string{"hi"}, nil)
	found := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-0" && args[i+1] == "/bin/echo" {
			found = true
		}
	}
	if !found {
		t.Errorf("buildQEMUArgs missing -0 argv0, got %v", args)
	}
}

func TestHostArchNonEmpty(t *testing.T) {
	if hostArch() == "" {
		t.Fatal("hostArch() empty")
	}
	if os.Getenv("GOARCH") == "" {
		t.Skip("no GOARCH env")
	}
}
