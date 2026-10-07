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
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
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

// Untag drops one name from an image. As long as another name still points at
// it the image data must survive; only the last name turns it into a removal.
func TestUntagKeepsRemainingTags(t *testing.T) {
	imgStore, err := image.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := imgStore.SaveRecord(&image.ImageRecord{
		ID:       "img1234",
		RepoTags: []string{"app:1.0", "app:2.0"},
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, Deps{Images: imgStore})

	rec := httptest.NewRecorder()
	srv.route(rec, httptest.NewRequest(http.MethodPost, "/libpod/images/app:1.0/untag", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("untag: got %d, want 201", rec.Code)
	}

	// The image data survives because app:2.0 still points at it.
	got, err := imgStore.Get("app:2.0")
	if err != nil {
		t.Fatalf("image was deleted although app:2.0 remains: %v", err)
	}
	if len(got.RepoTags) != 1 || got.RepoTags[0] != "app:2.0" {
		t.Errorf("RepoTags = %v, want [app:2.0]", got.RepoTags)
	}
	if _, err := imgStore.Get("app:1.0"); !common.IsNotFound(err) {
		t.Errorf("app:1.0 still resolves after untag: %v", err)
	}

	// Removing the last name removes the image itself.
	rec = httptest.NewRecorder()
	srv.route(rec, httptest.NewRequest(http.MethodPost, "/libpod/images/app:2.0/untag", nil))
	if rec.Code != http.StatusCreated {
		t.Fatalf("last untag: got %d, want 201", rec.Code)
	}
	if _, err := imgStore.Get("app:2.0"); err == nil {
		t.Error("app:2.0 still resolves after its image was removed")
	}
	if got, err := imgStore.Get("img1234"); err == nil && len(got.RepoTags) > 0 {
		t.Errorf("removed image still carries names: %v", got.RepoTags)
	}
}

// TestNetworkConnectHonorsAliasAndMAC: connect must respect the static_ip,
// aliases and MAC address from the request body instead of dropping them.
func TestNetworkConnectHonorsAliasAndMAC(t *testing.T) {
	rt := dokiruntime.NewRuntime(t.TempDir(), nil)
	if err := rt.SaveState(&dokiruntime.ContainerState{
		ID:      "cont1",
		Status:  common.StateCreated,
		Created: time.Now(),
		Config:  &dokiruntime.Config{},
	}); err != nil {
		t.Fatal(err)
	}
	netMgr, err := network.NewManager(t.TempDir(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, Deps{Runtime: rt, Network: netMgr})

	rec := httptest.NewRecorder()
	body := `{"container":"cont1","static_ip":"10.89.0.55","static_mac":"aa:bb:cc:dd:ee:ff","aliases":["web","db"]}`
	srv.route(rec, httptest.NewRequest(http.MethodPost, "/libpod/networks/bridge/connect", strings.NewReader(body)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("connect: got %d, want 204 (body %s)", rec.Code, rec.Body.String())
	}

	nw, err := netMgr.GetNetwork("bridge")
	if err != nil {
		t.Fatal(err)
	}
	ep := nw.Containers["cont1"]
	if ep == nil {
		t.Fatal("container has no endpoint after connect")
	}
	if ep.IPv4Address != "10.89.0.55" {
		t.Errorf("IPv4Address = %q, want 10.89.0.55", ep.IPv4Address)
	}
	if ep.MacAddress != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MacAddress = %q, want aa:bb:cc:dd:ee:ff", ep.MacAddress)
	}
	alias := map[string]bool{}
	for _, a := range ep.Aliases {
		alias[a] = true
	}
	if !alias["web"] || !alias["db"] {
		t.Errorf("Aliases = %v, want [web db]", ep.Aliases)
	}

	rec = httptest.NewRecorder()
	srv.route(rec, httptest.NewRequest(http.MethodPost, "/libpod/networks/bridge/disconnect",
		strings.NewReader(`{"container":"cont1"}`)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect: got %d, want 204", rec.Code)
	}
	nw, err = netMgr.GetNetwork("bridge")
	if err != nil {
		t.Fatal(err)
	}
	if ep := nw.Containers["cont1"]; ep != nil {
		t.Errorf("endpoint survives disconnect: %+v", ep)
	}
}

// runWait fires a wait request and fails the test if it does not answer
// within two seconds: the whole point of the condition handling is that wait
// must not hang.
func runWait(t *testing.T, srv *PodmanServer, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	done := make(chan struct{})
	go func() {
		srv.route(rec, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("wait %s hung", path)
	}
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

// TestWaitConditions: next-exit must resolve at once for a container that is
// not running (it has no next exit to wait for) instead of hanging forever.
func TestWaitConditions(t *testing.T) {
	rt := dokiruntime.NewRuntime(t.TempDir(), nil)
	if err := rt.SaveState(&dokiruntime.ContainerState{
		ID:       "w1",
		Status:   common.StateExited,
		Created:  time.Now(),
		ExitCode: 3,
		Config:   &dokiruntime.Config{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.SaveState(&dokiruntime.ContainerState{
		ID:      "w2",
		Status:  common.StateRunning,
		Created: time.Now(),
		Config:  &dokiruntime.Config{},
	}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, Deps{Runtime: rt})

	code, body := runWait(t, srv, "/libpod/containers/w1/wait?condition=next-exit&interval=10ms")
	if code != http.StatusOK || body != "3" {
		t.Errorf("next-exit on stopped container = (%d, %q), want (200, 3)", code, body)
	}

	code, _ = runWait(t, srv, "/libpod/containers/w2/wait?condition=running&interval=10ms")
	if code != http.StatusOK {
		t.Errorf("running condition = %d, want 200", code)
	}

	rec := httptest.NewRecorder()
	srv.route(rec, httptest.NewRequest(http.MethodPost,
		"/libpod/containers/w1/wait?condition=whenever", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unsupported condition = %d, want 400", rec.Code)
	}
}
