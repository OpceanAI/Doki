package builder

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/proot"
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
		return b.executeWorkdir(stage, inst, rootDir, workDir)
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
		return fmt.Errorf("RUN requires a command")
	}

	cmdStr := strings.Join(inst.Args, " ")
	if cmdStr == "" {
		return fmt.Errorf("RUN requires a command")
	}

	// Check build cache
	if cachePath, hit := b.checkCache(inst, b.envMap); hit {
		f, err := os.Open(cachePath)
		if err == nil {
			defer func() { _ = f.Close() }()
			return ExtractTar(f, rootDir)
		}
	}

	// Mount build secrets
	var secretFiles []string
	for name, srcPath := range b.secrets {
		destPath := filepath.Join(rootDir, "run", "secrets", name)
		if err := common.EnsureDir(filepath.Dir(destPath)); err != nil {
			return fmt.Errorf("ensure dir %s: %w", filepath.Dir(destPath), err)
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read secret %s: %w", name, err)
		}
		if err := os.WriteFile(destPath, data, 0400); err != nil {
			return fmt.Errorf("mount secret %s: %w", name, err)
		}
		secretFiles = append(secretFiles, destPath)
	}
	// CRIT-4: build secrets must NEVER end up in the committed layer or the
	// build cache. cleanupSecrets removes every mounted secret and the
	// /run/secrets directory. It is called explicitly BEFORE saveLayer/saveCache
	// (Docker keeps secrets in a tmpfs that is never committed); the defer is a
	// best-effort safety net for early-return error paths.
	cleanedUp := false
	cleanupSecrets := func() {
		if cleanedUp {
			return
		}
		cleanedUp = true
		for _, f := range secretFiles {
			if err := os.RemoveAll(f); err != nil {
				slog.Warn("remove secret failed", "path", f, "error", err)
			}
		}
		// Remove the now-empty /run/secrets dir so it is not committed either.
		if len(secretFiles) > 0 {
			_ = os.RemoveAll(filepath.Join(rootDir, "run", "secrets"))
		}
	}
	defer cleanupSecrets()

	// Build environment for the command.
	// Clear LD_PRELOAD family in the parent process so exec.Command does not
	// propagate libtermux-exec.so to the proot child. Then strip host env
	// (Termux/Android vars) so the guest does not inherit host paths.
	if prootBin := findProotBinary(); prootBin != "" {
		proot.UnsetProotKillers()
	}
	env := common.StripHostEnv(os.Environ())
	for k, v := range b.envMap {
		env = append(env, k+"="+v)
	}

	// Determine shell to use (SHELL instruction overrides default).
	shell := []string{"/bin/sh", "-c"}
	if len(stage.ImageConfig.Shell) > 0 {
		shell = stage.ImageConfig.Shell
	}

	var cmd *exec.Cmd
	if prootBin := findProotBinary(); prootBin != "" {
		// Replace host PATH with standard Linux paths for the guest rootfs
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
		// Run inside rootfs via proot for proper isolation
		prootArgs := []string{
			"-r", rootDir,
			"-i", "0:0",
			"-b", "/proc",
			"-b", "/sys",
			"-b", "/dev",
			"--kill-on-exit",
			"--link2symlink",
		}
		prootArgs = append(prootArgs, shell...)
		prootArgs = append(prootArgs, cmdStr)
		cmd = exec.Command(prootBin, prootArgs...)
		cmd.Dir = "/"
	} else {
		// CRIT-5: fail closed. Without an isolation backend (proot / user
		// namespaces) a RUN instruction would execute attacker-controlled
		// Dokifile commands directly on the host as the daemon user. Treat RUN
		// as untrusted code: refuse to build rather than run it unsandboxed.
		return fmt.Errorf("RUN requires an isolation backend (proot not found); refusing to execute build commands directly on the host")
	}
	cmd.Env = env
	// Discard stdin to prevent proot from blocking on input
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// Add a timeout to prevent indefinite hangs (10 minutes)
	timer := time.AfterFunc(10*time.Minute, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run command: %w", err)
	}

	// CRIT-4: strip secrets BEFORE the layer/cache are created.
	cleanupSecrets()

	// Create layer tar from the entire root directory
	digest, _, err := b.saveLayer(rootDir, inst.Raw)
	if err != nil {
		return fmt.Errorf("save run layer: %w", err)
	}
	stage.Layers = append(stage.Layers, digest)

	// Save to build cache
	var buf bytes.Buffer
	if err := CreateTar(rootDir, &buf); err == nil {
		if err := b.saveCache(inst, b.envMap, buf.Bytes()); err != nil {
			slog.Warn("save cache failed", "error", err)
		}
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
		} else if idx, err := strconv.Atoi(fromStage); err == nil && idx >= 0 && idx < len(b.stages) {
			// Support numeric stage index (COPY --from=0, COPY --from=1, etc.)
			stageName := b.stages[idx].Name
			if dir, ok := b.stageDirs[stageName]; ok {
				sourceDir = dir
			} else {
				return fmt.Errorf("stage index %d (name %q) has no build directory", idx, stageName)
			}
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
	dstIsDir := strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, string(os.PathSeparator))
	if filepath.IsAbs(dst) {
		dstPath = filepath.Join(rootDir, dst)
	} else {
		dstPath = filepath.Join(rootDir, dst)
	}
	if dstIsDir {
		if err := common.EnsureDir(dstPath); err != nil {
			return fmt.Errorf("ensure dir %s: %w", dstPath, err)
		}
	}

	// Handle glob patterns in source
	matches, err := filepath.Glob(srcPath)
	if err != nil || len(matches) == 0 {
		// Try direct path
		matches = []string{srcPath}
	}

	for _, match := range matches {
		// BUG fix (symlink path traversal): resolve symlinks on the source
		// path and verify the resolved path is still under the source
		// directory. Without this, a symlink like "src -> /etc" in the
		// build context could cause COPY to read files from outside the
		// context (e.g., /etc/passwd).
		resolved, err := filepath.EvalSymlinks(match)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(resolved, filepath.Clean(sourceDir)+string(os.PathSeparator)) &&
			resolved != filepath.Clean(sourceDir) {
			return fmt.Errorf("copy source %s resolves outside build context", src)
		}

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
					if err := applyChowChmodRecursive(target, chown, chmod); err != nil {
						slog.Warn("apply chown/chmod failed", "path", target, "error", err)
					}
				}
			} else {
				if err := common.CopyDir(match, dstPath); err != nil {
					return fmt.Errorf("copy dir %s: %w", src, err)
				}
				if chown != "" || chmod != "" {
					if err := applyChowChmodRecursive(dstPath, chown, chmod); err != nil {
						slog.Warn("apply chown/chmod failed", "path", dstPath, "error", err)
					}
				}
			}
		} else {
			target := dstPath
			// When destination is an existing directory, place the file inside it.
			if fiDest, err := os.Stat(dstPath); err == nil && fiDest.IsDir() {
				target = filepath.Join(dstPath, filepath.Base(match))
			}
			if err := common.EnsureDir(filepath.Dir(target)); err != nil {
				return fmt.Errorf("ensure dir %s: %w", filepath.Dir(target), err)
			}
			data, err := os.ReadFile(match)
			if err != nil {
				return fmt.Errorf("copy %s: %w", src, err)
			}
			if err := os.WriteFile(target, data, fi.Mode()); err != nil {
				return fmt.Errorf("write %s: %w", dst, err)
			}
			if chown != "" || chmod != "" {
				if err := applyChowChmod(target, chown, chmod); err != nil {
					slog.Warn("apply chown/chmod failed", "path", target, "error", err)
				}
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

	// Apply .dockerignore filtering for context copies
	if fromStage == "" && b.dockerignore != nil {
		if b.dockerignore.Matches(src) {
			return nil
		}
	}

	// Determine source directory for --from
	var sourceDir string
	if fromStage != "" {
		if dir, ok := b.stageDirs[fromStage]; ok {
			sourceDir = dir
		} else if idx, err := strconv.Atoi(fromStage); err == nil && idx >= 0 && idx < len(b.stages) {
			stageName := b.stages[idx].Name
			if dir, ok := b.stageDirs[stageName]; ok {
				sourceDir = dir
			} else {
				return fmt.Errorf("stage index %d (name %q) has no build directory", idx, stageName)
			}
		} else {
			return fmt.Errorf("stage %q not found for --from", fromStage)
		}
	} else {
		sourceDir = ctxDir
	}

	// Determine destination path
	var dstPath string
	dstIsDir := strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, string(os.PathSeparator))
	if filepath.IsAbs(dst) {
		dstPath = filepath.Join(rootDir, dst)
	} else {
		dstPath = filepath.Join(rootDir, dst)
	}
	if dstIsDir {
		if err := common.EnsureDir(dstPath); err != nil {
			return fmt.Errorf("ensure dir %s: %w", dstPath, err)
		}
	}
	if err := common.EnsureDir(filepath.Dir(dstPath)); err != nil {
		return fmt.Errorf("ensure dir %s: %w", filepath.Dir(dstPath), err)
	}

	// If source is a URL, download it
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		tmpDir, err := os.MkdirTemp("", "doki-add-")
		if err != nil {
			return fmt.Errorf("create temp dir: %w", err)
		}
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				slog.Warn("remove temp dir failed", "path", tmpDir, "error", err)
			}
		}()

		tmpFile := filepath.Join(tmpDir, filepath.Base(dstPath))
		if err := b.downloadAdd(src, tmpFile); err != nil {
			return fmt.Errorf("add URL %s: %w", src, err)
		}
		// Auto-extract tar archives
		if isArchive(tmpFile) {
			if err := extractArchive(tmpFile, dstPath); err != nil {
				return fmt.Errorf("extract archive %s: %w", tmpFile, err)
			}
		} else {
			data, err := os.ReadFile(tmpFile)
			if err != nil {
				return err
			}
			target := dstPath
			if dstIsDir {
				target = filepath.Join(dstPath, filepath.Base(src))
			}
			if err := os.WriteFile(target, data, 0644); err != nil {
				return err
			}
		}
	} else {
		// Local file - handle glob
		srcPath := filepath.Join(sourceDir, src)
		matches, err := filepath.Glob(srcPath)
		if err != nil || len(matches) == 0 {
			matches = []string{srcPath}
		}

		for _, match := range matches {
			// HIGH-8: like COPY, verify the resolved source stays inside the build
			// context. Without this, `ADD ../../etc/shadow` or a symlink pointing
			// outside the context reads arbitrary host files into the image.
			resolved, rerr := filepath.EvalSymlinks(match)
			if rerr != nil {
				continue
			}
			if !strings.HasPrefix(resolved, filepath.Clean(sourceDir)+string(os.PathSeparator)) &&
				resolved != filepath.Clean(sourceDir) {
				return fmt.Errorf("add source %s resolves outside build context", src)
			}
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
				target := dstPath
				if dstIsDir {
					target = filepath.Join(dstPath, filepath.Base(match))
				}
				if err := os.WriteFile(target, data, fi.Mode()); err != nil {
					return fmt.Errorf("write %s: %w", dst, err)
				}
				// Auto-extract local tar archives
				if isArchive(dstPath) {
					if err := extractArchive(dstPath, filepath.Dir(dstPath)); err != nil {
						return fmt.Errorf("extract archive %s: %w", dstPath, err)
					}
					if err := os.Remove(dstPath); err != nil {
						slog.Warn("remove archive failed", "path", dstPath, "error", err)
					}
				}
			}
		}
	}

	// Apply chown/chmod
	if chown != "" || chmod != "" {
		if fi, err := os.Stat(dstPath); err == nil && fi.IsDir() {
			if err := applyChowChmodRecursive(dstPath, chown, chmod); err != nil {
				return fmt.Errorf("apply chown/chmod: %w", err)
			}
		} else {
			if err := applyChowChmod(dstPath, chown, chmod); err != nil {
				return fmt.Errorf("apply chown/chmod: %w", err)
			}
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
	client := &http.Client{
		Timeout: 120 * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("close response body failed", "error", err)
		}
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close file failed", "path", destPath, "error", err)
		}
	}()

	_, err = io.Copy(f, io.LimitReader(resp.Body, 1<<30))
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
	defer func() { _ = f.Close() }()

	name := strings.ToLower(filepath.Base(archivePath))

	var reader io.Reader = f

	if strings.HasSuffix(name, ".gz") || strings.HasSuffix(name, ".tgz") {
		gzReader, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer func() {
			if err := gzReader.Close(); err != nil {
				slog.Warn("close gzip reader failed", "error", err)
			}
		}()
		reader = gzReader
	} else if strings.HasSuffix(name, ".bz2") || strings.HasSuffix(name, ".tar.bz2") {
		reader = bzip2.NewReader(f)
	} else if strings.HasSuffix(name, ".xz") || strings.HasSuffix(name, ".txz") || strings.HasSuffix(name, ".tar.xz") {
		if _, err := exec.LookPath("xz"); err != nil {
			return fmt.Errorf("xz not found: %w", err)
		}
		cmd := exec.Command("xz", "-dc")
		cmd.Stdin = f
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("xz decompress: %w", err)
		}
		if err := ExtractTar(stdout, destDir); err != nil {
			if waitErr := cmd.Wait(); waitErr != nil {
				slog.Warn("xz command wait failed", "error", waitErr)
			}
			return err
		}
		return cmd.Wait()
	} else if strings.HasSuffix(name, ".zst") || strings.HasSuffix(name, ".tar.zst") {
		if _, err := exec.LookPath("zstd"); err != nil {
			return fmt.Errorf("zstd not found: %w", err)
		}
		cmd := exec.Command("zstd", "-dc")
		cmd.Stdin = f
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return err
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("zstd decompress: %w", err)
		}
		if err := ExtractTar(stdout, destDir); err != nil {
			if waitErr := cmd.Wait(); waitErr != nil {
				slog.Warn("zstd command wait failed", "error", waitErr)
			}
			return err
		}
		return cmd.Wait()
	}

	return ExtractTar(reader, destDir)
}

func (b *Builder) executeEnv(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	args := inst.Args
	i := 0
	for i < len(args) {
		arg := args[i]
		parts := strings.SplitN(arg, "=", 2)
		var key, val string
		if len(parts) == 2 {
			key = parts[0]
			val = parts[1]
		} else {
			key = arg
			if i+1 < len(args) {
				i++
				val = args[i]
			}
		}
		b.envMap[key] = val
		envEntry := key + "=" + val
		found := false
		for j, existing := range stage.ImageConfig.Env {
			if strings.HasPrefix(existing, key+"=") {
				stage.ImageConfig.Env[j] = envEntry
				found = true
				break
			}
		}
		if !found {
			stage.ImageConfig.Env = append(stage.ImageConfig.Env, envEntry)
		}
		i++
	}
	return nil
}

func (b *Builder) executeWorkdir(stage *Stage, inst *Instruction, rootDir string, workDir *string) error {
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
		workDirPath := filepath.Join(rootDir, *workDir)
		if err := common.EnsureDir(workDirPath); err != nil {
			return fmt.Errorf("ensure workdir %s: %w", workDirPath, err)
		}
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

func isShellForm(raw string) bool {
	upper := strings.ToUpper(raw)
	for _, prefix := range []string{"CMD ", "ENTRYPOINT "} {
		if strings.HasPrefix(upper, prefix) {
			rest := strings.TrimSpace(raw[len(prefix):])
			return !strings.HasPrefix(rest, "[")
		}
	}
	return false
}

func (b *Builder) executeCmd(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	if isShellForm(inst.Raw) {
		stage.ImageConfig.Cmd = []string{"/bin/sh", "-c", strings.Join(inst.Args, " ")}
	} else {
		stage.ImageConfig.Cmd = inst.Args
	}
	return nil
}

func (b *Builder) executeEntrypoint(stage *Stage, inst *Instruction) error {
	if stage.ImageConfig == nil {
		stage.ImageConfig = &image.ImageConfig{}
	}
	if isShellForm(inst.Raw) {
		stage.ImageConfig.Entrypoint = []string{"/bin/sh", "-c", strings.Join(inst.Args, " ")}
	} else {
		stage.ImageConfig.Entrypoint = inst.Args
	}
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
			if _, err := fmt.Sscanf(strings.TrimPrefix(a, "--retries="), "%d", &retries); err != nil {
				return fmt.Errorf("invalid --retries value: %s", strings.TrimPrefix(a, "--retries="))
			}
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
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d.Nanoseconds()
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

func (b *Builder) executeArg(_ *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			// Only set default if build arg was not provided externally.
			if _, exists := b.argDefaults[kv[0]]; !exists {
				b.argDefaults[kv[0]] = kv[1]
			}
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
	// Store type|raw for later replay (append with ;; separator)
	// inst.Type is "ONBUILD", the actual instruction is in inst.Args[0]
	if len(inst.Args) == 0 {
		return fmt.Errorf("ONBUILD requires a sub-instruction")
	}
	if stage.Metadata == nil {
		stage.Metadata = make(map[string]string)
	}
	entry := inst.Args[0] + "|" + strings.Join(inst.Args[1:], " ")
	if existing, ok := stage.Metadata["onbuild"]; ok && existing != "" {
		stage.Metadata["onbuild"] = existing + ";;" + entry
	} else {
		stage.Metadata["onbuild"] = entry
	}
	return nil
}

func (b *Builder) executeMaintainer(stage *Stage, inst *Instruction) error {
	if len(inst.Args) > 0 {
		stage.Metadata["Maintainer"] = inst.Args[0]
	}
	return nil
}

// findProotBinary locates the proot binary (doki-proot preferred).
func findProotBinary() string {
	for _, name := range []string{"doki-proot", "proot"} {
		p, err := exec.LookPath(name)
		if err == nil {
			return p
		}
	}
	return ""
}

// ExtractTar extracts a tar archive (optionally gzip-compressed) to dest directory.
func ExtractTar(r io.Reader, dest string) error {
	// Peek at the first bytes to detect gzip.
	buf := make([]byte, 2)
	_, err := r.Read(buf)
	if err != nil {
		return err
	}
	mr := io.MultiReader(bytes.NewReader(buf), r)
	if buf[0] == 0x1f && buf[1] == 0x8b {
		gzr, err := gzip.NewReader(mr)
		if err != nil {
			return err
		}
		defer func() {
			if err := gzr.Close(); err != nil {
				slog.Warn("close gzip reader failed", "error", err)
			}
		}()
		r = gzr
	} else {
		r = mr
	}

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
		lexical := filepath.Clean(filepath.Join(dest, hdr.Name))
		if hdr.Name == "." || hdr.Name == "./" || lexical == cleanDest {
			continue
		}
		if !strings.HasPrefix(lexical, cleanDest+string(os.PathSeparator)) && lexical != cleanDest {
			return fmt.Errorf("tar: path traversal attempt: %s", hdr.Name)
		}
		// HIGH-9: resolve the parent directory with symlink semantics clamped to
		// dest, so a prior entry like "evil -> /" cannot make a later
		// "evil/etc/cron.d/x" write through the symlink onto the host.
		parent, perr := common.SecureJoin(cleanDest, filepath.Dir(hdr.Name))
		if perr != nil {
			return fmt.Errorf("tar: resolve %s: %w", hdr.Name, perr)
		}
		target := filepath.Join(parent, filepath.Base(hdr.Name))
		if !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) && target != cleanDest {
			return fmt.Errorf("tar: path traversal attempt: %s", hdr.Name)
		}
		mode := os.FileMode(hdr.Mode & 07777)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			// Replace an existing symlink at the leaf instead of writing through it.
			if fi, lerr := os.Lstat(target); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(target)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				if closeErr := f.Close(); closeErr != nil {
					slog.Warn("close file failed", "path", target, "error", closeErr)
				}
				return err
			}
			if err := f.Close(); err != nil {
				slog.Warn("close file failed", "path", target, "error", err)
			}
			if err := os.Chmod(target, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if filepath.IsAbs(hdr.Linkname) {
				resolved := filepath.Clean(hdr.Linkname)
				// Only block absolute symlinks that try to escape via ".."
				// traversal. Legitimate rootfs images (Alpine, Debian, etc.)
				// contain absolute symlinks like "bin/arch -> /bin/busybox"
				// that point to other paths within the rootfs.
				if strings.HasPrefix(resolved, "..") || strings.Contains(resolved, "/../") {
					return fmt.Errorf("tar: symlink traversal (absolute): %s -> %s", hdr.Name, hdr.Linkname)
				}
			} else {
				resolved := filepath.Clean(filepath.Join(filepath.Dir(target), hdr.Linkname))
				if !strings.HasPrefix(resolved, cleanDest+string(os.PathSeparator)) && resolved != cleanDest {
					return fmt.Errorf("tar: symlink traversal: %s -> %s", hdr.Name, hdr.Linkname)
				}
			}
			if err := os.Remove(target); err != nil {
				slog.Warn("remove symlink target failed", "path", target, "error", err)
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			linkTarget := filepath.Clean(filepath.Join(dest, hdr.Linkname))
			if !strings.HasPrefix(linkTarget, cleanDest+string(os.PathSeparator)) && linkTarget != cleanDest {
				return fmt.Errorf("tar: hardlink traversal: %s -> %s", hdr.Name, hdr.Linkname)
			}
			if err := os.Remove(target); err != nil {
				slog.Warn("remove hardlink target failed", "path", target, "error", err)
			}
			if err := os.Link(linkTarget, target); err != nil {
				return err
			}
		default:
		}
	}
	return nil
}

// CreateTar creates a tar archive from a directory.
func CreateTar(dir string, writer io.Writer) error {
	tw := tar.NewWriter(writer)
	defer func() {
		if err := tw.Close(); err != nil {
			slog.Warn("close tar writer failed", "error", err)
		}
	}()

	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil
		}

		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		hdr.Name = rel
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Debug("skipping unreadable file in tar", "path", path, "err", err)
			return nil
		}
		hdr.Size = int64(len(data))
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
		return nil
	})
}

// CompressGzip compresses data using gzip.
func CompressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		if closeErr := w.Close(); closeErr != nil {
			slog.Warn("close gzip writer failed", "error", closeErr)
		}
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ExecuteBuildContext extracts a build context and sets it up.
func (b *Builder) ExecuteBuildContext(contextDir, tarPath, _ string) error {
	if tarPath != "" {
		f, err := os.Open(tarPath)
		if err != nil {
			return fmt.Errorf("open context tar: %w", err)
		}
		defer func() { _ = f.Close() }()
		if err := ExtractTar(f, contextDir); err != nil {
			return fmt.Errorf("extract context: %w", err)
		}
	}
	return nil
}
