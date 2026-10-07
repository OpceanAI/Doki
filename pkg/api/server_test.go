package api

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/registry"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
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

// newTestDaemon starts an HTTP server on a unix socket that serves the given
// handler, and returns a client wired to that socket plus a cleanup function.
func newTestDaemon(t *testing.T, h http.Handler) (*http.Client, func()) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "dokid.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(ln) }()
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	return client, func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

// newTestAPIServer builds a daemon surface backed by throwaway stores.
func newTestAPIServer(t *testing.T) (*Server, *dokiruntime.Runtime, *image.Store) {
	t.Helper()
	dataDir := t.TempDir()
	cfg := common.DefaultConfig()
	cfg.DataDir = dataDir
	rt := dokiruntime.NewRuntime(filepath.Join(dataDir, "runtime"), nil)
	img, err := image.NewStore(filepath.Join(dataDir, "images"))
	if err != nil {
		t.Fatalf("image store: %v", err)
	}
	s := &Server{
		config:  cfg,
		router:  http.NewServeMux(),
		runtime: rt,
		image:   img,
	}
	s.registerRoutes()
	return s, rt, img
}

func doRequest(t *testing.T, c *http.Client, method, path, contentType string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, "http://doki"+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func doJSON(t *testing.T, c *http.Client, method, path, body string) *http.Response {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	return doRequest(t, c, method, path, "application/json", rd)
}

// TestContainerUpdateBlkioDeviceRules covers the Docker API 1.55 update
// fields: per-device blkio rules, MemorySwappiness, OomKillDisable and the CPU
// realtime limits. Null keeps a value, an empty array clears the rules.
func TestContainerUpdateBlkioDeviceRules(t *testing.T) {
	s, rt, _ := newTestAPIServer(t)
	if err := rt.SaveState(&dokiruntime.ContainerState{
		ID:     "cont1",
		Status: common.StateCreated,
		Config: &dokiruntime.Config{},
	}); err != nil {
		t.Fatal(err)
	}
	client, cleanup := newTestDaemon(t, s.router)
	defer cleanup()

	resp := doJSON(t, client, http.MethodPost, "/containers/cont1/update", `{
		"BlkioWeight": 400,
		"BlkioWeightDevice": [{"Path": "/dev/sda", "Weight": 500}],
		"BlkioDeviceReadBps": [{"Path": "/dev/sda", "Rate": 1048576}],
		"BlkioDeviceWriteBps": [{"Path": "/dev/sda", "Rate": 2097152}],
		"BlkioDeviceReadIOps": [{"Path": "/dev/sda", "Rate": 100}],
		"BlkioDeviceWriteIOps": [{"Path": "/dev/sda", "Rate": 200}],
		"MemorySwappiness": 30,
		"OomKillDisable": true,
		"CpuRealtimeRuntime": 1000,
		"CpuRealtimePeriod": 1000000
	}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: got %d, want 200", resp.StatusCode)
	}
	var out struct {
		Warnings []string `json:"Warnings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	_ = resp.Body.Close()
	if len(out.Warnings) == 0 {
		t.Error("expected a warning that per-device blkio limits are not enforced")
	}

	got, err := rt.State("cont1")
	if err != nil {
		t.Fatal(err)
	}
	res := got.Config.Resources
	if res == nil {
		t.Fatal("resources were not persisted")
	}
	if len(res.BlkioWeightDevice) != 1 || res.BlkioWeightDevice[0].Path != "/dev/sda" || res.BlkioWeightDevice[0].Weight != 500 {
		t.Errorf("BlkioWeightDevice = %+v", res.BlkioWeightDevice)
	}
	if len(res.BlkioDeviceReadBps) != 1 || res.BlkioDeviceReadBps[0].Rate != 1048576 {
		t.Errorf("BlkioDeviceReadBps = %+v", res.BlkioDeviceReadBps)
	}
	if len(res.BlkioDeviceWriteBps) != 1 || res.BlkioDeviceWriteBps[0].Rate != 2097152 {
		t.Errorf("BlkioDeviceWriteBps = %+v", res.BlkioDeviceWriteBps)
	}
	if len(res.BlkioDeviceReadIOps) != 1 || res.BlkioDeviceReadIOps[0].Rate != 100 {
		t.Errorf("BlkioDeviceReadIOps = %+v", res.BlkioDeviceReadIOps)
	}
	if len(res.BlkioDeviceWriteIOps) != 1 || res.BlkioDeviceWriteIOps[0].Rate != 200 {
		t.Errorf("BlkioDeviceWriteIOps = %+v", res.BlkioDeviceWriteIOps)
	}
	if res.MemorySwappiness == nil || *res.MemorySwappiness != 30 {
		t.Errorf("MemorySwappiness = %v, want 30", res.MemorySwappiness)
	}
	if !res.OomKillDisable {
		t.Error("OomKillDisable = false, want true")
	}
	if res.CPURealtimeRuntime != 1000 || res.CPURealtimePeriod != 1000000 {
		t.Errorf("CpuRealtime = (%d, %d), want (1000, 1000000)", res.CPURealtimeRuntime, res.CPURealtimePeriod)
	}

	// [] clears a rule set, null keeps the stored values.
	resp = doJSON(t, client, http.MethodPost, "/containers/cont1/update",
		`{"BlkioDeviceReadBps": [], "MemorySwappiness": null, "OomKillDisable": false}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second update: got %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()

	got, err = rt.State("cont1")
	if err != nil {
		t.Fatal(err)
	}
	res = got.Config.Resources
	if len(res.BlkioDeviceReadBps) != 0 {
		t.Errorf("BlkioDeviceReadBps not cleared by empty array: %+v", res.BlkioDeviceReadBps)
	}
	if len(res.BlkioDeviceWriteBps) != 1 {
		t.Errorf("BlkioDeviceWriteBps changed without being sent: %+v", res.BlkioDeviceWriteBps)
	}
	if len(res.BlkioWeightDevice) != 1 || res.BlkioWeightDevice[0].Weight != 500 {
		t.Errorf("BlkioWeightDevice changed without being sent: %+v", res.BlkioWeightDevice)
	}
	if res.MemorySwappiness == nil || *res.MemorySwappiness != 30 {
		t.Errorf("MemorySwappiness changed on null: %v", res.MemorySwappiness)
	}
	if res.OomKillDisable {
		t.Error("OomKillDisable = true, want false")
	}
}

// TestImageAttestationsEmpty pins GET /images/{name}/attestations: 200 with an
// honest empty list (no attestation data is invented), 404 for an unknown
// image, 400 for more than one platform.
func TestImageAttestationsEmpty(t *testing.T) {
	s, _, img := newTestAPIServer(t)
	if err := img.SaveRecord(&image.ImageRecord{ID: "img1", RepoTags: []string{"app:1.0"}}); err != nil {
		t.Fatal(err)
	}
	client, cleanup := newTestDaemon(t, s.router)
	defer cleanup()

	for _, path := range []string{
		"/images/app:1.0/attestations",
		"/images/app:1.0/attestations?platform=linux/arm64&type=https://slsa.dev/provenance/v1&statement=false",
		"/images/app:1.0/attestations?statement=true",
	} {
		resp := doJSON(t, client, http.MethodGet, path, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d, want 200", path, resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.TrimSpace(string(body)) != "[]" {
			t.Errorf("%s: body = %q, want []", path, strings.TrimSpace(string(body)))
		}
	}

	resp := doJSON(t, client, http.MethodGet, "/images/missing:1.0/attestations", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown image: got %d, want 404", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp = doJSON(t, client, http.MethodGet, "/images/app:1.0/attestations?platform=a&platform=b", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("two platforms: got %d, want 400", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// TestImageTagQueryParams covers the canonical ?repo=&tag= form of
// POST /images/{name}/tag as well as the JSON body variant.
func TestImageTagQueryParams(t *testing.T) {
	s, _, img := newTestAPIServer(t)
	if err := img.SaveRecord(&image.ImageRecord{ID: "img1", RepoTags: []string{"app:1.0"}}); err != nil {
		t.Fatal(err)
	}
	client, cleanup := newTestDaemon(t, s.router)
	defer cleanup()

	resp := doJSON(t, client, http.MethodPost, "/images/app:1.0/tag?repo=app&tag=2.0", "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("query tag: got %d, want 201", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if _, err := img.Get("app:2.0"); err != nil {
		t.Fatalf("app:2.0 not created from query params: %v", err)
	}

	resp = doJSON(t, client, http.MethodPost, "/images/app:2.0/tag", `{"repo":"app","tag":"3.0"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("body tag: got %d, want 201", resp.StatusCode)
	}
	_ = resp.Body.Close()
	if _, err := img.Get("app:3.0"); err != nil {
		t.Fatalf("app:3.0 not created from JSON body: %v", err)
	}
}

// TestImageGetCanonical covers GET /images/{name}/get and its /save alias.
func TestImageGetCanonical(t *testing.T) {
	s, _, img := newTestAPIServer(t)
	if err := img.SaveRecord(&image.ImageRecord{
		ID:       "img2",
		RepoTags: []string{"app:get"},
		Manifest: &registry.ManifestV2{Config: registry.ManifestBlob{Digest: "sha256:abc"}},
	}); err != nil {
		t.Fatal(err)
	}
	client, cleanup := newTestDaemon(t, s.router)
	defer cleanup()

	for _, path := range []string{"/images/app:get/get", "/images/app:get/save"} {
		resp := doJSON(t, client, http.MethodGet, path, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: got %d, want 200", path, resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/x-tar" {
			t.Errorf("%s: Content-Type = %q, want application/x-tar", path, ct)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if len(body) == 0 {
			t.Errorf("%s: empty tar stream", path)
		}
	}

	resp := doJSON(t, client, http.MethodGet, "/images/missing:get/get", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown image: got %d, want 404", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func tarBytes(t *testing.T, write func(tw *tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	write(tw)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestBuildTarContext covers POST /build with an application/x-tar body: a
// missing context is reported clearly, a context without any Dokifile is
// reported clearly, and zip-slip entries are rejected.
func TestBuildTarContext(t *testing.T) {
	s, _, _ := newTestAPIServer(t)
	client, cleanup := newTestDaemon(t, s.router)
	defer cleanup()

	resp := doJSON(t, client, http.MethodPost, "/build", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("no context: got %d, want 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "application/x-tar") {
		t.Errorf("no-context error should name the accepted forms, got %q", body)
	}

	empty := tarBytes(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "hello.txt", Mode: 0644, Size: 2})
		_, _ = tw.Write([]byte("hi"))
	})
	resp = doRequest(t, client, http.MethodPost, "/build", "application/x-tar", bytes.NewReader(empty))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("context without Dockerfile: got %d, want 400", resp.StatusCode)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "Dokifile") {
		t.Errorf("missing-dockerfile error = %q, want it to name the Dokifile", body)
	}

	evil := tarBytes(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0644, Size: 3})
		_, _ = tw.Write([]byte("pwn"))
	})
	resp = doRequest(t, client, http.MethodPost, "/build", "application/x-tar", bytes.NewReader(evil))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("zip-slip entry: got %d, want 400", resp.StatusCode)
	}
	body, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "unsafe path") {
		t.Errorf("zip-slip error = %q, want it to reject the unsafe path", body)
	}
}
