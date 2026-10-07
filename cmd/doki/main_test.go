package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDoctorCommandRegistered(t *testing.T) {
	cmd, ok := commands["doctor"]
	if !ok {
		t.Fatal("doctor command is not registered")
	}
	if cmd.Handler == nil {
		t.Fatal("doctor command has nil handler")
	}
	if len(cmd.Aliases) != 1 || cmd.Aliases[0] != "diagnose" {
		t.Fatalf("doctor aliases = %v, want [diagnose]", cmd.Aliases)
	}
}

// TestCommandCount guards the command surface: commands plus the groupCommands
// and their subcommands must keep at least 80 commands registered. Every entry
// must carry a matching name and either a handler or subcommands.
func TestCommandCount(t *testing.T) {
	total := len(commands)
	for key, cmd := range commands {
		if cmd == nil {
			t.Errorf("commands[%q] is nil", key)
			continue
		}
		if cmd.Name != key {
			t.Errorf("commands[%q].Name = %q, want %q", key, cmd.Name, key)
		}
		if cmd.Handler == nil && len(cmd.SubCommands) == 0 {
			t.Errorf("commands[%q] has neither handler nor subcommands", key)
		}
		total += len(cmd.SubCommands)
	}
	for key, grp := range groupCommands {
		if grp == nil {
			t.Errorf("groupCommands[%q] is nil", key)
			continue
		}
		if grp.Name != key {
			t.Errorf("groupCommands[%q].Name = %q, want %q", key, grp.Name, key)
		}
		if len(grp.SubCommands) == 0 {
			t.Errorf("groupCommands[%q] has no subcommands", key)
		}
		total += len(grp.SubCommands)
	}
	if total < 80 {
		t.Errorf("command count = %d, want >= 80 (commands + group subcommands)", total)
	}
}

// updateGolden rewrites the golden files instead of comparing against them.
// Usage: go test ./cmd/doki -run TestHelpGolden -update
var updateGolden = flag.Bool("update", false, "rewrite golden files")

// captureStdout runs f and returns everything it wrote to os.Stdout.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	f()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	_ = r.Close()
	return string(out)
}

// TestHelpGolden compares the `doki --help` output against
// testdata/help.golden so accidental help-text changes are reviewed.
func TestHelpGolden(t *testing.T) {
	got := captureStdout(t, printMainHelp)
	golden := filepath.Join("testdata", "help.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./cmd/doki -run TestHelpGolden -update` to create it)", golden, err)
	}
	if got != string(want) {
		t.Errorf("doki --help output differs from %s\ngot:\n%s\nwant:\n%s", golden, got, want)
	}
}

// TestFlagHelpersEqualsForm pins down that flagBool, flagStr and cleanIDs
// treat the "--key=value" and "--key value" forms identically: flag names are
// matched through cli.SplitFlag everywhere.
func TestFlagHelpersEqualsForm(t *testing.T) {
	space := []string{"--force", "--format", "json", "abc123"}
	equals := []string{"--force=true", "--format=json", "abc123"}
	for _, argv := range [][]string{space, equals} {
		if !flagBool(argv, "-f", "--force") {
			t.Errorf("flagBool(%v, --force) = false, want true", argv)
		}
		if got := flagStr(argv, "--format"); got != "json" {
			t.Errorf("flagStr(%v, --format) = %q, want json", argv, got)
		}
		if got := cleanIDs(argv); !reflect.DeepEqual(got, []string{"abc123"}) {
			t.Errorf("cleanIDs(%v) = %v, want [abc123]", argv, got)
		}
	}
	if flagBool([]string{"--force=false"}, "--force") {
		t.Error(`flagBool(["--force=false"], --force) = true, want false`)
	}
}

// TestCleanIDs is a golden test mapping a raw argv to the positional IDs
// cleanIDs must extract. It pins down the value-taking short flags (-t, -s, -f,
// -p, -o, -i, -m, -e, -v, -u, -w, -c) so a flag's value is never mistaken for a
// container/image ID.
func TestCleanIDs(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want []string
	}{
		{"stop time", []string{"-t", "5", "abc123"}, []string{"abc123"}},
		{"kill signal", []string{"-s", "KILL", "abc123"}, []string{"abc123"}},
		{"inspect format", []string{"-f", "{{.Id}}", "abc123"}, []string{"abc123"}},
		{"save output", []string{"-o", "out.tar", "abc123"}, []string{"abc123"}},
		{"load input", []string{"-i", "in.tar", "abc123"}, []string{"abc123"}},
		{"commit message", []string{"-m", "msg", "abc123"}, []string{"abc123"}},
		{"run env", []string{"-e", "A=1", "abc123"}, []string{"abc123"}},
		{"run volume", []string{"-v", "/data:/mnt", "abc123"}, []string{"abc123"}},
		{"run user", []string{"-u", "1000", "abc123"}, []string{"abc123"}},
		{"run workdir", []string{"-w", "/app", "abc123"}, []string{"abc123"}},
		{"run cpu shares", []string{"-c", "512", "abc123"}, []string{"abc123"}},
		{"run publish", []string{"-p", "8080:80", "abc123"}, []string{"abc123"}},
		{"equals form", []string{"--format=json", "abc123"}, []string{"abc123"}},
		{"plain ids", []string{"abc123", "def456"}, []string{"abc123", "def456"}},
		{"no flags", []string{"alpine"}, []string{"alpine"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanIDs(tc.argv)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("cleanIDs(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}
