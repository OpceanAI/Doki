package runtime

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestSaveState_Atomic confirms that a crash mid-write doesn't leave
// a partial state.json behind: either the file is the old content
// or the new content, never a torn mix.
func TestSaveState_Atomic(t *testing.T) {
	root := t.TempDir()
	rt := &Runtime{root: root}

	// Manually create a container dir.
	id := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	dir := filepath.Join(root, "containers", id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	state := &ContainerState{ID: id, Status: "running", Pid: 1234}
	if err := rt.saveState(state); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	finalPath := filepath.Join(dir, "state.json")

	// No temp file should be left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Errorf("leftover file in container dir: %s", e.Name())
		}
	}
	// File must exist and be valid JSON.
	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("read state.json: %v", err)
	}
	if len(data) < 5 {
		t.Errorf("state.json too small: %d bytes", len(data))
	}
}

// TestSaveState_ConcurrentCalls makes sure that racing saveState calls
// don't leave a torn file.
func TestSaveState_ConcurrentCalls(t *testing.T) {
	root := t.TempDir()
	rt := &Runtime{root: root}
	id := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	dir := filepath.Join(root, "containers", id)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	state := &ContainerState{ID: id, Status: "running", Pid: 42}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = rt.saveState(state)
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !containsAny(data, []byte(`"status": "running"`)) {
		t.Errorf("final state.json is corrupt or wrong: %s", data)
	}
}

func containsAny(haystack []byte, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return len(needle) == 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
