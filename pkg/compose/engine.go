package compose

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/OpceanAI/Doki/pkg/builder"
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
	Ports        interface{}         `yaml:"ports,omitempty"`
	Expose       []string            `yaml:"expose,omitempty"`
	Volumes      interface{}         `yaml:"volumes,omitempty"`
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
	Profiles     []string            `yaml:"profiles,omitempty"`
	Extends      interface{}         `yaml:"extends,omitempty"`
	Secrets      []string            `yaml:"secrets,omitempty"`
	Configs      []string            `yaml:"configs,omitempty"`
	ContainerName string             `yaml:"container_name,omitempty"`
	ShmSize      string              `yaml:"shm_size,omitempty"`
	PidsLimit    int64               `yaml:"pids_limit,omitempty"`
	OomKillDisable bool              `yaml:"oom_kill_disable,omitempty"`
	OomScoreAdj  int                 `yaml:"oom_score_adj,omitempty"`
	MacAddress   string              `yaml:"mac_address,omitempty"`
	Logging      *LoggingConfig      `yaml:"logging,omitempty"`
	Ulimits      map[string]interface{} `yaml:"ulimits,omitempty"`
	Tmpfs        interface{}         `yaml:"tmpfs,omitempty"`
	BlkioConfig  map[string]interface{} `yaml:"blkio_config,omitempty"`
	Devices      []string            `yaml:"devices,omitempty"`
	Platform     string              `yaml:"platform,omitempty"`
	Runtime      string              `yaml:"runtime,omitempty"`
	Scale        int                 `yaml:"scale,omitempty"`
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

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	Driver  string            `yaml:"driver,omitempty"`
	Options map[string]string `yaml:"options,omitempty"`
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
	project     string
	runtime     *runtime.Runtime
	image       *image.Store
	network     *network.Manager
	file        *ComposeFile
	projectDir  string
	envVars     map[string]string
	netCreated  map[string]bool
}

// NewEngine creates a new compose engine.
func NewEngine(project string, rt *runtime.Runtime, img *image.Store, net *network.Manager) *Engine {
	return &Engine{
		project:    project,
		runtime:    rt,
		image:      img,
		network:    net,
		netCreated: make(map[string]bool),
	}
}

// Load loads a compose file from disk.
func (e *Engine) Load(path string) error {
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
	var actualPath string
	var err error

	for _, candidate := range candidates {
		if common.PathExists(candidate) {
			data, err = os.ReadFile(candidate)
			if err == nil {
				actualPath = candidate
				break
			}
		}
	}

	if data == nil {
		return fmt.Errorf("no compose file found in %s", path)
	}

	// Y23: Auto-load .env from project directory.
	e.projectDir = filepath.Dir(actualPath)
	if fi, _ := os.Stat(actualPath); fi != nil && !fi.IsDir() {
		e.projectDir = filepath.Dir(actualPath)
	}
	if fi, _ := os.Stat(path); fi != nil && fi.IsDir() {
		e.projectDir = path
	}
	e.loadDotEnv()

	var file ComposeFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parse compose file: %w", err)
	}

	e.file = &file

	// Validate compose file version
	if file.Version != "" {
		parts := strings.SplitN(file.Version, ".", 3)
		if len(parts) < 1 {
			return fmt.Errorf("invalid compose file version: %s", file.Version)
		}
		major, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid compose file version: %s", file.Version)
		}
		minor := 0
		if len(parts) > 1 {
			minor, _ = strconv.Atoi(parts[1])
		}
		if major < 2 {
			return fmt.Errorf("unsupported compose file version: %s (requires >= 2.0)", file.Version)
		}
		_ = minor
	}

	// Y24: Variable interpolation.
	e.interpolateFile(e.file)

	// Y22: Process include statements (load and merge additional compose files).
	if err := e.processIncludes(); err != nil {
		return fmt.Errorf("process includes: %w", err)
	}

	// Y10: Resolve extends for each service.
	for name := range e.file.Services {
		if err := e.resolveExtends(name); err != nil {
			return fmt.Errorf("resolve extends for %s: %w", name, err)
		}
	}

	// Validate compose file structure (after extends resolution).
	if err := Validate(e.file); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return nil
}

// Y23: loadDotEnv loads .env file from project directory into envVars.
func (e *Engine) loadDotEnv() {
	e.envVars = make(map[string]string)
	envPath := filepath.Join(e.projectDir, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"") {
				val = val[1 : len(val)-1]
			}
			if strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'") {
				val = val[1 : len(val)-1]
			}
			e.envVars[key] = val
		}
	}
}

// Y24: interpolateFile replaces ${VAR} and $VAR in all string values.
func (e *Engine) interpolateFile(file *ComposeFile) {
	if file == nil {
		return
	}
	for name, svc := range file.Services {
		e.interpolateService(name, svc)
	}
}

func (e *Engine) interpolateService(name string, svc *Service) {
	_ = name
	svc.Image = e.interpolate(svc.Image)
	svc.NetworkMode = e.interpolate(svc.NetworkMode)
	svc.Restart = e.interpolate(svc.Restart)
	svc.WorkingDir = e.interpolate(svc.WorkingDir)
	svc.User = e.interpolate(svc.User)
	svc.Hostname = e.interpolate(svc.Hostname)
	svc.Domainname = e.interpolate(svc.Domainname)
	svc.StopSignal = e.interpolate(svc.StopSignal)
	svc.StopGracePeriod = e.interpolate(svc.StopGracePeriod)
	svc.Profile = e.interpolate(svc.Profile)
	svc.ContainerName = e.interpolate(svc.ContainerName)

	if svc.Command != nil {
		svc.Command = e.interpolateInterface(svc.Command)
	}
	if svc.Entrypoint != nil {
		svc.Entrypoint = e.interpolateInterface(svc.Entrypoint)
	}
	if svc.Environment != nil {
		svc.Environment = e.interpolateInterface(svc.Environment)
	}
	if svc.DNS != nil {
		svc.DNS = e.interpolateInterface(svc.DNS)
	}
	if svc.DNSSearch != nil {
		svc.DNSSearch = e.interpolateInterface(svc.DNSSearch)
	}
	if svc.DNSOpt != nil {
		svc.DNSOpt = e.interpolateInterface(svc.DNSOpt)
	}
	if svc.ExtraHosts != nil {
		svc.ExtraHosts = e.interpolateInterface(svc.ExtraHosts)
	}
	if svc.Ports != nil {
		svc.Ports = e.interpolateInterface(svc.Ports)
	}
	if svc.Volumes != nil {
		svc.Volumes = e.interpolateInterface(svc.Volumes)
	}
	for i, s := range svc.Secrets {
		svc.Secrets[i] = e.interpolate(s)
	}
	for i, c := range svc.Configs {
		svc.Configs[i] = e.interpolate(c)
	}
	for i, c := range svc.CapAdd {
		svc.CapAdd[i] = e.interpolate(c)
	}
	for i, c := range svc.CapDrop {
		svc.CapDrop[i] = e.interpolate(c)
	}
	for i, s := range svc.SecurityOpt {
		svc.SecurityOpt[i] = e.interpolate(s)
	}
	if svc.Sysctls != nil {
		for k, v := range svc.Sysctls {
			svc.Sysctls[k] = e.interpolate(v)
		}
	}
	if svc.Build != nil {
		svc.Build.Context = e.interpolate(svc.Build.Context)
		svc.Build.Dokifile = e.interpolate(svc.Build.Dokifile)
		svc.Build.Dockerfile = e.interpolate(svc.Build.Dockerfile)
		svc.Build.Target = e.interpolate(svc.Build.Target)
		svc.Build.Network = e.interpolate(svc.Build.Network)
	}
}

var varRegex = regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

func (e *Engine) interpolate(s string) string {
	if s == "" {
		return s
	}
	return varRegex.ReplaceAllStringFunc(s, func(match string) string {
		var varName, defaultVal, altVal string
		isDefault, isAlt := false, false

		if strings.HasPrefix(match, "${") && strings.HasSuffix(match, "}") {
			inner := match[2 : len(match)-1]
			if idx := strings.Index(inner, ":-"); idx >= 0 {
				varName = inner[:idx]
				defaultVal = inner[idx+2:]
				isDefault = true
			} else if idx := strings.Index(inner, ":+"); idx >= 0 {
				varName = inner[:idx]
				altVal = inner[idx+2:]
				isAlt = true
			} else {
				varName = inner
			}
		} else if strings.HasPrefix(match, "$") {
			varName = match[1:]
		}

		val, ok := e.envVars[varName]
		if !ok {
			val, ok = os.LookupEnv(varName)
		}

		if isAlt {
			if ok && val != "" {
				return altVal
			}
			return ""
		}
		if isDefault {
			if ok && val != "" {
				return val
			}
			return defaultVal
		}
		if ok {
			return val
		}
		return match
	})
}

func (e *Engine) interpolateInterface(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return e.interpolate(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = e.interpolateInterface(item)
		}
		return result
	case []string:
		result := make([]string, len(val))
		for i, item := range val {
			result[i] = e.interpolate(item)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, item := range val {
			if s, ok := item.(string); ok {
				result[k] = e.interpolate(s)
			} else {
				result[k] = e.interpolateInterface(item)
			}
		}
		return result
	}
	return v
}

// Y22: processIncludes loads and merges additional compose files listed in `include`.
func (e *Engine) processIncludes() error {
	for _, incPath := range e.file.Include {
		incPath = e.interpolate(incPath)
		fullPath := incPath
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(e.projectDir, incPath)
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("include file %s: %w", incPath, err)
		}
		var incFile ComposeFile
		if err := yaml.Unmarshal(data, &incFile); err != nil {
			return fmt.Errorf("parse include %s: %w", incPath, err)
		}
		e.interpolateFile(&incFile)
		// Merge services.
		if incFile.Services != nil {
			if e.file.Services == nil {
				e.file.Services = make(map[string]*Service)
			}
			for k, v := range incFile.Services {
				if _, exists := e.file.Services[k]; !exists {
					e.file.Services[k] = v
				}
			}
		}
		// Merge networks.
		if incFile.Networks != nil {
			if e.file.Networks == nil {
				e.file.Networks = make(map[string]*Network)
			}
			for k, v := range incFile.Networks {
				if _, exists := e.file.Networks[k]; !exists {
					e.file.Networks[k] = v
				}
			}
		}
		// Merge volumes.
		if incFile.Volumes != nil {
			if e.file.Volumes == nil {
				e.file.Volumes = make(map[string]*Volume)
			}
			for k, v := range incFile.Volumes {
				if _, exists := e.file.Volumes[k]; !exists {
					e.file.Volumes[k] = v
				}
			}
		}
		// Merge secrets.
		if incFile.Secrets != nil {
			if e.file.Secrets == nil {
				e.file.Secrets = make(map[string]*Secret)
			}
			for k, v := range incFile.Secrets {
				if _, exists := e.file.Secrets[k]; !exists {
					e.file.Secrets[k] = v
				}
			}
		}
		// Merge configs.
		if incFile.Configs != nil {
			if e.file.Configs == nil {
				e.file.Configs = make(map[string]*Config)
			}
			for k, v := range incFile.Configs {
				if _, exists := e.file.Configs[k]; !exists {
					e.file.Configs[k] = v
				}
			}
		}
	}
	return nil
}

// Y10: resolveExtends reads the referenced service file and merges its properties.
func (e *Engine) resolveExtends(name string) error {
	svc, ok := e.file.Services[name]
	if !ok || svc.Extends == nil {
		return nil
	}

	var extPath string
	var extSvc string

	switch v := svc.Extends.(type) {
	case string:
		extSvc = v
	case map[string]interface{}:
		if s, ok := v["service"].(string); ok {
			extSvc = s
		}
		if f, ok := v["file"].(string); ok {
			extPath = f
		}
	}

	if extSvc == "" {
		return nil
	}

	// Determine the file to load.
	var data []byte
	if extPath != "" {
		fullPath := extPath
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(e.projectDir, extPath)
		}
		var err error
		data, err = os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("extends file %s: %w", extPath, err)
		}
	} else {
		// Use the current compose file.
		var err error
		data, err = yaml.Marshal(e.file)
		if err != nil {
			return err
		}
	}

	var extFile ComposeFile
	if err := yaml.Unmarshal(data, &extFile); err != nil {
		return fmt.Errorf("parse extends file: %w", err)
	}

	baseSvc, ok := extFile.Services[extSvc]
	if !ok {
		return fmt.Errorf("extends service %s not found", extSvc)
	}

	// Merge base service properties into current service (current wins).
	e.mergeService(svc, baseSvc)

	// Clear Extends field.
	svc.Extends = nil

	return nil
}

// mergeService merges base into target (target values take precedence).
func (e *Engine) mergeService(target, base *Service) {
	if target.Image == "" && base.Image != "" {
		target.Image = base.Image
	}
	if target.Build == nil && base.Build != nil {
		target.Build = base.Build
	}
	if target.Command == nil && base.Command != nil {
		target.Command = base.Command
	}
	if target.Entrypoint == nil && base.Entrypoint != nil {
		target.Entrypoint = base.Entrypoint
	}
	// Merge environment: if target has none, use base
	if target.Environment == nil && base.Environment != nil {
		target.Environment = base.Environment
	}
	if target.EnvFile == nil && base.EnvFile != nil {
		target.EnvFile = base.EnvFile
	}
	if len(toStringSlice(target.Ports)) == 0 {
		target.Ports = base.Ports
	}
	if len(toStringSlice(target.Volumes)) == 0 {
		target.Volumes = base.Volumes
	}
	if target.Networks == nil && base.Networks != nil {
		target.Networks = base.Networks
	}
	if target.NetworkMode == "" {
		target.NetworkMode = base.NetworkMode
	}
	if target.DependsOn == nil && base.DependsOn != nil {
		target.DependsOn = base.DependsOn
	}
	if target.Restart == "" {
		target.Restart = base.Restart
	}
	if target.WorkingDir == "" {
		target.WorkingDir = base.WorkingDir
	}
	if target.User == "" {
		target.User = base.User
	}
	if target.Hostname == "" {
		target.Hostname = base.Hostname
	}
	if target.Domainname == "" {
		target.Domainname = base.Domainname
	}
	if !target.Tty {
		target.Tty = base.Tty
	}
	if !target.Privileged {
		target.Privileged = base.Privileged
	}
	if !target.Init {
		target.Init = base.Init
	}
	if target.DNS == nil {
		target.DNS = base.DNS
	}
	if target.DNSSearch == nil {
		target.DNSSearch = base.DNSSearch
	}
	if target.ExtraHosts == nil {
		target.ExtraHosts = base.ExtraHosts
	}
	if len(target.CapAdd) == 0 {
		target.CapAdd = base.CapAdd
	}
	if len(target.CapDrop) == 0 {
		target.CapDrop = base.CapDrop
	}
	if len(target.SecurityOpt) == 0 {
		target.SecurityOpt = base.SecurityOpt
	}
	if target.Healthcheck == nil {
		target.Healthcheck = base.Healthcheck
	}
	if target.StopSignal == "" {
		target.StopSignal = base.StopSignal
	}
	if target.StopGracePeriod == "" {
		target.StopGracePeriod = base.StopGracePeriod
	}
	if target.Deploy == nil {
		target.Deploy = base.Deploy
	}
	if target.Profile == "" {
		target.Profile = base.Profile
	}
	if len(target.Profiles) == 0 {
		target.Profiles = base.Profiles
	}
	if target.ContainerName == "" {
		target.ContainerName = base.ContainerName
	}
	if target.Logging == nil {
		target.Logging = base.Logging
	}
	if len(target.Secrets) == 0 {
		target.Secrets = base.Secrets
	}
	if len(target.Configs) == 0 {
		target.Configs = base.Configs
	}
}

// Up starts all services defined in the compose file.
func (e *Engine) Up() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	// Y8: Profile filtering.
	profiles := e.getProfilesFilter()

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

		if _, err := e.network.CreateNetwork(cfg); err != nil {
			return fmt.Errorf("create network %s: %w", name, err)
		}
		e.netCreated[name] = true
	}

	// Build images for services with build config.
	if err := e.Build(); err != nil {
		return err
	}

	// Start services in dependency order.
	ordered, err := e.orderServices()
	if err != nil {
		return fmt.Errorf("dependency ordering: %w", err)
	}

	for _, svcName := range ordered {
		svc := e.file.Services[svcName]

		// Y8: Skip services whose profile doesn't match.
		if !e.profileMatches(svc, profiles) {
			continue
		}

		// Wait for depends_on conditions (service_healthy, etc.)
		if err := e.waitForDependencies(svc); err != nil {
			return fmt.Errorf("wait for dependencies of %s: %w", svcName, err)
		}

		// Y: Scale support — start N instances if scale > 1.
		scale := svc.Scale
		if scale <= 0 {
			scale = 1
		}
		for i := 1; i <= scale; i++ {
			if err := e.startServiceN(svcName, i); err != nil {
				return fmt.Errorf("start service %s instance %d: %w", svcName, i, err)
			}
		}
	}

	return nil
}

// waitForDependencies polls health status of services listed in depends_on
// when condition is "service_healthy".
func (e *Engine) waitForDependencies(svc *Service) error {
	deps := svc.DependsOn
	if deps == nil {
		return nil
	}

	type depWait struct {
		name      string
		condition string
	}
	var toWait []depWait

	switch d := deps.(type) {
	case []interface{}:
		for _, dep := range d {
			if m, ok := dep.(map[string]interface{}); ok {
				name, _ := m["service"].(string)
				cond, _ := m["condition"].(string)
				if name == "" {
					continue
				}
				if cond == "" {
					cond = "service_started"
				}
				toWait = append(toWait, depWait{name: name, condition: cond})
			}
		}
	case map[string]interface{}:
		for name, cond := range d {
			condStr, _ := cond.(string)
			if condStr == "" {
				condStr = "service_started"
			}
			toWait = append(toWait, depWait{name: name, condition: condStr})
		}
	}

	timeout := 60
	if t := os.Getenv("COMPOSE_DEPENDENCY_TIMEOUT"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v > 0 {
			timeout = v
		}
	}
	for _, dep := range toWait {
		containerID := e.containerName(dep.name)
		switch dep.condition {
		case "service_started":
			// Wait for container to be running.
			var running bool
			for i := 0; i < timeout; i++ {
				state, err := e.runtime.State(containerID)
				if err == nil && state.Status != "" && state.Status != common.StateCreated {
					running = true
					break
				}
				time.Sleep(time.Second)
			}
			if !running {
				return fmt.Errorf("dependency %s not started after %ds", dep.name, timeout)
			}

		case "service_healthy":
			// Wait for container to be healthy.
			var healthy bool
			for i := 0; i < timeout; i++ {
				state, err := e.runtime.State(containerID)
				if err == nil && state.HealthStatus != nil && state.HealthStatus.Status == "healthy" {
					healthy = true
					break
				}
				time.Sleep(time.Second)
			}
			if !healthy {
				return fmt.Errorf("dependency %s not healthy after %ds", dep.name, timeout)
			}

		case "service_completed_successfully":
			// Wait for container to exit with code 0.
			var completed bool
			for i := 0; i < timeout; i++ {
				state, err := e.runtime.State(containerID)
				if err == nil && state.Status == common.StateExited && state.ExitCode == 0 {
					completed = true
					break
				}
				time.Sleep(time.Second)
			}
			if !completed {
				return fmt.Errorf("dependency %s did not complete successfully after %ds", dep.name, timeout)
			}
		}
	}
	return nil
}

// Y8: getProfilesFilter returns the list of active profiles from COMPOSE_PROFILES env.
func (e *Engine) getProfilesFilter() []string {
	val := os.Getenv("COMPOSE_PROFILES")
	if val == "" {
		return nil
	}
	return strings.Split(val, ",")
}

// Y8: profileMatches checks if a service profile matches the filter.
func (e *Engine) profileMatches(svc *Service, profiles []string) bool {
	// Services with no profile always start
	if svc.Profile == "" && len(svc.Profiles) == 0 {
		return true
	}
	// If no profiles specified, exclude services that have a profile
	if len(profiles) == 0 {
		return false
	}
	// Check singular Profile field
	if svc.Profile != "" {
		for _, p := range profiles {
			if strings.TrimSpace(p) == svc.Profile {
				return true
			}
		}
	}
	// Check plural Profiles field
	if len(svc.Profiles) > 0 {
		for _, sp := range svc.Profiles {
			for _, p := range profiles {
				if strings.TrimSpace(p) == sp {
					return true
				}
			}
		}
	}
	return false
}

// Down stops and removes all services.
func (e *Engine) Down() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	for svcName := range e.file.Services {
		svc := e.file.Services[svcName]

		scale := svc.Scale
		if scale <= 0 {
			scale = 1
		}

		timeout := 10
		if svc.StopGracePeriod != "" {
			if d, err := parseDuration(svc.StopGracePeriod); err == nil {
				timeout = int(d.Seconds())
			}
		}

		for i := 1; i <= scale; i++ {
			containerName := e.containerNameN(svcName, i)
			if err := e.runtime.Stop(containerName, timeout); err != nil {
				slog.Warn("stop", "service", svcName, "instance", i, "err", err)
			}
			if err := e.runtime.Delete(containerName, true); err != nil {
				slog.Warn("delete", "service", svcName, "instance", i, "err", err)
			}
		}
	}

	return nil
}

// Y25: Stop stops all services.
func (e *Engine) Stop() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	for svcName := range e.file.Services {
		svc := e.file.Services[svcName]
		containerName := e.containerName(svcName)
		timeout := 10
		if svc.StopGracePeriod != "" {
			if d, err := parseDuration(svc.StopGracePeriod); err == nil {
				timeout = int(d.Seconds())
			}
		}
		if err := e.runtime.Stop(containerName, timeout); err != nil {
			slog.Warn("stop", "service", svcName, "err", err)
		}
	}

	return nil
}

// Start starts stopped services. For containers that exited, recreates them via Up.
func (e *Engine) Start() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	for svcName := range e.file.Services {
		containerName := e.containerName(svcName)
		// Check if container exists and is stopped (exited).
		state, err := e.runtime.State(containerName)
		if err != nil || state == nil {
			// Container doesn't exist, create it.
			if err := e.startService(svcName); err != nil {
				slog.Warn("start", "service", svcName, "err", err)
			}
			continue
		}
		if state.Status == common.StateRunning {
			continue // already running
		}
		// Stopped/exited: need to recreate.
		_ = e.runtime.Delete(containerName, true)
		if err := e.startService(svcName); err != nil {
			slog.Warn("start", "service", svcName, "err", err)
		}
	}

	return nil
}

// Y25: Restart restarts all services.
func (e *Engine) Restart() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	for svcName := range e.file.Services {
		svc := e.file.Services[svcName]
		containerName := e.containerName(svcName)
		timeout := 10
		if svc.StopGracePeriod != "" {
			if d, err := parseDuration(svc.StopGracePeriod); err == nil {
				timeout = int(d.Seconds())
			}
		}
		if err := e.runtime.Stop(containerName, timeout); err != nil {
			slog.Warn("stop", "service", svcName, "err", err)
		}
		if err := e.runtime.Delete(containerName, true); err != nil {
			slog.Warn("delete", "service", svcName, "err", err)
		}
		if err := e.startService(svcName); err != nil {
			return fmt.Errorf("restart %s: %w", svcName, err)
		}
	}

	return nil
}

// Y25: Logs returns logs for all services.
func (e *Engine) Logs(tail int) (map[string]string, error) {
	if e.file == nil {
		return nil, fmt.Errorf("no compose file loaded")
	}

	logs := make(map[string]string)
	for svcName := range e.file.Services {
		containerName := e.containerName(svcName)
		logStr, err := e.runtime.GetLogs(containerName, tail)
		if err != nil {
			logs[svcName] = fmt.Sprintf("(error: %v)", err)
			continue
		}
		logs[svcName] = logStr
	}
	return logs, nil
}

// Y25: Pull pulls images for all services.
func (e *Engine) Pull() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	pulled := make(map[string]bool) // Track pulled images to avoid duplicates

	for name, svc := range e.file.Services {
		imageName := svc.Image
		if imageName == "" {
			imageName = e.project + "_" + name
		}

		if pulled[imageName] {
			continue // Skip if already pulled
		}

		if _, err := e.image.Pull(imageName); err != nil {
			return fmt.Errorf("pull %s: %w", imageName, err)
		}
		fmt.Printf("Pulled %s\n", imageName)
		pulled[imageName] = true
	}
	return nil
}

// Y11: Build builds images for services with build config.
func (e *Engine) Build() error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}

	bldr := builder.NewBuilder(e.image)

	for _, svc := range e.file.Services {
		if svc.Build == nil {
			continue
		}

		context := svc.Build.Context
		if context == "" {
			context = "."
		}
		if !filepath.IsAbs(context) {
			context = filepath.Join(e.projectDir, context)
		}

		dockerfile := svc.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = svc.Build.Dokifile
		}
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}
		// Resolve dockerfile path relative to context if not absolute
		if !filepath.IsAbs(dockerfile) {
			dockerfile = filepath.Join(context, dockerfile)
		}

		buildCfg := &builder.BuildConfig{
			Context:     context,
			Dokifile:    dockerfile,
			Tags:        []string{svc.Image},
			BuildArgs:   svc.Build.Args,
			Labels:      svc.Build.Labels,
			Target:      svc.Build.Target,
			NetworkMode: svc.Build.Network,
		}

		fmt.Printf("Building %s...\n", context)
		if err := bldr.Build(buildCfg); err != nil {
			return fmt.Errorf("build %s: %w", context, err)
		}
		fmt.Printf("Build completed for %s\n", context)
	}
	return nil
}

// Config validates the compose file and returns it as YAML.
func (e *Engine) Config() (string, error) {
	if e.file == nil {
		return "", fmt.Errorf("no compose file loaded")
	}
	data, err := yaml.Marshal(e.file)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Ps lists running services.
func (e *Engine) Ps() ([]common.ContainerInfo, error) {
	states, err := e.runtime.List()
	if err != nil {
		return nil, err
	}

	containers := make([]common.ContainerInfo, 0)
	for _, state := range states {
		name := state.ID
		// Filter: only include containers that belong to this compose project.
		// Containers may be named by container_name (e.g., "test-db-svc")
		// or by project convention (e.g., "project_service_1").
		belongsToProject := false
		for svcName := range e.file.Services {
			if e.containerName(svcName) == name || e.containerNameN(svcName, 1) == name {
				belongsToProject = true
				break
			}
		}
		if !belongsToProject {
			continue
		}

		shortID := state.ID

		// Determine image from state or service config.
		imageName := ""
		for svcName, svc := range e.file.Services {
			if e.containerName(svcName) == name || e.containerNameN(svcName, 1) == name {
				imageName = svc.Image
				break
			}
		}

		cmd := ""
		if state.Config != nil && len(state.Config.Args) > 0 {
			cmd = strings.Join(state.Config.Args, " ")
		}
		if len(cmd) > 20 {
			cmd = cmd[:17] + "..."
		}

		status := string(state.Status)
		if state.Status == common.StateRunning {
			status = "Up"
		}

		containers = append(containers, common.ContainerInfo{
			ID:      shortID,
			Image:   imageName,
			Names:   []string{"/" + name},
			Command: cmd,
			State:   state.Status,
			Status:  status,
			Created: state.Created.Unix(),
		})
	}

	return containers, nil
}

func (e *Engine) containerName(svcName string) string {
	svc := e.file.Services[svcName]
	if svc != nil && svc.ContainerName != "" {
		return svc.ContainerName
	}
	return e.project + "_" + svcName + "_1"
}

func (e *Engine) containerNameN(svcName string, n int) string {
	svc := e.file.Services[svcName]
	if svc != nil && svc.ContainerName != "" {
		return svc.ContainerName
	}
	return fmt.Sprintf("%s_%s_%d", e.project, svcName, n)
}

func (e *Engine) startService(name string) error {
	return e.startServiceN(name, 1)
}

func (e *Engine) startServiceN(name string, instance int) error {
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
	// String-form command should be run through a shell (Docker compat)
	if _, isStr := svc.Command.(string); isStr && len(cmd) == 1 {
		cmd = []string{"/bin/sh", "-c", cmd[0]}
	}
	if len(cmd) == 0 {
		if record, err := e.image.Get(imageName); err == nil && record.Config != nil {
			cmd = record.Config.Config.Cmd
		}
	}

	// Merge Entrypoint with Cmd (service's Entrypoint overrides image's)
	entrypoint := toStringSlice(svc.Entrypoint)
	if len(entrypoint) == 0 {
		if record, err := e.image.Get(imageName); err == nil && record.Config != nil {
			entrypoint = record.Config.Config.Entrypoint
		}
	}
	if len(entrypoint) > 0 {
		cmd = append(entrypoint, cmd...)
	}
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	containerID := e.containerNameN(name, instance)

	// Resolve image layers so rootfs gets populated.
	var imageLayers []string
	if _, err := e.image.Get(imageName); err == nil {
		imageLayers, _ = e.image.GetLayerPaths(imageName)
	}

	cfg := &runtime.Config{
		ID:          containerID,
		Args:        cmd,
		Env:         e.buildEnv(svc, imageName),
		Tty:         svc.Tty,
		ImageLayers: imageLayers,
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

	// Y13-Y21: Wire all compose fields into container config.
	cfg.User = svc.User
	cfg.Privileged = svc.Privileged
	cfg.ReadOnly = svc.ReadOnly
	cfg.Init = svc.Init

	cfg.DNS = toStringSlice(svc.DNS)
	cfg.DNSSearch = toStringSlice(svc.DNSSearch)
	cfg.DNSOptions = toStringSlice(svc.DNSOpt)
	cfg.ExtraHosts = toStringSlice(svc.ExtraHosts)
	cfg.CapAdd = svc.CapAdd
	cfg.CapDrop = svc.CapDrop
	cfg.SecurityOpt = svc.SecurityOpt

	// Wire compose niche fields into container config
	if svc.ShmSize != "" {
		sz := parseMemory(svc.ShmSize)
		if sz > 0 {
			if cfg.Resources == nil {
				cfg.Resources = &runtime.Resources{}
			}
			cfg.Resources.ShmSize = sz
		}
	}
	if svc.PidsLimit > 0 {
		if cfg.Resources == nil {
			cfg.Resources = &runtime.Resources{}
		}
		cfg.Resources.PidsLimit = svc.PidsLimit
	}
	if svc.OomKillDisable {
		if cfg.Resources == nil {
			cfg.Resources = &runtime.Resources{}
		}
		cfg.Resources.OomKillDisable = true
	}

	if svc.Sysctls != nil {
		cfg.Sysctls = svc.Sysctls
	}

	if svc.StopSignal != "" {
		cfg.StopSignal = svc.StopSignal
	}

	if svc.StopGracePeriod != "" {
		if d, err := parseDuration(svc.StopGracePeriod); err == nil {
			cfg.StopTimeout = int(d.Seconds())
		}
	}

	// Y9: Restart policy.
	if svc.Restart != "" {
		switch svc.Restart {
		case "always":
			cfg.RestartPolicy = common.RestartAlways
		case "unless-stopped":
			cfg.RestartPolicy = common.RestartUnlessStopped
		default:
			if strings.HasPrefix(svc.Restart, "on-failure") {
				cfg.RestartPolicy = common.RestartOnFailure
				if strings.HasPrefix(svc.Restart, "on-failure:") {
					if n, err := strconv.Atoi(strings.TrimPrefix(svc.Restart, "on-failure:")); err == nil {
						cfg.RestartMaxRetries = n
					}
				}
			}
		}
	}

	// Y6: Deploy resources.
	if svc.Deploy != nil && svc.Deploy.Resources != nil {
		if cfg.Resources == nil {
			cfg.Resources = &runtime.Resources{}
		}
		if svc.Deploy.Resources.Limits != nil {
			if svc.Deploy.Resources.Limits.CPUs != "" {
				cfg.Resources.NanoCpus = parseCPU(svc.Deploy.Resources.Limits.CPUs)
			}
			if svc.Deploy.Resources.Limits.Memory != "" {
				cfg.Resources.Memory = parseMemory(svc.Deploy.Resources.Limits.Memory)
			}
		}
	}

	// Y7: Healthcheck.
	if svc.Healthcheck != nil && !svc.Healthcheck.Disable {
		cfg.HealthCheck = &runtime.HealthCheckConfig{
			Test:    toStringSlice(svc.Healthcheck.Test),
			Retries: svc.Healthcheck.Retries,
		}
		if svc.Healthcheck.Interval != "" {
			if d, err := parseDuration(svc.Healthcheck.Interval); err == nil {
				cfg.HealthCheck.Interval = d
			}
		}
		if svc.Healthcheck.Timeout != "" {
			if d, err := parseDuration(svc.Healthcheck.Timeout); err == nil {
				cfg.HealthCheck.Timeout = d
			}
		}
		if cfg.HealthCheck.Retries == 0 {
			cfg.HealthCheck.Retries = 3
		}
	}

	// Parse ports (support short and long syntax).
	portSpecs := toStringSlice(svc.Ports)
	for _, ps := range portSpecs {
		port, _, err := common.ParsePortBinding(ps)
		if err != nil {
			return fmt.Errorf("parse port %s: %w", ps, err)
		}
		cfg.Ports = append(cfg.Ports, port)
	}

	// Y1-Y3: Parse volumes (support short and long syntax).
	volSpecs := toStringSlice(svc.Volumes)
	for _, vs := range volSpecs {
		mnt := parseVolume(vs, e.projectDir)
		if mnt.Target != "" {
			cfg.Mounts = append(cfg.Mounts, mnt)
		}
	}

	// Y4: Secrets - mount files into /run/secrets/<name>.
	for _, secretName := range svc.Secrets {
		if secretDef, ok := e.file.Secrets[secretName]; ok && secretDef.File != "" {
			secretPath := secretDef.File
			if !filepath.IsAbs(secretPath) {
				secretPath = filepath.Join(e.projectDir, secretPath)
			}
			cfg.Mounts = append(cfg.Mounts, common.Mount{
				Type:     common.MountBind,
				Source:   secretPath,
				Target:   "/run/secrets/" + secretName,
				ReadOnly: true,
			})
		}
	}

	// Y5: Configs - mount files into /<name>.
	for _, configName := range svc.Configs {
		if configDef, ok := e.file.Configs[configName]; ok && configDef.File != "" {
			configPath := configDef.File
			if !filepath.IsAbs(configPath) {
				configPath = filepath.Join(e.projectDir, configPath)
			}
			cfg.Mounts = append(cfg.Mounts, common.Mount{
				Type:     common.MountBind,
				Source:   configPath,
				Target:   "/" + configName,
				ReadOnly: true,
			})
		}
	}

	// Wire remaining compose fields
	if svc.Platform != "" {
		cfg.ImageRef = svc.Platform + "/" + imageName
	} else {
		cfg.ImageRef = imageName
	}
	if svc.Runtime != "" {
		cfg.Runtime = svc.Runtime
	}
	// Devices
	for _, dev := range svc.Devices {
		parts := strings.SplitN(dev, ":", 3)
		if len(parts) >= 2 {
			cfg.Mounts = append(cfg.Mounts, common.Mount{
				Type:   common.MountBind,
				Source: parts[0],
				Target: parts[1],
			})
		}
	}
	// Tmpfs mounts
	tmpfsSpecs := toStringSlice(svc.Tmpfs)
	for _, ts := range tmpfsSpecs {
		cfg.Mounts = append(cfg.Mounts, common.Mount{
			Type:   common.MountTmpfs,
			Target: ts,
		})
	}

	// Create container.
	if _, err := e.runtime.Create(cfg); err != nil {
		var conflict *common.ErrConflict
		if errors.As(err, &conflict) {
			// Container already exists — force delete and recreate.
			_ = e.runtime.Delete(containerID, true)
			if _, err := e.runtime.Create(cfg); err != nil {
				return fmt.Errorf("recreate %s: %w", name, err)
			}
		} else {
			return fmt.Errorf("create %s: %w", name, err)
		}
	}

	// Y1/Y3: Connect container to networks.
	if svc.NetworkMode != "host" && svc.NetworkMode != "none" {
		networkNames := e.serviceNetworks(name, svc)
		for _, netName := range networkNames {
			nw, err := e.network.GetNetwork(e.project + "_" + netName)
			if err != nil {
				continue
			}
			if err := e.network.Connect(nw.ID, containerID, "", nil, nil, 0); err != nil {
				slog.Warn("connect container to network", "network", nw.ID, "container", containerID, "err", err)
			}
		}
	}

	// Start container.
	if err := e.runtime.Start(cfg.ID); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}

	return nil
}

// Y12: buildEnv builds the environment slice including env_file and image defaults.
func (e *Engine) buildEnv(svc *Service, imageName string) []string {
	env := toEnvSlice(svc.Environment)

	// Include image default environment variables
	if record, err := e.image.Get(imageName); err == nil && record.Config != nil {
		for _, imgEnv := range record.Config.Config.Env {
			if !containsKey(env, imgEnv) {
				env = append(env, imgEnv)
			}
		}
	}

	// Y12: env_file support.
	envFile := svc.EnvFile
	if envFile != nil {
		var paths []string
		switch v := envFile.(type) {
		case string:
			paths = []string{v}
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					paths = append(paths, s)
				}
			}
		case []string:
			paths = v
		}
		for _, p := range paths {
			fullPath := p
			if !filepath.IsAbs(fullPath) {
				fullPath = filepath.Join(e.projectDir, fullPath)
			}
			env = append(env, readEnvFile(fullPath)...)
		}
	}

	return env
}

// containsKey checks if env slice already has a key defined.
func containsKey(env []string, entry string) bool {
	if idx := strings.Index(entry, "="); idx > 0 {
		key := entry[:idx+1]
		for _, e := range env {
			if strings.HasPrefix(e, key) {
				return true
			}
		}
	}
	return false
}

// Y1: serviceNetworks returns the list of networks for a service.
func (e *Engine) serviceNetworks(name string, svc *Service) []string {
	if svc.Networks == nil {
		// Default: connect to the default network if any networks are defined.
		if len(e.file.Networks) > 0 {
			for netName := range e.file.Networks {
				return []string{netName}
			}
		}
		return nil
	}

	switch v := svc.Networks.(type) {
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	case map[string]interface{}:
		result := make([]string, 0, len(v))
		for k := range v {
			result = append(result, k)
		}
		return result
	}
	return nil
}

func (e *Engine) orderServices() ([]string, error) {
	ordered := make([]string, 0, len(e.file.Services))
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(name string) bool
	dfs = func(name string) bool {
		if inStack[name] {
			return false
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
			return nil, fmt.Errorf("circular dependency detected involving service %q", name)
		}
	}

	return ordered, nil
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
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		result := make([]string, 0, len(val))
		for _, k := range keys {
			result = append(result, fmt.Sprintf("%s=%v", k, val[k]))
		}
		return result
	}

	return nil
}

// parseVolume parses a volume specification string into a Mount.
// Formats: "source:target:mode", "target", "volume_name:/target", etc.
func parseVolume(spec, projectDir string) common.Mount {
	// Long syntax: {type: volume, source: ..., target: ..., read_only: true}
	if strings.HasPrefix(strings.TrimSpace(spec), "{") {
		var v struct {
			Type     string `yaml:"type"`
			Source   string `yaml:"source"`
			Target   string `yaml:"target"`
			ReadOnly bool   `yaml:"read_only"`
		}
		if err := yaml.Unmarshal([]byte(spec), &v); err == nil && v.Target != "" {
			mountType := common.MountBind
			if v.Type == "volume" {
				mountType = common.MountVolume
			} else if v.Type == "tmpfs" {
				mountType = common.MountTmpfs
			}
			return common.Mount{
				Type:     mountType,
				Source:   v.Source,
				Target:   v.Target,
				ReadOnly: v.ReadOnly,
			}
		}
	}
	// Short syntax: src:dst:mode
	parts := strings.Split(spec, ":")
	mnt := common.Mount{Type: common.MountBind}

	switch len(parts) {
	case 1:
		// Anonymous volume: just the target path.
		mnt.Target = parts[0]
		mnt.Type = common.MountVolume
	case 2:
		if strings.HasPrefix(parts[1], "/") || strings.HasPrefix(parts[1], ".") {
			// source:target
			mnt.Source = parts[0]
			mnt.Target = parts[1]
		} else if strings.HasPrefix(parts[0], "/") || strings.HasPrefix(parts[0], ".") {
			// ./data:/app or /host/path:/container/path
			mnt.Source = parts[0]
			mnt.Target = parts[1]
		} else {
			// target:mode
			mnt.Target = parts[0]
			if parts[1] == "ro" {
				mnt.ReadOnly = true
			}
		}
	case 3:
		mnt.Source = parts[0]
		mnt.Target = parts[1]
		if parts[2] == "ro" {
			mnt.ReadOnly = true
		}
	}

	// Resolve relative source paths.
	if mnt.Source != "" && !filepath.IsAbs(mnt.Source) && !strings.HasPrefix(mnt.Source, ".") {
		// Named volume, not a path.
		mnt.Type = common.MountVolume
	} else if mnt.Source != "" && strings.HasPrefix(mnt.Source, ".") {
		mnt.Source = filepath.Join(projectDir, mnt.Source)
	}

	return mnt
}

// readEnvFile reads a .env file in the format KEY=VALUE.
func readEnvFile(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var result []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		result = append(result, line)
	}
	return result
}

// parseCPU parses a CPU count string like "1.5" into NanoCpus.
func parseCPU(s string) int64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(f * 1e9)
}

// parseMemory parses a memory string like "512M" or "1G" into bytes.
func parseMemory(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	multiplier := int64(1)
	last := s[len(s)-1]

	switch {
	case last == 'B' || last == 'b':
		s = s[:len(s)-1]
		if len(s) > 0 && (s[len(s)-1] == 'K' || s[len(s)-1] == 'k') {
			multiplier = 1024
			s = s[:len(s)-1]
		} else if len(s) > 0 && (s[len(s)-1] == 'M' || s[len(s)-1] == 'm') {
			multiplier = 1024 * 1024
			s = s[:len(s)-1]
		} else if len(s) > 0 && (s[len(s)-1] == 'G' || s[len(s)-1] == 'g') {
			multiplier = 1024 * 1024 * 1024
			s = s[:len(s)-1]
		}
	case last == 'K' || last == 'k':
		multiplier = 1024
		s = s[:len(s)-1]
	case last == 'M' || last == 'm':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case last == 'G' || last == 'g':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(val * float64(multiplier))
}

// parseDuration parses a duration string like "30s", "5m", "1h".
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err == nil {
		return d, nil
	}
	if n, err2 := strconv.Atoi(s); err2 == nil {
		return time.Duration(n) * time.Second, nil
	}
	return 0, err
}

func (e *Engine) StopServices(services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	for svcName, svc := range targets {
		containerName := e.containerName(svcName)
		timeout := 10
		if svc.StopGracePeriod != "" {
			if d, err := parseDuration(svc.StopGracePeriod); err == nil {
				timeout = int(d.Seconds())
			}
		}
		if err := e.runtime.Stop(containerName, timeout); err != nil {
			slog.Warn("stop", "service", svcName, "err", err)
		}
	}
	return nil
}

func (e *Engine) StartServices(services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	for svcName := range targets {
		containerName := e.containerName(svcName)
		state, err := e.runtime.State(containerName)
		if err != nil || state == nil {
			if err := e.startService(svcName); err != nil {
				slog.Warn("start", "service", svcName, "err", err)
			}
			continue
		}
		if state.Status == common.StateRunning {
			continue
		}
		_ = e.runtime.Delete(containerName, true)
		if err := e.startService(svcName); err != nil {
			slog.Warn("start", "service", svcName, "err", err)
		}
	}
	return nil
}

func (e *Engine) RestartServices(services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	for svcName, svc := range targets {
		containerName := e.containerName(svcName)
		timeout := 10
		if svc.StopGracePeriod != "" {
			if d, err := parseDuration(svc.StopGracePeriod); err == nil {
				timeout = int(d.Seconds())
			}
		}
		if err := e.runtime.Stop(containerName, timeout); err != nil {
			slog.Warn("stop", "service", svcName, "err", err)
		}
		if err := e.runtime.Delete(containerName, true); err != nil {
			slog.Warn("delete", "service", svcName, "err", err)
		}
		if err := e.startService(svcName); err != nil {
			return fmt.Errorf("restart %s: %w", svcName, err)
		}
	}
	return nil
}

func (e *Engine) KillServices(signal string, services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	sig := parseSignalStr(signal)
	for svcName := range targets {
		containerName := e.containerName(svcName)
		if err := e.runtime.Kill(containerName, sig); err != nil {
			slog.Warn("kill", "service", svcName, "err", err)
		}
	}
	return nil
}

func parseSignalStr(s string) syscall.Signal {
	s = strings.TrimPrefix(strings.ToUpper(s), "SIG")
	signals := map[string]syscall.Signal{
		"HUP": syscall.SIGHUP, "INT": syscall.SIGINT, "QUIT": syscall.SIGQUIT,
		"KILL": syscall.SIGKILL, "USR1": syscall.SIGUSR1, "USR2": syscall.SIGUSR2,
		"TERM": syscall.SIGTERM, "CONT": syscall.SIGCONT, "STOP": syscall.SIGSTOP,
		"9": syscall.SIGKILL, "15": syscall.SIGTERM, "1": syscall.SIGHUP,
	}
	if sig, ok := signals[s]; ok {
		return sig
	}
	return syscall.SIGTERM
}

func (e *Engine) PauseServices(services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	for svcName := range targets {
		containerName := e.containerName(svcName)
		if err := e.runtime.Pause(containerName); err != nil {
			slog.Warn("pause", "service", svcName, "err", err)
		}
	}
	return nil
}

func (e *Engine) UnpauseServices(services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	for svcName := range targets {
		containerName := e.containerName(svcName)
		if err := e.runtime.Unpause(containerName); err != nil {
			slog.Warn("unpause", "service", svcName, "err", err)
		}
	}
	return nil
}

func (e *Engine) Create() error {
	return e.Up()
}

func (e *Engine) Top(services []string) error {
	if e.file == nil {
		return fmt.Errorf("no compose file loaded")
	}
	targets := e.file.Services
	if len(services) > 0 {
		targets = make(map[string]*Service)
		for _, s := range services {
			svc, ok := e.file.Services[s]
			if !ok {
				return fmt.Errorf("no such service: %s", s)
			}
			targets[s] = svc
		}
	}
	for svcName := range targets {
		containerName := e.containerName(svcName)
		fmt.Printf("=== %s ===\n", svcName)
		state, err := e.runtime.State(containerName)
		if err != nil {
			fmt.Printf("  (error: %v)\n", err)
			continue
		}
		if state.Status != common.StateRunning {
			fmt.Printf("  (not running: %s)\n", state.Status)
			continue
		}
		fmt.Printf("  PID: %d  STATUS: %s\n", state.Pid, state.Status)
		if state.Config != nil && len(state.Config.Args) > 0 {
			fmt.Printf("  CMD: %s\n", strings.Join(state.Config.Args, " "))
		}
	}
	return nil
}

func (e *Engine) PrintConfigSubset(services, volumes, networks bool) {
	if e.file == nil {
		return
	}
	if services {
		names := make([]string, 0, len(e.file.Services))
		for name := range e.file.Services {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
	}
	if volumes {
		names := make([]string, 0, len(e.file.Volumes))
		for name := range e.file.Volumes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
	}
	if networks {
		names := make([]string, 0, len(e.file.Networks))
		for name := range e.file.Networks {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
	}
}

func (e *Engine) ProjectImages() ([]common.ImageInfo, error) {
	if e.file == nil {
		return nil, fmt.Errorf("no compose file loaded")
	}
	allImgs, err := e.image.List()
	if err != nil {
		return nil, err
	}
	projectImgs := make(map[string]bool)
	for _, svc := range e.file.Services {
		if svc.Image != "" {
			projectImgs[svc.Image] = true
		}
	}
	var result []common.ImageInfo
	for _, img := range allImgs {
		for _, tag := range img.RepoTags {
			if projectImgs[tag] {
				result = append(result, img)
				break
			}
		}
	}
	return result, nil
}
