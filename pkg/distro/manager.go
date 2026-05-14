package distro

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
)

//go:embed defs/*.yml
var distroFS embed.FS

type DistroManager struct {
	homeDir    string
	distroDir  string
	imageStore *image.Store
	defs       map[string]*DistroDefinition
}

func NewDistroManager(homeDir string, imageStore *image.Store) (*DistroManager, error) {
	m := &DistroManager{
		homeDir:    homeDir,
		distroDir:  filepath.Join(homeDir, "distros"),
		imageStore: imageStore,
		defs:       make(map[string]*DistroDefinition),
	}

	if err := m.loadDefinitions(); err != nil {
		return nil, fmt.Errorf("load distro definitions: %w", err)
	}

	return m, nil
}

func (m *DistroManager) loadDefinitions() error {
	entries, err := distroFS.ReadDir("defs")
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		data, err := distroFS.ReadFile(filepath.Join("defs", entry.Name()))
		if err != nil {
			continue
		}

		var def DistroDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			continue
		}

		m.defs[def.Name] = &def
		for _, alias := range def.Aliases {
			m.defs[alias] = &def
		}
	}

	return nil
}

func (m *DistroManager) Resolve(name string) (*DistroDefinition, error) {
	parts := strings.SplitN(name, ":", 2)
	distroName := parts[0]

	def, ok := m.defs[distroName]
	if !ok {
		return nil, fmt.Errorf("unknown distro: %s (available: alpine, ubuntu, debian, arch)", name)
	}

	if len(parts) > 1 {
		copy := *def
		copy.Source.Tag = parts[1]
		return &copy, nil
	}

	return def, nil
}

func (m *DistroManager) IsInstalled(name string) bool {
	metaPath := filepath.Join(m.distroDir, name, "metadata.json")
	_, err := os.Stat(metaPath)
	return err == nil
}

func (m *DistroManager) EnsureInstalled(def *DistroDefinition) error {
	if m.IsInstalled(def.Name) {
		return nil
	}

	imageRef := fmt.Sprintf("%s:%s", def.Source.Image, def.Source.Tag)
	if def.Source.Registry != "" && def.Source.Registry != "docker.io" {
		imageRef = fmt.Sprintf("%s/%s", def.Source.Registry, imageRef)
	}

	img, err := m.imageStore.Pull(imageRef)
	if err != nil {
		return fmt.Errorf("pull %s: %w", imageRef, err)
	}

	rootfsPath := filepath.Join(m.distroDir, def.Name, "rootfs")
	if err := m.extractRootfs(img, rootfsPath); err != nil {
		return fmt.Errorf("extract rootfs: %w", err)
	}

	meta := InstalledDistro{
		Name:        def.Name,
		Version:     def.Source.Tag,
		InstalledAt: time.Now(),
		RootfsPath:  rootfsPath,
		Source:      imageRef,
	}

	return m.saveMetadata(def.Name, &meta)
}

func (m *DistroManager) GetRootfsPath(name string) string {
	return filepath.Join(m.distroDir, name, "rootfs")
}

func (m *DistroManager) extractRootfs(img interface{}, target string) error {
	common.EnsureDir(target)
	// Extract OCI image layers to target directory
	if record, ok := img.(*image.ImageRecord); ok {
		layers, _ := m.imageStore.GetLayerPaths(record.ID)
		for _, layerPath := range layers {
			if err := extractLayer(layerPath, target); err != nil {
				return fmt.Errorf("extract layer %s: %w", filepath.Base(layerPath), err)
			}
		}
		return nil
	}
	// Fallback: minimal directory structure
	dirs := []string{"bin", "sbin", "dev", "proc", "sys", "tmp", "etc", "var", "run", "usr/bin", "usr/lib"}
	for _, dir := range dirs {
		common.EnsureDir(filepath.Join(target, dir))
	}
	return nil
}

func extractLayer(tarPath, dest string) error {
	cmd := exec.Command("tar", "-xf", tarPath, "-C", dest, "--no-same-owner")
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *DistroManager) saveMetadata(name string, meta *InstalledDistro) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}

	metaPath := filepath.Join(m.distroDir, name, "metadata.json")
	os.MkdirAll(filepath.Dir(metaPath), 0755)
	return os.WriteFile(metaPath, data, 0644)
}

func (m *DistroManager) List() []string {
	var names []string
	entries, err := os.ReadDir(m.distroDir)
	if err != nil {
		return names
	}
	for _, e := range entries {
		if e.IsDir() && m.IsInstalled(e.Name()) {
			names = append(names, e.Name())
		}
	}
	return names
}

// Remove deletes an installed distro.
func (m *DistroManager) Remove(name string) error {
	distroPath := filepath.Join(m.distroDir, name)
	return os.RemoveAll(distroPath)
}

// Update re-pulls and re-extracts a distro.
func (m *DistroManager) Update(def *DistroDefinition) error {
	m.Remove(def.Name)
	return m.EnsureInstalled(def)
}

// Search looks for available pre-defined distros by name.
func (m *DistroManager) Search(query string) []string {
	var results []string
	for _, def := range m.defs {
		if strings.Contains(def.Name, query) || strings.Contains(def.Description, query) {
			results = append(results, def.Name)
		}
	}
	return results
}

// RegisterDef adds a custom distro definition.
func (m *DistroManager) RegisterDef(def *DistroDefinition) {
	m.defs[def.Name] = def
	for _, alias := range def.Aliases {
		m.defs[alias] = def
	}
}
