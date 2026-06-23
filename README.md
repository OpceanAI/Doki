# DOKI

<sub>[ROOTLESS CONTAINER RUNTIME / GO 1.26 / APACHE-2.0]</sub>

> Rootless runtime architecture engineered to virtualize POSIX workloads
> under Android, Linux, and macOS boundaries without requiring elevated
> capabilities, kernel patches, or daemon特权.

[![Go](https://img.shields.io/badge/go-1.26.3+-00ADD8?style=flat-square&color=24292e)](https://go.dev)
[![License](https://img.shields.io/badge/license-Apache_2.0-555?style=flat-square&color=24292e)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.11.0-0F766E?style=flat-square&color=24292e)](https://github.com/OpceanAI/Doki/releases)
[![Downloads](https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat-square&color=24292e)](https://github.com/OpceanAI/Doki/releases)

---

## Overview

Doki is a container engine for environments where Docker cannot run:
Android phones, ARM single-board computers, unprivileged Linux hosts,
and macOS developer machines. It implements the Docker Engine API
v1.54, the Podman libpod API, OCI image distribution, Compose
orchestration, and an in-process Kubernetes control plane with a
real gRPC CRI server.

The project does not claim parity with Docker, Podman, or Kubernetes.
It tracks conformance against upstream specifications and reports
honestly where gaps remain.

---

## Subsystem Taxonomy

<sub>[CORE SUBSYSTEM / INVERSION LAYER / DETERMINISTIC FOOTPRINT]</sub>

```
SUBSYSTEM            IMPLEMENTATION                    BOUNDARY
─────────────────────────────────────────────────────────────────────
Docker Engine API    HTTP/JSON over Unix socket        v1.54 endpoint set
Podman libpod API    /libpod/* mounted in daemon       libpod v5 shim
OCI Image            distribution-spec v1.1 pull/push  manifest, index, layers
Compose              YAML parser + dependency resolver  compose-spec subset
Kubernetes CRI       gRPC over Unix socket             35+6 RPCs, kubelet
Kube-proxy           iptables/nftables/userspace       DNAT, MASQUERADE, RR
DokiLink Mesh        TCP gossip + Ed25519 signatures   TLS 1.3, NaCl secretbox
NAT Traversal        STUN RFC 8489 + relay fallback     hole punching, TURN
DHT Discovery        Kademlia 160-bit, k=8, alpha=3    decentralized routing
Storage              fuse-overlayfs / vfs / overlay2   content-addressable
DNS                  miekg/dns server on :8053         A, SRV, PTR, NXDOMAIN
macOS VZ             cgo bridge to Virtualization fw   VZVirtualMachine + ObjC
```

---

## Execution Flow

<sub>[CONTAINER LIFECYCLE / DAEMON PIPELINE]</sub>

1. User invokes `doki run alpine`.
2. CLI sends `POST /containers/create` to the daemon.
3. Daemon pulls OCI layers and extracts the tar stream to a rootfs
   directory. Runner selection occurs (proot, native, gVisor, etc.).
4. Daemon responds `201 Created`. Network setup and port forwarding
   are configured.
5. CLI sends `POST /containers/{id}/start`.
6. Daemon forks the runner binary with the OCI spec. For proot:
   `proot -S rootfs /bin/sh`.
7. Container stdout streams back to the CLI via
   `GET /containers/{id}/logs`.
8. On exit, the daemon cleans up the rootfs and removes the container
   if `--rm` was specified.

---

## System Initialization

<sub>[HOST VERIFICATION / DAEMON SPAWN]</sub>

```bash
# Verify host dependencies.
doki doctor

# Start the daemon in the background.
dokid &

# Or in the foreground for log visibility.
dokid --log-level=info --log-format=text
```

<sub>[WIRE-LEVEL API INTERACTION]</sub>

```bash
# Point Docker CLI at the Doki socket.
export DOCKER_HOST=unix://$HOME/.doki/doki.sock

# Standard Docker commands work without modification.
docker ps
docker run --rm alpine echo "hello from Doki"
docker pull nginx:alpine
docker images

# Python SDK.
python3 -c "
import docker
c = docker.DockerClient(base_url='unix://$HOME/.doki/doki.sock')
print(c.info())
"
```

---

## Isolation Modes

<sub>[RUNNER REGISTRY / 12 EXECUTION BACKENDS]</sub>

```
MODE         ROOT   ISOLATION              PLATFORM
────────────────────────────────────────────────────────────
proot        no     ptrace syscall intercept  android, linux
native       no     direct exec               all
namespaces   yes    user+mount+pid+net ns     linux
chroot       no     filesystem isolation      linux, android
gvisor       no     userspace kernel           linux
microVM      yes    crosvm/firecracker/qemu    linux, android(AVF)
wasm         no     WebAssembly runtime        all
qemuuser     no     QEMU user-mode emulation   all (cross-arch)
sysbox       yes    system container nesting   linux
fex          no     FEX-Emu x86-on-ARM         arm
pkdroid      no     Android pKVM (AVF)         android 14+
legacy32     no     32-bit fallback             armv7
```

The runner registry selects the best available mode automatically.
Explicit override via `DOKI_RUNTIME=proot` or `--runtime` flag.

---

## Platform Matrix

<sub>[BINARY AVAILABILITY / SUPPORTED COMBINATIONS]</sub>

```
PLATFORM              doki   dokid   compose   kube   kubectl
────────────────────────────────────────────────────────────────
android arm64 termux   yes    yes     yes      yes    yes
android armv7 termux   yes    yes     yes      yes    yes
linux arm64            yes    yes     yes      yes    yes
linux armv7            yes    yes     yes      yes    yes
linux amd64            yes    yes     yes      yes    yes
macos arm64            yes    ---     ---      yes    yes
macos amd64            yes    ---     ---      yes    yes
```

`dokid` and `doki-compose` require Linux/Android process and
networking primitives. macOS operates as a client platform paired
with a remote daemon or VM backend.

---

## Compatibility Targets

<sub>[UPSTREAM SPEC / CONFORMANCE STATUS]</sub>

```
INTERFACE              SPECIFICATION                    STATUS
──────────────────────────────────────────────────────────────────────
Docker Engine API      docs.docker.com/reference/api    core endpoints
Compose Spec           compose-spec.io                  service lifecycle
OCI Image              github.com/opencontainers/...    pull/push/digests
OCI Distribution       github.com/opencontainers/...    multi-arch, auth
Kubernetes CRI         kubernetes.io/docs/.../cri       gRPC 41 RPCs
Podman libpod          podman.io                        /libpod/* mounted
```

---

## Configuration

<sub>[ENVIRONMENT VARIABLES / RUNTIME PARAMETERS]</sub>

```bash
# CLI socket path (overrides default).
export DOKI_HOST=unix://$HOME/.doki/doki.sock

# Docker-compatible socket (fallback when DOKI_HOST is unset).
export DOCKER_HOST=unix://$HOME/.doki/doki.sock

# Daemon data root.
export DOKI_DATA_DIR=/var/lib/doki

# Storage driver override.
export DOKI_STORAGE_DRIVER=fuse-overlayfs

# Disable DokiLink mesh.
export DOKI_LINK_MESH=0

# Enable TLS on the daemon.
export DOKI_TLS=1

# STUN servers for NAT traversal (comma-separated).
export DOKI_LINK_STUN=stun.l.google.com:19302

# Relay peer for TURN-like fallback.
export DOKI_LINK_RELAY=203.0.113.5:7432
```

<sub>[DAEMON CONFIG FILE / config.json]</sub>

```json
{
  "data_dir": "/var/lib/doki",
  "socket": "/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "log_level": "info",
  "rate_limit": 100,
  "rate_burst": 200,
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
  }
}
```

---

## DokiLink Mesh

<sub>[PEER-TO-PEER NETWORKING / ENCRYPTED GOSSIP]</sub>

DokiLink provides multi-host container networking without a central
broker. Peers discover each other via mDNS (LAN), DHT (internet),
or static configuration. All traffic is authenticated via Ed25519
signatures and optionally encrypted with TLS 1.3 or NaCl secretbox.

NAT traversal follows a four-stage sequence: (1) both peers query a
STUN server to discover their public IP and port mapping, (2) peers
exchange public addresses via the gossip protocol, (3) both peers
issue simultaneous TCP SYN packets to each other's public address
(hole punching), (4) if the NAT allows the inbound SYN through the
pinhole, a direct encrypted channel is established. If hole punching
fails, traffic falls back to a relay peer acting as a TURN proxy.

Key derivation is order-independent (both peers compute the same
shared key via sorted-pubkey SHA-256). Per-connection nonces are
seeded from crypto/rand. Replay protection uses a 5-minute timestamp
window with an LRU nonce cache (1024 entries).

---

## Diagnostics

<sub>[HOST DEPENDENCY VERIFICATION]</sub>

```bash
# Check required host tools.
doki doctor

# List all system dependencies with status.
doki deps ls

# CI gate: exit non-zero if required deps are missing.
doki deps check

# Audit Go module dependencies.
doki deps go

# Best-effort install via detected package manager.
doki deps install pasta
```

---

## Development

<sub>[BUILD / TEST / RELEASE]</sub>

```bash
# Run the full test suite.
go test ./... -count=1

# Static analysis.
go vet ./...
staticcheck ./...

# Build all platform binaries.
make build-release sha256

# Cross-compile a single target.
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -o doki ./cmd/doki
```

---

## Documentation

<sub>[REFERENCE MATERIAL]</sub>

- [Wiki](https://github.com/OpceanAI/Doki/wiki) -- Installation, Quick Start,
  Architecture, Networking, Security, Storage, CLI Reference
- [Release Notes](RELEASE_NOTES.md) -- Changelog per version
- [Compatibility Roadmap](docs/COMPATIBILITY_ROADMAP.md) -- Conformance tracking
- [Technical Report](docs/technical-report/) -- Engineering documentation

---

## Project Stance

Doki is ambitious but honest. Claims are tied to tests, not marketing.
The priority is making rootless containers on Android work correctly,
then expanding compatibility one upstream interface at a time.

---

## License

Apache-2.0. See [LICENSE](LICENSE).
