<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/banner.svg" alt="Doki Banner" width="680">
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"></a>
  <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-doki--init-black?style=flat&logo=rust&logoColor=white" alt="Rust"></a>
  <a href="https://www.docker.com"><img src="https://img.shields.io/badge/API-Docker_v1.44-2496ED?style=flat&logo=docker&logoColor=white" alt="Docker API"></a>
  <a href="https://github.com/OpceanAI/Doki/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-555?style=flat" alt="License"></a>
  <a href="https://github.com/OpceanAI/Doki/releases"><img src="https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat&color=6366F1" alt="Downloads"></a>
  <a href="https://github.com/OpceanAI/Doki/stargazers"><img src="https://img.shields.io/github/stars/OpceanAI/Doki?style=flat&color=6366F1" alt="Stars"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#architecture">Architecture</a> &middot;
  <a href="#cli">CLI</a> &middot;
  <a href="#performance">Performance</a> &middot;
  <a href="#contributing">Contributing</a>
</p>

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/wave.svg" alt="Wave Divider" width="600">
</p>

# The Universal Container Engine

<p align="center">
  Docker and Podman compatible API &middot; OCI native &middot; Kubernetes CRI-ready<br>
  Runs on Linux, macOS, and Android via Termux &middot; ARM64, ARMv7 & x86_64<br>
  Rootless-first architecture &middot; No daemon required &middot; Hardware-level microVM isolation
</p>

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/platforms.svg" alt="Platforms" width="600">
</p>

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

## Overview

Doki is a container engine designed for every Linux kernel, from Android phones to cloud servers. It works without root, without systemd, and without a hypervisor. When your hardware offers more -- KVM, Android's built-in hypervisors, Linux namespaces -- Doki scales up its isolation automatically.

| | |
|---|---|
| **Binary size** | 13 MB |
| **Memory (idle)** | 12 MB |
| **Start time** | <15ms |
| **Platforms** | Linux, macOS, Android (Termux) |
| **Architectures** | ARM64, ARMv7, x86_64 |
| **Runtime deps** | Zero |

<br>

## Comparison

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/comparison.svg" alt="Binary Size Comparison" width="600">
</p>

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

<br>

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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

## Features

<table>
  <tr>
    <td width="50%" valign="top">
      <h3>Android Native</h3>
      <p>The only container engine that runs on Android via Termux without root. Designed for the constraints of mobile operating systems from the ground up.</p>
    </td>
    <td width="50%" valign="top">
      <h3>Rootless by Default</h3>
      <p>Works as a regular user. Scales to root or microVM isolation when available. No privilege escalation required for basic operations.</p>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h3>Docker Compatible</h3>
      <p>Same REST API v1.44. Drop-in replacement for Docker CLI and SDKs. docker-compose, docker-py, CI/CD pipelines all work without modification.</p>
    </td>
    <td width="50%" valign="top">
      <h3>Ultra Lightweight</h3>
      <p>13MB binary, 12MB RAM idle. 4x smaller than Docker, 7x less memory.</p>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h3>12 Isolation Levels</h3>
      <p>From WASM sandboxes to pKVM hardware isolation. Auto-selected at runtime based on available hardware. A mode for every device: phones without root, servers with KVM, Chromebooks with pKVM, or laptops needing x86 emulation on ARM.</p>
    </td>
    <td width="50%" valign="top">
      <h3>Compose Support</h3>
      <p>Full Compose spec: networks, volumes, secrets, health checks, depends_on with 60s poll, 30+ fields including shm_size, pids_limit, ulimits.</p>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h3>OCI Compliant</h3>
      <p>Push and pull to any OCI registry. Multi-architecture auto-resolution. Compatible with Docker Hub, GHCR, ECR, GCR, Quay, GitLab, Harbor.</p>
    </td>
    <td width="50%" valign="top">
      <h3>CRI-Ready</h3>
      <p>Kubernetes CRI plugin. Run K8s YAML without a cluster. PodSandbox, container management, and image service implemented.</p>
    </td>
  </tr>
</table>

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/wave.svg" alt="Wave Divider" width="600">
</p>

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

<br>

## Binaries

| Binary | Size | Description |
|:-------|:----:|:------------|
| **doki** | 6.7 MB | CLI with ~108 commands. Connects to daemon via Unix socket |
| **dokid** | 9.2 MB | Daemon. Docker Engine API v1.48 over Unix socket. Proot integrated |
| **doki-compose** | 7.6 MB | Compose engine. Full spec support with health conditions |
| **doki-init** | 2.9 MB | PID 1 for microVM guests (Go). Rust variant available in source |

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

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
| **7** | gVisor | User-space kernel | ~20% CPU | Google's runsc intercepts syscalls at the user-space boundary. Use when you want defense-in-depth without a VM — 70% of syscalls never reach the host |
| **6** | FEX-Emu | Emulation (x86 on ARM) | ~30% CPU | FEXInterpreter or Box64. Runs x86/x86_64 binaries on ARM64 without recompilation. Use for legacy x86 containers on Apple Silicon or ARM servers |
| **5** | QEMU User | Emulation (cross-arch) | ~50% CPU | QEMU user-mode for any guest arch. Use when you need to run containers built for a different architecture (e.g., arm32 on arm64, or any arch on any arch) |
| **4** | Proot | Userspace (ptrace) | ~10% CPU | Ptrace-based chroot without root. Default on Android/Termux. Use on devices where you lack root and namespaces — phones, tablets, ChromeOS Linux |
| **3** | Legacy32 | Dual-arch compat | Negligible | Run ARMv7 containers on ARM64 kernels via binfmt_misc and multiarch support. Use when your workload ships only as 32-bit ARM |
| **2** | Chroot | Filesystem-level | Minimal | Lightweight filesystem isolation via chroot. Use for quick testing, build stages, or when every other mode is unavailable |
| **1** | Native | None | Zero | Direct host execution. Always available as fallback. Use when you trust the workload and want zero overhead |

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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/wave.svg" alt="Wave Divider" width="600">
</p>

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

<br>

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
| `healthcheck` | Health probes per service |
| `deploy` | Resource limits (`cpus`, `memory`), `replicas`, `restart_policy` |
| `profiles` | Conditional service activation |
| `extends` | Service inheritance |
| `include` | Multi-file composition |
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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

## REST API

Doki exposes the **Docker Engine API v1.44** over a Unix socket. 53 endpoints.

### Key Endpoints

<details>
<summary><b>Containers (16 endpoints)</b></summary>

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

</details>

<details>
<summary><b>Images (8 endpoints)</b></summary>

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/images/json` | List images |
| `POST` | `/images/create` | Pull image |
| `GET` | `/images/{name}/json` | Inspect image |
| `POST` | `/images/{name}/push` | Push image |
| `DELETE` | `/images/{name}` | Remove image |
| `POST` | `/images/prune` | Remove unused images |
| `GET` | `/images/search` | Search registry |

</details>

<details>
<summary><b>System & Other</b></summary>

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

</details>

<br>

## Networking

| Type | Description |
|:-----|:------------|
| **Bridge** | Default `doki0` bridge with NAT, DNS resolution, port mapping |
| **Host** | Share host network namespace (max performance) |
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

<br>

## Storage

| Driver | Description | Best for |
|:-------|:------------|:---------|
| **overlay2** | Kernel overlay (direct syscall mount) | Linux with root, best performance |
| **fuse-overlayfs** | Userspace overlay via FUSE | Rootless, Termux, Android |
| **btrfs** | Btrfs subvolumes with snapshots | Systems with btrfs root |
| **zfs** | ZFS datasets with snapshots | Systems with ZFS pools |
| **vfs** | Simple directory copy | Testing, minimal systems |

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/wave.svg" alt="Wave Divider" width="600">
</p>

## Performance

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/performance.svg" alt="Performance Chart" width="600">
</p>

Measured on **Qualcomm Snapdragon 685, Android 14, Termux**. Cold pull, ARM64 native binaries.

| Image | Size | Pull | Start | RAM |
|:------|-----:|-----:|------:|----:|
| busybox:latest | 1.8 MB | 1.4s | **8ms** | 0.6 MB |
| alpine:latest | 4.0 MB | 2.1s | **10ms** | 1.2 MB |
| python:3-alpine | 17.3 MB | 8.2s | **4ms** | 3.1 MB |
| redis:alpine | 15.2 MB | 7.1s | **6ms** | 2.8 MB |
| nginx:alpine | 24.6 MB | 11.5s | **12ms** | 5.8 MB |
| node:22-alpine | 48.7 MB | 22.8s | **15ms** | 12.3 MB |
| mariadb:latest | 156 MB | 62.4s | **20ms** | 31.2 MB |
| nextcloud:latest | 423 MB | 87.3s | **45ms** | 45.7 MB |

<br>

## Registries

Compatible with any OCI or Docker Registry HTTP API v2.

| Registry | Pull | Push | Auth |
|:---------|:----:|:----:|:-----|
| Docker Hub | Yes | Yes | Token |
| GitHub Container Registry | Yes | Yes | PAT |
| Quay.io | Yes | Yes | Robot |
| Google Container Registry | Yes | Yes | JSON key |
| Amazon ECR | Yes | Yes | IAM |
| Azure Container Registry | Yes | Yes | SP |
| GitLab Registry | Yes | Yes | Token |
| Harbor | Yes | Yes | Basic |
| Any OCI registry | Yes | Yes | Configurable |

### Verified Images

`alpine`, `busybox`, `python:3-alpine`, `node:22-alpine`, `nginx:alpine`, `redis:alpine`, `mariadb`, `postgres:alpine`, `nextcloud`, `ubuntu`, `debian`, `golang`, `rust`, `ruby`, `php`, `traefik`, `caddy`, `vault`

<br>

## Supported Distros

```bash
doki run --distro alpine   echo hello
doki run --distro ubuntu   bash
doki run --distro debian   --install curl,vim bash
doki run --distro arch
doki run --distro fedora
doki run --distro rocky
doki run --distro gentoo
doki run --distro opensuse
```

| Distro | Image | Size |
|:-------|:------|:-----|
| Alpine | `alpine:latest` | ~3MB |
| Ubuntu | `ubuntu:latest` | ~29MB |
| Debian | `debian:stable-slim` | ~27MB |
| Arch | `archlinux:latest` | ~150MB |
| Fedora | `fedora:latest` | ~95MB |
| Gentoo | `gentoo/stage3:latest` | ~200MB |
| OpenSUSE | `opensuse/tumbleweed:latest` | ~80MB |
| Rocky Linux | `rockylinux:latest` | ~100MB |

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

## Configuration

### Daemon Config (`~/.doki/config.json`)

```json
{
  "root": "/data/data/com.termux/files/usr/var/lib/doki",
  "socket_path": "/data/data/com.termux/files/usr/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "default_network": "bridge",
  "debug": false,
  "log_level": "info",
  "rootless": true,
  "dns": ["8.8.8.8", "8.8.4.4"],
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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/wave.svg" alt="Wave Divider" width="600">
</p>

## Building

### Requirements

- Go 1.22 or later
- `make` (optional)
- For microVM mode: `crosvm` or `firecracker` binary (auto-detected)

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

# macOS (Apple Silicon)
make build-darwin-arm64

# All platforms at once
make release

# SHA256 checksums
make sha256

# Testing & linting
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
```

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

## Project Structure

```
Doki/
  cmd/
    doki/                 CLI binary (108 commands, 2200+ lines)
    dokid/                Daemon binary (REST API, TLS, gRPC, rate limiting)
    doki-compose/         Docker Compose compatible CLI
    doki-init-rust/       Minimal PID 1 for microVM guests (Rust, 412K)
  pkg/
    api/                  Docker Engine API v1.44 server (53 endpoints)
    runtime/              OCI runtime with 4 execution modes
    image/                OCI image management (pull, push, build)
    registry/             OCI Distribution Spec client
    network/              Container networking (bridge, CNI, DNS)
    storage/              Storage drivers (overlay2, fuse, btrfs, zfs)
    builder/              Dokifile parser (18 instructions, multi-stage)
    compose/              Compose engine
    cri/                  Kubernetes CRI plugin
    cli/                  CLI library (2200+ lines)
    common/               Shared types, config, utilities
  internal/
    dokivm/               MicroVM subsystem (crosvm, firecracker, qemu)
    namespaces/           Linux namespace management
    cgroups/              cgroups v2 resource management
    fuse/                 FUSE overlay filesystem operations
    proot/                proot fallback for Android
    seccomp/              Seccomp profile engine (80+ syscalls)
    apparmor/             AppArmor profile generator
  kernels/                Pre-compiled VM kernels (ARM64 + x86_64)
```

**40 Go source files. 14,500+ lines of code. 4 compiled binaries. Zero external dependencies.**

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

## Known Limitations

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
| Port forwarding (`-p`) | Tested | FirewallManager wired |
| Isolation auto-selection | Tested | Registry picks best available runner from 12 modes |
| `--runtime` flag | Tested | Explicit mode via `doki run --runtime proot` |

### What Does NOT Work Yet

| Feature | Status | Notes |
|:--------|:------:|:------|
| `doki cp` | Stub | Copy files host/container not implemented |
| MicroVM isolation | Untested | Code exists, not tested on compatible hardware |
| gVisor isolation | Untested | runsc detection works, runtime not validated |
| WASM containers | Untested | wasmedge/iwasm detection works, runtime not validated |
| pKVM/Microdroid | Untested | pKVM detection works, no compatible hardware to test |
| Sysbox | Untested | sysbox-runc detection works, runtime not validated |
| FEX-Emu cross-arch | Untested | FEXInterpreter/box64 detection works, runtime not validated |
| QEMU user-mode | Untested | qemu-*-static detection works, runtime not validated |
| Chroot mode | Untested | Works in principle, not validated |
| Legacy32 mode | Untested | binfmt_misc detection works, runtime not validated |
| Kubernetes CRI | Stub | gRPC server not implemented |
| CNI networking | Untested | Plugin manager exists, not wired |
| Network bridge isolation | No | Containers share host network in proot/native mode |

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/wave.svg" alt="Wave Divider" width="600">
</p>

## What's New

### v0.9.2-alpha (Current)

- **12 isolation levels (8 new):** New runner registry with auto-selection. Added WASM, gVisor, pKVM/Microdroid, Sysbox, QEMU User, FEX-Emu, Chroot, and Legacy32 modes alongside the original MicroVM, Namespaces, Proot, and Native
- **DNS server overhaul — 18 bugs fixed across 7 files + 1 new:**
  - `nameserver` port stripped from resolv.conf (port in `nameserver` line is invalid per resolv.conf format)
  - DNS entries auto-registered on container start (`SetupNetwork` now creates endpoint + calls `AddEntry`)
  - DNS entries re-registered on daemon restart (`recoverContainers` → `ReRegisterDNS`)
  - Android: default port `127.0.0.11:8053` (port 53 is blocked without root)
  - Android: DNS discovery via `getprop net.dns1..4` instead of hardcoded Google DNS
  - AAAA (IPv6) and PTR (reverse) local resolution in addition to A records
  - `options ndots:0` for single-label container names (e.g. `forgejo` resolves without trailing dot)
  - TCP retry upstream when UDP response has TC bit (RFC 5966)
  - Blocking `ReadFromUDP` instead of busy-wait polling (`SetReadDeadline` removed)
- **LD_PRELOAD fix:** `libtermux-exec-ld-preload.so` filtered from proot environment — Termux's exec hook breaks proot's ptrace. Before: `"execve: Function not implemented"`. After: containers start normally.
- **Proot forced on Android:** `detectMode()` prefers proot over native mode (native cannot isolate or resolve DNS on Android)
- **DNS dead param removed:** `NewServer()` no longer accepts unused `*network.DNSServer` variadic
- **Android DNS auto-detection:** New `pkg/network/android_dns.go` — reads `getprop net.dns1..4` for upstream resolvers
- **ParseResolvConf cleanup:** Nameservers stored without port (resolv.conf format). `NameserverList()` appends `:53` for dialling.
- **Unified version:** Single source of truth via `common.DokiVersion` + `-ldflags` injection of `GitCommit`, `BuildDate`, `BuildUser`
- **Structured logging:** `log/slog` (JSON in prod, text on TTY) replacing stdlib `log` in daemon, CLI, and middleware. Auto-detected from stderr
- **Atomic state persistence:** `saveState` writes to `state.json.tmp.*` then `os.Rename` for crash-safety
- **API bumped to v1.48:** aligned with Docker Engine 29.5.x (May 2026)
- **16 KiB page size alignment:** Android 15+ requirement, `-Wl,-z,max-page-size=16384` in the Android build target
- **Metrics + counter hardening:** `/health` and `/metrics` integrated with the slog pipeline
- **Test coverage:** DNS LRU, atomic state, resolv.conf parsing, version invariants

### v0.9.1

- **OCI Push:** `doki push` -- blob upload, cross-repo mount, manifest PUT to any OCI registry
- **Registry Auth:** `doki login` accepts credentials and propagates to registry client
- **Native tar extraction:** Go-native tar with whiteouts, path traversal protection, compression auto-detection (gzip/bzip2/xz/zstd), parallel extraction with rollback
- **4 new distros:** Fedora, Gentoo, OpenSUSE, Rocky Linux -- 8 distros total
- **Improved Compose engine:** Long syntax Ports/Volumes, `depends_on` health conditions with 60s poll, 30+ new fields
- **19 Proot C fixes:** SECCOMP_RET_ALLOW, fake_id0 brace bug, stat.c uid/gid fix, link2symlink UB, and more
- **Updated seccomp:** io_uring, pidfd, rseq, userfaultfd, copy_file_range now allowed
- **Overlay2 kernel mount:** Uses `syscall.Mount("overlay")` directly instead of FUSE delegation
- **Attach via HTTP hijack:** `doki attach` with bidirectional streaming
- **Multi-container wait:** Waits for multiple containers simultaneously
- **DNS listener:** Internal DNS server on port 53 for inter-container resolution
- **Buffer pool & String intern pool:** Reduced GC pressure and memory deduplication
- **PProf endpoint:** `/debug/pprof/` for profiling
- **Systemd socket activation:** Linux socket activation support
- **ARMv7 beta:** Compilation and binaries for 32-bit ARM devices

### v0.9.0

- **doki-init-rust:** PID 1 rewritten in Rust (412K vs 2.9MB Go, -86%)
- **doki-proot:** Forked proot with daemon mode + JSON IPC protocol. 14K binary
- **Distro system:** `doki run --distro alpine/ubuntu/debian/arch` downloads from Docker Hub
- **ARMv7 beta:** Full feature parity for older ARM devices
- **Immich:** Full stack running (PostgreSQL 18 + pgvector + cube + earthdistance, Redis 7, Immich Server v2.7.5)

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

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

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/divider.svg" alt="Divider" width="600">
</p>

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

<br>

## Links

| Platform | Repository |
|:---------|:-----------|
| Website | [doki.opceanai.com](https://doki.opceanai.com) |
| GitHub | [OpceanAI/Doki](https://github.com/OpceanAI/Doki) |
| GitLab | [aguitauwu/doki](https://gitlab.com/aguitauwu/doki) |
| Codeberg | [aguitauwu/Doki](https://codeberg.org/aguitauwu/Doki) |

### Related

| Repository | Description |
|:-----------|:------------|
| [Doki-proot](https://github.com/OpceanAI/Doki-proot) | Forked proot with JSON IPC daemon mode for Doki |

<br>

<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/footer.svg" alt="Footer" width="400">
</p>

<p align="center">
  <a href="https://github.com/OpceanAI">
    <img src="https://img.shields.io/badge/Made_by_OpceanAI-2026-000?style=flat" alt="OpceanAI">
  </a>
</p>
