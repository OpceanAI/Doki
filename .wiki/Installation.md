# Installation

<sub>[PER-PLATFORM SETUP / v0.11.0]</sub>

> Doki ships as static binaries with zero runtime dependencies.
> Pick your platform, download, verify, install.

---

## Termux (Android)

```bash
# Install dependencies.
pkg install proot

# One-line installer.
curl -fsSL https://raw.githubusercontent.com/OpceanAI/Doki/main/install.sh | bash

# Or manual install.
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-android-arm64
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/dokid-android-arm64
chmod +x doki-android-arm64 dokid-android-arm64
mv doki-android-arm64 $PREFIX/bin/doki
mv dokid-android-arm64 $PREFIX/bin/dokid

# Verify.
doki doctor
```

On Termux, network isolation tools (passt, slirp4netns) are not
installable and would not work if manually placed in $PATH: they
require /dev/net/tun and CAP_NET_ADMIN, which Termux does not
provide. Containers use the host network namespace via proot, which
is functional for normal use but provides no network isolation. This
is expected behavior, not a degradation.

---

## Linux

<sub>[APT / DEBIAN + UBUNTU]</sub>

```bash
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-linux-amd64
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/dokid-linux-amd64
chmod +x doki-linux-amd64 dokid-linux-amd64
sudo mv doki-linux-amd64 /usr/local/bin/doki
sudo mv dokid-linux-amd64 /usr/local/bin/dokid
```

<sub>[PACMAN / ARCH]</sub>

```bash
# From AUR (community-maintained).
yay -S doki-bin
```

<sub>[DNF / FEDORA]</sub>

```bash
# RPM (community-maintained, check availability).
sudo dnf install https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-0.11.0-1.x86_64.rpm
```

---

## macOS

macOS is a client platform. `doki`, `doki-kube`, and `doki-kubectl`
work natively. `dokid` requires a Linux backend or remote daemon.

```bash
# via Homebrew (community-maintained, check availability).
brew install doki

# Or manual install.
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-darwin-arm64
chmod +x doki-darwin-arm64
sudo mv doki-darwin-arm64 /usr/local/bin/doki
```

---

## ARM SBCs (Raspberry Pi, Orange Pi, etc.)

```bash
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-linux-arm64
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/dokid-linux-arm64
chmod +x doki-linux-arm64 dokid-linux-arm64
sudo mv doki-linux-arm64 /usr/local/bin/doki
sudo mv dokid-linux-arm64 /usr/local/bin/dokid
```

---

## Build from Source

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
make build-release sha256
```

Requires Go 1.26+. For macOS VZ backend, build with
`CGO_ENABLED=1` on darwin.

---

## Binary Matrix

```
BINARY          ANDROID-ARM64  LINUX-ARM64  LINUX-AMD64  DARWIN-ARM64
─────────────────────────────────────────────────────────────────────
doki            yes            yes          yes          yes
dokid           yes            yes          yes          ---
doki-compose    yes            yes          yes          ---
doki-init       yes            yes          yes          ---
doki-kube       yes            yes          yes          yes
doki-kubectl    yes            yes          yes          yes
```

---

## Verify Checksums

```bash
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt --ignore-missing
```

---

## Post-Install

```bash
# Verify dependencies.
doki doctor

# Start daemon.
dokid &

# Run first container.
doki run --rm alpine echo "ok"
```
