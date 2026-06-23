# Configuración

<sub>[SPEC]</sub> `doki v0.11.0`

![schema](https://img.shields.io/badge/schema-json?style=flat-square&color=24292e) ![daemon](https://img.shields.io/badge/daemon-dokid?style=flat-square&color=24292e) ![cli](https://img.shields.io/badge/cli-doki?style=flat-square&color=24292e)

Doki lee la configuración de un archivo JSON en `~/.doki/config.json` seguido de overrides por variables de entorno. El daemon (`dokid`) parsea el archivo al arrancar. El CLI (`doki`) lo lee durante el handshake del socket.

```
                 +----------+      archivo JSON     +----------+
  env vars --->  | dokid    | <--- ~/.doki/        |  disco   |
  overrides ---> | (daemon) |     config.json      |          |
                 +----+-----+                      +----------+
                      |
                      v
                 +----------+
                 | socket    |
                 | unix      |
                 +----+-----+
                      |
                      v
                 +----------+
                 | doki      |
                 | (cli)     |
                 +----------+
```

<hr>

## Ubicación del archivo de configuración

<sub>[PATHS]</sub>

Precedencia de override: flag <kbd>--config PATH</kbd> > env var <kbd>DOKI_CONFIG</kbd> > path por defecto.

```
PLATAFORMA           PATH POR DEFECTO
-----------------    -------------------------------------
Linux                ~/.doki/config.json
macOS                ~/.doki/config.json
Termux (Android)     $PREFIX/etc/doki/config.json
                     (fallback: ~/.doki/config.json)
```

<hr>

## Esquema completo

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

## Referencia de campos

<sub>[TOP-LEVEL]</sub>

```
FIELD              TYPE    DEFAULT              DESCRIPTION
----------------   ------  ------------------   --------------------------------------
root               string  específico de pl.   directorio raíz de datos
socket_path        string  específico de pl.   path del unix socket
pidfile            string  específico de pl.   archivo PID (al daemonizar)
storage_driver     string  auto-detectado      overlay2|fuse-overlayfs|btrfs|zfs|vfs
default_network    string  bridge              red por defecto para nuevos contenedores
default_runtime    string  auto                modo de aislamiento por defecto (o auto)
rootless           bool    detectado por pl.    corre rootless (sin ops privilegiadas)
debug              bool    false               habilita modo debug
log_level          string  info                debug|info|warn|error
log_format         string  auto                auto|json|text
log_driver         string  json-file           driver de log del contenedor
log_opts           object  ver esquema          opciones específicas del driver
experimental       bool    false               habilita features experimentales
```

<hr>

## DNS

<sub>[RESOLVER]</sub>

```
FIELD        TYPE    DEFAULT                                  DESCRIPTION
-----------  ------  ---------------------------------------  -------------------------
dns          array   específico de plataforma                 servidores DNS upstream
dns_listen   string  127.0.0.11:53 (Linux)                    dirección de escucha del
                     127.0.0.11:8053 (Android)                servidor DNS interno
dns_search   array   []                                       dominios de búsqueda por defecto
dns_opts     array   ["ndots:0"]                              opciones DNS por defecto
```

Nota v0.11.0: en Android, `dns_listen` por defecto es `:8053` porque el puerto 53 está bloqueado por SELinux. Override con <kbd>DOKI_DNS_LISTEN</kbd>.

<hr>

## Registries

<sub>[AUTH]</sub>

```
FIELD                TYPE    DEFAULT   DESCRIPTION
-----------------    ------  --------  -----------------------------------------------
registry_mirrors     array   []        registries mirror probados antes del principal
insecure_registries  array   []        registries sin verificación TLS
```

Ejemplo — acelera pulls en China añadiendo mirrors:

```json
{
  "registry_mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com"
  ]
}
```

Ejemplo — permite un registry HTTP local:

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
network.bridge          string  doki0          nombre del bridge por defecto
network.default_subnet  string  10.0.0.0/24    subnet por defecto para nuevos bridges
network.mtu             int     1500           MTU del bridge
network.ipv6            bool    false          habilita IPv6 en el bridge por defecto
```

<hr>

## Cgroup

<sub>[CGROUP]</sub>

```
FIELD                  TYPE    DEFAULT          DESCRIPTION
-------------------    ------  ---------------  --------------------------------
cgroup.version         string  v2               versión de cgroup
cgroup.memory_limit    string  0 (ilimitado)    límite global de memoria
cgroup.cpu_shares      int     1024             shares de CPU por defecto
```

<hr>

## Seccomp

<sub>[SECCOMP]</sub>

```
FIELD              TYPE    DEFAULT   DESCRIPTION
---------------    ------  --------  -----------------------------------------------
seccomp.profile    string  default   default|unconfined|path custom
seccomp.allow      array   []        syscalls permitidos más allá del perfil por defecto
seccomp.deny       array   []        syscalls denegados (override de allow)
```

Perfil default de v0.11.0 permite aproximadamente 80 syscalls incluyendo `io_uring_*`, `pidfd_*`, `rseq`, `userfaultfd`, `copy_file_range`, `landlock_*`.

Denegados por defecto: `init_module`, `finit_module`, `delete_module`, `kexec_load`, `kexec_file_load`, `iopl`, `ioperm`, `kcmp`, `process_vm_readv`, `process_vm_writev`.

Ver [Seguridad](Security.es) para la lista completa.

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
cert         certificado del servidor (PEM)
key          clave privada del servidor (PEM)
client_ca    bundle CA para mTLS (opcional)
verify       requiere certs de cliente (mTLS)
```

Con TLS habilitado, el daemon escucha en `tcp://0.0.0.0:2376` (o el puerto configurado). En el cliente, setea <kbd>DOKI_HOST=tcp://host:2376</kbd>.

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

Algoritmo token-bucket: `rps` requests por segundo sostenidos, `burst` permitido en picos. Aplicado por IP de origen.

<hr>

## Metrics & debug

<sub>[OBSERVABILITY]</sub>

```
FIELD          TYPE    DEFAULT          DESCRIPTION
-----------    ------  ---------------  -----------------------------------------------
metrics_addr   string  127.0.0.1:9090   dirección de escucha de métricas Prometheus
debug_addr     string  127.0.0.1:6060   dirección de escucha de pprof (set DOKI_DEBUG=1)
```

<hr>

## Variables de entorno

<sub>[ENV]</sub>

Todo campo de config puede ser sobrescrito por una variable de entorno. Convención de naming: `DOKI_<UPPER_SNAKE_CASE>`.

```
ENV VAR                   FIELD                       EXAMPLE
-----------------------   -------------------------   ---------------------------------------
DOKI_HOST                 (socket del daemon)         unix:///var/run/doki.sock
                                                       tcp://host:2375
DOKI_CONFIG               (path del archivo config)   /etc/doki/config.json
DOKI_DATA_DIR             root                         /var/lib/doki
DOKI_STORAGE_DRIVER       storage_driver               overlay2
DOKI_DEFAULT_NETWORK      default_network              host
DOKI_DEFAULT_RUNTIME      default_runtime              proot
DOKI_ROOTLESS             rootless                     1 para habilitar
DOKI_DEBUG                debug                        1 (también activa pprof)
DOKI_LOG_LEVEL            log_level                    debug|info|warn|error
DOKI_LOG_FORMAT           log_format                   json|text|auto
DOKI_DNS                  dns (separado por comas)     8.8.8.8,1.1.1.1
DOKI_DNS_LISTEN           dns_listen                   127.0.0.11:8053
DOKI_DNS_SEARCH           dns_search (comma-sep)       local,internal
DOKI_DNS_OPTS             dns_opts (comma-sep)         ndots:0,timeout:3
DOKI_REGISTRY_MIRRORS     registry_mirrors (comma)     https://mirror1,https://mirror2
DOKI_INSECURE_REGISTRIES  insecure_registries (comma) registry.local:5000
DOKI_TLS                  habilita TLS                 1
DOKI_TLS_CERT             tls.cert                     /etc/doki/cert.pem
DOKI_TLS_KEY              tls.key                      /etc/doki/key.pem
DOKI_TLS_CA               tls.client_ca                /etc/doki/ca.pem
DOKI_TLS_VERIFY           tls.verify                   1
DOKI_METRICS_ADDR         metrics_addr                 0.0.0.0:9090
DOKI_DEBUG_ADDR           debug_addr                   0.0.0.0:6060
DOKI_RATE_LIMIT           rate_limit.rps               200
DOKI_EXPERIMENTAL         experimental                 1
DOKI_NATIVE               fuerza native (per-proceso)  1
DOKI_KERNEL               path del kernel microVM      /usr/share/doki/vmlinux
DOKI_LINK_STUN            URL del servidor STUN link    stun:stun.example.org:3478
DOKI_LINK_RELAY           URL del servidor relay link   relay:relay.example.org:3478
DOKI_LINK_PAYLOAD_ENC     habilita cifrado de payload   1
```

<hr>

## Defaults específicos por plataforma

<sub>[AUTO-DETECT]</sub>

El daemon auto-detecta la plataforma y rellena los defaults:

```
FIELD              LINUX                TERMUX                       MACOS
---------------    -----------------    -------------------------    -----------------
root               ~/.doki/data          $PREFIX/var/lib/doki         ~/.doki/data
socket_path        /var/run/doki.sock    $PREFIX/var/run/doki.sock    ~/.doki/doki.sock
storage_driver     overlay2              fuse-overlayfs               vfs
dns_listen         127.0.0.11:53         127.0.0.11:8053             (ninguno)
default_runtime    namespaces            proot                        native
```

<hr>

## Validación

<sub>[CHECK]</sub>

Corre `doki config validate` para verificar errores en tu config:

```bash
$ doki config validate
INFO  config valid
$ doki config validate 2>&1 | head -10
WARN  dns_listen ":53" puede estar bloqueado en Android; usa ":8053"
WARN  insecure_registries contiene URLs HTTP; no recomendado para producción
```

<hr>

## Migrando desde Docker

<sub>[COMPAT]</sub>

La mayoría de env vars de Docker se reconocen:

```
DOCKER               DOKI
-----------------    ---------------------
DOCKER_HOST          DOKI_HOST
DOCKER_CONFIG        DOKI_CONFIG
DOCKER_TLS_VERIFY    DOKI_TLS_VERIFY
DOCKER_CERT_PATH     directorio DOKI_TLS_CA
DOCKER_BUILDKIT      DOKI_EXPERIMENTAL=1
DOCKER_RATE_LIMIT    DOKI_RATE_LIMIT
```

`doki` cae a env vars de Docker si las de Doki no están seteadas.

<hr>

## Config programática

<sub>[SOURCE]</sub>

`pkg/common/config.go` expone el struct `Config`. Lo carga `LoadConfig(path)`:

```
1.  leer archivo JSON
2.  aplicar overrides de env vars
3.  mezclar defaults de plataforma
4.  devolver *Config validado
       |
       v
  +-----------+
  | *Config   |
  +-----------+
```

Para tests, usa `LoadConfigFromString(jsonString)`.

<hr>

## Fuente

```
pkg/common/config.go     esquema + loader
pkg/common/defaults.go   defaults por plataforma
cmd/dokid/main.go        env var -> mapeo de config
```