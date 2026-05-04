package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	"github.com/OpceanAI/Doki/pkg/runtime"
)

// ComposeFile represents a docki-compose file (doki.yml).
type ComposeFile struct {
	Version  string               `yaml:"version,omitempty"`
	Name     string               `yaml:"name,omitempty"`
	Services map[string]*Service   `yaml:"services"`
	Networks map[string]*Network   `yaml:"networks,omitempty"`
	Volumes  map[string]*Volume    `yaml:"volumes,omitempty"`
	Configs  map[string]*Config    `yaml:"configs,omitempty"`
	Secrets  map[string]*Secret    `yaml:"secrets,omitempty"`
	Include  []string              `yaml:"include,omitempty"`
}

// Service represents a service (container) in the compose file.
type Service struct {
	Image        string              `yaml:"image,omitempty"`
	Build        *BuildConfig        `yaml:"build,omitempty"`
	Command      interface{}         `yaml:"command,omitempty"`
	Entrypoint   interface{}         `yaml:"entrypoint,omitempty"`
	Environment  interface{}         `yaml:"environment,omitempty"`
	EnvFile      interface{}         `yaml:"env_file,omitempty"`
	Ports        []string            `yaml:"ports,omitempty"`
	Expose       []string            `yaml:"expose,omitempty"`
	Volumes      []string            `yaml:"volumes,omitempty"`
	Networks     interface{}         `yaml:"networks,omitempty"`
	NetworkMode  string              `yaml:"network_mode,omitempty"`
	DependsOn    interface{}         `yaml:"depends_on,omitempty"`
	Restart      string              `yaml:"restart,omitempty"`
	Labels       interface{}         `yaml:"labels,omitempty"`
	WorkingDir   string              `yaml:"working_dir,omitempty"`
	User         string              `yaml:"user,omitempty"`
	Hostname     string              `yaml:"hostname,omitempty"`
	Domainname   string              `yaml:"domainname,omitempty"`
	Tty          bool                `yaml:"tty,omitempty"`
	StdinOpen    bool                `yaml:"stdin_open,omitempty"`
	Privileged   bool                `yaml:"privileged,omitempty"`
	ReadOnly     bool                `yaml:"read_only,omitempty"`
	Init         bool                `yaml:"init,omitempty"`
	DNS          interface{}         `yaml:"dns,omitempty"`
	DNSSearch    interface{}         `yaml:"dns_search,omitempty"`
	DNSOpt       interface{}         `yaml:"dns_opt,omitempty"`
	ExtraHosts   interface{}         `yaml:"extra_hosts,omitempty"`
	CapAdd       []string            `yaml:"cap_add,omitempty"`
	CapDrop      []string            `yaml:"cap_drop,omitempty"`
	SecurityOpt  []string            `yaml:"security_opt,omitempty"`
	Sysctls      map[string]string   `yaml:"sysctls,omitempty"`
	Healthcheck  *HealthcheckConfig  `yaml:"healthcheck,omitempty"`
	StopSignal   string              `yaml:"stop_signal,omitempty"`
	StopGracePeriod string           `yaml:"stop_grace_period,omitempty"`
	Deploy       *DeployConfig       `yaml:"deploy,omitempty"`
	Profile      string              `yaml:"profile,omitempty"`
}

// BuildConfig defines build settings for a service.
type BuildConfig struct {
	Context    string            `yaml:"context"`
	Dokifile   string            `yaml:"dokifile,omitempty"`
	Dockerfile string            `yaml:"dockerfile,omitempty"`
	Args       map[string]string `yaml:"args,omitempty"`
	Labels     map[string]string `yaml:"labels,omitempty"`
	Target     string            `yaml:"target,omitempty"`
	Network    string            `yaml:"network,omitempty"`
	CacheFrom  []string          `yaml:"cache_from,omitempty"`
}

// HealthcheckConfig defines healthcheck for a service.
type HealthcheckConfig struct {
	Test          interface{} `yaml:"test"`
	Interval      string      `yaml:"interval,omitempty"`
	Timeout       string      `yaml:"timeout,omitempty"`
	Retries       int         `yaml:"retries,omitempty"`
	StartPeriod   string      `yaml:"start_period,omitempty"`
	StartInterval string      `yaml:"start_interval,omitempty"`
	Disable       bool        `yaml:"disable,omitempty"`
}

// DeployConfig defines deployment settings.
type DeployConfig struct {
	Replicas     int                       `yaml:"replicas,omitempty"`
	Resources    *DeployResources          `yaml:"resources,omitempty"`
	RestartPolicy *DeployRestartPolicy      `yaml:"restart_policy,omitempty"`
}

// DeployResources defines resource limits.
type DeployResources struct {
	Limits       *ResourceLimits `yaml:"limits,omitempty"`
	Reservations *ResourceLimits `yaml:"reservations,omitempty"`
}

// ResourceLimits defines resource limits.
type ResourceLimits struct {
	CPUs    string `yaml:"cpus,omitempty"`
	Memory  string `yaml:"memory,omitempty"`
}

// DeployRestartPolicy defines restart policy for deployments.
type DeployRestartPolicy struct {
	Condition   string `yaml:"condition,omitempty"`
	Delay       string `yaml:"delay,omitempty"`
	MaxAttempts int    `yaml:"max_attempts,omitempty"`
}

// Network represents a compose network.
type Network struct {
	Driver     string            `yaml:"driver,omitempty"`
	External   bool              `yaml:"external,omitempty"`
	Name       string            `yaml:"name,omitempty"`
	Internal   bool              `yaml:"internal,omitempty"`
	EnableIPv6 bool              `yaml:"enable_ipv6,omitempty"`
}

// Volume represents a compose volume.
type Volume struct {
	Driver     string            `yaml:"driver,omitempty"`
	External   bool              `yaml:"external,omitempty"`
	Name       string            `yaml:"name,omitempty"`
	DriverOpts map[string]string `yaml:"driver_opts,omitempty"`
}

// Config represents a compose config.
type Config struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

// Secret represents a compose secret.
type Secret struct {
	File     string `yaml:"file,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

// Engine is the compose engine.
type Engine struct {
	project  string
	runtime  *runtime.Runtime
	image    *image.Store
	network  *network.Manager
	file     *ComposeFile
}

// NewEngine creates a new compose engine.
func NewEngine(project string, rt *runtime.Runtime, img *image.Store, net *network.Manager) *Engine {
	return &Engine{
		project: project,
		runtime: rt,
		image:   img,
		network: net,
	}
}

// Load loads a compose file from disk.
func (e *Engine) Load(path string) error {
	// Try multiple file names.
	candidates := []string{
		path,
		filepath.Join(path, "doki.yml"),
		filepath.Join(path, "doki.yaml"),
		filepath.Join(path, "doki-compose.yml"),
		filepath.Join(path, "doki-compose.yaml"),
		filepath.Join(path, "compose.yml"),
		filepath.Join(path, "compose.yaml"),
		filepath.Join(path, "docker-compose.yml"),
		filepath.Join(path, "docker-compose.yaml"),
	}

	var data []byte
	var err error

	for _, candidate := range candidates {
		if common.PathExists(candidate) {
			data, err = os.ReadFile(candidate)
			if err == nil {
				break
			}
		}
	}

	if data == nil {
		return fmt.Errorf("no compose file found in %s", path)
	}

	var file ComposeFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse compose file: %w", err)
	}

	e.file = &file
	return nil
}

// Up starts all services defined in the compose file.
func (e *Engine) Up() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	// Create networks.
	for name := range e.file.Networks {
		cfg := &network.NetworkConfig{
			Name:   e.project + "_" + name,
			Driver: "bridge",
		}

		if nwDef, ok := e.file.Networks[name]; ok {
			if nwDef.Driver != "" {
				cfg.Driver = nwDef.Driver
			}
		}

		e.network.CreateNetwork(cfg)
	}

	// Start services in dependency order.
	ordered := e.orderServices()

	for _, svcName := range ordered {
		if err := e.startService(svcName); err != nil {
			return fmt.Errorf("start service %s: %w", svcName, err)
		}
	}

	return nil
}

// Down stops and removes all services.
func (e *Engine) Down() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	for svcName := range e.file.Services {
		containerName := e.project + "_" + svcName + "_1"
		e.runtime.Stop(containerName, 10)
		e.runtime.Delete(containerName, true)
	}

	return nil
}

// Ps lists running services.
func (e *Engine) Ps() ([]common.ContainerInfo, error) {
	states, err := e.runtime.List()
	if err != nil {
		return nil, err
	}

	containers := make([]common.ContainerInfo, 0)
	for _, state := range states {
		containers = append(containers, common.ContainerInfo{
			ID:      state.ID[:12],
			Names:   []string{"/" + state.ID[:12]},
			State:   state.Status,
			Status:  string(state.Status),
			Created: state.Created.Unix(),
		})
	}

	return containers, nil
}

func (e *Engine) startService(name string) error {
	svc, ok := e.file.Services[name]
	if !ok {
		return fmt.Errorf("service %s not found", name)
	}

	imageName := svc.Image
	if imageName == "" {
		imageName = e.project + "_" + name
	}

	// Pull image if needed.
	if !e.image.Exists(imageName) {
		if _, err := e.image.Pull(imageName); err != nil {
			return fmt.Errorf("pull %s: %w", imageName, err)
		}
	}

	// Build container config.
	cmd := toStringSlice(svc.Command)
	if len(cmd) == 0 {
		// Try to get default command from image config.
		if record, err := e.image.Get(imageName); err == nil && record.Config != nil {
			cmd = record.Config.Config.Cmd
			if len(cmd) == 0 {
				cmd = record.Config.Config.Entrypoint
			}
		}
	}
	// Fallback for services without any command.
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	cfg := &runtime.Config{
		ID:   e.project + "_" + name + "_1",
		Args: cmd,
		Env:  toEnvSlice(svc.Environment),
		Tty:  svc.Tty,
	}

	if svc.WorkingDir != "" {
		cfg.Cwd = svc.WorkingDir
	}

	if svc.Hostname != "" {
		cfg.Hostname = svc.Hostname
	}

	if svc.NetworkMode == "host" {
		cfg.NetworkMode = common.NetworkHost
	}

	// Parse ports.
	for _, portSpec := range svc.Ports {
		port, binding := common.ParsePortBinding(portSpec)
		cfg.Ports = append(cfg.Ports, port)
		_ = binding
	}

	// Create container.
	if _, err := e.runtime.Create(cfg); err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	// Start container.
	if err := e.runtime.Start(cfg.ID); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}

	return nil
}

func (e *Engine) orderServices() []string {
	ordered := make([]string, 0, len(e.file.Services))
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(name string) bool
	dfs = func(name string) bool {
		if inStack[name] {
			return false // cycle detected
		}
		if visited[name] {
			return true
		}
		visited[name] = true
		inStack[name] = true
		defer delete(inStack, name)

		svc, ok := e.file.Services[name]
		if !ok {
			return true
		}

		if deps, ok := svc.DependsOn.([]interface{}); ok {
			for _, dep := range deps {
				if !dfs(fmt.Sprint(dep)) {
					return false
				}
			}
		} else if deps, ok := svc.DependsOn.(map[string]interface{}); ok {
			for dep := range deps {
				if !dfs(dep) {
					return false
				}
			}
		}

		ordered = append(ordered, name)
		return true
	}

	for name := range e.file.Services {
		if !dfs(name) {
			// Cycle detected; fall back to simple order.
			for name := range e.file.Services {
				ordered = append(ordered, name)
			}
			return ordered
		}
	}

	return ordered
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			result[i] = fmt.Sprint(item)
		}
		return result
	case []string:
		return val
	}

	return nil
}

func toEnvSlice(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			result[i] = fmt.Sprint(item)
		}
		return result
	case []string:
		return val
	case map[string]interface{}:
		result := make([]string, 0, len(val))
		for k, val := range val {
			result = append(result, fmt.Sprintf("%s=%v", k, val))
		}
		return result
	}

	return nil
}
