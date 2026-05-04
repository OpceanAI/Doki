package common

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DokiVersion is the current version of the Doki engine.
const DokiVersion = "0.3.0"

// DokiAPIVersion is the compatible Docker Engine API version.
const DokiAPIVersion = "1.44"

// ContainerState represents the state of a container.
type ContainerState string

const (
	StateCreated    ContainerState = "created"
	StateRunning    ContainerState = "running"
	StatePaused     ContainerState = "paused"
	StateRestarting ContainerState = "restarting"
	StateRemoving   ContainerState = "removing"
	StateExited     ContainerState = "exited"
	StateDead       ContainerState = "dead"
)

// ContainerStatus represents the status string for a container.
type ContainerStatus string

const (
	StatusCreated    ContainerStatus = "created"
	StatusRunning    ContainerStatus = "running"
	StatusPaused     ContainerStatus = "paused"
	StatusRestarting ContainerStatus = "restarting"
	StatusRemoving   ContainerStatus = "removing"
	StatusExited     ContainerStatus = "exited"
	StatusDead       ContainerStatus = "dead"
	StatusUp         ContainerStatus = "up"
)

// NetworkMode represents the networking mode for a container.
type NetworkMode string

const (
	NetworkBridge   NetworkMode = "bridge"
	NetworkHost     NetworkMode = "host"
	NetworkNone     NetworkMode = "none"
	NetworkContainer NetworkMode = "container"
)

// RestartPolicy represents container restart behavior.
type RestartPolicy string

const (
	RestartNo            RestartPolicy = "no"
	RestartAlways        RestartPolicy = "always"
	RestartUnlessStopped RestartPolicy = "unless-stopped"
	RestartOnFailure     RestartPolicy = "on-failure"
)

// LogDriver is the logging driver for containers.
type LogDriver string

const (
	LogJSONFile LogDriver = "json-file"
	LogSyslog   LogDriver = "syslog"
	LogJournald LogDriver = "journald"
	LogNone     LogDriver = "none"
)

// MountType represents the type of a mount.
type MountType string

const (
	MountBind   MountType = "bind"
	MountVolume MountType = "volume"
	MountTmpfs  MountType = "tmpfs"
	MountNpipe  MountType = "npipe"
	MountCluster MountType = "cluster"
)

// Port protocol types.
type PortProtocol string

const (
	ProtocolTCP  PortProtocol = "tcp"
	ProtocolUDP  PortProtocol = "udp"
	ProtocolSCTP PortProtocol = "sctp"
)

// DokiConfig holds the configuration for the Doki daemon.
type DokiConfig struct {
	Root               string `json:"root"`
	SocketPath         string `json:"socket_path"`
	StorageDriver      string `json:"storage_driver"`
	DefaultNetwork     string `json:"default_network"`
	Debug              bool   `json:"debug"`
	LogLevel           string `json:"log_level"`
	Rootless           bool   `json:"rootless"`
	DataDir            string `json:"data_dir"`
	ExecRoot           string `json:"exec_root"`
	DNS                []string `json:"dns"`
	DNSSearch          []string `json:"dns_search"`
	DNSOptions         []string `json:"dns_options"`
	DefaultUlimits     map[string]Ulimit `json:"default_ulimits"`
	RegistryMirrors    []string `json:"registry_mirrors"`
	InsecureRegistries []string `json:"insecure_registries"`
}

// Ulimit defines a resource limit.
type Ulimit struct {
	Name string `json:"Name"`
	Soft int64  `json:"Soft"`
	Hard int64  `json:"Hard"`
}

// DefaultConfig returns the default Doki configuration.
func DefaultConfig() *DokiConfig {
	return &DokiConfig{
		Root:           "/data/data/com.termux/files/usr/var/lib/doki",
		SocketPath:     "/data/data/com.termux/files/usr/var/run/doki.sock",
		StorageDriver:  "fuse-overlayfs",
		DefaultNetwork: "bridge",
		Debug:          false,
		LogLevel:       "info",
		Rootless:       true,
		DataDir:        "/data/data/com.termux/files/usr/var/lib/doki",
		ExecRoot:       "/data/data/com.termux/files/usr/var/run/doki",
		DNS:            []string{"8.8.8.8", "8.8.4.4"},
	}
}

// ContainerInfo holds the information about a container.
type ContainerInfo struct {
	ID        string            `json:"Id"`
	Name      string            `json:"Name"`
	Image     string            `json:"Image"`
	ImageID   string            `json:"ImageID"`
	Command   string            `json:"Command"`
	Created   int64             `json:"Created"`
	State     ContainerState    `json:"State"`
	Status    string            `json:"Status"`
	Ports     []Port            `json:"Ports"`
	Labels    map[string]string `json:"Labels"`
	Names     []string          `json:"Names"`
	SizeRw    int64             `json:"SizeRw,omitempty"`
	SizeRootFs int64            `json:"SizeRootFs,omitempty"`
	HostConfig *HostConfig       `json:"HostConfig,omitempty"`
	NetworkSettings *NetworkSettings `json:"NetworkSettings,omitempty"`
	Mounts    []MountPoint       `json:"Mounts"`
}

// HostConfig is the host-dependent container configuration.
type HostConfig struct {
	Binds          []string            `json:"Binds"`
	NetworkMode    NetworkMode         `json:"NetworkMode"`
	PortBindings   PortMap             `json:"PortBindings"`
	RestartPolicy  RestartPolicyConfig `json:"RestartPolicy"`
	AutoRemove     bool                `json:"AutoRemove"`
	Privileged     bool                `json:"Privileged"`
	PublishAllPorts bool               `json:"PublishAllPorts"`
	ReadonlyRootfs bool                `json:"ReadonlyRootfs"`
	DNS            []string            `json:"Dns"`
	DNSOptions     []string            `json:"DnsOptions"`
	DNSSearch      []string            `json:"DnsSearch"`
	ExtraHosts     []string            `json:"ExtraHosts"`
	VolumesFrom    []string            `json:"VolumesFrom"`
	CapAdd         []string            `json:"CapAdd"`
	CapDrop        []string            `json:"CapDrop"`
	Sysctls        map[string]string   `json:"Sysctls"`
	ShmSize        int64               `json:"ShmSize"`
	Tmpfs          map[string]string   `json:"Tmpfs"`
	Ulimits        []Ulimit            `json:"Ulimits"`
	SecurityOpt    []string            `json:"SecurityOpt"`
	Mounts         []Mount             `json:"Mounts"`
	Init           bool                `json:"Init"`
	Runtime        string              `json:"Runtime"`
	Isolation      string              `json:"Isolation"`
	CPUShares      int64               `json:"CpuShares"`
	Memory         int64               `json:"Memory"`
	NanoCpus       int64               `json:"NanoCpus"`
	CgroupParent   string              `json:"CgroupParent"`
	BlkioWeight    uint16              `json:"BlkioWeight"`
	PidsLimit      int64               `json:"PidsLimit"`
	OomKillDisable bool                `json:"OomKillDisable"`
	OomScoreAdj    int64               `json:"OomScoreAdj"`
}

// RestartPolicyConfig describes container restart behavior.
type RestartPolicyConfig struct {
	Name              string `json:"Name"`
	MaximumRetryCount int    `json:"MaximumRetryCount"`
}

// NetworkSettings holds network-related information for a container.
type NetworkSettings struct {
	Networks map[string]*EndpointSettings `json:"Networks"`
}

// EndpointSettings holds network endpoint configuration.
type EndpointSettings struct {
	IPAMConfig  *EndpointIPAMConfig `json:"IPAMConfig,omitempty"`
	Links       []string            `json:"Links,omitempty"`
	Aliases     []string            `json:"Aliases,omitempty"`
	NetworkID   string              `json:"NetworkID"`
	EndpointID  string              `json:"EndpointID"`
	Gateway     string              `json:"Gateway"`
	IPAddress   string              `json:"IPAddress"`
	IPPrefixLen int                 `json:"IPPrefixLen"`
	MacAddress  string              `json:"MacAddress"`
}

// EndpointIPAMConfig represents IPAM configuration for an endpoint.
type EndpointIPAMConfig struct {
	IPv4Address string `json:"IPv4Address,omitempty"`
	IPv6Address string `json:"IPv6Address,omitempty"`
}

// Port represents an open port on a container.
type Port struct {
	IP          string       `json:"IP,omitempty"`
	PrivatePort uint16       `json:"PrivatePort"`
	PublicPort  uint16       `json:"PublicPort,omitempty"`
	Type        PortProtocol `json:"Type"`
}

// PortMap is a collection of port bindings keyed by port+proto (e.g. "80/tcp").
type PortMap map[string][]PortBinding

// PortBinding represents a port binding between host and container.
type PortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

// Mount represents a mount point inside a container.
type Mount struct {
	Type          MountType       `json:"Type"`
	Source        string          `json:"Source,omitempty"`
	Target        string          `json:"Target"`
	ReadOnly      bool            `json:"ReadOnly,omitempty"`
	Consistency   string          `json:"Consistency,omitempty"`
	BindOptions   *BindOptions    `json:"BindOptions,omitempty"`
	VolumeOptions *VolumeOptions  `json:"VolumeOptions,omitempty"`
	TmpfsOptions  *TmpfsOptions   `json:"TmpfsOptions,omitempty"`
}

// BindOptions for bind mounts.
type BindOptions struct {
	Propagation            string `json:"Propagation,omitempty"`
	NonRecursive           bool   `json:"NonRecursive,omitempty"`
	CreateMountpoint       bool   `json:"CreateMountpoint,omitempty"`
	ReadOnlyNonRecursive   bool   `json:"ReadOnlyNonRecursive,omitempty"`
	ReadOnlyForceRecursive bool   `json:"ReadOnlyForceRecursive,omitempty"`
}

// VolumeOptions for volume mounts.
type VolumeOptions struct {
	NoCopy       bool              `json:"NoCopy,omitempty"`
	Labels       map[string]string `json:"Labels,omitempty"`
	DriverConfig *DriverConfig     `json:"DriverConfig,omitempty"`
}

// DriverConfig for volumes.
type DriverConfig struct {
	Name    string            `json:"Name,omitempty"`
	Options map[string]string `json:"Options,omitempty"`
}

// TmpfsOptions for tmpfs mounts.
type TmpfsOptions struct {
	SizeBytes int64 `json:"SizeBytes,omitempty"`
	Mode      uint32 `json:"Mode,omitempty"`
}

// MountPoint describes a mount point inside a container.
type MountPoint struct {
	Type        MountType `json:"Type"`
	Name        string    `json:"Name,omitempty"`
	Source      string    `json:"Source"`
	Destination string    `json:"Destination"`
	Driver      string    `json:"Driver,omitempty"`
	Mode        string    `json:"Mode"`
	RW          bool      `json:"RW"`
	Propagation string    `json:"Propagation,omitempty"`
}

// ImageInfo holds information about an image.
type ImageInfo struct {
	ID          string            `json:"Id"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Parent      string            `json:"Parent"`
	Comment     string            `json:"Comment"`
	Created     int64             `json:"Created"`
	Container   string            `json:"Container"`
	Size        int64             `json:"Size"`
	VirtualSize int64             `json:"VirtualSize"`
	Author      string            `json:"Author"`
	Architecture string           `json:"Architecture"`
	Os          string            `json:"Os"`
	Labels      map[string]string `json:"Labels"`
}

// NetworkInfo holds information about a network.
type NetworkInfo struct {
	ID         string            `json:"Id"`
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Scope      string            `json:"Scope"`
	Internal   bool              `json:"Internal"`
	EnableIPv6 bool              `json:"EnableIPv6"`
	IPAM       *IPAM             `json:"IPAM"`
	Options    map[string]string `json:"Options"`
	Labels     map[string]string `json:"Labels"`
	Containers map[string]EndpointResource `json:"Containers"`
	Created    time.Time         `json:"Created"`
}

// IPAM represents IP address management configuration.
type IPAM struct {
	Driver  string            `json:"Driver"`
	Options map[string]string `json:"Options,omitempty"`
	Config  []IPAMConfig      `json:"Config"`
}

// IPAMConfig represents IPAM configuration for a network.
type IPAMConfig struct {
	Subnet  string `json:"Subnet,omitempty"`
	Gateway string `json:"Gateway,omitempty"`
	IPRange string `json:"IPRange,omitempty"`
}

// EndpointResource contains network resources allocated for an endpoint.
type EndpointResource struct {
	Name        string `json:"Name,omitempty"`
	EndpointID  string `json:"EndpointID"`
	MacAddress  string `json:"MacAddress"`
	IPv4Address string `json:"IPv4Address"`
	IPv6Address string `json:"IPv6Address"`
}

// VolumeInfo holds information about a volume.
type VolumeInfo struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Options    map[string]string `json:"Options"`
	CreatedAt  time.Time         `json:"CreatedAt"`
}

// ExecConfig holds configuration for an exec instance.
type ExecConfig struct {
	ID          string   `json:"Id"`
	AttachStdin  bool     `json:"AttachStdin"`
	AttachStdout bool     `json:"AttachStdout"`
	AttachStderr bool     `json:"AttachStderr"`
	Tty          bool     `json:"Tty"`
	Cmd          []string `json:"Cmd"`
	Env          []string `json:"Env"`
	WorkingDir   string   `json:"WorkingDir"`
	User         string   `json:"User"`
	Privileged   bool     `json:"Privileged"`
	Running      bool     `json:"Running"`
	ExitCode     int      `json:"ExitCode"`
	Pid          int      `json:"Pid"`
}

// HealthCheckResult stores the result of a healthcheck probe.
type HealthCheckResult struct {
	Start    time.Time `json:"Start"`
	End      time.Time `json:"End"`
	ExitCode int       `json:"ExitCode"`
	Output   string    `json:"Output"`
}

// SearchResult is returned from image search.
type SearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	StarCount   int    `json:"star_count"`
	IsOfficial  bool   `json:"is_official"`
	IsAutomated bool   `json:"is_automated"`
}

// DefaultDaemonSocket returns the default daemon socket path.
func DefaultDaemonSocket() string {
	if s := os.Getenv("DOKI_HOST"); s != "" {
		return s
	}
	socket := "/data/data/com.termux/files/usr/var/run/doki.sock"
	if _, err := os.Stat(socket); err == nil {
		return socket
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".doki", "doki.sock")
}

// ParseImageRef parses a Docker image reference string into its components.
func ParseImageRef(ref string) (*ImageRef, error) {
	return ParseImageRefString(ref)
}

// ImageRef represents a parsed image reference.
type ImageRef struct {
	Registry string
	Name     string
	Tag      string
	Digest   string
}

// ParseImageRefString parses an image reference.
func ParseImageRefString(ref string) (*ImageRef, error) {
	ir := &ImageRef{Tag: "latest"}

	if idx := strings.Index(ref, "@"); idx != -1 {
		ir.Digest = ref[idx+1:]
		ref = ref[:idx]
	}
	if idx := strings.Index(ref, ":"); idx != -1 {
		ir.Tag = ref[idx+1:]
		ref = ref[:idx]
	}

	parts := strings.SplitN(ref, "/", 3)
	switch len(parts) {
	case 1:
		ir.Registry = "registry-1.docker.io"
		ir.Name = "library/" + parts[0]
	case 2:
		if strings.Contains(parts[0], ".") || parts[0] == "localhost" {
			ir.Registry = parts[0]
			ir.Name = parts[1]
		} else {
			ir.Registry = "registry-1.docker.io"
			ir.Name = parts[0] + "/" + parts[1]
		}
	case 3:
		ir.Registry = parts[0]
		ir.Name = parts[1] + "/" + parts[2]
	}

	return ir, nil
}

// HealthStatus represents the health status.
type HealthStatus struct {
	Status        string              `json:"Status"`
	FailingStreak int                 `json:"FailingStreak"`
	Log           []HealthCheckResult `json:"Log"`
}

// SystemInfo holds system-wide information.
type SystemInfo struct {
	ID              string   `json:"ID"`
	Name            string   `json:"Name"`
	ServerVersion   string   `json:"ServerVersion"`
	OSType          string   `json:"OSType"`
	OperatingSystem string   `json:"OperatingSystem"`
	Architecture    string   `json:"Architecture"`
	NCPU            int      `json:"NCPU"`
	MemTotal        int64    `json:"MemTotal"`
	Driver          string   `json:"Driver"`
	DriverStatus    [][2]string `json:"DriverStatus"`
	DockerRootDir   string   `json:"DockerRootDir"`
	Images          int      `json:"Images"`
	Containers      int      `json:"Containers"`
	ContainersRunning int    `json:"ContainersRunning"`
	ContainersPaused int     `json:"ContainersPaused"`
	ContainersStopped int    `json:"ContainersStopped"`
}

// ContainerJSON holds full container information.
type ContainerJSON struct {
	*ContainerInfo
	Config           *ContainerConfig   `json:"Config"`
	Image            string             `json:"Image"`
	ResolvConfPath   string             `json:"ResolvConfPath"`
	HostnamePath     string             `json:"HostnamePath"`
	HostsPath        string             `json:"HostsPath"`
	LogPath           string             `json:"LogPath"`
	RestartCount     int                `json:"RestartCount"`
	Driver           string             `json:"Driver"`
	Platform         string             `json:"Platform"`
	MountLabel       string             `json:"MountLabel"`
	ProcessLabel     string             `json:"ProcessLabel"`
	AppArmorProfile  string             `json:"AppArmorProfile"`
	GraphDriver      *GraphDriverData   `json:"GraphDriver"`
}

// ContainerConfig holds container-specific configuration.
type ContainerConfig struct {
	Hostname     string            `json:"Hostname"`
	Domainname   string            `json:"Domainname"`
	User         string            `json:"User"`
	AttachStdin  bool              `json:"AttachStdin"`
	AttachStdout bool              `json:"AttachStdout"`
	AttachStderr bool              `json:"AttachStderr"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Tty          bool              `json:"Tty"`
	OpenStdin    bool              `json:"OpenStdin"`
	StdinOnce    bool              `json:"StdinOnce"`
	Env          []string          `json:"Env"`
	Cmd          []string          `json:"Cmd"`
	Image        string            `json:"Image"`
	Volumes      map[string]struct{} `json:"Volumes"`
	WorkingDir   string            `json:"WorkingDir"`
	Entrypoint   []string          `json:"Entrypoint"`
	Labels       map[string]string `json:"Labels"`
	StopSignal   string            `json:"StopSignal"`
	StopTimeout  *int              `json:"StopTimeout,omitempty"`
	Shell        []string          `json:"Shell"`
}

// GraphDriverData holds information about the storage driver.
type GraphDriverData struct {
	Name string            `json:"Name"`
	Data map[string]string `json:"Data"`
}

// SystemEventsResponse holds events from the system.
type SystemEventsResponse struct {
	Status   string    `json:"status"`
	ID       string    `json:"id"`
	From     string    `json:"from"`
	Type     string    `json:"Type"`
	Action   string    `json:"Action"`
	Actor    EventActor `json:"Actor"`
	Time     int64     `json:"time"`
	TimeNano int64     `json:"timeNano"`
}

// EventActor describes something that generates events.
type EventActor struct {
	ID         string            `json:"ID"`
	Attributes map[string]string `json:"Attributes"`
}

// DeviceMapping represents a device mapping between host and container.
type DeviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}
