# Seguridad

Doki toma un enfoque defense-in-depth: seccomp + AppArmor + capabilities + user namespaces + TLS + verificación de imágenes + rate limiting. La versión v0.9.2 no añadió nuevas features de seguridad pero arregló varios bugs a nivel de regresión en el stack subyacente.

## Modelo de amenaza

Doki está diseñado para estos escenarios de amenaza:

### En alcance

| Amenaza | Mitigación |
|:--------|:-----------|
| Escape de contenedor vía exploit de kernel | Seccomp bloquea syscalls peligrosos |
| Contenedor leyendo datos de otros contenedores | Mount namespace + seccomp |
| Contenedor leyendo archivos del host | AppArmor + mounts read-only |
| Sniffing de red | Aislamiento de bridge (sin modo promiscuo por defecto) |
| Container DoS vía agotamiento de recursos | Límites de cgroups v2 |
| Ataque a la supply chain de imágenes | Protección contra path traversal, store content-addressable |
| Acceso no autorizado a la API | TLS + token auth + rate limiting |
| Imagen maliciosa con backdoor | Protección contra path traversal + validación de symlinks |

### Fuera de alcance

- Ataques side-channel (Spectre, Meltdown) — a nivel de kernel
- Acceso físico al host
- Escapes de hipervisor (solo relevante para modo MicroVM)
- 0-days de kernel (seccomp es mitigación, no fix)

## Capas

```
┌────────────────────────────────────────────────┐
│  Contenedor                                    │
│  ┌────────────┐                                │
│  │ App        │ ← código de usuario            │
│  └─────┬──────┘                                │
│  ┌─────▼──────┐                                │
│  │ Syscalls   │ ← filtro seccomp               │
│  └─────┬──────┘                                │
│  ┌─────▼──────┐                                │
│  │ Filesystem │ ← perfil AppArmor              │
│  └─────┬──────┘                                │
│  ┌─────▼──────┐                                │
│  │ Recursos   │ ← límites cgroups v2           │
│  └─────┬──────┘                                │
│  ┌─────▼──────┐                                │
│  │ Red        │ ← aislamiento bridge           │
│  └─────┬──────┘                                │
│  ┌─────▼──────┐                                │
│  │ Usuario    │ ← user namespaces              │
│  └─────┬──────┘                                │
└─────┬──┴───────────────────────────────────────┘
      │
      ▼  Kernel host
```

## Seccomp

Doki distribuye un perfil seccomp por defecto que permite ~80 syscalls y bloquea los peligrosos.

### Lista de allow por defecto

Syscalls estándar: `read`, `write`, `open`, `openat`, `close`, `stat`, `fstat`, `mmap`, `mprotect`, `brk`, `rt_sigaction`, `rt_sigprocmask`, `rt_sigreturn`, `ioctl`, `pread64`, `pwrite64`, `readv`, `writev`, `access`, `pipe`, `select`, `pselect6`, `poll`, `ppoll`, `dup`, `dup2`, `dup3`, `socket`, `connect`, `accept`, `sendto`, `recvfrom`, `sendmsg`, `recvmsg`, `bind`, `listen`, `getsockname`, `getpeername`, `setsockopt`, `getsockopt`, `clone`, `fork`, `vfork`, `execve`, `exit`, `exit_group`, `wait4`, `waitid`, `kill`, `tkill`, `tgkill`, `getpid`, `gettid`, `getuid`, `getgid`, `geteuid`, `getegid`, `setuid`, `setgid`, `setreuid`, `setregid`, `setsid`, `getrlimit`, `prlimit64`, `getrusage`, `gettimeofday`, `clock_gettime`, `nanosleep`, `sched_yield`, `sched_getaffinity`, `munmap`, `mremap`, `msync`, `madvise`, `mincore`, `futex`, `getrandom`, `getcwd`, `chdir`, `mkdir`, `mkdirat`, `rmdir`, `unlink`, `unlinkat`, `rename`, `renameat`, `link`, `linkat`, `symlink`, `symlinkat`, `readlink`, `readlinkat`, `chmod`, `fchmod`, `fchmodat`, `chown`, `fchown`, `fchownat`, `fstatfs`, `statfs`, `umask`, `getpriority`, `setpriority`, `getpriority`, `reboot` (kexec_load bloqueado, pero reboot está permitido para shutdown), `mount`, `umount2`, `unshare`, `setns`, `capget`, `capset`, `prctl`, `seccomp`, `personality`, `arch_prctl`, `time`, `set_tid_address`, `restart_syscall`, `exit`, `exit_group`

Syscalls modernos: `io_uring_setup`, `io_uring_enter`, `io_uring_register`, `pidfd_open`, `pidfd_send_signal`, `pidfd_getfd`, `rseq`, `userfaultfd`, `copy_file_range`, `landlock_create_ruleset`, `landlock_add_rule`, `landlock_restrict_self`, `memfd_create`, `close_range`, `faccessat2`, `process_mrelease`, `mseal`.

### Lista de deny por defecto

Los siguientes syscalls están explícitamente bloqueados, incluso con el perfil por defecto:

```
init_module, finit_module, delete_module      # Carga de módulos de kernel
kexec_load, kexec_file_load                   # Reemplazo de ejecución de kernel
iopl, ioperm                                  # Puertos I/O de hardware
kcmp                                          # Filtraciones de info de kernel (cross-PID)
process_vm_readv, process_vm_writev           # Acceso a memoria cross-proceso
bpf                                            # Carga de programas BPF
perf_event_open                                # Monitoreo de performance
lookup_dcookie                                # Filtraciones de info de dentry cache
quotactl                                       # Manipulación de quotas de filesystem
mount (con flags MS_REMOUNT|MS_BIND)         # Vector de escalada de privilegios
swapon, swapoff                                # Manipulación de swap
pivot_root                                     # Escape de chroot
reboot (con LINUX_REBOOT_CMD_KEXEC)          # Kexec-reboot
```

### Perfil custom

Sobrescribe el default con un path de perfil custom:

```json
{
  "seccomp": {
    "profile": "/etc/doki/seccomp/custom.json"
  }
}
```

El formato del perfil es el [esquema seccomp de la OCI runtime spec](https://github.com/opencontainers/runtime-spec/blob/main/config-linux.md#seccomp):

```json
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": ["SCMP_ARCH_X86_64", "SCMP_ARCH_AARCH64"],
  "syscalls": [
    {
      "names": ["read", "write", "open", "close"],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
```

### Deshabilitar seccomp

Para testing o compatibilidad con cargas inusuales:

```bash
doki run --security-opt seccomp=unconfined alpine echo hola
```

## AppArmor

AppArmor proporciona control de acceso obligatorio (MAC) encima del control de acceso discrecional (DAC). Doki genera un perfil por contenedor.

### Perfil por defecto

```c
#include <tunables/global>

profile doki-default flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  #include <abstractions/nameservice>

  # Deniega carga de módulos de kernel
  deny capability sys_module,
  # Deniega I/O raw
  deny capability sys_rawio,

  # Permite red
  network inet stream,
  network inet6 stream,

  # Deniega mount
  deny mount,
  deny umount,

  # Permite /docker/...
  /docker/** rwk,
  # Deniega todo lo demás
  deny /** w,
  deny /** a,
}
```

### Perfil custom

```bash
doki run --security-opt apparmor=my-profile alpine echo hola
```

El perfil `my-profile` debe estar cargado en el kernel (`apparmor_parser -a my-profile`).

## Capabilities

Por defecto, los contenedores corren con un set mínimo de capabilities:

```
CHOWN, DAC_OVERRIDE, FSETID, FOWNER, MKNOD, NET_RAW, SETGID, SETUID, SETFCAP, SETPCAP, NET_BIND_SERVICE, SYS_CHROOT, KILL, AUDIT_WRITE
```

Quita todas y añade solo lo que necesitas:

```bash
doki run --cap-drop=ALL --cap-add=NET_BIND_SERVICE my-server:latest
```

Sets comunes de capabilities:

| Caso de uso | Capabilities a añadir |
|:------------|:---------------------|
| Servidor web bindeando a puerto 80 | `NET_BIND_SERVICE` |
| Servidor de tiempo | `SYS_TIME` |
| Cliente NFS | `SYS_ADMIN` (con cuidado) |
| Ping | `NET_RAW` |
| Tracing/debug | `SYS_PTRACE` (muy peligroso) |

## User Namespaces

Por defecto, el usuario root del contenedor (UID 0) está mapeado a un UID alto en el host:

```json
{
  "uid_mappings": [{"container_id": 0, "host_id": 100000, "size": 65536}],
  "gid_mappings": [{"container_id": 0, "host_id": 100000, "size": 65536}]
}
```

Esto significa que incluso si el contenedor escapa, aparece como UID 100000 en el host — sin acceso root.

Deshabilita con `--userns=host` (no recomendado):

```bash
doki run --userns=host --rm alpine whoami
root  # ← peligroso: root real en el host
```

## cgroups v2

Límites de recursos vía cgroups v2 (solo Linux):

```bash
# Límite de memoria
doki run -m 512m my-image

# Límite de CPU
doki run --cpus 1.5 my-image

# Límite de PIDs
doki run --pids-limit 100 my-image

# Peso de block I/O
doki run --blkio-weight 500 my-image
```

Se requiere jerarquía unificada de cgroups v2. En distros más antiguas:

```bash
# Habilita cgroup v2
grubby --update-kernel=/boot/vmlinuz-$(uname -r) --args="systemd.unified_cgroup_hierarchy=1"
```

## TLS / mTLS

El daemon soporta TLS para conexiones de clientes:

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

Con `verify: true` (mTLS), el daemon requiere que los clientes presenten un certificado firmado por `client_ca`. El CLI y los SDKs de Docker manejan esto vía `DOCKER_CERT_PATH` o env vars `DOKI_TLS_*`.

Genera certs auto-firmados para testing:

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
```

Para producción, usa una CA real (Let's Encrypt, PKI interna, etc.).

## Rate Limiting

Rate limiter token-bucket por IP en la API:

```json
{
  "rate_limit": {
    "rps": 100,
    "burst": 200
  }
}
```

100 requests por segundo sostenidos, con ráfagas de hasta 200. Exceder esto retorna HTTP 429.

## Verificación de imágenes

La extracción de imágenes de Doki tiene múltiples capas de verificación:

### Protección contra path traversal

```go
// pkg/storage/layer.go
if strings.Contains(path, "..") {
    return fmt.Errorf("path traversal: %s", path)
}
if filepath.IsAbs(path) {
    return fmt.Errorf("absolute path: %s", path)
}
```

### Validación de symlinks

```go
// Si un target de symlink apunta fuera del rootfs, lo rechaza
realPath, err := filepath.EvalSymlinks(target)
if !strings.HasPrefix(realPath, rootfsDir) {
    return fmt.Errorf("symlink escape: %s -> %s", target, realPath)
}
```

### Restricciones de hardlinks

Los hardlinks deben apuntar dentro de la misma capa (no cruzando capas). Previene que un atacante haga hardlink de un archivo sensible de una capa inferior a un dir upper escribible.

### Verificación de contenido

El SHA256 de cada capa se verifica después de la descarga. Si el registry devuelve una capa con un digest diferente, la descarga se rechaza.

## Firma de imágenes (planeado)

Doki planea soportar firmas de [cosign](https://github.com/sigstore/cosign) para v1.0:

```bash
# Firma una imagen
cosign sign --key cosign.key myapp:1.0

# Doki verificará en el pull
doki pull myapp:1.0
INFO  verifying signature for myapp:1.0
INFO  signature valid (key: <fingerprint>)
```

## Audit Logging

El daemon loguea todas las requests de la API vía `log/slog`:

```json
{
  "time": "2024-01-15T10:30:00Z",
  "level": "INFO",
  "msg": "request",
  "method": "POST",
  "path": "/containers/create",
  "remote": "127.0.0.1:54321",
  "duration_ms": 12,
  "status": 201
}
```

En modo JSON (producción), esto es greppable y parseable. Pipea a tu SIEM.

## Logs de contenedor

El stdout/stderr de los contenedores se escribe a `data/containers/<id>/logs/*.log` por defecto. Con `--log-driver journald`, van al journal de systemd. Con `--log-driver local`, usan un formato binario con rotación.

Rotación de logs: `log_opts.max-size=10m`, `log_opts.max-file=3` (10 MB × 3 archivos = 30 MB máx por contenedor).

## Advisories de seguridad

Los issues de seguridad deben reportarse a security@doki.opceanai.com (PGP key en el sitio web). Seguimos un timeline de divulgación de 90 días.

## Checklist de hardening

Para deployments en producción:

- [ ] Habilita TLS en el socket del daemon (`DOKI_TLS=1`)
- [ ] Usa mTLS si expones la API a la red (`tls.verify: true`)
- [ ] Quita todas las capabilities por defecto (`--cap-drop=ALL`), añade solo lo necesario
- [ ] Usa modo rootless donde sea posible
- [ ] Corre con `--read-only` para contenedores estáticos
- [ ] Setea límites de memoria y CPU
- [ ] Setea `--pids-limit` para prevenir fork bombs
- [ ] Usa un perfil seccomp custom para cargas sensibles
- [ ] Usa un perfil AppArmor custom
- [ ] Pinea digests de imagen, no tags (`myapp@sha256:abc...`)
- [ ] Habilita content trust (cuando esté disponible)
- [ ] Audita logs a un SIEM central
- [ ] Actualiza Doki regularmente (parches de seguridad en releases de punto)

## Fuente

- `internal/seccomp/` — motor de perfil seccomp
- `internal/apparmor/` — generador de perfil AppArmor
- `pkg/common/capabilities.go` — sets de capabilities
- `pkg/storage/layer.go` — verificación de imágenes
- `pkg/api/auth.go` — configuración TLS
- `pkg/api/ratelimit.go` — rate limiting
- `cmd/dokid/main.go` — logging de requests
