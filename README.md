<p align="center">
  <img src="https://raw.githubusercontent.com/OpceanAI/Doki/main/.github/assets/banner.svg" alt="Doki Banner" width="680">
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26.3+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"></a>
  <a href="https://github.com/OpceanAI/Doki/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-555?style=flat" alt="License"></a>
  <a href="https://github.com/OpceanAI/Doki/releases"><img src="https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat&color=0F766E" alt="Downloads"></a>
</p>

# Doki

Doki is a rootless-first container engine built for places where Docker is heavy or unavailable: Android/Termux, small Linux hosts, ARM devices, and remote client workflows. It provides a Docker-compatible daemon API for common container operations, an OCI image path, a Compose-style workflow, and experimental Kubernetes/CRI components.

Doki is not yet a full Docker, Podman, or Kubernetes replacement. The project is moving toward those interfaces deliberately, with compatibility tracked against the upstream specs instead of marketing claims.

## Current Status

| Area | Status | Notes |
|:-----|:-------|:------|
| Android/Termux runtime | Working | `proot` path is the primary supported mode. |
| Linux runtime | Working / evolving | Native, namespace, chroot, qemu-user, and related runners exist with varying coverage. |
| Docker-compatible API | Partial | Core endpoints like `ps`, `images`, `info`, `version`, volumes, networks, and lifecycle paths are implemented. |
| Docker CLI / SDK usage | Partial | Use `DOCKER_HOST=unix://...` against `dokid`; edge endpoints need more compatibility tests. |
| Compose | Partial | Common service workflows exist; full Compose Spec coverage is still roadmap work. |
| OCI registry/image flow | Partial | Pull/push and image store paths exist; conformance tests should be expanded. |
| Kubernetes / CRI | Experimental | Useful for local experiments and API exploration, not production cluster replacement. |
| macOS | Client-oriented | `doki`, `doki-kube`, and `doki-kubectl` can be client tools; local Linux container execution needs a remote daemon or VM backend. |

## Install

Pick the binary for your platform from the release page and verify checksums before installing.

```bash
curl -fsSLO https://doki.opceanai.com/releases/SHA256SUMS.txt
curl -fsSLO https://doki.opceanai.com/releases/doki-linux-arm64
sha256sum -c SHA256SUMS.txt --ignore-missing
install -m 0755 doki-linux-arm64 ~/.local/bin/doki
```

For Termux, use the Android ARM64 binary where available and run `doki doctor` before starting the daemon.

```bash
pkg install proot iptables
mkdir -p ~/.local/bin
doki doctor
dokid &
doki info
```

## Quick Start

```bash
# Diagnose host dependencies.
doki doctor

# Start the Docker-compatible daemon.
dokid --socket "$HOME/.doki/doki.sock" &

# Use the Doki CLI.
doki ps
doki images
doki pull alpine
doki run --rm alpine echo "hello from Doki"
```

Use Docker-compatible clients by pointing them at the Doki socket.

```bash
export DOCKER_HOST=unix://$HOME/.doki/doki.sock
docker ps
```

```python
import os
import docker

client = docker.DockerClient(base_url=f"unix://{os.environ['HOME']}/.doki/doki.sock")
print(client.info())
```

```javascript
const Docker = require("dockerode");
const docker = new Docker({ socketPath: `${process.env.HOME}/.doki/doki.sock` });
docker.listImages().then(console.log);
```

## Binaries

| Binary | Purpose |
|:-------|:--------|
| `doki` | Main CLI for containers, images, volumes, networks, pods, kube helpers, mesh, and diagnostics. |
| `dokid` | Docker-compatible daemon over Unix socket and optional TCP/TLS. |
| `doki-compose` | Compose-style multi-service runner. |
| `doki-init` | Minimal PID 1 for guest/microVM paths; not an interactive CLI. |
| `doki-kube` | Experimental local Kubernetes control-plane components. |
| `doki-kubectl` | Lightweight kubectl-style client for Doki's experimental Kubernetes API. |

## Platform Matrix

| Platform | `doki` | `dokid` | `doki-compose` | `doki-init` | `doki-kube` | `doki-kubectl` |
|:---------|:------:|:-------:|:--------------:|:-----------:|:-----------:|:---------------:|
| Android ARM64 / Termux | Yes | Yes | Yes | Yes | Yes | Yes |
| Android ARMv7 / Termux | Yes | Yes | Yes | Yes | Yes | Yes |
| Linux ARM64 | Yes | Yes | Yes | Yes | Yes | Yes |
| Linux ARMv7 | Yes | Yes | Yes | Yes | Yes | Yes |
| Linux AMD64 | Yes | Yes | Yes | Yes | Yes | Yes |
| macOS ARM64 | Yes | No | No | No | Yes | Yes |
| macOS AMD64 | Yes | No | No | No | Yes | Yes |

`dokid`, `doki-compose`, and `doki-init` depend on Linux/Android process, filesystem, and networking primitives. macOS support is currently client-side unless paired with a Linux/Android daemon.

## Architecture

Doki has five main layers:

1. CLI clients: `doki`, `doki-compose`, and `doki-kubectl` parse user intent and call local APIs or helper packages.
2. API daemon: `dokid` exposes Docker-style endpoints, middleware, rate limiting, events, metrics, TLS, and optional TCP listeners.
3. Image and registry layer: OCI image metadata, layers, manifests, registry auth, pull/push, and local image store.
4. Runtime layer: runner registry picks the best available mode for the host: proot, native, namespaces, chroot, qemu-user, microVM, wasm, and other specialized runners.
5. Platform services: storage, networking, DNS, volumes, pod metadata, CRI/Kubernetes experiments, and DokiLink mesh primitives.

## Compatibility Targets

Doki tracks these upstream interfaces:

| Interface | Upstream reference | Doki target |
|:----------|:-------------------|:------------|
| Docker Engine API | https://docs.docker.com/reference/api/engine/ | Common Engine API operations first; conformance tests before claiming full parity. |
| Compose Spec | https://compose-spec.io/ | Service lifecycle, env files, networks, volumes, health checks, secrets, watch/publish over time. |
| OCI Image / Distribution | https://github.com/opencontainers/image-spec and https://github.com/opencontainers/distribution-spec | Correct manifests, descriptors, digests, layer handling, auth, push/pull, multi-arch resolution. |
| Kubernetes CRI | https://kubernetes.io/docs/concepts/containers/cri/ | Experimental runtime service and image service integration for kubelet-style workflows. |

## Configuration

Default paths are platform-aware:

| Host | Data dir | Socket |
|:-----|:---------|:-------|
| Termux | `$PREFIX/var/lib/doki` | `$PREFIX/var/run/doki.sock` |
| Linux | `/var/lib/doki` for daemon config, or user-level socket via `~/.doki/doki.sock` | `~/.doki/doki.sock` by default for CLI workflows |
| macOS | `~/Library/Application Support/doki` | `~/Library/Application Support/doki/doki.sock` |

Important environment variables:

| Variable | Use |
|:---------|:----|
| `DOKI_HOST` | CLI daemon socket, e.g. `unix://$HOME/.doki/doki.sock`. |
| `DOCKER_HOST` | Docker-compatible socket used when `DOKI_HOST` is unset. |
| `DOKI_SOCKET` | Daemon socket path. |
| `DOKI_DATA_DIR` | Daemon data root; overrides config files. |
| `DOKI_STORAGE_DRIVER` | Storage driver override. |
| `DOKI_LINK_MESH=0` | Disable DokiLink mesh startup. |
| `DOKI_TLS=1` | Enable daemon TLS options. |

Config files are written privately (`0600`) and atomically.

## Diagnostics

```bash
doki doctor
doki deps ls
doki deps check
doki deps go
```

`doki doctor` checks required host tools for the current platform and reports optional accelerators separately. On Termux, `proot` is required for the primary runtime path.

## Development

```bash
go test ./...
go vet ./...
make build-release sha256
```

For linting, use a `golangci-lint` version built with a Go toolchain new enough to read this module's export data. Older binaries built with Go 1.22 can fail with `unsupported version` errors against Go 1.26 packages.

## Roadmap

The detailed engineering roadmap is tracked in [docs/COMPATIBILITY_ROADMAP.md](docs/COMPATIBILITY_ROADMAP.md). It covers Podman/libpod, Kubernetes/CRI, DokiLink, DokiVM, proot/Termux, conformance tests, and repository-wide quality work.

Near-term hardening:

1. Docker API conformance harness for common CLI/SDK calls: `ps`, `run`, `logs`, `exec`, `pull`, `push`, `volumes`, `networks`, `events`, `stats`.
2. Compose Spec coverage table and golden tests for service config, env interpolation, networks, volumes, secrets, health checks, and `depends_on` behavior.
3. OCI registry conformance cases for digest verification, redirects, auth scopes, blob mounts, resumable pushes, and multi-arch manifest lists.
4. Runtime matrix tests on Android ARM64, Linux AMD64, Linux ARM64, and macOS client mode.
5. Storage corruption tests: interrupted layer extraction, partial metadata writes, volume prune failures, and state recovery.
6. Security profile work: seccomp/landlock/apparmor capability maps per runner, with explicit unsupported-feature errors.

Epic ideas worth adding after the base is solid:

1. `doki inspect --compat docker`: show exactly where a response differs from Docker Engine.
2. `doki conformance`: run a local compatibility suite and emit a scorecard.
3. `doki snapshot export/import`: portable backup of images, volumes, and container metadata.
4. `doki tunnel`: encrypted remote daemon access with short-lived device pairing.
5. `doki compose dev`: file watch, rebuild, restart, logs, and port status in one terminal UI.
6. `doki sbom` and `doki vuln`: SBOM generation plus pluggable scanner integration.
7. `doki profile`: per-device runtime profiles for phone, tablet, CI runner, server, and macOS client.
8. `doki bench`: startup, pull, extract, DNS, storage, and runtime benchmarks with reproducible output.
9. `doki policy`: deny privileged flags, host mounts, insecure registries, or unpinned images by default.
10. `doki app`: package a Compose stack as a signed mobile-friendly bundle.

## Project Stance

Doki should be ambitious, but claims should stay tied to tests. The strongest path is to make Android/Termux rootless containers excellent first, then expand compatibility one upstream interface at a time.

## License

Apache-2.0. See [LICENSE](LICENSE).
