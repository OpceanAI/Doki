# Installation

Doki ships as 4 binaries (`doki`, `dokid`, `doki-compose`, `doki-init`) for 4 platform/architecture combinations. Pick your platform below.

## Quick Install (Linux/macOS/Android via Termux)

```bash
curl -sL https://doki.opceanai.com | sh
```

This downloads the right binary for your platform to `~/.local/bin/`. Add it to your `$PATH` if it's not already.

## Termux (Android)

Termux is the primary supported Android environment. Doki runs rootless on Termux, no root required.

### From F-Droid (recommended)

1. Install [Termux from F-Droid](https://f-droid.org/packages/com.termux/) (NOT Google Play — the Play version is outdated)
2. Open Termux and run:

```bash
pkg update && pkg upgrade
pkg install proot curl
curl -sL https://doki.opceanai.com | sh
```

### From GitHub Releases

```bash
pkg install proot curl
mkdir -p $PREFIX/bin
curl -L -o $PREFIX/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-android-arm64
curl -L -o $PREFIX/bin/dokid https://github.com/OpceanAI/Doki/releases/latest/download/dokid-android-arm64
curl -L -o $PREFIX/bin/doki-compose https://github.com/OpceanAI/Doki/releases/latest/download/doki-compose-android-arm64
curl -L -o $PREFIX/bin/doki-init https://github.com/OpceanAI/Doki/releases/latest/download/doki-init-android-arm64
chmod +x $PREFIX/bin/doki*
```

### Verifying

```bash
$ doki version
Client: Doki
 Version:    0.9.3
 API version: 1.48
 GitCommit:  faab400
 Built:      2026-06-08

$ doki run --rm alpine echo "hello from doki"
hello from doki
```

### Termux-specific notes

- `LD_PRELOAD` and `LD_LIBRARY_PATH` are stripped from proot's environment automatically (v0.9.2+)
- DNS listens on `127.0.0.11:8053` (port 53 is blocked by SELinux without root)
- The default runtime is proot; override with `doki run --runtime native`
- Storage driver: `fuse-overlayfs` (no root needed)
- ARMv7 (32-bit) devices: use the `android-armv7` binaries (built with `GOOS=linux`, runs via proot)

## Linux

### Debian / Ubuntu

```bash
sudo apt update
sudo apt install -y curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Fedora / RHEL / Rocky

```bash
sudo dnf install -y curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Arch / Manjaro

```bash
sudo pacman -Syu curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Alpine

```bash
sudo apk add curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Gentoo

```bash
sudo emerge -av sys-fs/fuse-overlayfs net-firewall/iptables net-misc/curl
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Linux-specific notes

- Root mode requires `iptables` and `kmod` (for `modprobe overlay`)
- Rootless mode uses `fuse-overlayfs` (install from your package manager) and `pasta` (download from [passt](https://passt.top/) or use Doki's bundled binary)
- The `doki-init` binary is the PID 1 for microVM guests; you don't need it for normal containers
- For ARMv7 (32-bit ARM) devices, use the `android-armv7` binaries (Termux) or `linux-armv7` binaries (Raspberry Pi, postmarketOS)

## macOS

Doki ships a CLI-only `doki` binary for macOS Apple Silicon (arm64). The daemon and other binaries are Linux-only because they depend on `internal/namespaces` and overlayfs mounts that don't exist on Darwin.

### Homebrew (planned)

A Homebrew formula is in the works. For now, install manually.

### Manual install

```bash
curl -L -o /usr/local/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-darwin-arm64
chmod +x /usr/local/bin/doki
```

### macOS-specific notes

- The CLI runs in `ModeNative` only — no isolation, no bridge network
- To use the daemon, run `dokid` on a Linux server and point your local `doki` at it via `DOKI_HOST=tcp://server:2375`
- macOS is not a build target for `dokid`/`doki-compose`/`doki-init` — those will fail to build with darwin GOOS

## Windows / WSL2

Native Windows is not supported. Use WSL2 with the Ubuntu install instructions above.

```powershell
wsl --install
wsl --set-default-version 2
# Then follow the Debian/Ubuntu install steps inside WSL
```

## Chromebook (ChromeOS Linux container)

ChromeOS's Linux (Beta) container is essentially a Debian VM. The Doki CLI runs inside it; for container-in-container scenarios use the `proot` runtime.

```bash
sudo apt update
sudo apt install -y curl fuse-overlayfs proot
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

For pKVM isolation on Chromebooks with the right firmware, Doki will auto-detect and use it (level 11 in the [Isolation Levels](Isolation-Levels) page).

## Raspberry Pi / ARM boards

Use the `linux-armv7` (32-bit Raspbian) or `linux-arm64` (Raspberry Pi OS 64-bit, Ubuntu ARM64) binary.

```bash
# Detect your arch
uname -m
# aarch64 -> arm64, armv7l -> armv7

# For 64-bit Pi OS
curl -L -o /usr/local/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64
chmod +x /usr/local/bin/doki

# For 32-bit Raspbian
curl -L -o /usr/local/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-armv7
chmod +x /usr/local/bin/doki
```

Enable cgroups v2 in `/boot/firmware/cmdline.txt` by appending:

```
cgroup_memory=1 cgroup_enable=memory
```

Then reboot.

## postmarketOS / PinePhone / Librem

Mobile Linux distros based on Alpine or Arch. Use the Alpine or Arch install steps. Proot is the default runtime.

## Building from Source

See [Building from source](#building-from-source) below or the README's [Building section](../README.md#building).

## Verifying Downloads

Each binary has a `.sha256` file:

```bash
$ curl -L -O https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.sha256
$ sha256sum -c doki-linux-arm64.sha256
doki-linux-arm64: OK
```

## Troubleshooting

| Symptom | Fix |
|:--------|:----|
| `command not found: doki` | Add `$PREFIX/bin` (Termux) or `/usr/local/bin` (Linux) to `$PATH` |
| `execve: Function not implemented` (Termux) | Fixed in v0.9.2+; update to latest release |
| `port 53: permission denied` (Termux) | This is expected; Doki uses port 8053 by default on Android |
| `requires external cgo linking` (armv7) | Fixed in v0.9.3; armv7 builds use `GOOS=linux` workaround |
| `iptables: Unknown option` | Update to v0.9.2+; the DNAT bug was fixed |
| `cannot find proot` | `apt install proot` (Debian/Ubuntu) or `pkg install proot` (Termux) |

## Next Steps

- Continue to [Quick Start](Quick-Start) for a 5-minute tutorial
- Read [Architecture](Architecture) to understand the daemon
- Pick the right [Isolation Level](Isolation-Levels) for your workload
