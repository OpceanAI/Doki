package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/OpceanAI/Doki/pkg/common"
)

// newTestDaemon starts an HTTP server on a unix socket that serves the given
// handler, and returns a *DokiCLI wired to it plus a cleanup function.
func newTestDaemon(t *testing.T, h http.Handler) (*DokiCLI, func()) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "dokid.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	return New(sock), func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

// TestAPIVersionMatchesDaemon confirms the client's NegotiateVersion reports
// the same API version the daemon advertises. The daemon is simulated with the
// exact payload common.GetVersion() serves on /version.
func TestAPIVersionMatchesDaemon(t *testing.T) {
	c, cleanup := newTestDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(common.GetVersion())
	}))
	defer cleanup()

	got, err := c.NegotiateVersion()
	if err != nil {
		t.Fatalf("NegotiateVersion() error = %v", err)
	}
	if got != common.DokiAPIVersion {
		t.Errorf("NegotiateVersion() = %q, want %q", got, common.DokiAPIVersion)
	}
}

// TestNegotiateVersionMismatch confirms a daemon reporting a different API
// version is rejected rather than silently ignored.
func TestNegotiateVersionMismatch(t *testing.T) {
	c, cleanup := newTestDaemon(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.00"})
	}))
	defer cleanup()

	if _, err := c.NegotiateVersion(); err == nil {
		t.Error("NegotiateVersion() expected an error for a mismatched API version, got nil")
	}
}

func TestExpandArgv(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"bundle", []string{"-it", "alpine"}, []string{"-i", "-t", "alpine"}},
		{"bundle three", []string{"-itd", "alpine"}, []string{"-i", "-t", "-d", "alpine"}},
		{"bundle reverse", []string{"-ti", "alpine"}, []string{"-t", "-i", "alpine"}},
		{"bundle with trailing value flag", []string{"-itp", "8080:80", "alpine"}, []string{"-i", "-t", "-p", "8080:80", "alpine"}},
		{"attached publish", []string{"-p8080:80", "alpine"}, []string{"-p", "8080:80", "alpine"}},
		{"attached env", []string{"-eFOO=1", "alpine"}, []string{"-e", "FOO=1", "alpine"}},
		{"attached volume", []string{"-v/path:/path", "alpine"}, []string{"-v", "/path:/path", "alpine"}},
		{"attached equals", []string{"-m=64m", "alpine"}, []string{"-m", "64m", "alpine"}},
		{"explicit boolean", []string{"-t=true", "alpine"}, []string{"-t=true", "alpine"}},
		{"long flags untouched", []string{"--env", "X=1", "alpine"}, []string{"--env", "X=1", "alpine"}},
		{"single flags", []string{"-i", "-t", "alpine"}, []string{"-i", "-t", "alpine"}},
		{"command args untouched", []string{"-i", "alpine", "sh", "-zq"}, []string{"-i", "alpine", "sh", "-zq"}},
		{"after double dash untouched", []string{"--", "-zq"}, []string{"--", "-zq"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandArgv(tc.in)
			if err != nil {
				t.Fatalf("ExpandArgv(%v) error = %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ExpandArgv(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestExpandArgvErrors pins down that unparseable short-flag tokens produce an
// explicit error instead of being silently passed through.
func TestExpandArgvErrors(t *testing.T) {
	cases := [][]string{
		{"-zq", "alpine"},   // unknown letters
		{"-p=", "alpine"},   // empty attached value
		{"-i", "-z", "img"}, // unknown single short flag
	}
	for _, in := range cases {
		if got, err := ExpandArgv(in); err == nil {
			t.Errorf("ExpandArgv(%v) = %v, want error", in, got)
		}
	}
}
