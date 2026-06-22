package proot

import (
	"testing"
)

// TestNormalizeProotCwd verifies the proot -w argument is sanitized so
// proot does not produce the "<rootfs>/./." chdir warning or fall back
// to "/" silently. Relative paths like "." and "" must become "/", and
// paths missing a leading slash must gain one.
func TestNormalizeProotCwd(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "/"},
		{".", "/"},
		{"./", "/"},
		{"/", "/"},
		{"/root", "/root"},
		{"/app/bin", "/app/bin"},
		{"app/bin", "/app/bin"},
		{"./app", "/app"},
		{"//double", "/double"},
	}
	for _, tc := range cases {
		got := normalizeProotCwd(tc.in)
		if got != tc.want {
			t.Errorf("normalizeProotCwd(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestParseUser verifies the user string parser handles various formats
// including "uid:gid", "uid", and empty strings.
func TestParseUser(t *testing.T) {
	cases := []struct {
		in      string
		wantUID int
		wantGID int
	}{
		{"", -1, -1},
		{"0", 0, 0},
		{"1000", 1000, 1000},
		{"1000:1000", 1000, 1000},
		{"0:0", 0, 0},
		{"invalid", -1, -1},
		{"1000:invalid", 1000, 1000},
		{"invalid:1000", -1, -1},
	}
	for _, tc := range cases {
		uid, gid := parseUser(tc.in)
		if uid != tc.wantUID || gid != tc.wantGID {
			t.Errorf("parseUser(%q) = (%d, %d), want (%d, %d)",
				tc.in, uid, gid, tc.wantUID, tc.wantGID)
		}
	}
}
