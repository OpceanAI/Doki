package cli

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ShortFlags describes the short-flag grammar of one command: which letters
// take no value (and may be bundled, e.g. "-it") and which take a value that is
// either attached to the letter (e.g. "-p8080:80", "-eFOO=1", "-v/path:/path")
// or supplied as the next argument (e.g. "-p 8080:80").
type ShortFlags struct {
	Bool  string // letters that take no value
	Value string // letters that take a value
}

// Short-flag grammars for the commands whose argv is normalized. The same
// letter can be boolean in one command and value-taking in another (e.g. -v is
// --volumes in rm and --volume in run), so the grammar is per command.
var (
	// RunShortFlags is the grammar of run/create/update: -i, -t, -d, -P and
	// -x take no value; -a, -c, -e, -h, -l, -m, -n, -p, -u, -v and -w take a
	// value that may be attached ("-p8080:80", "-eFOO=1", "-v/path:/path").
	RunShortFlags = ShortFlags{Bool: "itdPx", Value: "acehlmnpuvw"}
	// ExecShortFlags is the grammar of exec (-i, -t, -d / -e, -u, -w).
	ExecShortFlags = ShortFlags{Bool: "itd", Value: "euw"}
	// LogsShortFlags is the grammar of logs (-f, -t / -n, -s).
	LogsShortFlags = ShortFlags{Bool: "ft", Value: "ns"}
	// CpShortFlags is the grammar of cp (-a, -L).
	CpShortFlags = ShortFlags{Bool: "aL"}
	// BuildShortFlags is the grammar of build (-q / -t, -f, -m).
	BuildShortFlags = ShortFlags{Bool: "q", Value: "tfm"}
	// KillShortFlags is the grammar of kill (-s).
	KillShortFlags = ShortFlags{Value: "s"}
	// RmShortFlags is the grammar of rm (-f, -v).
	RmShortFlags = ShortFlags{Bool: "fv"}
	// PsShortFlags is the grammar of ps (-a, -q / -n, -f).
	PsShortFlags = ShortFlags{Bool: "aq", Value: "nf"}
	// NoShortFlags is the grammar of commands without short flags
	// (attach, stats, top, rename).
	NoShortFlags = ShortFlags{}
)

// SplitFlag splits a "--key=value" or "-k=value" token into its name and
// attached value. hasValue reports whether the token carried an attached
// value. Tokens without an attached value are returned unchanged as name.
// All argv helpers (ValidateFlags, flagBool, cleanIDs, ...) use this helper so
// the "key=value" and "key value" forms are treated uniformly everywhere.
func SplitFlag(arg string) (name, value string, hasValue bool) {
	if eq := strings.Index(arg, "="); eq >= 0 && strings.HasPrefix(arg, "-") {
		return arg[:eq], arg[eq+1:], true
	}
	return arg, "", false
}

// expandShortFlags splits one short-flag token into individual tokens:
//
//	-it          -> -i -t
//	-p8080:80    -> -p 8080:80
//	-eFOO=1      -> -e FOO=1
//	-v/path:/path -> -v /path:/path
//	-itp         -> -i -t -p   (value comes from the next argument)
//	-t=true      -> -t=true    (explicit boolean value)
//
// It returns an explicit error for tokens it cannot parse (unknown letters,
// empty attached values) instead of silently passing them through.
func expandShortFlags(arg string, spec ShortFlags) ([]string, error) {
	if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
		return []string{arg}, nil
	}
	isBool := func(c byte) bool { return strings.IndexByte(spec.Bool, c) >= 0 }
	isValue := func(c byte) bool { return strings.IndexByte(spec.Value, c) >= 0 }

	out := make([]string, 0, len(arg))
	for j := 1; j < len(arg); j++ {
		c := arg[j]
		rest := arg[j+1:]
		switch {
		case isBool(c):
			if strings.HasPrefix(rest, "=") {
				// Explicit boolean value: keep "-t=true" as one token; the
				// flag helpers understand the key=value form.
				return append(out, "-"+string(c)+rest), nil
			}
			out = append(out, "-"+string(c))
		case isValue(c):
			hadEq := strings.HasPrefix(rest, "=")
			rest = strings.TrimPrefix(rest, "=")
			if rest == "" && hadEq {
				return nil, fmt.Errorf("invalid flag: %s (empty value for -%c)", arg, c)
			}
			out = append(out, "-"+string(c))
			if rest != "" {
				out = append(out, rest)
			}
			return out, nil
		default:
			return nil, fmt.Errorf("invalid flag: %s (unknown short flag -%c)", arg, c)
		}
	}
	return out, nil
}

// ExpandArgv expands bundled and attached short flags in the flag section of
// args (everything before the first positional argument or "--") using the
// run-style grammar. Command arguments are left untouched, so tokens such as
// the "-lc" of `doki exec c1 sh -lc x` are never rewritten. Tokens that cannot
// be parsed produce an explicit error.
func ExpandArgv(args []string) ([]string, error) {
	return NormalizeArgs(args, RunShortFlags)
}

// NormalizeArgs normalizes the flag section of args (everything before the
// first positional argument or "--") for the given short-flag grammar:
//
//   - bundled boolean short flags ("-it") are split into individual flags;
//   - short flags with attached values ("-p8080:80", "-eFOO=1") are split
//     into flag and value.
//
// Long flags are passed through unchanged: every flag helper accepts both the
// "--key=value" and "--key value" forms (see SplitFlag), so no rewriting is
// needed and boolean flags such as "--pull=false" keep their explicit value.
// Everything from the first positional argument on is passed through verbatim
// (it is the container command), as is everything after "--". Tokens that
// cannot be parsed produce an explicit error.
func NormalizeArgs(args []string, spec ShortFlags) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]

		// "--" stops flag parsing; the rest is the command.
		if a == "--" {
			out = append(out, args[i:]...)
			return out, nil
		}

		// First positional argument: everything after it is the command.
		if a == "-" || !strings.HasPrefix(a, "-") {
			out = append(out, args[i:]...)
			return out, nil
		}

		// Long flags: leave untouched.
		if strings.HasPrefix(a, "--") {
			out = append(out, a)
			continue
		}

		expanded, err := expandShortFlags(a, spec)
		if err != nil {
			return nil, err
		}
		out = append(out, expanded...)
	}
	return out, nil
}

// validProtocols are the transport protocols accepted by -p/--publish and
// --expose.
var validProtocols = map[string]bool{"tcp": true, "udp": true, "sctp": true}

// splitPortProto splits "8080:80/tcp" into "8080:80" and "tcp". When no
// "/proto" suffix is present the protocol defaults to "tcp".
func splitPortProto(s string) (spec, proto string, err error) {
	spec, proto = s, "tcp"
	if i := strings.LastIndex(s, "/"); i >= 0 {
		spec, proto = s[:i], s[i+1:]
		if !validProtocols[proto] {
			return "", "", fmt.Errorf("invalid protocol %q (must be tcp, udp or sctp)", proto)
		}
	}
	return spec, proto, nil
}

// validatePort validates a single port number (1-65535, or 0 when allowZero).
func validatePort(p string, allowZero bool) error {
	n, err := strconv.Atoi(p)
	if err != nil {
		return fmt.Errorf("invalid port %q", p)
	}
	if n == 0 && allowZero {
		return nil
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("port %q out of range (1-65535)", p)
	}
	return nil
}

// validatePortSpec validates "8080" or "8080-8090".
func validatePortSpec(p string, allowZero bool) error {
	if start, end, ok := strings.Cut(p, "-"); ok {
		if err := validatePort(start, allowZero); err != nil {
			return err
		}
		if err := validatePort(end, allowZero); err != nil {
			return err
		}
		a, _ := strconv.Atoi(start)
		b, _ := strconv.Atoi(end)
		if a > b {
			return fmt.Errorf("invalid port range %q (start greater than end)", p)
		}
		return nil
	}
	return validatePort(p, allowZero)
}

// ValidatePublishSpec validates a -p/--publish specification. Accepted
// formats (optionally suffixed with "/tcp", "/udp" or "/sctp"):
//
//	containerPort                     80
//	hostPort:containerPort            8080:80
//	hostRange:containerPort           8080-8090:80
//	ip:hostPort:containerPort         127.0.0.1:8080:80
//	ip:hostRange:containerPort        127.0.0.1:8080-8090:80
//	[ipv6]:hostPort:containerPort     [::1]:8080:80
//
// Container ports must be 1-65535; host ports may be 0 (ephemeral).
func ValidatePublishSpec(s string) error {
	spec, _, err := splitPortProto(s)
	if err != nil {
		return fmt.Errorf("invalid publish spec %q: %w", s, err)
	}
	if spec == "" {
		return fmt.Errorf("invalid publish spec %q: empty port specification", s)
	}

	bad := func(format string, args ...interface{}) error {
		return fmt.Errorf("invalid publish spec %q: %s", s, fmt.Sprintf(format, args...))
	}

	// Bracketed IPv6 address: [::1]:hostPort:containerPort.
	if strings.HasPrefix(spec, "[") {
		end := strings.Index(spec, "]")
		if end < 0 {
			return bad("missing closing bracket in IPv6 address")
		}
		ip := spec[1:end]
		if net.ParseIP(ip) == nil {
			return bad("invalid IPv6 address %q", ip)
		}
		rest := spec[end+1:]
		if !strings.HasPrefix(rest, ":") {
			return bad("expected ':hostPort:containerPort' after IPv6 address")
		}
		return validateHostContainerPorts(rest[1:], bad)
	}

	fields := strings.Split(spec, ":")
	switch len(fields) {
	case 1:
		// containerPort
		if err := validatePort(fields[0], false); err != nil {
			return bad("%v", err)
		}
	case 2:
		// hostPort:containerPort (host side may be a range)
		return validateHostContainerPorts(spec, bad)
	case 3:
		// ip:hostPort:containerPort
		if net.ParseIP(fields[0]) == nil {
			return bad("invalid IP address %q", fields[0])
		}
		return validateHostContainerPorts(fields[1]+":"+fields[2], bad)
	default:
		return bad("expected [ip:]hostPort:containerPort[/proto]")
	}
	return nil
}

// validateHostContainerPorts validates the "hostPort:containerPort" tail of a
// publish specification; the host side may be a port range.
func validateHostContainerPorts(spec string, bad func(string, ...interface{}) error) error {
	parts := strings.Split(spec, ":")
	if len(parts) != 2 {
		return bad("expected 'hostPort:containerPort'")
	}
	if err := validatePortSpec(parts[0], true); err != nil {
		return bad("%v", err)
	}
	if err := validatePortSpec(parts[1], false); err != nil {
		return bad("%v", err)
	}
	return nil
}

// ValidateExposeSpec validates an --expose specification: "port" or
// "port/proto" where proto is tcp, udp or sctp.
func ValidateExposeSpec(s string) error {
	spec, _, err := splitPortProto(s)
	if err != nil {
		return fmt.Errorf("invalid expose spec %q: %w", s, err)
	}
	if err := validatePort(spec, false); err != nil {
		return fmt.Errorf("invalid expose spec %q: %w", s, err)
	}
	return nil
}

// PublishBinding is one expanded -p/--publish mapping: host (ip:port) to a
// container port with its transport protocol. HostPort is "" when the spec
// lets the daemon pick the host port (bare "80" or host port 0).
type PublishBinding struct {
	HostIP        string
	HostPort      string
	ContainerPort string
	Proto         string // tcp, udp or sctp
}

// ExpandPublishSpec validates s with the same grammar as ValidatePublishSpec
// and expands host port ranges into one binding per port, so callers never
// need range-aware port handling. A container-side range is allowed only when
// it has the same length as the host range (pairwise mapping); anything else
// is an explicit error, never a silent truncation.
func ExpandPublishSpec(s string) ([]PublishBinding, error) {
	if err := ValidatePublishSpec(s); err != nil {
		return nil, err
	}
	spec, proto, err := splitPortProto(s)
	if err != nil {
		return nil, fmt.Errorf("invalid publish spec %q: %w", s, err)
	}

	hostIP := "0.0.0.0"
	rest := spec
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		hostIP = rest[1:end]
		rest = strings.TrimPrefix(rest[end+1:], ":")
	}
	fields := strings.Split(rest, ":")
	var hostPart, containerPart string
	switch len(fields) {
	case 1:
		containerPart = fields[0]
	case 2:
		hostPart, containerPart = fields[0], fields[1]
	case 3:
		hostIP, hostPart, containerPart = fields[0], fields[1], fields[2]
	default:
		return nil, fmt.Errorf("invalid publish spec %q: expected [ip:]hostPort:containerPort[/proto]", s)
	}

	expand := func(part string, allowZero bool) ([]string, error) {
		if start, end, ok := strings.Cut(part, "-"); ok {
			a, _ := strconv.Atoi(start)
			b, _ := strconv.Atoi(end)
			ports := make([]string, 0, b-a+1)
			for p := a; p <= b; p++ {
				if p == 0 && allowZero {
					ports = append(ports, "")
					continue
				}
				ports = append(ports, strconv.Itoa(p))
			}
			return ports, nil
		}
		if part == "0" && allowZero {
			return []string{""}, nil
		}
		return []string{part}, nil
	}

	if hostPart == "" {
		// Bare "80": the daemon picks the host port.
		return []PublishBinding{{HostIP: hostIP, HostPort: "", ContainerPort: containerPart, Proto: proto}}, nil
	}
	hosts, err := expand(hostPart, true)
	if err != nil {
		return nil, err
	}
	containers, err := expand(containerPart, false)
	if err != nil {
		return nil, err
	}
	if len(containers) > 1 && len(containers) != len(hosts) {
		return nil, fmt.Errorf("invalid publish spec %q: container port range %q must match host range length (%d ports)", s, containerPart, len(hosts))
	}
	out := make([]PublishBinding, 0, len(hosts))
	for i, h := range hosts {
		cp := containers[0]
		if len(containers) > 1 {
			cp = containers[i]
		}
		out = append(out, PublishBinding{HostIP: hostIP, HostPort: h, ContainerPort: cp, Proto: proto})
	}
	return out, nil
}
