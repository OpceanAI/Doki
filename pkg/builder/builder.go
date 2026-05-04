package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/registry"
)

// DokifileParser parses a Dokifile (Dockerfile-compatible).
type DokifileParser struct {
	escape    byte
	parsed    bool
	directives map[string]string
	stages    []*Stage
}

// Stage represents a build stage in a Dokifile.
type Stage struct {
	Name         string
	From         string
	FromStage    string
	Platform     string
	Instructions []Instruction
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
}

// Builder builds OCI images from Dokifiles.
type Builder struct {
	store    *image.Store
	registry *registry.Client
}

// NewBuilder creates a new image builder.
func NewBuilder(store *image.Store) *Builder {
	return &Builder{
		store:    store,
		registry: registry.NewClient(false),
	}
}

// NewDokifileParser creates a new Dokifile parser.
func NewDokifileParser() *DokifileParser {
	return &DokifileParser{
		escape:    '\\',
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

	for i, line := range lines {
		line = strings.TrimSpace(line)
		origLine := line

		// Skip empty lines.
		if line == "" {
			continue
		}

		// Handle parser directives.
		if strings.HasPrefix(line, "#") {
			if !p.parsed {
				line = strings.TrimPrefix(line, "#")
				line = strings.TrimSpace(line)
				if idx := strings.Index(line, "="); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					val := strings.TrimSpace(line[idx+1:])
					p.directives[key] = val
				}
			}
			continue
		}

		// Mark as parsed after first non-comment, non-directive line.
		if !p.parsed {
			p.parsed = true
		}

		// Handle heredoc continuation.
		if inHeredoc {
			if strings.TrimSpace(origLine) == heredocEnd {
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

		// Parse instruction.
		instruction, err := parseLine(line)
		if err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		if instruction == nil {
			continue
		}

		instruction.LineNum = i + 1

		// Check for heredoc start.
		if instruction.Type == "RUN" || instruction.Type == "COPY" {
			if strings.Contains(origLine, "<<EOF") || strings.Contains(origLine, "<<-EOF") {
				inHeredoc = true
				heredocEnd = "EOF"
			}
		}

		if instruction.Type == "FROM" {
			currentStage = p.parseFromInstruction(instruction)
			p.stages = append(p.stages, currentStage)
		} else if currentStage != nil {
			currentStage.Instructions = append(currentStage.Instructions, *instruction)
		}
	}

	return nil
}

func (p *DokifileParser) parseFromInstruction(inst *Instruction) *Stage {
	stage := &Stage{
		Instructions: make([]Instruction, 0),
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

	// Split instruction and args.
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

	// Parse arguments.
	args := parseArgs(argsStr)

	return &Instruction{
		Type: instType,
		Args: args,
		Raw:  line,
	}, nil
}

func parseArgs(s string) []string {
	if s == "" {
		return nil
	}

	// Handle JSON arrays.
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return parseJSONArray(s)
	}

	// Split by whitespace, respecting quotes.
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
			continue
		}

		if !inQuote && c == ',' {
			args = append(args, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		args = append(args, strings.TrimSpace(current.String()))
	}

	return args
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
	// Read Dokifile.
	dokifilePath := filepath.Join(cfg.Context, cfg.Dokifile)
	if cfg.Dokifile == "" {
		// Try multiple names.
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

	// Parse.
	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		return fmt.Errorf("parse dokifile: %w", err)
	}

	stages := parser.GetStages()
	if len(stages) == 0 {
		return fmt.Errorf("no FROM instruction found")
	}

	// Build each stage.
	for _, stage := range stages {
		// Pull base image if needed.
		if !b.store.Exists(stage.From) && cfg.Pull {
			if _, err := b.store.Pull(stage.From); err != nil {
				return fmt.Errorf("pull base image %s: %w", stage.From, err)
			}
		}

		// Execute instructions.
		for _, inst := range stage.Instructions {
			if err := b.executeInstruction(stage, &inst); err != nil {
				return fmt.Errorf("line %d: %w", inst.LineNum, err)
			}
		}
	}

	return nil
}

func (b *Builder) executeInstruction(stage *Stage, inst *Instruction) error {
	switch inst.Type {
	case "RUN":
		return b.executeRun(stage, inst, "")
	case "COPY":
		return b.executeCopy(stage, inst, "", "")
	case "ADD":
		return b.executeAdd(stage, inst, "", "")
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
