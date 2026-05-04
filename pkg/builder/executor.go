package builder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OpceanAI/Doki/pkg/common"
)

// ExecuteStage runs all instructions in a build stage.
func (b *Builder) ExecuteStage(stage *Stage, contextDir, workDir string) error {
	dir := workDir
	if dir == "" {
		dir = os.TempDir()
	}

	for _, inst := range stage.Instructions {
		if err := b.executeInstructionReal(stage, &inst, contextDir, &dir); err != nil {
			return fmt.Errorf("line %d (%s): %w", inst.LineNum, inst.Type, err)
		}
	}
	return nil
}

func (b *Builder) executeInstructionReal(stage *Stage, inst *Instruction, ctxDir string, workDir *string) error {
	switch inst.Type {
	case "RUN":
		return b.executeRun(stage, inst, *workDir)
	case "COPY":
		return b.executeCopy(stage, inst, ctxDir, *workDir)
	case "ADD":
		return b.executeAdd(stage, inst, ctxDir, *workDir)
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

func (b *Builder) executeRun(stage *Stage, inst *Instruction, workDir string) error {
	if len(inst.Args) == 0 {
		return nil
	}
	cmd := exec.Command("/bin/sh", "-c", strings.Join(inst.Args, " "))
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *Builder) executeCopy(stage *Stage, inst *Instruction, ctxDir, workDir string) error {
	args := inst.Args
	var src, dst string
	for i, a := range args {
		if a == "--from" && i+1 < len(args) {
			i++
			continue
		}
		if a == "--chown" || a == "--chmod" && i+1 < len(args) {
			i++
			continue
		}
		if !strings.HasPrefix(a, "-") {
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

	srcPath := filepath.Join(ctxDir, src)
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(workDir, dst)
	}

	if fi, err := os.Stat(srcPath); err == nil && fi.IsDir() {
		return common.CopyDir(srcPath, dst)
	}

	common.EnsureDir(filepath.Dir(dst))
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("copy %s: %w", src, err)
	}
	return os.WriteFile(dst, data, 0644)
}

func (b *Builder) executeAdd(stage *Stage, inst *Instruction, ctxDir, workDir string) error {
	return b.executeCopy(stage, inst, ctxDir, workDir)
}

func (b *Builder) executeEnv(stage *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
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
		common.EnsureDir(*workDir)
	}
	return nil
}

func (b *Builder) executeUser(stage *Stage, inst *Instruction) error {
	if len(inst.Args) > 0 {
		stage.Metadata["User"] = inst.Args[0]
	}
	return nil
}
func (b *Builder) executeExpose(stage *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		stage.Metadata[arg] = "expose"
	}
	return nil
}
func (b *Builder) executeLabel(stage *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			stage.Metadata["label:"+kv[0]] = kv[1]
		}
	}
	return nil
}
func (b *Builder) executeCmd(stage *Stage, inst *Instruction) error {
	stage.Metadata["Cmd"] = strings.Join(inst.Args, " ")
	return nil
}
func (b *Builder) executeEntrypoint(stage *Stage, inst *Instruction) error {
	stage.Metadata["Entrypoint"] = strings.Join(inst.Args, " ")
	return nil
}
func (b *Builder) executeVolume(stage *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		stage.Metadata["volume:"+arg] = "mount"
	}
	return nil
}
func (b *Builder) executeHealthcheck(stage *Stage, inst *Instruction) error {
	stage.Metadata["Healthcheck"] = strings.Join(inst.Args, " ")
	return nil
}
func (b *Builder) executeStopsignal(stage *Stage, inst *Instruction) error {
	if len(inst.Args) > 0 {
		stage.Metadata["StopSignal"] = inst.Args[0]
	}
	return nil
}
func (b *Builder) executeShell(stage *Stage, inst *Instruction) error {
	stage.Metadata["Shell"] = strings.Join(inst.Args, " ")
	return nil
}
func (b *Builder) executeArg(stage *Stage, inst *Instruction) error {
	for _, arg := range inst.Args {
		kv := strings.SplitN(arg, "=", 2)
		if len(kv) == 2 {
			stage.Metadata["arg:"+kv[0]] = kv[1]
		} else {
			stage.Metadata["arg:"+arg] = ""
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

// ExtractContainer extracts a tar archive (build context).
func ExtractTar(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			io.Copy(f, tr)
			f.Close()
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(target), 0755)
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
		defer f.Close()
		io.Copy(tw, f)
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

// ExecuteBuildContext extracts a build context and runs the Dockerfile.
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
