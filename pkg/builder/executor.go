package builder

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
)

// ExecuteStage runs all instructions in a build stage.
func (b *Builder) ExecuteStage(stage *Stage, contextDir, rootDir string) error {
	workDir := rootDir

	for _, inst := range stage.Instructions {
		// Substitute variables before execution (includes BuildConfig.BuildArgs)
		inst.Args = substituteVarsInSlice(inst.Args, b.envMap, b.argDefaults, b.argDefaults)
		inst.Raw = substituteVars(inst.Raw, b.envMap, b.argDefaults, b.argDefaults)

		if err := b.executeInstructionReal(stage, &inst, contextDir, rootDir, &workDir); err != nil {
			return fmt.Errorf("line %d (%s): %w", inst.LineNum, inst.Type, err)
		}
	}

	return nil
}

func (b *Builder) executeInstructionReal(stage *Stage, inst *Instruction, ctxDir, rootDir string, workDir *string) error {
	switch inst.Type {
	case "RUN":
		return b.executeRun(stage, inst, rootDir, *workDir)
	case "COPY":
		return b.executeCopy(stage, inst, ctxDir, rootDir)
	case "ADD":
		return b.executeAdd(stage, inst, ctxDir, rootDir)
	case "ENV":
		return b.executeEnv(stage, inst)
	case "WORKDIR":
		return b.executeWorkdir(stage, inst, workDir)
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
	}
	return nil
}

func (b *Builder) executeRun(stage *Stage, inst *Instruction, rootDir, workDir string) error {
	if len(inst.Args) == 0 {
		return nil
	}

	cmdStr := strings.Join(inst.Args, " ")
	if cmdStr == "" {
		return nil
	}

	// Check build cache
	if cachePath, hit := b.checkCache(inst, b.envMap); hit {
		f, err := os.Open(cachePath)
		if err == nil {
			defer f.Close()
			return ExtractTar(f, rootDir)
		}
	}

	// Mount build secrets
	var secretFiles []string
	for name, srcPath := range b.secrets {
		destPath := filepath.Join(rootDir, "run", "secrets", name)
		common.EnsureDir(filepath.Dir(destPath))
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read secret %s: %w", name, err)
		}
		if err := os.WriteFile(destPath, data, 0400); err != nil {
			return fmt.Errorf("mount secret %s: %w", name, err)
		}
		secretFiles = append(secretFiles, destPath)
	}
	defer func() {
		for _, f := range secretFiles {
			os.RemoveAll(f)
		}
	}()

	// Build environment for the command
	env := os.Environ()
	for k, v := range b.envMap {
		env = append(env, k+"="+v)
	}

	cmd := exec.Command("/bin/sh", "-c", cmdStr)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	// Create layer tar from the entire root directory
	digest, _, err := b.saveLayer(rootDir, inst.Raw)
	if err != nil {
		return fmt.Errorf("save run layer: %w", err)
	}
	stage.Layers = append(stage.Layers, digest)

	// Save to build cache
	var buf bytes.Buffer
	if err := CreateTar(rootDir, &buf); err == nil {
		b.saveCache(inst, b.envMap, buf.Bytes())
	}

	return nil
}

func (b *Builder) executeCopy(stage *Stage, inst *Instruction, ctxDir, rootDir string) error {
	args := inst.Args
	var src, dst, fromStage, chown, chmod string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--from" && i+1 < len(args):
			i++
			fromStage = args[i]
		case strings.HasPrefix(a, "--from="):
			fromStage = strings.TrimPrefix(a, "--from=")
		case a == "--chown" && i+1 < len(args):
			i++
			chown = args[i]
		case strings.HasPrefix(a, "--chown="):
			chown = strings.TrimPrefix(a, "--chown=")
		case a == "--chmod" && i+1 < len(args):
			i++
			chmod = args[i]
		case strings.HasPrefix(a, "--chmod="):
			chmod = strings.TrimPrefix(a, "--chmod=")
		case !strings.HasPrefix(a, "-"):
			if src == "" {
				src = a
			} else {
				dst = a
			}
		}
	}

	if src == "" || dst == "" {
		return nil
	}

	// Determine source directory
	var sourceDir string
	if fromStage != "" {
		if dir, ok := b.stageDirs[fromStage]; ok {
			sourceDir = dir
		} else {
			return fmt.Errorf("stage %q not found for --from", fromStage)
		}
	} else {
		sourceDir = ctxDir
	}

	srcPath := filepath.Join(sourceDir, src)

	// Apply .dockerignore filtering for context copies
	if fromStage == "" && b.dockerignore != nil {
		if b.dockerignore.Matches(src) {
			return nil
		}
	}

	// Determine destination path
	var dstPath string
	if filepath.IsAbs(dst) {
		dstPath = filepath.Join(rootDir, dst)
	} else {
		dstPath = filepath.Join(rootDir, dst)
	}

	// Handle glob patterns in source
	matches, err := filepath.Glob(srcPath)
	if err != nil || len(matches) == 0 {
		// Try direct path
		matches = []string{srcPath}
	}

	for _, match := range matches {
		fi, err := os.Stat(match)
		if err != nil {
			continue
		}

		if fi.IsDir() {
			// If destination doesn't exist and source is a dir, copy contents
			if common.PathExists(dstPath) {
				// Copy dir into existing dir
				baseName := filepath.Base(match)
				target := filepath.Join(dstPath, baseName)
				if err := common.CopyDir(match, target); err != nil {
					return fmt.Errorf("copy dir %s: %w", src, err)
				}
				if chown != "" || chmod != "" {
					applyChowChmodRecursive(target, chown, chmod)
				}
			} else {
				if err := common.CopyDir(match, dstPath); err != nil {
					return fmt.Errorf("copy dir %s: %w", src, err)
				}
				if chown != "" || chmod != "" {
					applyChowChmodRecursive(dstPath, chown, chmod)
				}
			}
		} else {
			common.EnsureDir(filepath.Dir(dstPath))
			data, err := os.ReadFile(match)
			if err != nil {
				return fmt.Errorf("copy %s: %w", src, err)
			}
			if err := os.WriteFile(dstPath, data, fi.Mode()); err != nil {
				return fmt.Errorf("write %s: %w", dst, err)
			}
			if chown != "" || chmod != "" {
				applyChowChmod(dstPath, chown, chmod)
			}
		}
	}

	// Save layer
	digest, _, err := b.saveLayer(rootDir, inst.Raw)
	if err != nil {
		return fmt.Errorf("save copy layer: %w", err)
	}
	stage.Layers = append(stage.Layers, digest)

	return nil
}

func (b *Builder) executeAdd(stage *Stage, inst *Instruction, ctxDir, rootDir string) error {
	args := inst.Args
	var src, dst, chown, chmod string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--chown" && i+1 < len(args):
			i++
			chown = args[i]
		case strings.HasPrefix(a, "--chown="):
			chown = strings.TrimPrefix(a, "--chown=")
		case a == "--chmod" && i+1 < len(args):
			i++
			chmod = args[i]
		case strings.HasPrefix(a, "--chmod="):
			chmod = strings.TrimPrefix(a, "--chmod=")
		case !strings.HasPrefix(a, "-"):
			if src == "" {
				src = a
			} else {
				dst = a
			}
		}
	}

	if src == "" || dst == "" {
		return nil
	}

	// Determine destination path
	var dstPath string
	if filepath.IsAbs(dst) {
		dstPath = filepath.Join(rootDir, dst)
	} else {
		dstPath = filepath.Join(rootDir, dst)
	}
	common.EnsureDir(filepath.Dir(dstPath))

	// If source is a URL, download it
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		if err := b.downloadAdd(src, dstPath); err != nil {
			return fmt.Errorf("add URL %s: %w", src, err)
		}
		// Auto-extract tar archives
		if isArchive(dstPath) {
			if err := extractArchive(dstPath, filepath.Dir(dstPath)); err != nil {
				return fmt.Errorf("extract archive %s: %w", dstPath, err)
			}
			os.Remove(dstPath)
		}
	} else {
		// Local file - handle glob
		srcPath := filepath.Join(ctxDir, src)
		matches, err := filepath.Glob(srcPath)
		if err != nil || len(matches) == 0 {
			matches = []string{srcPath}
		}

		for _, match := range matches {
			fi, err := os.Stat(match)
			if err != nil {
				continue
			}
			if fi.IsDir() {
				if err := common.CopyDir(match, dstPath); err != nil {
					return fmt.Errorf("add dir %s: %w", src, err)
				}
			} else {
				data, err := os.ReadFile(match)
				if err != nil {
					return fmt.Errorf("add %s: %w", src, err)
				}
				if err := os.WriteFile(dstPath, data, fi.Mode()); err != nil {
					return fmt.Errorf("write %s: %w", dst, err)
				}
				// Auto-extract local tar archives
				if isArchive(dstPath) {
					if err := extractArchive(dstPath, filepath.Dir(dstPath)); err != nil {
						return fmt.Errorf("extract archive %s: %w", dstPath, err)
					}
					os.Remove(dstPath)
				}
			}
		}
	}

	// Apply chown/chmod
	if chown != "" || chmod != "" {
		if fi, err := os.Stat(dstPath); err == nil && fi.IsDir() {
			applyChowChmodRecursive(dstPath, chown, chmod)
		} else {
			applyChowChmod(dstPath, chown, chmod)
		}
	}

	// Save layer
	digest, _, err := b.saveLayer(rootDir, inst.Raw)
	if err != nil {
		return fmt.Errorf("save add layer: %w", err)
	}
	stage.Layers = append(stage.Layers, digest)

	return nil
}

func (b *Builder) downloadAdd(url, destPath string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func isArchive(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(name, ".tar") ||
		strings.HasSuffix(name, ".tar.gz") ||
		strings.HasSuffix(name, ".tgz") ||
		strings.HasSuffix(name, ".tar.bz2") ||
		strings.HasSuffix(name, ".tar.xz") ||
		strings.HasSuffix(name, ".txz")
}

func extractArchive(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	name := strings.ToLower(filepath.Base(archivePath))

	var reader io.Reader = f

	if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".tgz") {
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		reader = gzReader
	} else if strings.HasSuffix(name, ".bz2") || strings.HasSuffix(name, ".tar.bz2") {
		reader = bzip2.NewReader(f)
	} else if strings.HasSuffix(name, ".xz") || strings.HasSuffix(name, ".txz") || strings.HasSuffix(name, ".tar.xz") {
		cmd := exec.Command("xz", "-dc")
		cmd.Stdin = f
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("xz decompress: %w", err)
		}
		reader = &out
	} else if strings.HasSuffix(name, ".zst") || strings.HasSuffix(name, ".tar.zst") {
		cmd := exec.Command("zstd", "-dc")
		cmd.Stdin = f
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("zstd decompress: %w", err)
		}
		reader = &out
	}

	return ExtractTar(reader, destDir)
}

func (b *Builder) executeEnv(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	for _, arg := range inst.Args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			b.envMap[parts[0]] = parts[1]
			// Update image config
			envEntry := parts[0] + "=" + parts[1]
			found := false
			for j, existing := range stage.ImageConfig.Env {
				if strings.HasPrefix(existing, parts[0]+"=") {
					stage.ImageConfig.Env[j] = envEntry
					found = true
					break
				}
			}
			if !found {
				stage.ImageConfig.Env = append(stage.ImageConfig.Env, envEntry)
			}
		}
	}
	return nil
}

func (b *Builder) executeWorkdir(stage *Stage, inst *Instruction, workDir *string) error {
	if len(inst.Args) > 0 {
		dir := inst.Args[0]
		if filepath.IsAbs(dir) {
			*workDir = dir
		} else {
			*workDir = filepath.Join(*workDir, dir)
		}
		if stage.ImageConfig != nil {
			stage.ImageConfig.WorkingDir = *workDir
		}
		common.EnsureDir(*workDir)
	}
	return nil
}

func (b *Builder) executeUser(stage *Stage, inst *Instruction) error {
	if len(inst.Args) > 0 {
		if stage.ImageConfig == nil {
			stage.ImageConfig = &image.ImageConfig{}
		}
		stage.ImageConfig.User = inst.Args[0]
	}
	return nil
}

func (b *Builder) executeExpose(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	if stage.ImageConfig.ExposedPorts == nil {
		stage.ImageConfig.ExposedPorts = make(map[string]struct{})
	}
	for _, arg := range inst.Args {
		port := arg
		if !strings.Contains(port, "/") {
			port += "/tcp"
		}
		stage.ImageConfig.ExposedPorts[port] = struct{}{}
	}
	return nil
}

func (b *Builder) executeLabel(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	if stage.ImageConfig.Labels == nil {
		stage.ImageConfig.Labels = make(map[string]string)
	}
	for _, arg := range inst.Args {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			stage.ImageConfig.Labels[kv[0]] = kv[1]
		}
	}
	return nil
}

func (b *Builder) executeCmd(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	stage.ImageConfig.Cmd = inst.Args
	return nil
}

func (b *Builder) executeEntrypoint(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	stage.ImageConfig.Entrypoint = inst.Args
	return nil
}

func (b *Builder) executeVolume(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	if stage.ImageConfig.Volumes == nil {
		stage.ImageConfig.Volumes = make(map[string]struct{})
	}
	for _, arg := range inst.Args {
		stage.ImageConfig.Volumes[arg] = struct{}{}
	}
	return nil
}

func (b *Builder) executeHealthcheck(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	if len(inst.Args) == 0 {
		return nil
	}

	// HEALTHCHECK NONE
	if strings.ToUpper(inst.Args[0]) == "NONE" {
		stage.ImageConfig.HealthCheck = nil
		return nil
	}

	// Parse options and command
	var test []string
	var interval, timeout, startPeriod int64
	var retries int

	for i := 0; i < len(inst.Args); i++ {
		a := inst.Args[i]
		switch {
		case strings.HasPrefix(a, "--interval="):
			interval = parseDurationNanos(strings.TrimPrefix(a, "--interval="))
		case strings.HasPrefix(a, "--timeout="):
			timeout = parseDurationNanos(strings.TrimPrefix(a, "--timeout="))
		case strings.HasPrefix(a, "--start-period="):
			startPeriod = parseDurationNanos(strings.TrimPrefix(a, "--start-period="))
		case strings.HasPrefix(a, "--retries="):
			fmt.Sscanf(strings.TrimPrefix(a, "--retries="), "%d", &retries)
		case a == "CMD":
			// CMD followed by the test command
			test = inst.Args[i+1:]
			i = len(inst.Args)
		case a == "NONE":
			stage.ImageConfig.HealthCheck = nil
			return nil
		default:
			if test == nil {
				test = append(test, a)
			}
		}
	}

	if test != nil {
		stage.ImageConfig.HealthCheck = &image.HealthCheckConfig{
			Test:        test,
			Interval:    interval,
			Timeout:     timeout,
			Retries:     retries,
			StartPeriod: startPeriod,
		}
	}

	return nil
}

func parseDurationNanos(s string) int64 {
	// Parse simple duration like "30s", "5m", "1h"
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var val int64
	unit := "s"
	for i, c := range s {
		if c >= '0' && c <= '9' {
			val = val*10 + int64(c-'0')
		} else {
			unit = strings.ToLower(s[i:])
			break
		}
	}
	switch unit {
	case "ns":
		return val
	case "us", "µs":
		return val * 1000
	case "ms":
		return val * 1000000
	case "s":
		return val * 1000000000
	case "m":
		return val * 60000000000
	case "h":
		return val * 3600000000000
	}
	return val * 1000000000
}

func (b *Builder) executeStopsignal(stage *Stage, inst *Instruction) error {
	if len(inst.Args) > 0 {
		if stage.ImageConfig == nil {
			stage.ImageConfig = &image.ImageConfig{}
		}
		stage.ImageConfig.StopSignal = inst.Args[0]
	}
	return nil
}

func (b *Builder) executeShell(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	stage.ImageConfig.Shell = inst.Args
	return nil
}

func (b *Builder) executeArg(stage *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			b.argDefaults[kv[0]] = kv[1]
		} else {
			// ARG without default - mark as present
			if _, exists := b.argDefaults[arg]; !exists {
				b.argDefaults[arg] = ""
			}
		}
	}
	return nil
}

func (b *Builder) executeOnbuild(stage *Stage, inst *Instruction) error {
	stage.Metadata["onbuild"] = strings.Join(inst.Args, " ")
	return nil
}

func (b *Builder) executeMaintainer(stage *Stage, inst *Instruction) error {
	if len(inst.Args) > 0 {
		stage.Metadata["Maintainer"] = inst.Args[0]
	}
	return nil
}

// ExtractTar extracts a tar archive to dest directory.
func ExtractTar(r io.Reader, dest string) error {
	cleanDest := filepath.Clean(dest)
	tr := tar.NewReader(r)
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
			return fmt.Errorf("tar: path traversal attempt: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg, tar.TypeRegA:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0755)
			os.Remove(target)
			os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}

// CreateTar creates a tar archive from a directory.
func CreateTar(dir string, writer io.Writer) error {
	tw := tar.NewWriter(writer)
	defer tw.Close()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		io.Copy(tw, f)
		f.Close()
		return nil
	})
}

// CompressGzip compresses data using gzip.
func CompressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(data)
	w.Close()
	return buf.Bytes(), nil
}

// ExecuteBuildContext extracts a build context and sets it up.
func (b *Builder) ExecuteBuildContext(contextDir, tarPath, outputDir string) error {
	if tarPath != "" {
		f, err := os.Open(tarPath)
		if err != nil {
			return fmt.Errorf("open context tar: %w", err)
		}
		defer f.Close()
		if err := ExtractTar(f, contextDir); err != nil {
			return fmt.Errorf("extract context: %w", err)
		}
	}
	return nil
}
