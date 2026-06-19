package builder

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpceanAI/Doki/pkg/image"
)

// --- PARSER TESTS ---

func TestParseFromWithAS(t *testing.T) {
	content := []byte("FROM alpine:3.18 AS builder\nRUN echo hi\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if len(stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(stages))
	}
	if stages[0].From != "alpine:3.18" {
		t.Errorf("From = %q, want alpine:3.18", stages[0].From)
	}
	if stages[0].Name != "builder" {
		t.Errorf("Name = %q, want builder", stages[0].Name)
	}
}

func TestParseFromMultiStage(t *testing.T) {
	content := []byte(`FROM golang:1.21 AS build
RUN go build
FROM build AS final
COPY --from=build /app /app
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if len(stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(stages))
	}
	if stages[1].From != "build" {
		t.Errorf("Stage[1].From = %q, want build", stages[1].From)
	}
	if stages[1].FromStage != "build" {
		t.Errorf("FromStage = %q, want build", stages[1].FromStage)
	}
}

func TestParseFromPlatform(t *testing.T) {
	content := []byte("FROM --platform=linux/amd64 ubuntu:22.04\nRUN echo hi\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if stages[0].Platform != "linux/amd64" {
		t.Errorf("Platform = %q, want linux/amd64", stages[0].Platform)
	}
	if stages[0].From != "ubuntu:22.04" {
		t.Errorf("From = %q, want ubuntu:22.04", stages[0].From)
	}
}

func TestParseRunShellForm(t *testing.T) {
	content := []byte("FROM alpine\nRUN echo hello world\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "RUN" {
		t.Errorf("Type = %q, want RUN", inst.Type)
	}
	if len(inst.Args) < 2 {
		t.Errorf("Args = %v, want at least 2 elements", inst.Args)
	}
}

func TestParseRunExecForm(t *testing.T) {
	content := []byte("FROM alpine\nRUN [\"/bin/sh\", \"-c\", \"echo hello\"]\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if len(inst.Args) != 3 {
		t.Errorf("Args = %v, want 3 elements", inst.Args)
	}
}

func TestParseCmdShellForm(t *testing.T) {
	content := []byte("FROM alpine\nCMD echo hello\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "CMD" {
		t.Errorf("Type = %q, want CMD", inst.Type)
	}
}

func TestParseCmdExecForm(t *testing.T) {
	content := []byte("FROM alpine\nCMD [\"nginx\", \"-g\", \"daemon off;\"]\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if len(inst.Args) != 3 {
		t.Errorf("Args = %v, want 3 elements", inst.Args)
	}
	if inst.Args[0] != "nginx" {
		t.Errorf("Args[0] = %q, want nginx", inst.Args[0])
	}
}

func TestParseCmdDefaultArgs(t *testing.T) {
	content := []byte(`FROM alpine
ENTRYPOINT ["/bin/echo"]
CMD ["hello", "world"]
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if len(stages[0].Instructions) != 2 {
		t.Fatalf("Instructions = %d, want 2", len(stages[0].Instructions))
	}
	cmdInst := stages[0].Instructions[1]
	if len(cmdInst.Args) != 2 {
		t.Errorf("CMD Args = %v, want 2 default args", cmdInst.Args)
	}
}

func TestParseEntrypointShellForm(t *testing.T) {
	content := []byte("FROM alpine\nENTRYPOINT /bin/echo hello\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ENTRYPOINT" {
		t.Errorf("Type = %q, want ENTRYPOINT", inst.Type)
	}
}

func TestParseEntrypointExecForm(t *testing.T) {
	content := []byte("FROM alpine\nENTRYPOINT [\"/bin/sh\", \"-c\"]\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if len(inst.Args) != 2 {
		t.Errorf("Args = %v, want 2 elements", inst.Args)
	}
}

func TestParseLabelSingle(t *testing.T) {
	content := []byte("FROM alpine\nLABEL version=1.0\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "LABEL" {
		t.Errorf("Type = %q, want LABEL", inst.Type)
	}
}

func TestParseLabelMultiple(t *testing.T) {
	content := []byte("FROM alpine\nLABEL version=1.0 maintainer=test@example.com\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "LABEL" {
		t.Errorf("Type = %q, want LABEL", inst.Type)
	}
}

func TestParseExposeSingle(t *testing.T) {
	content := []byte("FROM alpine\nEXPOSE 80\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "EXPOSE" {
		t.Errorf("Type = %q, want EXPOSE", inst.Type)
	}
}

func TestParseExposeMultiple(t *testing.T) {
	content := []byte("FROM alpine\nEXPOSE 80 443 8080\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if len(inst.Args) != 3 {
		t.Errorf("Args = %v, want 3 ports", inst.Args)
	}
}

func TestParseExposeProtocol(t *testing.T) {
	content := []byte("FROM alpine\nEXPOSE 80/tcp 53/udp\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if len(inst.Args) != 2 {
		t.Errorf("Args = %v, want 2 ports", inst.Args)
	}
}

func TestParseEnvKeyValue(t *testing.T) {
	content := []byte("FROM alpine\nENV APP_HOME=/app\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ENV" {
		t.Errorf("Type = %q, want ENV", inst.Type)
	}
}

func TestParseEnvKeySpaceValue(t *testing.T) {
	content := []byte("FROM alpine\nENV APP_HOME /app\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ENV" {
		t.Errorf("Type = %q, want ENV", inst.Type)
	}
	if len(inst.Args) != 2 {
		t.Errorf("Args = %v, want 2 (key, value)", inst.Args)
	}
}

func TestParseAddLocal(t *testing.T) {
	content := []byte("FROM alpine\nADD file.txt /dest/\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ADD" {
		t.Errorf("Type = %q, want ADD", inst.Type)
	}
}

func TestParseAddURL(t *testing.T) {
	content := []byte("FROM alpine\nADD https://example.com/file.tar.gz /tmp/\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ADD" {
		t.Errorf("Type = %q, want ADD", inst.Type)
	}
}

func TestParseAddChown(t *testing.T) {
	content := []byte("FROM alpine\nADD --chown=1000:1000 file.txt /dest/\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ADD" {
		t.Errorf("Type = %q, want ADD", inst.Type)
	}
}

func TestParseCopyLocal(t *testing.T) {
	content := []byte("FROM alpine\nCOPY file.txt /dest/\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "COPY" {
		t.Errorf("Type = %q, want COPY", inst.Type)
	}
}

func TestParseCopyFrom(t *testing.T) {
	content := []byte(`FROM golang AS build
RUN go build
FROM alpine
COPY --from=build /app /app
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	copyInst := p.GetStages()[1].Instructions[0]
	if copyInst.Type != "COPY" {
		t.Errorf("Type = %q, want COPY", copyInst.Type)
	}
}

func TestParseCopyChmod(t *testing.T) {
	content := []byte("FROM alpine\nCOPY --chmod=755 script.sh /app/\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "COPY" {
		t.Errorf("Type = %q, want COPY", inst.Type)
	}
}

func TestParseVolume(t *testing.T) {
	content := []byte("FROM alpine\nVOLUME /data\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "VOLUME" {
		t.Errorf("Type = %q, want VOLUME", inst.Type)
	}
}

func TestParseUserWithGroup(t *testing.T) {
	content := []byte("FROM alpine\nUSER nobody:nogroup\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Args[0] != "nobody:nogroup" {
		t.Errorf("Args[0] = %q, want nobody:nogroup", inst.Args[0])
	}
}

func TestParseWorkdirAbsolute(t *testing.T) {
	content := []byte("FROM alpine\nWORKDIR /app\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Args[0] != "/app" {
		t.Errorf("Args[0] = %q, want /app", inst.Args[0])
	}
}

func TestParseWorkdirRelative(t *testing.T) {
	content := []byte("FROM alpine\nWORKDIR /app\nWORKDIR src\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	insts := p.GetStages()[0].Instructions
	if insts[1].Args[0] != "src" {
		t.Errorf("Args[0] = %q, want src", insts[1].Args[0])
	}
}

func TestParseArgWithDefault(t *testing.T) {
	content := []byte("FROM alpine\nARG VERSION=3.18\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ARG" {
		t.Errorf("Type = %q, want ARG", inst.Type)
	}
}

func TestParseArgWithoutDefault(t *testing.T) {
	content := []byte("FROM alpine\nARG BUILD_DATE\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ARG" {
		t.Errorf("Type = %q, want ARG", inst.Type)
	}
}

func TestParseOnbuild(t *testing.T) {
	content := []byte("FROM alpine\nONBUILD RUN echo triggered\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "ONBUILD" {
		t.Errorf("Type = %q, want ONBUILD", inst.Type)
	}
}

func TestParseStopsignal(t *testing.T) {
	content := []byte("FROM alpine\nSTOPSIGNAL SIGTERM\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Args[0] != "SIGTERM" {
		t.Errorf("Args[0] = %q, want SIGTERM", inst.Args[0])
	}
}

func TestParseHealthcheckCmd(t *testing.T) {
	content := []byte("FROM alpine\nHEALTHCHECK --interval=30s CMD curl -f http://localhost/ || exit 1\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "HEALTHCHECK" {
		t.Errorf("Type = %q, want HEALTHCHECK", inst.Type)
	}
}

func TestParseHealthcheckNone(t *testing.T) {
	content := []byte("FROM alpine\nHEALTHCHECK NONE\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Args[0] != "NONE" {
		t.Errorf("Args[0] = %q, want NONE", inst.Args[0])
	}
}

func TestParseShell(t *testing.T) {
	content := []byte("FROM alpine\nSHELL [\"/bin/bash\", \"-c\"]\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if len(inst.Args) != 2 {
		t.Errorf("Args = %v, want 2 elements", inst.Args)
	}
	if inst.Args[0] != "/bin/bash" {
		t.Errorf("Args[0] = %q, want /bin/bash", inst.Args[0])
	}
}

func TestParseMaintainerInstr(t *testing.T) {
	content := []byte("FROM alpine\nMAINTAINER test@example.com\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "MAINTAINER" {
		t.Errorf("Type = %q, want MAINTAINER", inst.Type)
	}
	if inst.Args[0] != "test@example.com" {
		t.Errorf("Args[0] = %q, want test@example.com", inst.Args[0])
	}
}

// --- EDGE CASE TESTS ---

func TestParseLineContinuation(t *testing.T) {
	content := []byte("FROM alpine\nRUN apt-get update && \\\n    apt-get install -y curl\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	insts := p.GetStages()[0].Instructions
	if len(insts) != 1 {
		t.Fatalf("Instructions = %d, want 1 (continuation should join)", len(insts))
	}
	if insts[0].Type != "RUN" {
		t.Errorf("Type = %q, want RUN", insts[0].Type)
	}
}

func TestParseCommentsEdgeCase(t *testing.T) {
	content := []byte("# This is a comment\nFROM alpine\n# Inline comment\nRUN echo hi\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	insts := p.GetStages()[0].Instructions
	if len(insts) != 1 {
		t.Errorf("Instructions = %d, want 1", len(insts))
	}
}

func TestParseVariableSubstitution(t *testing.T) {
	content := []byte("FROM alpine\nENV MY_VAR=hello\nRUN echo ${MY_VAR}\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	insts := p.GetStages()[0].Instructions
	if len(insts) != 2 {
		t.Fatalf("Instructions = %d, want 2", len(insts))
	}
}

func TestParseEscapedVariable(t *testing.T) {
	content := []byte("FROM alpine\nRUN echo \\$NOT_A_VAR\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	insts := p.GetStages()[0].Instructions
	if len(insts) != 1 {
		t.Fatalf("Instructions = %d, want 1", len(insts))
	}
}

func TestParseEmptyInstruction(t *testing.T) {
	content := []byte("FROM alpine\n\n\nRUN echo hi\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	insts := p.GetStages()[0].Instructions
	if len(insts) != 1 {
		t.Errorf("Instructions = %d, want 1", len(insts))
	}
}

func TestParseInvalidInstruction(t *testing.T) {
	content := []byte("FROM alpine\nFOOBAR something\n")
	p := NewDokifileParser()
	err := p.Parse(content)
	if err == nil {
		t.Error("expected error for unknown instruction FOOBAR")
	}
}

func TestParseMissingFrom(t *testing.T) {
	content := []byte("RUN echo hello\n")
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if len(stages) != 0 {
		t.Errorf("stages = %d, want 0 (no FROM)", len(stages))
	}
}

func TestParseMultipleFrom(t *testing.T) {
	content := []byte(`FROM alpine AS stage1
RUN echo 1
FROM ubuntu AS stage2
RUN echo 2
FROM debian AS stage3
RUN echo 3
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if len(stages) != 3 {
		t.Fatalf("stages = %d, want 3", len(stages))
	}
	if stages[0].Name != "stage1" {
		t.Errorf("Stage[0].Name = %q, want stage1", stages[0].Name)
	}
	if stages[1].Name != "stage2" {
		t.Errorf("Stage[1].Name = %q, want stage2", stages[1].Name)
	}
	if stages[2].Name != "stage3" {
		t.Errorf("Stage[2].Name = %q, want stage3", stages[2].Name)
	}
}

// --- VARIABLE SUBSTITUTION TESTS ---

func TestSubstituteVarsBraces(t *testing.T) {
	envMap := map[string]string{"FOO": "bar"}
	result := substituteVars("hello ${FOO} world", envMap, nil, nil)
	if result != "hello bar world" {
		t.Errorf("result = %q, want 'hello bar world'", result)
	}
}

func TestSubstituteVarsNoBraces(t *testing.T) {
	envMap := map[string]string{"FOO": "bar"}
	result := substituteVars("hello $FOO world", envMap, nil, nil)
	if result != "hello bar world" {
		t.Errorf("result = %q, want 'hello bar world'", result)
	}
}

func TestSubstituteVarsEscaped(t *testing.T) {
	envMap := map[string]string{"FOO": "bar"}
	result := substituteVars("hello \\$FOO world", envMap, nil, nil)
	if result != "hello $FOO world" {
		t.Errorf("result = %q, want 'hello $FOO world'", result)
	}
}

func TestSubstituteVarsUndefined(t *testing.T) {
	result := substituteVars("hello ${UNDEFINED} world", nil, nil, nil)
	if result != "hello  world" {
		t.Errorf("result = %q, want 'hello  world'", result)
	}
}

func TestSubstituteVarsBuildArgPriority(t *testing.T) {
	envMap := map[string]string{"FOO": "from_env"}
	buildArgs := map[string]string{"FOO": "from_buildarg"}
	result := substituteVars("${FOO}", envMap, nil, buildArgs)
	if result != "from_buildarg" {
		t.Errorf("result = %q, want 'from_buildarg' (buildArgs should have priority)", result)
	}
}

// --- EXECUTOR TESTS ---

func TestExecuteEnv(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "ENV", Args: []string{"KEY=VALUE"}}
	err := b.executeEnv(stage, inst)
	if err != nil {
		t.Fatalf("executeEnv: %v", err)
	}
	if b.envMap["KEY"] != "VALUE" {
		t.Errorf("envMap[KEY] = %q, want VALUE", b.envMap["KEY"])
	}
	found := false
	for _, e := range stage.ImageConfig.Env {
		if e == "KEY=VALUE" {
			found = true
		}
	}
	if !found {
		t.Errorf("ImageConfig.Env = %v, want to contain KEY=VALUE", stage.ImageConfig.Env)
	}
}

func TestExecuteEnvOverwrite(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	b.executeEnv(stage, &Instruction{Type: "ENV", Args: []string{"KEY=first"}})
	b.executeEnv(stage, &Instruction{Type: "ENV", Args: []string{"KEY=second"}})
	if b.envMap["KEY"] != "second" {
		t.Errorf("envMap[KEY] = %q, want second", b.envMap["KEY"])
	}
	count := 0
	for _, e := range stage.ImageConfig.Env {
		if strings.HasPrefix(e, "KEY=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("KEY appears %d times in Env, want 1", count)
	}
}

func TestExecuteWorkdirAbsolute(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	workDir := "/"
	rootDir := t.TempDir()
	inst := &Instruction{Type: "WORKDIR", Args: []string{"/app"}}
	err := b.executeWorkdir(stage, inst, rootDir, &workDir)
	if err != nil {
		t.Fatalf("executeWorkdir: %v", err)
	}
	if workDir != "/app" {
		t.Errorf("workDir = %q, want /app", workDir)
	}
	if stage.ImageConfig.WorkingDir != "/app" {
		t.Errorf("WorkingDir = %q, want /app", stage.ImageConfig.WorkingDir)
	}
}

func TestExecuteWorkdirRelative(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	workDir := "/app"
	rootDir := t.TempDir()
	inst := &Instruction{Type: "WORKDIR", Args: []string{"src"}}
	err := b.executeWorkdir(stage, inst, rootDir, &workDir)
	if err != nil {
		t.Fatalf("executeWorkdir: %v", err)
	}
	if workDir != "/app/src" {
		t.Errorf("workDir = %q, want /app/src", workDir)
	}
}

func TestExecuteUser(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "USER", Args: []string{"nobody"}}
	err := b.executeUser(stage, inst)
	if err != nil {
		t.Fatalf("executeUser: %v", err)
	}
	if stage.ImageConfig.User != "nobody" {
		t.Errorf("User = %q, want nobody", stage.ImageConfig.User)
	}
}

func TestExecuteExpose(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "EXPOSE", Args: []string{"80", "443/udp"}}
	err := b.executeExpose(stage, inst)
	if err != nil {
		t.Fatalf("executeExpose: %v", err)
	}
	if _, ok := stage.ImageConfig.ExposedPorts["80/tcp"]; !ok {
		t.Error("80/tcp not in ExposedPorts")
	}
	if _, ok := stage.ImageConfig.ExposedPorts["443/udp"]; !ok {
		t.Error("443/udp not in ExposedPorts")
	}
}

func TestExecuteLabel(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "LABEL", Args: []string{"version=1.0", "maintainer=test"}}
	err := b.executeLabel(stage, inst)
	if err != nil {
		t.Fatalf("executeLabel: %v", err)
	}
	if stage.ImageConfig.Labels["version"] != "1.0" {
		t.Errorf("Labels[version] = %q, want 1.0", stage.ImageConfig.Labels["version"])
	}
	if stage.ImageConfig.Labels["maintainer"] != "test" {
		t.Errorf("Labels[maintainer] = %q, want test", stage.ImageConfig.Labels["maintainer"])
	}
}

func TestExecuteCmd(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "CMD", Args: []string{"nginx", "-g", "daemon off;"}}
	err := b.executeCmd(stage, inst)
	if err != nil {
		t.Fatalf("executeCmd: %v", err)
	}
	if len(stage.ImageConfig.Cmd) != 3 {
		t.Errorf("Cmd = %v, want 3 elements", stage.ImageConfig.Cmd)
	}
}

func TestExecuteEntrypoint(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "ENTRYPOINT", Args: []string{"/entrypoint.sh"}}
	err := b.executeEntrypoint(stage, inst)
	if err != nil {
		t.Fatalf("executeEntrypoint: %v", err)
	}
	if len(stage.ImageConfig.Entrypoint) != 1 {
		t.Errorf("Entrypoint = %v, want 1 element", stage.ImageConfig.Entrypoint)
	}
}

func TestExecuteVolume(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "VOLUME", Args: []string{"/data", "/logs"}}
	err := b.executeVolume(stage, inst)
	if err != nil {
		t.Fatalf("executeVolume: %v", err)
	}
	if _, ok := stage.ImageConfig.Volumes["/data"]; !ok {
		t.Error("/data not in Volumes")
	}
	if _, ok := stage.ImageConfig.Volumes["/logs"]; !ok {
		t.Error("/logs not in Volumes")
	}
}

func TestExecuteHealthcheckCmd(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "HEALTHCHECK", Args: []string{"--interval=30s", "--timeout=10s", "--retries=3", "CMD", "curl", "-f", "http://localhost/"}}
	err := b.executeHealthcheck(stage, inst)
	if err != nil {
		t.Fatalf("executeHealthcheck: %v", err)
	}
	if stage.ImageConfig.HealthCheck == nil {
		t.Fatal("HealthCheck is nil")
	}
	if stage.ImageConfig.HealthCheck.Interval != 30000000000 {
		t.Errorf("Interval = %d, want 30000000000", stage.ImageConfig.HealthCheck.Interval)
	}
	if stage.ImageConfig.HealthCheck.Retries != 3 {
		t.Errorf("Retries = %d, want 3", stage.ImageConfig.HealthCheck.Retries)
	}
}

func TestExecuteHealthcheckNone(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{
		HealthCheck: &image.HealthCheckConfig{Test: []string{"something"}},
	}}
	inst := &Instruction{Type: "HEALTHCHECK", Args: []string{"NONE"}}
	err := b.executeHealthcheck(stage, inst)
	if err != nil {
		t.Fatalf("executeHealthcheck: %v", err)
	}
	if stage.ImageConfig.HealthCheck != nil {
		t.Error("HealthCheck should be nil after NONE")
	}
}

func TestExecuteStopsignal(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "STOPSIGNAL", Args: []string{"SIGTERM"}}
	err := b.executeStopsignal(stage, inst)
	if err != nil {
		t.Fatalf("executeStopsignal: %v", err)
	}
	if stage.ImageConfig.StopSignal != "SIGTERM" {
		t.Errorf("StopSignal = %q, want SIGTERM", stage.ImageConfig.StopSignal)
	}
}

func TestExecuteShell(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "SHELL", Args: []string{"/bin/bash", "-c"}}
	err := b.executeShell(stage, inst)
	if err != nil {
		t.Fatalf("executeShell: %v", err)
	}
	if len(stage.ImageConfig.Shell) != 2 {
		t.Errorf("Shell = %v, want 2 elements", stage.ImageConfig.Shell)
	}
}

func TestExecuteArgWithDefault(t *testing.T) {
	b := NewBuilder(nil)
	b.argDefaults = make(map[string]string)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "ARG", Args: []string{"VERSION=3.18"}}
	err := b.executeArg(stage, inst)
	if err != nil {
		t.Fatalf("executeArg: %v", err)
	}
	if b.argDefaults["VERSION"] != "3.18" {
		t.Errorf("argDefaults[VERSION] = %q, want 3.18", b.argDefaults["VERSION"])
	}
}

func TestExecuteArgWithoutDefault(t *testing.T) {
	b := NewBuilder(nil)
	b.argDefaults = make(map[string]string)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "ARG", Args: []string{"BUILD_DATE"}}
	err := b.executeArg(stage, inst)
	if err != nil {
		t.Fatalf("executeArg: %v", err)
	}
	if _, ok := b.argDefaults["BUILD_DATE"]; !ok {
		t.Error("BUILD_DATE not in argDefaults")
	}
}

func TestExecuteArgExternalOverride(t *testing.T) {
	b := NewBuilder(nil)
	b.argDefaults = map[string]string{"VERSION": "override"}
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "ARG", Args: []string{"VERSION=default"}}
	err := b.executeArg(stage, inst)
	if err != nil {
		t.Fatalf("executeArg: %v", err)
	}
	if b.argDefaults["VERSION"] != "override" {
		t.Errorf("argDefaults[VERSION] = %q, want override (external should win)", b.argDefaults["VERSION"])
	}
}

func TestExecuteOnbuild(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{Metadata: make(map[string]string)}
	inst := &Instruction{Type: "ONBUILD", Args: []string{"RUN", "echo", "triggered"}}
	err := b.executeOnbuild(stage, inst)
	if err != nil {
		t.Fatalf("executeOnbuild: %v", err)
	}
	if stage.Metadata["onbuild"] == "" {
		t.Error("onbuild metadata should not be empty")
	}
}

func TestExecuteMaintainer(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{Metadata: make(map[string]string)}
	inst := &Instruction{Type: "MAINTAINER", Args: []string{"test@example.com"}}
	err := b.executeMaintainer(stage, inst)
	if err != nil {
		t.Fatalf("executeMaintainer: %v", err)
	}
	if stage.Metadata["Maintainer"] != "test@example.com" {
		t.Errorf("Maintainer = %q, want test@example.com", stage.Metadata["Maintainer"])
	}
}

// --- COPY TESTS ---

func TestExecuteCopyLocalFile(t *testing.T) {
	ctxDir := t.TempDir()
	rootDir := t.TempDir()
	storeDir := t.TempDir()
	os.WriteFile(filepath.Join(ctxDir, "test.txt"), []byte("hello"), 0644)

	store, err := image.NewStore(storeDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	b := NewBuilder(store)
	b.stageDirs = make(map[string]string)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}

	inst := &Instruction{Type: "COPY", Args: []string{"test.txt", "/dest/"}}
	err = b.executeCopy(stage, inst, ctxDir, rootDir)
	if err != nil {
		t.Fatalf("executeCopy: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(rootDir, "dest", "test.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want hello", string(data))
	}
}

func TestExecuteCopyGlob(t *testing.T) {
	ctxDir := t.TempDir()
	rootDir := t.TempDir()
	storeDir := t.TempDir()
	os.WriteFile(filepath.Join(ctxDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(ctxDir, "b.txt"), []byte("b"), 0644)

	store, _ := image.NewStore(storeDir)
	b := NewBuilder(store)
	b.stageDirs = make(map[string]string)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}

	inst := &Instruction{Type: "COPY", Args: []string{"*.txt", "/dest/"}}
	err := b.executeCopy(stage, inst, ctxDir, rootDir)
	if err != nil {
		t.Fatalf("executeCopy: %v", err)
	}
}

func TestExecuteCopyDirectory(t *testing.T) {
	ctxDir := t.TempDir()
	rootDir := t.TempDir()
	storeDir := t.TempDir()
	os.MkdirAll(filepath.Join(ctxDir, "mydir"), 0755)
	os.WriteFile(filepath.Join(ctxDir, "mydir", "file.txt"), []byte("content"), 0644)

	store, _ := image.NewStore(storeDir)
	b := NewBuilder(store)
	b.stageDirs = make(map[string]string)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}

	inst := &Instruction{Type: "COPY", Args: []string{"mydir", "/dest/"}}
	err := b.executeCopy(stage, inst, ctxDir, rootDir)
	if err != nil {
		t.Fatalf("executeCopy: %v", err)
	}
}

// --- DOCKERIGNORE TESTS ---

func TestDockerignoreParse(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte("*.log\n# comment\n\n*.tmp\n"), 0644)
	di, err := ParseDockerignore(filepath.Join(dir, ".dockerignore"))
	if err != nil {
		t.Fatalf("ParseDockerignore: %v", err)
	}
	if len(di.patterns) != 2 {
		t.Errorf("patterns = %d, want 2", len(di.patterns))
	}
}

func TestDockerignoreMatches(t *testing.T) {
	di := &Dockerignore{patterns: []string{"*.log", "*.tmp"}}
	if !di.Matches("test.log") {
		t.Error("should match test.log")
	}
	if !di.Matches("dir/test.tmp") {
		t.Error("should match dir/test.tmp (base name)")
	}
	if di.Matches("test.go") {
		t.Error("should not match test.go")
	}
}

func TestDockerignoreMissing(t *testing.T) {
	di, err := ParseDockerignore("/nonexistent/.dockerignore")
	if err != nil {
		t.Fatalf("ParseDockerignore: %v", err)
	}
	if len(di.patterns) != 0 {
		t.Errorf("patterns = %d, want 0", len(di.patterns))
	}
}

// --- PARSE ARGS TESTS ---

func TestParseArgsEmpty(t *testing.T) {
	args := parseArgs("")
	if args != nil {
		t.Errorf("args = %v, want nil", args)
	}
}

func TestParseArgsJSONArray(t *testing.T) {
	args := parseArgs(`["nginx", "-g", "daemon off;"]`)
	if len(args) != 3 {
		t.Errorf("args = %v, want 3 elements", args)
	}
	if args[2] != "daemon off;" {
		t.Errorf("args[2] = %q, want 'daemon off;'", args[2])
	}
}

func TestParseArgsShellForm(t *testing.T) {
	args := parseArgs("echo hello world")
	if len(args) != 3 {
		t.Errorf("args = %v, want 3 elements", args)
	}
}

func TestParseArgsQuoted(t *testing.T) {
	args := parseArgs(`echo "hello world" foo`)
	if len(args) != 3 {
		t.Errorf("args = %v, want 3 elements", args)
	}
	if args[1] != "hello world" {
		t.Errorf("args[1] = %q, want 'hello world'", args[1])
	}
}

// --- VALIDATE TESTS ---

func TestValidateValid(t *testing.T) {
	err := Validate([]byte("FROM alpine\nRUN echo test\n"))
	if err != nil {
		t.Errorf("Validate valid: %v", err)
	}
}

func TestValidateInvalid(t *testing.T) {
	err := Validate([]byte("INVALID instruction\n"))
	if err == nil {
		t.Error("Validate should return error for invalid Dokifile")
	}
}

func TestValidateEmpty(t *testing.T) {
	err := Validate([]byte(""))
	if err != nil {
		t.Errorf("Validate empty: %v", err)
	}
}

// --- DOCKERIGNORE DOUBLE STAR TESTS ---

func TestDockerignoreDoubleStar(t *testing.T) {
	di := &Dockerignore{patterns: []string{"**/*.log"}}
	if !di.Matches("test.log") {
		t.Error("should match test.log")
	}
	if !di.Matches("dir/sub/test.log") {
		t.Error("should match dir/sub/test.log")
	}
}

// --- HEALTHCHECK PARSING ---

func TestExecuteHealthcheckStartPeriod(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "HEALTHCHECK", Args: []string{"--interval=5s", "--timeout=3s", "--start-period=10s", "--retries=5", "CMD", "echo", "ok"}}
	err := b.executeHealthcheck(stage, inst)
	if err != nil {
		t.Fatalf("executeHealthcheck: %v", err)
	}
	hc := stage.ImageConfig.HealthCheck
	if hc == nil {
		t.Fatal("HealthCheck is nil")
	}
	if hc.StartPeriod != 10000000000 {
		t.Errorf("StartPeriod = %d, want 10000000000", hc.StartPeriod)
	}
	if hc.Timeout != 3000000000 {
		t.Errorf("Timeout = %d, want 3000000000", hc.Timeout)
	}
	if hc.Retries != 5 {
		t.Errorf("Retries = %d, want 5", hc.Retries)
	}
}

// --- ONBUILD PARSING ---

func TestParseOnbuildInstructions(t *testing.T) {
	data := "RUN|echo hello;;COPY|file.txt /dest/"
	insts := parseOnbuildInstructions(data)
	if len(insts) != 2 {
		t.Fatalf("insts = %d, want 2", len(insts))
	}
	if insts[0].Type != "RUN" {
		t.Errorf("insts[0].Type = %q, want RUN", insts[0].Type)
	}
	if insts[1].Type != "COPY" {
		t.Errorf("insts[1].Type = %q, want COPY", insts[1].Type)
	}
}

func TestParseOnbuildInstructionsEmpty(t *testing.T) {
	insts := parseOnbuildInstructions("")
	if insts != nil {
		t.Errorf("expected nil for empty onbuild data")
	}
}

// --- ENV KEY=VALUE PARSING IN EXECUTOR ---

func TestExecuteEnvMultipleInOneInstruction(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "ENV", Args: []string{"KEY1=val1", "KEY2=val2"}}
	err := b.executeEnv(stage, inst)
	if err != nil {
		t.Fatalf("executeEnv: %v", err)
	}
	if b.envMap["KEY1"] != "val1" {
		t.Errorf("envMap[KEY1] = %q, want val1", b.envMap["KEY1"])
	}
	if b.envMap["KEY2"] != "val2" {
		t.Errorf("envMap[KEY2] = %q, want val2", b.envMap["KEY2"])
	}
}

// --- COPY --from NUMERIC INDEX ---

func TestParseCopyFromNumeric(t *testing.T) {
	content := []byte(`FROM alpine AS build
RUN echo build
FROM alpine
COPY --from=0 /app /app
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := p.GetStages()
	if len(stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(stages))
	}
	copyInst := stages[1].Instructions[0]
	if copyInst.Type != "COPY" {
		t.Errorf("Type = %q, want COPY", copyInst.Type)
	}
}

// --- DIRECTIVE PARSING ---

func TestParseDirectivesOnlyBeforeFirstInstruction(t *testing.T) {
	content := []byte(`# syntax=dockerfile:1
# escape=` + "`" + `
FROM alpine
# this is NOT a directive
RUN echo hi
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.GetDirective("syntax") != "dockerfile:1" {
		t.Errorf("syntax = %q, want dockerfile:1", p.GetDirective("syntax"))
	}
}

// --- LABEL WITH QUOTES ---

func TestParseLabelWithQuotes(t *testing.T) {
	content := []byte(`FROM alpine
LABEL "com.example.version"="1.0"
`)
	p := NewDokifileParser()
	if err := p.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	inst := p.GetStages()[0].Instructions[0]
	if inst.Type != "LABEL" {
		t.Errorf("Type = %q, want LABEL", inst.Type)
	}
}

// --- EXPOSE PORT ONLY (no protocol) ---

func TestExecuteExposeDefaultProtocol(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "EXPOSE", Args: []string{"8080"}}
	err := b.executeExpose(stage, inst)
	if err != nil {
		t.Fatalf("executeExpose: %v", err)
	}
	if _, ok := stage.ImageConfig.ExposedPorts["8080/tcp"]; !ok {
		t.Error("8080/tcp not in ExposedPorts (should default to tcp)")
	}
}

// --- STOPSIGNAL NUMERIC ---

func TestExecuteStopsignalNumeric(t *testing.T) {
	b := NewBuilder(nil)
	stage := &Stage{ImageConfig: &image.ImageConfig{}}
	inst := &Instruction{Type: "STOPSIGNAL", Args: []string{"9"}}
	err := b.executeStopsignal(stage, inst)
	if err != nil {
		t.Fatalf("executeStopsignal: %v", err)
	}
	if stage.ImageConfig.StopSignal != "9" {
		t.Errorf("StopSignal = %q, want 9", stage.ImageConfig.StopSignal)
	}
}
