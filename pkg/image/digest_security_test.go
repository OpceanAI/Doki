package image

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpceanAI/Doki/pkg/registry"
)

// TestValidateManifestDigests ensures a malicious digest that tries to escape
// the layers/ directory (write-what-where, CRIT-1) is rejected before any
// filesystem path is built from it.
func TestValidateManifestDigests(t *testing.T) {
	good := &registry.ManifestV2{
		Config: registry.ManifestBlob{Digest: "sha256:" + "a" + repeat("0", 63)},
		Layers: []registry.ManifestBlob{
			{Digest: "sha256:" + repeat("b", 64)},
		},
	}
	if err := validateManifestDigests(good); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}

	bad := []*registry.ManifestV2{
		{Config: registry.ManifestBlob{Digest: "sha256:../../../../etc/passwd"}},
		{Config: registry.ManifestBlob{Digest: "../../../../home/user/.bashrc"}},
		{
			Config: registry.ManifestBlob{Digest: "sha256:" + repeat("a", 64)},
			Layers: []registry.ManifestBlob{{Digest: "sha256:short"}},
		},
		{
			Config: registry.ManifestBlob{Digest: "sha256:" + repeat("a", 64)},
			Layers: []registry.ManifestBlob{{Digest: "/etc/cron.d/evil"}},
		},
	}
	for i, m := range bad {
		if err := validateManifestDigests(m); err == nil {
			t.Errorf("case %d: malicious manifest accepted", i)
		}
	}
}

// TestDownloadLayerRejectsBadDigest confirms a poisoned digest never touches the
// filesystem, and that no file is planted outside layers/.
func TestDownloadLayerRejectsBadDigest(t *testing.T) {
	dir := t.TempDir()
	s := &Store{root: dir}
	layer := registry.ManifestBlob{Digest: "sha256:../../../../pwned"}
	err := s.downloadLayer("reg", "img", layer, s.layerPath(layer.Digest))
	if err == nil {
		t.Fatal("expected error for malicious digest")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "..", "pwned")); statErr == nil {
		t.Fatal("file was planted outside layers/")
	}
}

func repeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
