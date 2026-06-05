# Configuration

Doki is configured via a JSON file at `~/.doki/config.json` and environment variables. The daemon (`dokid`) reads the config on startup; the CLI (`doki`) reads it when establishing the connection.

## Config File Location

| Platform | Default path |
|:---------|:-------------|
| Linux | `~/.doki/config.json` |
| macOS | `~/.doki/config.json` |
| Termux (Android) | `$PREFIX/etc/doki/config.json` or `~/.doki/config.json` |

Override with `--config PATH` flag or `DOKI_CONFIG` env var.

## Full Schema

```json
{
  "root": "/home/user/.doki/data",
  "socket_path": "/var/run/doki.sock",
  "pidfile": "/var/run/dokid.pid",
  "storage_driver": "fuse-overlayfs",
  "default_network": "bridge",
  "default_runtime": "auto",
  "rootless": true,
  "debug": false,
  "log_level": "info",
  "log_format": "auto",
  "log_driver": "json-file",
  "log_opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "dns": ["8.8.8.8", "8.8.4.4"],
  "dns_listen": "127.0.0.11:53",
  "dns_search": [],
  "dns_opts": ["ndots:0"],
  "registry_mirrors": [
    "https://mirror.gcr.io"
  ],
  "insecure_registries": [
    "registry.local:5000"
  ],
  "experimental": false,
  "metrics_addr": "127.0.0.1:9090",
  "debug_addr": "127.0.0.1:6060",
  "rate_limit": {
    "rps": 100,
    "burst": 200
  },
  "tls": {
    "cert": "/etc/doki/cert.pem",
    "key": "/etc/doki/key.pem",
    "client_ca": "/etc/doki/ca.pem",
    "verify": true
  },
  "network": {
    "bridge": "doki0",
    "default_subnet": "10.0.0.0/24",
    "mtu": 1500,
    "ipv6": false
  },
  "cgroup": {
    "version": "v2",
    "memory_limit": "0",
    "cpu_shares": 1024
  },
  "seccomp": {
    "profile": "default",
    "allow": ["io_uring_setup", "pidfd_open"],
    "deny": ["init_module", "kexec_load"]
  },
  "apparmor": {
    "enabled": false,
    "profile_template": "doki-default"
  },
  "image": {
    "pull_policy": "missing",
    "default_platform": "auto",
    "content_trust": false
  }
}
```

## Field Reference

### Top-level

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `root` | string | platform-specific | Data root directory |
| `socket_path` | string | platform-specific | Unix socket path |
| `pidfile` | string | platform-specific | PID file (when daemonizing) |
| `storage_driver` | string | auto-detected | `overlay2`, `fuse-overlayfs`, `btrfs`, `zfs`, `vfs` |
| `default_network` | string | `bridge` | Default network for new containers |
| `default_runtime` | string | `auto` | Default isolation mode (or `auto`) |
| `rootless` | bool | platform-detected | Run rootless (no privileged ops) |
| `debug` | bool | `false` | Enable debug mode |
| `log_level` | string | `info` | `debug`, `info`, `warn`, `error` |
| `log_format` | string | `auto` | `auto`, `json`, `text` |
| `log_driver` | string | `json-file` | Container log driver |
| `log_opts` | object | see schema | Driver-specific options |
| `experimental` | bool | `false` | Enable experimental features |

### DNS

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `dns` | array | platform-specific | Upstream DNS servers |
| `dns_listen` | string | `127.0.0.11:53` (Linux), `127.0.0.11:8053` (Android) | Internal DNS server listen address |
| `dns_search` | array | `[]` | Default search domains |
| `dns_opts` | array | `["ndots:0"]` | Default DNS options |

**v0.9.2 note**: On Android, `dns_listen` defaults to `:8053` because port 53 is blocked by SELinux. Override with `DOKI_DNS_LISTEN` env var.

### Registries

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `registry_mirrors` | array | `[]` | Mirror registries to try before the main one |
| `insecure_registries` | array | `[]` | Registries to skip TLS verification for |

**Example**: Speed up pulls in China by adding a mirror:

```json
{
  "registry_mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com"
  ]
}
```

**Example**: Allow a local HTTP registry:

```json
{
  "insecure_registries": ["registry.local:5000"]
}
```

### Network

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `network.bridge` | string | `doki0` | Default bridge name |
| `network.default_subnet` | string | `10.0.0.0/24` | Default subnet for new bridges |
| `network.mtu` | int | `1500` | Bridge MTU |
| `network.ipv6` | bool | `false` | Enable IPv6 on default bridge |

### Cgroup

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `cgroup.version` | string | `v2` | cgroup version |
| `cgroup.memory_limit` | string | `0` (unlimited) | Global memory limit |
| `cgroup.cpu_shares` | int | `1024` | Default CPU shares |

### Seccomp

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `seccomp.profile` | string | `default` | Profile name (default, unconfined, custom path) |
| `seccomp.allow` | array | `[]` | Extra syscalls to allow beyond the default profile |
| `seccomp.deny` | array | `[]` | Syscalls to deny (overrides allow) |

**v0.9.2 default profile** allows: ~80 syscalls including `io_uring_*`, `pidfd_*`, `rseq`, `userfaultfd`, `copy_file_range`, `landlock_*`.

**Denied by default**: `init_module`, `finit_module`, `delete_module`, `kexec_load`, `kexec_file_load`, `iopl`, `ioperm`, `kcmp`, `process_vm_readv`, `process_vm_writev`.

See [Security](Security) for the full list.

### TLS

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

| Field | Description |
|:------|:------------|
| `cert` | Server certificate (PEM) |
| `key` | Server private key (PEM) |
| `client_ca` | CA bundle for mTLS (optional) |
| `verify` | Require client certs (mTLS) |

With TLS enabled, the daemon listens on `tcp://0.0.0.0:2376` (or whatever port). Set `DOKI_HOST=tcp://host:2376` on the client.

### Rate Limiting

```json
{
  "rate_limit": {
    "rps": 100,
    "burst": 200
  }
}
```

Token-bucket: `rps` requests per second sustained, `burst` allowed in spikes. Per-IP.

### Metrics & Debug

| Field | Type | Default | Description |
|:------|:-----|:--------|:------------|
| `metrics_addr` | string | `127.0.0.1:9090` | Prometheus metrics listen address |
| `debug_addr` | string | `127.0.0.1:6060` | pprof debug listen address (set `DOKI_DEBUG=1` to enable) |

## Environment Variables

All config fields can be overridden by env vars. Naming convention: `DOKI_<UPPER_SNAKE_CASE>`.

| Env var | Config field | Example |
|:--------|:-------------|:--------|
| `DOKI_HOST` | (daemon socket) | `unix:///var/run/doki.sock` or `tcp://host:2375` |
| `DOKI_CONFIG` | (config file path) | `/etc/doki/config.json` |
| `DOKI_DATA_DIR` | `root` | `/var/lib/doki` |
| `DOKI_STORAGE_DRIVER` | `storage_driver` | `overlay2` |
| `DOKI_DEFAULT_NETWORK` | `default_network` | `host` |
| `DOKI_DEFAULT_RUNTIME` | `default_runtime` | `proot` |
| `DOKI_ROOTLESS` | `rootless` | `1` to enable |
| `DOKI_DEBUG` | `debug` | `1` to enable, also turns on pprof |
| `DOKI_LOG_LEVEL` | `log_level` | `debug`, `info`, `warn`, `error` |
| `DOKI_LOG_FORMAT` | `log_format` | `json`, `text`, `auto` |
| `DOKI_DNS` | `dns` (comma-separated) | `8.8.8.8,1.1.1.1` |
| `DOKI_DNS_LISTEN` | `dns_listen` | `127.0.0.11:8053` |
| `DOKI_DNS_SEARCH` | `dns_search` (comma-separated) | `local,internal` |
| `DOKI_DNS_OPTS` | `dns_opts` (comma-separated) | `ndots:0,timeout:3` |
| `DOKI_REGISTRY_MIRRORS` | `registry_mirrors` (comma-separated) | `https://mirror1,https://mirror2` |
| `DOKI_INSECURE_REGISTRIES` | `insecure_registries` (comma-separated) | `registry.local:5000` |
| `DOKI_TLS` | enables TLS | `1` |
| `DOKI_TLS_CERT` | `tls.cert` | `/etc/doki/cert.pem` |
| `DOKI_TLS_KEY` | `tls.key` | `/etc/doki/key.pem` |
| `DOKI_TLS_CA` | `tls.client_ca` | `/etc/doki/ca.pem` |
| `DOKI_TLS_VERIFY` | `tls.verify` | `1` |
| `DOKI_METRICS_ADDR` | `metrics_addr` | `0.0.0.0:9090` |
| `DOKI_DEBUG_ADDR` | `debug_addr` | `0.0.0.0:6060` |
| `DOKI_RATE_LIMIT` | `rate_limit.rps` | `200` |
| `DOKI_EXPERIMENTAL` | `experimental` | `1` |
| `DOKI_NATIVE` | force native mode (per-process) | `1` |
| `DOKI_KERNEL` | microVM kernel path | `/usr/share/doki/vmlinux` |

## Platform-Specific Defaults

The daemon auto-detects the platform and fills in sensible defaults:

| Field | Linux | Termux | macOS |
|:------|:------|:-------|:------|
| `root` | `~/.doki/data` | `$PREFIX/var/lib/doki` | `~/.doki/data` |
| `socket_path` | `/var/run/doki.sock` | `$PREFIX/var/run/doki.sock` | `~/.doki/doki.sock` |
| `storage_driver` | `overlay2` | `fuse-overlayfs` | `vfs` |
| `dns_listen` | `127.0.0.11:53` | `127.0.0.11:8053` | (none) |
| `default_runtime` | `namespaces` | `proot` | `native` |

## Validation

Run `doki config validate` to check your config for errors:

```bash
$ doki config validate
INFO  config valid
$ doki config validate 2>&1 | head -10
WARN  dns_listen ":53" may be blocked on Android; use ":8053"
WARN  insecure_registries contains HTTP URLs; not recommended for production
```

## Migrating from Docker

Most Docker env vars are recognized:

| Docker | Doki |
|:-------|:----|
| `DOCKER_HOST` | `DOKI_HOST` |
| `DOCKER_CONFIG` | `DOKI_CONFIG` |
| `DOCKER_TLS_VERIFY` | `DOKI_TLS_VERIFY` |
| `DOCKER_CERT_PATH` | `DOKI_TLS_CA` directory |
| `DOCKER_BUILDKIT` | `DOKI_EXPERIMENTAL=1` |
| `DOCKER_RATE_LIMIT` | `DOKI_RATE_LIMIT` |

`doki` falls back to Docker env vars if Doki ones aren't set.

## Programmatic Config

`pkg/common/config.go` exposes the `Config` struct. It's loaded by `LoadConfig(path)` which:

1. Reads the JSON file
2. Applies env var overrides
3. Merges platform defaults
4. Returns a validated `*Config`

For tests, use `LoadConfigFromString(jsonString)`.

## Source

- `pkg/common/config.go` — schema + loader
- `pkg/common/defaults.go` — platform defaults
- `cmd/dokid/main.go` — env var → config mapping
