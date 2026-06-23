# Seguridad

<sub>[DEFENSE-IN-DEPTH / v0.11.0]</sub>

> Doki apila controles independientes: seccomp, AppArmor, capabilities
> de Linux, user namespaces, cgroups v2, TLS 1.3, NaCl secretbox,
> verificacion de imagenes, rate limiting. Ningun control unico es
> confiado a sostener todo. v0.11.0 shippea la criptografia completa de
> DokiLink Mesh: identidades Ed25519, derivacion de clave
> order-independent, gossip con proteccion de replay.

<hr>

## Modelo de Amenaza

<sub>[SCOPE / SUPERFICIES DE ATAQUE / MITIGACIONES]</sub>

```text
                    SUPERFICIE DE ATAQUE    MITIGACION
                    --------------          ----------
  +-----------+     syscall de kernel       deny list de seccomp
  | contenedor| -->  tabla                   AppArmor MAC
  +-----------+     contenedores pares       mount namespace
        |           fs del host              read-only + AppArmor
        |           red del host             bridge isolation (no promisc)
        v           recursos del host        limites cgroups v2
  +-----------+     supply chain de imagenes CAS + bloqueo path traversal
  |   host    |     socket de la API         TLS 1.3 + token + rate limit
  +-----------+     imagen maliciosa         validacion symlink + hardlink
        |           gossip de mesh           sig Ed25519 + ventana de replay
        v           transporte de mesh       TLS 1.3 + NaCl secretbox L2
  +-----------+
  |  kernel   |     side-channel             FUERA DE SCOPE
  +-----------+     acceso fisico            FUERA DE SCOPE
                    escape de hipervisor     FUERA DE SCOPE (solo MicroVM)
                    0-day de kernel          seccomp mitiga, no fixea
```

### En alcance

```text
AMENAZA                                 MITIGACION
------------------------------------    ----------------------------------
escape de contenedor via exploit kernel  seccomp bloquea syscalls peligrosos
contenedor leyendo pares                 mount namespace + seccomp
contenedor leyendo archivos del host     AppArmor + mounts read-only
sniffing de red                          bridge isolation (no modo promisc)
container DoS via agotamiento recursos   limites cgroups v2
ataque a supply chain de imagenes        bloqueo path traversal + CAS
acceso no autorizado a la API            TLS + token auth + rate limit
imagen maliciosa con backdoor            bloqueo path traversal + validacion symlinks
gossip de mesh replay / forgery          sig Ed25519 + nonce + timestamp
transporte de mesh eavesdrop / tamper    TLS 1.3 + NaCl secretbox L2
```

### Fuera de alcance

```text
ITEM                                  RAZON
-----------------------------------   ----------------------------------
side-channel (Spectre, Meltdown)      nivel kernel, no nivel contenedor
acceso fisico al host                 dominio de seguridad fisica
escape de hipervisor                  solo relevante para modo MicroVM
0-day de kernel                       seccomp mitiga, no fixea
```

<hr>

## Capas de Defensa

<sub>[STACK CONTENEDOR -> HOST]</sub>

```text
+----------------------------------------------------------+
| contenedor                                               |
|  +----------+   +-----------+   +-------------+          |
|  | app      |   | syscalls  |   | filesystem  |          |
|  | cod user |-->| seccomp   |-->| AppArmor    |          |
|  +----------+   +-----------+   +-------------+          |
|  +-------------+   +-----------+   +----------------+    |
|  | recursos   |   | red       |   | usuario        |    |
|  | cgroups v2 |-->| bridge    |-->| user namespace |    |
|  +-------------+   +-----------+   +----------------+    |
+----------------------------------------------------------+
                         |
                         v
                  +-------------+
                  | host kernel |
                  +-------------+
```

<hr>

## Seccomp

<sub>[FILTRO DE SYSCALLS / ~80 PERMITIDOS]</sub>

Doki distribuye un perfil seccomp por defecto. Permite ~80 syscalls y
bloquea los peligrosos.

### Lista de allow por defecto

Syscalls estandar: `read`, `write`, `open`, `openat`, `close`, `stat`,
`fstat`, `mmap`, `mprotect`, `brk`, `rt_sigaction`, `rt_sigprocmask`,
`rt_sigreturn`, `ioctl`, `pread64`, `pwrite64`, `readv`, `writev`,
`access`, `pipe`, `select`, `pselect6`, `poll`, `ppoll`, `dup`, `dup2`,
`dup3`, `socket`, `connect`, `accept`, `sendto`, `recvfrom`, `sendmsg`,
`recvmsg`, `bind`, `listen`, `getsockname`, `getpeername`,
`setsockopt`, `getsockopt`, `clone`, `fork`, `vfork`, `execve`, `exit`,
`exit_group`, `wait4`, `waitid`, `kill`, `tkill`, `tgkill`, `getpid`,
`gettid`, `getuid`, `getgid`, `geteuid`, `getegid`, `setuid`, `setgid`,
`setreuid`, `setregid`, `setsid`, `getrlimit`, `prlimit64`,
`getrusage`, `gettimeofday`, `clock_gettime`, `nanosleep`,
`sched_yield`, `sched_getaffinity`, `munmap`, `mremap`, `msync`,
`madvise`, `mincore`, `futex`, `getrandom`, `getcwd`, `chdir`, `mkdir`,
`mkdirat`, `rmdir`, `unlink`, `unlinkat`, `rename`, `renameat`, `link`,
`linkat`, `symlink`, `symlinkat`, `readlink`, `readlinkat`, `chmod`,
`fchmod`, `fchmodat`, `chown`, `fchown`, `fchownat`, `fstatfs`,
`statfs`, `umask`, `getpriority`, `setpriority`, `reboot`
(`kexec_load` bloqueado; `reboot` permitido para shutdown), `mount`,
`umount2`, `unshare`, `setns`, `capget`, `capset`, `prctl`, `seccomp`,
`personality`, `arch_prctl`, `time`, `set_tid_address`,
`restart_syscall`, `exit`, `exit_group`.

Syscalls modernos: `io_uring_setup`, `io_uring_enter`,
`io_uring_register`, `pidfd_open`, `pidfd_send_signal`, `pidfd_getfd`,
`rseq`, `userfaultfd`, `copy_file_range`, `landlock_create_ruleset`,
`landlock_add_rule`, `landlock_restrict_self`, `memfd_create`,
`close_range`, `faccessat2`, `process_mrelease`, `mseal`.

### Lista de deny por defecto

```text
SYSCALL                                  RAZON
-------------------------------------    --------------------------------
init_module, finit_module, delete_module carga de modulos de kernel
kexec_load, kexec_file_load              reemplazo de ejecucion de kernel
iopl, ioperm                             puertos I/O de hardware
kcmp                                     filtraciones de info kernel (cross-PID)
process_vm_readv, process_vm_writev      acceso a memoria cross-proceso
bpf                                      carga de programas BPF
perf_event_open                          monitoreo de performance
lookup_dcookie                           filtraciones de info de dentry cache
quotactl                                 manipulacion de quotas de filesystem
mount (MS_REMOUNT|MS_BIND)               vector de escalada de privilegios
swapon, swapoff                          manipulacion de swap
pivot_root                               escape de chroot
reboot (LINUX_REBOOT_CMD_KEXEC)          kexec-reboot
```

### Perfil custom

```json
{
  "seccomp": {
    "profile": "/etc/doki/seccomp/custom.json"
  }
}
```

El formato del perfil es el
[esquema seccomp de la OCI runtime spec](https://github.com/opencontainers/runtime-spec/blob/main/config-linux.md#seccomp):

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

```bash
doki run --security-opt seccomp=unconfined alpine echo hola
```

<hr>

## AppArmor

<sub>[MANDATORY ACCESS CONTROL / POR CONTENEDOR]</sub>

AppArmor proporciona MAC encima de DAC. Doki genera un perfil por
contenedor.

### Perfil por defecto

```c
#include <tunables/global>

profile doki-default flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  #include <abstractions/nameservice>

  // Deniega carga de modulos de kernel.
  deny capability sys_module,
  // Deniega I/O raw.
  deny capability sys_rawio,

  // Permite red.
  network inet stream,
  network inet6 stream,

  // Deniega mount.
  deny mount,
  deny umount,

  // Permite /docker/...
  /docker/** rwk,
  // Deniega todo lo demas.
  deny /** w,
  deny /** a,
}
```

### Perfil custom

```bash
doki run --security-opt apparmor=my-profile alpine echo hola
```

El perfil `my-profile` debe estar cargado en el kernel
(`apparmor_parser -a my-profile`).

<hr>

## Capabilities

<sub>[CAPABILITIES DE LINUX / DEFAULT MINIMO]</sub>

Los contenedores corren con un set minimo de capabilities por defecto:

```text
CHOWN, DAC_OVERRIDE, FSETID, FOWNER, MKNOD, NET_RAW, SETGID, SETUID,
SETFCAP, SETPCAP, NET_BIND_SERVICE, SYS_CHROOT, KILL, AUDIT_WRITE
```

Quita todas y anade solo lo requerido:

```bash
doki run --cap-drop=ALL --cap-add=NET_BIND_SERVICE my-server:latest
```

```text
CASO DE USO                       CAPABILITY
------------------------------    ----------------------
servidor web en puerto 80         NET_BIND_SERVICE
servidor de tiempo                SYS_TIME
cliente NFS                       SYS_ADMIN (con cuidado)
ping                              NET_RAW
tracing / debug                   SYS_PTRACE (muy peligroso)
```

<hr>

## User Namespaces

<sub>[MAPEO DE UID / ROOTLESS]</sub>

El usuario root del contenedor (UID 0) esta mapeado a un UID alto en el
host por defecto:

```json
{
  "uid_mappings": [{"container_id": 0, "host_id": 100000, "size": 65536}],
  "gid_mappings": [{"container_id": 0, "host_id": 100000, "size": 65536}]
}
```

Si el contenedor escapa, aparece como UID 100000 en el host. Sin acceso
root.

Deshabilita con `--userns=host` (no recomendado):

```bash
doki run --userns=host --rm alpine whoami
root  # <- peligroso: root real en el host
```

<hr>

## cgroups v2

<sub>[LIMITES DE RECURSOS / SOLO LINUX]</sub>

```bash
# Limite de memoria.
doki run -m 512m my-image

# Limite de CPU.
doki run --cpus 1.5 my-image

# Limite de PIDs.
doki run --pids-limit 100 my-image

# Peso de block I/O.
doki run --blkio-weight 500 my-image
```

Se requiere jerarquia unificada de cgroups v2. En distros mas antiguas:

```bash
# Habilita cgroup v2.
grubby --update-kernel=/boot/vmlinuz-$(uname -r) --args="systemd.unified_cgroup_hierarchy=1"
```

<hr>

## TLS / mTLS

<sub>[SOCKET DEL DAEMON / TLS 1.3 MINIMO]</sub>

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

Con `verify: true` (mTLS), el daemon requiere que los clientes
presenten un certificado firmado por `client_ca`. El CLI y los SDKs de
Docker manejan esto via `DOCKER_CERT_PATH` o env vars `DOKI_TLS_*`.
`NewTLSWrapper` enforcea `RequireAndVerifyClientCert` cuando un pool
de CA de cliente esta configurado y clona el config del caller para
evitar side effects.

Genera certs auto-firmados para testing:

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem -days 365 -nodes
```

Para produccion, usa una CA real (Let's Encrypt, PKI interna).

<hr>

## Rate Limiting

<sub>[TOKEN BUCKET POR IP]</sub>

```json
{
  "rate_limit": {
    "rps": 100,
    "burst": 200
  }
}
```

100 requests por segundo sostenidos, con ráfagas de hasta 200. Exceder
esto retorna HTTP 429.

<hr>

## Sandbox Landlock

<sub>[LSM UNPRIVILEGED / ABI v1-v9 / v0.10+]</sub>

Sandbox unprivileged via el LSM Landlock de Linux (Landlock ABI v9,
Linux 5.13+). A diferencia de seccomp (filtrado de syscalls) y AppArmor
(MAC basado en paths), Landlock provee control de acceso a filesystem
que cualquier usuario puede configurar sin root.

```go
// pkg/landlock/landlock.go
cfg := &SandboxConfig{
    FSRules: []FSRule{
        {Path: rootfs, Access: LandlockAccessFSExecute},
    },
    NetRules: []NetRule{
        {Port: 443, Access: LandlockAccessNetBindTCP},
    },
}
```

```text
TIPO DE ACCESO   CONTROLA
-----------      -------------------------------------------------------
filesystem       execute, write_file, read_file, read_dir, remove_dir,
                 remove_file, make_char, make_dir, make_reg, make_sock,
                 make_fifo, make_block, make_sym, refer, truncate,
                 ioctl_dev, resolve_unix
network          bind_tcp, connect_tcp
scope            abstract_unix_socket, signal
```

Doki probea el ABI de Landlock mas alto soportado en el host (v1 hasta
v9). Si Landlock no esta disponible (kernel < 5.13 o no compilado con
`CONFIG_SECURITY_LANDLOCK`), el sandbox se salta graceful.

<hr>

## Verificacion de Imagenes

<sub>[GUARDIAS DE EXTRACCION / PKG/STORAGE/LAYER.GO]</sub>

### Proteccion contra path traversal

```go
// pkg/storage/layer.go
if strings.Contains(path, "..") {
    return fmt.Errorf("path traversal: %s", path)
}
if filepath.IsAbs(path) {
    return fmt.Errorf("absolute path: %s", path)
}
```

### Validacion de symlinks

```go
// Si un target de symlink apunta fuera del rootfs, lo rechaza.
realPath, err := filepath.EvalSymlinks(target)
if !strings.HasPrefix(realPath, rootfsDir) {
    return fmt.Errorf("symlink escape: %s -> %s", target, realPath)
}
```

### Restricciones de hardlinks

Los hardlinks deben apuntar dentro de la misma capa (no cruzando
capas). Previene que un atacante haga hardlink de un archivo sensible
de una capa inferior a un dir upper escribible.

### Verificacion de contenido

El SHA256 de cada capa se verifica despues de la descarga. Si el
registry devuelve una capa con un digest diferente, la descarga se
rechaza.

<hr>

## Criptografia de DokiLink

<sub>[ED25519 / NACL SECRETBOX / PROTECCION DE REPLAY / v0.11.0]</sub>

DokiLink autentica todo el trafico de mesh con firmas Ed25519 y lo
cifra con TLS 1.3 y NaCl secretbox opcional. La identidad criptografica
se genera una vez por install y se persiste en `$DOKI_ROOT/keys/` con
permisos 0600.

### Identidad de install

```text
ARTIFACTO              ALGORITMO         LIFETIME     PATH
------------------     --------------    ---------    --------------------
keypair de identidad   Ed25519           permanente   keys/id_ed25519
install ID             base32(pub[:8])   permanente   derivado, 12 chars
certificado CA         ECDSA P-256       365 dias     keys/ca.crt
clave privada CA       ECDSA P-256       permanente   keys/ca.key (0600)
cert leaf de link      ECDSA P-256       90 dias      keys/<id>.crt
clave leaf de link     ECDSA P-256       90 dias      keys/<id>.key
pubkey pinnada de peer Ed25519           TOFU         keys/peers/<id>.pub.pem
```

La clave privada Ed25519 (64 bytes: seed + pub) nunca sale del host.
La clave publica (32 bytes) se broadcastea como fingerprint del peer y
se usa para firmar mensajes de mesh (HELLO, ADVERTISE, REVOKE, BYE).

### Capas de encryptacion

```text
CAPA     CUANDO                  LIBRERIA                         NOTAS
------   ----------------        ----------------------------     ------------------
L0 none  solo loopback           --                               default Android/Termux
L1 TLS   cualquier inter-host    crypto/tls (stdlib)              default, MinVersion = TLS 1.3
L2 box   opt-in solo payload     golang.org/x/crypto/nacl/secretbox  DOKI_LINK_PAYLOAD_ENC=1
```

### Configuracion TLS 1.3

```go
// pkg/netlink/crypto.go
cfg := &tls.Config{
    MinVersion:   tls.VersionTLS13,
    Certificates: []tls.Certificate{key},
}
// mTLS enforceado cuando un pool de CA cliente esta configurado.
if clone.ClientCAs != nil && clone.ClientAuth == tls.NoClientCert {
    clone.ClientAuth = tls.RequireAndVerifyClientCert
}
```

`NewTLSWrapper` clona el config del caller. La version minima esta
pineada a TLS 1.3. TLS 1.2 e inferiores no se negocian.

### Derivacion de clave NaCl secretbox

```go
// DeriveSecretKey es ORDER-INDEPENDENT: ambos pares derivan la misma
// clave de 32 bytes independientemente de cual pubkey se pasa primero.
func DeriveSecretKey(localPub, remotePub []byte) [32]byte {
    a := copy(localPub)
    b := copy(remotePub)
    if string(b) < string(a) {
        a, b = b, a              // sort lexicografico -> input estable
    }
    h := sha256.New()
    h.Write([]byte("dokilink-v1|"))
    h.Write(a)
    h.Write([]byte("|"))
    h.Write(b)
    var out [32]byte
    copy(out[:], h.Sum(nil))
    return out
}
```

```text
INPUT                          OUTPUT
---------------------------    ----------------------------------
localPub (32) + remotePub (32) clave NaCl secretbox de 32 bytes
derivacion                     SHA-256("dokilink-v1|" + min + "|" + max)
orden                          sorteado lexicograficamente (estable)
self-connection                deriva una clave estable igualmente
```

### Framing

```text
TRANSPORTE   LAYOUT DEL FRAME
---------    -----------------------------------------------------
TCP stream   4-byte BE length || nonce(24) || secretbox(plaintext)
UDP dgram    nonce(24) || secretbox(payload)
por-conn     base de nonce sembrada desde crypto/rand (nunca cero)
overhead     24 bytes nonce + 16 bytes tag secretbox por frame
max frame    16 MiB (rechazado por encima)
```

Cada conexion siembra su contador desde `crypto/rand` para que dos
conexiones independientes compartiendo la misma clave derivada nunca
reusen un nonce. El reuse de nonce en secretbox destruye
confidencialidad e integridad; el seeding es load-bearing.
`secretboxStreamConn.Close` usa `atomic.Bool` via `CompareAndSwap` para
prevenir un race de double-close.

### Proteccion de replay

```text
MECANISMO                  VALOR
----------------------     ----------------------------------
campos de mensaje         nonce aleatorio 16 bytes + timestamp
ventana de replay         5 minutos (replayWindow)
cache de nonce            map[string]time.Time, cap 1024
eviction                  LRU-style on insert + cleanup ticker
condiciones de rechazo    timestamp zero / mas viejo que ventana / nonce visto
cap de tamano gossip      MaxGossipMessageBytes = 4 KiB
guard del listener        io.LimitReader(cap+1) -> prevencion OOM DoS
```

```go
// pkg/netlink/mesh.go
const replayWindow    = 5 * time.Minute
const seenNonceLimit  = 1024
const MaxGossipMessageBytes = 4 * 1024

func (m *Mesh) checkReplay(msg Message) error {
    if msgTime.IsZero() || time.Since(msgTime) > replayWindow {
        return ErrStaleMessage
    }
    if _, seen := m.seenNonces[msg.Nonce]; seen {
        return ErrReplayDetected
    }
    if len(m.seenNonces) >= seenNonceLimit {
        // evicta nonces expirados
    }
    m.seenNonces[msg.Nonce] = time.Now()
    return nil
}
```

### Comparacion constant-time

```text
CALL SITE                          FUNCION
------------------------------     ----------------------------------
sort de pubkey en DeriveSecretKey  crypto/subtle.ConstantTimeCompare
mismatch TOFU en TrustStore.Trust  crypto/subtle.ConstantTimeCompare
verif de tag secretbox.Open        libreria NaCl (constant-time)
ed25519.Verify de firma            Go ed25519 (constant-time)
```

El mismatch de pubkey TOFU (trust-on-first-use) se chequea con
`crypto/subtle.ConstantTimeCompare`, no con un `bytesEqual` no
constant-time. Esto le niega al atacante un oraculo sobre la clave
pineada.

### Handshake de DokiLink

```text
PEER A                          PEER B
  |                               |
  |  STUN binding request         |
  |------------------------------>|
  |  XOR-MAPPED-ADDRESS           |
  |<------------------------------|
  |                               |
  |  TCP simultaneous open        |
  |  (hole punching)              |
  |<------------------------------>|
  |                               |
  |  gossip firmado Ed25519       |
  |  nonce + timestamp            |
  |  TLS 1.3 (L1)                 |
  |  secretbox opcional (L2)      |
  |<------------------------------>|
  |                               |
  |  ventana de replay: 5 min     |
  |  cache de nonce: 1024 entries |
```

<hr>

## Audit Logging

<sub>[LOG/SLOG / JSON / SIEM]</sub>

El daemon loguea todas las requests de la API via `log/slog`:

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

En modo JSON (produccion), esto es greppable y parseable. Pipea a un
SIEM.

<hr>

## Logs de Contenedor

<sub>[STDOUT/STDERR / ROTACION]</sub>

El stdout/stderr del contenedor se escribe a
`data/containers/<id>/logs/*.log` por defecto. Con
`--log-driver journald`, van al journal de systemd. Con
`--log-driver local`, usan un formato binario con rotacion.

```text
OPCION                DEFAULT     EFECTO
------------------    ---------   ----------------------------------
log_opts.max-size     10m         rota cuando un archivo excede esto
log_opts.max-file     3           guarda esta cantidad de archivos rotados
peor caso             30 MB       10 MB x 3 archivos por contenedor
```

<hr>

## Firma de Imagenes

<sub>[COSIGN / PLANEADO]</sub>

Doki planea soportar firmas de
[cosign](https://github.com/sigstore/cosign):

```bash
# Firma una imagen.
cosign sign --key cosign.key myapp:1.0

# Doki verificara en el pull.
doki pull myapp:1.0
INFO  verifying signature for myapp:1.0
INFO  signature valid (key: <fingerprint>)
```

<hr>

## Advisories de Seguridad

<sub>[DIVULGACION / 90 DIAS]</sub>

Los issues de seguridad deben reportarse a security@doki.opceanai.com
(PGP key en el sitio web). Se sigue un timeline de divulgacion de 90
dias.

<hr>

## Checklist de Hardening

<sub>[DEPLOYMENT EN PRODUCCION]</sub>

```text
[ ] habilita TLS en el socket del daemon        (DOKI_TLS=1)
[ ] usa mTLS si expones la API a la red         (tls.verify: true)
[ ] quita todas las capabilities por defecto    (--cap-drop=ALL)
[ ] usa modo rootless donde sea posible
[ ] corre con --read-only para contenedores estaticos
[ ] setea limites de memoria y CPU
[ ] setea --pids-limit para prevenir fork bombs
[ ] usa un perfil seccomp custom para cargas sensibles
[ ] usa un perfil AppArmor custom
[ ] pinea digests de imagen, no tags            (myapp@sha256:abc...)
[ ] habilita content trust (cuando este disponible)
[ ] audita logs a un SIEM central
[ ] actualiza Doki regularmente (parches de seguridad en releases de punto)
[ ] habilita DOKI_LINK_PAYLOAD_ENC=1 para mesh cross-host
```

<hr>

## Fuente

<sub>[PAQUETES CRITICOS DE SEGURIDAD]</sub>

```text
FILE                              ROL
------------------------------    -----------------------------------
internal/seccomp/                 motor de perfil seccomp
internal/apparmor/                generador de perfil AppArmor
pkg/common/capabilities.go        sets de capabilities
pkg/storage/layer.go              verificacion de imagenes
pkg/api/auth.go                   configuracion TLS
pkg/api/ratelimit.go              rate limiting
cmd/dokid/main.go                 logging de requests
pkg/netlink/keys.go               identidad de install, Ed25519 + ECDSA CA
pkg/netlink/crypto.go             wrappers TLS 1.3 + NaCl secretbox
pkg/netlink/mesh.go               gossip, proteccion de replay, cache de nonce
pkg/landlock/landlock.go          sandbox de filesystem unprivileged
```
