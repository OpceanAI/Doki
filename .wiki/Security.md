# Security

<sub>[DEFENSE-IN-DEPTH / v0.11.0]</sub>

> Doki stacks independent controls: seccomp, AppArmor, Linux
> capabilities, user namespaces, cgroups v2, TLS 1.3, NaCl secretbox,
> image verification, rate limiting. No single control is trusted to
> hold. v0.11.0 ships full DokiLink Mesh cryptography: Ed25519
> identities, order-independent key derivation, replay-protected gossip.

<hr>

## Threat Model

<sub>[SCOPE / ATTACK SURFACES / MITIGATIONS]</sub>

```text
                    ATTACK SURFACE          MITIGATION
                    --------------          ----------
  +-----------+     kernel syscall         seccomp deny list
  | container | -->  table                  AppArmor MAC
  +-----------+     peer containers         mount namespace
        |           host filesystem         read-only + AppArmor
        |           host network            bridge isolation (no promisc)
        v           host resources          cgroups v2 limits
  +-----------+     image supply chain      CAS + path-traversal block
  |   host    |     API socket              TLS 1.3 + token + rate limit
  +-----------+     malicious image         symlink + hardlink validation
        |           mesh gossip             Ed25519 sig + replay window
        v           mesh transport          TLS 1.3 + NaCl secretbox
  +-----------+
  |  kernel   |     side-channel            OUT OF SCOPE
  +-----------+     physical access         OUT OF SCOPE
                    hypervisor escape       OUT OF SCOPE (MicroVM only)
                    kernel 0-day            seccomp mitigates, not fixes
```

### In scope

```text
THREAT                                  MITIGATION
------------------------------------    ----------------------------------
container escape via kernel exploit     seccomp blocks dangerous syscalls
container reading peer containers       mount namespace + seccomp
container reading host files            AppArmor + read-only mounts
network sniffing                        bridge isolation (no promisc mode)
container DoS via resource exhaustion   cgroups v2 limits
image supply chain attack               path traversal block + CAS
unauthorized API access                 TLS + token auth + rate limit
malicious image with backdoor           path traversal + symlink validation
mesh gossip replay / forgery            Ed25519 sig + nonce + timestamp
mesh transport eavesdrop / tamper       TLS 1.3 + NaCl secretbox L2
```

### Out of scope

```text
ITEM                                  REASON
-----------------------------------   ----------------------------------
side-channel (Spectre, Meltdown)      kernel-level, not container-level
physical access to host               physical security domain
hypervisor escape                     only relevant for MicroVM mode
kernel 0-day                          seccomp is mitigation, not fix
```

<hr>

## Defense Layers

<sub>[CONTAINER -> HOST STACK]</sub>

```text
+----------------------------------------------------------+
| container                                                |
|  +----------+   +-----------+   +-------------+          |
|  | app      |   | syscalls  |   | filesystem  |          |
|  | user code|-->| seccomp   |-->| AppArmor    |          |
|  +----------+   +-----------+   +-------------+          |
|  +-------------+   +-----------+   +----------------+    |
|  | resources   |   | network   |   | user           |    |
|  | cgroups v2  |-->| bridge    |-->| user namespace |    |
|  +-------------+   +-----------+   +----------------+    |
+----------------------------------------------------------+
                         |
                         v
                  +-------------+
                  | host kernel |
                  +-------------+
```

<hr>

## Seccomp

<sub>[SYSCALL FILTER / ~80 ALLOWED]</sub>

Doki ships a default seccomp profile. It allows roughly 80 syscalls
and blocks the dangerous ones.

### Default allow list

Standard syscalls: `read`, `write`, `open`, `openat`, `close`, `stat`,
`fstat`, `mmap`, `mprotect`, `brk`, `rt_sigaction`, `rt_sigprocmask`,
`rt_sigreturn`, `ioctl`, `pread64`, `pwrite64`, `readv`, `writev`,
`access`, `pipe`, `select`, `pselect6`, `poll`, `ppoll`, `dup`, `dup2`,
`dup3`, `socket`, `connect`, `accept`, `sendto`, `recvfrom`, `sendmsg`,
`recvmsg`, `bind`, `listen`, `getsockname`, `getpeername`,
`setsockopt`, `getsockopt`, `clone`, `fork`, `vfork`, `execve`, `exit`,
`exit_group`, `wait4`, `waitid`, `kill`, `tkill`, `tgkill`, `getpid`,
`gettid`, `getuid`, `getgid`, `geteuid`, `getegid`, `setuid`, `setgid`,
`setreuid`, `setregid`, `setsid`, `getrlimit`, `prlimit64`,
`getrusage`, `gettimeofday`, `clock_gettime`, `nanosleep`,
`sched_yield`, `sched_getaffinity`, `munmap`, `mremap`, `msync`,
`madvise`, `mincore`, `futex`, `getrandom`, `getcwd`, `chdir`, `mkdir`,
`mkdirat`, `rmdir`, `unlink`, `unlinkat`, `rename`, `renameat`, `link`,
`linkat`, `symlink`, `symlinkat`, `readlink`, `readlinkat`, `chmod`,
`fchmod`, `fchmodat`, `chown`, `fchown`, `fchownat`, `fstatfs`,
`statfs`, `umask`, `getpriority`, `setpriority`, `reboot`
(`kexec_load` blocked; `reboot` allowed for shutdown), `mount`,
`umount2`, `unshare`, `setns`, `capget`, `capset`, `prctl`, `seccomp`,
`personality`, `arch_prctl`, `time`, `set_tid_address`,
`restart_syscall`, `exit`, `exit_group`.

Modern syscalls: `io_uring_setup`, `io_uring_enter`, `io_uring_register`,
`pidfd_open`, `pidfd_send_signal`, `pidfd_getfd`, `rseq`,
`userfaultfd`, `copy_file_range`, `landlock_create_ruleset`,
`landlock_add_rule`, `landlock_restrict_self`, `memfd_create`,
`close_range`, `faccessat2`, `process_mrelease`, `mseal`.

### Default deny list

```text
SYSCALL                                  REASON
-------------------------------------    --------------------------------
init_module, finit_module, delete_module kernel module loading
kexec_load, kexec_file_load              kernel execution replacement
iopl, ioperm                             hardware I/O ports
kcmp                                     kernel info leaks (cross-PID)
process_vm_readv, process_vm_writev      cross-process memory access
bpf                                      BPF program loading
perf_event_open                          performance monitoring
lookup_dcookie                           dentry cache info leaks
quotactl                                 filesystem quota manipulation
mount (MS_REMOUNT|MS_BIND)               privilege escalation vector
swapon, swapoff                          swap manipulation
pivot_root                               chroot escape
reboot (LINUX_REBOOT_CMD_KEXEC)          kexec-reboot
```

### Custom profile

```json
{
  "seccomp": {
    "profile": "/etc/doki/seccomp/custom.json"
  }
}
```

Profile format is the
[OCI runtime spec seccomp schema](https://github.com/opencontainers/runtime-spec/blob/main/config-linux.md#seccomp):

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64", "SCMP_ARCH_AARCH64"],
  "syscalls": [
    {
      "names": ["read", "write", "open", "close"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

### Disable seccomp

```bash
doki run --security-opt seccomp=unconfined alpine echo hello
```

<hr>

## AppArmor

<sub>[MANDATORY ACCESS CONTROL / PER-CONTAINER]</sub>

AppArmor provides MAC on top of DAC. Doki generates a profile per
container.

### Default profile

```c
#include <tunables/global>

profile doki-default flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  #include <abstractions/nameservice>

  // Deny kernel module loading.
  deny capability sys_module,
  // Deny raw I/O.
  deny capability sys_rawio,

  // Allow network.
  network inet stream,
  network inet6 stream,

  // Deny mount.
  deny mount,
  deny umount,

  // Allow /docker/...
  /docker/** rwk,
  // Deny everything else.
  deny /** w,
  deny /** a,
}
```

### Custom profile

```bash
doki run --security-opt apparmor=my-profile alpine echo hello
```

The profile `my-profile` must be loaded in the kernel
(`apparmor_parser -a my-profile`).

<hr>

## Capabilities

<sub>[LINUX CAPABILITIES / MINIMAL DEFAULT]</sub>

Containers run with a minimal capability set by default:

```text
CHOWN, DAC_OVERRIDE, FSETID, FOWNER, MKNOD, NET_RAW, SETGID, SETUID,
SETFCAP, SETPCAP, NET_BIND_SERVICE, SYS_CHROOT, KILL, AUDIT_WRITE
```

Drop all and add only what is required:

```bash
doki run --cap-drop=ALL --cap-add=NET_BIND_SERVICE my-server:latest
```

```text
USE CASE                          CAPABILITY
------------------------------    ----------------------
web server binding to port 80     NET_BIND_SERVICE
time server                       SYS_TIME
NFS client                        SYS_ADMIN (carefully)
ping                              NET_RAW
tracing / debug                   SYS_PTRACE (very dangerous)
```

<hr>

## User Namespaces

<sub>[UID MAPPING / ROOTLESS]</sub>

The container root user (UID 0) is mapped to a high UID on the host by
default:

```json
{
  "uid_mappings": [{"container_id": 0, "host_id": 100000, "size": 65536}],
  "gid_mappings": [{"container_id": 0, "host_id": 100000, "size": 65536}]
}
```

If the container escapes, it appears as UID 100000 on the host. No root
access.

Disable with `--userns=host` (not recommended):

```bash
doki run --userns=host --rm alpine whoami
root  # <- dangerous: actual root on the host
```

<hr>

## cgroups v2

<sub>[RESOURCE LIMITS / LINUX ONLY]</sub>

```bash
# Memory limit.
doki run -m 512m my-image

# CPU limit.
doki run --cpus 1.5 my-image

# PIDs limit.
doki run --pids-limit 100 my-image

# Block I/O weight.
doki run --blkio-weight 500 my-image
```

cgroups v2 unified hierarchy is required. On older distros:

```bash
# Enable cgroup v2.
grubby --update-kernel=/boot/vmlinuz-$(uname -r) --args="systemd.unified_cgroup_hierarchy=1"
```

<hr>

## TLS / mTLS

<sub>[DAEMON SOCKET / TLS 1.3 MINIMUM]</sub>

The daemon supports TLS for client connections:

```json
{
  "tls": {
    "cert": "/etc/doki/cert.pem",
    "key": "/etc/doki/key.pem",
    "client_ca": "/etc/doki/ca.pem",
    "verify": true
  }
}
```

With `verify: true` (mTLS), the daemon requires clients to present a
certificate signed by `client_ca`. The Docker CLI and SDKs handle this
via `DOCKER_CERT_PATH` or `DOKI_TLS_*` env vars. `NewTLSWrapper`
enforces `RequireAndVerifyClientCert` when a client CA pool is
configured and clones the caller's config to avoid side effects.

Generate self-signed certs for testing:

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
```

For production, use a real CA (Let's Encrypt, internal PKI).

<hr>

## Rate Limiting

<sub>[PER-IP TOKEN BUCKET]</sub>

```json
{
  "rate_limit": {
    "rps": 100,
    "burst": 200
  }
}
```

100 requests per second sustained, bursts up to 200. Exceeding this
returns HTTP 429.

<hr>

## Landlock Sandbox

<sub>[UNPRIVILEGED LSM / ABI v1-v9 / v0.10+]</sub>

Unprivileged sandbox via the Linux Landlock LSM (Landlock ABI v9,
Linux 5.13+). Unlike seccomp (syscall filtering) and AppArmor
(path-based MAC), Landlock provides filesystem access control that any
user can configure without root.

```go
// pkg/landlock/landlock.go
cfg := &SandboxConfig{
    FSRules: []FSRule{
        {Path: rootfs, Access: LandlockAccessFSExecute},
    },
    NetRules: []NetRule{
        {Port: 443, Access: LandlockAccessNetBindTCP},
    },
}
```

```text
ACCESS TYPE    CONTROLS
-----------    -------------------------------------------------------
filesystem     execute, write_file, read_file, read_dir, remove_dir,
               remove_file, make_char, make_dir, make_reg, make_sock,
               make_fifo, make_block, make_sym, refer, truncate,
               ioctl_dev, resolve_unix
network        bind_tcp, connect_tcp
scope          abstract_unix_socket, signal
```

Doki probes the highest supported Landlock ABI on the host (v1 through
v9). If Landlock is unavailable (kernel < 5.13 or not compiled with
`CONFIG_SECURITY_LANDLOCK`), the sandbox is skipped gracefully.

<hr>

## Image Verification

<sub>[EXTRACTION GUARDS / PKG/STORAGE/LAYER.GO]</sub>

### Path traversal protection

```go
// pkg/storage/layer.go
if strings.Contains(path, "..") {
    return fmt.Errorf("path traversal: %s", path)
}
if filepath.IsAbs(path) {
    return fmt.Errorf("absolute path: %s", path)
}
```

### Symlink validation

```go
// If a symlink target points outside the rootfs, reject it.
realPath, err := filepath.EvalSymlinks(target)
if !strings.HasPrefix(realPath, rootfsDir) {
    return fmt.Errorf("symlink escape: %s -> %s", target, realPath)
}
```

### Hardlink restrictions

Hardlinks must point within the same layer (not across layers).
Prevents an attacker from hardlinking a sensitive file from a lower
layer into a writable upper dir.

### Content verification

Each layer's SHA256 is verified after download. If the registry
returns a layer with a different digest, the download is rejected.

<hr>

## DokiLink Cryptography

<sub>[ED25519 / NACL SECRETBOX / REPLAY PROTECTION / v0.11.0]</sub>

DokiLink authenticates all mesh traffic with Ed25519 signatures and
encrypts it with TLS 1.3 and optional NaCl secretbox. The cryptographic
identity is generated once per install and persisted at
`$DOKI_ROOT/keys/` with 0600 permissions.

### Install identity

```text
ARTIFACT              ALGORITHM         LIFETIME     PATH
------------------    --------------    ---------    --------------------
identity keypair      Ed25519           permanent    keys/id_ed25519
install ID            base32(pub[:8])   permanent    derived, 12 chars
CA certificate        ECDSA P-256       365 days     keys/ca.crt
CA private key        ECDSA P-256       permanent    keys/ca.key (0600)
link leaf cert        ECDSA P-256       90 days      keys/<id>.crt
link leaf key         ECDSA P-256       90 days      keys/<id>.key
peer pinned pubkey    Ed25519           TOFU         keys/peers/<id>.pub.pem
```

The Ed25519 private key (64 bytes: seed + pub) never leaves the host.
The public key (32 bytes) is broadcast as the peer fingerprint and used
to sign mesh messages (HELLO, ADVERTISE, REVOKE, BYE).

### Encryption layers

```text
LAYER     WHEN                LIBRARY                         NOTES
------    ----------------    ----------------------------    ------------------
L0 none   loopback only       --                              Android/Termux default
L1 TLS    any inter-host      crypto/tls (stdlib)             default, MinVersion = TLS 1.3
L2 box    payload-only opt-in golang.org/x/crypto/nacl/secretbox  DOKI_LINK_PAYLOAD_ENC=1
```

### TLS 1.3 configuration

```go
// pkg/netlink/crypto.go
cfg := &tls.Config{
    MinVersion:   tls.VersionTLS13,
    Certificates: []tls.Certificate{key},
}
// mTLS enforced when a client CA pool is configured.
if clone.ClientCAs != nil && clone.ClientAuth == tls.NoClientCert {
    clone.ClientAuth = tls.RequireAndVerifyClientCert
}
```

`NewTLSWrapper` clones the caller's config. The minimum version is
pinned to TLS 1.3. TLS 1.2 and below are not negotiated.

### NaCl secretbox key derivation

```go
// DeriveSecretKey is ORDER-INDEPENDENT: both peers derive the same
// 32-byte key regardless of which pubkey is passed first.
func DeriveSecretKey(localPub, remotePub []byte) [32]byte {
    a := copy(localPub)
    b := copy(remotePub)
    if string(b) < string(a) {
        a, b = b, a              // lexicographic sort -> stable input
    }
    h := sha256.New()
    h.Write([]byte("dokilink-v1|"))
    h.Write(a)
    h.Write([]byte("|"))
    h.Write(b)
    var out [32]byte
    copy(out[:], h.Sum(nil))
    return out
}
```

```text
INPUT                          OUTPUT
---------------------------    ----------------------------------
localPub (32) + remotePub (32) 32-byte NaCl secretbox key
derivation                     SHA-256("dokilink-v1|" + min + "|" + max)
order                          sorted lexicographically (stable)
self-connection                still derives a stable key
```

### Framing

```text
TRANSPORT    FRAME LAYOUT
---------    -----------------------------------------------------
TCP stream   4-byte BE length || nonce(24) || secretbox(plaintext)
UDP dgram    nonce(24) || secretbox(payload)
per-conn     nonce base seeded from crypto/rand (never zero)
overhead     24 bytes nonce + 16 bytes secretbox tag per frame
max frame    16 MiB (rejected above)
```

Each connection seeds its counter from `crypto/rand` so that two
independent connections sharing the same derived key never reuse a
nonce. Nonce reuse in secretbox destroys confidentiality and integrity;
the seeding is load-bearing. `secretboxStreamConn.Close` uses
`atomic.Bool` via `CompareAndSwap` to prevent a double-close race.

### Replay protection

```text
MECHANISM                  VALUE
----------------------     ----------------------------------
message fields            random 16-byte nonce + timestamp
replay window             5 minutes (replayWindow)
nonce cache               map[string]time.Time, cap 1024
eviction                  LRU-style on insert + cleanup ticker
reject conditions         timestamp zero / older than window / seen nonce
gossip size cap           MaxGossipMessageBytes = 4 KiB
listener guard            io.LimitReader(cap+1) -> OOM DoS prevention
```

```go
// pkg/netlink/mesh.go
const replayWindow    = 5 * time.Minute
const seenNonceLimit  = 1024
const MaxGossipMessageBytes = 4 * 1024

func (m *Mesh) checkReplay(msg Message) error {
    if msgTime.IsZero() || time.Since(msgTime) > replayWindow {
        return ErrStaleMessage
    }
    if _, seen := m.seenNonces[msg.Nonce]; seen {
        return ErrReplayDetected
    }
    if len(m.seenNonces) >= seenNonceLimit {
        // evict expired nonces
    }
    m.seenNonces[msg.Nonce] = time.Now()
    return nil
}
```

### Constant-time comparison

```text
CALL SITE                          FUNCTION
------------------------------     ----------------------------------
DeriveSecretKey pubkey sort        crypto/subtle.ConstantTimeCompare
TrustStore.Trust TOFU mismatch     crypto/subtle.ConstantTimeCompare
secretbox.Open tag verify          NaCl library (constant-time)
ed25519.Verify signature           Go ed25519 (constant-time)
```

TOFU (trust-on-first-use) pubkey mismatch is checked with
`crypto/subtle.ConstantTimeCompare`, not a non-constant-time
`bytesEqual`. This denies the attacker an oracle on the pinned key.

### DokiLink handshake

```text
PEER A                          PEER B
  |                               |
  |  STUN binding request         |
  |------------------------------>|
  |  XOR-MAPPED-ADDRESS           |
  |<------------------------------|
  |                               |
  |  TCP simultaneous open        |
  |  (hole punching)              |
  |<------------------------------>|
  |                               |
  |  Ed25519 signed gossip        |
  |  nonce + timestamp            |
  |  TLS 1.3 (L1)                 |
  |  optional secretbox (L2)      |
  |<------------------------------>|
  |                               |
  |  replay window: 5 min         |
  |  nonce cache: 1024 entries    |
```

<hr>

## Audit Logging

<sub>[LOG/SLOG / JSON / SIEM]</sub>

The daemon logs all API requests via `log/slog`:

```json
{
  "time": "2024-01-15T10:30:00Z",
  "level": "INFO",
  "msg": "request",
  "method": "POST",
  "path": "/containers/create",
  "remote": "127.0.0.1:54321",
  "duration_ms": 12,
  "status": 201
}
```

In JSON mode (production), this is greppable and parseable. Pipe to a
SIEM.

<hr>

## Container Logs

<sub>[STDOUT/STDERR / ROTATION]</sub>

Container stdout/stderr is written to
`data/containers/<id>/logs/*.log` by default. With
`--log-driver journald`, logs go to the systemd journal. With
`--log-driver local`, logs use a binary format with rotation.

```text
OPTION                DEFAULT     EFFECT
------------------    ---------   ----------------------------------
log_opts.max-size     10m         rotate when a file exceeds this
log_opts.max-file     3           keep this many rotated files
worst case            30 MB       10 MB x 3 files per container
```

<hr>

## Image Signing

<sub>[COSIGN / PLANNED]</sub>

Doki plans to support [cosign](https://github.com/sigstore/cosign)
signatures:

```bash
# Sign an image.
cosign sign --key cosign.key myapp:1.0

# Doki will verify on pull.
doki pull myapp:1.0
INFO  verifying signature for myapp:1.0
INFO  signature valid (key: <fingerprint>)
```

<hr>

## Security Advisories

<sub>[DISCLOSURE / 90 DAYS]</sub>

Security issues should be reported to security@dok1.xyz (PGP
key on the website). A 90-day disclosure timeline is followed.

<hr>

## Hardening Checklist

<sub>[PRODUCTION DEPLOYMENT]</sub>

```text
[ ] enable TLS on the daemon socket          (DOKI_TLS=1)
[ ] use mTLS if exposing the API to network  (tls.verify: true)
[ ] drop all capabilities by default         (--cap-drop=ALL)
[ ] use rootless mode where possible
[ ] run with --read-only for static containers
[ ] set memory and CPU limits
[ ] set --pids-limit to prevent fork bombs
[ ] use a custom seccomp profile for sensitive workloads
[ ] use a custom AppArmor profile
[ ] pin image digests, not tags              (myapp@sha256:abc...)
[ ] enable content trust (when available)
[ ] audit logs to a central SIEM
[ ] update Doki regularly (security patches in point releases)
[ ] enable DOKI_LINK_PAYLOAD_ENC=1 for cross-host mesh
```

<hr>

## Source

<sub>[SECURITY-CRITICAL PACKAGES]</sub>

```text
FILE                              ROLE
------------------------------    -----------------------------------
internal/seccomp/                 seccomp profile engine
internal/apparmor/                AppArmor profile generator
pkg/common/capabilities.go        capability sets
pkg/storage/layer.go              image verification
pkg/api/auth.go                   TLS configuration
pkg/api/ratelimit.go              rate limiting
cmd/dokid/main.go                 request logging
pkg/netlink/keys.go               install identity, Ed25519 + ECDSA CA
pkg/netlink/crypto.go             TLS 1.3 + NaCl secretbox wrappers
pkg/netlink/mesh.go               gossip, replay protection, nonce cache
pkg/landlock/landlock.go          unprivileged filesystem sandbox
```
