package cli

import (
	"reflect"
	"strings"
	"testing"
)

// TestValidatePublishSpec covers every accepted -p/--publish format and the
// rejection of malformed ones.
func TestValidatePublishSpec(t *testing.T) {
	valid := []string{
		"80",                         // containerPort
		"80/tcp",                     // containerPort/proto
		"8080:80",                    // hostPort:containerPort
		"8080:80/udp",                // hostPort:containerPort/proto
		"8080-8090:80",               // hostRange:containerPort
		"127.0.0.1:8080:80",          // ip:hostPort:containerPort
		"127.0.0.1:8080-8090:80",     // ip:hostRange:containerPort
		"127.0.0.1:8080-8090:80/tcp", // ip:hostRange:containerPort/proto
		"192.168.1.10:8080:80/sctp",  // ip:hostPort:containerPort/proto
		"[::1]:8080:80",              // ipv6:hostPort:containerPort
		"[::1]:8080-8090:80/udp",     // ipv6:hostRange:containerPort/proto
		"0:80",                       // host port 0 = ephemeral
		"1.2.3.4:0-1024:80",          // host range starting at 0
	}
	for _, s := range valid {
		if err := ValidatePublishSpec(s); err != nil {
			t.Errorf("ValidatePublishSpec(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"",                    // empty
		":",                   // empty fields
		":80",                 // empty host port
		"8080:",               // empty container port
		"abc",                 // not a port
		"8080:abc",            // container port not a number
		"8080:0",              // container port 0
		"8080:70000",          // out of range
		"70000:80",            // host port out of range
		"8090-8080:80",        // reversed range
		"8080-:80",            // open-ended range
		"-80:80",              // negative port
		"8080:80/icmp",        // bad protocol
		"8080:80/tcp/udp",     // two protocols
		"8080:80/",            // empty protocol
		"1.2.3.4.5:8080:80",   // invalid IP
		"1:2:3:4",             // too many fields
		"8080:8081:80:81",     // too many fields
		"[::1:8080:80",        // unterminated IPv6
		"[not-an-ip]:8080:80", // invalid IPv6
		"[::1]8080:80",        // missing separator after IPv6
		"8080-8090",           // range without container port
	}
	for _, s := range invalid {
		if err := ValidatePublishSpec(s); err == nil {
			t.Errorf("ValidatePublishSpec(%q) = nil, want error", s)
		}
	}
}

// TestValidateExposeSpec covers the "port" and "port/proto" forms.
func TestValidateExposeSpec(t *testing.T) {
	valid := []string{"80", "80/tcp", "53/udp", "3868/sctp", "65535"}
	for _, s := range valid {
		if err := ValidateExposeSpec(s); err != nil {
			t.Errorf("ValidateExposeSpec(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"", "abc", "0", "70000", "80/icmp", "80/tcp/udp", "8080-8090", "80/", "80:90"}
	for _, s := range invalid {
		if err := ValidateExposeSpec(s); err == nil {
			t.Errorf("ValidateExposeSpec(%q) = nil, want error", s)
		}
	}
}

// TestNormalizeArgsShortFlags covers bundles and attached values for the
// per-command grammars.
func TestNormalizeArgsShortFlags(t *testing.T) {
	cases := []struct {
		name string
		spec ShortFlags
		in   []string
		want []string
	}{
		{"run attached publish", RunShortFlags, []string{"-p8080:80", "img"}, []string{"-p", "8080:80", "img"}},
		{"run attached env", RunShortFlags, []string{"-eFOO=1", "img"}, []string{"-e", "FOO=1", "img"}},
		{"run attached volume", RunShortFlags, []string{"-v/path:/path", "img"}, []string{"-v", "/path:/path", "img"}},
		{"run bundle", RunShortFlags, []string{"-itd", "img"}, []string{"-i", "-t", "-d", "img"}},
		{"run bundle with value at end", RunShortFlags, []string{"-itp", "8080:80", "img"}, []string{"-i", "-t", "-p", "8080:80", "img"}},
		{"rm boolean bundle", RmShortFlags, []string{"-fv", "c1"}, []string{"-f", "-v", "c1"}},
		{"ps boolean bundle", PsShortFlags, []string{"-aq", "img"}, []string{"-a", "-q", "img"}},
		{"logs bundle with value", LogsShortFlags, []string{"-ftn5", "c1"}, []string{"-f", "-t", "-n", "5", "c1"}},
		{"kill attached signal", KillShortFlags, []string{"-sKILL", "c1"}, []string{"-s", "KILL", "c1"}},
		{"command tail untouched", ExecShortFlags, []string{"-it", "c1", "sh", "-lc", "x"}, []string{"-i", "-t", "c1", "sh", "-lc", "x"}},
		{"stdin positional", CpShortFlags, []string{"-", "c1:/x"}, []string{"-", "c1:/x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeArgs(tc.in, tc.spec)
			if err != nil {
				t.Fatalf("NormalizeArgs(%v) error = %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestNormalizeArgsErrors pins down explicit errors for unparseable tokens.
func TestNormalizeArgsErrors(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"-zq", "img"}, "unknown short flag"},
		{[]string{"-p=", "img"}, "empty value"},
		{[]string{"-v"}, ""}, // value from next argument: valid
	}
	for _, tc := range cases {
		_, err := NormalizeArgs(tc.in, RunShortFlags)
		if tc.want == "" {
			if err != nil {
				t.Errorf("NormalizeArgs(%v) error = %v, want nil", tc.in, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("NormalizeArgs(%v) error = %v, want containing %q", tc.in, err, tc.want)
		}
	}
}

// TestSplitFlag checks the shared name/value extraction used by every flag
// helper so the "--key=value" and "--key value" forms match uniformly.
func TestSplitFlag(t *testing.T) {
	cases := []struct {
		in       string
		name     string
		value    string
		hasValue bool
	}{
		{"--format", "--format", "", false},
		{"--format=json", "--format", "json", true},
		{"-t=5", "-t", "5", true},
		{"-eFOO=1", "-eFOO", "1", true},
		{"positional=x", "positional=x", "", false},
		{"-", "-", "", false},
	}
	for _, tc := range cases {
		name, value, hasValue := SplitFlag(tc.in)
		if name != tc.name || value != tc.value || hasValue != tc.hasValue {
			t.Errorf("SplitFlag(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, name, value, hasValue, tc.name, tc.value, tc.hasValue)
		}
	}
}

// TestValidateFlags checks that unknown flags are rejected with a clear error
// while values and command arguments are never inspected.
func TestValidateFlags(t *testing.T) {
	valid := []string{"-n", "--tail", "--format"}
	values := []string{"-n", "--tail", "--format"}

	if err := ValidateFlags([]string{"--tail", "5", "c1"}, valid, values); err != nil {
		t.Errorf("ValidateFlags valid input error = %v", err)
	}
	if err := ValidateFlags([]string{"--tail=5", "c1"}, valid, values); err != nil {
		t.Errorf("ValidateFlags key=value input error = %v", err)
	}
	// Command arguments after the first positional are not flags.
	if err := ValidateFlags([]string{"c1", "sh", "-lc"}, valid, values); err != nil {
		t.Errorf("ValidateFlags command tail error = %v", err)
	}
	err := ValidateFlags([]string{"--bogus", "c1"}, valid, values)
	if err == nil || err.Error() != "invalid flag: --bogus" {
		t.Errorf("ValidateFlags unknown flag error = %v, want %q", err, "invalid flag: --bogus")
	}
}

// TestParseRunFlagsErrors pins down the explicit errors of the run parser.
func TestParseRunFlagsErrors(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"invalid publish", []string{"-p", "8080:abc", "img"}, "invalid publish spec"},
		{"invalid publish proto", []string{"-p", "8080:80/icmp", "img"}, "invalid publish spec"},
		{"invalid expose", []string{"--expose", "80/icmp", "img"}, "invalid expose spec"},
		{"unknown flag", []string{"--bogus", "img"}, "invalid flag"},
		{"unknown short flag", []string{"-z", "img"}, "invalid flag"},
		{"missing value", []string{"--name"}, "flag needs an argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := ParseRunFlags(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("ParseRunFlags(%v) error = %v, want containing %q", tc.in, err, tc.want)
			}
		})
	}
}

// TestParseRunFlagsAcceptedForms checks that bundles, attached values and the
// key=value form all reach the flag values.
func TestParseRunFlagsAcceptedForms(t *testing.T) {
	image, cmd, flags, err := ParseRunFlags([]string{"-itp8080:80", "--name=web", "-eFOO=1", "nginx:alpine", "echo", "-it", "hi"})
	if err != nil {
		t.Fatalf("ParseRunFlags error = %v", err)
	}
	if image != "nginx:alpine" {
		t.Errorf("image = %q, want nginx:alpine", image)
	}
	if !flags.Interactive || !flags.TTY {
		t.Errorf("Interactive/TTY = %v/%v, want true/true", flags.Interactive, flags.TTY)
	}
	if flags.Name != "web" {
		t.Errorf("Name = %q, want web", flags.Name)
	}
	if len(flags.Ports) != 1 || flags.Ports[0] != "8080:80" {
		t.Errorf("Ports = %v, want [8080:80]", flags.Ports)
	}
	if len(flags.Env) != 1 || flags.Env[0] != "FOO=1" {
		t.Errorf("Env = %v, want [FOO=1]", flags.Env)
	}
	// Command arguments are preserved verbatim, bundles included.
	if !reflect.DeepEqual(cmd, []string{"echo", "-it", "hi"}) {
		t.Errorf("cmd = %v, want [echo -it hi]", cmd)
	}
}

// TestParseRunFlagsBadNumbers pins down that malformed numeric values are
// explicit errors instead of silently becoming 0.
func TestParseRunFlagsBadNumbers(t *testing.T) {
	flags := []string{
		"--stop-timeout", "--cpus", "--cpu-shares", "--cpu-period",
		"--cpu-quota", "--pids-limit", "--blkio-weight", "--health-retries",
		"--memory", "--memory-swap", "--shm-size",
	}
	for _, f := range flags {
		_, _, _, err := ParseRunFlags([]string{f, "not-a-number", "img"})
		if err == nil || !strings.Contains(err.Error(), "invalid value for "+f) {
			t.Errorf("ParseRunFlags([%s not-a-number img]) error = %v, want invalid value for %s", f, err, f)
		}
	}
	if _, _, _, err := ParseRunFlags([]string{"-m", "512x", "img"}); err == nil {
		t.Error("ParseRunFlags([-m 512x img]) = nil, want error")
	}
}

// TestExpandPublishSpec covers range expansion, IP forms and protocols.
func TestExpandPublishSpec(t *testing.T) {
	one, err := ExpandPublishSpec("8080:80")
	if err != nil || len(one) != 1 || one[0] != (PublishBinding{HostIP: "0.0.0.0", HostPort: "8080", ContainerPort: "80", Proto: "tcp"}) {
		t.Errorf("ExpandPublishSpec(8080:80) = %v, %v", one, err)
	}
	bare, err := ExpandPublishSpec("80")
	if err != nil || len(bare) != 1 || bare[0].ContainerPort != "80" || bare[0].HostPort != "" {
		t.Errorf("ExpandPublishSpec(80) = %v, %v", bare, err)
	}
	rng, err := ExpandPublishSpec("8080-8081:80")
	if err != nil || len(rng) != 2 || rng[0].HostPort != "8080" || rng[1].HostPort != "8081" {
		t.Errorf("ExpandPublishSpec(8080-8081:80) = %v, %v", rng, err)
	}
	ip, err := ExpandPublishSpec("127.0.0.1:8080-8081:80/udp")
	if err != nil || len(ip) != 2 || ip[0].HostIP != "127.0.0.1" || ip[0].Proto != "udp" {
		t.Errorf("ExpandPublishSpec(127.0.0.1:8080-8081:80/udp) = %v, %v", ip, err)
	}
	sctp, err := ExpandPublishSpec("8080:80/sctp")
	if err != nil || len(sctp) != 1 || sctp[0].Proto != "sctp" {
		t.Errorf("ExpandPublishSpec(8080:80/sctp) = %v, %v", sctp, err)
	}
	v6, err := ExpandPublishSpec("[::1]:8080:80")
	if err != nil || len(v6) != 1 || v6[0].HostIP != "::1" {
		t.Errorf("ExpandPublishSpec([::1]:8080:80) = %v, %v", v6, err)
	}
	if _, err := ExpandPublishSpec("8080-8082:80-81"); err == nil {
		t.Error("ExpandPublishSpec(8080-8082:80-81) = nil, want length-mismatch error")
	}
	if _, err := ExpandPublishSpec("8080:80/icmp"); err == nil {
		t.Error("ExpandPublishSpec(8080:80/icmp) = nil, want error")
	}
	pair, err := ExpandPublishSpec("8080-8081:80-81")
	if err != nil || len(pair) != 2 || pair[1].ContainerPort != "81" {
		t.Errorf("ExpandPublishSpec(8080-8081:80-81) = %v, %v", pair, err)
	}
}

// TestNormalizeArgsPerCommand pins down attached values and bundle errors for
// the exec, build, logs, cp, kill and rm grammars.
func TestNormalizeArgsPerCommand(t *testing.T) {
	cases := []struct {
		name string
		spec ShortFlags
		in   []string
		want []string
	}{
		{"exec attached env", ExecShortFlags, []string{"-eFOO=1", "c1", "sh"}, []string{"-e", "FOO=1", "c1", "sh"}},
		{"exec bundle", ExecShortFlags, []string{"-it", "c1", "sh"}, []string{"-i", "-t", "c1", "sh"}},
		{"exec trailing bundle kept", ExecShortFlags, []string{"c1", "sh", "-lc"}, []string{"c1", "sh", "-lc"}},
		{"build attached tag", BuildShortFlags, []string{"-te2e:1", "."}, []string{"-t", "e2e:1", "."}},
		{"logs attached tail", LogsShortFlags, []string{"-n100", "c1"}, []string{"-n", "100", "c1"}},
		{"cp bundle", CpShortFlags, []string{"-aL", "a", "b"}, []string{"-a", "-L", "a", "b"}},
		{"kill attached signal", KillShortFlags, []string{"-sKILL", "c1"}, []string{"-s", "KILL", "c1"}},
		{"rm bundle", RmShortFlags, []string{"-fv", "c1"}, []string{"-f", "-v", "c1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeArgs(tc.in, tc.spec)
			if err != nil {
				t.Fatalf("NormalizeArgs(%v) error = %v", tc.in, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("NormalizeArgs(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name string
		spec ShortFlags
		in   []string
	}{
		{"exec unknown", ExecShortFlags, []string{"-z", "c1"}},
		{"build unknown", BuildShortFlags, []string{"-z", "."}},
		{"logs unknown", LogsShortFlags, []string{"-z", "c1"}},
		{"rm unknown", RmShortFlags, []string{"-z", "c1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeArgs(tc.in, tc.spec); err == nil {
				t.Errorf("NormalizeArgs(%v) = nil, want error", tc.in)
			}
		})
	}
}
