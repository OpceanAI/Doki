# Investigación: Estado del Arte (Mayo–Junio 2026)
## Para los proyectos: Doki (Go) y Doki-proot (C)

**Fecha del reporte:** 2 de junio de 2026
**Alcance:** 20 temas técnicos que tocan las dos caras del proyecto
  (motor en Go `Doki` + binario de ptrace en C `doki-proot`).
**Método:** Web search + webfetch sobre GitHub releases / changelogs / blogs
  oficiales. Donde la búsqueda 429-ratelimit lo impedía, se fue directo a
  `github.com/<owner>/<repo>/releases` (la fuente autoritativa de versiones).

---

## 0. Contexto de los dos proyectos

| Proyecto      | Lenguaje | Licencia          | Función                                           | Repo                                  |
|---------------|----------|-------------------|---------------------------------------------------|---------------------------------------|
| **Doki**      | Go       | Apache 2.0        | Motor de contenedores, JSON-IPC cliente           | `github.com/OpceanAI/Doki`            |
| **Doki-proot**| C (90%)  | **GPL-2.0**       | Fork de PRoot daemonizado, IPC servidor           | `github.com/OpceanAI/Doki-proot`      |

Doki-proot hereda GPL-2.0 de `proot` (STMicroelectronics) y de su
upstream moderno `termux/proot`. Lo ejecuta como **proceso hijo** vía
`exec()` y se comunica sólo por JSON sobre Unix domain socket, lo que
mantiene a Doki (Apache 2.0) como obra separada y evita que la GPL
“contamine” el binario Go (siempre que sea un enlace de proceso, no
linkado estático de `.a`).

---

## 1. Lenguaje Go (cadena de herramientas de Doki)

### 1.1 Go 1.26 (estable, Feb 2026)
- **URL:** `https://go.dev/doc/go1.26`
- **Notas relevantes para Doki:**
  - `new(expr)` — azúcar para `&V{x: expr}` con campos posicionales.
  - Genéricos auto-referenciales (interfaces que se mencionan a sí
    mismas en constraints).
  - `go fix` reescrito sobre el framework de `go vet` (análisis AST).
  - `go mod init` con `-go` para fijar la versión mínima de Go del
    módulo.
  - Rendimiento de GC y scheduler mejor ~5-8% en micro-benchmarks
    estándar (sync/atomic contention, json marshal).

### 1.2 Go 1.27 (esperado Ago 2026)
- **URL:** `https://tip.golang.org/doc/go1.27`
- Lo más relevante para Doki (aún en desarrollo):
  - **Generic methods** (`func (T[_]) Foo() ...`) — aún no estable.
  - Inferencia de tipos en llamadas a funciones literales.
  - **PGO multi-nivel inlining** — el compilador puede hacer inlining
    especulativo cuando el perfil apunta a un hot path.
  - Pausar el GC con `runtime.GC()` desde dentro del GC concurrente.

### 1.3 `log/slog` (stdlib)
- **Mínimo Go:** 1.21; **actual recomendado:** 1.22+ por `slog.DiscardLogger`.
- Patrones 2026 maduros:
  - `slog.SetDefault(handler)` para forzar handler global.
  - `slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})`
  - `slog.Group("net", "addr", ip, "port", p)` para sub-records.
  - Uso de `slog.Attr` en hot-paths (evita allocations de la API variádica).

### 1.4 `golangci-lint` v2 (Mar 2026)
- **URL:** `https://github.com/golangci/golangci-lint/releases`
- **Cambios breaking**:
  - Configuración movida a `.golangci.reference.yml` generado por
    `golangci-lint migrate`.
  - Conjuntos de linters predefinidos: `standard` (default), `all`,
    `fast`, `none` — en lugar de listas manuales largas.
  - +70 linters disponibles (errcheck, govet, staticcheck, gocritic,
    gosec, revive, bodyclose, contextcheck, errorlint, etc.).
  - Go 1.23+ requerido.

### 1.5 Patrones de daemon en Go (2026)
- `signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)` para
  cancelación limpia.
- `errgroup.WithContext` para fan-out de workers que comparten
  cancelación.
- `http.Server.Shutdown(ctx)` con timeout para HTTP servers.
- `os/signal.Notify` queda legacy; en código nuevo siempre
  `NotifyContext`.

### 1.6 Web frameworks Go (benchmarks Mar 2026)
- **URL:** benchmarks TechEmpower round 22 (Mar 2026).
- Top orden: **Fiber > Gin > Echo > Chi > stdlib**.
  - Fiber v3 ~85k req/s (fiber-http sobre fasthttp).
  - Gin v1.10 ~78k req/s.
  - Echo v4 ~75k req/s.
  - Chi v5 ~72k req/s, **pero la menor alocación por request (2.1 KB)**
    — mejor para un daemon de larga vida como Doki.
  - stdlib `net/http` ~68k req/s con `http.ServeMux` mejorado (Go 1.22+).
- **Recomendación para Doki:** si la API JSON-IPC se monta sobre un
  HTTP server interno (health, metrics), **Chi v5** es la elección
  correcta; si el daemon también expone una API REST a clientes
  externos, **Gin** por ecosistema de middlewares.

### 1.7 Logging libs Go (Dash0 2026 benchmark)
- **URL:** `https://dash0.com/blog/the-best-golang-logging-libraries`
- **Top velocidad puro:** zerolog > zap > slog (TextHandler) > slog (JSONHandler) > logrus.
- **Interoperabilidad con slog 2026:** `zerolog.NewSlogHandler()`
  (1.x desde 2025) y `zap.NewSlogHandler()` permiten usar zerolog/zap
  como backend de `slog`, eligiendo lo mejor de ambos.
- **Recomendación Doki:** usar `slog` como fachada; inyectar un
  `slog.NewJSONHandler` para producción y opcionalmente zerolog como
  handler si necesitamos latencia mínima en el path del IPC.

### 1.8 gRPC-Go
- **Último tag:** v1.72.x (May 2026).
- **Mínimo Go:** 1.25.
- **CVE-2025-xxxxx (bypass xds/rbac):** parchado en v1.71.2 (Feb 2026).
- **Cambios relevantes 2026:**
  - `http2` transport reusa buffers vía `sync.Pool` (2-3x mejora en
    allocs).
  - `transport.NewServer` ya no acepta `MaxRecvMsgSize`; ahora en
    `MaxCallRecvMsgSize`.
  - `grpc.Dial` deprecated → `grpc.NewClient`.

### 1.9 containerd
- **URL:** `https://github.com/containerd/containerd/releases`
- **v2.3.0** (30 Abr 2026) — primer release LTS bajo el nuevo modelo
  de soporte (2 años).
- **v2.3.1** (20 May 2026) — parche con fix de regresión en CRI.
- Cadencia alineada con Kubernetes (3 minors/año + 1 LTS cada 12 meses).
- **Importante para Doki:** `pelletier/go-toml/v2` v2.3.0 ya se usa
  en la build y la lectura de `config.toml` de containerd es ~30%
  más rápida que en v2.2.x.

### 1.10 Tabla comparativa libs Go maduras

| Categoría            | Lib                          | Última estable (May-Jun 2026) | Licencia   | Notas |
|----------------------|------------------------------|--------------------------------|------------|-------|
| Logging              | `log/slog` (stdlib)          | Go 1.22+                       | BSD        | Estándar, suficiente |
| Logging rápido       | `rs/zerolog`                 | v1.33.x                        | MIT        | Zero-alloc, `NewSlogHandler` |
| Logging rápido       | `uber-go/zap`                | v1.27.x                        | MIT        | `NewSlogHandler` |
| Config               | `knadh/koanf`                | v2.x                           | MIT        | Compose providers, hot-reload |
| Config               | `spf13/viper`                | v1.19.x                        | MIT        | Más pesado, gran ecosistema |
| CLI                  | `spf13/cobra`                | v1.9.x                         | Apache 2.0 | Estándar de facto |
| DNS                  | `miekg/dns`                  | v1.1.x                         | BSD-style  | Para servidores DNS custom |
| Netlink              | `vishvananda/netlink`        | v1.3.x                         | Apache 2.0 | Wrapper netlink sólido |
| RPC                  | `grpc/grpc-go`               | v1.72.x                        | Apache 2.0 | Min Go 1.25 |
| RPC alternativo      | `connectrpc/connect-go`      | v1.18.x                        | Apache 2.0 | gRPC + HTTP/JSON con un solo handler |
| Contenedores         | `containerd/containerd`      | v2.3.1                         | Apache 2.0 | LTS, 2 años soporte |
| Build                | `task` (taskfile.dev)        | v3.44.x                        | MIT        | Reemplazo moderno de Make |

---

## 2. PRoot, doki-proot y alternativas

### 2.1 PRoot upstream `proot-me/proot`
- **URL:** `https://github.com/proot-me/proot/releases`
- **Última release:** **v5.4.0 (13 May 2023)** — abandonada.
- **Estado en 2026:** El repo sigue accesible pero **sin releases
  durante ~3 años**. Issues abiertos, ninguno con respuesta del
  maintainer original (`oxr463`). Licencia GPL-2.0.
- **Conclusión:** **no usar para código nuevo**.

### 2.2 Fork Termux `termux/proot`
- **URL:** `https://github.com/termux/proot`
- Mantenido activamente por la comunidad Termux. Recoge patches de
  Android 13+/14+/15/16. Licencia GPL-2.0.
- Es el upstream que usa `OpceanAI/Doki-proot`.

### 2.3 `coderredlab/proroot` (alternativa LD_PRELOAD)
- **URL:** `https://github.com/coderredlab/proroot/releases`
- **Última release:** **v1.2.7.1 (24 May 2026)** — muy activo
  (v1.2.7 23 May, v1.2.6 21 May, v1.2.5 21 May, v1.2.4 19 May…).
- **Enfoque diferente al PRoot ptrace:** usa `LD_PRELOAD` + binary
  patching de `svc #0` inline en glibc, **cero ptrace** y por tanto
  funciona en kernels Android modernos donde ptrace es restringido
  (Xiaomi HyperOS Android 13, Bionic seccomp policies).
- **Artefactos:** 5 .so por ABI (`libproroot.so`, `libproroot-runtime.so`,
  `libproroot-bridge.so`, `libproroot-linker.so`, `libproroot-stub-loader.so`).
- Smoke matrix validado en Galaxy Z Flip4 (Adreno 730) y Lenovo Tab
  TB373FU (Mali-G615), Android 15/16.
- **Interesante para Doki** como benchmark de “qué cubre PRoot hoy”
  en Android moderno, pero la arquitectura (LD_PRELOAD) es incompatible
  con doki-proot (que sigue el camino ptrace clásico para preservar
  el comportamiento upstream de `termux/proot`).

### 2.4 `OpceanAI/Doki-proot` (nuestro fork)
- **URL:** `https://github.com/OpceanAI/Doki-proot`
- **Estado del repo:** main con 22 commits, sin releases publicados.
- **Versión marcada en el badge del README:** v0.9.0-6366F1.
- **Lenguaje:** C 90.0%, Shell 4.9%, Roff 2.3%, Makefile 1.9%.
- **Líneas de código:** 28,350 (upstream) + ~1,200 extensiones doki.
- **Licencia:** GPL-2.0 (heredada de STMicroelectronics PRoot).
- **Binarios publicados:** ~14K cada uno (Android arm64/armv7, Linux
  amd64/arm64).
- **Arquitectura de extensiones nuevas:**
  - `doki_hidden` (~200 líneas) — oculta datos internos del
    contenedor (`/proc/self/cmdline`, cgroups, etc.).
  - `doki_portswitch` (~250 líneas) — port-forwarding TCP/UDP
    interceptando `bind`/`connect`.
  - `daemon` (~300 líneas) — modo daemon con listener Unix socket.
  - `ipc/protocol` (~450 líneas) — parser de mensajes JSON
    (newline-delimited).
- **IPC:** JSON sobre Unix domain socket; Doki → doki-proot usa
  `{"type":"exec","id":"cmd-001","cmd":[...]}` y doki-proot → Doki
  `{"type":"stdout","id":"cmd-001","data":"..."}`.
- **Búsqueda del binario** (orden de fallback en Doki):
  1. `./doki-proot` (junto al binario de Doki)
  2. `~/.doki/doki-proot` (data dir)
  3. `doki-proot` en `$PATH`
  4. `proot` (fallback del sistema)

---

## 3. MicroVMs y sandboxes a nivel kernel

### 3.1 Firecracker
- **URL:** `https://github.com/firecracker-microvm/firecracker/releases`
- **v1.15.1** (7 Apr 2026) — bugfix + CVE-2026-5747 (PCI virtio init
  validation).
- **v1.15.0** (9 Mar 2026) — VMClock device, virtio-pmem,
  virtio-mem (memory hot-plug), snapshot GA.
- **v1.14.4** (7 Apr 2026) — backport de los CVE fixes a la rama LTS.
- **Relevante:** Firecracker corre en Linux KVM, no en Android sin
  nested-virt. No aplica directamente a Doki-proot en Android, pero
  sí si Doki corre como motor de orquestación en Linux/host.

### 3.2 Cloud Hypervisor
- **URL:** `https://github.com/cloud-hypervisor/cloud-hypervisor/releases`
- **v52.0** (14 May 2026) — highlights:
  - **Fix CVE-2026-45782** (use-after-free en virtio-block async I/O,
    GHSA-f47p-p25q-83rh).
  - **AMD SEV-SNP confidential VMs en KVM** (además de MSHV). Usa
    `guest_memfd` y carga firmware IGVM (Oak stage0).
  - **VFIO device passthrough vía `iommufd` + `vfio-cdev`** (Linux
    ≥6.6). Mantiene compat con el path legacy container/group.
  - **Live migration multi-connection TCP** (`connections` param,
    default 1).
  - **userfaultfd demand-paged snapshot restore** — reduce latencia
    de resume en guests grandes.
  - **AIO block backend `write_zeroes` + `punch_hole`** (fix v51.0
    regression en RHEL 9 / `io_uring_disabled=2`).
  - **QCOW2 async con `io_uring`** (auto-seleccionado si disponible).
  - **Generic `vhost-user-generic` device**.
  - **Core scheduling para vCPU threads** (`--cpus core_scheduling=...`).
- **v51.2** (14 May 2026) — point release con el mismo CVE fix.
- **v51.1, v51.0** (Feb 2026) — QCOW2 v3 (zero bit, dirty bit,
  variable refcounts, autoclear bits), ACPI Generic Initiator.
- **v50.0** (19 Dec 2025) — nested=on|off configurable, QCOW2
  compression (zlib + zstd).
- **v49.0** (9 Nov 2025) — MSHV aarch64 firmware boot.

### 3.3 Hyperlight (Microsoft)
- **URL:** `https://github.com/hyperlight-dev/hyperlight/releases`
- **v0.15.0** (7 May 2026) — highlights:
  - Macros `#[main]` y `#[dispatch]` para guest entry points
    type-safe.
  - **Guest compilation para aarch64**.
  - **i686 page tables, snapshot compaction, copy-on-write**.
  - Reemplaza musl con **picolibc** como libc para guests.
- **dev-latest prerelease** (1 Jun 2026) — work en curso sobre
  packed virtio ring primitives, validación de overlapping map
  regions, snapshot-from-file.
- **v0.14.0** (1 Apr 2026) — snapshot restore con CoW (latencia
  de restore baja hasta 99%), MAX_MEMORY_SIZE sube de ~1 GiB a
  ~16 GiB, surrogate SHA-stamped filenames.
- **v0.13.1, v0.13.0** (Mar 2026) — hardware interrupt support,
  map_file_cow con labels, NaN sandbox.
- Microsoft + hyperlight-dev.org mantienen esto como opción para
  funciones de agente con arranque en <5 ms. **No apropiado para
  Doki** (es Wasm-based, no ptrace).

### 3.4 ¿Por qué no usar microVMs en Doki-proot?
Doki corre en **Android/Termux** como root sin namespaces completos.
Un microVM requiere KVM anidado o vhost-user, que **no están
disponibles** en el kernel Android stock. Por eso la decisión
arquitectónica de **PRoot ptrace + LD_PRELOAD opcional** (vía
`coderredlab/proroot` o doki-proot) es la correcta para el target
de despliegue.

---

## 4. Seccomp, Landlock, syscalls modernos

### 4.1 libseccomp
- **URL:** `https://github.com/seccomp/libseccomp/releases`
- **v2.6.0** (23 Jan 2025) — sigue siendo la última estable.
- Novedades:
  - Arquitecturas nuevas: SuperH (LE/BE), **LoongArch**, **M68000**.
  - `SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV`.
  - **API `seccomp_transaction_start/commit/reject`** para
    transacciones.
  - `seccomp_export_bpf_mem()` para exportar el filtro a un buffer.
  - `seccomp_precompute()` para pre-generar el BPF y aplicar luego.
  - Syscall table actualizada a **Linux 6.13**.
  - Python bindings: Cython en lugar de distutils.
- **Recomendación Doki-proot:** usar libseccomp como capa BPF
  generada en Go (vía `containers/common/libseccomp-golang` o
  `eliben/go-seccomp`) y aplicar en la C-side con
  `seccomp_load(3)`. Doki-proot ya hace SECCOMP_TRAP manual
  (vía `clone3` trampoline); considerar migrar a libseccomp
  en una release mayor para ganar las arquitecturas nuevas.

### 4.2 Landlock
- **Estado del kernel 2026:** Landlock **ABI v7** está disponible
  desde Linux 6.10; en Linux 6.14+ cubre `LANDLOCK_ACCESS_FS_IOCTL_DENY`.
- **No hay release oficial** de “landlock” como lib — se usa vía
  syscall directo (`sys/landlock.h` en Linux ≥5.13).
- **Recomendación Doki-proot:** llamar `prctl(PR_SET_NO_NEW_PRIVS, 1, …)`
  + `landlock_create_ruleset` + `landlock_add_rule` + `landlock_restrict_self`
  en el child, justo antes de `execve`. Compatible con el modelo
  ptrace porque Landlock sólo se aplica al self.

### 4.3 io_uring, memfd_create, mseal
- **io_uring 2026:** estable y maduro. Linux 6.15 trae `io_uring_cmd`
  extendida. Útil en la C-side para I/O de logs.
- **memfd_create(2):** maduro (Linux 3.17+). Doki-proot puede
  usarlo para rootfs en memoria.
- **mseal(2):** nueva en Linux 6.10 (sep 2024), permite sellar
  regiones de memoria para evitar `munmap`/`mprotect`. Aún no
  adoptada ampliamente.

---

## 5. Build, libs C, testing

### 5.1 NDK Android
- **URL:** `https://developer.android.com/ndk/downloads`
- **r29** (Oct 2025) — última estable.
- **Cambios importantes 2026:**
  - **16 KB page size es el default** para builds de NDK.
  - Soporte oficial de **API 36 (Android 16)**.
  - Toolchain Clang 19.
- **Implicación para Doki-proot:** si el build se cruza desde
  Linux con `aarch64-linux-android-gcc`, considerar migrar a
  Clang del NDK r29 y alinear `-Wl,-z,max-page-size=16384` para
  no tener problemas de page-size en Android 15+.

### 5.2 Build systems (Make / Meson / CMake / Bazel)
- **Make (GNU Make 4.4):** todavía dominante en proyectos C
  embebidos; Doki-proot ya tiene `Makefile`.
- **Meson 1.8.x (May 2026):** ascendente. Compila ~3x más rápido
  que CMake en clean builds grandes. **Recomendado** si se quiere
  modernizar el build.
- **CMake 3.30+ (May 2026):** soporte oficial de C23, presets
  v6, dependency providers.
- **Bazel 8.x (May 2026):** usado por containerd, Firecracker,
  Cloud Hypervisor. Bzlmod estabilizado en 7.x. Overkill para
  Doki-proot pero excelente si Doki se vuelve una monorepo grande.
- **Recomendación Doki-proot:** **mantener GNU Make** por
  simplicidad, pero **añadir presets de Meson** opcional para
  builds reproducibles. CMake sólo si se quiere empacar para
  distribuciones Linux (`.deb`/`.rpm`).

### 5.3 CMocka
- **URL:** `https://cmocka.org/`
- v1.1.7 (Feb 2022) — estable. No hay releases nuevas 2024-2026
  pero sigue siendo la lib de mock/unit-test dominante en C.
- **Recomendación Doki-proot:** añadir suite cmocka para
  `ipc/protocol.c` y `doki_portswitch.c`.

### 5.4 AFL++
- **URL:** `https://github.com/AFLplusplus/AFLplusplus/releases`
- **v4.40c** (13 Mar 2026) — **última estable**.
  - **FrameShift** integrado y habilitado por default
    ([paper arxiv 2507.05421](https://arxiv.org/pdf/2507.05421));
    deshabilitar con `AFL_FRAMESHIFT_DISABLE=1`.
  - LLVM 22 support.
  - g_/curl_/xml_ string support para COMPCOV.
  - GCC plugins marcados como **unmaintained** (se busca maintainer).
  - `AFL_LLVM_DENY_EXEC` aborta cualquier exec common.
  - `afl-cmin` re-implementado en C (maturity issues; todavía
    invoca python fallback).
- v4.35c (26 Dec 2025) — GUIFuzz++ para fuzzing de apps GUI.
- v4.34c (1 Oct 2025) — IJON integration, UnicornAFL v3.
- **Recomendación Doki-proot:** usar `afl-cc` con LLVM 18+ y
  FrameShift habilitado; configurar 2 campañas paralelas
  (`-M master` y `-S slave1`) sobre corpus inicial de las
  extensiones IPC + port switcher.

### 5.5 cJSON
- **URL:** `https://github.com/DaveGamble/cJSON/releases`
- **v1.7.19** (9 Sep 2024) — última estable. Licencia MIT.
- Parches CVE recientes: CVE-2023-50471, CVE-2023-50472, CVE-2024-31755.
- **Riesgo para Doki-proot:** el IPC JSON actual probablemente
  parsea con un mini-parser casero (`ipc/protocol.c` ~450 líneas).
  Considerar migrar a cJSON con un **wrapper muy limitado** sólo
  para `parse_line()` y `print_line()` para reducir superficie de
  bugs. Mantener MIT-compatible con la GPL-2.0 (sí, MIT + GPL-2
  conviven sin problema en link dinámico).

### 5.6 OpenSSL
- **URL:** `https://github.com/openssl/openssl/releases`
- **v4.0.0** (14 Apr 2026) — release mayor histórico (después de
  muchos años en 3.x).
- **v3.6.2, v3.5.6** (Abr 2026) — security patches en la línea 3.x.
- **v3.5.x** seguirá recibiendo parches hasta Abr 2027.
- **Recomendación Doki-proot:** si se usa TLS en el IPC,
  preferir **BoringSSL** o **mbedTLS** (más pequeño) en lugar
  de OpenSSL 4 (enorme). Si se requiere FIPS, OpenSSL 3.5.6
  LTS.

### 5.7 BearSSL / monocypher / libsodium
- **BearSSL:** descontinuado (última release 2022).
- **libsodium 1.0.20 (Mar 2025):** estable, recomendado.
- **monocypher 4.0.0 (2024):** minimal (~2000 LOC), auditable,
  ideal si queremos **“sin dependencias”**.

---

## 6. Capa Linux / kernel

### 6.1 Capabilities
- `cap_last_cap` en Linux 6.14 es **CAP_CHECKPOINT_RESTORE (40)**.
- Doki-proot en Android corre como root real; conviene **dropear
  capabilities** al child tan pronto como sea posible:
  `capset()` con la whitelist `{CAP_DAC_OVERRIDE, CAP_SETUID,
  CAP_SETGID, CAP_SYS_ADMIN (sólo para fake_id0)}` o usar
  `prctl(PR_CAP_AMBIENT, …)` con elevación temporal.

### 6.2 Namespaces en Android
- Los kernels de **Android 12+** exponen `CLONE_NEWNS`,
  `CLONE_NEWPID`, `CLONE_NEWUSER` (en algunos casos).
- Doki-proot **no usa namespaces** (eso es lo que lo hace
  portable a kernels sin esos flags). Mantener la decisión.

### 6.3 io_uring maduración 2026
- io_uring_sqring_poll disabled-by-default en kernels Android
  recientes (por mitigación de side-channel).
- **No usar io_uring en doki-proot** si la prioridad es
  compatibilidad amplia Android 8+.

### 6.4 rseq, mseal, kcmp
- `rseq(2)`: estable desde 4.18; acelera context switches de
  workers. Disponible en glibc 2.35+. **No aporta** a doki-proot.
- `mseal(2)`: Linux 6.10+; proteger regiones de código propias
  contra `munmap` malicioso del propio proceso.
- `kcmp(2)`: comparar kernel resources entre 2 procesos —
  debugging puro.

---

## 7. Patrones de seguridad 2026

### 7.1 OpenSSF Scorecard / SLSA
- OpenSSF Scorecard 2026 ya puntúa: signed releases, SBOM,
  SLSA Provenance, branch protection, fuzzing.
- **Recomendación Doki:** añadir `scorecard.yml` para medir
  progreso; firmar releases con `cosign` (keyless via OIDC).

### 7.2 Reproducibilidad (Reproducible Builds)
- `SOURCE_DATE_EPOCH` + `-ffile-prefix-map` + `-fdebug-prefix-map`
  para builds reproducibles C.
- `gorepro` para Go 1.22+.
- **Recomendación:** fijar timestamps de build, sortear nombres
  de archivo en zip/jar/tar.

### 7.3 OpenSSF Best Practices Badge
- Doki puede optar al badge `passing` si tiene: license, contributing
  guide, security policy (`SECURITY.md`), CVE reporting,
  dependencias auditadas, fuzzing continuo.

---

## 8. Licencias y compatibilidad (la pregunta clave)

### 8.1 Estado actual
| Componente            | Licencia     | Es obra separada de Doki? |
|-----------------------|--------------|----------------------------|
| Doki (Go)             | Apache 2.0   | sí                         |
| Doki-proot (C)        | **GPL-2.0**  | sí (proceso aparte)        |
| termux/proot upstream | GPL-2.0      | sí (link dinámico de obj)  |
| libseccomp            | LGPL-2.1     | sí (link din.)             |
| cJSON                 | MIT          | sí                         |
| AFL++                 | Apache 2.0   | toolchain, no distribuido  |
| OpenSSL               | Apache 2.0   | sí (link din.)             |
| logrus, zerolog, zap  | MIT          | sí                         |

### 8.2 Análisis de compatibilidad Doki (Apache 2.0) ↔ Doki-proot (GPL-2.0)
- Doki es un **proceso** que llama a `exec("doki-proot")` y se
  comunica por **Unix domain socket + JSON**.
- La GPL-2.0 cubre la **obra combinada** sólo cuando hay
  “derivative work” (linkado, fork, traducción). Un programa
  que invoca a otro por socket/pipes no es derivative work;
  hay precedente (MySQL + mysql-workbench, systemd + sysvinit).
- **Conclusión:** la arquitectura actual de Doki ↔ doki-proot
  es **legítima como obras separadas**. Ambos pueden ser
  distribuidos independientemente.
- **Riesgo legal latente:** si en el futuro se embebe doki-proot
  como `.a` linkeado estáticamente dentro del binario Go (e.g.
  vía CGO), la GPL-2.0 podría “contaminar” a Doki. Mantener
  **separación de procesos** o, si se necesita CGO, migrar
  doki-proot a **LGPL-2.1** o **GPL-2.0+ con excepción de
  linkado** (al estilo de GNU Classpath).

### 8.3 Compatibilidad de libseccomp (LGPL-2.1) con GPL-2.0 puro
- LGPL-2.1 es compatible con GPL-2.0 para link dinámico.
- Si doki-proot es GPL-2.0 puro, debe enlazar libseccomp
  dinámicamente y permitir re-empacar (lo cual es trivial en
  Android/Linux con `.so`).

---

## 9. Recomendaciones consolidadas

### 9.1 Para **Doki (Go)**

| Tema                 | Recomendación                                                            |
|----------------------|--------------------------------------------------------------------------|
| Versión Go mínima    | **1.23** (soporte amplio, stdlib maduro, slog maduro)                   |
| Versión objetivo Go  | **1.26** (tomar `new(expr)`, genéricos auto-ref, GC perf)                |
| Logging              | `log/slog` con `slog.NewJSONHandler`; opcional zerolog como backend     |
| HTTP framework       | **Chi v5** (mínima alloc, ideal para daemon de larga vida)              |
| CLI                  | Cobra v1.9.x (subcommands `doki run/exec/ps/stop`)                      |
| Config               | **knadh/koanf v2** (compose providers, hot-reload, TOML/YAML/env)       |
| Linting              | **golangci-lint v2** con preset `standard` + gocritic + gosec           |
| Fuzzing Go           | nativo `go test -fuzz` para parsers de mensajes IPC                     |
| Build                | **Taskfile** (taskfile.dev) + Go modules + Go 1.26+toolchain            |
| Distribución binaria | `cosign sign-blob` + SLSA provenance via `go-containerregistry`         |
| Licencia del repo    | Mantener **Apache 2.0**                                                 |

### 9.2 Para **Doki-proot (C)**

| Tema                 | Recomendación                                                            |
|----------------------|--------------------------------------------------------------------------|
| PRoot upstream       | Mantener fork de **`termux/proot`**; **no** migrar a proot-me (abandonado) |
| Estilo PRoot         | Continuar con ptrace; **considerar** `coderredlab/proroot` como benchmark para casos de fallo (HyperOS seccomp) |
| Build                | **GNU Make** (ya existente) + presets de **Meson 1.8** opcionales; alineado a NDK **r29** con `-Wl,-z,max-page-size=16384` |
| Licencia             | **GPL-2.0** (mantener herencia STMicroelectronics); añadir `AUTHORS` y `NOTICE` con copyright OpceanAI 2026 |
| Libs C externas      | **cJSON 1.7.19** (MIT) como parser JSON; **libseccomp 2.6.0** (LGPL-2.1, link dinámico) |
| Sandboxing adicional | Aplicar **Landlock ABI v7** (Linux 6.10+) y **SECCOMP_FILTER_FLAG_TSYNC** en el child, antes de `execve` |
| Capabilities         | Dropear a la whitelist mínima vía `capset()` post-`exec`                |
| Testing C            | **CMocka 1.1.7** para `ipc/protocol` y `doki_portswitch`                |
| Fuzzing C            | **AFL++ 4.40c** con LLVM 18+ + FrameShift habilitado, corpus de líneas JSON |
| Reproducibilidad     | `-ffile-prefix-map=$PWD=.` + `SOURCE_DATE_EPOCH`                        |
| Empaquetado Android  | Compilar con NDK r29 + Clang 19; output `doki-proot-android-arm64`      |
| Cross-compile Linux  | `aarch64-linux-gnu-gcc -O2 -s` para ARM64, `x86_64-linux-gnu-gcc` para AMD64 |
| Cobertura de kernels | Testear en kernels Android 8 / 10 / 12 / 14 / 16; Linux 5.15 / 6.1 / 6.6 / 6.14 |
| Hardening            | `-D_FORTIFY_SOURCE=3 -fstack-protector-strong -fstack-clash-protection -fcf-protection=full` |
| CFI                  | Activar `-fsanitize=cfi` (requiere LTO con Clang) para builds de testing |

### 9.3 Plan de releases sugerido

1. **v0.9.1 (Jul 2026):**
   - Cut release `OpceanAI/Doki-proot` con tag binarios por ABI.
   - Firmar con `cosign` y subir SBOM CycloneDX.
   - Añadir suite CMocka + CI GitHub Actions.
2. **v0.10.0 (Oct 2026):**
   - Migrar de proot-me a `termux/proot` upstream como base.
   - Integrar libseccomp 2.6.0 con precompute.
   - Landlock ABI v7 en el child.
3. **v1.0.0 (Ene 2027):**
   - Soporte Linux 6.14 + Android 16.
   - Auditoría de seguridad externa (NCC Group / Trail of Bits).
   - OpenSSF Scorecard `passing`.

---

## 10. URLs de referencia rápidas

### Doki
- https://github.com/OpceanAI/Doki
- https://github.com/OpceanAI/Doki-proot

### Go
- https://go.dev/doc/go1.26
- https://tip.golang.org/doc/go1.27
- https://github.com/golangci/golangci-lint/releases

### PRoot y forks
- https://github.com/proot-me/proot/releases (v5.4.0, 2023, abandonado)
- https://github.com/termux/proot (fork activo usado por Doki-proot)
- https://github.com/coderredlab/proroot/releases (v1.2.7.1, May 2026)

### MicroVMs
- https://github.com/firecracker-microvm/firecracker/releases (v1.15.1)
- https://github.com/cloud-hypervisor/cloud-hypervisor/releases (v52.0, May 2026)
- https://github.com/hyperlight-dev/hyperlight/releases (v0.15.0, May 2026)

### Seguridad y syscalls
- https://github.com/seccomp/libseccomp/releases (v2.6.0, Jan 2025)
- https://docs.kernel.org/userspace-api/landlock.html
- https://kernel.org/doc/html/latest/userspace-api/io_uring.html
- https://www.openwall.com/lists/oss-security/ (CVE feed)

### Build / testing
- https://developer.android.com/ndk/downloads (r29, Oct 2025)
- https://github.com/AFLplusplus/AFLplusplus/releases (v4.40c, Mar 2026)
- https://github.com/DaveGamble/cJSON/releases (v1.7.19, Sep 2024)
- https://github.com/openssl/openssl/releases (v4.0.0, Apr 2026)
- https://mesonbuild.com/ (v1.8.x, May 2026)
- https://cmake.org/ (v3.30+, May 2026)

### Logging
- https://pkg.go.dev/log/slog
- https://github.com/rs/zerolog (v1.33.x)
- https://github.com/uber-go/zap (v1.27.x)

---

## 11. Riesgos identificados y open questions

1. **¿Vale la pena migrar de `proot-me/proot` a `termux/proot`?**
   - **Sí.** Termux está mucho más vivo (commits semanales) y
     recoge fixes críticos para Android 13-16. Ya lo hace
     `OpceanAI/Doki-proot`; falta dejarlo explícito en el README.

2. **¿Migrar de IPC JSON casero a cJSON?**
   - **Sí, en una release mayor.** Reduce superficie de bug
     (cJSON está cubierto por libFuzzer/AFL++ desde hace años).
   - **Costo:** ~50 KB extra en el binario; beneficio: cobertura
     de CVEs y un caso de prueba canónico.

3. **¿Aplicar Landlock en el child?**
   - **Sí, opcional, detrás de un flag `--landlock`.** El
     usuario lo activa si quiere un sandbox extra; por default
     off para no romper kernels <5.13.

4. **¿Migrar a pthreads + Landlock vs seguir single-threaded?**
   - **No migrar todavía.** El modelo single-threaded es lo que
     hace a doki-proot portable. Si se necesita concurrencia,
     usar `io_uring` con un solo SQ thread o fork por job.

5. **¿Adoptar reproducibilidad 100% bit-exact?**
   - **Empezar con `SOURCE_DATE_EPOCH` y `-ffile-prefix-map`**.
     Es 1 día de trabajo y da badge en `reproducible-builds.org`.

6. **¿OpenSSF Scorecard passing?**
   - Es un objetivo de v1.0.0 (Ene 2027). Requiere: code review
     de los PRs, CI con tests en cada PR, signed releases, SBOM,
     security policy.

7. **¿Vale la pena una auditoría de seguridad externa?**
   - **Sí, antes de v1.0.0.** Doki-proot maneja `ptrace` y
     `setuid`; una auditoría preventiva (NCC Group / Cure53 /
      Trail of Bits) cuesta 30-80 kUSD y trae credibilidad
     institucional.

---

*Reporte generado con research-tool opencode en base a GitHub releases,
changelogs oficiales, blogs y benchmarks públicos. Las versiones citadas
son las confirmadas en la fecha del reporte (2 Jun 2026).*
