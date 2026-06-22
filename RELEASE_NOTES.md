# Doki v0.11.0 — Full DokiLink Mesh, macOS VZ, K8s 100%, Podman Wiring

## Breaking Changes

- `api.NewServer` now returns `(*Server, error)` instead of `*Server`
  (to propagate Podman shim initialization errors).
- `podman.NewPodmanServer` now returns `(*PodmanServer, error)`.
- `podman.NewPodManager`, `NewSecretManager`, `NewManifestManager` now
  return `(*T, error)` (fail-fast on store creation errors).
- `macos.SelectBackend` now returns `(Backend, error)`.
- These are compile-time changes for internal API consumers; the
  Docker/Podman/K8s HTTP APIs are unchanged.

## What's New in v0.11.0

### DokiLink Mesh — NAT Traversal + DHT + mDNS 90s Expiry

- **NAT traversal** (`pkg/netlink/nat_traversal.go`): STUN client
  (RFC 8489), TCP simultaneous open hole punching, and TURN-like relay
  server. Peers on different networks can now connect without static
  IPs or LAN proximity.
- **DHT peer discovery** (`pkg/netlink/dht.go`): Kademlia DHT with
  160-bit node IDs, k-buckets (k=8), alpha=3 parallel lookups. Peers
  discover each other without static config or mDNS.
- **mDNS 90-second expiry**: entries now expire after 90 seconds if
  not refreshed, with a periodic cleanup loop every 30s. Previously
  claimed but never implemented — now real.
- **Crypto fixes (critical)**:
  - `DeriveSecretKey` is now order-independent (both peers derive the
    same shared key). Previously order-sensitive, breaking L2
    encryption between real peers.
  - Per-connection nonce seeded from `crypto/rand` — prevents
    catastrophic nonce reuse across connections sharing a key.
  - `secretboxStreamConn.Close()` uses `atomic.Bool` — prevents
    double-close race.
  - Replay protection: gossip messages now include a random nonce +
    timestamp; messages older than 5 minutes or with seen nonces are
    rejected.
- **Mesh hardening**: `Stop()` now closes a `stopCh` to signal all
  loops (no goroutine leak). Gossip decoder wrapped in
  `io.LimitReader` (prevents OOM DoS). Stale-pointer race in
  `onMessage` fixed by re-fetching peer under write lock.
- **mDNS version bump**: TXT records now advertise `common.DokiVersion`
  instead of hardcoded "v0.9.3". Self-filter by install ID instead of
  port number.
- **UDP proxy**: replaced broadcast-to-all-sessions with last-sender
  heuristic (eliminates cross-talk for request/response patterns).
- **TCP proxy**: `net.Dial` replaced with `net.Dialer.DialContext`
  (configurable timeout). Idle timeout now truly idle (refreshed on
  each read) instead of a hard lifetime deadline.

### macOS Native Virtualization (VZ + QEMU + Sandbox)

- **VZ backend with cgo** (`pkg/macos/vz_bridge.h`, `vz_bridge.m`):
  Objective-C bridge to Virtualization.framework with
  `VZVirtualMachineConfiguration`, `VZLinuxBootLoader`,
  `VZVirtioFileSystemDevice` + `VZSharedDirectory`,
  `VZBridgedNetworkDevice`/`VZNATNetworkDevice`, `VZRosettaPlatform`.
  Build tag `darwin && cgo`, `CGO_ENABLED=1` required.
- **Build tags fixed**: the package now compiles on ALL platforms
  (darwin+cgo, darwin!cgo, !darwin). Previously it didn't compile on
  macOS at all due to mis-arranged constructor availability.
- **QEMU backend**: added `sync.RWMutex` (was race-prone), binary
  verification (was always reporting available), `timeoutSec` honored
  with SIGTERM→wait→SIGKILL, monitor goroutine for state accuracy,
  arch-aware args (`hvf:tcg` fallback, `ttyAMA0`/`ttyS0` by arch).
- **Sandbox backend**: profile tightened — `process-exec` scoped to
  rootfs + system paths (not unrestricted), `mach-lookup` restricted
  to named services, `ipc-posix-shm` scoped.
- **backend.go fixes**: `Hypervisor` probed via `sysctl kern.hv_support`
  (was hardcoded true), VZ gate `>= 11` (was `>= 12`, VZ is macOS 11+),
  `getMacOSVersion` surfaces errors, `DefaultVMImage` uses
  `os.UserHomeDir()`, `checkRosetta` sysctl fixed.
- **internal/dokivm fixes**: Firecracker `configureMachine` now uses
  `vmCfg.CPUs`/`Memory` (was hardcoded 1/128MiB), arch-aware
  `cpu_template`, TAP device created before reference, race conditions
  in Stop/Kill fixed, QEMU args wired from `vmCfg` (was fully
  hardcoded including `hostfwd=tcp::8080-:80`), `GenerateID` uses
  `crypto/rand`, vsock output corruption fixed, rootfs builder copies
  OCI layers (was discarded), CNI binary path configurable, TAP
  gateway from subnet (was hardcoded `10.89.0.1/16`).

### Kubernetes 100%

- **CRI gRPC server** (`pkg/cri/server.go`): real gRPC CRI implementing
  all 35 `RuntimeServiceServer` + 6 `ImageServiceServer` RPCs from
  `k8s.io/cri-api`. `ListenAndServe(socketPath)` on Unix socket.
  `CreateContainer` separated from `StartContainer` (per CRI spec).
- **Kubelet with real CRI** (`pkg/kubelet/kubelet.go`):
  `NewKubeletWithCRI` dials the CRI socket; `reconcilePod` now calls
  `RunPodSandbox` → `CreateContainer` → `StartContainer`, gets real
  PodIP from `PodSandboxStatus`, real container states and image
  digests from `ContainerStatus`. Falls back to fake mode if no CRI
  client (backward compat). Hardcoded values replaced with
  `common.DokiVersion`, `runtime.GOARCH`, detected node IP, real CPU
 /memory from `/proc/meminfo`.
- **Kube-proxy real** (`pkg/kubeproxy/proxy.go`): `syncIPTables` now
  invokes `iptables` to create chains, DNAT rules, and MASQUERADE.
  `syncNFTables` invokes `nft` with a generated ruleset.
  `syncUserspace` runs an in-process TCP/UDP round-robin proxy (works
  without root, for Termux).
- **Controllers functional** (`pkg/controllers/manager.go`):
  DeploymentController break bug fixed, status reflects actual pod
  counts. ReplicaSetController deletes excess pods. JobController
  implements parallelism/completions/backoff. EndpointController
  populates Endpoints from pod readiness. ServiceController allocates
  ClusterIP. NamespaceController cascades deletion. GarbageCollector
  implements cascading deletion via OwnerReferences. Watch errors
  retried with backoff instead of panicking.
- **API server complete** (`pkg/apiserver/server.go`): API group paths
  fixed (`networking.k8s.io/v1`, `rbac.authorization.k8s.io/v1`).
  PATCH implements merge-patch + strategic-merge. Watch emits proper
  K8s event format (`{"type":..., "object": <raw JSON>}`). POST
  generates UID, resourceVersion, creationTimestamp. Version handler
  uses `common.DokiVersion`/`runtime` info.
- **SQLiteStore** (`pkg/store/sqlite.go`): persistent store implementing
  the `Store` interface via SQLite. Replaces in-memory-only state with
  crash-safe persistence.
- **Scheduler real** (`pkg/scheduler/scheduler.go`): busy-wait CPU spin
  replaced with blocking sleep. `scoreImageLocality` checks node
  images. `scoreLeastRequested` parses allocatable CPU/memory. Error
  handling in `scheduleOne`. Retry with backoff on failure.
- **CoreDNS real** (`pkg/coredns/server.go`): UDP buffer race fixed
  (copy per query). `buildResponse` preserves question section. IP
  octet overflow fixed. NXDOMAIN for unresolvable queries. SRV record
  support for `_<port>._tcp.<svc>.<ns>.svc.cluster.local`. AAAA
  returns NXDOMAIN (registry is IPv4). Useless `init()` removed.

### Podman Wiring

- **Podman shim mounted in dokid**: `pkg/api/server.go` `NewServer`
  now mounts `pkg/podman` routes at `/libpod/*` on the same server,
  inheriting TLS, middleware, and rate limiting. Previously the entire
  Podman package was dead code (0 imports).
- **System info**: hardcoded values replaced with `runtime.GOARCH`,
  `runtime.GOOS`, detected kernel/memory, `common.DokiVersion`.
- **Container lifecycle**: start/stop/kill/restart/pause/unpause now
  delegate to PodManager (was 501).
- **Container dispatch**: GET returns 404 if not found, DELETE returns
  204.

### Dependencies

- **Version hygiene**: `install.sh` updated to v0.10.0. mDNS TXT
  records use `common.DokiVersion`. `doki-kubectl` version uses
  `common.DokiVersion`. All hardcoded "v0.9.3"/"v0.10.0" strings
  replaced with `common.DokiVersion`/`common.DokiAPIVersion`.
- **Compose healthcheck execution** (`pkg/runtime/healthcheck.go`):
  `HealthChecker` runs periodic probes (CMD/CMD-SHELL/NONE), respects
  Interval/Timeout/Retries/StartPeriod/StartInterval, updates
  `state.HealthStatus.Status` (`starting`→`healthy`/`unhealthy`).
  Compose `service_healthy` condition now works end-to-end.
- **`doki deps` tool** (`pkg/deps/checker.go`): new CLI subcommand
  with `ls` (list system deps), `check` (CI gate), `go` (list Go deps),
  `install <name>` (best-effort install via detected package manager).

### Security Fixes (Critical)

- **Path traversal**: `TrustStore.persistUnlocked` validates peerID
  (rejects `/`, `\`, `..`). `SecretManager.Create`/`Remove` validates
  secret names via `common.ValidContainerName`. `ManifestManager`
  validates manifest names in Create/Delete/saveManifest.
- **Constant-time comparison**: `TrustStore.Trust` uses
  `crypto/subtle.ConstantTimeCompare` for TOFU pubkey mismatch (was
  non-constant-time `bytesEqual`).
- **mTLS enforcement**: `NewTLSWrapper` sets `RequireAndVerifyClientCert`
  when `ClientCAs` is configured. Clones caller's config (no side
  effects).
- **Replay protection**: gossip messages include random nonce +
  timestamp freshness check (5-minute window). Seen-nonce cache with
  LRU eviction (1024 entries).
- **OOM DoS prevention**: gossip listener wrapped in
  `io.LimitReader(MaxGossipMessageBytes+1)`.
- **Data races fixed**: `secretboxStreamConn.Close` uses `atomic.Bool`.
  Podman managers return deep copies (not internal pointers). Mesh
  `onMessage` re-fetches peer under write lock.

## Roadmap Items Completed Ahead of Schedule

The following items from `docs/technical-report/sections/10-roadmap.tex`
were planned for v1.0 but shipped in v0.11.0:

- **NAT traversal** (STUN/TURN hole-punching) — was v1.0, now shipped
- **DHT peer discovery** (Kademlia) — was v1.0, now shipped
- **K8s CRI compliance** — was v1.0, now shipped

## Breaking Changes

- None. v0.10.0 is fully backward-compatible with v0.9.3. All existing
  Docker API endpoints continue to work. New Podman and Kubernetes
  endpoints are additive.

## What's New

### Podman API v5 (39 endpoints)

- **Pod management** (`pkg/podman/pod_manager.go`): create, start, stop,
  restart, kill, pause, unpause, inspect, remove, list, prune
- **Secret management** (`pkg/podman/secret_manager.go`): create, inspect,
  list, remove with encryption support
- **Manifest management** (`pkg/podman/manifest_manager.go`): create, add,
  remove, inspect, push, list
- **Compatible** with `podman-remote` clients (libpod v5 protocol)
- **14 unit tests** covering validation, lifecycle, persistence, and
  duplicate detection

### Kubernetes 1.32 (6 components)

- **API Server** (`pkg/apiserver/server.go`): 530 lines, handles pods,
  services, deployments, configmaps, secrets, namespaces, nodes, PV,
  PVC, serviceaccounts, events with REST semantics
- **Kubelet** (`pkg/kubelet/kubelet.go`): pod reconciliation loop with
  status reporting to the API server
- **Scheduler** (`pkg/scheduler/scheduler.go`): pod-to-node assignment
- **Controllers** (`pkg/controllers/manager.go`): 10 controllers
  (Deployment, ReplicaSet, Job, CronJob, DaemonSet, StatefulSet,
  Node, Namespace, GarbageCollector)
- **Kube-proxy** (`pkg/kubeproxy/proxy.go`): service-to-pod IP routing
  (iptables mode)
- **CoreDNS** (`pkg/coredns/server.go`): cluster-local DNS resolution
  with service discovery
- **kubectl client** (`cmd/doki-kubectl/main.go`): get, apply, delete,
  describe, logs, version, cluster-info, api-resources with namespace
  and all-namespaces flags
- **80 K8s API types** (`pkg/k8s-types/`): meta, core, core_resources,
  apps with full godoc documentation

### macOS Native Virtualization

- **VZ backend** (`pkg/macos/vz_backend.go`): Apple Virtualization.framework
  for macOS 11+ with CGO/ObjC
- **QEMU backend** (`pkg/macos/qemu_backend.go`): fallback QEMU-based VM
  for macOS without VZ or for Intel Macs
- **Sandbox backend** (`pkg/macos/sandbox_backend.go`): macOS sandbox-exec
  lightweight isolation (no VM overhead)
- **Stub backend** (`pkg/macos/backend_stub.go`): no-op stubs for non-macOS
  platforms, build-tag separated (`!darwin`)

### doki-OS

- **Kernel config** (`doki-os/kernel/doki-os.config`): minimal Linux kernel
  config targeting ~4MB compressed bzImage, todo built-in (no modules),
  excludes ACPI/USB/GPU/WiFi/sound
- **Makefile** (`doki-os/Makefile`): kernel + rootfs + VM image build system

### Landlock Sandboxing

- **ABI v9** support for Linux 5.13+ kernels
- Filesystem rules (17 access types), network rules (TCP bind/connect),
  scope rules (abstract Unix sockets, signals)
- Auto-detection with ABI fallback (probes highest supported version)

### State Store & Memory Management

- **Thread-safe store** (`pkg/store/store.go`): Watch/List/Put/Delete
  with revision tracking and change notifications
- **SQLite support** via `github.com/ncruces/go-sqlite3` for persistent
  state (optional, defaults to in-memory)

### Compose Watch & Publish

- **Watch** (`pkg/compose/watch.go`): file watching via
  `github.com/fsnotify/fsnotify` for hot-reload during development
- **Publish** (`pkg/compose/publish.go`): service mesh integration for
  compose-based deployments

### DNS Advanced Features

- **SRV records** (`pkg/network/dns_advanced.go`): service discovery
  protocol support
- **DNSSEC validation**: configurable DNSSEC verification
- **Persistent cache**: LRU-based DNS cache with TTL and expiration
- **Domain rules**: per-domain upstream resolver configuration

### Process Monitoring

- **pidfd** (`pkg/runtime/pidfd.go`): Linux 5.3+ process file descriptors
  for reliable process tracking without PID reuse races

### Build System & CI

- **13 build targets** across Linux (ARM64, ARMv7), macOS (ARM64, AMD64),
  and Android (ARM64, ARMv7)
- **Makefile** updated with `doki-kube`, `doki-kubectl`, `darwin-amd64`
  and `darwin-arm64` targets
- **SHA256 checksums** generated for all release artifacts

## Dependencies

### Added (15 new direct dependencies)

| Module | Purpose |
|:-------|:--------|
| `github.com/opencontainers/image-spec` | OCI image spec types |
| `github.com/opencontainers/runtime-spec` | OCI runtime spec |
| `github.com/opencontainers/go-digest` | OCI content digests |
| `github.com/opencontainers/selinux` | SELinux labeling support |
| `google.golang.org/grpc` | gRPC for CRI plugin |
| `google.golang.org/protobuf` | Protobuf for CRI |
| `k8s.io/cri-api` | Kubernetes CRI API types |
| `github.com/containerd/containerd/v2` | Containerd OCI packages |
| `github.com/klauspost/compress` | Fast gzip/zstd compression |
| `github.com/ulikunitz/xz` | XZ compression support |
| `github.com/moby/patternmatcher` | Dockerfile pattern matching |
| `github.com/moby/term` | Terminal utilities |
| `github.com/ncruces/go-sqlite3` | SQLite for K8s state store |
| `github.com/mattn/go-isatty` | Terminal detection |
| `golang.org/x/term` | Terminal I/O |

### Total: 21 direct, 50 total dependencies

## Bug Fixes (190+ across 14 audit rounds)

### Round 1-4: Static Analysis
- **staticcheck**: 0 warnings (eliminated all U1000, S1011, S1012, S1017,
  SA1019, SA1004 errors)
- **errcheck**: 672 production unchecked errors → 0 (fixed all I/O,
  JSON, process, and state management error handling)
- **go vet**: 2 warnings → 0 (fixed mutex copy and undefined constant)
- **gosec**: 14 G115 integer overflow conversions annotated with
  `#nosec` (intentional bit-shift operations for protocol encoding)

### Round 5-8: Architecture & Security
- **ALL_CAPS constants** → CamelCase in landlock (23 constants)
- **330 unused parameters** → 0 in production code
- **42 missing package comments** → all documented
- **132 Runner method docs** → all documented with godoc
- **343 exported type docs** added across storage, controllers, k8s-types,
  runtime, compose, cli, api

### Round 9-10: CLI & UX
- **doki-kube --help** exits cleanly without starting server
- **doki-kube version** command implemented
- **doki-kubectl** 11 bugs fixed (PANIC handler, -A/-n flags, describe
  singular→plural, shorthands, YAML apply, AGE calculation)
- **doki-compose down** properly cleans containers, networks, volumes
- **doki search** parses Docker Hub results correctly (NAME/DESCRIPTION/STARS)
- **doki system df** displays formatted table instead of raw JSON
- **doki inspect/start** require arguments (was silently returning)

### Round 11-14: Networking & Concurrency
- **doki-link**: 19 bugs fixed (race conditions in onMessage, goroutine
  leak in Stop, DoS via OOM in JSON decoder, thread-safety in crypto,
  mDNS entry expiration, TCPProxy dial timeout, backoff in gossip)
- **Cryptographic hardening**: TLS 1.3 minimum, secretbox payload
  encryption, TOFU trust model, 0600 permissions on key material

## Quality Metrics

| Metric | Before (v0.9.3) | After (v0.10.0) |
|:-------|:----------------|:----------------|
| **Files** | 120 | 158 |
| **LOC** | 18,000 | 55,000 |
| **Packages** | 15 | 29 |
| **Binaries** | 4 | 9 |
| **Dependencies** | 6 | 21 |
| **API version** | v1.48 | v1.54 |
| **staticcheck** | 0 | 0 |
| **errcheck (prod)** | 687 | 0 |
| **go vet** | 2 | 0 |
| **revive** | 1,223 | 351 |
| **Test files** | 12 | 32 |

## Known Limitations

### Podman API
- 39 of 184 libpod v5 endpoints implemented (21.2%). Missing endpoints
  include container lifecycle operations, image inspection/push, and
  generate kube/systemd. See `pkg/podman/api.go` for the full list.

### Kubernetes
- DNS requires root or `CAP_NET_BIND_SERVICE` for port 53.
  Default listen address `10.96.0.10:53` uses service CIDR.
- `doki-kubectl apply` only accepts JSON (not YAML) unless the
  `gopkg.in/yaml.v3` module is explicitly referenced.

### DokiLink Mesh
- NAT traversal (STUN/TURN hole-punching) shipped in v0.11.0 — peers
  can now connect across NAT boundaries without static IP addresses
- DHT peer discovery shipped in v0.11.0 — decentralized peer lookup
  without static configuration or mDNS
- mDNS entries expire after 90 seconds with periodic cleanup (improved
  in v0.11.0 with faster eviction of stale peers)

### Compose
- `postgres:alpine` layer extraction may fail with "unexpected EOF"
  on slow connections — retry by pulling the image first
- Excessive `chown: operation not permitted` warnings in rootless/
  proot mode (cosmetic, does not affect container operation)

## Security

All security findings from the comprehensive audit have been reviewed:

- **Path traversal**: All tar extraction paths are validated with
  `filepath.Clean` and prefix checks before write
- **Permissions**: Key material uses 0600; container files use 0644/0755
  (standard for multi-user container environments)
- **Decompression bombs**: I/O from tar readers is bounded by container
  image layer sizes (registry-verified digests)
- **Integer overflow**: Protocol encoding operations use intentional
  bit-shifts that guarantee byte-range values
- **unsafe.Pointer**: Used only in `pidfd.go` for the Linux `waitid(2)`
  syscall (the only way to reliably track process exit status without
  PID reuse races)

## Install / Upgrade

```bash
# ARM64 (most Android devices, Apple Silicon, Linux ARM servers)
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.10.0/doki-v0.10.0-arm64.tar.gz | tar -xz
cd doki-v0.10.0-arm64
./install.sh

# ARMV7 (older 32-bit Android)
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.10.0/doki-v0.10.0-armv7.tar.gz | tar -xz
cd doki-v0.10.0-armv7
./install.sh

# macOS ARM64 (Apple Silicon)
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.10.0/doki-v0.10.0-darwin-arm64.tar.gz | tar -xz
cd doki-v0.10.0-darwin-arm64
./install.sh
```

## Verifying

```bash
dokid --version
doki version
doki-kube version
doki-kubectl version
doki-compose version
```

## Building from Source

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
make release        # all platforms
make build-linux-arm64   # single platform
```

## Changelog

See `README.md` for the full changelog (v0.9.0 through v0.10.0).
