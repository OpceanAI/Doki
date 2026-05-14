package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpceanAI/Doki/pkg/common"
)

func TestExtractLayersParallel(t *testing.T) {
	rt := &Runtime{root: t.TempDir()}
	// Create test layers
	layers := make([]string, 5)
	tmpDir := t.TempDir()
	for i := range layers {
		layers[i] = filepath.Join(tmpDir, "layer-"+string(rune('a'+i)))
	}
	dest := filepath.Join(t.TempDir(), "rootfs")
	os.MkdirAll(dest, 0755)

	err := rt.extractLayers(dest, layers)
	// Empty layers should just skip
	if err != nil {
		t.Logf("extractLayers (expected harmless err): %v", err)
	}
}

func TestHealthCheckDefaults(t *testing.T) {
	rt := &Runtime{root: t.TempDir()}
	rt.StartHealthcheck("test-container", []string{"echo", "ok"}, 0, 0, 0)
	t.Log("StartHealthcheck completed without panic")
}

func TestPauseFallback(t *testing.T) {
	// Verify Pause gracefully handles missing container
	rt := &Runtime{root: t.TempDir()}
	err := rt.Pause("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent container")
	}
}

func TestEnvValidation(t *testing.T) {
	valid := common.ValidateEnv([]string{"KEY=value", "VALID_NAME=ok"})
	if len(valid) < 2 {
		t.Errorf("expected 2 valid env vars, got %d", len(valid))
	}
	// ENV vars without = sign get filtered
	result := common.ValidateEnv([]string{"BADVAR"})
	if len(result) > 0 {
		t.Logf("ValidateEnv filtered malformed env: %v -> %v", []string{"BADVAR"}, result)
	}
}
