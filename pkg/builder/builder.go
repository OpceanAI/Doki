package builder

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/registry"
)

// Dockerignore represents parsed .dockerignore rules.
type Dockerignore struct {
	patterns []string
}

// ParseDockerignore parses a .dockerignore file.
func ParseDockerignore(path string) (*Dockerignore, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Dockerignore{}, nil
		}
		return nil, err
	}
	defer f.Close()

	di := &Dockerignore{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		di.patterns = append(di.patterns, line)
	}
	return di, scanner.Err()
}

// Matches checks if a path matches any .dockerignore pattern.
func (d *Dockerignore) Matches(path string) bool {
	for _, pattern := range d.patterns {
		// Remove leading / for relative matching
		cleanPattern := strings.TrimPrefix(pattern, "/")
		if matched, _ := filepath.Match(cleanPattern, path); matched {
			return true
		}
		if matched, _ := filepath.Match(cleanPattern, filepath.Base(path)); matched {
			return true
		}
		// Handle ** patterns
		if strings.Contains(cleanPattern, "**") {
			if matchDoubleStar(cleanPattern, path) {
				return true
			}
		}
	}
	return false
}

func matchDoubleStar(pattern, name string) bool {
	parts := strings.Split(pattern, "**")
	switch len(parts) {
	case 1:
		matched, _ := filepath.Match(pattern, name)
		return matched
	case 2:
		for i := 0; i <= len(name); i++ {
			if matchDoubleStar(parts[0], name[:i]) {
				if matched, _ := filepath.Match(parts[1], name[i:]); matched {
					return true
				}
				if matchDoubleStar(parts[1], name[i:]) {
					return true
				}
			}
		}
	}
	return false
}

// DokifileParser parses a Dokifile (Dockerfile-compatible).
type DokifileParser struct {
	escape     byte
	parsed     bool
	directives map[string]string
	stages     []*Stage
}

// Stage represents a build stage in a Dokifile.
type Stage struct {
	Name         string
	From         string
	FromStage    string
	Platform     string
	Instructions []Instruction
	Metadata     map[string]string
	ImageConfig  *image.ImageConfig
	Layers       []string
}

// Instruction represents a single instruction in a Dokifile.
type Instruction struct {
	Type    string
	Args    []string
	Raw     string
	LineNum int
}

// BuildConfig holds configuration for a build.
type BuildConfig struct {
	Context     string
	Dokifile    string
	Tags        []string
	BuildArgs   map[string]string
	Labels      map[string]string
	NoCache     bool
	Pull        bool
	Target      string
	Platform    string
	NetworkMode string
	ExtraHosts  []string
	Output      string
	Secrets     map[string]string
	SSH         []string
	ContextTar  string
}

// Builder builds OCI images from Dokifiles.
type Builder struct {
	store         *image.Store
	registry      *registry.Client
	stageDirs     map[string]string
	envMap        map[string]string
	argDefaults   map[string]string
	cacheDir      string
	secrets       map[string]string
	dockerignore  *Dockerignore
	noCache       bool
}

// NewBuilder creates a new image builder.
func NewBuilder(store *image.Store) *Builder {
	return &Builder{
		store:       store,
		registry:    registry.NewClient(false),
		stageDirs:   make(map[string]string),
		envMap:      make(map[string]string),
		argDefaults: make(map[string]string),
		secrets:     make(map[string]string),
	}
}

// NewDokifileParser creates a new Dokifile parser.
func NewDokifileParser() *DokifileParser {
	return &DokifileParser{
		escape:     '\\',
		directives: make(map[string]string),
	}
}

// Parse parses a Dokifile.
func (p *DokifileParser) Parse(content []byte) error {
	lines := strings.Split(string(content), "\n")
	p.stages = nil

	var currentStage *Stage
	var heredocLines []string
	inHeredoc := false
	heredocEnd := ""
	heredocStripTabs := false

	for i, line := range lines {
		origLine := line
		trimmedLine := strings.TrimSpace(line)

		if trimmedLine == "" {
			continue
		}

		if strings.HasPrefix(trimmedLine, "#") {
			if !p.parsed {
				l := strings.TrimPrefix(trimmedLine, "#")
				l = strings.TrimSpace(l)
				if idx := strings.Index(l, "="); idx > 0 {
					key := strings.TrimSpace(l[:idx])
					val := strings.TrimSpace(l[idx+1:])
					p.directives[key] = val
				}
			}
			continue
		}

		if !p.parsed {
			p.parsed = true
		}

		if inHeredoc {
			testLine := origLine
			if heredocStripTabs {
				testLine = strings.TrimLeft(testLine, "\t")
			}
			if strings.TrimSpace(testLine) == heredocEnd {
				inHeredoc = false
				if currentStage != nil && len(currentStage.Instructions) > 0 {
					last := &currentStage.Instructions[len(currentStage.Instructions)-1]
					last.Raw += "\n" + strings.Join(heredocLines, "\n")
				}
				heredocLines = nil
			} else {
				heredocLines = append(heredocLines, origLine)
			}
			continue
		}

		instruction, err := parseLine(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		if instruction == nil {
			continue
		}

		instruction.LineNum = i + 1

		if heredocDelim, stripTabs := detectHeredoc(origLine, instruction.Type); heredocDelim != "" {
			inHeredoc = true
			heredocEnd = heredocDelim
			heredocStripTabs = stripTabs
		}

		if instruction.Type == "FROM" {
			currentStage = p.parseFromInstruction(instruction)
			p.stages = append(p.stages, currentStage)
		} else if currentStage != nil {
			currentStage.Instructions = append(currentStage.Instructions, *instruction)
		}
	}

	// Set FromStage for stages whose From matches another stage's Name.
	for _, stage := range p.stages {
		for _, other := range p.stages {
			if stage != other && stage.From == other.Name {
				stage.FromStage = other.Name
				break
			}
		}
	}

	return nil
}

func (p *DokifileParser) parseFromInstruction(inst *Instruction) *Stage {
	stage := &Stage{
		Instructions: make([]Instruction, 0),
		Metadata:     make(map[string]string),
	}

	args := inst.Args
	for i, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--platform="):
			stage.Platform = strings.TrimPrefix(arg, "--platform=")
		case arg == "AS" && i+1 < len(args):
			stage.Name = args[i+1]
		case !strings.HasPrefix(arg, "--") && stage.From == "":
			stage.From = arg
		}
	}

	return stage
}

func parseLine(line string) (*Instruction, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, nil
	}

	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 1 {
		return nil, nil
	}

	instType := strings.ToUpper(parts[0])
	argsStr := ""
	if len(parts) > 1 {
		argsStr = parts[1]
	}

	validInstructions := map[string]bool{
		"FROM": true, "RUN": true, "CMD": true, "LABEL": true,
		"EXPOSE": true, "ENV": true, "ADD": true, "COPY": true,
		"ENTRYPOINT": true, "VOLUME": true, "USER": true,
		"WORKDIR": true, "ARG": true, "ONBUILD": true,
		"STOPSIGNAL": true, "HEALTHCHECK": true, "SHELL": true,
		"MAINTAINER": true,
	}

	if !validInstructions[instType] {
		return nil, fmt.Errorf("unknown instruction: %s", instType)
	}

	args := parseArgs(argsStr)

	return &Instruction{
		Type: instType,
		Args: args,
		Raw:  line,
	}, nil
}

func detectHeredoc(line, instType string) (string, bool) {
	// Look for <<DELIM or <<-DELIM
	idx := strings.Index(line, "<<")
	if idx < 0 {
		return "", false
	}
	rest := line[idx+2:]
	stripTabs := false
	if len(rest) > 0 && rest[0] == '-' {
		stripTabs = true
		rest = rest[1:]
	}
	// Extract delimiter: take characters until whitespace or end of string
	rest = strings.TrimLeft(rest, " \t")
	delim := ""
	for _, c := range rest {
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			break
		}
		delim += string(c)
	}
	if delim == "" {
		return "", false
	}
	if instType != "RUN" && instType != "COPY" {
		return "", false
	}
	return delim, stripTabs
}

var validVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func substituteVarsInSlice(args []string, envMap, argDefaults, buildArgs map[string]string) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		result[i] = substituteVars(arg, envMap, argDefaults, buildArgs)
	}
	return result
}

func substituteVars(s string, envMap, argDefaults, buildArgs map[string]string) string {
	// Replace ${VAR} and $VAR
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '$' {
			result.WriteByte('$')
			i += 2
			continue
		}
		if s[i] == '$' {
			if i+1 < len(s) && s[i+1] == '{' {
				// ${VAR}
				end := strings.IndexByte(s[i+2:], '}')
				if end >= 0 {
					varName := s[i+2 : i+2+end]
					val := resolveVar(varName, envMap, argDefaults, buildArgs)
					result.WriteString(val)
					i += 3 + end
					continue
				}
			} else {
				// $VAR
				j := i + 1
				for j < len(s) && (s[j] >= 'A' && s[j] <= 'Z' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= '0' && s[j] <= '9' || s[j] == '_') {
					j++
				}
				if j > i+1 {
					varName := s[i+1 : j]
					val := resolveVar(varName, envMap, argDefaults, buildArgs)
					result.WriteString(val)
					i = j
					continue
				}
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

func resolveVar(name string, envMap, argDefaults, buildArgs map[string]string) string {
	if v, ok := buildArgs[name]; ok {
		return v
	}
	if v, ok := envMap[name]; ok {
		return v
	}
	if v, ok := argDefaults[name]; ok {
		return v
	}
	return ""
}

func (b *Builder) ensureCacheDir() string {
	if b.cacheDir != "" {
		return b.cacheDir
	}
	home, _ := os.UserHomeDir()
	b.cacheDir = filepath.Join(home, ".doki", "cache", "build")
	common.EnsureDir(b.cacheDir)
	return b.cacheDir
}

func (b *Builder) saveLayer(rootDir string, createdBy string) (string, int64, error) {
	var buf bytes.Buffer
	if err := CreateTar(rootDir, &buf); err != nil {
		return "", 0, fmt.Errorf("create tar: %w", err)
	}

	h := sha256.New()
	h.Write(buf.Bytes())
	digestHex := hex.EncodeToString(h.Sum(nil))
	digest := "sha256:" + digestHex

	layerPath := b.store.GetLayerPath(digest)
	common.EnsureDir(filepath.Dir(layerPath))
	if err := os.WriteFile(layerPath, buf.Bytes(), 0644); err != nil {
		return "", 0, fmt.Errorf("write layer: %w", err)
	}

	return digest, int64(len(buf.Bytes())), nil
}

func (b *Builder) instructionHash(inst *Instruction, envMap map[string]string) string {
	h := sha256.New()
	h.Write([]byte(inst.Raw))
	for k, v := range envMap {
		h.Write([]byte(k + "=" + v))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (b *Builder) checkCache(inst *Instruction, envMap map[string]string) (string, bool) {
	if b.noCache {
		return "", false
	}
	hash := b.instructionHash(inst, envMap)
	cacheDir := b.ensureCacheDir()
	cachePath := filepath.Join(cacheDir, hash+".tar")
	if common.PathExists(cachePath) {
		return cachePath, true
	}
	return "", false
}

func (b *Builder) saveCache(inst *Instruction, envMap map[string]string, tarData []byte) error {
	hash := b.instructionHash(inst, envMap)
	cacheDir := b.ensureCacheDir()
	cachePath := filepath.Join(cacheDir, hash+".tar")
	return os.WriteFile(cachePath, tarData, 0644)
}

func parseChown(s string) (string, string) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 1 {
		parts = strings.SplitN(s, ".", 2)
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func applyChowChmod(path string, chown, chmod string) error {
	if chmod != "" {
		mode, err := strconv.ParseUint(chmod, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid chmod: %s", chmod)
		}
		if err := os.Chmod(path, os.FileMode(mode)); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}
	}
	if chown != "" {
		uidStr, gidStr := parseChown(chown)
		uid := -1
		gid := -1
		if uidStr != "" {
			uid, _ = strconv.Atoi(uidStr)
		}
		if gidStr != "" {
			gid, _ = strconv.Atoi(gidStr)
		}
		if uid >= 0 && gid >= 0 {
			if err := os.Chown(path, uid, gid); err != nil {
				return fmt.Errorf("chown %s: %w", path, err)
			}
		}
	}
	return nil
}

func applyChowChmodRecursive(root string, chown, chmod string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return applyChowChmod(path, chown, chmod)
	})
}

// GetStages returns all parsed stages.
func (p *DokifileParser) GetStages() []*Stage {
	return p.stages
}

// GetDirective returns a parser directive value.
func (p *DokifileParser) GetDirective(key string) string {
	return p.directives[key]
}

// Build executes a build from a Dokifile.
func (b *Builder) Build(cfg *BuildConfig) error {
	// Parse .dockerignore
	if di, err := ParseDockerignore(filepath.Join(cfg.Context, ".dockerignore")); err == nil {
		b.dockerignore = di
	}

	// Initialize build args and secrets
	b.envMap = make(map[string]string)
	b.argDefaults = make(map[string]string)
	for k, v := range cfg.BuildArgs {
		b.argDefaults[k] = v
	}
	b.secrets = cfg.Secrets
	if b.secrets == nil {
		b.secrets = make(map[string]string)
	}
	b.noCache = cfg.NoCache

	// Handle context tar
	contextDir := cfg.Context
	if cfg.ContextTar != "" {
		tmpDir, err := os.MkdirTemp("", "doki-context-")
		if err != nil {
			return fmt.Errorf("create context temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		f, err := os.Open(cfg.ContextTar)
		if err != nil {
			return fmt.Errorf("open context tar: %w", err)
		}
		defer f.Close()
		if err := ExtractTar(f, tmpDir); err != nil {
			return fmt.Errorf("extract context tar: %w", err)
		}
		contextDir = tmpDir
	}

	// Read Dokifile
	dokifilePath := filepath.Join(cfg.Context, cfg.Dokifile)
	if cfg.Dokifile == "" {
		for _, name := range []string{"Dokifile", "dokifile", "Dockerfile", "dockerfile"} {
			if common.PathExists(filepath.Join(cfg.Context, name)) {
				dokifilePath = filepath.Join(cfg.Context, name)
				break
			}
		}
	}

	content, err := os.ReadFile(dokifilePath)
	if err != nil {
		return fmt.Errorf("read dokifile: %w", err)
	}

	// Parse
	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		return fmt.Errorf("parse dokifile: %w", err)
	}

	stages := parser.GetStages()
	if len(stages) == 0 {
		return fmt.Errorf("no FROM instruction found")
	}

	// If target is specified, only build up to that stage
	if cfg.Target != "" {
		targetIdx := -1
		for i, s := range stages {
			if s.Name == cfg.Target && i == len(stages)-1 {
				targetIdx = i
				break
			}
		}
		if targetIdx < 0 {
			found := false
			for i, s := range stages {
				if s.Name == cfg.Target {
					stages = stages[:i+1]
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("target stage %q not found", cfg.Target)
			}
		}
	}

	// Build each stage
	var finalWorkDir string
	for _, stage := range stages {
		workDir, err := os.MkdirTemp("", "doki-build-")
		if err != nil {
			return fmt.Errorf("create build temp dir: %w", err)
		}

		if stage.FromStage != "" {
			// Multi-stage: copy from referenced stage dir
			if srcDir, ok := b.stageDirs[stage.FromStage]; ok {
				if err := common.CopyDir(srcDir, workDir); err != nil {
					os.RemoveAll(workDir)
					return fmt.Errorf("copy from stage %s: %w", stage.FromStage, err)
				}
			}
		} else if stage.From != "" {
			// Pull base image if needed
			if !b.store.Exists(stage.From) && cfg.Pull {
				if _, err := b.store.Pull(stage.From); err != nil {
					os.RemoveAll(workDir)
					return fmt.Errorf("pull base image %s: %w", stage.From, err)
				}
			}
			// Extract base image layers into workDir
			if err := b.extractBaseImageToDir(stage.From, workDir); err != nil {
				os.RemoveAll(workDir)
				return fmt.Errorf("extract base image %s: %w", stage.From, err)
			}
		}

		// Initialize per-stage image config
		stage.ImageConfig = &image.ImageConfig{}

		// Execute stage instructions
		if err := b.ExecuteStage(stage, contextDir, workDir); err != nil {
			return fmt.Errorf("stage %s: %w", stage.From, err)
		}

		// Save stage dir for --from references
		if stage.Name != "" {
			b.stageDirs[stage.Name] = workDir
		}

		finalWorkDir = workDir

		// Clean up intermediate stages (not the last one)
		if len(stages) > 1 && stage != stages[len(stages)-1] {
			defer os.RemoveAll(workDir)
		}
	}

	// Save the final image
	finalStage := stages[len(stages)-1]
	return b.commitImage(finalStage, finalWorkDir, cfg.Tags)
}

func (b *Builder) extractBaseImageToDir(imageRef, destDir string) error {
	record, err := b.store.Get(imageRef)
	if err != nil {
		// Image not found locally, start with empty dir
		return nil
	}

	for _, layerDigest := range record.Layers {
		layerPath := b.store.GetLayerPath(layerDigest)
		if !common.PathExists(layerPath) {
			continue
		}
		f, err := os.Open(layerPath)
		if err != nil {
			return fmt.Errorf("open layer %s: %w", layerDigest, err)
		}
		if err := ExtractTar(f, destDir); err != nil {
			f.Close()
			return fmt.Errorf("extract layer %s: %w", layerDigest, err)
		}
		f.Close()
	}

	return nil
}

func (b *Builder) commitImage(stage *Stage, workDir string, tags []string) error {
	// Create layer tar
	digest, size, err := b.saveLayer(workDir, "built by Doki")
	if err != nil {
		return fmt.Errorf("save final layer: %w", err)
	}

	config := &image.Config{
		Created:      time.Now().UTC().Format(time.RFC3339),
		Architecture: getArch(),
		OS:           "linux",
		Config:       *stage.ImageConfig,
		RootFS: image.RootFS{
			Type:    "layers",
			DiffIDs: append(stage.Layers, digest),
		},
	}

	if len(tags) == 0 {
		tags = []string{"doki:latest"}
	}

	configData, _ := json.Marshal(config)
	configDigest := "sha256:" + hex.EncodeToString(sha256Hash(configData))

	record := &image.ImageRecord{
		ID:           configDigest,
		RepoTags:     tags,
		Config:       config,
		Size:         size,
		Created:      common.NowTimestamp(),
		Architecture: getArch(),
		OS:           "linux",
		Layers:       append(stage.Layers, digest),
	}

	return b.store.SaveRecord(record)
}

func sha256Hash(data []byte) []byte {
	h := sha256.New()
	h.Write(data)
	return h.Sum(nil)
}

func getArch() string {
	if arch, err := exec.Command("uname", "-m").Output(); err == nil {
		return strings.TrimSpace(string(arch))
	}
	return "arm64"
}

func parseArgs(s string) []string {
	if s == "" {
		return nil
	}

	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return parseJSONArray(s)
	}

	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(s); i++ {
		c := s[i]

		if !inQuote && (c == '"' || c == '\'') {
			inQuote = true
			quoteChar = c
			continue
		}

		if inQuote && c == quoteChar {
			inQuote = false
			continue
		}

		if !inQuote && (c == ' ' || c == '\t') {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

func parseJSONArray(s string) []string {
	s = s[1 : len(s)-1]
	var args []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(s); i++ {
		c := s[i]

		if c == '"' {
			inQuote = !inQuote
			if !inQuote {
				args = append(args, strings.TrimSpace(current.String()))
				current.Reset()
			}
			continue
		}

		if !inQuote {
			if c == ',' {
				if current.Len() > 0 {
					args = append(args, strings.TrimSpace(current.String()))
					current.Reset()
				}
				continue
			}
			if c == ' ' || c == '\t' {
				continue
			}
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		args = append(args, strings.TrimSpace(current.String()))
	}

	return args
}

// Validate checks if a Dokifile is valid.
func Validate(content []byte) error {
	parser := NewDokifileParser()
	return parser.Parse(content)
}

// ListSupportedInstructions returns all supported Dokifile instructions.
func ListSupportedInstructions() []string {
	return []string{
		"FROM", "RUN", "CMD", "LABEL", "EXPOSE", "ENV",
		"ADD", "COPY", "ENTRYPOINT", "VOLUME", "USER",
		"WORKDIR", "ARG", "ONBUILD", "STOPSIGNAL",
		"HEALTHCHECK", "SHELL", "MAINTAINER",
	}
}

// executeInstruction is the old dispatch from Build(); preserved for backwards compat
// but now Build() uses ExecuteStage instead.
func (b *Builder) executeInstruction(stage *Stage, inst *Instruction) error {
	switch inst.Type {
	case "RUN":
		return b.executeRun(stage, inst, "/", "/")
	case "COPY":
		return b.executeCopy(stage, inst, ".", "/")
	case "ADD":
		return b.executeAdd(stage, inst, ".", "/")
	case "ENV":
		return b.executeEnv(stage, inst)
	case "WORKDIR":
		var dir string
		if len(inst.Args) > 0 {
			dir = inst.Args[0]
		}
		return b.executeWorkdir(stage, inst, &dir)
	case "USER":
		return b.executeUser(stage, inst)
	case "EXPOSE":
		return b.executeExpose(stage, inst)
	case "LABEL":
		return b.executeLabel(stage, inst)
	case "CMD":
		return b.executeCmd(stage, inst)
	case "ENTRYPOINT":
		return b.executeEntrypoint(stage, inst)
	case "VOLUME":
		return b.executeVolume(stage, inst)
	case "HEALTHCHECK":
		return b.executeHealthcheck(stage, inst)
	case "STOPSIGNAL":
		return b.executeStopsignal(stage, inst)
	case "SHELL":
		return b.executeShell(stage, inst)
	case "ARG":
		return b.executeArg(stage, inst)
	case "ONBUILD":
		return b.executeOnbuild(stage, inst)
	case "MAINTAINER":
		return b.executeMaintainer(stage, inst)
	default:
		return nil
	}
}
