package main

import (
	"fmt"
	"os"

	"github.com/OpceanAI/Doki/pkg/registry"
)

func main() {
	client := registry.NewClient(false)

	fmt.Println("▶ Getting auth token for alpine...")
	ref, _ := registry.ParseImageRef("alpine:latest")
	fmt.Printf("  Registry: %s, Name: %s, Tag: %s\n", ref.Registry, ref.Name, ref.Tag)

	fmt.Println("▶ Resolving manifest...")
	manifest, digest, err := client.ResolveManifest(ref.Registry, ref.Name, ref.Tag)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  ✓ Manifest: schema v%d, digest: %s\n", manifest.SchemaVersion, digest)
	fmt.Printf("  Config: %s (%d bytes)\n", manifest.Config.Digest, manifest.Config.Size)
	fmt.Printf("  Layers: %d\n", len(manifest.Layers))
	for i, layer := range manifest.Layers {
		fmt.Printf("    [%d] %s (%d bytes)\n", i, layer.Digest[:30], layer.Size)
	}

	fmt.Println("▶ Getting image config...")
	config, err := client.GetConfig(ref.Registry, ref.Name, manifest)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ Config: %d bytes\n", len(config))
}

// Test also node:22-alpine
func testNode() {
	client := registry.NewClient(false)
	fmt.Println("\n▶ Pulling node:22-alpine...")
	ref, _ := registry.ParseImageRef("node:22-alpine")
	manifest, digest, err := client.ResolveManifest(ref.Registry, ref.Name, ref.Tag)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		return
	}

	fmt.Printf("  ✓ Manifest digest: %s\n", digest)
	fmt.Printf("  Layers: %d\n", len(manifest.Layers))
	fmt.Printf("  Config: %s\n", manifest.Config.Digest)
}
