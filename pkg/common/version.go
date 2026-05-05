package common

import (
	"fmt"
	"runtime"
)

var (
	// Version is set at build time. Defaults to DokiVersion.
	Version = DokiVersion
	// GitCommit is set at build time.
	GitCommit = "unknown"
	// BuildTime is set at build time.
	BuildTime = "unknown"
)

// VersionInfo holds version information.
type VersionInfo struct {
	Version    string `json:"Version"`
	APIVersion string `json:"ApiVersion"`
	GitCommit  string `json:"GitCommit"`
	GoVersion  string `json:"GoVersion"`
	OS         string `json:"Os"`
	Arch       string `json:"Arch"`
	BuildTime  string `json:"BuildTime,omitempty"`
}

// GetVersion returns version information.
func GetVersion() *VersionInfo {
	return &VersionInfo{
		Version:    Version,
		APIVersion: DokiAPIVersion,
		GitCommit:  GitCommit,
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		BuildTime:  BuildTime,
	}
}

// UserAgent returns the user agent string.
func UserAgent() string {
	return fmt.Sprintf("Doki/%s (go/%s; %s/%s)", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
