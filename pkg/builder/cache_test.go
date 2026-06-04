package builder

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpceanAI/Doki/pkg/image"
)

func TestBuildCachePutGet(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 1024*1024)

	layerPath := filepath.Join(dir, "test.tar")
	os.WriteFile(layerPath, []byte("test layer content"), 0644)

	if err := cache.Put("test-key", layerPath); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok := cache.Get("test-key")
	if !ok {
		t.Fatal("Get: not found")
	}
	if got == "" {
		t.Error("Get: empty path")
	}

	if _, err := os.Stat(got); err != nil {
		t.Fatalf("cached file: %v", err)
	}
}

func TestBuildCachePutBytes(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 1024*1024)

	data := []byte("test bytes content")
	if err := cache.PutBytes("bytes-key", data); err != nil {
		t.Fatalf("PutBytes: %v", err)
	}

	got, ok := cache.GetBytes("bytes-key")
	if !ok {
		t.Fatal("GetBytes: not found")
	}
	if string(got) != string(data) {
		t.Errorf("GetBytes = %q, want %q", got, data)
	}
}

func TestBuildCacheEviction(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 50)

	for i := 0; i < 5; i++ {
		key := "key-" + string(rune('0'+i))
		data := make([]byte, 20)
		data[0] = byte(i)
		cache.PutBytes(key, data)
		time.Sleep(10 * time.Millisecond)
	}

	stats := cache.Stats()
	if stats.Entries > 3 {
		t.Errorf("expected eviction, got %d entries", stats.Entries)
	}
}

func TestBuildCacheHas(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 1024*1024)

	if cache.Has("nonexistent") {
		t.Error("Has: should be false for nonexistent key")
	}

	cache.PutBytes("exists", []byte("data"))
	if !cache.Has("exists") {
		t.Error("Has: should be true for existing key")
	}
}

func TestBuildCacheDelete(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 1024*1024)

	cache.PutBytes("to-delete", []byte("data"))
	if !cache.Has("to-delete") {
		t.Fatal("should exist before delete")
	}

	cache.Delete("to-delete")
	if cache.Has("to-delete") {
		t.Error("should not exist after delete")
	}
}

func TestBuildCachePurge(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 1024*1024)

	cache.PutBytes("a", []byte("aaa"))
	cache.PutBytes("b", []byte("bbb"))

	if err := cache.Purge(); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	stats := cache.Stats()
	if stats.Entries != 0 {
		t.Errorf("entries after purge = %d, want 0", stats.Entries)
	}
}

func TestBuildCacheStats(t *testing.T) {
	dir := t.TempDir()
	cache := NewBuildCache(dir, 1024*1024)

	cache.PutBytes("a", []byte("aaa"))
	cache.PutBytes("b", []byte("bbb"))

	stats := cache.Stats()
	if stats.Entries != 2 {
		t.Errorf("entries = %d, want 2", stats.Entries)
	}
	if stats.TotalSize != 6 {
		t.Errorf("total size = %d, want 6", stats.TotalSize)
	}
}

func TestGenerateKey(t *testing.T) {
	key1 := GenerateKey("RUN", []string{"apt-get", "update"}, map[string]string{"A": "1"})
	key2 := GenerateKey("RUN", []string{"apt-get", "update"}, map[string]string{"A": "1"})
	key3 := GenerateKey("RUN", []string{"apt-get", "upgrade"}, map[string]string{"A": "1"})

	if key1 != key2 {
		t.Error("same input should produce same key")
	}
	if key1 == key3 {
		t.Error("different input should produce different key")
	}
	if len(key1) != 64 {
		t.Errorf("key length = %d, want 64", len(key1))
	}
}

func TestDetectBuildKit(t *testing.T) {
	addr := DetectBuildKit()
	_ = addr
}

func TestParseBuildKitAddr(t *testing.T) {
	tests := []struct {
		addr    string
		network string
		address string
	}{
		{"unix:///run/buildkit/buildkitd.sock", "unix", "/run/buildkit/buildkitd.sock"},
		{"tcp://localhost:1234", "tcp", "localhost:1234"},
		{"/tmp/buildkit.sock", "unix", "/tmp/buildkit.sock"},
	}
	for _, tt := range tests {
		network, address := parseBuildKitAddr(tt.addr)
		if network != tt.network {
			t.Errorf("parseBuildKitAddr(%q) network = %q, want %q", tt.addr, network, tt.network)
		}
		if address != tt.address {
			t.Errorf("parseBuildKitAddr(%q) address = %q, want %q", tt.addr, address, tt.address)
		}
	}
}

func TestProgressTracker(t *testing.T) {
	var events []ProgressEvent
	tracker := NewProgressTracker(nil, func(event ProgressEvent) {
		events = append(events, event)
	})

	tracker.SetStage("build", 3)
	tracker.Step("step 1")
	tracker.Output("output line")
	tracker.Step("step 2")
	tracker.Complete("done")

	if len(events) != 5 {
		t.Errorf("events = %d, want 5", len(events))
	}
	if events[0].Type != "stage" {
		t.Errorf("events[0].Type = %s, want stage", events[0].Type)
	}
	if events[4].Type != "complete" {
		t.Errorf("events[4].Type = %s, want complete", events[4].Type)
	}
}

func TestProgressTrackerWithWriter(t *testing.T) {
	tracker := NewProgressTrackerWithWriter(os.Stderr)
	tracker.Step("test step")
	tracker.Complete("done")
}

func TestNewBuilderWithOptions(t *testing.T) {
	dir := t.TempDir()
	store := &image.Store{}

	cache := NewBuildCache(dir, 1024*1024)
	tracker := NewProgressTracker(nil, nil)

	builder := NewBuilder(store).
		WithCache(cache).
		WithProgress(tracker)

	if builder.cache != cache {
		t.Error("cache not set")
	}
	if builder.progress != tracker {
		t.Error("progress not set")
	}
}
