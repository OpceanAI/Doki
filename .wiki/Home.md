# Doki Wiki

Welcome to the Doki (Universal Container Engine) documentation wiki. This wiki is the in-depth companion to the [README](../README.md) — start there for the elevator pitch, then come here for the details.

> **Language**: This wiki is available in English (`Page.md`) and Spanish (`Page.es.md`). If a Spanish version doesn't exist for a page, please open an issue.

## Latest Release

**v0.10.0** (June 2026) — Podman API v5, Kubernetes 1.32, macOS native VZ/QEMU, doki-OS VM, Landlock LSM, 55K LOC

- [Release notes](../RELEASE_NOTES.md)
- [Download binaries](https://github.com/OpceanAI/Doki/releases/tag/v0.10.0)
- [What's new in v0.10.0](../README.md#whats-new)

## Table of Contents

### Getting Started

| Page | What it covers |
|:-----|:---------------|
| [Installation](Installation) | Per-platform install: Termux, Linux (apt/dnf/pacman/portage), macOS, Android NDK, WSL2, Chromebook, Raspberry Pi, postmarketOS |
| [Quick Start](Quick-Start) | 5-minute tutorial: install → daemon → pull → run → compose → logs → cleanup |

### Concepts

| Page | What it covers |
|:-----|:---------------|
| [Architecture](Architecture) | Daemon internals, pipeline, OCI compliance, registry client, image layer cache |
| [Isolation Levels](Isolation-Levels) | Detailed coverage of all 12 modes: WASM, pKVM, microVM, sysbox, namespaces, gVisor, FEX, QEMU, proot, legacy32, chroot, native |
| [Security](Security) | Seccomp profile, AppArmor, capabilities, user namespaces, TLS, threat model |

### Reference

| Page | What it covers |
|:-----|:---------------|
| [CLI Reference](CLI-Reference) | All 108 Docker commands, 39 Podman endpoints, doki-kube and doki-kubectl |
| [Configuration](Configuration) | `config.json` schema, env vars, socket paths per OS, DNS, registries, log levels |
| [Networking](Networking) | Bridge, CNI plugins, port mapping, DNS, iptables (DNAT/SNAT), rootless (pasta), IPv6, DokiLink mesh |
| [Storage](Storage) | 5 drivers, VFS, overlay2 kernel requirements, btrfs/zfs, rootless FUSE, content-addressable store |

## Repository Layout

The wiki mirrors the source tree in `pkg/`, `internal/`, and `cmd/`. If you want to understand the code, read [Architecture](Architecture) first, then dive into specific packages.

## Contributing to the Wiki

The wiki source is stored in `.wiki/` at the repo root. To add a page:

1. Create `Your-Page.md` in `.wiki/`
2. Optionally add `Your-Page.es.md` for the Spanish version
3. Add a link from [Home.md](Home) ToC
4. Commit + push to `main`
5. The CI workflow `.github/workflows/wiki-sync.yml` pushes the page to GitHub Wiki, GitLab Wiki branch, and Codeberg Wiki

Wiki pages use [GitHub Flavored Markdown](https://github.github.com/gfm/) with anchors in kebab-case (`#isolation-levels`). Code blocks should be tagged (` ```bash `, ` ```yaml `, ` ```dockerfile `). Tables for comparative content; [Mermaid](https://mermaid.js.org/) blocks for diagrams (rendered natively on GitHub, GitLab, and Codeberg).

## Mirrors

This wiki is synced to three forges. All edits should go to GitHub (`OpceanAI/Doki`), which is the source of truth:

- **GitHub**: [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki) (primary)
- **GitLab**: [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home) (mirror, `wiki` branch)
- **Codeberg**: [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki) (mirror, separate repo)

If you spot a divergence between mirrors, open an issue on GitHub.
