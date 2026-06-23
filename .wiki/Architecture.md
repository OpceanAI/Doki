# Architecture

<sub>[DAEMON INTERNALS / PIPELINE STAGES]</sub>

> Doki is structured in five layers, each with a clear boundary and
> deterministic failure mode. This page describes the pipeline from
> CLI input to kernel syscall.

---

## Layered Architecture

```
LAYER 1: CLI
  doki / doki-compose / doki-kubectl
  parse user intent -> HTTP/gRPC call to daemon
 ─────────────────────────────────────────────
LAYER 2: API DAEMON (dokid)
  Docker Engine v1.54  +  Podman libpod  +  TLS  +  rate limit
  HTTP/JSON over Unix socket / TCP
  middleware: logging, CORS, recovery, request-id, rate-limit
 ─────────────────────────────────────────────
LAYER 3: IMAGE + REGISTRY
  OCI manifest/digest/layer handling
  pull/push with auth, multi-arch resolution
  local image store (content-addressable)
  layer cache with dedup
 ─────────────────────────────────────────────
LAYER 4: RUNTIME
  runner registry: 12 backends
  selects best available mode for host
  proot -> native -> gVisor -> microVM -> wasm
  OCI spec generation + validation
 ─────────────────────────────────────────────
LAYER 5: PLATFORM SERVICES
  storage (fuse-overlayfs / vfs / overlay2)
  networking (bridge / DNS / DokiLink mesh / NAT / DHT)
  volumes / pods / CRI / Kubernetes control plane
```

---

## Pipeline Stages

<sub>[CONTAINER CREATE -> START]</sub>

```
1. CLI sends POST /containers/create
2. Daemon validates config (name, image, ports, volumes, env)
3. Image store resolves image reference -> digest
4. Layer extraction: tar stream -> rootfs directory
   - chown failures in rootless mode: logged once, not fatal
5. OCI runtime spec generated from container config
6. Runner registry selects backend (proot, native, gVisor, etc.)
7. Container state persisted to store (status=created)
8. CLI sends POST /containers/{id}/start
9. Runtime forks runner binary with OCI spec
10. Network setup: bridge (root) or pasta/slirp4netns/host (rootless)
11. DNS entries registered for container ID
12. Port forwarding via DokiLink TCP/UDP proxy
13. Container state updated (status=running, pid=N)
14. Log stream available via GET /containers/{id}/logs
```

---

## Runner Selection

<sub>[PRIORITY ORDER / AUTOMATIC FALLBACK]</sub>

```
PRIORITY  RUNNER      CONDITION
────────────────────────────────────────────────────────────
1         wasm        image media type is wasm
2         qemuuser    target arch != host arch
3         fex         x86 image on ARM host
4         namespaces  host has CAP_SYS_ADMIN + /proc/self/ns
5         chroot      host supports chroot(2)
6         microVM     /dev/kvm or AVF device present
7         gvisor      gVisor binary installed
8         pkdroid     Android 14+ with pKVM
9         sysbox      sysbox runtime installed
10        proot       proot binary installed (Termux default)
11        native      fallback, direct exec
12        legacy32    armv7 32-bit fallback
```

Explicit override: `DOKI_RUNTIME=gvisor` or `--runtime gvisor`.

---

## Package Layout

```
cmd/
  doki/           CLI binary
  dokid/          daemon binary
  doki-compose/   compose binary
  doki-kube/      Kubernetes control plane binary
  doki-kubectl/   kubectl client binary
pkg/
  api/            Docker Engine API server
  podman/         Podman libpod API shim
  runtime/        runner registry + 12 backends
  image/          OCI image store + registry client
  network/        bridge, DNS, DokiLink, NAT, DHT
  storage/        fuse-overlayfs, vfs, overlay2
  compose/        YAML parser, dependency resolver
  cri/            gRPC CRI server (41 RPCs)
  kubelet/        pod reconciliation via CRI
  kubeproxy/      iptables/nftables/userspace proxy
  scheduler/      pod-to-node assignment
  controllers/    deployment, replicaSet, job, endpoint, etc.
  apiserver/      Kubernetes REST API
  coredns/        cluster DNS server
  store/          in-memory + SQLite persistent store
  netlink/        mesh, proxy, crypto, NAT traversal, DHT
  macos/          VZ cgo bridge, QEMU, sandbox backends
  deps/           system dependency checker
  common/         shared types, config, validation
  cli/            CLI client library
internal/
  proot/          proot IPC client
  namespaces/     Linux namespace operations
  fuse/           FUSE overlayfs
  cgroups/        cgroup v2 manager
  seccomp/        seccomp profile generator
  apparmor/       AppArmor profile generator
  dokivm/         microVM backends (crosvm, firecracker, QEMU)
  gvisor/         gVisor runner integration
  qemu/           QEMU integration
  wasm/           WebAssembly runtime
```

---

## Failure Modes

```
COMPONENT       FAILURE              BEHAVIOR
──────────────────────────────────────────────────────────
image pull      network timeout      retry 3x, then error
layer extract   disk full            abort create, cleanup partial
runner fork     exec permission      fall back to next runner
network setup   pasta not found      fall back to host netns
DNS             port in use          log warning, continue without DNS
mesh listener   port in use          log warning, mesh disabled
store           disk full            reject writes, serve reads
CRI socket      permission denied    kubelet runs in fake mode
```

---

## See Also

- [Isolation Levels](Isolation-Levels) -- detailed runner mode descriptions
- [Networking](Networking) -- bridge, DNS, DokiLink mesh internals
- [Security](Security) -- seccomp, AppArmor, capabilities, TLS
- [Configuration](Configuration) -- config.json schema and env vars
