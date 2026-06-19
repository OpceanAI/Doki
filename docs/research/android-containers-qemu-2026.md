# Research: Linux Containers on Android via QEMU/KVM (May–June 2026)

**Date:** June 2026  
**Scope:** Feasibility of running Linux containers on Android using QEMU (with/without KVM) for the Doki container engine  
**Context:** Doki currently uses proot (ptrace-based) as the default Android/Termux execution mode. This research evaluates whether QEMU-based approaches can provide stronger isolation and better compatibility.

---

## Table of Contents

1. [KVM on Android](#1-kvm-on-android)
2. [QEMU on Android (Termux)](#2-qemu-on-android-termux)
3. [Android Kernel Capabilities for Containers](#3-android-kernel-capabilities-for-containers)
4. [Microdroid / AVF (Android Virtualization Framework)](#4-microdroid--avf-android-virtualization-framework)
5. [Practical Approaches for Containers on Android](#5-practical-approaches-for-containers-on-android)
6. [Custom Rootfs for Android QEMU VM](#6-custom-rootfs-for-android-qemu-vm)
7. [Existing Solutions](#7-existing-solutions)
8. [Recommendations for Doki](#8-recommendations-for-doki)

---

## 1. KVM on Android

### 1.1 Which Android Devices Support KVM?

**Short answer:** Very few, and getting access is extremely restricted.

| Device/SoC | KVM Status | Notes |
|:-----------|:-----------|:------|
| **Google Pixel 6/7/8/9 (Tensor G1–G4)** | `/dev/kvm` exists in kernel | SELinux blocks access from user-space apps. Only system processes with `vendor_hypervisor_service` SELinux context can open it. |
| **Snapdragon 8 Gen 1/2/3/4 devices** | Hypervisor is **Gunyah** (Qualcomm's own), NOT KVM | No `/dev/kvm` node. Gunyah has its own device node (`/dev/gunyah`), but it's locked to vendor processes. |
| **MediaTek Dimensity 9200/9300** | Hypervisor is **GenieZone** | Similar to Gunyah—vendor-locked, no user-space access. |
| **Samsung Exynos 2200/2400** | Hypervisor is **Halla** | Same story—vendor-locked. |
| **Custom kernel builds (Xiaomi, OnePlus, etc.)** | Some custom ROMs expose `/dev/kvm` | Requires unlocked bootloader + custom kernel with `CONFIG_KVM=y`. Very rare. |
| **Android emulators (x86_64)** | `/dev/kvm` available on host | This is the host machine's KVM, not the Android guest's. |

**Key finding:** On production Android devices, `/dev/kvm` is either absent (non-KVM hypervisors) or present but SELinux-blocked. The 2026 landscape has not changed materially from 2024 in this regard.

**Root access changes things:** On rooted devices with custom kernels, `/dev/kvm` can be made accessible. The Termux crosvm PR (#29792, May 2026) shows that even with root, crosvm on Android fails because `minijail`'s `clone(CLONE_NEWPID)` fails inside the Android sandbox. This is a fundamental limitation of Android's security model, not just SELinux.

### 1.2 Is /dev/kvm Accessible on Android?

**Without root:** No. SELinux policy on all stock Android ROMs denies `untrusted_app` and `untrusted_app_27` domains from opening `/dev/kvm`. The denial is `avc: denied { open } for comm="..." name="kvm" dev="tmpfs" ino=... scontext=u:r:untrusted_app:s0:c512,c768 tcontext=u:object_r:kvm_device:s0 tclass=chr_file`.

**With root (magisk/KernelSU):** Possible but requires:
1. Setting permissive SELinux mode (`setenforce 0`), OR
2. Adding a custom SELinux policy allowing the Termux app domain to access `/dev/kvm`
3. Even then, Android's `restrict_syscall` and seccomp-bpf filters may block `ioctl(KVM_CREATE_VM)` from non-system processes.

**Termux root-repo:** The `termux-root` repository does NOT ship a KVM-enabled QEMU. The `qemu-system-*` packages in Termux use TCG (software emulation) only.

### 1.3 What Hypervisor Does Android Use Internally?

| Android Version | Hypervisor | Architecture |
|:----------------|:-----------|:-------------|
| **Android 10–11** | None (or vendor-specific TrustZone) | N/A |
| **Android 12–13** | Vendor-specific: Gunyah (Qualcomm), GenieZone (MediaTek), Halla (Samsung) | ARM64 EL2 |
| **Android 14** | pKVM begins appearing on Pixel 8/9 (Tensor G3/G4) | ARM64 EL2, based on KVM code |
| **Android 15+** | **pKVM** is the reference hypervisor for AVF | Protected KVM — the host Linux kernel CANNOT access VM memory |

**pKVM (Protected KVM):** This is Google's fork of KVM where the hypervisor runs at EL2 and the host Linux kernel (EL1) is treated as an untrusted guest. Key properties:
- The host kernel cannot read/write pKVM guest memory
- The host kernel cannot inject interrupts into pKVM guests
- pKVM guests are isolated from each other AND from the host
- Access is mediated through hypercalls, not `/dev/kvm` ioctls
- Only the Android Virtualization Framework (AVF) can create pKVM guests

### 1.4 Can User-Space Apps Access KVM Without Root?

**No.** This is a hard "no" on all stock Android devices as of June 2026. The access chain is:

```
App → SELinux denies → /dev/kvm open fails
```

Even if SELinux were bypassed:
```
App → ioctl(KVM_CREATE_VM) → seccomp-bpf blocks → EPERM
```

The ONLY path to hardware virtualization from user-space on Android is through the **Android Virtualization Framework (AVF)**, which provides a controlled API for creating pKVM guests. See Section 4.

### 1.5 Android Virtualization Framework (AVF) / Microdroid

**AVF** is Google's abstraction layer over vendor hypervisors (pKVM, Gunyah, GenieZone, Halla). It provides:

1. **`VirtualMachineManager`** — Android system service that manages VM lifecycle
2. **`VirtualMachine`** — API for creating and controlling VMs
3. **Microdroid** — A minimal Linux OS that runs inside AVF VMs
4. **pVM Firmware** — Verified boot chain for VM guests

**How it works:**
```
Android App
  → VirtualMachineManagerService (system_server)
    → vendor hypervisor (pKVM/Gunyah/GenieZone/Halla)
      → Microdroid VM
        → Linux kernel (minimal)
          → payload (attestation, ML inference, etc.)
```

**Restrictions:**
- Only available on Android 12+ (pKVM requires Android 15+ for full protection)
- Only accessible from Android apps with `android.permission.USE_BIOMETRIC` or specific AVF permissions
- VMs cannot access the network by default
- VMs cannot access arbitrary host filesystem
- Communication with host is via **vsock** (virtio-vsock) only
- The VM payload must be signed and verified (attestation)
- No arbitrary Linux distribution can run — only Microdroid or custom AVF-compatible images

**Can Doki use AVF directly?** Not from Termux. AVF is an Android framework API (Java/Kotlin), not a Linux syscall interface. A Termux app running in the `untrusted_app` SELinux domain cannot call AVF APIs. You would need a companion Android app (APK) that creates VMs via AVF and exposes them to Termux via vsock or a Unix socket.

---

## 2. QEMU on Android (Termux)

### 2.1 Can QEMU Run in Termux?

**Yes.** Termux ships `qemu-system-*` packages:

```bash
pkg install qemu-system-aarch64-headless  # ARM64 system emulation
pkg install qemu-system-x86_64-headless   # x86_64 system emulation
pkg install qemu-user-aarch64             # ARM64 user-mode
pkg install qemu-user-x86_64              # x86_64 user-mode
```

**Important notes:**
- These are **TCG-only** (software emulation). No KVM acceleration.
- The `qemu-system-*-headless` variants omit SDL/GTK display support (appropriate for Termux).
- QEMU system-mode CAN boot a Linux kernel inside Termux, but it's slow.
- The crosvm PR (#29792) in Termux (May 2026) attempts to package crosvm for Termux, but it fails on Android due to minijail namespace issues (see Section 1.2).

### 2.2 Performance: QEMU User-Mode vs System-Mode on Android ARM64

| Mode | CPU Overhead | Memory Overhead | Use Case |
|:-----|:-------------|:----------------|:---------|
| **QEMU user-mode** (same arch) | ~5-10% | ~20 MB | Cross-distro compat (e.g., glibc vs musl). Rarely needed on ARM64. |
| **QEMU user-mode** (cross arch: x86→ARM64) | ~50-70% | ~30 MB | Running x86 containers on ARM64. |
| **QEMU system-mode** (same arch, TCG) | ~300-500% | ~128-256 MB minimum | Running a full Linux VM. |
| **QEMU system-mode** (cross arch) | ~1000-2000% | ~256-512 MB minimum | Not practical for containers. |

**Benchmarks (estimated, ARM64 host, ARM64 guest, TCG):**
- Boot to shell: 15-30 seconds
- `apt install`: 5-10x slower than native
- Network throughput: ~50-100 Mbps (virtio-net, user-mode networking)
- Disk I/O: ~30-50% of native (virtio-blk, raw image)

**Key insight:** QEMU system-mode on ARM64 with TCG is **usable but slow** for lightweight workloads (alpine, busybox). For heavy workloads (databases, build systems), the 3-5x CPU overhead is prohibitive.

### 2.3 Can QEMU System-Mode Run a Linux Kernel on Android's ARM64?

**Yes.** QEMU system-mode can boot a standard ARM64 Linux kernel:

```bash
qemu-system-aarch64 \
  -M virt -cpu cortex-a72 -m 256 \
  -kernel Image \
  -drive file=rootfs.ext4,format=raw,if=virtio \
  -append "root=/dev/vda console=ttyAMA0" \
  -nographic
```

**Requirements:**
- A compiled ARM64 Linux kernel (`Image`, not `Image.gz` for QEMU virt machine)
- A rootfs image (ext4, or initramfs)
- ~128-256 MB RAM for the guest

**Can we use the Android host kernel?** No. The Android host kernel is configured for the specific SoC hardware. QEMU's `virt` machine requires a kernel configured for QEMU's virtual hardware (virtio devices, GICv3 interrupt controller, etc.). You need a **separate kernel** compiled with:
- `CONFIG_ARCH_VIRT=y`
- `CONFIG_VIRTIO_BLK=y`
- `CONFIG_VIRTIO_NET=y`
- `CONFIG_9P_FS=y` (for shared filesystem)
- `CONFIG_OVERLAY_FS=y` (for containers inside the VM)
- `CONFIG_NAMESPACES=y`, `CONFIG_CGROUPS=y` (for containers)

### 2.4 What Acceleration Is Available?

| Platform | Acceleration | Accessible from Termux? |
|:---------|:-------------|:------------------------|
| **macOS ARM64** | HVF (Hypervisor.framework) | Yes (via QEMU `-accel hvf`) |
| **Linux x86_64** | KVM | Yes (via QEMU `-accel kvm`) |
| **Linux ARM64** | KVM | Yes (if `/dev/kvm` accessible) |
| **Android ARM64** | **None** (TCG only) | **No KVM, no HVF equivalent** |
| **Android ARM64 (rooted, custom kernel)** | KVM (maybe) | Only if SELinux + seccomp allow |
| **Android ARM64 (AVF)** | pKVM | Only via AVF API, not QEMU |

**Bottom line:** On Android, QEMU runs in **pure software emulation (TCG)**. There is no hardware acceleration path available from Termux.

### 2.5 Memory Overhead of Running a VM Inside Android

| Component | Memory |
|:----------|:-------|
| Android OS (baseline) | ~1-2 GB |
| Termux + Doki (proot mode) | ~20-50 MB |
| QEMU system-mode process | ~30-50 MB (VMM overhead) |
| Guest kernel | ~20-30 MB |
| Guest userspace (minimal) | ~30-50 MB |
| Guest page cache + apps | ~64-256 MB |
| **Total for VM approach** | **~200-400 MB** |

Compare to proot mode:
| Component | Memory |
|:----------|:-------|
| Android OS (baseline) | ~1-2 GB |
| Termux + Doki (proot mode) | ~20-50 MB |
| Container rootfs (shared pages) | ~5-30 MB |
| **Total for proot approach** | **~30-80 MB** |

**The VM approach adds 150-350 MB of overhead.** On a phone with 8 GB RAM, this is tolerable. On a phone with 4 GB, it's significant.

---

## 3. Android Kernel Capabilities for Containers

### 3.1 Which Kernel Features Does Android's Kernel Have?

Android kernels are based on mainline Linux but with significant modifications. As of Android 14/15 (kernel 5.15/6.1/6.6):

| Feature | Status | Notes |
|:--------|:-------|:------|
| **CONFIG_NAMESPACES** | **Yes** (since Android 8+) | But some are restricted by SELinux |
| **CONFIG_USER_NS** | **Partial** | Enabled in kernel but `unprivileged_userns_clone` may be 0. Android 14+ enables it for `untrusted_app` on some devices. |
| **CONFIG_PID_NS** | **Yes** | Works, but PID 1 semantics differ under proot |
| **CONFIG_NET_NS** | **Yes** (root only) | Requires `CAP_SYS_ADMIN`; not available to untrusted apps |
| **CONFIG_MNT_NS** | **Yes** | But mount operations are restricted by SELinux |
| **CONFIG_UTS_NS** | **Yes** | Works |
| **CONFIG_IPC_NS** | **Yes** | Works |
| **CONFIG_CGROUPS** | **Yes** | cgroups v1 on most devices; cgroups v2 on Android 13+ |
| **CONFIG_OVERLAY_FS** | **Varies** | Some vendor kernels disable it. GKI (Generic Kernel Image) kernels typically have it. |
| **CONFIG_SECCOMP** | **Yes** | Android uses seccomp-bpf extensively |
| **CONFIG_SECURITY_SELINUX** | **Yes** | Enforcing on all production devices |
| **CONFIG_VETH** | **Usually no** | Not needed for Android's networking model |
| **CONFIG_BRIDGE** | **Usually no** | Same |
| **CONFIG_TUN** | **Usually no** | Requires root |
| **CONFIG_BPF** | **Yes** | eBPF available on Android 12+ |
| **CONFIG_MEMCG** | **Yes** | Memory cgroups enabled |

### 3.2 Namespaces: Which Ones Are Enabled?

**User namespaces (`CLONE_NEWUSER`):**
- Android 12+: Kernel has `CONFIG_USER_NS=y`
- But `kernel.unprivileged_userns_clone` sysctl may be 0
- Android 14+: Google is relaxing this — Pixel devices allow unprivileged user namespaces
- Samsung, Xiaomi, etc.: Typically still blocked
- **For Doki:** Cannot rely on user namespaces on all Android devices

**PID namespaces (`CLONE_NEWPID`):**
- Available with `CAP_SYS_ADMIN` (root)
- Without root: requires user namespace first (which is blocked on many devices)
- **For Doki:** Not available without root on most devices

**Network namespaces (`CLONE_NEWNET`):**
- Requires `CAP_SYS_ADMIN`
- **For Doki:** Not available without root

**Mount namespaces (`CLONE_NEWNS`):**
- Requires `CAP_SYS_ADMIN` on most kernels
- Some kernels allow it with user namespaces
- **For Doki:** Not available without root on most devices

### 3.3 cgroups v2: Available on Android 13+?

**Yes.** Android 13+ uses cgroups v2 (unified hierarchy) by default:
- `/sys/fs/cgroup` is mounted as cgroup v2
- Android uses it for app process management (memory limits, CPU scheduling)
- Available to root; read-only for unprivileged apps
- **For Doki:** Can be used for resource limits if running as root. Without root, cannot create new cgroups.

### 3.4 overlayfs: Available in Android Kernel?

**It depends on the vendor:**
- **GKI (Generic Kernel Image) devices** (Pixel 6+, most Android 13+): `CONFIG_OVERLAY_FS=y` or `=m`. Available.
- **Vendor-custom kernels** (Samsung, Xiaomi, etc.): May be disabled. Some vendors patch it out.
- **FUSE-overlayfs:** Works in user-space via FUSE. Available on all Android versions that support FUSE (Android 5+). This is what Doki currently uses.

**For Doki:** `fuse-overlayfs` is the safe choice. Kernel overlayfs is a nice-to-have that works on some devices.

### 3.5 What Can proot Do That the Kernel Can't?

| Capability | Kernel (with root) | proot (without root) |
|:-----------|:-------------------|:---------------------|
| Filesystem isolation | `mount --bind`, `pivot_root` | ptrace-based path rewriting |
| PID isolation | PID namespaces | Fake PID 1 (intercepts `getpid()`) |
| Network isolation | Network namespaces | None (shares host network) |
| User isolation | User namespaces, `setuid` | Fake UID/GID (intercepts `getuid()`) |
| Mount isolation | Mount namespaces | bind emulation via ptrace |
| IPC isolation | IPC namespaces | SysV IPC emulation |
| Resource limits | cgroups | None |
| Security | seccomp, AppArmor/SELinux | None (inherits host policy) |

**proot's magic:** It uses `ptrace(PTRACE_SYSCALL)` to intercept every syscall from the guest process and rewrites path arguments in-place. This gives the illusion of a separate filesystem root without any kernel support. The cost is ~10% CPU overhead per syscall.

### 3.6 SELinux Impact on Container Operations

SELinux is the **single biggest obstacle** to running real containers on Android:

1. **File access:** SELinux labels (`seclabel`) on files restrict which processes can read/write them. A container process in `untrusted_app` domain cannot access files labeled `system_file`, `vendor_file`, etc.

2. **Network access:** `socket()` calls are filtered by SELinux. Creating raw sockets, TAP devices, or netlink sockets is denied for `untrusted_app`.

3. **Mount operations:** `mount()` is denied for non-system domains. Even with root, SELinux may block mount unless the domain has `mount` permission.

4. **Process execution:** `execve()` is filtered. Running binaries from `/data/data/com.termux/` is allowed (app domain), but running binaries from `/system/bin/` may trigger SELinux denials.

5. **ptrace:** `ptrace()` is restricted. Android 10+ blocks cross-process ptrace by default. proot works because it ptraces its own children (same UID), but even this can be blocked on some vendor kernels.

**Impact on Doki approaches:**
- **proot:** Works because it operates within the SELinux constraints (same UID, same domain, ptracing children)
- **Namespaces:** Blocked by SELinux (cannot create new namespaces from `untrusted_app`)
- **QEMU system-mode:** Works because QEMU is just a regular process — it doesn't need special SELinux permissions. The guest kernel inside QEMU has its own security model.
- **AVF/Microdroid:** Works because it goes through the official Android framework API, which has the proper SELinux permissions.

---

## 4. Microdroid / AVF (Android Virtualization Framework)

### 4.1 How Does pKVM Work on Android 15+?

pKVM is a **protected hypervisor** at ARM EL2:

```
┌─────────────────────────────────────────┐
│  EL0: User-space (Android apps, Termux) │
├─────────────────────────────────────────┤
│  EL1: Host Linux kernel (Android)       │ ← Cannot access pKVM guest memory
│       + vendor HALs                      │
├─────────────────────────────────────────┤
│  EL2: pKVM hypervisor                   │ ← Controls memory access, interrupts
│       (based on KVM code, but protected) │
├─────────────────────────────────────────┤
│  EL3: TrustZone / PSCI                  │
└─────────────────────────────────────────┘
```

**Key properties:**
- Host kernel runs as a "guest" of pKVM (even the host Linux kernel is at the mercy of the hypervisor)
- pKVM guests have memory that the host kernel CANNOT read or write
- Page table manipulation by the host is intercepted by pKVM
- VM creation/management is done through the AVF Java API, not `/dev/kvm`
- Attestation: pKVM can prove to a remote verifier that a specific payload is running in an isolated VM

### 4.2 What Is Microdroid?

Microdroid is a **minimal Android-derived Linux OS** designed to run inside AVF VMs:

- **Size:** ~10-20 MB (kernel + initramfs)
- **Boot time:** <1 second
- **Components:** Linux kernel, init, linker, bionic libc, a payload runner
- **No:** Android framework, apps, GUI, most Android services
- **Purpose:** Run verified, attested workloads (ML models, key management, DRM)

**Microdroid is NOT a general-purpose Linux distribution.** It:
- Cannot run arbitrary binaries (payload must be APK or native binary signed by the app)
- Has no package manager
- Has no network by default (can be enabled via `VirtualMachineConfig.Builder.setProtectedVm(false)` + network config)
- Uses a read-only rootfs (overlay for writable state)
- Communicates with host via vsock only

### 4.3 Can Doki Use AVF to Run a Linux VM?

**Not directly.** AVF is designed for specific use cases (attestation, ML, DRM), not general-purpose containers. However, a **companion Android app** could:

1. Create an AVF VM with a custom payload
2. The payload could be a minimal Linux init system that runs containers
3. Expose the VM to Termux via vsock

**Restrictions:**
- Requires Android 12+ (pKVM requires Android 15+)
- Requires the device to support AVF (not all devices do)
- VM must be created by an Android app (not Termux directly)
- VM payload must be signed by the app's signing key
- No direct network access from VM (requires host-side proxy)
- Limited to the device's hypervisor capabilities

**Practical assessment:** AVF is theoretically the "best" approach for hardware-isolated containers on Android, but the restrictions make it impractical for a general-purpose container engine like Doki. It could work as an **optional premium feature** for specific use cases (e.g., running sensitive workloads in attested VMs).

### 4.4 VM-Host Communication

| Channel | Available | Notes |
|:--------|:----------|:------|
| **vsock (virtio-vsock)** | Yes | Primary channel. Stream-oriented, like TCP but for VM↔host. |
| **virtio-serial** | Yes | Serial console, used for debug output |
| **virtio-fs / 9p** | Limited | Shared filesystem. Microdroid uses a read-only shared partition. |
| **Network (NAT)** | Opt-in | Requires `setProtectedVm(false)` + explicit network config |
| **Shared memory** | Possible | Via virtio-shm or custom mechanism |

### 4.5 Restrictions Summary

| Restriction | Impact on Doki |
|:------------|:---------------|
| Must be Android app (not Termux CLI) | Requires companion APK |
| Payload must be signed | Cannot run arbitrary rootfs |
| No network by default | Must implement host-side proxy |
| No arbitrary filesystem access | Must bundle rootfs in VM image |
| Device must support AVF | Not universal |
| Android 12+ (pKVM: Android 15+) | Limits device coverage |
| VM memory is fixed at creation | Cannot dynamically resize |

---

## 5. Practical Approaches for Containers on Android

### Approach A: proot (Current Doki Approach)

**How it works:** Doki uses `doki-proot` (ptrace-based) to intercept syscalls and rewrite filesystem paths, giving each container the illusion of its own root filesystem.

**Strengths:**
- Works on ALL Android devices (Android 5+, no root required)
- Zero additional memory overhead (~10% CPU per syscall)
- Fast startup (<15ms container start)
- No kernel dependencies beyond ptrace
- Already implemented and tested in Doki

**Weaknesses:**
- No real isolation (PID, network, user namespaces are faked)
- Cannot run setuid binaries
- Cannot use `mount()` inside containers
- Some syscalls are not fully emulated (e.g., `io_uring`, `bpf`)
- Performance overhead on syscall-heavy workloads (~10-20%)
- Cannot run Docker-in-Docker
- SELinux still applies to container processes

**Best for:** Development environments, CLI tools, lightweight servers, CI/CD on mobile

### Approach B: QEMU System-Mode + Linux VM + Containers Inside

**How it works:** Run QEMU system-mode in Termux, boot a Linux kernel with a custom rootfs, and run real containers (with namespaces, cgroups) inside the VM.

**Strengths:**
- Real Linux kernel with full namespace support
- Real cgroups for resource limits
- Real overlayfs for container filesystems
- Real network stack (NAT via QEMU user-mode networking)
- Can run Docker-in-Docker
- SELinux of the host does NOT apply inside the VM
- Can run containers with full isolation

**Weaknesses:**
- 3-5x CPU overhead (TCG software emulation)
- 150-400 MB additional memory
- Slow boot (15-30 seconds to shell)
- Requires a separate kernel binary
- Requires a rootfs image (ext4 or initramfs)
- Complex setup (kernel, rootfs, networking)
- No hardware acceleration (KVM not accessible)
- Storage I/O overhead

**Performance estimate:**
- Container start: 20-40 seconds (VM boot + container start)
- CPU-bound workload: 30-50% of native speed
- Memory: 256-512 MB for a usable VM
- Network: ~50-100 Mbps through QEMU user-mode NAT

**Best for:** Running full Docker workloads, Docker-in-Docker, workloads that require real namespaces, testing production container images

### Approach C: AVF/Microdroid + Linux VM + Containers Inside

**How it works:** Use Android's Virtualization Framework to create a pKVM-isolated VM running a custom Linux payload that includes container runtime.

**Strengths:**
- Hardware-level isolation (pKVM)
- Near-native performance (KVM acceleration)
- Fast boot (<1 second for Microdroid)
- Attestation support
- Google-supported path

**Weaknesses:**
- Requires companion Android app (APK)
- Requires Android 12+ (pKVM: Android 15+)
- Not all devices support AVF
- No network by default
- Payload must be signed
- Complex architecture (Termux → Android app → AVF → VM → containers)
- Not accessible from Termux CLI directly
- Microdroid is not a general-purpose Linux

**Best for:** Enterprise/security-sensitive workloads, attested compute, DRM, key management

### Approach D: Kernel Namespaces Directly (Root Required)

**How it works:** On rooted Android devices with permissive SELinux, use Linux namespaces directly (like rootless Podman on desktop Linux).

**Strengths:**
- Near-native performance
- Real isolation (PID, network, mount, user namespaces)
- Real cgroups
- Low overhead (~5-10 MB per container)
- Fast startup (<100ms)

**Weaknesses:**
- Requires root
- Requires permissive SELinux or custom policy
- Not all namespace types available on all devices
- User namespaces blocked on many vendor kernels
- Network namespaces require veth/bridge setup (no `CONFIG_VETH` on many kernels)
- SELinux labels still apply
- Fragile across Android versions and vendors

**Best for:** Rooted devices, custom ROMs, development/testing

### Comparison Matrix

| Criterion | A: proot | B: QEMU VM | C: AVF | D: Namespaces |
|:----------|:---------|:-----------|:-------|:--------------|
| **Root required** | No | No | No (but needs APK) | Yes |
| **SELinux issues** | None | None | None | Major |
| **Device coverage** | 100% | 100% | ~30% (Android 12+) | ~10% (rooted) |
| **CPU overhead** | ~10% | ~300-500% | ~5% | ~0% |
| **Memory overhead** | ~10 MB | ~200-400 MB | ~50-100 MB | ~5-10 MB |
| **Container start** | <15ms | 20-40s | 1-5s | <100ms |
| **Real isolation** | No | Yes | Yes | Yes |
| **Network** | Shared host | NAT (QEMU) | vsock only | Bridge (if veth) |
| **Docker-in-Docker** | No | Yes | Maybe | Yes |
| **Overlayfs** | fuse-overlayfs | Kernel overlayfs | Kernel overlayfs | Kernel overlayfs |
| **cgroups** | No | Yes | Yes | Yes |
| **Implementation complexity** | Low | Medium | Very High | Medium |
| **Doki readiness** | ✅ Done | ⚠️ Partial (QEMU backend exists) | ❌ Not started | ⚠️ Partial |

---

## 6. Custom Rootfs for Android QEMU VM

### 6.1 What Would a Minimal Rootfs Need?

For running containers inside QEMU on Android, the rootfs needs:

```
/bin/busybox          # Core utilities
/bin/sh               # Shell (busybox ash or bash)
/sbin/init            # Init system (busybox init or custom)
/etc/passwd           # User database
/etc/group            # Group database
/etc/resolv.conf      # DNS (provided by QEMU)
/proc/                # Mount point
/sys/                 # Mount point
/dev/                 # Mount point (devtmpfs)
/tmp/                 # tmpfs
/var/lib/doki/        # Doki data directory
/usr/bin/dokid        # Doki daemon
/usr/bin/doki         # Doki CLI
/usr/bin/runc         # OCI runtime (for real containers)
/usr/bin/fuse-overlayfs  # Storage driver
```

**Minimum viable rootfs:** ~50-80 MB (Alpine-based)

### 6.2 Kernel Requirements

**Cannot use the Android host kernel.** Need a separate kernel compiled for QEMU's `virt` machine:

```
CONFIG_ARCH_VIRT=y
CONFIG_ARM64_VA_BITS_48=y
CONFIG_SMP=y
CONFIG_NR_CPUS=4
CONFIG_CGROUPS=y
CONFIG_NAMESPACES=y
CONFIG_USER_NS=y
CONFIG_PID_NS=y
CONFIG_NET_NS=y
CONFIG_MNT_NS=y
CONFIG_UTS_NS=y
CONFIG_IPC_NS=y
CONFIG_OVERLAY_FS=y
CONFIG_VIRTIO=y
CONFIG_VIRTIO_BLK=y
CONFIG_VIRTIO_NET=y
CONFIG_VIRTIO_CONSOLE=y
CONFIG_HW_RANDOM_VIRTIO=y
CONFIG_9P_FS=y
CONFIG_NET_9P=y
CONFIG_NET_9P_VIRTIO=y
CONFIG_TUN=y
CONFIG_VETH=y
CONFIG_BRIDGE=y
CONFIG_BRIDGE_NETFILTER=y
CONFIG_NETFILTER_XT_MATCH_ADDRTYPE=y
CONFIG_NETFILTER_XT_MATCH_CONNTRACK=y
CONFIG_NF_NAT=y
CONFIG_NF_CONNTRACK=y
CONFIG_IP_NF_IPTABLES=y
CONFIG_IP_NF_NAT=y
CONFIG_IP_NF_FILTER=y
CONFIG_FUSE_FS=y
```

**Kernel size:** ~5-10 MB (compressed Image.gz)

### 6.3 Network: How to Get Networking from QEMU Guest on Android?

**QEMU user-mode networking (SLIRP):**
```bash
-netdev user,id=net0,hostfwd=tcp::8080-:80
-device virtio-net-device,netdev=net0
```

- Guest gets DHCP from QEMU's built-in DHCP server (10.0.2.x)
- QEMU proxies TCP/UDP from guest to Android's network stack
- Port forwarding: `hostfwd=tcp::8080-:80` maps host:8080 to guest:80
- DNS: QEMU forwards to Android's DNS (10.0.2.3)
- **Performance:** ~50-100 Mbps, adequate for most workloads
- **No root required** on the Android host

**Alternative: TAP device (requires root):**
```bash
-netdev tap,id=net0,ifname=tap0,script=no,downscript=no
```
- Better performance (~native speed)
- Requires root + TUN/TAP kernel module
- Requires manual bridge setup

### 6.4 Storage: How to Share Android Filesystem with QEMU Guest?

**Option 1: 9p virtio (recommended)**
```bash
-fsdev local,id=shared,path=/data/data/com.termux/files/home,security_model=none
-device virtio-9p-device,fsdev=shared,mount_tag=hostshare
```
Inside guest:
```bash
mount -t 9p -o trans=virtio hostshare /mnt/host
```
- Read/write access to Android filesystem
- Performance: ~30-50% of native I/O
- No root required

**Option 2: virtio-fs (better performance, needs daemon)**
```bash
# On host: run virtiofsd
virtiofsd --shared-dir /data/data/com.termux/files/home --socket-path=/tmp/vhost.sock
# QEMU args:
-chardev socket,id=char0,path=/tmp/vhost.sock
-device vhost-user-fs-pci,chardev=char0,tag=hostshare
```
- Better performance than 9p
- Requires `virtiofsd` binary (Rust, ~10 MB)
- More complex setup

**Option 3: Block device (ext4 image)**
```bash
-drive file=rootfs.ext4,format=raw,if=virtio
```
- Best I/O performance
- No sharing with Android host
- Fixed size

### 6.5 Performance: What's the Overhead?

| Component | Overhead | Notes |
|:----------|:---------|:------|
| CPU (TCG) | 3-5x slower | Every guest instruction is emulated |
| Memory | +200-400 MB | QEMU + guest kernel + guest userspace |
| Disk I/O (9p) | ~50% of native | Path translation overhead |
| Disk I/O (virtio-blk) | ~80% of native | Near-native for raw block |
| Network (SLIRP) | ~50-100 Mbps | Adequate for most workloads |
| Boot time | 15-30s | Kernel + init + userspace |
| Container start (inside VM) | 1-3s | After VM is running |

---

## 7. Existing Solutions

### 7.1 Termux proot-distro

**How it works:** Python orchestration layer around `proot`. Pulls OCI images from Docker Hub, extracts layers, and launches containers via proot.

**Key features (2026):**
- Pulls OCI images directly from registries (Docker Hub, GHCR, etc.)
- Builds OCI images from Dockerfiles (RUN via proot)
- Push built images to registries
- Supports `--isolated` and `--minimal` modes
- Cross-arch via QEMU user-mode (`--emulator`)
- Build cache, layer caching

**Limitations:**
- No real isolation (proot-based)
- No PID/network/IPC namespaces
- No cgroups
- No Docker-in-Docker
- RUN in Dockerfile runs under proot (no real kernel features)

**Relevance to Doki:** Doki's proot mode is similar but with a Go daemon + Docker API compatibility. proot-distro is more of a "distro manager" than a container engine.

### 7.2 Linux Deploy

**How it works:** Android app that creates a disk image (ext4 loopback) and uses `chroot` (requires root) to run a Linux distribution.

**Key features:**
- Requires root
- Uses `chroot` (not proot)
- Creates ext4 loopback images
- Supports VNC/X11 for GUI
- Supports SSH
- Installs full distributions (Ubuntu, Debian, Kali, etc.)

**Limitations:**
- Requires root
- No namespace isolation (just chroot)
- No OCI image support
- No Docker API
- Essentially a chroot manager

**Relevance to Doki:** Linux Deploy is a legacy solution. Doki's chroot mode (Level 2) is similar but with OCI support.

### 7.3 UserLAnd

**How it works:** Android app that uses proot (or optionally SSH/VNC) to run Linux distributions.

**Key features:**
- No root required
- Uses proot for filesystem isolation
- Supports VNC for GUI applications
- Supports SSH for terminal access
- Downloads pre-built rootfs tarballs

**Limitations:**
- No OCI image support
- No Docker API
- No real container isolation
- Limited to pre-built distributions
- Last release: v2.8.3 (Oct 2021) — appears unmaintained

**Relevance to Doki:** UserLAnd is the closest "consumer" competitor but lacks OCI/Docker compatibility. Doki's proot mode is strictly better.

### 7.4 Andronix

**How it works:** Android app that provides pre-configured scripts for installing Linux distributions via proot (in Termux) or chroot (with root).

**Key features:**
- No root required (proot mode)
- Pre-configured desktop environments (XFCE, LXDE, etc.)
- VNC integration for GUI
- Supports multiple distributions

**Limitations:**
- Essentially a script generator for Termux/proot-distro
- No OCI image support
- No Docker API
- No real container isolation
- Depends on Termux + proot-distro

**Relevance to Doki:** Andronix is a UI layer on top of proot-distro. Doki provides the same functionality with a proper container engine underneath.

### 7.5 Projects Running Actual Containers (Not chroot) on Android

**There are very few:**

1. **Docker on Android (rooted, custom kernel):**
   - Requires root + custom kernel with all namespace/cgroup features
   - Uses standard Docker Engine
   - Fragile, breaks with Android updates
   - Not a product, just a hack

2. **Podman on Android (rooted):**
   - Similar to Docker, requires root + custom kernel
   - Rootless Podman doesn't work because user namespaces are blocked

3. **LXC on Android (rooted):**
   - Requires root + custom kernel
   - Uses LXC tooling
   - Not OCI-compatible

4. **Doki (this project):**
   - The only container engine that works on Android WITHOUT root
   - Uses proot as fallback, scales up to namespaces/microVM when available
   - OCI-compatible, Docker API compatible

**Key finding:** As of June 2026, there is NO production-ready solution that runs real (namespace-isolated) containers on Android without root. Doki's proot-based approach is the state of the art for unprivileged Android containers.

---

## 8. Recommendations for Doki

### 8.1 Short-Term (v0.10–v1.0): Optimize proot Mode

The current proot approach is the **correct default** for Android. Recommendations:

1. **Continue using proot as the default Android runtime.** It works on 100% of devices, requires no root, and has acceptable performance.

2. **Improve proot compatibility:**
   - Track `termux/proot` upstream for Android 15/16 fixes
   - Consider `coderredlab/proroot` (LD_PRELOAD approach) as an alternative backend for devices where ptrace is restricted (Xiaomi HyperOS, etc.)
   - Add `--runtime proroot` flag for the LD_PRELOAD variant

3. **Add QEMU user-mode as a cross-arch runner** (Level 5 in Doki's isolation table):
   - Already partially implemented in `pkg/runtime/runners/qemuuser/`
   - Enable `doki run --platform linux/amd64 alpine` on ARM64 Android
   - Requires `qemu-user-x86_64` package in Termux

4. **Document limitations clearly:** Users should understand that proot mode does not provide real isolation.

### 8.2 Medium-Term (v1.0–v2.0): QEMU System-Mode as Optional Backend

Add QEMU system-mode as an **opt-in** backend for users who need real container isolation:

1. **Implement a `doki vm` subcommand:**
   ```bash
   doki vm init          # Download kernel + rootfs, set up QEMU
   doki vm start         # Boot the VM
   doki vm stop          # Shut down the VM
   doki vm status        # Show VM state
   ```

2. **Inside the VM, run a full Doki daemon** with namespace support:
   - The VM runs a minimal Alpine Linux with `dokid` as PID 1
   - Containers inside the VM get real namespaces, cgroups, overlayfs
   - Networking: QEMU user-mode NAT with port forwarding to Android host

3. **Pre-built artifacts:**
   - Ship a pre-compiled ARM64 kernel (~5 MB)
   - Ship a pre-built rootfs image (~50 MB, Alpine-based)
   - Auto-download on first `doki vm init`

4. **Integration with existing Doki CLI:**
   - `doki run --runtime qemu-vm alpine` → boots VM if needed, runs container inside
   - Transparent to the user: same `doki` commands, just a different backend

5. **Performance expectations:**
   - VM boot: 15-30s (first time), ~5s (subsequent with snapshot)
   - Container start inside VM: 1-3s
   - CPU: 30-50% of native (TCG)
   - Memory: 256-512 MB for the VM

### 8.3 Long-Term (v2.0+): AVF Integration (Optional)

If AVF becomes more accessible, consider a companion Android app:

1. **Companion APK** that creates AVF VMs
2. **vsock bridge** between Termux and the AVF VM
3. **Hardware-isolated containers** for security-sensitive workloads

This is a significant engineering effort and should only be pursued if there's clear demand.

### 8.4 Recommended Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Doki CLI (doki)                                        │
│  ├── doki run alpine                                    │
│  ├── doki run --runtime qemu-vm alpine                  │
│  └── doki run --runtime proot alpine                    │
├─────────────────────────────────────────────────────────┤
│  Doki Daemon (dokid)                                    │
│  ├── Registry: selects best runtime                     │
│  │   ├── Level 4: proot (default on Android)            │
│  │   ├── Level 5: QEMU user (cross-arch)                │
│  │   ├── Level 8: namespaces (rooted devices)           │
│  │   ├── Level 10: microVM (KVM devices)                │
│  │   └── Level 11: pKVM/AVF (Android 15+)              │
│  ├── QEMU VM Manager (optional)                         │
│  │   ├── Boots Linux VM via QEMU TCG                    │
│  │   ├── Runs inner dokid inside VM                     │
│  │   └── Port-forwards to Android host                  │
│  └── proot Manager (default)                            │
│      ├── doki-proot (GPL-2.0, separate process)         │
│      └── JSON IPC over Unix socket                      │
├─────────────────────────────────────────────────────────┤
│  Android / Termux                                       │
│  ├── No root required (proot mode)                      │
│  ├── SELinux: untrusted_app domain                      │
│  └── No KVM access                                      │
└─────────────────────────────────────────────────────────┘
```

### 8.5 Priority Matrix

| Feature | Effort | Impact | Priority |
|:--------|:-------|:-------|:---------|
| Optimize proot mode | Low | High (all users) | **P0** |
| QEMU user-mode cross-arch | Medium | Medium (cross-arch users) | **P1** |
| QEMU system-mode VM backend | High | High (power users) | **P2** |
| Namespace mode (rooted) | Medium | Low (few rooted devices) | **P3** |
| AVF companion app | Very High | Medium (enterprise) | **P4** |
| crosvm backend | High | Low (complex, fragile) | **Not recommended** |

### 8.6 Final Recommendation

**Stick with proot as the primary Android runtime.** It's the right tradeoff: works everywhere, no root, acceptable performance. Add QEMU system-mode as an **optional opt-in** for users who need real isolation, but don't make it the default — the 3-5x CPU overhead and 200+ MB memory cost are too high for casual use.

The QEMU system-mode approach is **feasible** but should be positioned as a "power user" feature, not the primary experience. The pre-built kernel + rootfs approach (similar to what Lima/Colima does on macOS) is the most practical implementation path.

AVF/pKVM is the **future** of Android virtualization, but it's not accessible from Termux today. Monitor Google's progress on AVF and reconsider when/if they expose a CLI-accessible API.
