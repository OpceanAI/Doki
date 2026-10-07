// Package main is the Doki registry test suite. It resolves the manifest and
// config of a reference image through the real registry client and asserts the
// structural invariants of what comes back. Every check reports a clear
// message and the suite exits non-zero on the first failure.
package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/OpceanAI/Doki/pkg/registry"
)

// testImage is the reference image the suite resolves.
const testImage = "alpine:latest"

// digestPattern matches "sha256:" followed by 64 hex characters.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// validator accumulates assertion failures so the suite reports all problems
// before exiting.
type validator struct {
	name string
	fail int
}

// check asserts cond and reports a clear message when it does not hold.
func (v *validator) check(cond bool, format string, args ...interface{}) {
	if cond {
		return
	}
	v.fail++
	fmt.Printf("  ASSERT FAIL: %s: %s\n", v.name, fmt.Sprintf(format, args...))
}

// err returns a summary error when any assertion failed.
func (v *validator) err() error {
	if v.fail == 0 {
		return nil
	}
	return fmt.Errorf("%s: %d assertion(s) failed", v.name, v.fail)
}

// checkImageRef asserts the parsed reference carries all three parts.
func checkImageRef(v *validator, ref *registry.ImageRef) {
	v.check(ref != nil, "parsed reference is nil")
	if ref == nil {
		return
	}
	v.check(ref.Registry != "", "registry is empty")
	v.check(ref.Name != "", "name is empty")
	v.check(ref.Tag != "", "tag is empty")
}

// checkManifest asserts the manifest is schema v2 with a config and at least
// one layer, and that every digest is well-formed.
func checkManifest(v *validator, m *registry.ManifestV2, digest string) {
	v.check(m != nil, "manifest is nil")
	if m == nil {
		return
	}
	v.check(m.SchemaVersion == 2, "manifest schema version = %d, want 2", m.SchemaVersion)
	v.check(digestPattern.MatchString(digest), "manifest digest %q is not sha256:<64 hex>", digest)
	v.check(digestPattern.MatchString(m.Config.Digest), "config digest %q is not sha256:<64 hex>", m.Config.Digest)
	v.check(m.Config.Size > 0, "config size = %d, want > 0", m.Config.Size)
	v.check(len(m.Layers) > 0, "manifest has no layers")
	for i, l := range m.Layers {
		v.check(digestPattern.MatchString(l.Digest), "layer %d digest %q is not sha256:<64 hex>", i, l.Digest)
		v.check(l.Size >= 0, "layer %d size = %d, want >= 0", i, l.Size)
	}
}

// checkConfig asserts the image config blob is non-empty JSON-looking data.
func checkConfig(v *validator, config []byte) {
	v.check(len(config) > 0, "config blob is empty")
}

func main() {
	fmt.Println("▶ Getting auth token for alpine...")
	ref, err := registry.ParseImageRef(testImage)
	if err != nil {
		fmt.Printf("ERROR: cannot parse image reference %q: %v\n", testImage, err)
		os.Exit(1)
	}
	fmt.Printf("  Registry: %s, Name: %s, Tag: %s\n", ref.Registry, ref.Name, ref.Tag)

	v := &validator{name: "image-ref"}
	checkImageRef(v, ref)
	if err := v.err(); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	client := registry.NewClient(false)

	fmt.Println("▶ Resolving manifest...")
	manifest, digest, err := client.ResolveManifest(ref.Registry, ref.Name, ref.Tag)
	if err != nil {
		fmt.Printf("ERROR: cannot resolve manifest (registry unreachable or image missing): %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  ok Manifest: schema v%d, digest: %s\n", manifest.SchemaVersion, digest)
	fmt.Printf("  Config: %s (%d bytes)\n", manifest.Config.Digest, manifest.Config.Size)
	fmt.Printf("  Layers: %d\n", len(manifest.Layers))
	for i, layer := range manifest.Layers {
		d := layer.Digest
		if len(d) > 30 {
			d = d[:30]
		}
		fmt.Printf("    [%d] %s (%d bytes)\n", i, d, layer.Size)
	}

	fmt.Println("▶ Checking manifest structure...")
	v = &validator{name: "manifest"}
	checkManifest(v, manifest, digest)
	if err := v.err(); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ok Manifest structure is valid")

	fmt.Println("▶ Getting image config...")
	config, err := client.GetConfig(ref.Registry, ref.Name, manifest)
	if err != nil {
		fmt.Printf("ERROR: cannot get image config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ok Config: %d bytes\n", len(config))

	v = &validator{name: "config"}
	checkConfig(v, config)
	if err := v.err(); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("▶ All registry checks passed")
}
