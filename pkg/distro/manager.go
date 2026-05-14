package distro

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	if record, ok := img.(*image.ImageRecord); ok {
		layers, _ := m.imageStore.GetLayerPaths(record.ID)
		if len(layers) == 0 {
			return fmt.Errorf("no layers found for image %s", record.ID)
		}

		sem := make(chan struct{}, 3)
		errCh := make(chan error, len(layers))
		var wg sync.WaitGroup
		extracted := make([]string, 0, len(layers))
		var mu sync.Mutex

		cleanTarget := filepath.Clean(target)

		for i, layerPath := range layers {
			if !common.PathExists(layerPath) {
				continue
			}
			wg.Add(1)
			go func(idx int, lp string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				if err := extractLayerNative(lp, cleanTarget); err != nil {
					errCh <- fmt.Errorf("layer %d (%s): %w", idx, filepath.Base(lp), err)
					return
				}
				mu.Lock()
				extracted = append(extracted, lp)
				mu.Unlock()
			}(i, layerPath)
		}
		wg.Wait()
		close(errCh)

		var firstErr error
		for err := range errCh {
			if firstErr == nil {
				firstErr = err
			}
		}
		if firstErr != nil {
			os.RemoveAll(target)
			return firstErr
		}
		return nil
	}
	return fmt.Errorf("invalid image type for extraction")
}

func extractLayerNative(tarPath, dest string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	magic := make([]byte, 4)
	n, _ := f.Read(magic)
	f.Seek(0, io.SeekStart)

	var decompressed io.Reader
	switch {
	case n >= 2 && magic[0] == 0x1f && magic[1] == 0x8b:
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		decompressed = gz
	case n >= 2 && magic[0] == 0x42 && magic[1] == 0x5a:
		decompressed = bzip2.NewReader(f)
	case n >= 4 && magic[0] == 0xfd && magic[1] == 0x37 && magic[2] == 0x7a && magic[3] == 0x58:
		xzCmd := exec.Command("xz", "-dc")
		xzCmd.Stdin = f
		var xzOut bytes.Buffer
		xzCmd.Stdout = &xzOut
		if err := xzCmd.Run(); err != nil {
			return fmt.Errorf("xz decompression failed: %w", err)
		}
		decompressed = &xzOut
	case n >= 4 && magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd:
		zstdCmd := exec.Command("zstd", "-dc")
		zstdCmd.Stdin = f
		var zstdOut bytes.Buffer
		zstdCmd.Stdout = &zstdOut
		if err := zstdCmd.Run(); err != nil {
			return fmt.Errorf("zstd decompression failed: %w", err)
		}
		decompressed = &zstdOut
	default:
		decompressed = f
	}

	tr := tar.NewReader(decompressed)
	cleanDest := filepath.Clean(dest)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Clean(filepath.Join(dest, hdr.Name))
		if hdr.Name == "." || hdr.Name == "./" || target == cleanDest {
			continue
		}
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) && target != cleanDest {
			return fmt.Errorf("tar: path traversal attempt: %s -> %s", hdr.Name, target)
		}

		baseName := filepath.Base(hdr.Name)
		if strings.HasPrefix(baseName, ".wh.") {
			if baseName == ".wh..wh..opq" {
				opqDir := filepath.Clean(filepath.Join(dest, filepath.Dir(hdr.Name)))
				if strings.HasPrefix(opqDir, cleanDest+string(os.PathSeparator)) || opqDir == cleanDest {
					entries, _ := os.ReadDir(opqDir)
					for _, e := range entries {
						os.RemoveAll(filepath.Join(opqDir, e.Name()))
					}
				}
				continue
			}
			whTarget := filepath.Clean(filepath.Join(dest, filepath.Dir(hdr.Name), baseName[4:]))
			if strings.HasPrefix(whTarget, cleanDest+string(os.PathSeparator)) || whTarget == cleanDest {
				os.RemoveAll(whTarget)
			}
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeReg, tar.TypeRegA:
			os.MkdirAll(filepath.Dir(target), 0755)
			if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
				os.Remove(target)
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			os.Chmod(target, os.FileMode(hdr.Mode))
			os.Chtimes(target, hdr.ModTime, hdr.ModTime)
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0755)
			linkTarget := hdr.Linkname
			if !filepath.IsAbs(linkTarget) {
				resolved := filepath.Clean(filepath.Join(filepath.Dir(target), linkTarget))
				if !strings.HasPrefix(resolved, cleanDest+string(os.PathSeparator)) && resolved != cleanDest {
					return fmt.Errorf("tar: symlink escape: %s -> %s", hdr.Linkname, resolved)
				}
			}
			os.Remove(target)
			os.Symlink(hdr.Linkname, target)
		case tar.TypeLink:
			os.MkdirAll(filepath.Dir(target), 0755)
			linkTarget := filepath.Clean(filepath.Join(dest, hdr.Linkname))
			if !strings.HasPrefix(linkTarget, cleanDest+string(os.PathSeparator)) && linkTarget != cleanDest {
				return fmt.Errorf("tar: hardlink escape")
			}
			os.Remove(target)
			if err := os.Link(linkTarget, target); err != nil {
				data, readErr := os.ReadFile(linkTarget)
				if readErr != nil {
					return fmt.Errorf("tar: hardlink %s: %w", hdr.Name, err)
				}
				os.WriteFile(target, data, 0644)
			}
		case tar.TypeBlock, tar.TypeChar:
			os.MkdirAll(filepath.Dir(target), 0755)
			os.Remove(target)
			dev := int(hdr.Devmajor)<<8 | int(hdr.Devminor)
			mode := syscall.S_IFBLK
			if hdr.Typeflag == tar.TypeChar {
				mode = syscall.S_IFCHR
			}
			syscall.Mknod(target, uint32(mode)|uint32(hdr.Mode&0777), dev)
		case tar.TypeFifo:
			os.MkdirAll(filepath.Dir(target), 0755)
			os.Remove(target)
			syscall.Mknod(target, syscall.S_IFIFO|uint32(hdr.Mode&0777), 0)
		}
	}
	return nil
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
