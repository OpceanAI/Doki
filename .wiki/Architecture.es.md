# Arquitectura

<sub>[INTERNALES DEL DAEMON / ETAPAS DEL PIPELINE]</sub>

> Doki esta estructurado en cinco capas, cada una con un limite
> claro y un modo de fallo determinista. Esta pagina describe el
> pipeline desde la entrada del CLI hasta la syscall del kernel.

---

## Arquitectura por Capas

```
CAPA 1: CLI
  doki / doki-compose / doki-kubectl
  parsea intento del usuario -> llamada HTTP/gRPC al daemon
 ─────────────────────────────────────────────
CAPA 2: API DAEMON (dokid)
  Docker Engine v1.54  +  Podman libpod  +  TLS  +  rate limit
  HTTP/JSON sobre Unix socket / TCP
  middleware: logging, CORS, recovery, request-id, rate-limit
 ─────────────────────────────────────────────
CAPA 3: IMAGEN + REGISTRY
  manejo de manifest/digest/layer OCI
  pull/push con auth, resolucion multi-arch
  store de imagenes local (content-addressable)
  cache de capas con dedup
 ─────────────────────────────────────────────
CAPA 4: RUNTIME
  registro de runners: 12 backends
  selecciona el mejor modo disponible para el host
  proot -> native -> gVisor -> microVM -> wasm
  generacion + validacion de spec OCI
 ─────────────────────────────────────────────
CAPA 5: SERVICIOS DE PLATAFORMA
  almacenamiento (fuse-overlayfs / vfs / overlay2)
  networking (bridge / DNS / DokiLink mesh / NAT / DHT)
  volumes / pods / CRI / plano de control Kubernetes
```

---

## Etapas del Pipeline

<sub>[CONTAINER CREATE -> START]</sub>

```
1. CLI envia POST /containers/create
2. Daemon valida config (nombre, imagen, puertos, volumes, env)
3. Image store resuelve referencia -> digest
4. Extraccion de capas: stream tar -> directorio rootfs
   - fallos de chown en modo rootless: log una vez, no fatal
5. Spec OCI generada desde la config del contenedor
6. Registro de runners selecciona backend (proot, native, gVisor, etc.)
7. Estado del contenedor persistido (status=created)
8. CLI envia POST /containers/{id}/start
9. Runtime hace fork del binario runner con spec OCI
10. Setup de red: bridge (root) o pasta/slirp4netns/host (rootless)
11. Entradas DNS registradas para el ID del contenedor
12. Port forwarding via proxy TCP/UDP de DokiLink
13. Estado actualizado (status=running, pid=N)
14. Stream de logs disponible via GET /containers/{id}/logs
```

---

## Seleccion de Runner

<sub>[ORDEN DE PRIORIDAD / FALLBACK AUTOMATICO]</sub>

```
PRIORIDAD  RUNNER      CONDICION
────────────────────────────────────────────────────────────
1          wasm        media type de imagen es wasm
2          qemuuser    arch destino != arch host
3          fex         imagen x86 en host ARM
4          namespaces  host tiene CAP_SYS_ADMIN + /proc/self/ns
5          chroot      host soporta chroot(2)
6          microVM     /dev/kvm o dispositivo AVF presente
7          gvisor      binario gVisor instalado
8          pkdroid     Android 14+ con pKVM
9          sysbox      runtime sysbox instalado
10         proot       binario proot instalado (default Termux)
11         native      fallback, exec directo
12         legacy32    fallback 32-bit armv7
```

Override explicito: `DOKI_RUNTIME=gvisor` o `--runtime gvisor`.

---

## Estructura de Paquetes

```
cmd/
  doki/           binario CLI
  dokid/          binario daemon
  doki-compose/   binario compose
  doki-kube/      binario plano de control Kubernetes
  doki-kubectl/   binario cliente kubectl
pkg/
  api/            servidor Docker Engine API
  podman/         shim Podman libpod API
  runtime/        registro de runners + 12 backends
  image/          store de imagenes OCI + cliente de registry
  network/        bridge, DNS, DokiLink, NAT, DHT
  storage/        fuse-overlayfs, vfs, overlay2
  compose/        parser YAML, resolver de dependencias
  cri/            servidor gRPC CRI (41 RPCs)
  kubelet/        reconciliacion de pods via CRI
  kubeproxy/      proxy iptables/nftables/userspace
  scheduler/      asignacion pod-a-nodo
  controllers/    deployment, replicaSet, job, endpoint, etc.
  apiserver/      REST API de Kubernetes
  coredns/        servidor DNS del cluster
  store/          store en memoria + SQLite persistente
  netlink/        mesh, proxy, crypto, NAT traversal, DHT
  macos/          puente cgo VZ, QEMU, sandbox
  deps/           verificador de dependencias del sistema
  common/         tipos compartidos, config, validacion
  cli/            libreria cliente CLI
internal/
  proot/          cliente IPC de proot
  namespaces/     operaciones de namespace Linux
  fuse/           FUSE overlayfs
  cgroups/        manager cgroup v2
  seccomp/        generador de perfil seccomp
  apparmor/       generador de perfil AppArmor
  dokivm/         backends microVM (crosvm, firecracker, QEMU)
  gvisor/         integracion runner gVisor
  qemu/           integracion QEMU
  wasm/           runtime WebAssembly
```

---

## Modos de Fallo

```
COMPONENTE       FALLO                COMPORTAMIENTO
──────────────────────────────────────────────────────────
image pull       timeout de red       retry 3x, luego error
layer extract    disco lleno          aborta create, cleanup parcial
runner fork      permiso de exec      fallback al siguiente runner
network setup    pasta no encontrado  fallback a host netns
DNS              puerto en uso        log warning, continuar sin DNS
mesh listener    puerto en uso        log warning, mesh deshabilitado
store            disco lleno          rechazar writes, servir reads
CRI socket       permiso denegado     kubelet corre en modo fake
```

---

## Ver Tambien

- [Niveles de Aislamiento](Isolation-Levels.es) -- descripcion detallada de modos de runner
- [Networking](Networking.es) -- internales de bridge, DNS, DokiLink mesh
- [Seguridad](Security.es) -- seccomp, AppArmor, capabilities, TLS
- [Configuracion](Configuration.es) -- schema config.json y variables de entorno
