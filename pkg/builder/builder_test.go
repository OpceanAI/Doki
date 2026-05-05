package builder

import (
	"testing"
)

func TestParseBasicDockerfile(t *testing.T) {
	content := []byte(`FROM alpine:latest
RUN apk add --no-cache nginx
COPY nginx.conf /etc/nginx/
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
`)

	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	stages := parser.GetStages()
	if len(stages) != 1 {
		t.Fatalf("len(stages) = %d, want 1", len(stages))
	}
	stage := stages[0]
	if stage.From != "alpine:latest" {
		t.Errorf("From = %q, want alpine:latest", stage.From)
	}
	if len(stage.Instructions) != 4 {
		t.Errorf("len(Instructions) = %d, want 4", len(stage.Instructions))
	}
	if stage.Instructions[0].Type != "RUN" {
		t.Errorf("Instr[0].Type = %q, want RUN", stage.Instructions[0].Type)
	}
	if stage.Instructions[1].Type != "COPY" {
		t.Errorf("Instr[1].Type = %q, want COPY", stage.Instructions[1].Type)
	}
	if stage.Instructions[2].Type != "EXPOSE" {
		t.Errorf("Instr[2].Type = %q, want EXPOSE", stage.Instructions[2].Type)
	}
	if stage.Instructions[3].Type != "CMD" {
		t.Errorf("Instr[3].Type = %q, want CMD", stage.Instructions[3].Type)
	}
}

func TestParseMultiStageBuild(t *testing.T) {
	content := []byte(`FROM golang:alpine AS builder
WORKDIR /build
COPY . .
RUN go build -o app .

FROM alpine:latest
COPY --from=builder /build/app /usr/local/bin/app
CMD ["/usr/local/bin/app"]
`)

	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	stages := parser.GetStages()
	if len(stages) != 2 {
		t.Fatalf("len(stages) = %d, want 2", len(stages))
	}
	if stages[0].Name != "builder" {
		t.Errorf("Stage[0].Name = %q, want builder", stages[0].Name)
	}
	if stages[1].From != "alpine:latest" {
		t.Errorf("Stage[1].From = %q, want alpine:latest", stages[1].From)
	}
}

func TestParseWithPlatform(t *testing.T) {
	content := []byte(`FROM --platform=linux/arm64 alpine:latest
RUN echo hello
`)

	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	stages := parser.GetStages()
	if len(stages) != 1 {
		t.Fatalf("len(stages) = %d, want 1", len(stages))
	}
	if stages[0].Platform != "linux/arm64" {
		t.Errorf("Platform = %q, want linux/arm64", stages[0].Platform)
	}
}

func TestParseAllInstructions(t *testing.T) {
	content := []byte(`FROM alpine:latest
LABEL maintainer="test@example.com" version="1.0"
ENV APP_HOME=/app
WORKDIR ${APP_HOME}
RUN mkdir -p data
COPY . .
ADD https://example.com/file.tar.gz /tmp/
USER nobody
VOLUME /data
EXPOSE 8080/tcp
HEALTHCHECK --interval=30s CMD curl -f http://localhost:8080/ || exit 1
STOPSIGNAL SIGTERM
SHELL ["/bin/ash", "-c"]
ARG BUILD_DATE
CMD ["/bin/sh"]
`)

	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	stages := parser.GetStages()
	if len(stages) != 1 {
		t.Fatalf("len(stages) = %d, want 1", len(stages))
	}

	expected := []string{
		"LABEL", "ENV", "WORKDIR", "RUN", "COPY", "ADD",
		"USER", "VOLUME", "EXPOSE", "HEALTHCHECK", "STOPSIGNAL",
		"SHELL", "ARG", "CMD",
	}

	instructions := stages[0].Instructions
	if len(instructions) != len(expected) {
		t.Fatalf("len(Instructions) = %d, want %d", len(instructions), len(expected))
	}
	for i, exp := range expected {
		if instructions[i].Type != exp {
			t.Errorf("Instr[%d].Type = %q, want %q", i, instructions[i].Type, exp)
		}
	}
}

func TestParseEmptyDockerfile(t *testing.T) {
	parser := NewDokifileParser()
	if err := parser.Parse([]byte("")); err != nil {
		t.Fatalf("Parse empty: %v", err)
	}
	stages := parser.GetStages()
	if len(stages) != 0 {
		t.Errorf("empty file should have 0 stages, got %d", len(stages))
	}
}

func TestParseNoFrom(t *testing.T) {
	content := []byte(`LABEL foo=bar
CMD ["echo"]
`)
	parser := NewDokifileParser()
	err := parser.Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := parser.GetStages()
	// Instructions without FROM are dropped (no stage to attach to).
	if len(stages) != 0 {
		t.Errorf("expected 0 stages without FROM, got %d", len(stages))
	}
}

func TestParseComments(t *testing.T) {
	content := []byte(`# This is a comment
FROM alpine:latest
# Another comment
RUN echo hello
`)

	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := parser.GetStages()
	if len(stages) != 1 {
		t.Fatalf("len(stages) = %d, want 1", len(stages))
	}
	if len(stages[0].Instructions) != 1 {
		t.Errorf("len(Instructions) = %d, want 1", len(stages[0].Instructions))
	}
}

func TestParseParserDirectives(t *testing.T) {
	content := []byte(`# syntax=docker/dockerfile:1
# escape=\
FROM alpine:latest
RUN echo test
`)

	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parser.GetDirective("syntax") != "docker/dockerfile:1" {
		t.Errorf("syntax directive = %q, want docker/dockerfile:1", parser.GetDirective("syntax"))
	}
	if parser.GetDirective("escape") != `\` {
		t.Errorf("escape directive = %q, want backslash", parser.GetDirective("escape"))
	}
}

func TestParseUnknownInstruction(t *testing.T) {
	content := []byte(`FROM alpine:latest
FOOBAR something
`)
	parser := NewDokifileParser()
	err := parser.Parse(content)
	if err == nil {
		t.Fatal("expected error for unknown instruction")
	}
}

func TestParseJSONArrayArgs(t *testing.T) {
	content := []byte(`FROM alpine:latest
CMD ["nginx", "-g", "daemon off;"]
ENTRYPOINT ["/docker-entrypoint.sh"]
`)
	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := parser.GetStages()
	if len(stages) != 1 {
		t.Fatalf("len(stages) = %d", len(stages))
	}
	// CMD should have 3 args.
	cmdInst := stages[0].Instructions[0]
	if len(cmdInst.Args) != 3 {
		t.Errorf("CMD Args = %v, want 3 elements", cmdInst.Args)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate([]byte(`FROM alpine:latest
RUN echo test`)); err != nil {
		t.Errorf("Validate valid: %v", err)
	}

	if err := Validate([]byte(`INVALID instruction`)); err == nil {
		t.Error("Validate should return error for invalid Dockerfile")
	}
}

func TestListSupportedInstructions(t *testing.T) {
	instrs := ListSupportedInstructions()
	if len(instrs) != 18 {
		t.Errorf("len(instructions) = %d, want 18", len(instrs))
	}
	expected := map[string]bool{
		"FROM": true, "RUN": true, "CMD": true, "LABEL": true,
		"EXPOSE": true, "ENV": true, "ADD": true, "COPY": true,
		"ENTRYPOINT": true, "VOLUME": true, "USER": true,
		"WORKDIR": true, "ARG": true, "ONBUILD": true,
		"STOPSIGNAL": true, "HEALTHCHECK": true, "SHELL": true,
		"MAINTAINER": true,
	}
	for _, instr := range instrs {
		if !expected[instr] {
			t.Errorf("unexpected instruction: %s", instr)
		}
	}
}

func TestParseEntrypoint(t *testing.T) {
	content := []byte(`FROM alpine:latest
ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["--help"]
`)
	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := parser.GetStages()
	if stages[0].Instructions[0].Type != "ENTRYPOINT" {
		t.Errorf("Instr[0].Type = %q, want ENTRYPOINT", stages[0].Instructions[0].Type)
	}
}

func TestParseEnv(t *testing.T) {
	content := []byte(`FROM alpine:latest
ENV NODE_ENV=production
ENV PATH=/usr/local/bin:$PATH
`)
	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := parser.GetStages()
	if len(stages[0].Instructions) != 2 {
		t.Errorf("expected 2 ENV instructions, got %d", len(stages[0].Instructions))
	}
}

func TestParseMaintainer(t *testing.T) {
	content := []byte(`FROM alpine:latest
MAINTAINER test@example.com
`)
	parser := NewDokifileParser()
	if err := parser.Parse(content); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	stages := parser.GetStages()
	if stages[0].Instructions[0].Type != "MAINTAINER" {
		t.Errorf("Instr[0].Type = %q, want MAINTAINER", stages[0].Instructions[0].Type)
	}
}

func TestNewBuilder(t *testing.T) {
	b := NewBuilder(nil)
	if b == nil {
		t.Fatal("NewBuilder returned nil")
	}
}
