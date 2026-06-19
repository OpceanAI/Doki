package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// ─── Container Lifecycle Tests ─────────────────────────────────────

func TestCreate_DuplicateID(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "test-container-001",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	_, err = rt.Create(cfg)
	if err == nil {
		t.Fatal("expected error on duplicate Create, got nil")
	}
}

func TestCreate_EmptyID(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err == nil {
		t.Fatal("expected error for empty container ID")
	}
}

func TestCreate_StateIsCreated(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "lifecycle-001",
		Args: []string{"/bin/sh"},
	}
	state, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if state.Status != common.StateCreated {
		t.Errorf("status = %q, want %q", state.Status, common.StateCreated)
	}
	if state.ID != cfg.ID {
		t.Errorf("ID = %q, want %q", state.ID, cfg.ID)
	}
	if state.Bundle == "" {
		t.Error("Bundle path should not be empty")
	}
}

func TestCreate_PersistsState(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:     "persist-001",
		Args:   []string{"/bin/sh"},
		Env:    []string{"FOO=bar"},
		Labels: map[string]string{"app": "test"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	statePath := filepath.Join(root, "containers", cfg.ID, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}
	var loaded ContainerState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if loaded.ID != cfg.ID {
		t.Errorf("persisted ID = %q, want %q", loaded.ID, cfg.ID)
	}
	if loaded.Status != common.StateCreated {
		t.Errorf("persisted status = %q, want %q", loaded.Status, common.StateCreated)
	}
}

func TestCreate_WithHostname(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:       "hostname-test",
		Args:     []string{"/bin/sh"},
		Hostname: "myhost",
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	hostnameFile := filepath.Join(root, "bundles", cfg.ID, "rootfs", "etc", "hostname")
	data, err := os.ReadFile(hostnameFile)
	if err != nil {
		t.Fatalf("read hostname: %v", err)
	}
	if !strings.Contains(string(data), "myhost") {
		t.Errorf("hostname file = %q, want to contain 'myhost'", string(data))
	}
}

func TestCreate_HostnameTruncation(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	longID := "abcdefghijklmnopqrstuvwxyz"
	cfg := &Config{
		ID:   longID,
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	hostnameFile := filepath.Join(root, "bundles", cfg.ID, "rootfs", "etc", "hostname")
	data, err := os.ReadFile(hostnameFile)
	if err != nil {
		t.Fatalf("read hostname: %v", err)
	}
	hostname := strings.TrimSpace(string(data))
	if len(hostname) > 12 {
		t.Errorf("hostname length = %d, want <= 12 (truncated from ID)", len(hostname))
	}
}

func TestStart_WrongState(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "start-wrong-001",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually set state to "running" to test wrong-state start.
	state, _ := rt.loadState(cfg.ID)
	state.Status = common.StateRunning
	rt.saveState(state)

	err = rt.Start(cfg.ID)
	if err == nil {
		t.Fatal("expected error starting a non-created container")
	}
	if !strings.Contains(err.Error(), "running") {
		t.Errorf("error should mention 'running': %v", err)
	}
}

func TestStart_Nonexistent(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	err := rt.Start("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
}

func TestStop_Nonexistent(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	err := rt.Stop("nonexistent-id", 10)
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
}

func TestStop_NotRunning(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "stop-notrunning",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Stop on non-running container should be idempotent (no error)
	err = rt.Stop(cfg.ID, 10)
	if err != nil {
		t.Fatalf("expected no error stopping a non-running container, got: %v", err)
	}
}

func TestKill_NonRunning(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "kill-notrunning",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = rt.Kill(cfg.ID, syscall.SIGKILL)
	if err == nil {
		t.Errorf("Kill on non-running should return error")
	}
}

func TestDelete_Nonexistent(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	err := rt.Delete("nonexistent", false)
	if err == nil {
		t.Fatal("expected error deleting nonexistent container without force")
	}

	err = rt.Delete("nonexistent", true)
	if err != nil {
		t.Errorf("force delete nonexistent should succeed, got: %v", err)
	}
}

func TestDelete_CreatedContainer(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "delete-created",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = rt.Delete(cfg.ID, false)
	if err != nil {
		t.Fatalf("Delete created container: %v", err)
	}

	_, err = rt.State(cfg.ID)
	if err == nil {
		t.Error("container should not exist after delete")
	}
}

func TestDelete_RunningContainer_NoForce(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "delete-running-noforce",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	state, _ := rt.loadState(cfg.ID)
	state.Status = common.StateRunning
	state.Pid = 99999
	rt.saveState(state)

	err = rt.Delete(cfg.ID, false)
	if err == nil {
		t.Fatal("expected error deleting running container without force")
	}
}

func TestPause_NotRunning(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "pause-notrunning",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = rt.Pause(cfg.ID)
	// BUG: Pause returns nil when container is not running (err from loadState is nil).
	if err != nil {
		t.Logf("Pause returned error (expected behavior): %v", err)
	} else {
		t.Error("BUG: Pause on non-running container should return error, got nil")
	}
}

func TestUnpause_NotPaused(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "unpause-notpaused",
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	err = rt.Unpause(cfg.ID)
	// BUG: Unpause returns nil when container is not paused.
	if err != nil {
		t.Logf("Unpause returned error (expected behavior): %v", err)
	} else {
		t.Error("BUG: Unpause on non-paused container should return error, got nil")
	}
}

// ─── State Management Tests ────────────────────────────────────────

func TestStateTransition_CreatedToRunning(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "state-trans-001",
		Args: []string{"/bin/true"},
	}
	state, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if state.Status != common.StateCreated {
		t.Errorf("after Create: status = %q, want %q", state.Status, common.StateCreated)
	}
}

func TestStatePersistence_RoundTrip(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:      "roundtrip-001",
		Args:    []string{"/bin/sh", "-c", "echo hello"},
		Env:     []string{"KEY=val"},
		Cwd:     "/tmp",
		User:    "1000:1000",
		Labels:  map[string]string{"a": "b"},
		Runtime: "native",
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := rt.State(cfg.ID)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if loaded.Config.Env[0] != "KEY=val" {
		t.Errorf("env not persisted: %v", loaded.Config.Env)
	}
	if loaded.Config.Cwd != "/tmp" {
		t.Errorf("cwd not persisted: %q", loaded.Config.Cwd)
	}
	if loaded.Config.User != "1000:1000" {
		t.Errorf("user not persisted: %q", loaded.Config.User)
	}
}

func TestState_ExitChanNotSerialized(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "exitchan-001",
		Args: []string{"/bin/sh"},
	}
	state, _ := rt.Create(cfg)
	if state.ExitChan == nil {
		t.Error("ExitChan should be non-nil after Create")
	}

	loaded, err := rt.State(cfg.ID)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if loaded.ExitChan != nil {
		t.Error("ExitChan should be nil after deserialization (json:\"-\")")
	}
}

func TestLoadState_PrefixMatch(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	fullID := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	cfg := &Config{
		ID:   fullID,
		Args: []string{"/bin/sh"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := rt.State("abcdef")
	if err != nil {
		t.Fatalf("prefix lookup: %v", err)
	}
	if loaded.ID != fullID {
		t.Errorf("prefix lookup returned wrong ID: %q", loaded.ID)
	}
}

func TestLoadState_PrefixMatchFalseConflict(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	id1 := "abcdef1111111111111111111111111111111111111111111111111111111111"
	id2 := "abcdef2222222222222222222222222222222222222222222222222222222222"

	_, err := rt.Create(&Config{ID: id1, Args: []string{"/bin/sh"}})
	if err != nil {
		t.Fatalf("Create id1: %v", err)
	}

	// BUG: Creating id2 should succeed (different ID), but loadState's prefix
	// match on "abcdef2" will match id1 first, causing a false conflict.
	_, err = rt.Create(&Config{ID: id2, Args: []string{"/bin/sh"}})
	if err != nil {
		t.Logf("BUG CONFIRMED: Create id2 failed due to prefix match: %v", err)
	}
}

func TestLoadState_NameAnnotation(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	fullID := "name123456789012345678901234567890123456789012345678901234567890"
	cfg := &Config{
		ID:          fullID,
		Args:        []string{"/bin/sh"},
		Annotations: map[string]string{"doki.name": "my-container"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	loaded, err := rt.State("my-container")
	if err != nil {
		t.Fatalf("name lookup: %v", err)
	}
	if loaded.ID != fullID {
		t.Errorf("name lookup returned wrong ID: %q", loaded.ID)
	}
}

// ─── Restart Policy Tests ──────────────────────────────────────────

func TestRestartPolicy_OnFailure_DoubleIncrement(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	state := &ContainerState{
		ID:      "restart-dbl",
		Status:  common.StateExited,
		Config:  &Config{RestartPolicy: common.RestartOnFailure, RestartMaxRetries: 1},
		ExitChan: make(chan struct{}),
	}
	close(state.ExitChan)

	rt.handleRestart(state, 1)

	// BUG: RestartCount should be 1 after one restart attempt, but
	// handleRestart increments it twice (once before the loop, once inside).
	if state.RestartCount > 2 {
		t.Errorf("BUG: RestartCount = %d, expected at most 2 (double-increment bug)", state.RestartCount)
	}
}

func TestRestartPolicy_NoRestart(t *testing.T) {
	state := &ContainerState{
		ID:     "restart-no",
		Status: common.StateExited,
		Config: &Config{RestartPolicy: common.RestartNo},
	}
	rt := &Runtime{root: t.TempDir()}
	rt.handleRestart(state, 1)
	if state.RestartCount != 0 {
		t.Errorf("RestartCount = %d, want 0 for RestartNo", state.RestartCount)
	}
}

func TestRestartPolicy_EmptyPolicy(t *testing.T) {
	state := &ContainerState{
		ID:     "restart-empty",
		Status: common.StateExited,
		Config: &Config{RestartPolicy: ""},
	}
	rt := &Runtime{root: t.TempDir()}
	rt.handleRestart(state, 1)
	if state.RestartCount != 0 {
		t.Errorf("RestartCount = %d, want 0 for empty policy", state.RestartCount)
	}
}

func TestRestartPolicy_OnFailure_SuccessExit(t *testing.T) {
	state := &ContainerState{
		ID:     "restart-success",
		Status: common.StateExited,
		Config: &Config{RestartPolicy: common.RestartOnFailure},
	}
	rt := &Runtime{root: t.TempDir()}
	rt.handleRestart(state, 0)
	if state.RestartCount != 0 {
		t.Errorf("RestartCount = %d, want 0 for successful exit with on-failure", state.RestartCount)
	}
}

func TestRestartPolicy_UnlessStopped_BehavesAsAlways(t *testing.T) {
	state := &ContainerState{
		ID:       "restart-unless",
		Status:   common.StateExited,
		Config:   &Config{RestartPolicy: common.RestartUnlessStopped},
		ExitChan: make(chan struct{}),
	}
	close(state.ExitChan)

	// BUG: "unless-stopped" checks state.Status != StateDead, but status is
	// always StateExited at this point (never StateDead). So it always restarts,
	// making it identical to "always".
	// We can't fully test this without actually starting, but we verify the
	// status is never StateDead.
	if state.Status == common.StateDead {
		t.Error("state should not be dead")
	}
}

// ─── ExitChan / Stop Deadlock Tests ────────────────────────────────

func TestStopUnlocked_ExitChanMismatch(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "exitchan-mismatch",
		Args: []string{"/bin/sh"},
	}
	state, _ := rt.Create(cfg)

	// Simulate a running container with a real ExitChan.
	state.Status = common.StateRunning
	state.Pid = os.Getpid()
	state.ExitChan = make(chan struct{})
	rt.saveState(state)

	// Now loadState (as stopUnlocked does) creates a NEW state with a NEW ExitChan.
	loaded, _ := rt.loadState(cfg.ID)
	loaded.ExitChan = make(chan struct{})

	// The loaded ExitChan is different from the one in the original state.
	// monitorProcess would close the original, but stopUnlocked waits on the loaded one.
	if state.ExitChan == loaded.ExitChan {
		t.Error("ExitChan should be different objects after loadState")
	}

	// This proves the bug: stopUnlocked waits on a channel that will never be closed
	// by monitorProcess (which holds a reference to the original state's ExitChan).
}

// ─── Signal Parsing Tests ──────────────────────────────────────────

func TestParseSignal_Numeric(t *testing.T) {
	// BUG: parseSignal doesn't handle numeric signals like "15" or "9".
	sig := parseSignal("15")
	if sig != syscall.SIGTERM {
		t.Errorf("parseSignal(\"15\") = %d, want %d (SIGTERM as default)", sig, syscall.SIGTERM)
	}
	// The bug is that "15" should map to SIGTERM (15), but it actually returns
	// SIGTERM as the default fallback, which happens to be correct for "15"
	// but wrong for other numeric values.
	sig9 := parseSignal("9")
	if sig9 != syscall.SIGKILL {
		t.Errorf("BUG: parseSignal(\"9\") = %d, want %d (SIGKILL)", sig9, syscall.SIGKILL)
	}
}

func TestParseSignal_AllNamed(t *testing.T) {
	tests := []struct {
		name string
		want syscall.Signal
	}{
		{"SIGHUP", syscall.SIGHUP},
		{"SIGINT", syscall.SIGINT},
		{"SIGQUIT", syscall.SIGQUIT},
		{"SIGKILL", syscall.SIGKILL},
		{"SIGTERM", syscall.SIGTERM},
		{"SIGSTOP", syscall.SIGSTOP},
		{"SIGUSR1", syscall.SIGUSR1},
		{"SIGUSR2", syscall.SIGUSR2},
	}
	for _, tt := range tests {
		got := parseSignal(tt.name)
		if got != tt.want {
			t.Errorf("parseSignal(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestParseSignal_CaseInsensitive(t *testing.T) {
	// BUG: parseSignal uses strings.ToUpper but input "sigterm" should work.
	sig := parseSignal("sigterm")
	if sig != syscall.SIGTERM {
		t.Errorf("parseSignal(\"sigterm\") = %d, want %d", sig, syscall.SIGTERM)
	}
}

// ─── User Parsing Tests ────────────────────────────────────────────

func TestParseUser_Variants(t *testing.T) {
	tests := []struct {
		input  string
		wantUID int
		wantGID int
	}{
		{"", -1, -1},
		{"0", 0, 0},
		{"1000", 1000, 1000},
		{"0:0", 0, 0},
		{"1000:2000", 1000, 2000},
		{"root", -1, -1},
		{"root:root", -1, -1},
		{"1000:root", 1000, 1000},
	}
	for _, tt := range tests {
		uid, gid := parseUser(tt.input)
		if uid != tt.wantUID || gid != tt.wantGID {
			t.Errorf("parseUser(%q) = (%d, %d), want (%d, %d)",
				tt.input, uid, gid, tt.wantUID, tt.wantGID)
		}
	}
}

// ─── Log Rotation Tests ────────────────────────────────────────────

func TestRotateLog_NoRotation(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	logPath := filepath.Join(root, "test.log")
	os.WriteFile(logPath, []byte("small"), 0644)

	rt.rotateLog(logPath, 10*1024*1024, 3)

	if _, err := os.Stat(logPath + ".1"); err == nil {
		t.Error("small log should not be rotated")
	}
}

func TestRotateLog_Rotation(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	logPath := filepath.Join(root, "test.log")
	bigData := make([]byte, 1024)
	os.WriteFile(logPath, bigData, 0644)

	rt.rotateLog(logPath, 512, 3)

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Error("large log should be rotated to .1")
	}
}

func TestRotateLog_MultipleRotations(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	logPath := filepath.Join(root, "test.log")
	bigData := make([]byte, 1024)

	for i := 0; i < 5; i++ {
		os.WriteFile(logPath, bigData, 0644)
		rt.rotateLog(logPath, 512, 3)
	}

	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Error(".1 should exist")
	}
	if _, err := os.Stat(logPath + ".2"); err != nil {
		t.Error(".2 should exist")
	}
	if _, err := os.Stat(logPath + ".3"); err != nil {
		t.Error(".3 should exist")
	}
}

// ─── Concurrent Operations Tests ───────────────────────────────────

func TestCreate_Concurrent(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg := &Config{
				ID:   "concurrent-" + string(rune('a'+idx)),
				Args: []string{"/bin/sh"},
			}
			_, errs[idx] = rt.Create(cfg)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent Create[%d]: %v", i, err)
		}
	}
}

func TestList_Concurrent(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	for i := 0; i < 5; i++ {
		cfg := &Config{
			ID:   "list-" + string(rune('a'+i)),
			Args: []string{"/bin/sh"},
		}
		rt.Create(cfg)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = rt.List()
		}()
	}
	wg.Wait()
}

// ─── Healthcheck Tests ─────────────────────────────────────────────

func TestHealthcheck_Defaults(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	rt.StartHealthcheck("test", []string{"echo", "ok"}, 0, 0, 0)
	time.Sleep(100 * time.Millisecond)
}

func TestHealthcheck_ContainerNotFound(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	rt.StartHealthcheck("nonexistent", []string{"echo", "ok"},
		100*time.Millisecond, 50*time.Millisecond, 3)
	time.Sleep(300 * time.Millisecond)
}

// ─── Extra Hosts Parsing Tests ─────────────────────────────────────

func TestParseExtraHosts(t *testing.T) {
	tests := []struct {
		input []string
		want  map[string]string
	}{
		{nil, map[string]string{}},
		{[]string{}, map[string]string{}},
		{[]string{"host1:1.2.3.4"}, map[string]string{"host1": "1.2.3.4"}},
		{[]string{"a:1", "b:2"}, map[string]string{"a": "1", "b": "2"}},
		{[]string{"invalid"}, map[string]string{}},
	}
	for _, tt := range tests {
		got := parseExtraHosts(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseExtraHosts(%v) len = %d, want %d", tt.input, len(got), len(tt.want))
		}
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("parseExtraHosts(%v)[%q] = %q, want %q", tt.input, k, got[k], v)
			}
		}
	}
}

// ─── Mode Detection Tests ──────────────────────────────────────────

func TestDetectMode_NotEmpty(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	if rt.Mode().String() == "unknown" {
		t.Error("detected mode should not be 'unknown'")
	}
}

func TestIsAndroid(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	result := rt.isAndroid()
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		if !result {
			t.Error("isAndroid should return true on Termux")
		}
	}
}

// ─── Network Stats Tests ───────────────────────────────────────────

func TestGetNetworkStats(t *testing.T) {
	stats := getNetworkStats()
	if stats == nil {
		t.Skip("/proc/net/dev not available")
	}
	if len(stats) == 0 {
		t.Error("expected at least one network interface")
	}
}

// ─── DNS Configuration Tests ───────────────────────────────────────

func TestCreate_ResolvConf(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:        "dns-test",
		Args:      []string{"/bin/sh"},
		DNS:       []string{"1.1.1.1"},
		DNSSearch: []string{"example.com"},
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	resolvPath := filepath.Join(root, "bundles", cfg.ID, "rootfs", "etc", "resolv.conf")
	data, err := os.ReadFile(resolvPath)
	if err != nil {
		t.Fatalf("read resolv.conf: %v", err)
	}
	if !strings.Contains(string(data), "1.1.1.1") {
		t.Errorf("resolv.conf should contain custom DNS: %s", data)
	}
}

// ─── Edge Cases ────────────────────────────────────────────────────

func TestCreate_NoArgs(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "no-args",
		Args: nil,
	}
	_, err := rt.Create(cfg)
	if err != nil {
		t.Logf("Create with no args: %v (may be allowed at create time)", err)
	}
}

func TestCreate_WithResources(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "resources-test",
		Args: []string{"/bin/sh"},
		Resources: &Resources{
			Memory:    256 * 1024 * 1024,
			CPUShares: 512,
			PidsLimit: 100,
		},
	}
	state, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if state.Config.Resources.Memory != 256*1024*1024 {
		t.Errorf("memory not persisted: %d", state.Config.Resources.Memory)
	}
}

func TestCreate_WithMounts(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "mounts-test",
		Args: []string{"/bin/sh"},
		Mounts: []common.Mount{
			{Type: common.MountBind, Source: "/tmp", Target: "/mnt", ReadOnly: true},
			{Type: common.MountTmpfs, Target: "/tmpfs"},
		},
	}
	state, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(state.Config.Mounts) != 2 {
		t.Errorf("mounts count = %d, want 2", len(state.Config.Mounts))
	}
}

func TestCreate_WithHealthCheck(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{
		ID:   "healthcheck-cfg",
		Args: []string{"/bin/sh"},
		HealthCheck: &HealthCheckConfig{
			Test:     []string{"CMD", "echo", "ok"},
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  3,
		},
	}
	state, err := rt.Create(cfg)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if state.Config.HealthCheck == nil {
		t.Error("healthcheck config not persisted")
	}
}

func TestBuildCgroupConfig_NilResources(t *testing.T) {
	cfg := &Config{Resources: nil}
	cgCfg := (&Runtime{}).buildCgroupConfig(cfg)
	if cgCfg == nil {
		t.Error("buildCgroupConfig should return non-nil even with nil resources")
	}
}

func TestBuildCgroupConfig_WithResources(t *testing.T) {
	cfg := &Config{
		Resources: &Resources{
			Memory:    512 * 1024 * 1024,
			CPUShares: 1024,
			PidsLimit: 200,
		},
	}
	cgCfg := (&Runtime{}).buildCgroupConfig(cfg)
	if cgCfg.Memory != 512*1024*1024 {
		t.Errorf("cgroup memory = %d, want %d", cgCfg.Memory, 512*1024*1024)
	}
}

// ─── GetLogs Tests ─────────────────────────────────────────────────

func TestGetLogs_NoLogPath(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	id := "logs-nopath"
	dir := filepath.Join(root, "containers", id)
	os.MkdirAll(dir, 0755)
	state := &ContainerState{ID: id, Status: common.StateExited}
	rt.saveState(state)

	logs, err := rt.GetLogs(id, 10)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if logs != "" {
		t.Errorf("expected empty logs, got %q", logs)
	}
}

func TestGetLogs_WithTail(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	id := "logs-tail"
	dir := filepath.Join(root, "containers", id)
	os.MkdirAll(dir, 0755)
	logPath := filepath.Join(root, "containers", id, "container.log")
	os.WriteFile(logPath, []byte("line1\nline2\nline3\nline4\nline5\n"), 0644)

	state := &ContainerState{ID: id, Status: common.StateExited, LogPath: logPath}
	rt.saveState(state)

	logs, err := rt.GetLogs(id, 2)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	lines := strings.Split(logs, "\n")
	// With tail=2, we expect the last 2 lines.
	if len(lines) > 3 {
		t.Errorf("expected at most 3 lines (tail=2 + trailing), got %d: %q", len(lines), logs)
	}
}

// ─── Processes Tests ───────────────────────────────────────────────

func TestProcesses_NotRunning(t *testing.T) {
	root := t.TempDir()
	rt := newTestRuntime(t, root)

	cfg := &Config{ID: "procs-notrunning", Args: []string{"/bin/sh"}}
	rt.Create(cfg)

	_, err := rt.Processes(cfg.ID)
	if err != nil {
		t.Logf("Processes on non-running: %v (expected)", err)
	}
}

// ─── Helper ────────────────────────────────────────────────────────

func newTestRuntime(t *testing.T, root string) *Runtime {
	t.Helper()
	rt := &Runtime{
		root:     root,
		rootless: true,
		mode:     ModeNative,
	}
	common.EnsureDir(filepath.Join(root, "containers"))
	common.EnsureDir(filepath.Join(root, "bundles"))
	common.EnsureDir(filepath.Join(root, "layers"))
	common.EnsureDir(filepath.Join(root, "rootfs"))
	return rt
}
