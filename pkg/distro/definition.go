package distro

import "time"

type SourceType string

const (
	SourceDocker SourceType = "docker"
)

type DistroDefinition struct {
	Name           string         `yaml:"name"`
	Description    string         `yaml:"description"`
	DefaultVersion string         `yaml:"default_version"`
	Source         DistroSource   `yaml:"source"`
	Packages       PackageManager `yaml:"packages"`
	Aliases        []string       `yaml:"aliases"`
}

type DistroSource struct {
	Type     SourceType `yaml:"type"`
	Image    string     `yaml:"image"`
	Tag      string     `yaml:"tag"`
	Registry string     `yaml:"registry"`
}

type PackageManager struct {
	Manager string `yaml:"manager"`
	Update  string `yaml:"update"`
	Install string `yaml:"install"`
}

type InstalledDistro struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	InstalledAt time.Time `json:"installed_at"`
	Size        int64     `json:"size"`
	Source      string    `json:"source"`
	RootfsPath  string    `json:"rootfs_path"`
}
