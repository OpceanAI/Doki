# Doki Wiki

<sub>[DOCUMENTATION INDEX / v0.11.0]</sub>

> Companion documentation for the Doki rootless container runtime.
> Start at the [README](../README.md) for the project definition, then
> use this wiki for subsystem detail, installation procedures, and
> CLI reference.

This wiki is available in English (`Page.md`) and Spanish (`Page.es.md`).

---

## Getting Started

- [Installation](Installation) -- Per-platform setup: Termux, Linux, macOS, Raspberry Pi, WSL2, Chromebook
- [Quick Start](Quick-Start) -- 5-minute walkthrough: install, daemon, pull, run, compose, cleanup

## Concepts

- [Architecture](Architecture) -- Daemon internals, pipeline stages, OCI compliance, layer cache
- [Isolation Levels](Isolation-Levels) -- All 12 runner modes: proot, native, gVisor, microVM, wasm, pKVM, others
- [Security](Security) -- Seccomp, AppArmor, capabilities, user namespaces, TLS, threat model

## Reference

- [CLI Reference](CLI-Reference) -- Full command catalog: Docker, Podman, Compose, Kubernetes, Mesh, Deps
- [Configuration](Configuration) -- config.json schema, environment variables, socket paths per OS
- [Networking](Networking) -- Bridge, CNI, port mapping, DNS, iptables, rootless fallback, DokiLink mesh
- [Storage](Storage) -- 5 drivers, VFS, overlay2, btrfs/zfs, rootless FUSE, content-addressable store

---

## Repository Layout

The wiki mirrors the source tree in `pkg/`, `internal/`, and `cmd/`.
Read [Architecture](Architecture) first, then dive into specific packages.

## Contributing to the Wiki

Wiki source lives in `.wiki/` at the repo root. To add a page:

1. Create `Your-Page.md` in `.wiki/`
2. Optionally add `Your-Page.es.md` for the Spanish version
3. Add a link from this page
4. Commit and push to `main`
5. The CI workflow syncs to GitHub Wiki, GitLab Wiki, and Codeberg Wiki

Pages use GitHub Flavored Markdown with tagged code blocks.

## Mirrors

All edits go to GitHub (`OpceanAI/Doki`), the source of truth:

- GitHub: [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki)
- GitLab: [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home)
- Codeberg: [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki)
