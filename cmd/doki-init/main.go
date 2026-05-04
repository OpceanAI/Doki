package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// doki-init is the minimal PID 1 for Doki microVMs.
// It runs inside the VM guest, sets up the environment, executes the container
// command, and communicates with the host via vsock.

func main() {
	mode := os.Getenv("DOKI_INIT_MODE")
	if mode == "" {
		mode = "default"
	}

	switch mode {
	case "default":
		runInit()
	case "oneshot":
		runOneshot()
	default:
		runInit()
	}
}

func runInit() {
	// Mount essential filesystems.
	mount("proc", "/proc", "proc", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
	mount("sysfs", "/sys", "sysfs", syscall.MS_NOSUID|syscall.MS_NOEXEC|syscall.MS_NODEV, "")
	mount("devtmpfs", "/dev", "devtmpfs", syscall.MS_NOSUID, "")
	mount("tmpfs", "/tmp", "tmpfs", 0, "")
	mount("tmpfs", "/run", "tmpfs", 0, "")

	// Create symlinks.
	os.Symlink("/proc/self/fd", "/dev/fd")
	os.Symlink("/proc/self/fd/0", "/dev/stdin")
	os.Symlink("/proc/self/fd/1", "/dev/stdout")
	os.Symlink("/proc/self/fd/2", "/dev/stderr")

	// Handle signals (forward to child).
	sigCh := make(chan os.Signal, 32)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGCHLD)

	// Start vsock listener for host communication.
	go startVsockServer()

	// Reap zombies.
	go func() {
		for range sigCh {
			// Reap children.
			var ws syscall.WaitStatus
			for {
				pid, err := syscall.Wait4(-1, &ws, syscall.WNOHANG, nil)
				if err != nil || pid <= 0 {
					break
				}
			}
		}
	}()

	// Run the container command.
	runCommand()
}

func runOneshot() {
	runCommand()
	os.Exit(0)
}

func runCommand() {
	args := []string{"/bin/sh"}
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}

	// Check for command via kernel cmdline.
	if cmdline := readCmdline(); cmdline != "" {
		if cmd := parseCmdlineCmd(cmdline); len(cmd) > 0 {
			args = cmd
		}
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(ws.ExitStatus())
			}
		}
		os.Exit(1)
	}
	os.Exit(0)
}

// ─── Vsock server (guest side) ────────────────────────────────────

func startVsockServer() {
	// Listen on vsock port 10000 (exec channel).
	ln, err := net.Listen("unix", "/dev/vsock")
	if err != nil {
		// On systems without vsock, use a dummy listener.
		return
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleVsockConn(conn)
	}
}

func handleVsockConn(conn net.Conn) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	for {
		var msg map[string]interface{}
		if err := dec.Decode(&msg); err != nil {
			if err != io.EOF {
				return
			}
			break
		}

		msgType, _ := msg["type"].(string)

		switch msgType {
		case "exec":
			handleVsockExec(msg, enc, conn)

		case "signal":
			sig, _ := msg["sig"].(string)
			sendSignalToChild(sig)

		case "health":
			enc.Encode(map[string]string{"type": "health", "data": "healthy"})

		case "exit":
			code, _ := msg["code"].(float64)
			os.Exit(int(code))
		}
	}
}

func handleVsockExec(msg map[string]interface{}, enc *json.Encoder, conn net.Conn) {
	cmdArgs := toStringSlice(msg["cmd"])
	envVars := toStringSlice(msg["env"])
	_ = envVars

	if len(cmdArgs) == 0 {
		cmdArgs = []string{"/bin/sh"}
	}

	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	cmd.Start()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				enc.Encode(map[string]interface{}{
					"type": "stdout",
					"data": string(buf[:n]),
				})
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				enc.Encode(map[string]interface{}{
					"type": "stderr",
					"data": string(buf[:n]),
				})
			}
			if err != nil {
				break
			}
		}
	}()

	cmd.Wait()
	exitCode := 0
	if ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
		exitCode = ws.ExitStatus()
	}
	enc.Encode(map[string]interface{}{
		"type": "exit",
		"code": exitCode,
	})
	_ = conn
}

// ─── Helpers ───────────────────────────────────────────────────────

func mount(source, target, fstype string, flags uintptr, data string) {
	syscall.Mount(source, target, fstype, flags, data)
}

func readCmdline() string {
	data, err := os.ReadFile("/proc/cmdline")
	if err != nil {
		return ""
	}
	return string(data)
}

func parseCmdlineCmd(cmdline string) []string {
	// Parse "doki.cmd=echo:hello" from kernel cmdline.
	for _, part := range splitStr(cmdline, " ") {
		if len(part) > 9 && part[:9] == "doki.cmd=" {
			val := part[9:]
			return splitStr(val, ":")
		}
	}
	return nil
}

func splitStr(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func toStringSlice(v interface{}) []string {
	if arr, ok := v.([]interface{}); ok {
		result := make([]string, len(arr))
		for i, item := range arr {
			result[i] = fmt.Sprint(item)
		}
		return result
	}
	return nil
}

func sendSignalToChild(sig string) {
	// Find child process and send signal.
	proc, err := os.FindProcess(1)
	if err == nil {
		switch sig {
		case "SIGTERM":
			proc.Signal(syscall.SIGTERM)
		case "SIGKILL":
			proc.Signal(syscall.SIGKILL)
		case "SIGINT":
			proc.Signal(syscall.SIGINT)
		case "SIGHUP":
			proc.Signal(syscall.SIGHUP)
		}
	}
}
