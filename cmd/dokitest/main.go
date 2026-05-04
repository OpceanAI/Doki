package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpceanAI/Doki/pkg/builder"
	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/runtime"
)

func main() {
	fmt.Println("=== DOKI HELLO WORLD TEST ===")
	fmt.Println()

	// 1. Parse Dokifile.
	fmt.Println("[1] Parsing Dokifile...")
	data, err := os.ReadFile("test/Dokifile")
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	parser := builder.NewDokifileParser()
	if err := parser.Parse(data); err != nil {
		fmt.Printf("ERROR parsing: %v\n", err)
		os.Exit(1)
	}

	stages := parser.GetStages()
	fmt.Printf("  ✓ Parsed %d build stage(s)\n", len(stages))
	for _, stage := range stages {
		fmt.Printf("    Stage: FROM %s, %d instructions\n", stage.From, len(stage.Instructions))
		for _, inst := range stage.Instructions {
			fmt.Printf("      [%2d] %s %v\n", inst.LineNum, inst.Type, inst.Args)
		}
	}
	fmt.Println()

	// 2. Setup data directories.
	fmt.Println("[2] Setting up data directories...")
	dataDir := "/data/data/com.termux/files/tmp/doki-test"
	for _, dir := range []string{
		filepath.Join(dataDir, "images", "manifests"),
		filepath.Join(dataDir, "images", "layers"),
		filepath.Join(dataDir, "images", "blobs"),
		filepath.Join(dataDir, "containers"),
		filepath.Join(dataDir, "runtimes"),
		filepath.Join(dataDir, "networks"),
	} {
		common.EnsureDir(dir)
	}
	fmt.Printf("  ✓ Data dir: %s\n", dataDir)
	fmt.Println()

	// 3. Initialize image store.
	fmt.Println("[3] Initializing image store...")
	imgStore, err := image.NewStore(filepath.Join(dataDir, "images"))
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✓ Image store ready")
	fmt.Println()

	// 4. Pull alpine image.
	fmt.Println("[4] Pulling alpine:latest...")
	record, err := imgStore.Pull("alpine:latest")
	if err != nil {
		fmt.Printf("  ⚠ Pull failed (expected if no network): %v\n", err)
		fmt.Println("  Skipping pull test (offline mode)")
	} else {
		fmt.Printf("  ✓ Pulled: %s\n", record.ID[:12])
		fmt.Printf("    Repo: %s\n", record.RepoTags[0])
		fmt.Printf("    Arch: %s/%s\n", record.Architecture, record.OS)
	}

	// 5. Create and start a simple container.
	fmt.Println()
	fmt.Println("[5] Create container...")
	containerID := common.GenerateID(64)

	cfg := &runtime.Config{
		ID:   containerID,
		Args: []string{"echo", "Hello World from Doki!"},
		Env:  []string{"DOKI_TEST=true"},
	}

	rt := runtime.NewRuntime(filepath.Join(dataDir, "runtimes"), nil)
	state, err := rt.Create(cfg)
	if err != nil {
		fmt.Printf("  ERROR: %v\n", err)
	} else {
		fmt.Printf("  ✓ Container created: %s\n", state.ID[:12])
		fmt.Printf("    State: %s\n", state.Status)
		fmt.Printf("    Created: %s\n", state.Created)
	}

	// 6. List containers.
	fmt.Println()
	fmt.Println("[6] Container list:")
	containers, _ := rt.List()
	for _, c := range containers {
		fmt.Printf("  %s  %s\n", c.ID[:12], c.Status)
	}

	// 7. Version info.
	fmt.Println()
	fmt.Println("[7] Doki Version:")
	v := common.GetVersion()
	fmt.Printf("  Version:    %s\n", v.Version)
	fmt.Printf("  API:        %s\n", v.APIVersion)
	fmt.Printf("  Go:         %s\n", v.GoVersion)
	fmt.Printf("  OS/Arch:    %s/%s\n", v.OS, v.Arch)

	// 8. Supported Dokifile instructions.
	fmt.Println()
	fmt.Println("[8] Supported Dokifile instructions:")
	for _, inst := range builder.ListSupportedInstructions() {
		fmt.Printf("  - %s\n", inst)
	}

	fmt.Println()
	fmt.Println("=== TEST COMPLETE ===")
}
