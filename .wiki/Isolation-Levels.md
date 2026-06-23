# Isolation Levels

<sub>[DOC: ISOLATION-LEVELS]</sub>

Doki v0.11.0. Twelve runner modes. Range from a WASM sandbox with no syscalls to hardware-level microVMs. The runner registry in <kbd>pkg/runtime/registry.go</kbd> probes the host and selects the strongest mode that works. A specific mode can be forced with <kbd>doki run --runtime &lt;mode&gt;</kbd>.

<hr>

## Mode Matrix

<sub>[MATRIX]</sub>

Columns: mode, root required, isolation level, platforms, kernel requirements, overhead.

```text
MODE        ROOT  ISOLATION             PLATFORMS              KERNEL REQ     OVERHEAD
────────────────────────────────────────────────────────────────────────────────────────
wasm        no    sandbox user-space    any with wasm runtime  n/a            minimal
pkvm        yes   hardware vm           android 15+ (tensor/sd) pKVM cap     5-20 MB RAM
microvm    yes   hardware vm           kvm/gunyah/geniezone   4.18+          5-20 MB RAM
sysbox      no    kernel DinD           linux 4.18+            user ns        moderate
namespaces  yes   kernel                linux 3.8+             any            negligible
gvisor      no    user-space kernel     linux 4.14+            any            ~20% CPU
fex         no    emulation x86 to ARM64 arm64 host            n/a            ~30% CPU
qemu-user   no    emulation cross-arch  any with qemu-static   binfmt_misc    ~50% CPU
proot       no    userspace ptrace      termux/android/linux   n/a            ~10% CPU
legacy32   no    dual-arch compat      arm64 host              binfmt_misc    negligible
chroot      yes   filesystem            any unix               n/a            minimal
native      no    none                  any                    n/a            zero
```

<hr>

## Automatic Selection Priority

<sub>[PRIORITY]</sub>

Detection runs in strict order. The first mode whose requirements are met wins. Order is strongest isolation to weakest.

```text
PRIORITY  MODE        PROBE
─────────────────────────────────────────────────────────────
1         wasm        which wasmedge || which iwasm
2         pkvm        /dev/kvm readable AND android 15+ kernel
3         microvm    /dev/kvm readable OR gunyah/geniezone node
4         sysbox      which sysbox-runc
5         gvisor      which runsc
6         namespaces  uid == 0 AND /proc/self/ns accessible
7         fex         which FEXInterpreter OR which box64
8         qemu-user   which qemu-<arch>-static
9         proot       which proot OR doki-proot fallback
10        legacy32   uname -m == aarch64 AND binfmt_misc registered
11        chroot      uid == 0
12        native      always available, last resort
```

Override with <kbd>--runtime</kbd>. The override skips the probe and forces the mode. Failure to initialize aborts the run.

```bash
doki run --runtime proot alpine echo hello
doki run --runtime microvm alpine echo hello
doki info --format json | jq '.Isolations'
```

<hr>

## WASM

<sub>[MODE 12]</sub>

Runs WASI modules via <kbd>wasmedge</kbd> or <kbd>iwasm</kbd>. The module never makes a real syscall. All I/O is mediated by the WASM host. Cold start is roughly 1-5ms. Requires an OCI image with <kbd>application/wasm</kbd> media type or a <kbd>wasm-oci</kbd> config. Syscall surface is limited. No real <kbd>fork</kbd> or <kbd>execve</kbd>. Some libraries break. Networking needs WASI socket extensions. Detection via <kbd>which wasmedge</kbd>. Runtime not validated on production workloads. Code: <kbd>pkg/runtime/runners/wasm/runner.go</kbd> (planned).

<hr>

## pKVM / Microdroid

<sub>[MODE 11]</sub>

Protected Kernel-based Virtual Machine. Google hypervisor on Android 15+. Host kernel runs in EL1. Guest VMs run in a separate protected world. Memory is encrypted and isolated at hardware level. Requires a pKVM-capable kernel (Tensor G3/G4, Snapdragon 8 Gen 3/4), readable <kbd>/dev/kvm</kbd>, and <kbd>microdroid</kbd> (Doki bundles it). Boot time ~50ms. RAM overhead 5-20 MB per guest. No GPU passthrough (planned v1.0). Detection works. No compatible hardware in CI. Code: <kbd>pkg/runtime/runners/pkvm/runner.go</kbd> (planned).

<hr>

## MicroVM

<sub>[MODE 10]</sub>

Lightweight VMs via crosvm (Chromium OS VMM) or Firecracker (AWS). Boots in microseconds. Minimal device model. Requires <kbd>/dev/kvm</kbd> readable or vendor-specific node (Gunyah/GenieZone/Halla). RAM overhead 5-20 MB. Boot time 5-50ms. I/O throughput within 5% of native. Each guest needs its own kernel. Limited device passthrough.

```text
CHIP                              HYPERVISOR  VMM         GEN
──────────────────────────────────────────────────────────────────
Qualcomm Snapdragon 8 Gen 1-4     Gunyah      crosvm      2022+
MediaTek Dimensity 7200-9300      GenieZone   crosvm      2023+
Samsung Exynos 2200/2400          Halla       crosvm      2022+
Google Tensor G1-G4               KVM         crosvm      2021+
Intel Core/Xeon                   KVM         Firecracker any kvm-capable
AMD Ryzen/EPYC                    KVM         Firecracker any kvm-capable
```

Code: <kbd>internal/dokivm/</kbd>. Detection works. Not tested.

<hr>

## Sysbox

<sub>[MODE 9]</sub>

Sysbox is a runc-compatible runtime that adds nested namespace support to OCI containers. Enables running a full Docker daemon inside a container with proper UTS/PID/IPC/Mount isolation. Requires <kbd>sysbox-runc</kbd> in <kbd>$PATH</kbd>, Linux kernel 4.18+, user namespaces enabled. Near-native performance. ~5% overhead for nested namespace operations. Some <kbd>ptrace</kbd> operations fail across the nested boundary. Needs sysbox-runc installed separately. Code: <kbd>pkg/runtime/runners/sysbox/runner.go</kbd> (planned). Detection works. Not tested.

<hr>

## Namespaces

<sub>[MODE 8]</sub>

Standard Linux namespaces: UTS, PID, IPC, Mount, Net, User, Cgroup. The default rootful mode Docker and Podman use. Requires Linux kernel 3.8+, root or user namespaces, <kbd>/proc/self/ns/</kbd> accessible. Negligible overhead: under 1% CPU, under 0.5% memory. Best of all kernel-level modes. Kernel exploits can break out (CVE-2022-0185, CVE-2022-0492). Does not isolate kernel resources like <kbd>/proc</kbd> and <kbd>/sys</kbd>. Code: <kbd>pkg/runtime/runtime.go:startWithNamespaces()</kbd>. Tested.

<hr>

## gVisor

<sub>[MODE 7]</sub>

Google's user-space kernel. The <kbd>runsc</kbd> runtime intercepts syscalls and re-implements them in Go. Roughly 70% of syscalls never reach the host kernel. Requires <kbd>runsc</kbd> in <kbd>$PATH</kbd>, Linux kernel 4.14+, no raw socket access. ~20% CPU overhead. Network throughput ~70% of native. Some syscalls unimplemented (raw sockets, certain ioctls). Larger image size. Not all applications work (anything using <kbd>perf</kbd> or <kbd>eBPF</kbd> directly). Code: <kbd>pkg/runtime/runners/gvisor/runner.go</kbd> (planned). Detection works. Not tested.

<hr>

## FEX-Emu

<sub>[MODE 6]</sub>

FEXInterpreter or Box64 translates x86/x86_64 binaries to ARM64 at runtime. The container runs an x86 image. FEX translates each instruction on the fly. Requires <kbd>FEXInterpreter</kbd> or <kbd>box64</kbd> in <kbd>$PATH</kbd>, ARM64 host, x86 or x86_64 image. ~30% CPU overhead for compute-bound workloads. I/O near-native. Memory overhead ~20% due to translation cache. Does not handle kernel-level operations (KPTI, vDSO). Some AVX/AVX2 instructions not translated. Code: <kbd>pkg/runtime/runners/fex/runner.go</kbd> (planned). Detection works. Not tested.

<hr>

## QEMU User

<sub>[MODE 5]</sub>

QEMU user-mode emulation. Runs binaries of a different architecture via <kbd>qemu-aarch64-static</kbd>, <kbd>qemu-x86_64-static</kbd>, etc. Requires <kbd>qemu-&lt;arch&gt;-static</kbd> in <kbd>$PATH</kbd> or binfmt_misc registered. Any host architecture. ~50% CPU overhead. Slowest of the emulated modes. No KVM acceleration (user-mode, not system-mode). Some Linux-specific features (e.g. <kbd>prctl(PR_SET_NAME)</kbd>) behave differently. Code: <kbd>pkg/runtime/runners/qemu/runner.go</kbd> (planned). Detection works. Not tested.

<hr>

## Proot

<sub>[MODE 4]</sub>

PRoot is a userspace <kbd>chroot</kbd>/<kbd>mount</kbd> implementation using <kbd>ptrace</kbd> to intercept syscalls. No root required. Requires <kbd>proot</kbd> in <kbd>$PATH</kbd> or Doki's bundled <kbd>doki-proot</kbd> fallback (v0.9.2); v0.9.3+ uses <kbd>FindProotBinary()</kbd>. Default runtime on Android/Termux. ~10% CPU overhead from ptrace. Slower than native namespaces. Some syscalls fail (raw <kbd>mount</kbd>, <kbd>pivot_root</kbd>). On Termux, <kbd>LD_PRELOAD</kbd> must be stripped (v0.9.3+ handles this). Code: <kbd>pkg/runtime/runtime.go:retryWithQemu()</kbd>, <kbd>internal/proot/manager.go:FindProotBinary()</kbd>. Tested on Termux/Android.

<hr>

## Legacy32

<sub>[MODE 3]</sub>

Runs ARMv7 containers on ARM64 kernels via <kbd>binfmt_misc</kbd> and multiarch support. The container believes it runs on ARMv7. The kernel is ARM64 with ARMv7 compatibility. Requires ARM64 host kernel, <kbd>binfmt_misc</kbd> registered for ARMv7, <kbd>qemu-arm-static</kbd> for non-binfmt paths. Negligible overhead when <kbd>binfmt_misc</kbd> is set up. ARM64 kernel handles ARMv7 syscalls natively. No real 32-bit memory addressing (always 64-bit). Some 32-bit-only operations (OABI syscalls) unsupported. Code: <kbd>pkg/runtime/runners/legacy32/runner.go</kbd> (planned). Detection works. Not tested.

<hr>

## Chroot

<sub>[MODE 2]</sub>

Plain <kbd>chroot(2)</kbd> for filesystem isolation. No PID namespace, no network namespace, no user namespace. Changes the root directory only. Requires root. Negligible overhead. No real isolation. Process can escape via <kbd>/proc</kbd>. Requires root. Not suitable for multi-tenant. Code: <kbd>pkg/runtime/runtime.go:startWithChroot()</kbd>. Not tested.

<hr>

## Native

<sub>[MODE 1]</sub>

No isolation. The container is a directory plus environment variables. The process runs directly on the host. No requirements. Always available. Zero overhead. No isolation. Process can do anything the host user can. Do not use for untrusted code. Fallback when nothing else works. Also the macOS CLI mode. Code: <kbd>pkg/runtime/runtime.go:startWithNative()</kbd>. Tested.

<hr>

## Future Levels

<sub>[FUTURE]</sub>

Planned for v0.11.0+ and beyond.

```text
LEVEL                 VERSION  DESCRIPTION
──────────────────────────────────────────────────────────────────────────
landlock              v0.11   kernel sandbox on top of any mode, restricts fs
io_uring isolation    v0.11   per-container ring with restricted opcode set
gpu passthrough       v0.11   for AI/ML workloads on microVM
confidential compute  v1.0    SEV-SNP / TDX on AMD/Intel, TrustZone on ARM
```

<hr>

## Reference

<sub>[SOURCE]</sub>

- Source: <kbd>pkg/runtime/registry.go</kbd>, <kbd>pkg/runtime/runners/*/</kbd>
- Decision logic: <kbd>pkg/runtime/runtime.go:detectMode()</kbd>
- Proot fallback: <kbd>pkg/runtime/runtime.go:retryWithQemu()</kbd>
- Auto-detection: <kbd>pkg/runtime/registry.go:hostPlatform()</kbd>