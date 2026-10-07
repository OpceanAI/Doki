package main

import (
	"strings"
	"testing"

	"github.com/OpceanAI/Doki/pkg/registry"
)

const goodDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

// TestCheckManifestValid passes a well-formed manifest through the validator.
func TestCheckManifestValid(t *testing.T) {
	m := &registry.ManifestV2{
		SchemaVersion: 2,
		Config:        registry.ManifestBlob{Digest: goodDigest, Size: 100},
		Layers: []registry.ManifestBlob{
			{Digest: goodDigest, Size: 10},
			{Digest: goodDigest, Size: 20},
		},
	}
	v := &validator{name: "manifest"}
	checkManifest(v, m, goodDigest)
	if err := v.err(); err != nil {
		t.Fatalf("checkManifest(valid) = %v, want nil", err)
	}
}

// TestCheckManifestRejects drives each structural invariant and asserts the
// validator reports a failure for every violation.
func TestCheckManifestRejects(t *testing.T) {
	cases := []struct {
		name string
		m    *registry.ManifestV2
	}{
		{"nil manifest", nil},
		{"wrong schema version", &registry.ManifestV2{
			SchemaVersion: 1,
			Config:        registry.ManifestBlob{Digest: goodDigest, Size: 1},
			Layers:        []registry.ManifestBlob{{Digest: goodDigest, Size: 1}},
		}},
		{"empty layers", &registry.ManifestV2{
			SchemaVersion: 2,
			Config:        registry.ManifestBlob{Digest: goodDigest, Size: 1},
		}},
		{"bad config digest", &registry.ManifestV2{
			SchemaVersion: 2,
			Config:        registry.ManifestBlob{Digest: "sha256:short", Size: 1},
			Layers:        []registry.ManifestBlob{{Digest: goodDigest, Size: 1}},
		}},
		{"empty config size", &registry.ManifestV2{
			SchemaVersion: 2,
			Config:        registry.ManifestBlob{Digest: goodDigest, Size: 0},
			Layers:        []registry.ManifestBlob{{Digest: goodDigest, Size: 1}},
		}},
		{"bad layer digest", &registry.ManifestV2{
			SchemaVersion: 2,
			Config:        registry.ManifestBlob{Digest: goodDigest, Size: 1},
			Layers:        []registry.ManifestBlob{{Digest: "not-a-digest", Size: 1}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &validator{name: tc.name}
			checkManifest(v, tc.m, goodDigest)
			if err := v.err(); err == nil {
				t.Errorf("checkManifest(%s) = nil, want assertion failure", tc.name)
			}
		})
	}

	// The manifest digest itself must be validated too.
	v := &validator{name: "digest"}
	checkManifest(v, &registry.ManifestV2{
		SchemaVersion: 2,
		Config:        registry.ManifestBlob{Digest: goodDigest, Size: 1},
		Layers:        []registry.ManifestBlob{{Digest: goodDigest, Size: 1}},
	}, "bogus")
	if err := v.err(); err == nil {
		t.Error("checkManifest with bad top-level digest = nil, want assertion failure")
	}
}

// TestCheckImageRef asserts reference validation catches missing parts.
func TestCheckImageRef(t *testing.T) {
	ref, err := registry.ParseImageRef(testImage)
	if err != nil {
		t.Fatalf("ParseImageRef(%q) error = %v", testImage, err)
	}
	v := &validator{name: "image-ref"}
	checkImageRef(v, ref)
	if err := v.err(); err != nil {
		t.Errorf("checkImageRef(valid) = %v, want nil", err)
	}

	broken := &registry.ImageRef{Registry: "", Name: "", Tag: ""}
	v = &validator{name: "image-ref-broken"}
	checkImageRef(v, broken)
	if err := v.err(); err == nil {
		t.Error("checkImageRef(empty) = nil, want assertion failure")
	}
}

// TestParseImageRefParts pins down the reference parsing the suite relies on.
func TestParseImageRefParts(t *testing.T) {
	ref, err := registry.ParseImageRef("alpine:latest")
	if err != nil {
		t.Fatalf("ParseImageRef error = %v", err)
	}
	if ref.Name != "library/alpine" || ref.Tag != "latest" || ref.Registry == "" {
		t.Errorf("ParseImageRef(alpine:latest) = %+v, want library/alpine:latest on the default registry", ref)
	}
}

// TestDigestPattern checks the digest grammar used by the assertions.
func TestDigestPattern(t *testing.T) {
	if !digestPattern.MatchString(goodDigest) {
		t.Errorf("digestPattern rejects valid digest %q", goodDigest)
	}
	for _, bad := range []string{"", "sha256:", "sha256:xyz", "sha512:" + strings.Repeat("a", 64), strings.Repeat("a", 64)} {
		if digestPattern.MatchString(bad) {
			t.Errorf("digestPattern accepts invalid digest %q", bad)
		}
	}
}

// TestCheckConfig asserts the config blob must be non-empty.
func TestCheckConfig(t *testing.T) {
	v := &validator{name: "config"}
	checkConfig(v, []byte(`{"architecture":"arm64"}`))
	if err := v.err(); err != nil {
		t.Errorf("checkConfig(non-empty) = %v, want nil", err)
	}
	v = &validator{name: "config-empty"}
	checkConfig(v, nil)
	if err := v.err(); err == nil {
		t.Error("checkConfig(empty) = nil, want assertion failure")
	}
}
