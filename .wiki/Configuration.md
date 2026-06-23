# Configuration

<sub>[SPEC]</sub> `doki v0.11.0`

![schema](https://img.shields.io/badge/schema-json?style=flat-square&color=24292e) ![daemon](https://img.shields.io/badge/daemon-dokid?style=flat-square&color=24292e) ![cli](https://img.shields.io/badge/cli-doki?style=flat-square&color=24292e)

Doki reads configuration from a JSON file at `~/.doki/config.json` followed by environment variable overrides. The daemon (`dokid`) parses the file at startup. The CLI (`doki`) reads it during socket handshake.

```
                 +----------+      JSON file       +----------+
  env vars --->  | dokid    | <--- ~/.doki/        |  disk    |
  overrides ---> | (daemon) |     config.json      |          |
                 +----+-----+                     +----------+
                      |
                      v
                 +----------+
                 | unix     |
                 | socket   |
                 +----+-----+
                      |
                      v
                 +----------+
                 | doki     |
                 | (cli)    |
                 +----------+
```

<hr>

## Config file location

<sub>[PATHS]</sub>

Override precedence: <kbd>--config PATH</kbd> flag > <kbd>DOKI_CONFIG</kbd> env var > default path.

```
PLATFORM             DEFAULT PATH
-----------------    -------------------------------------
Linux                ~/.doki/config.json
macOS                ~/.doki/config.json
Termux (Android)     $PREFIX/etc/doki/config.json
                     (fallback: ~/.doki/config.json)
```

<hr>

## Full schema

<sub>[JSON]</sub>

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

<hr>

## Field reference

<sub>[TOP-LEVEL]</sub>

```
FIELD              TYPE    DEFAULT              DESCRIPTION
----------------   ------  ------------------   --------------------------------------
root               string  platform-specific    data root directory
socket_path        string  platform-specific    unix socket path
pidfile            string  platform-specific    PID file (when daemonizing)
storage_driver     string  auto-detected        overlay2|fuse-overlayfs|btrfs|zfs|vfs
default_network    string  bridge               default network for new containers
default_runtime    string  auto                 default isolation mode (or auto)
rootless           bool    platform-detected    run rootless (no privileged ops)
debug              bool    false                enable debug mode
log_level          string  info                 debug|info|warn|error
log_format         string  auto                 auto|json|text
log_driver         string  json-file            container log driver
log_opts           object  see schema           driver-specific options
experimental       bool    false                enable experimental features
```

<hr>

## DNS

<sub>[RESOLVER]</sub>

```
FIELD        TYPE    DEFAULT                                  DESCRIPTION
-----------  ------  ---------------------------------------  -------------------------
dns          array   platform-specific                        upstream DNS servers
dns_listen   string  127.0.0.11:53 (Linux)                   internal DNS listen addr
                     127.0.0.11:8053 (Android)
dns_search   array   []                                       default search domains
dns_opts     array   ["ndots:0"]                              default DNS options
```

v0.11.0 note: on Android, `dns_listen` defaults to `:8053` because port 53 is blocked by SELinux. Override with <kbd>DOKI_DNS_LISTEN</kbd>.

<hr>

## Registries

<sub>[AUTH]</sub>

```
FIELD                TYPE    DEFAULT   DESCRIPTION
-----------------    ------  --------  -----------------------------------------------
registry_mirrors     array   []        mirror registries tried before the main one
insecure_registries  array   []        registries whose TLS verification is skipped
```

Example — accelerate pulls in China by adding mirrors:

```json
{
  "registry_mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com"
  ]
}
```

Example — allow a local HTTP registry:

```json
{
  "insecure_registries": ["registry.local:5000"]
}
```

<hr>

## Network

<sub>[NETHW]</sub>

```
FIELD                   TYPE    DEFAULT        DESCRIPTION
---------------------   ------  ------------   --------------------------------
network.bridge          string  doki0          default bridge name
network.default_subnet  string  10.0.0.0/24    default subnet for new bridges
network.mtu             int     1500           bridge MTU
network.ipv6            bool    false          enable IPv6 on default bridge
```

<hr>

## Cgroup

<sub>[CGROUP]</sub>

```
FIELD                  TYPE    DEFAULT          DESCRIPTION
-------------------    ------  ---------------  --------------------------------
cgroup.version         string  v2               cgroup version
cgroup.memory_limit    string  0 (unlimited)    global memory limit
cgroup.cpu_shares      int     1024             default CPU shares
```

<hr>

## Seccomp

<sub>[SECCOMP]</sub>

```
FIELD              TYPE    DEFAULT   DESCRIPTION
---------------    ------  --------  -----------------------------------------------
seccomp.profile    string  default   default|unconfined|custom path
seccomp.allow      array   []        syscalls allowed beyond the default profile
seccomp.deny       array   []        syscalls denied (overrides allow)
```

v0.11.0 default profile permits approximately 80 syscalls including `io_uring_*`, `pidfd_*`, `rseq`, `userfaultfd`, `copy_file_range`, `landlock_*`.

Denied by default: `init_module`, `finit_module`, `delete_module`, `kexec_load`, `kexec_file_load`, `iopl`, `ioperm`, `kcmp`, `process_vm_readv`, `process_vm_writev`.

See [Security](Security) for the full list.

<hr>

## TLS

<sub>[MTLS]</sub>

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

```
FIELD        DESCRIPTION
-----------  --------------------------------------
cert         server certificate (PEM)
key          server private key (PEM)
client_ca    CA bundle for mTLS (optional)
verify       require client certs (mTLS)
```

With TLS enabled, the daemon listens on `tcp://0.0.0.0:2376` (or whatever port is configured). On the client side, set <kbd>DOKI_HOST=tcp://host:2376</kbd>.

<hr>

## Rate limiting

<sub>[TOKEN-BUCKET]</sub>

```json
{
  "rate_limit": {
    "rps": 100,
    "burst": 200
  }
}
```

Token-bucket algorithm: `rps` requests per second sustained, `burst` permitted during spikes. Applied per source IP.

<hr>

## Metrics & debug

<sub>[OBSERVABILITY]</sub>

```
FIELD          TYPE    DEFAULT          DESCRIPTION
-----------    ------  ---------------  -----------------------------------------------
metrics_addr   string  127.0.0.1:9090   Prometheus metrics listen address
debug_addr     string  127.0.0.1:6060   pprof debug listen addr (set DOKI_DEBUG=1)
```

<hr>

## Environment variables

<sub>[ENV]</sub>

Every config field can be overridden by an environment variable. Naming convention: `DOKI_<UPPER_SNAKE_CASE>`.

```
ENV VAR                   FIELD                       EXAMPLE
-----------------------   -------------------------   ---------------------------------------
DOKI_HOST                 (daemon socket)             unix:///var/run/doki.sock
                                                     tcp://host:2375
DOKI_CONFIG               (config file path)          /etc/doki/config.json
DOKI_DATA_DIR             root                        /var/lib/doki
DOKI_STORAGE_DRIVER       storage_driver              overlay2
DOKI_DEFAULT_NETWORK      default_network             host
DOKI_DEFAULT_RUNTIME      default_runtime             proot
DOKI_ROOTLESS             rootless                    1 to enable
DOKI_DEBUG                debug                       1 (also enables pprof)
DOKI_LOG_LEVEL            log_level                   debug|info|warn|error
DOKI_LOG_FORMAT           log_format                  json|text|auto
DOKI_DNS                  dns (comma-separated)       8.8.8.8,1.1.1.1
DOKI_DNS_LISTEN           dns_listen                  127.0.0.11:8053
DOKI_DNS_SEARCH           dns_search (comma-sep)      local,internal
DOKI_DNS_OPTS             dns_opts (comma-separated)  ndots:0,timeout:3
DOKI_REGISTRY_MIRRORS     registry_mirrors (comma)    https://mirror1,https://mirror2
DOKI_INSECURE_REGISTRIES  insecure_registries (comma) registry.local:5000
DOKI_TLS                  enables TLS                 1
DOKI_TLS_CERT             tls.cert                    /etc/doki/cert.pem
DOKI_TLS_KEY              tls.key                     /etc/doki/key.pem
DOKI_TLS_CA               tls.client_ca               /etc/doki/ca.pem
DOKI_TLS_VERIFY           tls.verify                  1
DOKI_METRICS_ADDR         metrics_addr                0.0.0.0:9090
DOKI_DEBUG_ADDR           debug_addr                  0.0.0.0:6060
DOKI_RATE_LIMIT           rate_limit.rps              200
DOKI_EXPERIMENTAL         experimental                1
DOKI_NATIVE               force native (per-process)  1
DOKI_KERNEL               microVM kernel path         /usr/share/doki/vmlinux
DOKI_LINK_STUN            link STUN server URL        stun:stun.example.org:3478
DOKI_LINK_RELAY           link relay server URL       relay:relay.example.org:3478
DOKI_LINK_PAYLOAD_ENC     enable payload encryption   1
```

<hr>

## Platform-specific defaults

<sub>[AUTO-DETECT]</sub>

The daemon auto-detects the platform and populates defaults:

```
FIELD              LINUX                TERMUX                       MACOS
---------------    -----------------    -------------------------    -----------------
root               ~/.doki/data          $PREFIX/var/lib/doki         ~/.doki/data
socket_path        /var/run/doki.sock    $PREFIX/var/run/doki.sock    ~/.doki/doki.sock
storage_driver     overlay2              fuse-overlayfs               vfs
dns_listen         127.0.0.11:53         127.0.0.11:8053             (none)
default_runtime    namespaces            proot                        native
```

<hr>

## Validation

<sub>[CHECK]</sub>

Run `doki config validate` to verify the config file for errors:

```bash
$ doki config validate
INFO  config valid
$ doki config validate 2>&1 | head -10
WARN  dns_listen ":53" may be blocked on Android; use ":8053"
WARN  insecure_registries contains HTTP URLs; not recommended for production
```

<hr>

## Migrating from Docker

<sub>[COMPAT]</sub>

Most Docker env vars are recognized:

```
DOCKER               DOKI
-----------------    ---------------------
DOCKER_HOST          DOKI_HOST
DOCKER_CONFIG        DOKI_CONFIG
DOCKER_TLS_VERIFY    DOKI_TLS_VERIFY
DOCKER_CERT_PATH     DOKI_TLS_CA directory
DOCKER_BUILDKIT      DOKI_EXPERIMENTAL=1
DOCKER_RATE_LIMIT    DOKI_RATE_LIMIT
```

`doki` falls back to Docker env vars when the Doki equivalents are not set.

<hr>

## Programmatic config

<sub>[SOURCE]</sub>

`pkg/common/config.go` exposes the `Config` struct. It is loaded by `LoadConfig(path)`:

```
1.  read JSON file
2.  apply env var overrides
3.  merge platform defaults
4.  return validated *Config
       |
       v
  +-----------+
  | *Config   |
  +-----------+
```

For tests, use `LoadConfigFromString(jsonString)`.

<hr>

## Source

```
pkg/common/config.go     schema + loader
pkg/common/defaults.go   platform defaults
cmd/dokid/main.go        env var -> config mapping
```