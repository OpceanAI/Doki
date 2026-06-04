package common

import (
	"fmt"
	"runtime"
)

// Build-time injected variables. Override with -ldflags "-X":
//
//	-X github.com/OpceanAI/Doki/pkg/common.DokiGitCommit=$(git rev-parse --short HEAD)
//	-X github.com/OpceanAI/Doki/pkg/common.DokiBuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)
//	-X github.com/OpceanAI/Doki/pkg/common.DokiVersion=$(git describe --tags --always --dirty)
var (
	// Version is the effective runtime version. It defaults to DokiVersion
	// but is overridden at build time via -ldflags.
	Version = DokiVersion
	// GitCommit is the short git commit SHA at build time. "unknown" in dev.
	GitCommit = "unknown"
	// BuildDate is the RFC3339 build timestamp. "unknown" in dev.
	BuildDate = "unknown"
	// BuildUser is the user that built the binary (from $USER). "unknown" in CI.
	BuildUser = "unknown"
	// GoVersion is captured at init() (not via ldflags) to keep the build simple.
	GoVersion = runtime.Version()
)

// VersionInfo holds the structured version payload returned by the
// /v1.41/version endpoint and emitted on startup logs.
type VersionInfo struct {
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
	BuildDate     string `json:"BuildDate,omitempty"`
	BuildUser     string `json:"BuildUser,omitempty"`
	Experimental  bool   `json:"Experimental"`
}

// GetVersion returns the current VersionInfo snapshot.
func GetVersion() *VersionInfo {
	return &VersionInfo{
		Version:       Version,
		APIVersion:    DokiAPIVersion,
		MinAPIVersion: DokiMinClient,
		GitCommit:     GitCommit,
		GoVersion:     GoVersion,
		Os:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		BuildDate:     BuildDate,
		BuildUser:     BuildUser,
		Experimental:  false,
	}
}

// UserAgent returns the User-Agent string for outbound HTTP requests
// (registries, OCI distribution, etc.). Mirrors the Docker CLI format.
func UserAgent() string {
	return fmt.Sprintf("Doki/%s (%s; %s/%s)", Version, GoVersion, runtime.GOOS, runtime.GOARCH)
}

// FullVersion returns a human-readable banner used in --version and startup logs.
//
//	"Doki 0.9.2 (commit a1b2c3d, built 2026-06-02T14:00:00Z by root), API 1.48 (min 1.43)"
func FullVersion() string {
	return fmt.Sprintf("Doki %s (commit %s, built %s by %s), API %s (min %s)",
		Version, GitCommit, BuildDate, BuildUser, DokiAPIVersion, DokiMinClient)
}
