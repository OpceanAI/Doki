package podman

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	dokiruntime "github.com/OpceanAI/Doki/pkg/runtime"
)

func newTestServer(t *testing.T, deps Deps) *PodmanServer {
	t.Helper()
	srv, err := NewPodmanServer(t.TempDir(), deps)
	if err != nil {
		t.Fatalf("NewPodmanServer: %v", err)
	}
	return srv
}

// An unwired libpod surface must say it has no engine. Returning "[]" or a
// fabricated container ID is worse than an error, because the client builds
// on the lie: `podman ps` reports a clean host and `podman create` hands back
// an ID for a container that does not exist.
func TestUnwiredEndpointsFailHonestly(t *testing.T) {
	srv := newTestServer(t, Deps{})

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/libpod/containers/json", ""},
		{http.MethodPost, "/libpod/containers/create", `{"image":"busybox"}`},
		{http.MethodGet, "/libpod/images/json", ""},
		{http.MethodPost, "/libpod/images/pull?reference=busybox", ""},
		{http.MethodGet, "/libpod/volumes/json", ""},
		{http.MethodGet, "/libpod/networks/json", ""},
		{http.MethodGet, "/libpod/events", ""},
	}
	for _, c := range cases {
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		req := httptest.NewRequest(c.method, c.path, body)
		rec := httptest.NewRecorder()
		srv.route(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s: got %d, want 503 (unwired engine must not fake success)",
				c.method, c.path, rec.Code)
		}
	}
}

// Endpoints Doki deliberately does not implement must return 501, not an empty
// success payload that reads as "checked, nothing found".
func TestDeferredEndpointsReturn501(t *testing.T) {
	srv := newTestServer(t, Deps{})
	for _, path := range []string{
		"/libpod/auto-update",
		"/libpod/quadlets/json",
		"/libpod/artifacts/",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.route(rec, req)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: got %d, want 501", path, rec.Code)
		}
	}
}

func TestValidateBindSource(t *testing.T) {
	if err := validateBindSource("bind", "/"); err == nil {
		t.Error("binding the host root should be refused")
	}
	if err := validateBindSource("bind", "/etc/shadow"); err == nil {
		t.Error("binding /etc should be refused")
	}
	if err := validateBindSource("bind", "relative/path"); err == nil {
		t.Error("relative bind source should be refused")
	}
	if err := validateBindSource("bind", "/srv/../etc"); err == nil {
		t.Error("traversal in bind source should be refused")
	}
	if err := validateBindSource("bind", "/srv/data"); err != nil {
		t.Errorf("ordinary bind source refused: %v", err)
	}
	// Only bind sources name a host path; a volume source is a volume name.
	if err := validateBindSource("volume", "myvol"); err != nil {
		t.Errorf("volume source should not be path-checked: %v", err)
	}
}

func TestLibpodStateVocabulary(t *testing.T) {
	// Podman says "configured", not Docker's "created".
	if got := libpodState(common.StateCreated); got != "configured" {
		t.Errorf("StateCreated -> %q, want %q", got, "configured")
	}
	if got := libpodState(common.StateRunning); got != "running" {
		t.Errorf("StateRunning -> %q", got)
	}
	if got := libpodState(common.StateExited); got != "exited" {
		t.Errorf("StateExited -> %q", got)
	}
}

func TestToLibpodContainer(t *testing.T) {
	created := time.Now().Add(-time.Hour)
	st := &dokiruntime.ContainerState{
		ID:      "abcdef0123456789",
		Status:  common.StateRunning,
		Created: created,
		Pid:     4242,
		Config: &dokiruntime.Config{
			ImageRef:    "busybox:latest",
			ImageDigest: "sha256:deadbeef",
			Args:        []string{"sh", "-c", "sleep 1"},
			Annotations: map[string]string{"doki.name": "web"},
			Labels:      map[string]string{"env": "test"},
			Mounts:      []common.Mount{{Type: common.MountVolume, Source: "data", Target: "/data"}},
		},
	}
	c := toLibpodContainer(st)
	if len(c.Names) != 1 || c.Names[0] != "web" {
		t.Errorf("Names = %v, want [web]", c.Names)
	}
	if c.Image != "busybox:latest" {
		t.Errorf("Image = %q", c.Image)
	}
	if c.State != "running" || c.Pid != 4242 {
		t.Errorf("State = %q, Pid = %d", c.State, c.Pid)
	}
	if len(c.Mounts) != 1 || c.Mounts[0] != "/data" {
		t.Errorf("Mounts = %v", c.Mounts)
	}
	if c.Created != created.Unix() {
		t.Errorf("Created = %d, want %d", c.Created, created.Unix())
	}
}

// A container with no explicit name still needs one; podman clients index by
// name and a blank one collides across containers.
func TestContainerNameFallsBackToShortID(t *testing.T) {
	st := &dokiruntime.ContainerState{ID: "0123456789abcdef0123"}
	if got := containerName(st); got != "0123456789ab" {
		t.Errorf("containerName = %q, want 0123456789ab", got)
	}
}

func TestEnvMapToSlice(t *testing.T) {
	if got := envMapToSlice(nil); got != nil {
		t.Errorf("empty env should stay nil, got %v", got)
	}
	got := envMapToSlice(map[string]string{"A": "1"})
	if len(got) != 1 || got[0] != "A=1" {
		t.Errorf("envMapToSlice = %v", got)
	}
}

// The libpod list shape is an array of objects, not Docker's wrapper object.
// A client that unmarshals into []struct must not choke on it.
func TestContainersListShape(t *testing.T) {
	srv := newTestServer(t, Deps{})
	req := httptest.NewRequest(http.MethodGet, "/libpod/containers/json", nil)
	rec := httptest.NewRecorder()
	srv.route(rec, req)
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not JSON: %v", err)
	}
	if _, ok := body["cause"]; !ok {
		t.Error("error payload should carry libpod's cause/message/response shape")
	}
}
