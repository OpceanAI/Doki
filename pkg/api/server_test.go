package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpceanAI/Doki/pkg/common"
)

func TestVolumeManagerSkipsBadMetadata(t *testing.T) {
	root := t.TempDir()
	badDir := filepath.Join(root, "bad")
	if err := os.MkdirAll(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, "volume.json"), []byte("not-json"), 0644); err != nil {
		t.Fatal(err)
	}

	goodDir := filepath.Join(root, "good")
	if err := os.MkdirAll(goodDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(common.VolumeInfo{Name: "good", Driver: "local", Mountpoint: goodDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goodDir, "volume.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	vm, err := NewVolumeManager(root)
	if err != nil {
		t.Fatalf("NewVolumeManager() error = %v", err)
	}
	if _, err := vm.Get("good"); err != nil {
		t.Fatalf("expected good volume to load: %v", err)
	}
	if _, err := vm.Get("bad"); !common.IsNotFound(err) {
		t.Fatalf("bad metadata loaded unexpectedly: %v", err)
	}
}

func TestVolumeManagerCreateRemove(t *testing.T) {
	vm, err := NewVolumeManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	vol, err := vm.Create("data", "", nil, nil)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(vol.Mountpoint, "volume.json")); err != nil {
		t.Fatalf("volume metadata missing: %v", err)
	}
	if err := vm.Remove("data"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(vol.Mountpoint); !os.IsNotExist(err) {
		t.Fatalf("volume directory still exists: %v", err)
	}
}

func TestRequestIDMiddlewareStoresContext(t *testing.T) {
	mw := NewMiddleware()
	seen := ""
	h := mw.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/_ping", nil)
	req.Header.Set("X-Request-ID", "req-123")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if seen != "req-123" {
		t.Fatalf("request ID in context = %q, want req-123", seen)
	}
	if got := rr.Header().Get("X-Request-ID"); got != "req-123" {
		t.Fatalf("response request ID = %q, want req-123", got)
	}
}
