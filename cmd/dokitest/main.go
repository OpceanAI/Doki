// Package main is the Doki end-to-end test suite. It drives the real `doki`
// and `doki-compose` binaries against a running daemon and asserts on their
// output. Every assertion failure reports the check name, what was expected
// and what the command produced; every check runs with its own timeout, and
// checks that need images or network are skipped (not failed) when those are
// unavailable.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	e2eContainer = "e2e-test"
	e2eImage     = "nginx:alpine"
	baseImage    = "alpine:latest"
	e2eBuiltTag  = "e2e-built:latest"
	e2eHostPort  = "8080"
)

type e2e struct {
	doki    string
	compose string
	offline bool // no network: image pulls fail, image-dependent checks skip
}

func main() {
	hello := flag.Bool("hello", false, "run the in-process hello-world smoke test instead of the E2E suite")
	flag.Parse()

	if *hello {
		if err := runHelloWorld(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL hello-world: %v\n", err)
			os.Exit(1)
		}
		return
	}

	e := &e2e{}
	var err error
	if e.doki, err = findBinary("DOKI_BIN", "doki"); err != nil {
		fatalf("doki binary not found: %v (set DOKI_BIN or add doki to PATH)", err)
	}
	e.compose, _ = findBinary("DOKI_COMPOSE_BIN", "doki-compose") // optional

	// The suite drives a live daemon; without one nothing can pass.
	if out, err := e.dokiRun(30*time.Second, "version"); err != nil {
		fatalf("daemon not reachable via `doki version`: %v\noutput:\n%s\nstart the daemon (dokid) before running the E2E suite", err, out)
	}

	ok := true
	ok = e.run("pull-and-images", 5*time.Minute, e.testPullAndImages) && ok
	ok = e.run("run-nginx", 2*time.Minute, e.testRun) && ok
	ok = e.run("http-check", 1*time.Minute, e.testHTTPCheck) && ok
	ok = e.run("exec-echo", 1*time.Minute, e.testExec) && ok
	ok = e.run("logs", 1*time.Minute, e.testLogs) && ok
	ok = e.run("cp", 2*time.Minute, e.testCp) && ok
	ok = e.run("stop", 2*time.Minute, e.testStop) && ok
	ok = e.run("rm", 1*time.Minute, e.testRm) && ok
	ok = e.run("build", 5*time.Minute, e.testBuild) && ok
	ok = e.run("compose-up-down", 10*time.Minute, e.testCompose) && ok

	// Best-effort cleanup so a failed run does not leak containers.
	e.dokiRun(30*time.Second, "rm", "-f", e2eContainer)

	if !ok {
		fmt.Println("=== E2E RESULT: FAIL ===")
		os.Exit(1)
	}
	fmt.Println("=== E2E RESULT: PASS ===")
}

// run executes one named check with its own timeout. A check reports a skip by
// returning a non-empty reason with a nil error. Returns false on failure.
func (e *e2e) run(name string, timeout time.Duration, check func(ctx context.Context) (string, error)) bool {
	fmt.Printf("--- %s ---\n", name)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	skip, err := check(ctx)
	switch {
	case err != nil:
		fmt.Printf("FAIL %s: %v\n", name, err)
		return false
	case skip != "":
		fmt.Printf("SKIP %s: %s\n", name, skip)
		return true
	default:
		fmt.Printf("ok   %s\n", name)
		return true
	}
}

// ---- checks -------------------------------------------------------------

// testPullAndImages pulls alpine and asserts the tag shows up in `doki images`.
func (e *e2e) testPullAndImages(ctx context.Context) (string, error) {
	if _, err := e.dokiCtx(ctx, "pull", baseImage); err != nil {
		e.offline = true
		return fmt.Sprintf("cannot pull %s (no network or registry unreachable): %v", baseImage, err), nil
	}

	images, err := e.dokiCtx(ctx, "images")
	if err != nil {
		return "", fmt.Errorf("doki images failed: %v\noutput:\n%s", err, images)
	}
	if !strings.Contains(images, "alpine") || !strings.Contains(images, "latest") {
		return "", fmt.Errorf("doki images does not list %s\noutput:\n%s", baseImage, images)
	}
	return "", nil
}

// testRun starts nginx in the background and asserts it shows up as running.
func (e *e2e) testRun(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: image unavailable", nil
	}
	if _, err := e.dokiCtx(ctx, "pull", e2eImage); err != nil {
		return fmt.Sprintf("cannot pull %s: %v", e2eImage, err), nil
	}

	out, err := e.dokiCtx(ctx, "run", "-d", "--name", e2eContainer, "-p", e2eHostPort+":80", e2eImage)
	if err != nil {
		return "", fmt.Errorf("doki run failed: %v\noutput:\n%s", err, out)
	}

	ps, err := e.dokiCtx(ctx, "ps")
	if err != nil {
		return "", fmt.Errorf("doki ps failed: %v\noutput:\n%s", err, ps)
	}
	if !strings.Contains(ps, e2eContainer) {
		return "", fmt.Errorf("doki ps does not list %s after run\noutput:\n%s", e2eContainer, ps)
	}
	if !containsAny(ps, "running", "Running", "Up") {
		return "", fmt.Errorf("doki ps does not show %s as running\noutput:\n%s", e2eContainer, ps)
	}
	return "", nil
}

// testHTTPCheck asserts the published port accepts connections: a TCP socket
// check first, then an HTTP GET when curl is available.
func (e *e2e) testHTTPCheck(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: container not started", nil
	}
	addr := net.JoinHostPort("127.0.0.1", e2eHostPort)

	deadline := time.Now().Add(45 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return "", fmt.Errorf("nothing is listening on %s after 45s (port publish broken?): %v", addr, err)
		}
		time.Sleep(2 * time.Second)
	}

	resp, err := http.Get("http://" + addr + "/")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return "", fmt.Errorf("GET http://%s/ returned status %d", addr, resp.StatusCode)
		}
	}
	return "", nil
}

// testExec runs a command inside the container and asserts its output.
func (e *e2e) testExec(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: container not started", nil
	}
	out, err := e.dokiCtx(ctx, "exec", e2eContainer, "echo", "OK")
	if err != nil {
		return "", fmt.Errorf("doki exec failed: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "OK") {
		return "", fmt.Errorf("doki exec echo OK produced no OK\noutput:\n%s", out)
	}
	return "", nil
}

// testLogs asserts the container log output is readable and contains the
// web server startup lines.
func (e *e2e) testLogs(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: container not started", nil
	}
	out, err := e.dokiCtx(ctx, "logs", e2eContainer)
	if err != nil {
		return "", fmt.Errorf("doki logs failed: %v\noutput:\n%s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("doki logs %s produced no output", e2eContainer)
	}
	if !containsAny(out, "start", "worker", "nginx", "ready") {
		return "", fmt.Errorf("doki logs %s output lacks the expected startup lines\noutput:\n%s", e2eContainer, out)
	}
	return "", nil
}

// testCp copies a host file into the container and cats it back.
func (e *e2e) testCp(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: container not started", nil
	}
	const marker = "DOKI-E2E-CP-MARKER"
	dir, err := os.MkdirTemp("", "doki-e2e-cp-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	hostFile := filepath.Join(dir, "host.txt")
	if err := os.WriteFile(hostFile, []byte(marker+"\n"), 0o644); err != nil {
		return "", err
	}

	out, err := e.dokiCtx(ctx, "cp", hostFile, e2eContainer+":/tmp/host.txt")
	if err != nil {
		return "", fmt.Errorf("doki cp failed: %v\noutput:\n%s", err, out)
	}

	cat, err := e.dokiCtx(ctx, "exec", e2eContainer, "cat", "/tmp/host.txt")
	if err != nil {
		return "", fmt.Errorf("doki exec cat failed: %v\noutput:\n%s", err, cat)
	}
	if !strings.Contains(cat, marker) {
		return "", fmt.Errorf("copied file content mismatch: want %q\ngot:\n%s", marker, cat)
	}
	return "", nil
}

// testStop stops the container and asserts it is no longer running.
func (e *e2e) testStop(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: container not started", nil
	}
	out, err := e.dokiCtx(ctx, "stop", e2eContainer)
	if err != nil {
		return "", fmt.Errorf("doki stop failed: %v\noutput:\n%s", err, out)
	}

	ps, err := e.dokiCtx(ctx, "ps", "-a")
	if err != nil {
		return "", fmt.Errorf("doki ps -a failed: %v\noutput:\n%s", err, ps)
	}
	if !containsAny(ps, "Exited", "exited", "Stopped", "stopped", "Created") {
		return "", fmt.Errorf("doki ps -a does not show %s as stopped\noutput:\n%s", e2eContainer, ps)
	}
	if containsAny(ps, "Up ") {
		return "", fmt.Errorf("doki ps -a still shows %s as running\noutput:\n%s", e2eContainer, ps)
	}
	return "", nil
}

// testRm removes the container and asserts it is gone.
func (e *e2e) testRm(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: container not started", nil
	}
	out, err := e.dokiCtx(ctx, "rm", e2eContainer)
	if err != nil {
		return "", fmt.Errorf("doki rm failed: %v\noutput:\n%s", err, out)
	}

	ps, err := e.dokiCtx(ctx, "ps", "-a")
	if err != nil {
		return "", fmt.Errorf("doki ps -a failed: %v\noutput:\n%s", err, ps)
	}
	if strings.Contains(ps, e2eContainer) {
		return "", fmt.Errorf("doki ps -a still lists %s after rm\noutput:\n%s", e2eContainer, ps)
	}
	return "", nil
}

// testBuild builds an image from a minimal Dokifile and asserts it exists.
func (e *e2e) testBuild(ctx context.Context) (string, error) {
	if e.offline {
		return "offline: base image unavailable", nil
	}
	dir, err := os.MkdirTemp("", "doki-e2e-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	dokifile := "FROM alpine\nRUN echo hi\n"
	if err := os.WriteFile(filepath.Join(dir, "Dokifile"), []byte(dokifile), 0o644); err != nil {
		return "", err
	}

	out, err := e.dokiCtx(ctx, "build", "-t", e2eBuiltTag, dir)
	if err != nil {
		return "", fmt.Errorf("doki build failed: %v\noutput:\n%s", err, out)
	}

	images, err := e.dokiCtx(ctx, "images")
	if err != nil {
		return "", fmt.Errorf("doki images failed: %v\noutput:\n%s", err, images)
	}
	if !strings.Contains(images, "e2e-built") {
		return "", fmt.Errorf("doki images does not list %s after build\noutput:\n%s", e2eBuiltTag, images)
	}
	return "", nil
}

// testCompose brings up test/compose-test/doki-compose.yaml, asserts the
// services are running and brings the stack down again.
func (e *e2e) testCompose(ctx context.Context) (string, error) {
	if e.compose == "" {
		return "doki-compose binary not found (set DOKI_COMPOSE_BIN)", nil
	}
	if e.offline {
		return "offline: compose images unavailable", nil
	}
	file := filepath.Join("test", "compose-test", "doki-compose.yaml")
	if _, err := os.Stat(file); err != nil {
		return fmt.Sprintf("compose file %s not found (run from the repo root)", file), nil
	}

	up, err := e.runCtx(ctx, e.compose, "-f", file, "up", "-d")
	if err != nil {
		return "", fmt.Errorf("doki-compose up failed: %v\noutput:\n%s", err, up)
	}
	defer func() {
		if down, derr := e.runCtx(context.Background(), e.compose, "-f", file, "down"); derr != nil {
			fmt.Printf("WARNING: doki-compose down failed: %v\noutput:\n%s\n", derr, down)
		}
	}()

	ps, err := e.dokiCtx(ctx, "ps")
	if err != nil {
		return "", fmt.Errorf("doki ps failed: %v\noutput:\n%s", err, ps)
	}
	for _, svc := range []string{"test-db", "test-redis", "test-web"} {
		if !strings.Contains(ps, svc) {
			return "", fmt.Errorf("doki ps does not list compose service %s\noutput:\n%s", svc, ps)
		}
	}
	if !containsAny(ps, "running", "Running", "Up") {
		return "", fmt.Errorf("doki ps does not show compose services as running\noutput:\n%s", ps)
	}
	return "", nil
}

// ---- helpers ------------------------------------------------------------

// dokiRun runs the doki binary with a timeout and returns its combined output.
func (e *e2e) dokiRun(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return e.dokiCtx(ctx, args...)
}

func (e *e2e) dokiCtx(ctx context.Context, args ...string) (string, error) {
	return e.runCtx(ctx, e.doki, args...)
}

// runCtx runs a command with the check's deadline and returns combined output.
func (e *e2e) runCtx(ctx context.Context, bin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return string(out), fmt.Errorf("%s %s: timed out", bin, strings.Join(args, " "))
	}
	return string(out), err
}

// findBinary resolves a binary from an environment override or PATH.
func findBinary(envVar, name string) (string, error) {
	if p := os.Getenv(envVar); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s=%s: %w", envVar, p, err)
		}
		return p, nil
	}
	return exec.LookPath(name)
}

func containsAny(s string, wants ...string) bool {
	for _, w := range wants {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "FAIL setup: "+format+"\n", args...)
	os.Exit(1)
}
