package common

import (
	"os"
	"strings"
	"testing"
)

func TestIsHostEnvDenied(t *testing.T) {
	cases := []struct {
		name  string
		denied bool
	}{
		{"LD_PRELOAD", true},
		{"LD_PRELOAD32", true},
		{"LD_PRELOAD64", true},
		{"LD_LIBRARY_PATH", true},
		{"LD_SHOW_AUXV", true},
		{"TERMUX_VERSION", true},
		{"TERMUX__PREFIX", true},
		{"TERMUX__ROOTFS", true},
		{"TERMUX__HOME", true},
		{"PREFIX", true},
		{"ANDROID_ROOT", true},
		{"ANDROID_DATA", true},
		{"ANDROID_STORAGE", true},
		{"ANDROID_PROPERTY_WORKSPACE", true},
		{"TMPDIR", true},
		{"TMP", true},
		{"TEMP", true},
		{"HOME", true},
		// safe
		{"PATH", false},
		{"USER", false},
		{"SHELL", false},
		{"FOO", false},
		{"", false},
		{"_", false},
	}
	for _, c := range cases {
		if got := isHostEnvDenied(c.name); got != c.denied {
			t.Errorf("isHostEnvDenied(%q) = %v, want %v", c.name, got, c.denied)
		}
	}
}

func TestStripHostEnv_Termux(t *testing.T) {
	in := []string{
		"TERMUX_VERSION=0.118.3",
		"LD_PRELOAD=/data/data/com.termux/files/usr/lib/libtermux-exec-ld-preload.so",
		"ANDROID_DATA=/data",
		"ANDROID_ROOT=/system",
		"TERMUX__PREFIX=/data/data/com.termux/files/usr",
		"TERMUX__ROOTFS=/data/data/com.termux/files",
		"TERMUX__HOME=/data/data/com.termux/files/home",
		"PREFIX=/data/data/com.termux/files/usr",
		"LD_PRELOAD32=",
		"LD_PRELOAD64=",
		"LD_LIBRARY_PATH=/data/data/com.termux/files/usr/lib",
		"LD_SHOW_AUXV=1",
		"TMPDIR=/data/data/com.termux/files/usr/tmp",
		"TMP=/data/data/com.termux/files/usr/tmp",
		"TEMP=/data/data/com.termux/files/usr/tmp",
		"HOME=/data/data/com.termux/files/home",
		"ANDROID_STORAGE=/storage",
		"ANDROID_PROPERTY_WORKSPACE=/data/property",
		// safe
		"PATH=/usr/bin:/bin",
		"USER=u0_a276",
		"FOO=bar",
	}
	out := StripHostEnv(in)
	denied := []string{
		"TERMUX_VERSION", "LD_PRELOAD", "ANDROID_DATA", "ANDROID_ROOT",
		"TERMUX__PREFIX", "TERMUX__ROOTFS", "TERMUX__HOME", "PREFIX",
		"LD_PRELOAD32", "LD_PRELOAD64", "LD_LIBRARY_PATH", "LD_SHOW_AUXV",
		"TMPDIR", "TMP", "TEMP", "HOME", "ANDROID_STORAGE",
		"ANDROID_PROPERTY_WORKSPACE",
	}
	for _, d := range denied {
		for _, e := range out {
			if strings.HasPrefix(e, d+"=") {
				t.Errorf("StripHostEnv did not strip %q (found %q in %v)", d, e, out)
			}
		}
	}
	// safe vars must survive
	safeFound := map[string]bool{"PATH": false, "USER": false, "FOO": false}
	for _, e := range out {
		for k := range safeFound {
			if strings.HasPrefix(e, k+"=") {
				safeFound[k] = true
			}
		}
	}
	for k, found := range safeFound {
		if !found {
			t.Errorf("StripHostEnv lost safe var %q (out=%v)", k, out)
		}
	}
}

func TestStripHostEnv_KeepsSafe(t *testing.T) {
	in := []string{
		"PATH=/usr/bin:/bin",
		"USER=root",
		"SHELL=/bin/sh",
		"FOO=bar=baz",
		"=orphan",
		"noequals",
	}
	out := StripHostEnv(in)
	if len(out) != len(in) {
		t.Errorf("expected all %d safe entries to survive, got %d (out=%v)", len(in), len(out), out)
	}
}

func TestAndroidEnv(t *testing.T) {
	env := AndroidEnv()
	if len(env) == 0 {
		t.Fatal("AndroidEnv() returned empty slice")
	}
	want := map[string]string{
		"PATH":   "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME":   "/root",
		"LANG":   "C.UTF-8",
		"LC_ALL": "C.UTF-8",
		"TMPDIR": "/tmp",
	}
	for _, e := range env {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			t.Errorf("malformed env entry: %q", e)
			continue
		}
		k, v := e[:eq], e[eq+1:]
		if want[k] != "" && want[k] != v {
			t.Errorf("AndroidEnv()[%q] = %q, want %q", k, v, want[k])
		}
	}
	for k := range want {
		found := false
		for _, e := range env {
			if strings.HasPrefix(e, k+"=") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AndroidEnv() missing %q", k)
		}
	}
}

func TestIsTermux(t *testing.T) {
	// Save and restore env.
	for _, k := range []string{"TERMUX_VERSION", "PREFIX", "TERMUX__PREFIX"} {
		old, had := os.LookupEnv(k)
		defer func(k, old string, had bool) {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		}(k, old, had)
	}
	os.Unsetenv("TERMUX_VERSION")
	os.Unsetenv("TERMUX__PREFIX")
	os.Unsetenv("PREFIX")
	if IsTermux() {
		t.Error("IsTermux() = true with all vars unset, want false")
	}
	os.Setenv("TERMUX_VERSION", "0.118.3")
	if !IsTermux() {
		t.Error("IsTermux() = false with TERMUX_VERSION set, want true")
	}
	os.Unsetenv("TERMUX_VERSION")
	os.Setenv("PREFIX", "/data/data/com.termux/files/usr")
	if !IsTermux() {
		t.Error("IsTermux() = false with PREFIX=/data/data/com.termux, want true")
	}
	os.Unsetenv("PREFIX")
	os.Setenv("TERMUX__PREFIX", "/data/data/com.termux/files/usr")
	if !IsTermux() {
		t.Error("IsTermux() = false with TERMUX__PREFIX set, want true")
	}
}

func TestTermuxPrefix(t *testing.T) {
	for _, k := range []string{"TERMUX__PREFIX", "PREFIX"} {
		old, had := os.LookupEnv(k)
		defer func(k, old string, had bool) {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		}(k, old, had)
	}
	os.Unsetenv("TERMUX__PREFIX")
	os.Unsetenv("PREFIX")
	if got := TermuxPrefix(); got != "/data/data/com.termux/files/usr" {
		t.Errorf("TermuxPrefix() with no env = %q, want canonical fallback", got)
	}
	os.Setenv("PREFIX", "/data/data/com.termux/files/usr/")
	if got := TermuxPrefix(); got != "/data/data/com.termux/files/usr" {
		t.Errorf("TermuxPrefix() with trailing slash = %q, want no trailing slash", got)
	}
	os.Setenv("TERMUX__PREFIX", "/custom/termux/prefix")
	if got := TermuxPrefix(); got != "/custom/termux/prefix" {
		t.Errorf("TermuxPrefix() with TERMUX__PREFIX = %q, want override", got)
	}
}

func TestStripHostEnvFromOS(t *testing.T) {
	// This test mutates the process env. We restore it manually.
	for _, k := range []string{"LD_PRELOAD", "TERMUX_VERSION"} {
		old, had := os.LookupEnv(k)
		defer func(k, old string, had bool) {
			if had {
				os.Setenv(k, old)
			} else {
				os.Unsetenv(k)
			}
		}(k, old, had)
	}
	os.Setenv("LD_PRELOAD", "/fake/lib.so")
	os.Setenv("TERMUX_VERSION", "0.118")
	os.Setenv("PATH", "/usr/bin")
	out := StripHostEnvFromOS()
	// After the call, the process env must no longer contain the denied vars.
	if v := os.Getenv("LD_PRELOAD"); v != "" {
		t.Errorf("LD_PRELOAD leaked through StripHostEnvFromOS: %q", v)
	}
	if v := os.Getenv("TERMUX_VERSION"); v != "" {
		t.Errorf("TERMUX_VERSION leaked through StripHostEnvFromOS: %q", v)
	}
	// PATH must survive
	if v := os.Getenv("PATH"); v != "/usr/bin" {
		t.Errorf("PATH was clobbered by StripHostEnvFromOS: %q", v)
	}
	// The returned slice must not contain the denied vars either.
	for _, e := range out {
		if strings.HasPrefix(e, "LD_PRELOAD=") || strings.HasPrefix(e, "TERMUX_VERSION=") {
			t.Errorf("StripHostEnvFromOS returned denied var: %q", e)
		}
	}
}
