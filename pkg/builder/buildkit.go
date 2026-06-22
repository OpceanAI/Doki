package builder

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"time"
)

// BuildKitClient communicates with a BuildKit daemon for advanced builds.
// BuildKit provides:
//   - Parallel stage execution
//   - Content-addressable layer caching
//   - Advanced mount types (cache, secret, ssh)
//   - Multi-platform builds
//   - Build graph optimization
type BuildKitClient struct {
	addr    string // BuildKit daemon address (e.g., "unix:///run/buildkit/buildkitd.sock")
	log     *slog.Logger
	timeout time.Duration
}

// NewBuildKitClient creates a new BuildKit client.
func NewBuildKitClient(addr string) *BuildKitClient {
	return &BuildKitClient{
		addr:    addr,
		log:     slog.Default().With("component", "builder.buildkit"),
		timeout: 30 * time.Minute,
	}
}

// IsAvailable checks if a BuildKit daemon is reachable.
func (c *BuildKitClient) IsAvailable() bool {
	if c.addr == "" {
		return false
	}
	// Try to connect to the socket.
	network, address := parseBuildKitAddr(c.addr)
	conn, err := net.DialTimeout(network, address, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Build sends a build request to the BuildKit daemon.
// This is the "delegation" path: instead of building locally,
// we ask BuildKit to do it (which is faster and supports more features).
func (c *BuildKitClient) Build(ctx context.Context, cfg *BuildConfig) error {
	if !c.IsAvailable() {
		return fmt.Errorf("buildkit daemon not available at %s", c.addr)
	}

	c.log.Info("delegating build to buildkit",
		"addr", c.addr,
		"context", cfg.Context,
		"tags", cfg.Tags,
	)

	// Use buildctl CLI to communicate with BuildKit.
	// This avoids importing the complex BuildKit Go SDK.
	buildctlPath, err := exec.LookPath("buildctl")
	if err != nil {
		return fmt.Errorf("buildctl not found: %w", err)
	}

	args := []string{
		"--addr", c.addr,
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + cfg.Context,
		"--local", "dockerfile=" + cfg.Context,
	}

	// Add tags.
	for _, tag := range cfg.Tags {
		args = append(args, "--output", "type=image,name="+tag)
	}

	// Add build args.
	for k, v := range cfg.BuildArgs {
		args = append(args, "--opt", "build-arg:"+k+"="+v)
	}

	// Add platform.
	if cfg.Platform != "" {
		args = append(args, "--opt", "platform="+cfg.Platform)
	}

	// Add no-cache.
	if cfg.NoCache {
		args = append(args, "--no-cache")
	}

	cmd := exec.CommandContext(ctx, buildctlPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("buildctl build: %w", err)
	}

	c.log.Info("buildkit build completed")
	return nil
}

// Solve sends a solve request to BuildKit.
// This is the advanced API for custom build graphs.
func (c *BuildKitClient) Solve(_ context.Context, _ []byte) error {
	if !c.IsAvailable() {
		return fmt.Errorf("buildkit daemon not available")
	}

	// For now, we use buildctl CLI.
	// A future version could use the gRPC API directly.
	return fmt.Errorf("solve API not yet implemented")
}

// Status returns the BuildKit daemon status.
func (c *BuildKitClient) Status() (map[string]interface{}, error) {
	if !c.IsAvailable() {
		return nil, fmt.Errorf("buildkit daemon not available")
	}
	// TODO: implement buildctl debug info
	return map[string]interface{}{
		"addr":      c.addr,
		"available": true,
	}, nil
}

// DetectBuildKit searches for a BuildKit daemon.
// Returns the address if found, empty string otherwise.
func DetectBuildKit() string {
	// Check common socket paths.
	paths := []string{
		"unix:///run/buildkit/buildkitd.sock",
		"unix:///run/user/" + fmt.Sprintf("%d", os.Getuid()) + "/buildkit/buildkitd.sock",
		"unix:///var/run/buildkit/buildkitd.sock",
		"unix:///tmp/buildkit/buildkitd.sock",
	}
	for _, addr := range paths {
		c := NewBuildKitClient(addr)
		if c.IsAvailable() {
			return addr
		}
	}

	// Check if buildctl is in PATH.
	if _, err := exec.LookPath("buildctl"); err == nil {
		// Try default address.
		return "unix:///run/buildkit/buildkitd.sock"
	}

	return ""
}

// StartBuildKitDaemon starts a BuildKit daemon if not already running.
// This is useful for environments where BuildKit is not pre-installed.
func StartBuildKitDaemon(ctx context.Context) (string, error) {
	// Check if already running.
	addr := DetectBuildKit()
	if addr != "" {
		return addr, nil
	}

	// Check if buildkitd is available.
	buildkitdPath, err := exec.LookPath("buildkitd")
	if err != nil {
		return "", fmt.Errorf("buildkitd not found: %w", err)
	}

	// Start buildkitd in the background.
	sockPath := "/tmp/doki-buildkit/buildkitd.sock"
	_ = os.MkdirAll("/tmp/doki-buildkit", 0755)

	cmd := exec.CommandContext(ctx, buildkitdPath,
		"--addr", "unix://"+sockPath,
		"--root", "/tmp/doki-buildkit/data",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start buildkitd: %w", err)
	}

	// Wait for socket to appear.
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(sockPath); err == nil {
			return "unix://" + sockPath, nil
		}
	}

	return "", fmt.Errorf("timeout waiting for buildkitd socket")
}

func parseBuildKitAddr(addr string) (network, address string) {
	if len(addr) > 7 && addr[:7] == "unix://" {
		return "unix", addr[7:]
	}
	if len(addr) > 6 && addr[:6] == "tcp://" {
		return "tcp", addr[6:]
	}
	return "unix", addr
}

// BuildKitProgressWriter parses BuildKit progress output and reports it.
type BuildKitProgressWriter struct {
	log *slog.Logger
}

func NewBuildKitProgressWriter(log *slog.Logger) *BuildKitProgressWriter {
	return &BuildKitProgressWriter{log: log}
}

func (w *BuildKitProgressWriter) Write(p []byte) (n int, err error) {
	w.log.Debug("buildkit", "output", string(p))
	return len(p), nil
}

// BuildKitBuildConfig extends BuildConfig with BuildKit-specific options.
type BuildKitBuildConfig struct {
	*BuildConfig

	// BuildKitAddr is the BuildKit daemon address.
	// If empty, falls back to local build.
	BuildKitAddr string

	// ExportCache is the cache export destination.
	ExportCache string

	// ImportCache is the cache import source.
	ImportCache string

	// LLB is the raw LLB definition (advanced).
	LLB []byte
}

// BuildWithBuildKit builds using BuildKit if available, falls back to local.
func (b *Builder) BuildWithBuildKit(ctx context.Context, cfg *BuildKitBuildConfig) error {
	// Try BuildKit first.
	if cfg.BuildKitAddr != "" {
		client := NewBuildKitClient(cfg.BuildKitAddr)
		if client.IsAvailable() {
			return client.Build(ctx, cfg.BuildConfig)
		}
		b.log.Warn("buildkit not available, falling back to local build")
	}

	// Fallback to local build.
	return b.Build(cfg.BuildConfig)
}

// GetBuildWriter returns an io.Writer for build output.
func (b *Builder) GetBuildWriter() io.Writer {
	return os.Stderr
}
