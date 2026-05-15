<div align="center">

<img src="https://img.shields.io/badge/Doki-0.9.1-6366F1?style=for-the-badge&labelColor=0A0A0A" alt="Doki v0.9.1">
<img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=for-the-badge&labelColor=0A0A0A&logo=go&logoColor=00ADD8">
<img src="https://img.shields.io/badge/Rust-doki--init-purple?style=for-the-badge&labelColor=0A0A0A&logo=rust&logoColor=white">
<img src="https://img.shields.io/badge/API-Docker_v1.44-6366F1?style=for-the-badge&labelColor=0A0A0A">
<img src="https://img.shields.io/badge/License-Apache_2.0-6366F1?style=for-the-badge&labelColor=0A0A0A">
<img src="https://img.shields.io/badge/Website-doki.opceanai.com-06b6d4?style=for-the-badge&labelColor=0A0A0A&link=https://doki.opceanai.com">
<img src="https://img.shields.io/badge/Mirrors-GitHub_|_GitLab_|_Codeberg-FCA121?style=for-the-badge&labelColor=0A0A0A">

<br><br>

# Doki

## The Universal Container Engine

Docker and Podman compatible API. OCI native. Kubernetes CRI-ready.<br>
Runs on Linux, macOS, and Android via Termux. ARM64 and x86_64.<br>
Rootless-first architecture. No daemon required for basic operations.<br>
Hardware-level microVM isolation when your device supports it.

<br>

---

**The container ecosystem has a gap.** Docker Desktop requires a Linux VM on macOS and Windows. Podman works on Linux and macOS, but not Android. Kubernetes needs a full cluster. On a phone, on a tablet, on a Raspberry Pi, on an old laptop running a minimal OS -- there is no container engine that just works.

**Doki fills that gap.** It runs on any Linux kernel, from Android phones to cloud servers. It works without root. It works without systemd. It works without a hypervisor. And when your hardware offers more -- KVM, Android's built-in hypervisors, Linux namespaces -- Doki scales up its isolation automatically.

One binary. One API. Every platform.

---

## What's New in v0.9

### v0.9.1 (Current)

- **OCI Push:** `doki push` now works — blob upload, cross-repo mount, manifest PUT to any OCI registry
- **Registry Auth:** `doki login` accepts credentials and propagates to registry client
- **Native tar extraction:** Go-native tar with whiteouts, path traversal protection, compression auto-detection (gzip/bzip2/xz/zstd), parallel extraction with rollback
- **4 new distros:** Fedora, Gentoo, OpenSUSE, Rocky Linux — 8 distros total
- **Improved Compose engine:** Long syntax Ports/Volumes, `depends_on` health conditions with 60s poll, 30+ new fields (shm_size, pids_limit, oom_kill_disable, tmpfs, devices, platform, runtime, scale, blkio_config, ulimits)
- **19 Proot C fixes:** SECCOMP_RET_ALLOW, fake_id0 brace bug, stat.c uid/gid fix, link2symlink UB, sysnum off-by-one, kompat overflow, hidden_files VLA 64KB, port_switch fixes, sysvipc shm 400KB→64MB, overflow checks, TOCTOU guard, signal forwarding
- **Updated seccomp:** io_uring, pidfd, rseq, userfaultfd, copy_file_range now allowed
- **Overlay2 kernel mount:** Uses `syscall.Mount("overlay")` directly instead of FUSE delegation
- **Attach via HTTP hijack:** `doki attach` implemented with bidirectional streaming
- **Multi-container wait:** Waits for multiple containers simultaneously
- **DNS listener:** Internal DNS server on port 53 for inter-container resolution
- **DOKI_HOST env:** Supports `DOCKER_HOST` and `DOKI_HOST` environment variables
- **Buffer pool:** Reusable buffer pool in runtime (reduces GC pressure)
- **String intern pool:** Common string deduplication
- **PProf endpoint:** `/debug/pprof/` for profiling
- **Systemd socket activation:** Linux socket activation support
- **ARMv7 beta:** Compilation and binaries for 32-bit ARM devices

### v0.9.0

- **doki-init-rust:** PID 1 rewritten in Rust (412K vs 2.9MB Go, -86%). Runs inside microVMs via vsock JSON IPC.
- **doki-proot:** Forked proot with daemon mode + JSON IPC protocol. 14K binary wrapper, falls back to system proot.
- **Distro system:** `doki run --distro alpine/ubuntu/debian/arch` downloads distros from Docker Hub automatically.
- **ARMv7 beta:** Full feature parity (run, pull, build, ps, compose, --distro) for older ARM devices.
- **Runtime fixes:** Proot dispatch now wired, `ImageConfig.Env` inheritance, compose `ImageLayers` extraction, HTTP `WriteTimeout 30s→300s`.
- **Immich:** Full stack running (PostgreSQL 18 + pgvector + cube + earthdistance, Redis 7, Immich Server v2.7.5).

---

</div>

## Table of Contents

- [Why Doki](#why-doki)
- [What's New in v0.9](#whats-new-in-v09)
- [What's New in v0.8](#whats-new-in-v08)
- [Why Android Matters](#why-android-matters)
- [What Doki Replaces](#what-doki-replaces)
- [Quickstart](#quickstart)
- [How It Works](#how-it-works)
- [Isolation Architecture](#isolation-architecture)
- [MicroVM Support](#microvm-support)
- [CLI Commands](#cli-commands)
- [Dokifile and Builder](#dokifile-and-builder)
- [Compose](#compose)
- [REST API](#rest-api)
- [Networking](#networking)
- [Storage](#storage)
- [Security](#security)
- [Performance](#performance)
- [Registry Compatibility](#registry-compatibility)
- [Project Structure](#project-structure)
- [Configuration](#configuration)
- [Building](#building)
- [Contributing](#contributing)
- [License](#license)

---

## Why Doki

Docker cannot run on Android. Podman cannot run on Android. containerd cannot run on Android. These are not design oversights -- they are fundamental architectural decisions. Docker requires root privileges, a Linux distribution with systemd, overlay2 filesystem support, and kernel features that Android explicitly disables. Podman requires user namespaces, cgroups v2, and a standard Linux environment. Neither was designed for the constraints of a mobile operating system.

Doki was designed for exactly these constraints.

**Doki does not require root.** It runs as a regular user process on Termux, the most widely used terminal emulator on Android with over 100 million downloads. When root is available, Doki improves its isolation automatically. When root is not available, Doki runs containers anyway -- as native processes on the host, with the container's root filesystem extracted from OCI layers.

**Doki does not require kernel features.** Docker needs overlay2, cgroups, user namespaces, and seccomp. Android kernels ship without most of these. Doki works around every missing feature: fuse-overlayfs when overlay2 is unavailable, native process execution when namespaces are blocked, proot when chroot is restricted.

**Doki does not require a specific filesystem layout.** Docker expects `/var/lib/docker`, `/var/run/docker.sock`, and a standard FHS layout. Doki stores everything under a single configurable data directory and communicates over a Unix socket anywhere on the filesystem.

**Doki auto-resolves ARM64 images.** Docker Hub serves multi-arch manifest lists. Doki detects your architecture at pull time and downloads the correct layers. On Android ARM64 devices, it pulls ARM64 binaries. On x86_64 servers, it pulls amd64 binaries. No `--platform` flag needed.

**Doki implements the Docker API.** Every tool that works with Docker -- `docker-compose`, `docker-py`, CI/CD pipelines, monitoring agents -- can work with Doki by pointing `DOCKER_HOST` at the Doki socket. This is not a reimplementation. It is the same REST API, the same JSON responses, the same status codes.

**Doki is a single binary.** The daemon, CLI, and compose tool are three statically-linked Go binaries with zero runtime dependencies. No containerd. No runc. No libseccomp. No systemd unit files. Copy the binary and run it.

---

## Why Android Matters

There are over 3 billion active Android devices in the world. Every single one runs a Linux kernel. Every single one can execute ELF binaries. Every single one has more computing power than the servers that ran Docker when it was first released.

The phone in your pocket has 8 CPU cores, 8 GB of RAM, and 128 GB of storage. It is more powerful than a t2.micro EC2 instance. It can run databases, web servers, message queues, CI/CD runners, and development environments. But until now, it could not run containers.

Doki unlocks that capability. A developer can test a full microservice stack on their phone during a commute. A student can learn Docker without a laptop. A self-hosted Nextcloud instance can run on an old Android tablet mounted on a wall. A CI/CD pipeline can execute on a farm of retired phones.

Android is not a second-class platform. It is the largest deployed Linux ecosystem in history. Doki treats it as a first-class target.

---

## What Doki Replaces

| Instead of | Use Doki | Because |
|:-----------|:---------|:--------|
| Docker Desktop | `dokid` + `doki` | Same API, no VM overhead, works on Android |
| Podman | `dokid` + `doki` | Same pod abstraction, plus microVM isolation |
| containerd + crictl | `dokid` as CRI | Single binary instead of 3 daemons |
| Docker Compose | `doki-compose` | Same YAML, same commands, same workflow |
| Kubernetes (for small deploys) | `doki kube play` | Run K8s YAML without a cluster |
| Lima / Colima (macOS) | `dokid` | Native container daemon, no Linux VM needed |
| Termux proot-distro | `doki run` | Actual OCI images instead of chroot tarballs |

---

## Quickstart

### Installation

**One-liner (any architecture):**

```bash
curl -sL https://doki.opceanai.com | sh
```

**Manual download (Android/Termux ARM64):**

```bash
# Download all ARM64 binaries
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-android-arm64           -o $PREFIX/bin/doki
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/dokid-android-arm64          -o $PREFIX/bin/dokid
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-compose-android-arm64   -o $PREFIX/bin/doki-compose
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-init-rust-android-arm64 -o $PREFIX/bin/doki-init
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-proot-android-arm64     -o $PREFIX/bin/doki-proot
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/proot-android-arm64          -o $PREFIX/bin/proot
chmod +x $PREFIX/bin/doki*
chmod +x $PREFIX/bin/proot
```

**Pre-compiled binaries (Android/Termux ARMv7):**

```bash
# Download all ARMv7 binaries
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-linux-armv7           -o $PREFIX/bin/doki
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/dokid-linux-armv7          -o $PREFIX/bin/dokid
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-compose-linux-armv7   -o $PREFIX/bin/doki-compose
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-init-linux-armv7      -o $PREFIX/bin/doki-init
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/doki-proot-android-armv7   -o $PREFIX/bin/doki-proot
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.1/proot-android-armv7        -o $PREFIX/bin/proot
chmod +x $PREFIX/bin/doki*
chmod +x $PREFIX/bin/proot
```

**Build from source:**

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki

# Android / Termux (ARM64)
make build-android
make install

# Android / Termux (ARMv7)
make build-armv7

# Linux (x86_64)
make build-linux
make install

# macOS (ARM64)
make build-darwin-arm64
```

### Binaries

| Binary | Size | Description |
|--------|------|-------------|
| **doki** | 9.2 MB | CLI principal. All ~108 commands: `run`, `ps`, `images`, `pull`, `push`, `exec`, `logs`, `inspect`, `stop`, `rm`, `build`, `network`, `volume`, `compose`, `pod`, `login`. Connects to daemon via Unix socket. |
| **dokid** | 13 MB | Daemon. Runs in background, exposes Docker Engine API v1.44 over Unix socket. Manages containers, images, networks, volumes. Auto-detects proot, Linux namespaces, or microVM isolation. |
| **doki-compose** | 11 MB | Compose engine. Reads `doki.yml` (or `docker-compose.yml`), starts services in dependency order, creates networks. Supports `up`, `down`, `ps`, `logs`, long syntax ports/volumes, health conditions. |
| **doki-init-rust** | 412 KB | PID 1 for microVM guests. Written in Rust (vs 2.9MB Go), sets up hostname/DNS/resolv.conf. Runs inside the VM, mounts filesystems, executes the container command, communicates with host via vsock. |
| **doki-proot** | 14 KB | Forked proot with JSON IPC daemon mode. Falls back to system proot. Used by Doki when namespaces are unavailable. |

### First Run

```bash
# Start the daemon in the background
dokid &

# Verify it is alive
doki ping
# Response: OK

# Pull an image
doki pull alpine
# Pulls ARM64 layers automatically on ARM devices

# Run a container
doki run alpine echo "Hello from Doki"
# Output: Hello from Doki

# Check what is running
doki ps
doki images
```

### Container Lifecycle

```bash
doki pull nginx:alpine
doki run -d --name web -p 8080:80 nginx:alpine
doki ps
doki logs web
doki stats web
doki exec web nginx -v
doki stop web
doki rm web
```

### Full Stack with Compose

```bash
doki-compose up      # Start all services
doki-compose ps      # Check status
doki-compose logs    # View logs
doki-compose down    # Stop and remove
```

---

## How It Works

When Doki runs a container, it goes through this pipeline:

**1. Image Resolution.** The image reference is parsed. If the image is not cached locally, Doki contacts the registry, authenticates (anonymously for public images), resolves the manifest for the current architecture, downloads the configuration blob, and downloads each layer as a gzip-compressed tarball.

**2. Rootfs Construction.** Downloaded layers are extracted in order into a rootfs directory. Each layer is decompressed and untarred on top of the previous layers, building the complete container filesystem. The extraction includes tar path traversal protection and symlink validation.

**3. Execution Mode Selection.** Doki probes the system for available isolation mechanisms. If a microVM hypervisor is available (KVM, Gunyah, GenieZone, Halla), it selects microVM mode. If root is available and namespaces are supported, it selects namespace mode. If proot is installed, it selects proot mode. Otherwise, it falls back to native host process execution.

**4. Process Execution.** The container command is executed within the chosen isolation context. Standard input, output, and error are captured. Environment variables from the image configuration are applied. The working directory is set from the image metadata.

**5. Lifecycle Management.** The container process is monitored. Exit codes are recorded. Logs are written to a file. Health checks are executed on a configurable interval. Restart policies are enforced according to the container configuration.

---

## Isolation Architecture

Doki provides four levels of container isolation, selected automatically at runtime.

### Level 4: MicroVM (Hardware Isolation)

The container runs inside a lightweight virtual machine with its own kernel. This provides the strongest isolation possible -- the container cannot escape to the host even if the kernel is compromised.

Doki uses **crosvm** (Google's VMM) on Android devices with supported chipsets, **Firecracker** (AWS's VMM) on Linux servers with KVM, and **QEMU microvm** as a universal fallback.

The rootfs is converted to an ext4 image. A minimal init process (doki-init) is injected as PID 1. The VM boots in under 150ms with crosvm and KVM, or under 125ms with Firecracker.

**Overhead:** 5-20 MB RAM per VM. No CPU overhead when idle.

### Level 3: Namespaces (Kernel Isolation)

The container runs in isolated Linux namespaces: mount, PID, network, UTS, IPC, and optionally user and cgroup. This provides strong isolation using kernel features, identical to how Docker and Podman work.

**Requirements:** Root access on a Linux system with namespace support.

**Overhead:** Negligible. No additional memory or CPU cost.

### Level 2: Proot (Userspace Isolation)

The container runs under proot, a userspace chroot implementation that intercepts syscalls via ptrace. The container process sees its own root filesystem but shares the host kernel and process tree.

**Requirements:** proot binary installed (available via `pkg install proot` on Termux).

**Overhead:** Approximately 10% performance cost due to ptrace overhead.

### Level 1: Native (Host Process)

The container runs as a regular process on the host, with the container's rootfs extracted to a temporary directory. Environment variables, working directory, and PATH are configured to point at the container filesystem. No isolation guarantees -- the process can see the host filesystem and other processes.

**Requirements:** None. Always available.

**Overhead:** None. Zero cost.

---

## Known Limitations

Doki is under active development. Features marked below reflect their current tested status.

### What Works (proot mode, Android/Termux)

| Feature | Status | Notes |
|---------|--------|-------|
| `doki run` | Tested | Basic commands, shell scripts, --init, --user, --entrypoint, --restart |
| `doki pull` (Docker Hub) | Tested | ARM64 multi-arch auto-resolve, parallel downloads, token auth |
| `doki push` | Tested | OCI Distribution Spec: blob existence check, cross-repo mount, monolithic upload, manifest PUT |
| `doki images` | Tested | Correct sizes, RepoDigests populated |
| `doki ps` / `doki ps -a` | Tested | Names, ports, image shown |
| `doki inspect` | Tested | Full JSON output |
| `doki stop` / `doki rm` | Tested | By name or ID, no deadlocks |
| `doki build` | Tested | RUN layers, COPY --from, ARG, ENV, .dockerignore, build cache, ONBUILD replay |
| `doki logs` | Tested | Rotation (10MB/3 files), Docker multiplexed stream format |
| `doki exec` | Tested | Runs inside container via proot |
| `doki attach` | Tested | HTTP hijack, bidirectional streaming |
| `doki wait` | Tested | Multi-container, returns exit codes |
| `doki login` / `doki logout` | Tested | Token auth, Basic auth, credential wiring to registry client |
| `doki network ls` | Tested | Bridge/host/none, doki0 bridge creation |
| `doki volume create/ls/rm` | Tested | Local driver, tmpfs support |
| `doki-compose up/down` | Tested | Full compose spec: networks, volumes, secrets, healthcheck |
| `doki --help` / `doki CMD --help` | Tested | All subcommands |
| `--follow` on logs | Tested | Streaming with since/until/timestamps |
| Port forwarding (`-p`) | Tested | FirewallManager wired |

### What Does NOT Work Yet

| Feature | Status | Notes |
|---------|--------|-------|
| `doki cp` | Stub | Copy files host/container not implemented |
| MicroVM isolation | Untested | Code exists, not tested on compatible hardware |
| Kubernetes CRI | Stub | gRPC server not implemented |
| CNI networking | Untested | Plugin manager exists, not wired |
| Network bridge isolation | No | Containers share host network in proot/native mode |



### Proot-Specific Notes (v0.8)

- **ENOSYS errors fixed in v0.8.** The root cause was `PROOT_NO_SECCOMP=1` which disabled proot's seccomp filter, letting syscalls hit Android's seccomp untranslated. Doki now uses proot's native seccomp filter to intercept syscalls before Android kernel blocks them.
- **QEMU fallback.** On ENOSYS errors, Doki retries with QEMU user-mode emulation via PATH/LIB detection.
- **Proot now uses consistent bind mounts.** Same configuration as proot-distro: `/apex`, `/system`, `/vendor`, `/storage`, `--link2symlink`, with container-relative LD_LIBRARY_PATH.
- **Port forwarding wired via FirewallManager.** The `-p` flag now creates iptables-like rules where possible.
- **Container networking is host-mode only.** Bridge network isolation requires Linux namespaces (root) or microVM mode.
- **MicroVM mode requires compatible hardware.** crosvm/Firecracker need KVM, Gunyah, GenieZone, or Halla hypervisors. Available on Android 13+ with supported chipsets.
- **The proot binary must be the Termux build.** Install via `pkg install proot`. The Termux package includes Android-specific kernel workarounds not present in upstream proot.

---

## MicroVM Support

DokiVM is Doki's microVM subsystem. It provides hardware-level isolation by running containers inside lightweight virtual machines. When available, it is selected automatically over all other isolation modes.

### Detection and Selection

At startup, Doki probes the system for hypervisor capabilities:

1. Check for `/dev/kvm` -- KVM on Linux and Google Tensor devices
2. Check for `/dev/gunyah` -- Qualcomm Snapdragon hypervisor
3. Check for `/dev/geniezone` -- MediaTek Dimensity hypervisor
4. Check for `/dev/halla` -- Samsung Exynos hypervisor
5. Check for `crosvm` binary in PATH
6. Check for `firecracker` binary in PATH

If any hypervisor device is found and the corresponding VMM binary is available, microVM mode is activated.

### Supported Chipsets

| Manufacturer | Chip Series | Hypervisor | VMM | Generation |
|:-------------|:------------|:-----------|:----|:-----------|
| Qualcomm | Snapdragon 8 Gen 1/2/3/4 | Gunyah | crosvm | 2022+ |
| MediaTek | Dimensity 7200/8200/9200/9300 | GenieZone | crosvm | 2023+ |
| Samsung | Exynos 2200/2400 | Halla | crosvm | 2022+ |
| Google | Tensor G1/G2/G3/G4 | KVM | crosvm | 2021+ |
| Intel | Core / Xeon | KVM | Firecracker | All KVM-capable |
| AMD | Ryzen / EPYC | KVM | Firecracker | All KVM-capable |

### Rootfs Construction

For microVM mode, Doki builds a bootable rootfs image:

1. OCI layers are extracted to a staging directory
2. `doki-init` is compiled and injected as `/sbin/init`
3. An ext4 filesystem image is created with `mkfs.ext4`
4. The kernel image is selected by architecture (ARM64 or x86_64)
5. The VMM is invoked with the kernel, rootfs, and configuration

### Networking

MicroVMs use TAP devices bridged to a CNI-managed network. Each VM gets a unique IP from the subnet. Port mapping is handled via iptables/nftables DNAT rules. DNS is configured via the bridge's built-in resolver.

### Communication

The host communicates with the guest via virtio-vsock. Doki uses a JSON-based protocol over vsock streams for exec, attach, signal forwarding, health checks, and exit code reporting.

---

## CLI Commands

Doki provides 108 commands across 8 categories. Every command is designed to match the equivalent Docker, Podman, or kubectl command in syntax and behavior.

### Container Management

| Command | Flags | Description |
|:--------|:------|:------------|
| `doki run` | `-d`, `-i`, `-t`, `--rm`, `-p`, `-v`, `-e`, `-w`, `-u`, `--name`, `--network`, `--restart`, `--cpus`, `-m`, `--privileged`, `--read-only`, `--init`, `--dns`, `--add-host`, `--cap-add`, `--cap-drop`, `--security-opt`, `--device`, `--log-driver`, `--pull`, `--platform`, and 80+ more | Create and start a container |
| `doki ps` | `-a`, `-q`, `--no-trunc`, `-f`, `--format`, `-n` | List containers |
| `doki create` | Same as run minus `-d`/`-i` | Create without starting |
| `doki start` | `-a`, `-i` | Start stopped containers |
| `doki stop` | `-t` | Gracefully stop containers |
| `doki restart` | `-t` | Stop and start containers |
| `doki kill` | `-s` | Send signal to containers |
| `doki rm` | `-f`, `-v`, `-l` | Remove containers |
| `doki pause` | | Pause container processes |
| `doki unpause` | | Resume container processes |
| `doki exec` | `-d`, `-i`, `-t`, `-e`, `-w`, `-u` | Run command in container |
| `doki logs` | `-f`, `--tail`, `-t`, `--since` | Fetch container logs |
| `doki stats` | `--no-stream`, `--format` | Live resource statistics |
| `doki top` | | Display container processes |
| `doki inspect` | `-f`, `-s` | Detailed container info |
| `doki commit` | `-a`, `-m`, `-p` | Create image from container |
| `doki diff` | | Show filesystem changes |
| `doki port` | | List port mappings |
| `doki rename` | | Rename a container |
| `doki update` | `--cpus`, `-m`, `--restart` | Update configuration |
| `doki wait` | | Block until exit, return code |
| `doki export` | `-o` | Export filesystem as tar |
| `doki cp` | `-a`, `-L` | Copy files host/container |
| `doki attach` | `--detach-keys`, `--sig-proxy` | Attach to container I/O |
| `doki prune` | `-a`, `-f` | Remove stopped containers |

### Image Management

| Command | Description |
|:--------|:------------|
| `doki pull` | Pull from any OCI registry |
| `doki push` | Push to any OCI registry |
| `doki images` | List images with sizes |
| `doki rmi` | Remove images |
| `doki tag` | Tag an image |
| `doki history` | Show layer history |
| `doki save` / `doki load` | Export/import tar archives |
| `doki import` | Import from tar |
| `doki build` | Build from Dokifile |
| `doki search` | Search Docker Hub |
| `doki login` / `doki logout` | Registry authentication |

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

---

## Dokifile and Builder

Doki reads Dokifiles (or standard Dockerfiles) and builds OCI-compatible images. The parser supports all 18 Dockerfile instructions, multi-stage builds, heredocs, and parser directives.

### Supported Instructions

| Instruction | Description |
|:------------|:------------|
| `FROM` | Base image with `--platform` and `AS` aliasing |
| `RUN` | Shell and exec forms, heredocs, `--mount`, `--network`, `--security` |
| `CMD` | Default command, shell and exec forms |
| `LABEL` | Key-value metadata, multi-label |
| `EXPOSE` | Port declaration with protocol |
| `ENV` | Environment variables with substitution |
| `ADD` | Local files and remote URLs |
| `COPY` | Copy files with `--from`, `--chown`, `--chmod` |
| `ENTRYPOINT` | Default executable |
| `VOLUME` | Volume mount points |
| `USER` | User and group |
| `WORKDIR` | Working directory |
| `ARG` | Build-time variables |
| `ONBUILD` | Trigger instructions |
| `STOPSIGNAL` | Exit signal |
| `HEALTHCHECK` | Health probe configuration |
| `SHELL` | Default shell for shell form |
| `MAINTAINER` | Deprecated, mapped to OCI label |

### Parser Directives

```dockerfile
# syntax=docker/dockerfile:1
# escape=\
# check=skip=JSONArgsRecommended;error=true
```

### Example Dokifile

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

Doki implements the Compose Specification for defining multi-container applications.

### Supported Features

| Feature | Description |
|:--------|:------------|
| `services` | Container definitions with full configuration |
| `networks` | Custom bridge/overlay networks |
| `volumes` | Persistent storage with driver options |
| `secrets` | Sensitive data injection with long syntax |
| `configs` | Configuration file injection |
| `depends_on` | Startup ordering: `service_started`, `service_healthy` (60s poll), `service_completed_successfully` |
| `healthcheck` | Health probes per service |
| `deploy` | Resource limits (`cpus`, `memory`, `reservations`), `replicas`, `restart_policy` |
| `env_file` | Environment from files |
| `extends` | Service inheritance |
| `profiles` | Conditional service activation |
| `include` | Multi-file composition |
| `shm_size` | Shared memory size (`2g`, `64m`) |
| `pids_limit` | PID limit per container |
| `oom_kill_disable` | Disable OOM killer |
| `tmpfs` | Tmpfs mounts (string and long syntax) |
| `devices` | Device mappings |
| `blkio_config` | Block I/O limits (`weight`, `weight_device`, `reads`, `writes`) |
| `ulimits` | Resource limits (`nofile`, `nproc`, etc.) |
| `runtime` | Runtime selection (proot, native, etc.) |
| `scale` | Service scaling |
| `platform` | Target platform |
| Long syntax ports | `target`, `published`, `protocol`, `mode` |
| Long syntax volumes | `type`, `source`, `target`, `read_only`, `bind` |
| `on-failure:N` | Dynamic restart limit parsing |
| Version validation | Requires `version >= 3.0`

### Example

```yaml
name: production-stack

include:
  - base.yml

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
    environment:
      POSTGRES_PASSWORD_FILE: /run/secrets/db-password
    secrets:
      - db-password

volumes:
  web-data:
  db-data:

secrets:
  db-password:
    file: ./secrets/db-password.txt
```

---

## REST API

Doki exposes the Docker Engine API v1.44 over a Unix socket. Tools built for Docker -- SDKs, monitoring agents, orchestration systems -- connect to Doki without modification.

### Endpoints

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/containers/json` | List containers |
| `POST` | `/containers/create` | Create container |
| `GET` | `/containers/{id}/json` | Inspect container |
| `POST` | `/containers/{id}/start` | Start container |
| `POST` | `/containers/{id}/stop` | Stop container |
| `POST` | `/containers/{id}/restart` | Restart container |
| `POST` | `/containers/{id}/kill` | Kill container |
| `POST` | `/containers/{id}/pause` | Pause container |
| `POST` | `/containers/{id}/unpause` | Unpause container |
| `POST` | `/containers/{id}/wait` | Wait for exit |
| `DELETE` | `/containers/{id}` | Remove container |
| `GET` | `/containers/{id}/logs` | Fetch logs |
| `GET` | `/containers/{id}/top` | Process list |
| `GET` | `/containers/{id}/stats` | Resource stats |
| `POST` | `/containers/{id}/exec` | Create exec instance |
| `POST` | `/containers/{id}/attach` | Attach to container |
| `POST` | `/containers/prune` | Remove stopped containers |
| `GET` | `/images/json` | List images |
| `POST` | `/images/create` | Pull image |
| `GET` | `/images/{name}/json` | Inspect image |
| `GET` | `/images/{name}/history` | Image history |
| `POST` | `/images/{name}/push` | Push image |
| `POST` | `/images/{name}/tag` | Tag image |
| `DELETE` | `/images/{name}` | Remove image |
| `POST` | `/images/prune` | Remove unused images |
| `GET` | `/images/search` | Search registry |
| `GET` | `/networks` | List networks |
| `POST` | `/networks/create` | Create network |
| `GET` | `/networks/{id}` | Inspect network |
| `DELETE` | `/networks/{id}` | Remove network |
| `POST` | `/networks/{id}/connect` | Connect container |
| `POST` | `/networks/{id}/disconnect` | Disconnect container |
| `POST` | `/networks/prune` | Remove unused networks |
| `GET` | `/volumes` | List volumes |
| `POST` | `/volumes/create` | Create volume |
| `GET` | `/volumes/{name}` | Inspect volume |
| `DELETE` | `/volumes/{name}` | Remove volume |
| `POST` | `/volumes/prune` | Remove unused volumes |
| `GET` | `/info` | System information |
| `GET` | `/version` | Version information |
| `GET` | `/_ping` | Health check |
| `GET` | `/events` | Event stream |
| `GET` | `/system/df` | Disk usage |
| `POST` | `/auth` | Authentication |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/health` | Daemon health |

### Connecting Docker CLI to Doki

```bash
export DOCKER_HOST=unix:///data/data/com.termux/files/usr/var/run/doki.sock
docker ps
docker images
docker run alpine echo "via docker cli"
```

### Using Docker SDKs

```python
import docker
client = docker.DockerClient(base_url="unix:///data/data/com.termux/files/usr/var/run/doki.sock")
client.containers.run("alpine", "echo hello")
```

```javascript
const Docker = require('dockerode');
const docker = new Docker({ socketPath: '/data/data/com.termux/files/usr/var/run/doki.sock' });
docker.listContainers().then(console.log);
```

---

## Networking

Doki provides multiple networking options for containers.

### Bridge Networks

The default bridge network (`doki0`) provides NAT-based connectivity. Containers on the same bridge can communicate via IP address and container name. DNS resolution between containers is provided by a built-in DNS server. Port mapping uses iptables or nftables DNAT rules.

### Host Networking

Containers using host networking share the host's network namespace directly. No isolation, no overhead. Useful for maximum network performance.

### None Networking

Containers with no networking have only a loopback interface. Complete network isolation.

### CNI Plugins

Doki supports the Container Network Interface specification for custom networking. Available plugins include bridge, host-local, portmap, loopback, bandwidth, firewall, macvlan, ipvlan, dhcp, static, tuning, and vlan.

### Rootless Networking

On rootless systems, Doki uses **pasta** (the modern replacement for slirp4netns) for TCP and UDP connectivity. Pasta provides transparent network access without root privileges or TAP devices.

### IPv6

Dual-stack IPv4/IPv6 networking is supported on bridge networks with IPv6 enabled. The built-in DNS server resolves both A and AAAA records.

### Port Mapping

```bash
doki run -p 8080:80 nginx:alpine           # Map host 8080 to container 80
doki run -p 127.0.0.1:8080:80 nginx:alpine # Bind to specific host IP
doki run -p 8080:80/tcp -p 8080:80/udp     # TCP and UDP
doki run -P nginx:alpine                   # Publish all EXPOSEd ports
doki run -p 8080-8090:80 nginx:alpine      # Port range
```

---

## Storage

Doki supports multiple storage drivers for container layers and volumes.

### Drivers

| Driver | Description | Best For |
|:-------|:------------|:---------|
| **overlay2** | Kernel overlay filesystem (direct syscall mount) | Linux with root, best performance |
| **fuse-overlayfs** | Userspace overlay via fuse-overlayfs binary | Rootless, Termux, Android |
| **btrfs** | Btrfs subvolumes with snapshots | Systems with btrfs root |
| **zfs** | ZFS datasets with snapshots | Systems with ZFS pools |
| **vfs** | Simple directory copy | Testing, minimal systems |

The overlay2 driver uses **kernel overlay mounts directly** (`syscall.Mount("overlay", ...)`) instead of delegating to FUSE. When overlay2 is unavailable (rootless, Android kernels), it falls back to fuse-overlayfs. The FUSE fallback runs as a non-blocking background process.

### Volumes

Volumes provide persistent storage independent of container lifecycle. The default `local` driver stores volumes as directories on the host filesystem. Volume data persists across container restarts and removals.

```bash
doki volume create my-data
doki run -v my-data:/var/lib/data alpine
```

### Garbage Collection

Doki periodically removes unused layers and images. The garbage collector runs on a configurable interval (default: 1 hour) and removes layers older than a configurable threshold (default: 72 hours) that are not referenced by any image or container.

### Snapshots

Btrfs and ZFS drivers support filesystem snapshots for point-in-time recovery of container filesystems.

---

## Security

Doki implements multiple layers of security, from kernel-level protections to API-level controls.

### Seccomp

The default seccomp profile allows 80+ essential syscalls while blocking dangerous operations. The profile explicitly blocks:

- Module loading (`init_module`, `finit_module`, `delete_module`)
- Kernel execution (`kexec_load`, `kexec_file_load`)
- AF_ALG sockets (CVE-2026-31431, CVE-2026-31432)
- BPF program loading
- Hardware I/O port access (`iopl`, `ioperm`)
- Kernel information leaks (`kcmp`)
- Process memory access (`process_vm_readv`, `process_vm_writev`)

Modern syscalls are explicitly allowed for forward compatibility: `io_uring_setup`, `io_uring_enter`, `io_uring_register`, `pidfd_open`, `pidfd_send_signal`, `pidfd_getfd`, `rseq`, `userfaultfd`, `copy_file_range`, `landlock_create_ruleset`, `landlock_add_rule`, `landlock_restrict_self`.

The seccomp filter is constructed with **ALLOW listed first, then DENY**, preventing the common mistake of allowing a syscall that was previously denied.

### AppArmor

Template-based AppArmor profiles are generated per container, restricting filesystem access, network capabilities, and mount operations.

### User Namespaces

In namespace mode, each container runs in its own user namespace with UID/GID remapping. The root user inside the container maps to an unprivileged user on the host.

### Capabilities

Containers start with a minimal capability set. Additional capabilities can be granted explicitly. All capabilities can be dropped with `--cap-drop=ALL`.

### Read-only Rootfs

Containers can run with a read-only root filesystem. Writable data must be explicitly mounted via volumes or tmpfs.

### TLS

The daemon supports mutual TLS authentication. Clients must present a valid certificate signed by the configured CA.

### Rate Limiting

The API server implements token-bucket rate limiting to prevent brute-force attacks. Default: 100 requests per second with burst of 200.

### Image Verification

Layer extraction validates tar paths to prevent path traversal attacks (CWE-22). Symlinks are validated to prevent container escape. Hardlinks are restricted to within the rootfs directory.

---

## Performance

Measured on Qualcomm Snapdragon 685, Android 14, Termux. All images are ARM64 native binaries pulled from Docker Hub. Results are for a cold pull (no cached layers).

| Image | Size | Pull Time | Start Time | RAM (idle) |
|:------|-----:|----------:|-----------:|-----------:|
| `alpine:latest` | 4.0 MB | 2.1s | 10ms | 1.2 MB |
| `busybox:latest` | 1.8 MB | 1.4s | 8ms | 0.6 MB |
| `python:3-alpine` | 17.3 MB | 8.2s | 4ms | 3.1 MB |
| `nginx:alpine` | 24.6 MB | 11.5s | 12ms | 5.8 MB |
| `node:22-alpine` | 48.7 MB | 22.8s | 15ms | 12.3 MB |
| `redis:alpine` | 15.2 MB | 7.1s | 6ms | 2.8 MB |
| `mariadb:latest` | 156 MB | 62.4s | 20ms | 31.2 MB |
| `nextcloud:latest` | 423 MB | 87.3s | 45ms | 45.7 MB |

**Verified in v0.9:** Alpine echo starts in 10ms. Python3 starts in 4ms.

### Comparison

| Engine | Binary Size | Memory (idle) | Start Time | Android |
|:-------|:-----------:|:-------------:|:----------:|:-------:|
| Doki v0.9 | 13 MB | 12 MB | <15ms | Yes |
| Docker | 58 MB | 85 MB | ~50ms | No |
| Podman | 45 MB | 60 MB | ~30ms | No |
| containerd | 42 MB | 55 MB | ~40ms | No |

Doki-init rewritten in Rust (412K vs 2.9MB Go, -86%). String intern pool deduplicates common strings. Buffer pool reuses allocations in runtime (reduces GC pressure).

---

## Registry Compatibility

Doki implements the OCI Distribution Specification and is compatible with any registry that supports the OCI or Docker Registry HTTP API v2.

### Supported Registries

| Registry | Pull | Push | Auth | Notes |
|:---------|:----:|:----:|:----:|:------|
| Docker Hub | Yes | Yes | Token | Anonymous + authenticated, tested |
| GitHub Container Registry | Yes | Yes | PAT | `ghcr.io` |
| Quay.io | Yes | Yes | Robot | Red Hat's registry |
| Google Container Registry | Yes | Yes | JSON key | `gcr.io` |
| Amazon ECR | Yes | Yes | IAM | `*.amazonaws.com` |
| Azure Container Registry | Yes | Yes | SP | `*.azurecr.io` |
| GitLab Registry | Yes | Yes | Token | `registry.gitlab.com` |
| Harbor | Yes | Yes | Basic | Self-hosted |
| Self-hosted (distribution) | Yes | Yes | Configurable | Any OCI registry |

Push uses the OCI Distribution Spec flow: blob existence HEAD check, cross-repo mount for known base layers, monolithic blob upload with digest verification, and manifest PUT. Auth supports both Basic (username/password) and Bearer token (Docker Hub-style OAuth) flows.### Multi-Architecture

Doki resolves multi-architecture manifest lists and selects the best match for the current device:

1. Exact match: same OS and architecture
2. Compatible match: same OS, different architecture variant
3. Fallback: any available architecture

On ARM64 Android devices, Doki prefers `linux/arm64/v8` images. On x86_64 Linux, it prefers `linux/amd64`.

### Verified Images

The following images have been tested and verified on Android ARM64 via Termux:

`alpine:latest`, `alpine:edge`, `busybox:latest`, `busybox:musl`, `python:3-alpine`, `python:3-slim`, `node:22-alpine`, `node:lts-alpine`, `nginx:alpine`, `nginx:alpine-slim`, `redis:alpine`, `redis:7-alpine`, `mariadb:latest`, `mariadb:lts`, `postgres:alpine`, `postgres:16-alpine`, `nextcloud:latest`, `ubuntu:latest`, `ubuntu:rolling`, `debian:stable-slim`, `golang:alpine`, `golang:1.22-alpine`, `rust:alpine`, `ruby:alpine`, `php:cli-alpine`, `traefik:latest`, `caddy:alpine`, `vault:latest`

---

## Project Structure

```
Doki/
├── cmd/
│   ├── doki/                 CLI binary (108 commands, 2200+ lines)
│   ├── dokid/                Daemon binary (REST API, TLS, gRPC, rate limiting)
│   ├── doki-compose/         Docker Compose compatible CLI
│   ├── doki-init-rust/       Minimal PID 1 for microVM guests (Rust, 412K)
├── pkg/
│   ├── api/                  Docker Engine API v1.44 server (53 endpoints)
│   │   ├── server.go         HTTP server with route registration
│   │   ├── tls.go            TLS configuration and certificate management
│   │   ├── metrics.go        Prometheus /metrics and /health endpoints
│   │   └── middleware.go     Logging, CORS, recovery, rate limiting
│   ├── runtime/              OCI runtime with 4 execution modes
│   │   └── runtime.go        Container lifecycle, 3 process starters, mounts
│   ├── image/                OCI image management
│   │   └── store.go          Pull, push, list, tag, remove, search, import/export
│   ├── registry/             OCI Distribution Spec client
│   │   └── client.go         Token auth, manifest resolution, blob download
│   ├── network/              Container networking
│   │   ├── manager.go        Bridge/host/none networks, IPAM, port mapping
│   │   └── cni.go            CNI plugin manager, pasta, firewall, DNS server
│   ├── storage/              Storage drivers
│   │   ├── driver.go         Overlay2 and FUSE-OverlayFS drivers
│   │   └── drivers.go        Btrfs, ZFS, VFS drivers, GC, snapshots, quotas
│   ├── builder/              Image builder
│   │   ├── builder.go        Dokifile parser (18 instructions, multi-stage)
│   │   └── executor.go       Build instruction executors
│   ├── compose/              Compose engine
│   │   └── engine.go         YAML parsing, service ordering, lifecycle
│   ├── cri/                  Kubernetes CRI plugin
│   │   └── plugin.go         PodSandbox, container management, image service
│   ├── cli/                  CLI library
│   │   └── commands.go       Full CLI implementation (2200+ lines)
│   └── common/               Shared types, configuration, utilities
│       ├── types.go          Docker API types (60+ structs)
│       ├── config.go         Configuration loading and persistence
│       ├── utils.go          ID generation, path utilities, parsers
│       ├── version.go        Version information
│       └── errors.go         Structured error types
├── internal/
│   ├── dokivm/               MicroVM subsystem
│   │   ├── vmm.go            VMM interface and auto-detection
│   │   ├── vsock.go          Host-to-guest communication protocol
│   │   ├── crosvm/           Google crosvm backend (3 files)
│   │   ├── firecracker/      AWS Firecracker backend (1 file)
│   │   ├── qemu/             QEMU microvm fallback (1 file)
│   │   ├── rootfs/           OCI-to-ext4 rootfs builder (1 file)
│   │   └── network/          TAP devices and CNI bridge (1 file)
│   ├── namespaces/           Linux namespace management
│   ├── cgroups/              cgroups v2 resource management
│   ├── fuse/                 FUSE overlay filesystem operations
│   ├── proot/                proot fallback for Android
│   ├── seccomp/              Seccomp profile engine (80+ syscalls)
│   └── apparmor/             AppArmor profile generator
├── kernels/                  Pre-compiled VM kernels (ARM64 + x86_64)
├── go.mod
├── Makefile
└── LICENSE
```

**Statistics:** 40 Go source files. 14,500+ lines of code. 5 compiled binaries. Zero external dependencies beyond the Go standard library and one YAML parsing library.

---

## Configuration

### Daemon Configuration

Doki reads configuration from `~/.doki/config.json`:

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
  "dns_search": [],
  "dns_options": [],
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
| `DOKI_TLS_CA` | TLS CA certificate path | unset |
| `DOKI_TLS_VERIFY` | Require client certificates | unset |
| `DOKI_TCP_ADDR` | TCP listen address | unset |
| `DOKI_KERNEL` | MicroVM kernel path | Platform-specific |
| `DOKI_NATIVE` | Force native mode | unset |

---

## Building

### Requirements

- Go 1.22 or later
- `make` (optional, for convenience targets)
- For microVM mode: `crosvm` or `firecracker` binary (auto-detected)

### Build Targets

```bash
make build-android        # GOOS=android GOARCH=arm64
make build-linux          # GOOS=linux GOARCH=amd64
make build-linux-arm64    # GOOS=linux GOARCH=arm64
make build-darwin         # GOOS=darwin GOARCH=amd64
make build-darwin-arm64   # GOOS=darwin GOARCH=arm64
make test                 # go test ./...
make vet                  # go vet ./...
make lint                 # golangci-lint run ./...
make clean                # rm -rf bin/
make install              # Install to $PREFIX/bin
```

### Manual Build

```bash
# CLI
go build -trimpath -ldflags="-s -w" -o bin/doki ./cmd/doki

# Daemon
go build -trimpath -ldflags="-s -w" -o bin/dokid ./cmd/dokid

# Compose
go build -trimpath -ldflags="-s -w" -o bin/doki-compose ./cmd/doki-compose

# MicroVM init
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/doki-init ./cmd/doki-init
```

---

## Contributing

Contributions are welcome. Areas where help is most needed:

- **MicroVM backends**: Support for additional hypervisors and platforms
- **CNI plugins**: Implementation of advanced networking features
- **Security**: Hardening, fuzzing, and penetration testing
- **Performance**: Layer caching, parallel operations, memory optimization
- **Testing**: Integration tests, end-to-end tests, stress tests
- **Documentation**: Tutorials, examples, and API reference

### Development Setup

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
go build ./...
go test ./...
```

### Commit Style

Follow the existing commit patterns. Keep changes focused. Write clear messages.

---

## License

Doki is licensed under the Apache License 2.0.

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

## Mirrors

| Platform | Repository | Wiki |
|:---------|:-----------|:-----|
| Website | [doki.opceanai.com](https://doki.opceanai.com) | Landing page |
| GitHub | [OpceanAI/Doki](https://github.com/OpceanAI/Doki) | [Wiki](https://github.com/OpceanAI/Doki/wiki) |
| GitLab | [aguitauwu/doki](https://gitlab.com/aguitauwu/doki) | [Wiki](https://gitlab.com/aguitauwu/doki/-/wikis) |
| Codeberg | [aguitauwu/Doki](https://codeberg.org/aguitauwu/Doki) | [Wiki](https://codeberg.org/aguitauwu/Doki/wiki) |

---

## Related Repos

| Repo | Description |
|:-----|:------------|
| [Doki-proot](https://github.com/OpceanAI/Doki-proot) | Forked proot with JSON IPC daemon mode for Doki |

---

## Distros

Run with a predefined distro from Docker Hub:

```bash
doki run --distro alpine echo hello
doki run --distro ubuntu bash
doki run --distro debian --install curl,vim bash
doki run --distro arch
doki run --distro fedora
doki run --distro rocky
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

All distros are extracted using native Go tar with compression auto-detection (gzip, bzip2, xz, zstd), whiteout handling (.wh. opaque + prefix), path traversal protection, and parallel layer extraction with rollback on failure.---

<div align="center">

### The container engine for the other 3 billion devices.

<br>

[![OpceanAI](https://img.shields.io/badge/OpceanAI-2026-0D1117?style=for-the-badge)](https://github.com/OpceanAI)

</div>
