package cli

import (
	"testing"
)

func TestParseRunFlagsBasic(t *testing.T) {
	image, cmd, flags := ParseRunFlags([]string{"alpine", "echo", "hello"})
	if image != "alpine" {
		t.Errorf("image = %q, want alpine", image)
	}
	if len(cmd) != 2 || cmd[0] != "echo" || cmd[1] != "hello" {
		t.Errorf("cmd = %v, want [echo hello]", cmd)
	}
	_ = flags
}

func TestParseRunFlagsDetach(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-d", "nginx:alpine"})
	if !flags.Detach {
		t.Error("-d should set Detach")
	}
}

func TestParseRunFlagsInteractive(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-i", "alpine", "sh"})
	if !flags.Interactive {
		t.Error("-i should set Interactive")
	}
}

func TestParseRunFlagsTTY(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-t", "alpine", "sh"})
	if !flags.TTY {
		t.Error("-t should set TTY")
	}
}

func TestParseRunFlagsName(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--name", "mycontainer", "alpine"})
	if flags.Name != "mycontainer" {
		t.Errorf("Name = %q, want mycontainer", flags.Name)
	}
}

func TestParseRunFlagsNetwork(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--network", "host", "alpine"})
	if flags.Network != "host" {
		t.Errorf("Network = %q, want host", flags.Network)
	}
}

func TestParseRunFlagsRestartPolicy(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--restart", "always", "alpine"})
	if flags.RestartPolicy != "always" {
		t.Errorf("RestartPolicy = %q, want always", flags.RestartPolicy)
	}
}

func TestParseRunFlagsPort(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-p", "8080:80", "nginx"})
	if len(flags.Ports) != 1 || flags.Ports[0] != "8080:80" {
		t.Errorf("Ports = %v, want [8080:80]", flags.Ports)
	}
}

func TestParseRunFlagsVolume(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-v", "/data:/mnt", "alpine"})
	if len(flags.Volumes) != 1 || flags.Volumes[0] != "/data:/mnt" {
		t.Errorf("Volumes = %v, want [/data:/mnt]", flags.Volumes)
	}
}

func TestParseRunFlagsEnv(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-e", "NODE_ENV=production", "-e", "PORT=3000", "alpine"})
	if len(flags.Env) != 2 {
		t.Fatalf("len(Env) = %d, want 2", len(flags.Env))
	}
	if flags.Env[0] != "NODE_ENV=production" {
		t.Errorf("Env[0] = %q", flags.Env[0])
	}
	if flags.Env[1] != "PORT=3000" {
		t.Errorf("Env[1] = %q", flags.Env[1])
	}
}

func TestParseRunFlagsWorkdir(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-w", "/app", "alpine"})
	if flags.Workdir != "/app" {
		t.Errorf("Workdir = %q, want /app", flags.Workdir)
	}
}

func TestParseRunFlagsUser(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-u", "1000:1000", "alpine"})
	if flags.User != "1000:1000" {
		t.Errorf("User = %q, want 1000:1000", flags.User)
	}
}

func TestParseRunFlagsMemory(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-m", "256m", "alpine"})
	if flags.Memory != 256*1024*1024 {
		t.Errorf("Memory = %d, want 268435456", flags.Memory)
	}
}

func TestParseRunFlagsMemoryWithM(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--memory", "512m", "alpine"})
	if flags.Memory != 512*1024*1024 {
		t.Errorf("Memory = %d, want 536870912", flags.Memory)
	}
}

func TestParseRunFlagsMemoryWithG(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--memory", "1g", "alpine"})
	if flags.Memory != 1*1024*1024*1024 {
		t.Errorf("Memory = %d, want 1073741824", flags.Memory)
	}
}

func TestParseRunFlagsCPUs(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--cpus", "1.5", "alpine"})
	if flags.NanoCPUs != 1500000000 {
		t.Errorf("NanoCPUs = %d, want 1500000000", flags.NanoCPUs)
	}
}

func TestParseRunFlagsCPUShares(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--cpu-shares", "512", "alpine"})
	if flags.CPUShares != 512 {
		t.Errorf("CPUShares = %d, want 512", flags.CPUShares)
	}
}

func TestParseRunFlagsPrivileged(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--privileged", "alpine"})
	if !flags.Privileged {
		t.Error("--privileged should set Privileged")
	}
}

func TestParseRunFlagsReadOnly(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--read-only", "alpine"})
	if !flags.ReadOnly {
		t.Error("--read-only should set ReadOnly")
	}
}

func TestParseRunFlagsInit(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--init", "alpine"})
	if !flags.Init {
		t.Error("--init should set Init")
	}
}

func TestParseRunFlagsRM(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--rm", "alpine", "echo", "test"})
	if !flags.RM {
		t.Error("--rm should set RM")
	}
}

func TestParseRunFlagsHostname(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-h", "myhost", "alpine"})
	if flags.Hostname != "myhost" {
		t.Errorf("Hostname = %q, want myhost", flags.Hostname)
	}
}

func TestParseRunFlagsEntrypoint(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--entrypoint", "/custom-init.sh", "alpine"})
	if flags.Entrypoint != "/custom-init.sh" {
		t.Errorf("Entrypoint = %q, want /custom-init.sh", flags.Entrypoint)
	}
}

func TestParseRunFlagsStopSignal(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--stop-signal", "SIGINT", "alpine"})
	if flags.StopSignal != "SIGINT" {
		t.Errorf("StopSignal = %q, want SIGINT", flags.StopSignal)
	}
}

func TestParseRunFlagsDNS(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--dns", "8.8.8.8", "--dns", "8.8.4.4", "alpine"})
	if len(flags.DNS) != 2 || flags.DNS[0] != "8.8.8.8" || flags.DNS[1] != "8.8.4.4" {
		t.Errorf("DNS = %v, want [8.8.8.8 8.8.4.4]", flags.DNS)
	}
}

func TestParseRunFlagsExtraHosts(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--add-host", "myhost:192.168.1.1", "alpine"})
	if len(flags.ExtraHosts) != 1 || flags.ExtraHosts[0] != "myhost:192.168.1.1" {
		t.Errorf("ExtraHosts = %v", flags.ExtraHosts)
	}
}

func TestParseRunFlagsCapAdd(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--cap-add", "NET_ADMIN", "alpine"})
	if len(flags.CapAdd) != 1 || flags.CapAdd[0] != "NET_ADMIN" {
		t.Errorf("CapAdd = %v", flags.CapAdd)
	}
}

func TestParseRunFlagsCapDrop(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--cap-drop", "ALL", "alpine"})
	if len(flags.CapDrop) != 1 || flags.CapDrop[0] != "ALL" {
		t.Errorf("CapDrop = %v", flags.CapDrop)
	}
}

func TestParseRunFlagsPublishAll(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-P", "alpine"})
	if !flags.PublishAll {
		t.Error("-P should set PublishAll")
	}
}

func TestParseRunFlagsShmSize(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--shm-size", "64m", "alpine"})
	if flags.ShmSize != 64*1024*1024 {
		t.Errorf("ShmSize = %d, want 67108864", flags.ShmSize)
	}
}

func TestParseRunFlagsPullPolicy(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--pull", "always", "alpine"})
	if flags.Pull != "always" {
		t.Errorf("Pull = %q, want always", flags.Pull)
	}
}

func TestParseRunFlagsPlatform(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--platform", "linux/arm64", "alpine"})
	if flags.Platform != "linux/arm64" {
		t.Errorf("Platform = %q, want linux/arm64", flags.Platform)
	}
}

func TestParseRunFlagsLogDriver(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"--log-driver", "json-file", "alpine"})
	if flags.LogDriver != "json-file" {
		t.Errorf("LogDriver = %q, want json-file", flags.LogDriver)
	}
}

func TestParseRunFlagsLabel(t *testing.T) {
	_, _, flags := ParseRunFlags([]string{"-l", "env=production", "alpine"})
	if flags.Labels["env"] != "production" {
		t.Errorf("Labels[env] = %q, want production", flags.Labels["env"])
	}
}

func TestParseRunFlagsNoImage(t *testing.T) {
	image, _, _ := ParseRunFlags([]string{"-d", "--name", "test"})
	if image != "" {
		t.Errorf("image = %q, want empty", image)
	}
}

func TestParseRunFlagsDoubleDash(t *testing.T) {
	image, cmd, _ := ParseRunFlags([]string{"alpine", "--", "echo", "hello", "--verbose"})
	if image != "alpine" {
		t.Errorf("image = %q, want alpine", image)
	}
	if len(cmd) != 3 {
		t.Errorf("cmd len = %d, want 3: %v", len(cmd), cmd)
	}
	if cmd[0] != "echo" || cmd[1] != "hello" || cmd[2] != "--verbose" {
		t.Errorf("cmd = %v, want [echo hello --verbose]", cmd)
	}
}

func TestParseRunFlagsCombined(t *testing.T) {
	image, cmd, flags := ParseRunFlags([]string{
		"-d", "--name", "web", "-p", "8080:80",
		"-e", "ENV=prod", "-v", "/data:/app",
		"--restart", "always", "-m", "256m",
		"nginx:alpine", "nginx", "-g", "daemon off;",
	})

	if image != "nginx:alpine" {
		t.Errorf("image = %q", image)
	}
	if len(cmd) != 3 {
		t.Errorf("cmd = %v", cmd)
	}
	if flags.Name != "web" {
		t.Errorf("Name = %q", flags.Name)
	}
	if !flags.Detach {
		t.Error("Detach not set")
	}
	if flags.RestartPolicy != "always" {
		t.Errorf("RestartPolicy = %q", flags.RestartPolicy)
	}
	if flags.Memory != 256*1024*1024 {
		t.Errorf("Memory = %d", flags.Memory)
	}
	if len(flags.Ports) != 1 {
		t.Errorf("Ports len = %d", len(flags.Ports))
	}
	if len(flags.Env) != 1 {
		t.Errorf("Env len = %d", len(flags.Env))
	}
	if len(flags.Volumes) != 1 {
		t.Errorf("Volumes len = %d", len(flags.Volumes))
	}
}

func TestNewDokiCLI(t *testing.T) {
	c := New("/tmp/test.sock")
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.socket != "/tmp/test.sock" {
		t.Errorf("socket = %q, want /tmp/test.sock", c.socket)
	}
	if c.client == nil {
		t.Error("client is nil")
	}
}

func TestNewDokiCLIDefault(t *testing.T) {
	c := New("")
	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.socket == "" {
		t.Error("socket should not be empty")
	}
}
