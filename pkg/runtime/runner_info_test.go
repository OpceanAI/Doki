package runtime

import "testing"

func TestExecutionModeInfo(t *testing.T) {
	info := ModeProot.Info()
	if info.Name != "proot" {
		t.Fatalf("ModeProot name = %q, want proot", info.Name)
	}
	if info.Level <= ModeNative.Info().Level {
		t.Fatalf("proot level = %d, native level = %d; want proot stronger", info.Level, ModeNative.Info().Level)
	}
	if info.Isolation == "" || len(info.Platforms) == 0 || info.Description == "" {
		t.Fatalf("incomplete proot info: %+v", info)
	}
}

func TestExecutionModeInfosSortedByLevel(t *testing.T) {
	infos := ExecutionModeInfos()
	if len(infos) != len(AllExecutionModes()) {
		t.Fatalf("len(ExecutionModeInfos) = %d, want %d", len(infos), len(AllExecutionModes()))
	}
	seen := map[ExecutionMode]bool{}
	last := 99
	for _, info := range infos {
		if info.Level > last {
			t.Fatalf("levels not descending at %+v after %d", info, last)
		}
		last = info.Level
		seen[info.Mode] = true
	}
	for _, mode := range AllExecutionModes() {
		if !seen[mode] {
			t.Fatalf("missing mode %s", mode)
		}
	}
}
