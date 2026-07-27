package kubelet

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

func testKubelet(t *testing.T) *Kubelet {
	t.Helper()
	return &Kubelet{
		store:  store.NewMemoryStore(),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func putObj(t *testing.T, s store.Store, key string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(key, &store.StoredObject{Value: data}); err != nil {
		t.Fatal(err)
	}
}

// A ConfigMap volume must materialize each data key as a file the container can
// read. Before K12 these volumes were skipped and the container came up with an
// empty mount and misbehaved silently.
func TestProjectConfigMap(t *testing.T) {
	k := testKubelet(t)
	cm := k8s.ConfigMap{Data: map[string]string{
		"app.conf":  "key=value\n",
		"log.level": "debug",
	}}
	cm.Name = "myconfig"
	putObj(t, k.store, store.KeyFor("", "configmaps", "default", "myconfig"), cm)

	pod := &k8s.Pod{Spec: k8s.PodSpec{}}
	pod.Namespace = "default"
	pod.UID = "pod-uid-1"
	vol := k8s.Volume{Name: "config"}
	vol.ConfigMap = &k8s.ConfigMapVolumeSource{}
	vol.ConfigMap.Name = "myconfig"

	dir := k.projectConfigMap(pod, vol)
	if dir == "" {
		t.Fatal("projectConfigMap returned empty dir")
	}
	got, err := os.ReadFile(filepath.Join(dir, "app.conf"))
	if err != nil {
		t.Fatalf("read projected file: %v", err)
	}
	if string(got) != "key=value\n" {
		t.Errorf("app.conf = %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "log.level")); string(b) != "debug" {
		t.Errorf("log.level = %q", b)
	}
}

// An explicit items[] mapping restricts and renames the projected keys.
func TestProjectConfigMapItems(t *testing.T) {
	k := testKubelet(t)
	cm := k8s.ConfigMap{Data: map[string]string{"a": "1", "b": "2", "c": "3"}}
	cm.Name = "cm"
	putObj(t, k.store, store.KeyFor("", "configmaps", "default", "cm"), cm)

	pod := &k8s.Pod{}
	pod.Namespace = "default"
	pod.UID = "uid"
	vol := k8s.Volume{Name: "v"}
	vol.ConfigMap = &k8s.ConfigMapVolumeSource{Items: []k8s.KeyToPath{{Key: "a", Path: "renamed/a.txt"}}}
	vol.ConfigMap.Name = "cm"

	dir := k.projectConfigMap(pod, vol)
	if b, _ := os.ReadFile(filepath.Join(dir, "renamed/a.txt")); string(b) != "1" {
		t.Errorf("renamed a.txt = %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "b")); !os.IsNotExist(err) {
		t.Error("key b should not be projected when items[] is set")
	}
}

// Secret Data is base64 in JSON and decodes into bytes; the projected file must
// contain the decoded content.
func TestProjectSecret(t *testing.T) {
	k := testKubelet(t)
	sec := k8s.Secret{Data: map[string][]byte{"token": []byte("s3cr3t")}}
	sec.Name = "creds"
	putObj(t, k.store, store.KeyFor("", "secrets", "default", "creds"), sec)

	pod := &k8s.Pod{}
	pod.Namespace = "default"
	pod.UID = "uid2"
	vol := k8s.Volume{Name: "s"}
	vol.Secret = &k8s.SecretVolumeSource{SecretName: "creds"}

	dir := k.projectSecret(pod, vol)
	if dir == "" {
		t.Fatal("projectSecret returned empty dir")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "token")); string(b) != "s3cr3t" {
		t.Errorf("token = %q, want s3cr3t", b)
	}
}

// A missing, non-optional ConfigMap yields no directory (the pod's volume is
// deferred) rather than a bogus empty mount.
func TestProjectConfigMapMissing(t *testing.T) {
	k := testKubelet(t)
	pod := &k8s.Pod{}
	pod.Namespace = "default"
	pod.UID = "uid3"
	vol := k8s.Volume{Name: "v"}
	vol.ConfigMap = &k8s.ConfigMapVolumeSource{}
	vol.ConfigMap.Name = "nope"
	if dir := k.projectConfigMap(pod, vol); dir != "" {
		t.Errorf("missing configMap should return empty dir, got %q", dir)
	}
}
