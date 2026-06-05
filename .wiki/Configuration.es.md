# Configuración

Doki se configura vía un archivo JSON en `~/.doki/config.json` y variables de entorno. El daemon (`dokid`) lee la config al arrancar; el CLI (`doki`) la lee al establecer la conexión.

## Ubicación del archivo de configuración

| Plataforma | Path por defecto |
|:-----------|:----------------|
| Linux | `~/.doki/config.json` |
| macOS | `~/.doki/config.json` |
| Termux (Android) | `$PREFIX/etc/doki/config.json` o `~/.doki/config.json` |

Sobrescribe con el flag `--config PATH` o la env var `DOKI_CONFIG`.

## Esquema completo

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

## Referencia de campos

### Top-level

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `root` | string | específico de plataforma | Directorio raíz de datos |
| `socket_path` | string | específico de plataforma | Path del Unix socket |
| `pidfile` | string | específico de plataforma | Archivo PID (cuando se daemoniza) |
| `storage_driver` | string | auto-detectado | `overlay2`, `fuse-overlayfs`, `btrfs`, `zfs`, `vfs` |
| `default_network` | string | `bridge` | Red por defecto para nuevos contenedores |
| `default_runtime` | string | `auto` | Modo de aislamiento por defecto (o `auto`) |
| `rootless` | bool | auto-detectado por plataforma | Corre rootless (sin ops privilegiadas) |
| `debug` | bool | `false` | Habilita modo debug |
| `log_level` | string | `info` | `debug`, `info`, `warn`, `error` |
| `log_format` | string | `auto` | `auto`, `json`, `text` |
| `log_driver` | string | `json-file` | Driver de log del contenedor |
| `log_opts` | object | ver esquema | Opciones específicas del driver |
| `experimental` | bool | `false` | Habilita features experimentales |

### DNS

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `dns` | array | específico de plataforma | Servidores DNS upstream |
| `dns_listen` | string | `127.0.0.11:53` (Linux), `127.0.0.11:8053` (Android) | Dirección de escucha del servidor DNS interno |
| `dns_search` | array | `[]` | Dominios de búsqueda por defecto |
| `dns_opts` | array | `["ndots:0"]` | Opciones DNS por defecto |

**Nota v0.9.2**: En Android, `dns_listen` por defecto es `:8053` porque el puerto 53 está bloqueado por SELinux. Sobrescribe con la env var `DOKI_DNS_LISTEN`.

### Registries

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `registry_mirrors` | array | `[]` | Registries mirror a intentar antes que el principal |
| `insecure_registries` | array | `[]` | Registries a los que se les salta la verificación TLS |

**Ejemplo**: Acelera pulls en China añadiendo un mirror:

```json
{
  "registry_mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com"
  ]
}
```

**Ejemplo**: Permite un registry HTTP local:

```json
{
  "insecure_registries": ["registry.local:5000"]
}
```

### Network

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `network.bridge` | string | `doki0` | Nombre del bridge por defecto |
| `network.default_subnet` | string | `10.0.0.0/24` | Subnet por defecto para nuevos bridges |
| `network.mtu` | int | `1500` | MTU del bridge |
| `network.ipv6` | bool | `false` | Habilita IPv6 en el bridge por defecto |

### Cgroup

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `cgroup.version` | string | `v2` | Versión de cgroup |
| `cgroup.memory_limit` | string | `0` (ilimitado) | Límite global de memoria |
| `cgroup.cpu_shares` | int | `1024` | Shares de CPU por defecto |

### Seccomp

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `seccomp.profile` | string | `default` | Nombre del perfil (default, unconfined, path custom) |
| `seccomp.allow` | array | `[]` | Syscalls extra a permitir más allá del perfil por defecto |
| `seccomp.deny` | array | `[]` | Syscalls a denegar (override de allow) |

**Perfil default de v0.9.2** permite: ~80 syscalls incluyendo `io_uring_*`, `pidfd_*`, `rseq`, `userfaultfd`, `copy_file_range`, `landlock_*`.

**Denegados por defecto**: `init_module`, `finit_module`, `delete_module`, `kexec_load`, `kexec_file_load`, `iopl`, `ioperm`, `kcmp`, `process_vm_readv`, `process_vm_writev`.

Ver [Seguridad](Security.es) para la lista completa.

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

| Campo | Descripción |
|:------|:------------|
| `cert` | Certificado del servidor (PEM) |
| `key` | Clave privada del servidor (PEM) |
| `client_ca` | Bundle CA para mTLS (opcional) |
| `verify` | Requiere certs de cliente (mTLS) |

Con TLS habilitado, el daemon escucha en `tcp://0.0.0.0:2376` (o el puerto que sea). Configura `DOKI_HOST=tcp://host:2376` en el cliente.

### Rate Limiting

```json
{
  "rate_limit": {
    "rps": 100,
    "burst": 200
  }
}
```

Token-bucket: `rps` requests por segundo sostenidos, `burst` permitido en picos. Por IP.

### Metrics & Debug

| Campo | Tipo | Default | Descripción |
|:------|:-----|:--------|:------------|
| `metrics_addr` | string | `127.0.0.1:9090` | Dirección de escucha de métricas Prometheus |
| `debug_addr` | string | `127.0.0.1:6060` | Dirección de escucha de pprof debug (setea `DOKI_DEBUG=1` para habilitar) |

## Variables de entorno

Todos los campos de config pueden ser sobrescritos por env vars. Convención de naming: `DOKI_<UPPER_SNAKE_CASE>`.

| Env var | Campo de config | Ejemplo |
|:--------|:----------------|:--------|
| `DOKI_HOST` | (socket del daemon) | `unix:///var/run/doki.sock` o `tcp://host:2375` |
| `DOKI_CONFIG` | (path del archivo de config) | `/etc/doki/config.json` |
| `DOKI_DATA_DIR` | `root` | `/var/lib/doki` |
| `DOKI_STORAGE_DRIVER` | `storage_driver` | `overlay2` |
| `DOKI_DEFAULT_NETWORK` | `default_network` | `host` |
| `DOKI_DEFAULT_RUNTIME` | `default_runtime` | `proot` |
| `DOKI_ROOTLESS` | `rootless` | `1` para habilitar |
| `DOKI_DEBUG` | `debug` | `1` para habilitar, también activa pprof |
| `DOKI_LOG_LEVEL` | `log_level` | `debug`, `info`, `warn`, `error` |
| `DOKI_LOG_FORMAT` | `log_format` | `json`, `text`, `auto` |
| `DOKI_DNS` | `dns` (separado por comas) | `8.8.8.8,1.1.1.1` |
| `DOKI_DNS_LISTEN` | `dns_listen` | `127.0.0.11:8053` |
| `DOKI_DNS_SEARCH` | `dns_search` (separado por comas) | `local,internal` |
| `DOKI_DNS_OPTS` | `dns_opts` (separado por comas) | `ndots:0,timeout:3` |
| `DOKI_REGISTRY_MIRRORS` | `registry_mirrors` (separado por comas) | `https://mirror1,https://mirror2` |
| `DOKI_INSECURE_REGISTRIES` | `insecure_registries` (separado por comas) | `registry.local:5000` |
| `DOKI_TLS` | habilita TLS | `1` |
| `DOKI_TLS_CERT` | `tls.cert` | `/etc/doki/cert.pem` |
| `DOKI_TLS_KEY` | `tls.key` | `/etc/doki/key.pem` |
| `DOKI_TLS_CA` | `tls.client_ca` | `/etc/doki/ca.pem` |
| `DOKI_TLS_VERIFY` | `tls.verify` | `1` |
| `DOKI_METRICS_ADDR` | `metrics_addr` | `0.0.0.0:9090` |
| `DOKI_DEBUG_ADDR` | `debug_addr` | `0.0.0.0:6060` |
| `DOKI_RATE_LIMIT` | `rate_limit.rps` | `200` |
| `DOKI_EXPERIMENTAL` | `experimental` | `1` |
| `DOKI_NATIVE` | fuerza modo native (per-proceso) | `1` |
| `DOKI_KERNEL` | path del kernel microVM | `/usr/share/doki/vmlinux` |

## Defaults específicos por plataforma

El daemon auto-detecta la plataforma y rellena defaults razonables:

| Campo | Linux | Termux | macOS |
|:------|:------|:-------|:------|
| `root` | `~/.doki/data` | `$PREFIX/var/lib/doki` | `~/.doki/data` |
| `socket_path` | `/var/run/doki.sock` | `$PREFIX/var/run/doki.sock` | `~/.doki/doki.sock` |
| `storage_driver` | `overlay2` | `fuse-overlayfs` | `vfs` |
| `dns_listen` | `127.0.0.11:53` | `127.0.0.11:8053` | (ninguno) |
| `default_runtime` | `namespaces` | `proot` | `native` |

## Validación

Corre `doki config validate` para chequear tu config por errores:

```bash
$ doki config validate
INFO  config valid
$ doki config validate 2>&1 | head -10
WARN  dns_listen ":53" puede estar bloqueado en Android; usa ":8053"
WARN  insecure_registries contiene URLs HTTP; no recomendado para producción
```

## Migrando desde Docker

La mayoría de env vars de Docker se reconocen:

| Docker | Doki |
|:-------|:----|
| `DOCKER_HOST` | `DOKI_HOST` |
| `DOCKER_CONFIG` | `DOKI_CONFIG` |
| `DOCKER_TLS_VERIFY` | `DOKI_TLS_VERIFY` |
| `DOCKER_CERT_PATH` | directorio `DOKI_TLS_CA` |
| `DOCKER_BUILDKIT` | `DOKI_EXPERIMENTAL=1` |
| `DOCKER_RATE_LIMIT` | `DOKI_RATE_LIMIT` |

`doki` cae a env vars de Docker si las de Doki no están seteadas.

## Config programática

`pkg/common/config.go` expone el struct `Config`. Lo carga `LoadConfig(path)` que:

1. Lee el archivo JSON
2. Aplica los overrides de env vars
3. Mezcla los defaults de la plataforma
4. Devuelve un `*Config` validado

Para tests, usa `LoadConfigFromString(jsonString)`.

## Fuente

- `pkg/common/config.go` — esquema + loader
- `pkg/common/defaults.go` — defaults por plataforma
- `cmd/dokid/main.go` — env var → mapeo de config
