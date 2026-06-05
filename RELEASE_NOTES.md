# Doki v0.9.2 — DNS Overhaul, LD_PRELOAD Fix, Android Native Support, 12 Isolation Levels

## Breaking Changes

- **DNS default port on Android**: Changed from `:53` to `127.0.0.11:8053` — port 53 is blocked on non-rooted Android. Set `DOKI_DNS_LISTEN` to override.

## What's New

### 12 Isolation Levels
- New runner registry with auto-selection from 12 modes: WASM, pKVM/Microdroid, MicroVM, Sysbox, Namespaces, gVisor, FEX-Emu, QEMU User, Proot, Legacy32, Chroot, and Native
- Each mode has a specific use case: WASM for untrusted sandboxes, pKVM for Android protected VMs, FEX/QEMU for cross-architecture emulation, Sysbox for Docker-in-Docker, gVisor for defense-in-depth, Legacy32 for ARMv7 compat, Chroot for lightweight isolation
- `doki run --runtime <mode>` for explicit selection; auto-detection picks the best available

### Android Native Mode
- Android detection via `/system/bin/` and `ro.build.version.release` — proot is forced automatically
- `LD_PRELOAD` and `LD_LIBRARY_PATH` stripped from proot environment via `common.StripHostEnv()` — fixes Termux's `libtermux-exec-ld-preload.so` hooking `execve` and breaking proot's ptrace
- Android DNS auto-discovery via `getprop net.dns1` through `net.dns4`
- proot forced when available (`detectMode()`), with `--link2symlink` for better compat
- 16KB page size alignment for Android 15+ (`-Wl,-z,max-page-size=16384`)

### DNS System Rewrite (18 Bugs Fixed)
- **Port stripping**: `ParseResolvConf` now stores clean IPs (no port); `NameserverList()` appends `:53` for dialling; `GenerateResolvConf` strips port with `net.SplitHostPort()`
- **DNS registration in SetupNetwork**: creates endpoint + registers DNS if missing (no prior `Connect()` call needed)
- **DNS recovery on restart**: `recoverContainers` calls `netMgr.ReRegisterDNS(st.ID)` for each recovered container
- **Dynamic DNS Well-Known Address**: generated per-container instead of hardcoded
- **Default port 8053 on Android**: set via `init()` in `cmd/dokid/main.go`
- **iptables DNAT for root mode**: redirects DNS traffic on privileged setups
- **AAAA + PTR local resolution**: handles IPv6 and reverse DNS locally
- **ndots:0**: `GenerateResolvConf` adds `options ndots:0` by default
- **TCP retry upstream**: DNS queries that fail over UDP are retried over TCP
- **No busy-wait polling**: fixed `resolv.conf` watching to use inotify instead

### Cross-Platform Releases
- Makefile rewritten: all targets output to `releases/` (not `bin/`)
- Supported platforms: `android-arm64`, `android-armv7`, `linux-arm64`, `linux-armv7`, `darwin-arm64`
- SHA256 checksums generated for all binaries
- `make release` builds everything + checksums in one command

### Other
- Dead parameter removed: `NewServer()` no longer accepts unused `*network.DNSServer` variadic
- Readme updated with v0.9.2 changelog and `DOKI_DNS_LISTEN` env var documentation
- `.golangci.yml` linting configuration added

## Documentation

- Replaced all ASCII-art diagrams in `README.md`, `README.es.md`, and the 10-page wiki (EN+ES) with native Mermaid blocks. Renders on GitHub, GitLab, and Codeberg without plugins
- Added Spanish translation of the full README (1:1 structure, 1212 lines)
- Wiki source lives in `.wiki/` at the repo root and is auto-synced to `OpceanAI/doki-wiki` (GitHub) and the `wiki` branch of `aguitauwu/doki` (GitLab) by `.github/workflows/wiki-sync.yml`
- 6 distinct diagrams refreshed: isolation decision tree, DNS architecture, daemon subsystem map, defense-in-depth stack, storage directory tree, middleware pipeline

## Installation

Download the appropriate binary for your platform from the release assets, or build from source:

```bash
git clone https://github.com/OpceanAI/Doki
cd Doki
make release
```
