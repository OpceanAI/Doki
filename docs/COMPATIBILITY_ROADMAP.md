# Doki Compatibility Roadmap

This document turns Doki's big compatibility goals into testable engineering work. It is based on the code currently in this repository and on the upstream interfaces Doki wants to emulate or integrate with.

Primary references:

- Docker Engine API: https://docs.docker.com/reference/api/engine/
- Podman/libpod API reference: https://docs.podman.io/en/latest/_static/api.html
- Kubernetes CRI: https://kubernetes.io/docs/concepts/containers/cri/
- OCI Image Spec: https://github.com/opencontainers/image-spec
- OCI Distribution Spec: https://github.com/opencontainers/distribution-spec
- Compose Specification: https://compose-spec.io/
- QEMU microvm machine: https://www.qemu.org/docs/master/system/i386/microvm.html
- Termux proot-distro behavior: https://github.com/termux/proot-distro

## Guiding Rule

Do not claim full compatibility until there is an automated conformance test for that surface. Doki should expose partial compatibility honestly and publish a scorecard.

## Podman Compatibility

Current code:

- `pkg/podman/api.go` registers `/libpod/...` endpoints.
- `pkg/podman/pod_manager.go`, `secret_manager.go`, and `manifest_manager.go` implement local managers.
- Podman routes currently sit beside the Docker-like API in `pkg/api/server.go`.

High-impact gaps:

1. Build an endpoint matrix generated from `PodmanServer.RegisterRoutes` and checked in as a test fixture.
2. Add compatibility tests for `podman --remote --url unix://... info`, `ps`, `images`, `pod create/list/rm`, `volume create/list/rm`, `network ls`, `secret create/list/rm`, and manifest operations.
3. Normalize response fields to Podman's libpod schema, especially casing and nested error payloads.
4. Add method checks per route. Several handlers route by path only; compatibility should reject unsupported methods with 405.
5. Add version negotiation endpoints and explicit API version reporting for libpod clients.
6. Add socket tests with the real `podman-remote` client when available.

Epic additions:

1. `doki podman scorecard`: run a local Podman client compatibility suite.
2. `doki generate systemd`: Podman-style unit generation for containers and pods.
3. Quadlet compatibility for common `.container`, `.pod`, `.volume`, and `.network` files.

## Kubernetes / CRI Compatibility

Current code:

- `pkg/cri/server.go` implements CRI v1 gRPC services.
- `pkg/cri/plugin.go` adapts runtime, image, and network managers.
- `pkg/kubelet`, `pkg/apiserver`, `pkg/controllers`, `pkg/scheduler`, and `pkg/kubeproxy` are experimental local components.

High-impact gaps:

1. Add a CRI conformance harness using `crictl` or direct gRPC calls for `Version`, `Status`, `RunPodSandbox`, `CreateContainer`, `StartContainer`, `StopContainer`, `RemoveContainer`, `List*`, `ImageStatus`, `PullImage`, and `RemoveImage`.
2. Implement complete `RuntimeConfig` and `Status` responses with conditions that kubelet can interpret correctly.
3. Implement CRI events or a documented polling strategy. Kubernetes has moved toward event-based runtime updates where supported.
4. Add image pull progress/resource controls to avoid unbounded CPU, I/O, or network usage during pulls.
5. Map Kubernetes security context to Doki runner capabilities: privileged, readonly rootfs, capabilities, SELinux/AppArmor/seccomp, runAsUser, runAsGroup, fsGroup.
6. Add pod log path semantics and `ContainerStatus` details compatible with kubelet expectations.
7. Add conformance tests for namespace, labels, annotations, restart policy, liveness/readiness/startup probe behavior.

Epic additions:

1. `doki kube conformance`: local CRI/Kubernetes API scorecard.
2. `doki kube play --explain`: show how each Kubernetes field maps to Doki runtime features.
3. Tiny single-node kube profile for Termux experiments with strict feature gates.

## DokiLink

Current code:

- `pkg/netlink` contains identity, trust, signed gossip, static discovery, optional mDNS, UDP helpers, NAT traversal, proxying, and mesh tests.
- Gossip messages are signed and replay-protected with nonce cache and timestamp window.

High-impact gaps:

1. Define a protocol version in every gossip message and reject incompatible versions explicitly.
2. Add peer capability exchange: API version, supported runtimes, architecture, reachable addresses, relay support, and trust level.
3. Add persistent peer health: last seen, failure count, RTT estimate, and backoff state.
4. Add signed trust bootstrap commands: `doki link invite`, `doki link accept`, `doki link revoke`, `doki link rotate-key`.
5. Add transport choices: direct TCP, UDP hole-punch attempt, relay fallback, and explicit local-only mode.
6. Add abuse controls: per-peer rate limits, message type quotas, and bounded container announcement cache.
7. Encrypt gossip payloads after trust establishment; signatures alone authenticate but do not hide metadata.

Epic additions:

1. `doki link status --graph`: visualize peers, routes, trust, latency, and relay paths.
2. `doki link serve`: securely expose selected containers or Compose services to trusted peers.
3. `doki link sync`: move images or app bundles between phones without a registry.

## DokiVM / MicroVM

Current code:

- `internal/dokivm/vmm.go` defines the backend interface and detection.
- `internal/dokivm/qemu`, `firecracker`, and `crosvm` provide backend packages.
- QEMU uses the `microvm` machine type and falls back between KVM and TCG.

High-impact gaps:

1. Add backend capability structs: supportsKVM, supportsVsock, supportsVirtioFS, supportsTap, supportsUserNet, supportsSnapshots, supportsBalloon, supportsJailer.
2. Stop hardcoding network defaults like `hostfwd=tcp::8080-:80`; derive from container port mappings.
3. Add kernel/rootfs validation before VM start with actionable errors.
4. Add per-VM log files and serial capture; `Logs` should return real output.
5. Add health and boot timeout detection instead of assuming successful start after process spawn.
6. Add cleanup tests for failed starts, stale sockets, tap devices, and work directories.
7. Add backend-specific argument golden tests for QEMU, Firecracker, and crosvm.
8. Add a documented Android AVF/pKVM path separate from generic QEMU/crosvm detection.

Epic additions:

1. `doki vm doctor`: backend scorecard for KVM, crosvm, Firecracker, QEMU, vsock, tap, virtiofs.
2. VM snapshot/restore for fast cold starts.
3. Signed minimal guest image builder with reproducible kernel/rootfs inputs.

## proot / Termux Compatibility

Current code:

- `internal/proot` centralizes binary detection, base args, Android binds, env scrubbing, and kernel release spoofing.
- `pkg/runtime/runners/proot` tries IPC first and falls back to direct CLI mode.

High-impact gaps:

1. Keep all sockets under `os.TempDir()` or Doki runtime dir, never hardcoded `/tmp`.
2. Add a `proot doctor` check: binary path, version, ptrace availability, storage path executability, LD_PRELOAD cleanup, Android version, Termux prefix, and known-bad env vars.
3. Add golden tests for proot argv construction: binds, cwd, uid/gid, rootfs path, command separator, and Android-specific binds.
4. Add explicit unsupported-feature errors for systemd, real cgroups, kernel mounts, privileged mode, nested containers, and FUSE when unavailable.
5. Improve port publishing in proot mode: host-loopback mapping, collision detection, and clear `doki ps` port output.
6. Cache extracted rootfs metadata to reduce startup overhead on phones.
7. Add a battery/thermal-friendly mode: lower concurrency for pull/extract/build and clear progress output.

Epic additions:

1. `doki termux setup`: install packages, create dirs, verify execution permissions, and tune config.
2. `doki run --android-bind storage`: safe presets for `/sdcard`, app-private storage, and Termux home.
3. `doki profile phone`: runtime defaults optimized for mobile CPU, memory, storage, and network constraints.

## Repository-Wide Quality Work

1. Add `docs/compat/*.md` scorecards and keep them updated by tests.
2. Replace old `golangci-lint` config options with current syntax and pin a linter version built with the repo Go version.
3. Add integration tests that start `dokid` with a temporary `DOKI_DATA_DIR` and socket, then exercise `doki ps/images/info/version`.
4. Add generated API route inventories for Docker, Podman, and Kubernetes endpoints.
5. Add benchmark suite: pull, extract, create, start, stop, DNS lookup, image list, and volume operations.
6. Add fault injection tests: corrupt metadata, interrupted layer extraction, failed network setup, full disk, missing proot, stale sockets.
7. Add feature gates for experimental components so users get explicit warnings and stable defaults.

## Proposed Milestones

1. M1: Termux reliability: doctor, proot argv tests, temp data isolation, rootfs cache, clear unsupported-feature errors.
2. M2: Docker API scorecard: route inventory, SDK smoke tests, endpoint compatibility table.
3. M3: Podman scorecard: libpod endpoint matrix, pod/secret/manifest conformance, podman-remote smoke tests.
4. M4: OCI hardening: digest verification, auth scopes, multi-arch fixtures, interrupted pull recovery.
5. M5: DokiLink secure mesh: invites, key rotation, encrypted payloads, relay fallback, peer health.
6. M6: DokiVM backend hardening: capability detection, logs, boot health, argument golden tests, snapshots.
7. M7: Kubernetes/CRI scorecard: crictl suite, kubelet smoke test, pod lifecycle and probe conformance.
