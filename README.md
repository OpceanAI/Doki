# The Universal Container Engine

Rootless containers for everywhere Docker can't reach.

<p>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"></a>
  <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-doki--init-black?style=flat&logo=rust&logoColor=white" alt="Rust"></a>
  <a href="https://github.com/OpceanAI/Doki/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-555?style=flat" alt="License"></a>
  <a href="https://github.com/OpceanAI/Doki/releases"><img src="https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat&color=6366F1" alt="Downloads"></a>
  <a href="https://github.com/OpceanAI/Doki/stargazers"><img src="https://img.shields.io/github/stars/OpceanAI/Doki?style=flat&color=6366F1" alt="Stars"></a>
</p>

Docker and Podman compatible API -- OCI native -- Kubernetes CRI-ready.
Runs on Linux, macOS, and Android via Termux -- ARM64, ARMv7, x86_64.
Rootless-first architecture -- No daemon required -- Hardware-level microVM isolation.

---

## Overview

Doki is a container engine designed for every Linux kernel, from Android phones to cloud servers. It works without root, without systemd, and without a hypervisor. When your hardware offers more -- KVM, Android's built-in hypervisors, Linux namespaces -- Doki scales up its isolation automatically.

| Metric | Value |
|:-------|:------|
| **Version** | v0.11.1 |
| **Binary size** | 13 MB |
| **Memory (idle)** | 12 MB |
| **Start time** | <15ms |
| **Platforms** | Linux, macOS, Android (Termux) |
| **Architectures** | ARM64, ARMv7, x86_64 |
| **Runtime deps** | Zero |

### Binary Availability by Platform (v0.11.1)

| Platform | doki | dokid | doki-compose | doki-init | doki-kube | doki-kubectl |
|:---------|:----:|:-----:|:------------:|:---------:|:---------:|:------------:|
| Linux ARM64 | Yes | Yes | Yes | Yes | Yes | Yes |
| Linux ARMv7 | Yes | Yes | Yes | Yes | Yes | Yes |
| Linux x86_64 | Yes | Yes | Yes | Yes | Yes | Yes |
| Android ARM64 (Termux) | Yes | Yes | Yes | Yes | Yes | Yes |
| Android ARMv7 (Termux) | Yes | Yes | Yes | Yes | Yes | Yes |
| macOS ARM64 (Apple Silicon) | Yes | -- | -- | -- | Yes | Yes |
| macOS x86_64 (Intel) | Yes | -- | -- | -- | Yes | Yes |

**Note:** Android ARMv7 binaries are built with `GOOS=linux` (Go 1.22+ requires external linker for `GOOS=android` on 32-bit ARM). The binaries run via proot; Android detection uses filesystem probes.

`dokid`, `doki-compose`, and `doki-init` are Linux/Android only -- they depend on Linux namespaces, cgroups v2, and overlayfs syscalls. On macOS, `doki` runs in `ModeNative` only and connects to a remote daemon over the network if needed. `doki-kube` and `doki-kubectl` are available on macOS as client binaries.

---

## Comparison

| Metric | Doki | Docker | Podman | containerd |
|:-------|:----:|:------:|:------:|:----------:|
| Binary size | **13 MB** | 58 MB | 45 MB | 42 MB |
| Memory (idle) | **12 MB** | 85 MB | 60 MB | 55 MB |
| Start time | **<15ms** | ~50ms | ~30ms | ~40ms |
| Android support | **Yes** | No | No | No |
| Root required | **No** | Yes | Optional | Yes |
| Daemon required | **No** | Yes | No | Yes |
| microVM isolation | **Yes** | No | No | No |
| Zero dependencies | **Yes** | No | No | No |

---

## What Doki Replaces

| Instead of | Use Doki | Because |
|:-----------|:---------|:--------|
| Docker Desktop | `dokid` + `doki` | Same API, no VM overhead, works on Android |
| Podman | `dokid` + `doki` | Same pod abstraction, plus microVM isolation |
| containerd + crictl | `dokid` as CRI | Single binary instead of 3 daemons |
| Docker Compose | `doki-compose` | Same YAML, same commands, same workflow |
| Kubernetes (small deploys) | `doki kube play` | Run K8s YAML without a cluster |
| Lima / Colima (macOS) | `dokid` | Native container daemon, no Linux VM needed |
| Termux proot-distro | `doki run` | Actual OCI images instead of chroot tarballs |
| kubectl + minikube | `doki-kubectl` + `doki-kube` | Single-binary K8s control plane with real CRI |

---

## Features

### Android Native

The only container engine that runs on Android via Termux without root. Designed for the constraints of mobile operating systems from the ground up. Host network namespace via proot fallback when `/proc/sys/net` is unavailable.

### Rootless by Default

Works as a regular user. Scales to root or microVM isolation when available. No privilege escalation required for basic operations.

### Docker Compatible

Same REST API v1.54. Drop-in replacement for Docker CLI and SDKs. docker-compose, docker-py, CI/CD pipelines all work without modification.

### Ultra Lightweight

13MB binary, 12MB RAM idle. 4x smaller than Docker, 7x less memory.

### 12 Isolation Levels

From WASM sandboxes to pKVM hardware isolation. Auto-selected at runtime based on available hardware. A mode for every device: phones without root, servers with KVM, Chromebooks with pKVM, or laptops needing x86 emulation on ARM. New in v0.11: pKVM/Microdroid detection and macOS VZ backend.

### Compose Support

Full Compose spec: networks, volumes, secrets, health checks, depends_on with 60s poll, 30+ fields including shm_size, pids_limit, ulimits. Healthcheck execution engine runs periodic probes and reports container health status end-to-end.

### OCI Compliant

Push and pull to any OCI registry. Multi-architecture auto-resolution. Compatible with Docker Hub, GHCR, ECR, GCR, Quay, GitLab, Harbor.

### Kubernetes 100%

Complete Kubernetes 1.32 control plane in a single binary: apiserver, kubelet with real CRI gRPC, scheduler, 10 functional controllers, kube-proxy (iptables/nftables/userspace modes), and CoreDNS. SQLiteStore with crash-safe persistence. kubectl-compatible CLI.

### DokiLink Mesh

Peer-to-peer multi-host container networking. NAT traversal via STUN (RFC 8489) with TCP simultaneous open hole punching and TURN relay fallback. DHT peer discovery (Kademlia, 160-bit, k=8, alpha=3). mDNS LAN discovery with 90-second expiry and cleanup loop. TLS 1.3 encryption with NaCl secretbox option. Ed25519 identity with TOFU trust model.

### Podman API v5

39 endpoints compatible with podman-remote clients. Pod, secret, and manifest management. Mounted alongside the Docker API on the same socket with shared TLS, middleware, and rate limiting.

### macOS Native Virtualization

VZ backend via cgo bridge to Virtualization.framework for macOS 11+. QEMU backend as fallback on Intel Macs or where VZ is unavailable. Sandbox backend for lightweight isolation without VM overhead.

### Diagnostics

`doki deps` tool for host dependency verification with `ls`, `check` (CI gate), `go` (Go module deps), and `install` (best-effort via detected package manager). `doki doctor` for environment health checks.

---

## Quick Start

### Install

```bash
curl -sL https://doki.opceanai.com | sh
```

### First Run

```bash
# Start the daemon
dokid &

# Pull and run
doki pull alpine
doki run alpine echo "Hello from Doki"

# Check what's running
doki ps
doki images
```

### Use with Docker CLI

```bash
export DOCKER_HOST=unix:///var/run/doki.sock
docker ps
docker images
docker run alpine echo "via docker cli"
docker-compose up
```

### Use with Docker SDKs

```python
import docker
client = docker.DockerClient(base_url="unix:///var/run/doki.sock")
client.containers.run("alpine", "echo hello")
```

```javascript
const Docker = require('dockerode');
const docker = new Docker({ socketPath: '/var/run/doki.sock' });
docker.listContainers().then(console.log);
```

### Use with Kubernetes

```bash
# Start the K8s control plane
doki kube play my-app.yaml

# Manage with kubectl-compatible CLI
doki-kubectl get pods
doki-kubectl apply -f deployment.yaml
doki-kubectl describe pod web-abc123
doki-kubectl logs web-abc123
```

---

## Binaries

| Binary | Size | Description |
|:-------|:----:|:------------|
| **doki** | 6.7 MB | CLI with 108+ commands. Connects to daemon via Unix socket |
| **dokid** | 9.2 MB | Daemon. Docker Engine API v1.54 + Podman API v5 over Unix socket |
| **doki-compose** | 7.6 MB | Compose engine with watch, publish, healthcheck execution, and full spec support |
| **doki-init** | 2.9 MB | PID 1 for microVM guests (Go). Rust variant available in source |
| **doki-kube** | 8.1 MB | Kubernetes control plane (apiserver, kubelet, scheduler, controllers, kube-proxy, CoreDNS) |
| **doki-kubectl** | 4.3 MB | kubectl-compatible CLI for managing Kubernetes resources |

---

## Architecture

### Pipeline

When Doki runs a container, it goes through this pipeline:

1. **Image Resolution** -- Parse reference, contact registry, authenticate, resolve manifest for current architecture, download layers
2. **Rootfs Construction** -- Extract layers in order, build complete container filesystem with path traversal protection
3. **Execution Mode Selection** -- Probe system and select best runner from 12 available modes: WASM, pKVM, microVM, sysbox, namespaces, gVisor, FEX, QEMU, proot, legacy32, chroot, or native
4. **Process Execution** -- Execute container command within chosen isolation context with environment variables applied
5. **Lifecycle Management** -- Monitor process, record exit codes, write logs, execute health checks, enforce restart policies

### Isolation Levels

Doki selects the strongest isolation mode available on your hardware. Each mode exists for a specific use case:

| Level | Mode | Isolation | Overhead | Why / When |
|:-----:|:-----|:----------|:---------|:-----------|
| **12** | WASM | Sandbox (user-space) | Minimal | Run WASI/Wasm containers for untrusted code. No syscalls leak to host. Use for plugins, serverless functions, or polyglot microservices |
| **11** | pKVM/Microdroid | Hardware-level (vm) | 5-20 MB RAM | Android 15+ protected VM. Google's pKVM isolates workloads from the host OS and from each other. Use for sensitive compute on Chromebooks/phones |
| **10** | MicroVM | Hardware-level (vm) | 5-20 MB RAM | KVM, Gunyah, GenieZone, Halla hypervisors. Full hardware isolation with micro-second boot. Use when you need VM-level security with container speed |
| **9** | Sysbox | Kernel-level (DinD) | Moderate | Rootless Docker-in-Docker via sysbox-runc. Use when you need to run a full Docker daemon inside a container (CI runners, build farms) |
| **8** | Namespaces | Kernel-level | Negligible | Standard Linux namespace isolation. Use on servers with root access. Best performance for trusted multi-tenant workloads |
| **7** | gVisor | User-space kernel | ~20% CPU | Google's runsc intercepts syscalls at the user-space boundary. Use when you want defense-in-depth without a VM -- 70% of syscalls never reach the host |
| **6** | FEX-Emu | Emulation (x86 on ARM) | ~30% CPU | FEXInterpreter or Box64. Runs x86/x86_64 binaries on ARM64 without recompilation. Use for legacy x86 containers on Apple Silicon or ARM servers |
| **5** | QEMU User | Emulation (cross-arch) | ~50% CPU | QEMU user-mode for any guest arch. Use when you need to run containers built for a different architecture (e.g., arm32 on arm64, or any arch on any arch) |
| **4** | Proot | Userspace (ptrace) | ~10% CPU | Ptrace-based chroot without root. Default on Android/Termux. Use on devices where you lack root and namespaces -- phones, tablets, ChromeOS Linux |
| **3** | Legacy32 | Dual-arch compat | Negligible | Run ARMv7 containers on ARM64 kernels via binfmt_misc and multiarch support. Use when your workload ships only as 32-bit ARM |
| **2** | Chroot | Filesystem-level | Minimal | Lightweight filesystem isolation via chroot. Use for quick testing, build stages, or when every other mode is unavailable |
| **1** | Native | None | Zero | Direct host execution. Always available as fallback. Use when you trust the workload and want zero overhead |

### Isolation Level Detection

The runner registry in `pkg/runtime/registry.go` probes the host and selects the strongest mode that works. Probe order (top-down, first that passes wins):

| Level | Mode | Detection probe |
|:-----:|:-----|:----------------|
| 12 | WASM | `which wasmedge` or `which iwasm` |
| 11 | pKVM/Microdroid | `/dev/kvm` readable + Android 15+ |
| 10 | MicroVM | `/dev/kvm` readable + `crosvm`/`firecracker` in `$PATH` |
| 9 | Sysbox | `sysbox-runc` in `$PATH` |
| 8 | Namespaces | `unshare --user --map-root-user true` exits 0 |
| 7 | gVisor | `runsc` in `$PATH` |
| 6 | FEX-Emu | `FEXInterpreter` or `box64` in `$PATH` |
| 5 | QEMU User | `qemu-aarch64-static` / `qemu-x86_64-static` etc. in `$PATH` |
| 4 | Proot | `proot` in `$PATH` (or shipped) |
| 3 | Legacy32 | `binfmt_misc` registered + multiarch qemu |
| 2 | Chroot | always (uses `chroot(2)`) |
| 1 | Native | always (no isolation) |

Override with `doki run --runtime <mode>`:

```bash
doki run --runtime proot alpine echo "always proot"
doki run --runtime native alpine echo "no isolation"
doki run --runtime wasm wasi-example.wasm
```

### MicroVM Support

DokiVM provides hardware-level isolation via lightweight virtual machines.

| Manufacturer | Chip Series | Hypervisor | VMM | Generation |
|:-------------|:------------|:-----------|:----|:-----------|
| Qualcomm | Snapdragon 8 Gen 1/2/3/4 | Gunyah | crosvm | 2022+ |
| MediaTek | Dimensity 7200/8200/9200/9300 | GenieZone | crosvm | 2023+ |
| Samsung | Exynos 2200/2400 | Halla | crosvm | 2022+ |
| Google | Tensor G1/G2/G3/G4 | KVM | crosvm | 2021+ |
| Intel | Core / Xeon | KVM | Firecracker | All KVM-capable |
| AMD | Ryzen / EPYC | KVM | Firecracker | All KVM-capable |

### macOS Virtualization

On macOS, Doki provides three VM backends:

| Backend | Technology | Requirements | Best For |
|:--------|:-----------|:-------------|:---------|
| **VZ** | Virtualization.framework via cgo/ObjC bridge | macOS 11+, Apple Silicon | Native performance, Rosetta support, minimal overhead |
| **QEMU** | QEMU with HVF accelerator | macOS 10.15+, Intel or Apple Silicon | Fallback when VZ unavailable, x86 emulation on ARM |
| **Sandbox** | macOS sandbox-exec profile | macOS 10.7+ | Lightweight isolation without full VM overhead |

The VZ backend uses `VZVirtualMachineConfiguration`, `VZLinuxBootLoader`, `VZVirtioFileSystemDevice` with shared directories, and `VZBridgedNetworkDevice`/`VZNATNetworkDevice`. Build tag `darwin && cgo`, `CGO_ENABLED=1` required.

---

## CLI

Doki provides **108 commands** across 8 categories.

### Container Management

| Command | Description |
|:--------|:------------|
| `doki run` | Create and start a container (80+ flags) |
| `doki ps` | List containers |
| `doki create` | Create without starting |
| `doki start` | Start stopped containers |
| `doki stop` | Gracefully stop containers |
| `doki restart` | Stop and start containers |
| `doki kill` | Send signal to containers |
| `doki rm` | Remove containers |
| `doki exec` | Run command in running container |
| `doki logs` | Fetch container logs (streaming support) |
| `doki stats` | Live resource statistics |
| `doki top` | Display container processes |
| `doki inspect` | Detailed container info |
| `doki build` | Build image from Dokifile |
| `doki commit` | Create image from container |
| `doki attach` | Attach to container I/O |
| `doki wait` | Block until exit, return code |
| `doki cp` | Copy files between host and container |

### Image Management

| Command | Description |
|:--------|:------------|
| `doki pull` | Pull from any OCI registry (multi-arch auto-resolve) |
| `doki push` | Push to any OCI registry |
| `doki images` | List images with sizes |
| `doki rmi` | Remove images |
| `doki tag` | Tag an image |
| `doki build` | Build from Dokifile (18 instructions, multi-stage) |
| `doki login` / `doki logout` | Registry authentication |
| `doki search` | Search Docker Hub |

### Network, Volume, System

| Network | Volume | System |
|:--------|:-------|:-------|
| `doki network ls` | `doki volume ls` | `doki info` |
| `doki network create` | `doki volume create` | `doki version` |
| `doki network rm` | `doki volume rm` | `doki system df` |
| `doki network inspect` | `doki volume inspect` | `doki system prune` |
| `doki network connect` | `doki volume prune` | `doki system events` |
| `doki network disconnect` | | `doki ping` |
| `doki network prune` | | |

### Podman and Kubernetes

| Podman | Kubernetes |
|:-------|:-----------|
| `doki pod create/ps/rm/start/stop` | `doki kube play` |
| `doki generate kube` | `doki kube down` |
| `doki play kube` | `doki kube generate` |
| `doki auto-update` | `doki apply -f` |
| `doki unshare` / `untag` | |
| `doki mount` / `unmount` | |
| `doki healthcheck` | |

### DokiLink Mesh

| Command | Description |
|:--------|:------------|
| `doki mesh status` | Show install ID and Ed25519 public key |
| `doki mesh ls` | List known peers |
| `doki link add <name> <addr> --pub <key>` | Add a static peer |
| `doki link rm <name>` | Remove a static peer |

### Diagnostics

| Command | Description |
|:--------|:------------|
| `doki doctor` | Verify host environment and dependencies |
| `doki deps ls` | List all system dependencies with status |
| `doki deps check` | CI gate: exit non-zero if required deps missing |
| `doki deps go` | Audit Go module dependencies |
| `doki deps install <name>` | Best-effort install via detected package manager |

---

## Dokifile Builder

Doki reads Dokifiles (or standard Dockerfiles) and builds OCI-compatible images. The parser supports all 18 Dockerfile instructions, multi-stage builds, heredocs, and parser directives.

### Supported Instructions

```
FROM      RUN       CMD       LABEL     EXPOSE    ENV
ADD       COPY      ENTRYPOINT VOLUME   USER      WORKDIR
ARG       ONBUILD   STOPSIGNAL HEALTHCHECK SHELL  MAINTAINER
```

### Example

```dockerfile
FROM alpine:latest AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY . .
RUN gcc -static -o app main.c

FROM alpine:latest
COPY --from=builder /build/app /usr/local/bin/app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q --spider http://localhost:8080/ || exit 1
USER nobody
CMD ["/usr/local/bin/app"]
```

---

## Compose

Full Compose Specification support for multi-container applications.

### Supported Features

| Feature | Description |
|:--------|:------------|
| `services` | Container definitions with full configuration |
| `networks` | Custom bridge/overlay networks |
| `volumes` | Persistent storage with driver options |
| `secrets` | Sensitive data injection with long syntax |
| `depends_on` | Startup ordering: `service_started`, `service_healthy` (60s poll), `service_completed_successfully` |
| `healthcheck` | Health probes per service with real periodic execution engine |
| `deploy` | Resource limits (`cpus`, `memory`), `replicas`, `restart_policy` |
| `profiles` | Conditional service activation |
| `extends` | Service inheritance |
| `include` | Multi-file composition |
| `watch` | File watching via fsnotify for hot-reload during development |
| `publish` | Service mesh integration for compose-based deployments |
| Long syntax | Ports, volumes, devices, blkio_config, ulimits |

### Example

```yaml
name: production-stack

services:
  web:
    image: nginx:alpine
    ports: ["80:80", "443:443"]
    volumes:
      - web-data:/usr/share/nginx/html
    depends_on:
      api:
        condition: service_healthy
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 256M
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 10s
      retries: 3

  api:
    image: python:3-alpine
    command: uvicorn main:app --host 0.0.0.0
    environment:
      DATABASE_URL: postgresql://user:pass@db:5432/app
    depends_on:
      db:
        condition: service_started

  db:
    image: postgres:alpine
    volumes:
      - db-data:/var/lib/postgresql/data
    secrets:
      - db-password
```

---

## REST API

Doki exposes the **Docker Engine API v1.54** and **Podman libpod API v5** over Unix sockets. Both APIs share the same server, inheriting TLS, middleware, and rate limiting.

### Docker Engine API -- Containers (16 endpoints)

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/containers/json` | List containers |
| `POST` | `/containers/create` | Create container |
| `GET` | `/containers/{id}/json` | Inspect container |
| `POST` | `/containers/{id}/start` | Start container |
| `POST` | `/containers/{id}/stop` | Stop container |
| `POST` | `/containers/{id}/restart` | Restart container |
| `POST` | `/containers/{id}/kill` | Kill container |
| `DELETE` | `/containers/{id}` | Remove container |
| `GET` | `/containers/{id}/logs` | Fetch logs |
| `POST` | `/containers/{id}/exec` | Create exec instance |
| `POST` | `/containers/{id}/attach` | Attach to container |
| `POST` | `/containers/prune` | Remove stopped containers |

### Docker Engine API -- Images (7 endpoints)

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/images/json` | List images |
| `POST` | `/images/create` | Pull image |
| `GET` | `/images/{name}/json` | Inspect image |
| `POST` | `/images/{name}/push` | Push image |
| `DELETE` | `/images/{name}` | Remove image |
| `POST` | `/images/prune` | Remove unused images |
| `GET` | `/images/search` | Search registry |

### Docker Engine API -- System and Other

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/info` | System information |
| `GET` | `/version` | Version information |
| `GET` | `/_ping` | Health check |
| `GET` | `/events` | Event stream |
| `GET` | `/system/df` | Disk usage |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/health` | Daemon health |
| `POST` | `/auth` | Authentication |

### Podman API (39 endpoints)

| Category | Endpoints |
|:---------|:----------|
| **Pods** | `/libpod/pods/create`, `/libpod/pods/json`, `/libpod/pods/{id}/json`, `/libpod/pods/{id}/start`, `/libpod/pods/{id}/stop`, `/libpod/pods/{id}/restart`, `/libpod/pods/{id}/kill`, `/libpod/pods/{id}/pause`, `/libpod/pods/{id}/unpause`, `/libpod/pods/{id}/exists`, `/libpod/pods/{id}`, `/libpod/pods/prune` |
| **Secrets** | `/libpod/secrets/create`, `/libpod/secrets/json`, `/libpod/secrets/{id}/json`, `/libpod/secrets/{id}` |
| **Manifests** | `/libpod/manifests/create`, `/libpod/manifests/{name}/add`, `/libpod/manifests/{name}/remove`, `/libpod/manifests/{name}/json`, `/libpod/manifests/{name}/push`, `/libpod/manifests/json` |

### Kubernetes API

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/api/v1/pods` | List pods |
| `POST` | `/api/v1/pods` | Create pod |
| `GET` | `/api/v1/services` | List services |
| `POST` | `/api/v1/services` | Create service |
| `GET` | `/apis/apps/v1/deployments` | List deployments |
| `POST` | `/apis/apps/v1/deployments` | Create deployment |
| `GET` | `/version` | Server version info |

Full API group paths: `api/v1`, `apis/apps/v1`, `apis/batch/v1`, `networking.k8s.io/v1`, `rbac.authorization.k8s.io/v1`.

### CRI gRPC (41 RPCs)

The CRI plugin implements the full Kubernetes Container Runtime Interface:

| Service | RPCs | Description |
|:--------|:-----|:------------|
| RuntimeService | 35 | RunPodSandbox, StopPodSandbox, RemovePodSandbox, PodSandboxStatus, ListPodSandbox, CreateContainer, StartContainer, StopContainer, RemoveContainer, ListContainers, ContainerStatus, UpdateContainerResources, ExecSync, Exec, Attach, PortForward, and more |
| ImageService | 6 | ListImages, ImageStatus, PullImage, RemoveImage, ImageFsInfo |

---

## Networking

### Network Types

| Type | Description |
|:-----|:------------|
| **Bridge** | Default `doki0` bridge with NAT, DNS resolution, port mapping |
| **Host** | Share host network namespace (max performance). On Termux/Android, falls back to host network via proot when `/proc/sys/net` is unavailable |
| **None** | Loopback only (complete isolation) |
| **CNI** | bridge, host-local, portmap, macvlan, ipvlan, dhcp, vlan |
| **Rootless** | Uses **pasta** for TCP/UDP without root or TAP devices |
| **IPv6** | Dual-stack IPv4/IPv6 on bridge networks |

### Port Mapping

```bash
doki run -p 8080:80 nginx:alpine                    # Map host 8080 to container 80
doki run -p 127.0.0.1:8080:80 nginx:alpine          # Bind to specific IP
doki run -p 8080:80/tcp -p 8080:80/udp              # TCP and UDP
doki run -P nginx:alpine                            # Publish all EXPOSEd ports
doki run -p 8080-8090:80 nginx:alpine               # Port range
```

### Termux-Specific Networking

On Android/Termux, the host network namespace is inaccessible to unprivileged processes. Doki detects this at startup and:
- Falls back to `host` network mode via proot, sharing the Termux app's network namespace
- DNS listens on `127.0.0.11:8053` (port 53 is blocked by SELinux)
- Upstream DNS resolvers are read from `getprop net.dns1..net.dns4`
- Port mapping uses socat in rootless mode (iptables unavailable)

Override DNS listen address with `DOKI_DNS_LISTEN=IP:PORT` or `dns.listen` in `config.json`.

### Port Forwarding Internals

Port mapping uses iptables DNAT in root mode and `socat` in rootless mode:

- DNAT rules use `[]string` (no shell parsing) and include the `-A OUTPUT` chain for local traffic
- Rootless socat forwards target the container bridge IP directly (not localhost)
- Veth pairs tracked via `Endpoint.VethHost`/`Endpoint.VethPeer` for idempotent teardown
- Tear down deletes both veth ends via `ip link del` before removing the bridge

---

## DokiLink Mesh

DokiLink provides multi-host container networking without a central broker. Peers discover each other via mDNS (LAN), DHT (internet), or static configuration. All traffic is authenticated via Ed25519 signatures and encrypted with TLS 1.3 or NaCl secretbox.

### NAT Traversal

NAT traversal follows a four-stage sequence:

1. **STUN**: Both peers query a STUN server (RFC 8489) to discover their public IP and port mapping
2. **Exchange**: Peers exchange public addresses via the gossip protocol
3. **Hole Punching**: Both peers issue simultaneous TCP SYN packets to each other's public address using `TCPConn.SetDeadline` with coordinated timing
4. **Fallback**: If hole punching fails (symmetric NAT), traffic routes through a relay peer acting as a TURN proxy

### DHT Peer Discovery

Kademlia DHT with 160-bit node IDs provides decentralized peer discovery:

| Parameter | Value | Description |
|:----------|:------|:------------|
| Node ID | 160-bit | SHA-1 hash of Ed25519 public key |
| k-buckets | k=8 | Maximum peers per routing bucket |
| Parallelism | alpha=3 | Concurrent lookups during FIND_NODE |
| RPCs | PING, STORE, FIND_NODE, FIND_VALUE | Standard Kademlia operations |

### mDNS Discovery

LAN peer discovery via mDNS (multicast DNS):

- Peers advertise via `_doki-link._tcp.local` TXT records
- Entries expire after 90 seconds if not refreshed
- Background cleanup loop runs every 30 seconds
- Self-filtering by install ID prevents self-discovery
- TXT records advertise `common.DokiVersion` for version compatibility

### Encryption Layers

| Layer | Encryption | Description |
|:------|:-----------|:------------|
| **L0** | None | Loopback only -- default on Android/Termux |
| **L1** | TLS 1.3 | Default, signed by per-install ECDSA P-256 CA |
| **L2** | NaCl secretbox | Opt-in with `DOKI_LINK_PAYLOAD_ENC=1`, key derived from both peers' Ed25519 public keys |

Key derivation is order-independent (both peers compute the same shared key via sorted-pubkey SHA-256). Per-connection nonces are seeded from `crypto/rand`. Replay protection uses a 5-minute timestamp window with an LRU nonce cache (1024 entries).

### Usage

```bash
# Show the local install ID and public key
doki mesh status

# Add a static peer
doki link add mybuddy 192.168.1.42:7432 \
  --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# List known peers
doki mesh ls

# Publish a container reachable through the mesh
doki run -d -p 0.0.0.0:9090:80 --name web nginx:alpine
```

---

## DNS

Doki runs an internal DNS server that handles inter-container name resolution and forwards external queries to upstream resolvers.

### Architecture

Containers point `/etc/resolv.conf` at `nameserver 127.0.0.11`. The Doki internal DNS server resolves local container names to bridge IPs and forwards external queries upstream.

### Defaults

| Platform | Default listen | Why |
|:---------|:----------------|:----|
| Linux | `127.0.0.11:53` | Standard unprivileged port |
| Android (Termux) | `127.0.0.11:8053` | Port 53 is blocked by SELinux (EACCES) on non-root |
| macOS | not used (ModeNative) | No bridge network |

### Container Name Resolution

```bash
$ doki network create backend
$ doki run -d --name db --network backend postgres:alpine
$ doki run -d --name api --network backend my-api:latest
$ doki exec api sh -c 'getent hosts db'
172.20.0.2      db.backend
```

### Key Behaviors

- **AAAA + PTR**: IPv6 forward and reverse lookups work alongside A records
- **SRV records**: Service discovery protocol support for `_<port>._tcp.<svc>.<ns>.svc.cluster.local`
- **ndots:0**: Container names like `forgejo` resolve directly, no `forgejo.local` retry loop
- **TCP retry**: When upstream UDP returns TC bit, the query is retried over TCP per RFC 5966
- **no busy-wait**: `ReadFromUDP` blocks on the socket, no polling loop
- **LRU cache**: 1024 entries, 5-minute TTL, auto-registration on container start, re-registration on daemon restart

---

## Storage

| Driver | Description | Best for |
|:-------|:------------|:---------|
| **overlay2** | Kernel overlay (direct syscall mount) | Linux with root, best performance |
| **fuse-overlayfs** | Userspace overlay via FUSE | Rootless, Termux, Android |
| **btrfs** | Btrfs subvolumes with snapshots | Systems with btrfs root |
| **zfs** | ZFS datasets with snapshots | Systems with ZFS pools |
| **vfs** | Simple directory copy | Testing, minimal systems |

---

## Security

| Layer | Protection |
|:------|:-----------|
| **Seccomp** | 80+ allowed syscalls, blocks module loading, BPF, AF_ALG, hardware I/O |
| **AppArmor** | Template-based profiles per container |
| **User namespaces** | UID/GID remapping, root maps to unprivileged user |
| **Capabilities** | Minimal default set, explicit grants, `--cap-drop=ALL` support |
| **TLS** | Mutual TLS authentication with client certificates |
| **Rate limiting** | Token-bucket: 100 req/s, burst 200 |
| **Image verification** | Path traversal protection, symlink validation, hardlink restrictions |
| **Landlock LSM** | Linux 5.13+ unprivileged sandboxing via Landlock ABI v9 |
| **mTLS enforcement** | `RequireAndVerifyClientCert` when `ClientCAs` is configured |
| **Constant-time comparison** | TOFU pubkey verification uses `crypto/subtle.ConstantTimeCompare` |
| **Replay protection** | Gossip messages include random nonce + 5-minute timestamp window, LRU nonce cache |
| **OOM DoS prevention** | Gossip listener wrapped in `io.LimitReader(MaxGossipMessageBytes+1)` |

### Blocked Syscalls

```
init_module, finit_module, delete_module    # Module loading
kexec_load, kexec_file_load                 # Kernel execution
iopl, ioperm                                # Hardware I/O
kcmp                                        # Kernel info leaks
process_vm_readv, process_vm_writev         # Process memory access
```

### Allowed Modern Syscalls

```
io_uring_setup, io_uring_enter, io_uring_register  # Async I/O
pidfd_open, pidfd_send_signal, pidfd_getfd         # PID file descriptors
rseq, userfaultfd, copy_file_range                 # Modern kernel features
landlock_create_ruleset, landlock_add_rule         # Landlock sandboxing
```

---

## Configuration

### Daemon Config (`~/.doki/config.json`)

```json
{
  "data_dir": "/data/data/com.termux/files/usr/var/lib/doki",
  "socket": "/data/data/com.termux/files/usr/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "default_network": "bridge",
  "debug": false,
  "log_level": "info",
  "rootless": true,
  "dns": {
    "listen": "127.0.0.11:8053",
    "upstream": ["8.8.8.8:53", "8.4.4.4:53"],
    "cache_capacity": 256
  },
  "mesh": {
    "enabled": true,
    "listen": ":7432",
    "stun_servers": ["stun.l.google.com:19302"],
    "enable_mdns": true,
    "payload_encryption": false
  },
  "registry_mirrors": [],
  "insecure_registries": []
}
```

### Environment Variables

| Variable | Description | Default |
|:---------|:------------|:--------|
| `DOKI_HOST` | Daemon socket path | Platform-specific |
| `DOKI_DATA_DIR` | Data directory | `~/.doki/data` |
| `DOKI_STORAGE_DRIVER` | Storage driver | `fuse-overlayfs` |
| `DOKI_TLS` | Enable TLS | unset |
| `DOKI_TLS_CERT` | TLS certificate path | unset |
| `DOKI_TLS_KEY` | TLS key path | unset |
| `DOKI_KERNEL` | MicroVM kernel path | Platform-specific |
| `DOKI_NATIVE` | Force native mode | unset |
| `DOKI_DNS_LISTEN` | DNS server listen address | `127.0.0.11:8053` (Android) / `127.0.0.11:53` (Linux) |
| `DOKI_DEBUG` | Enable debug mode (pprof on `:6060`) | unset |
| `DOKI_RATE_LIMIT` | Requests per second | `100` |
| `DOKI_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |
| `DOKI_LOG_FORMAT` | Log format (json/text) | auto-detect |
| `DOKI_LINK_MESH` | Enable DokiLink mesh (`1`/`0`) | `1` |
| `DOKI_LINK_ADDR` | Override mesh gossip listen address | `:7432` |
| `DOKI_LINK_STUN` | STUN servers for NAT traversal (comma-separated) | `stun.l.google.com:19302` |
| `DOKI_LINK_RELAY` | Relay peer for TURN fallback | unset |
| `DOKI_LINK_PAYLOAD_ENC` | Enable NaCl secretbox (L2 encryption) | unset |
| `DOKI_USE_SOCAT` | Force socat for port forwarding | unset |

---

## Building

### Requirements

- Go 1.22 or later
- `make` (optional)
- For microVM mode: `crosvm` or `firecracker` binary (auto-detected)
- For macOS VZ backend: CGO enabled, macOS 11+ SDK

### Build Targets

```bash
# Android / Termux (ARM64)
make build-android-arm64
make install

# Android / Termux (ARMv7)
make build-android-armv7

# Linux (ARM64)
make build-linux-arm64

# Linux (ARMv7)
make build-linux-armv7

# Linux (x86_64)
make build-linux-amd64

# macOS (Apple Silicon)
make build-darwin-arm64

# macOS (Intel)
make build-darwin-amd64

# All platforms at once
make release

# SHA256 checksums
make sha256

# Testing and linting
make test      # go test ./...
make vet       # go vet ./...
make clean     # rm -rf releases/
```

### Manual Build

```bash
make release
# Or equivalently:
go build -trimpath -ldflags="-s -w" -o releases/doki ./cmd/doki
go build -trimpath -ldflags="-s -w" -o releases/dokid ./cmd/dokid
go build -trimpath -ldflags="-s -w" -o releases/doki-compose ./cmd/doki-compose
go build -trimpath -ldflags="-s -w" -o releases/doki-init ./cmd/doki-init
go build -trimpath -ldflags="-s -w" -o releases/doki-kube ./cmd/doki-kube
go build -trimpath -ldflags="-s -w" -o releases/doki-kubectl ./cmd/doki-kubectl
```

---

## Project Structure

```
Doki/
  cmd/
    doki/                 CLI binary (108 commands, 1600+ lines)
    dokid/                Daemon binary (REST API, TLS, rate limiting)
    doki-compose/         Docker Compose compatible CLI
    doki-init/            Minimal PID 1 for containers (Go)
    doki-init-rust/       Minimal PID 1 for microVM guests (Rust, 412K)
    doki-kube/            Kubernetes control plane (all-in-one)
    doki-kubectl/         kubectl-compatible CLI client
    dokitest/             Integration test suite
    regtest/              Registry test suite
  pkg/
    api/                  Docker Engine API v1.54 server
    podman/               Podman libpod v5 API (39 endpoints)
    compose/              Compose engine with watch + publish + healthcheck
    apiserver/            Kubernetes API server
    kubelet/              Kubernetes kubelet agent (real CRI client)
    scheduler/            Kubernetes scheduler
    controllers/          Kubernetes controllers (10 functional controllers)
    kubeproxy/            Kubernetes kube-proxy (iptables/nftables/userspace)
    coredns/              Cluster DNS for Kubernetes
    kubectl/              kubectl HTTP client library
    k8s-types/            80 Kubernetes API types
    store/                In-memory state store + SQLiteStore with crash-safe persistence
    runtime/              OCI runtime with 12 execution modes
    image/                OCI image management (pull, push, build)
    registry/             OCI Distribution Spec client
    network/              Container networking (bridge, CNI, DNS, pasta)
    storage/              Storage drivers (overlay2, fuse, btrfs, zfs)
    builder/              Dokifile parser (18 instructions, multi-stage)
    cli/                  CLI library (3200+ lines)
    common/               Shared types, config, utilities
    netlink/              DokiLink Mesh (gossip, proxy, NAT traversal, DHT, mDNS)
    landlock/             Landlock LSM sandbox (Linux 5.13+)
    macos/                macOS native VM (VZ + QEMU + Sandbox backends)
    security/             Seccomp and AppArmor profiles
    distro/               Linux distribution management
    cri/                  Kubernetes CRI gRPC server (41 RPCs)
    oci/                  OCI spec generation
    deps/                 Dependency management (doki deps tool)
    scheduler/            Pod scheduling
  internal/
    dokivm/               MicroVM subsystem (crosvm, firecracker, qemu)
    namespaces/           Linux namespace management
    cgroups/              cgroups v2 resource management
    fuse/                 FUSE overlay filesystem operations
    proot/                Proot fallback for Android
    seccomp/              Seccomp profile engine
    apparmor/             AppArmor profile generator
  doki-os/                doki-OS VM kernel config + Makefile
```

---

## Compatibility

### What Works

| Feature | Status | Notes |
|:--------|:------:|:------|
| `doki run` | Tested | Basic commands, shell scripts, --init, --user, --entrypoint, --restart |
| `doki pull` | Tested | ARM64 multi-arch auto-resolve, parallel downloads, token auth |
| `doki push` | Tested | OCI Distribution Spec: blob upload, cross-repo mount, manifest PUT |
| `doki images` | Tested | Correct sizes, RepoDigests populated |
| `doki ps` / `doki ps -a` | Tested | Names, ports, image shown |
| `doki inspect` | Tested | Full JSON output |
| `doki stop` / `doki rm` | Tested | By name or ID, no deadlocks |
| `doki build` | Tested | RUN layers, COPY --from, ARG, ENV, .dockerignore, build cache |
| `doki logs` | Tested | Rotation (10MB/3 files), Docker multiplexed stream format |
| `doki exec` | Tested | Runs inside container via proot |
| `doki attach` | Tested | HTTP hijack, bidirectional streaming |
| `doki wait` | Tested | Multi-container, returns exit codes |
| `doki login` / `doki logout` | Tested | Token auth, Basic auth, credential wiring |
| `doki network ls` | Tested | Bridge/host/none, doki0 bridge creation |
| `doki volume create/ls/rm` | Tested | Local driver, tmpfs support |
| `doki-compose up/down` | Tested | Full compose spec: networks, volumes, secrets, healthcheck |
| `doki cp` | Tested | Copy files host/container with tar extraction |
| Port forwarding (`-p`) | Tested | iptables DNAT (root) and socat (rootless) |
| Isolation auto-selection | Tested | Registry picks best available runner from 12 modes |
| `--runtime` flag | Tested | Explicit mode via `doki run --runtime proot` |
| Kubernetes CRI gRPC | Functional | All 35+6 RPCs implemented on Unix socket |
| Kubelet with real CRI | Functional | Reconcile loop calls RunPodSandbox/CreateContainer/StartContainer |
| Kube-proxy | Functional | iptables/nftables/userspace modes, DNAT + MASQUERADE |
| K8s controllers | Functional | Deployment, ReplicaSet, Job, Endpoint, Service, Namespace, GC |
| SQLiteStore | Functional | Crash-safe persistent state |
| Podman API | Functional | 39 endpoints, pod/secret/manifest management |
| Compose healthcheck execution | Functional | Periodic probes, status reporting, `service_healthy` condition |
| DokiLink Mesh NAT traversal | Functional | STUN + TCP hole punching + TURN relay fallback |
| DokiLink DHT | Functional | Kademlia 160-bit, k=8, peer discovery |
| DokiLink mDNS | Functional | LAN discovery with 90s expiry + 30s cleanup |
| macOS VZ backend | Functional | Virtualization.framework with cgo bridge |

### What Does NOT Work Yet

| Feature | Status | Notes |
|:--------|:------:|:------|
| MicroVM isolation | Untested | Code exists, not tested on compatible hardware |
| gVisor isolation | Untested | runsc detection works, runtime not validated |
| WASM containers | Untested | wasmedge/iwasm detection works, runtime not validated |
| pKVM/Microdroid | Untested | pKVM detection works, no compatible hardware to test |
| Sysbox | Untested | sysbox-runc detection works, runtime not validated |
| FEX-Emu cross-arch | Untested | FEXInterpreter/box64 detection works, runtime not validated |
| QEMU user-mode | Untested | qemu-*-static detection works, runtime not validated |
| Chroot mode | Untested | Works in principle, not validated |
| Legacy32 mode | Untested | binfmt_misc detection works, runtime not validated |
| CNI networking | Untested | Plugin manager exists, not wired |
| Network bridge isolation | Partial | Works rootful (iptables DNAT); in proot/native, containers share host network |

---

## What's New

### v0.11.0 / v0.11.1 (Current)

Doki 0.11 is the networking and maturity release: full DokiLink Mesh with NAT traversal and DHT, macOS VZ cgo backend, Kubernetes 100% with real CRI, and production-ready Podman API.

#### DokiLink Mesh -- NAT Traversal + DHT + mDNS

- **NAT traversal**: STUN client (RFC 8489), TCP simultaneous open hole punching, and TURN-like relay server. Peers on different networks connect without static IPs.
- **DHT peer discovery**: Kademlia DHT with 160-bit node IDs, k-buckets (k=8), alpha=3 parallel lookups. Decentralized routing without static config or mDNS.
- **mDNS 90-second expiry**: entries expire after 90 seconds if not refreshed, cleanup loop every 30s.
- **Crypto fixes**: Order-independent key derivation (both peers derive the same shared key). Per-connection nonces from `crypto/rand`. Replay protection with 5-minute timestamp window and LRU nonce cache. `secretboxStreamConn.Close()` uses `atomic.Bool` to prevent double-close race.
- **Mesh hardening**: `Stop()` closes `stopCh` to signal all loops. Gossip decoder wrapped in `io.LimitReader` (OOM DoS prevention). mDNS TXT records advertise `common.DokiVersion`.

#### macOS Native Virtualization

- **VZ backend with cgo**: Objective-C bridge to Virtualization.framework (`VZVirtualMachineConfiguration`, `VZLinuxBootLoader`, `VZVirtioFileSystemDevice`, `VZBridgedNetworkDevice`/`VZNATNetworkDevice`, `VZRosettaPlatform`). Build tag `darwin && cgo`.
- **QEMU backend fixes**: `sync.RWMutex` for thread safety, binary verification, arch-aware args, SIGTERM/SIGKILL timeout.
- **Sandbox backend**: Tightened profiles, scoped process-exec and mach-lookup.
- **Build tag compatibility**: Package compiles on all platforms (darwin+cgo, darwin!cgo, !darwin).

#### Kubernetes 100%

- **CRI gRPC server**: Real gRPC CRI implementing all 35 RuntimeServiceServer + 6 ImageServiceServer RPCs on Unix socket.
- **Kubelet with real CRI**: `NewKubeletWithCRI` dials CRI socket, calls `RunPodSandbox` / `CreateContainer` / `StartContainer`, gets real PodIP, container states, and image digests.
- **Kube-proxy real**: iptables chains with DNAT/MASQUERADE, nftables ruleset generation, userspace TCP/UDP round-robin proxy (works without root).
- **Controllers functional**: DeploymentController, ReplicaSetController, JobController (parallelism/completions/backoff), EndpointController, ServiceController (ClusterIP allocation), NamespaceController (cascading deletion), GarbageCollector (OwnerReferences).
- **API server complete**: API group paths (`networking.k8s.io/v1`, `rbac.authorization.k8s.io/v1`), PATCH (merge-patch + strategic-merge), Watch (K8s event format).
- **SQLiteStore**: Persistent store with crash-safe persistence via SQLite.
- **Scheduler real**: Busy-wait replaced with blocking sleep, image locality scoring, least-requested scoring.
- **CoreDNS real**: UDP buffer race fixed, SRV record support, NXDOMAIN for unresolvable queries.

#### Podman Wiring

- Podman shim mounted in dokid at `/libpod/*` on the same server, inheriting TLS, middleware, and rate limiting.
- System info uses `runtime.GOARCH`, `runtime.GOOS`, detected kernel/memory, `common.DokiVersion`.
- Container lifecycle delegates to PodManager (start/stop/kill/restart/pause/unpause).
- Container dispatch returns 404 if not found, DELETE returns 204.

#### Compose Healthcheck Execution

- `HealthChecker` runs periodic probes (CMD/CMD-SHELL/NONE), respects Interval/Timeout/Retries/StartPeriod/StartInterval.
- Updates `state.HealthStatus.Status` (`starting` -> `healthy`/`unhealthy`).
- Compose `service_healthy` condition works end-to-end.

#### Diagnostics

- `doki deps` tool with `ls` (list system deps), `check` (CI gate), `go` (list Go deps), `install <name>` (best-effort install via detected package manager).

#### Security Fixes

- Path traversal validation in TrustStore, SecretManager, and ManifestManager.
- Constant-time comparison via `crypto/subtle.ConstantTimeCompare` for TOFU pubkey verification.
- mTLS enforcement with `RequireAndVerifyClientCert`.
- Replay protection with random nonce + timestamp freshness check.
- OOM DoS prevention via `io.LimitReader`.

### v0.10.0

Doki 0.10 is a massive expansion: **Podman 1:1 API compatibility, full Kubernetes distribution, macOS native VM support, doki-OS VM image, and 20 new dependencies** bringing the engine to 55,000+ lines of code across 158 files.

#### New Platforms and APIs

| Feature | Description |
|:--------|:------------|
| **Podman API v5** | 39 endpoints compatible with `podman-remote` clients. Pod, secret, and manifest management |
| **Kubernetes 1.32** | Full control plane: apiserver, kubelet, scheduler, controllers (10), kube-proxy, CoreDNS |
| **macOS Native** | VZ (Virtualization.framework) and QEMU backends for Apple Silicon and Intel Macs |
| **doki-OS** | Minimal Linux kernel config (~4MB bzImage) for container-optimized VM guests |
| **Landlock LSM** | Linux 5.13+ unprivileged sandboxing via Landlock ABI v9 |

#### New Binaries

| Binary | Description |
|:-------|:------------|
| `doki-kube` | All-in-one Kubernetes control plane |
| `doki-kubectl` | kubectl-compatible CLI (get, apply, delete, describe, logs) |

#### Quality

- **staticcheck**: 0 warnings
- **errcheck production**: 0 unchecked errors
- **go vet**: 0 warnings

### v0.9.3

This release shipped **DokiLink-Lite** (mesh networking) and **190+ bug fixes** across 4 rounds of comprehensive auditing.

#### DokiLink-Lite (Mesh Networking)

A TCP/UDP proxy + mesh layer that lets you forward a container's published port to another Doki instance. Pure Go stdlib + `crypto/tls` + `golang.org/x/crypto/nacl`.

| Feature | Description |
|:--------|:------------|
| TCP/UDP proxy | Half-close, idle timeouts, transport wrappers |
| TLS 1.3 (L1) | Default encryption, per-install ECDSA P-256 CA |
| NaCl secretbox (L2) | Opt-in via `DOKI_LINK_PAYLOAD_ENC=1`, Ed25519-derived key |
| Install identity | Ed25519 keypair + ECDSA CA at `$DOKI_ROOT/keys/` |
| TOFU trust | Public key recorded on first contact, verified on reconnect |
| Static peers | `$DOKI_ROOT/mesh/peers.json` via `doki link add/rm` |
| mDNS (opt-in) | Built with `-tags netlink_mdns`, LAN-only discovery |
| Gossip | Signed JSON messages over TCP, 15s peer discovery tick |

#### Critical Bug Fixes

- kill not updating state -- polls with `signal(0)` then saves exited state
- stop not updating state on SIGKILL failure -- always saves exit code 137
- Exec no output -- returns stdout/stderr bytes
- Compose flags after command -- parser continues after subcommand
- Container list Name always empty -- `stateToInfo` sets `info.Name`
- Create ignores `?name=` -- reads URL query parameter
- Image inspect Config -- PascalCase conversion
- `ps --format` -- template execution

### v0.9.2

This release was a **stability + correctness** pass over v0.9.1.

#### Critical Networking Fixes

1. **iptables DNAT missing `-A` flag** -- `-A OUTPUT` was missing, causing "Unknown option" error
2. **Port forwarding to localhost** -- socat now targets container bridge IP
3. **Orphaned veth pairs** -- `Endpoint` tracks `VethHost`/`VethPeer`, teardown deletes both ends
4. **proot missing on fresh hosts** -- `FindProotBinary()` falls back to system PATH

#### DNS Server Rewrite (18 bugs fixed)

LRU cache (1024 entries, 5 min TTL), AAAA + PTR support, ndots:0, TCP retry on TC bit, Android upstream via `getprop net.dns*`, port stripping, auto-registration, recovery on daemon restart.

#### LD_PRELOAD Fix for Termux

`libtermux-exec-ld-preload.so` was breaking proot's ptrace-based syscall forwarding. Fix: `StripHostEnv()` strips `LD_PRELOAD` and `LD_LIBRARY_PATH` from container environments.

### v0.9.1

- **OCI Push:** `doki push` -- blob upload, cross-repo mount, manifest PUT
- **Registry Auth:** `doki login` accepts credentials and propagates to registry client
- **Native tar extraction:** Go-native tar with whiteouts, path traversal protection, compression auto-detection, parallel extraction with rollback
- **4 new distros:** Fedora, Gentoo, OpenSUSE, Rocky Linux -- 8 distros total
- **Improved Compose engine:** Long syntax Ports/Volumes, `depends_on` health conditions with 60s poll, 30+ new fields
- **19 Proot C fixes:** SECCOMP_RET_ALLOW, fake_id0 brace bug, stat.c uid/gid fix, link2symlink UB, and more
- **Overlay2 kernel mount:** Uses `syscall.Mount("overlay")` directly instead of FUSE delegation
- **Attach via HTTP hijack:** `doki attach` with bidirectional streaming
- **DNS listener:** Internal DNS server on port 53 for inter-container resolution
- **ARMv7 beta:** Compilation and binaries for 32-bit ARM devices

### v0.9.0

- **doki-init-rust:** PID 1 rewritten in Rust (412K vs 2.9MB Go, -86%)
- **doki-proot:** Forked proot with daemon mode + JSON IPC protocol. 14K binary
- **Distro system:** `doki run --distro alpine/ubuntu/debian/arch` downloads from Docker Hub
- **ARMv7 beta:** Full feature parity for older ARM devices
- **Immich:** Full stack running (PostgreSQL 18 + pgvector + cube + earthdistance, Redis 7, Immich Server v2.7.5)

---

## Contributing

Contributions are welcome. Areas where help is most needed:

| Area | Description |
|:-----|:------------|
| **MicroVM backends** | Support for additional hypervisors and platforms |
| **CNI plugins** | Implementation of advanced networking features |
| **Security** | Hardening, fuzzing, and penetration testing |
| **Performance** | Layer caching, parallel operations, memory optimization |
| **Testing** | Integration tests, end-to-end tests, stress tests |
| **Documentation** | Tutorials, examples, and API reference |

### Development Setup

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
go build ./...
go test ./...
```

### Commit Style

- Use imperative mood ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable

---

## License

Apache License 2.0

```
Copyright 2024-2026 OpceanAI

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

---

## Links

| Platform | Repository | Source of truth |
|:---------|:-----------|:----------------|
| GitHub | [OpceanAI/Doki](https://github.com/OpceanAI/Doki) | Yes (primary) |
| GitLab | [aguitauwu/doki](https://gitlab.com/aguitauwu/doki) | mirror |
| Codeberg | [aguitauwu/Doki](https://codeberg.org/aguitauwu/Doki) | mirror |
| Website | [doki.opceanai.com](https://doki.opceanai.com) | docs / install script |
| Spanish README | [README.es.md](README.es.md) | translation |

> Main is the only source of truth. Mirrors are force-synced from `main` after each release. If you find a divergence, open an issue on GitHub.

### Wikis

| Platform | Wiki |
|:---------|:-----|
| GitHub | [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki) |
| GitLab | [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home) |
| Codeberg | [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki) |

### Related

| Repository | Description |
|:-----------|:------------|
| [Doki-proot](https://github.com/OpceanAI/Doki-proot) | Forked proot with JSON IPC daemon mode for Doki |
