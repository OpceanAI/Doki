# Niveles de aislamiento

Doki v0.9.2 soporta **12 niveles de aislamiento** — desde un sandbox WASM sin syscalls hasta microVMs a nivel de hardware. El registro de runners en `pkg/runtime/registry.go` prueba el host y elige el modo más fuerte que funcione. También puedes forzar un modo específico con `doki run --runtime <mode>`.

## Árbol de decisión

```
                         ┌─ pKVM / Microdroid   (Android 15+ VM protegida)
         ┌─ Hardware VM ─┤
         │               └─ MicroVM              (KVM / Gunyah / GenieZone / Halla)
         │
         ├─ Kernel ──────┬─ Sysbox               (DinD rootless)
         │               ├─ Namespaces           (default rootful)
         │               └─ gVisor               (defense-in-depth)
 Host ───┤
         ├─ Emulación ──┬─ FEX-Emu               (x86 en ARM)
         │               └─ QEMU User            (cross-arch)
         │
         ├─ Userspace ─── Proot                  (default en Android)
         │
         ├─ Compat ──────┬─ Legacy32             (ARMv7 en ARM64)
         │               └─ Chroot               (solo filesystem)
         │
         ├─ Sandbox ───── WASM                   (código no confiable)
         │
         └─ Ninguno ────── Native                (cero overhead)
```

## Tabla resumen

| Nivel | Modo | Aislamiento | Overhead | Caso de uso |
|:-----:|:-----|:----------|:---------|:-----------|
| 12 | WASM | Sandbox (user-space) | Mínimo | Código no confiable, serverless, plugins |
| 11 | pKVM/Microdroid | Hardware (vm) | 5-20 MB RAM | Cómputo sensible en teléfonos/Chromebooks |
| 10 | MicroVM | Hardware (vm) | 5-20 MB RAM | Seguridad de VM con velocidad de contenedor |
| 9 | Sysbox | Kernel (DinD) | Moderado | Docker-in-Docker, runners CI |
| 8 | Namespaces | Kernel | Insignificante | Multi-tenant confiable en servidores |
| 7 | gVisor | Kernel en user-space | ~20% CPU | Defense-in-depth sin VM |
| 6 | FEX-Emu | Emulación (x86→ARM) | ~30% CPU | x86 legacy en Apple Silicon |
| 5 | QEMU User | Emulación (cross-arch) | ~50% CPU | Contenedores cross-arch |
| 4 | Proot | Userspace (ptrace) | ~10% CPU | Default Android, sin root |
| 3 | Legacy32 | Compatibilidad dual-arch | Insignificante | Contenedores ARMv7 en ARM64 |
| 2 | Chroot | Filesystem | Mínimo | Testing rápido, etapas de build |
| 1 | Native | Ninguno | Cero | Carga confiable, fallback |

## Cobertura detallada

Cada nivel se implementa en `pkg/runtime/runners/<mode>/runner.go` (donde aplique) o directamente en `pkg/runtime/runtime.go` para los modos legacy.

### Nivel 12: WASM

**Qué es**: Corre módulos WASI (WebAssembly System Interface) usando `wasmedge` o `iwasm` como runtime. El módulo nunca hace un syscall real — todo I/O es mediado por el host WASM.

**Requisitos**:
- `wasmedge` o `iwasm` en `$PATH`
- Una imagen OCI con `mediatype: application/wasm` (o `doki run --runtime wasm` en cualquier imagen con un media type `wasm-oci`)

**Casos de uso**:
- Código de usuario no confiable (plugins, webhooks)
- Funciones serverless
- Microservicios políglotas
- Cargas sensibles a cold-start

**Rendimiento**: Overhead mínimo. Los módulos WASM compilan a código nativo al cargar. Cold start ~1-5ms.

**Trade-offs**:
- Superficie de syscall limitada (no hay `fork`, `execve` real, etc.)
- Algunas librerías (Go `os/exec`, Node `child_process`) no funcionan
- Networking requiere extensiones de socket WASI

**Referencia de código**: `pkg/runtime/runners/wasm/runner.go` (planeado — aún no conectado a `startProcess()`)

**Estado**: No testeado. La detección funciona (`which wasmedge`), runtime no validado en cargas de producción.

### Nivel 11: pKVM / Microdroid

**Qué es**: Protected Kernel-based Virtual Machine, el hipervisor de Google en Android 15+. El kernel host corre en EL1 (o Ring 0), los guests VM corren en un mundo protegido separado. La memoria está cifrada y aislada a nivel de hardware.

**Requisitos**:
- Dispositivo Android 15+ con kernel pKVM-capable (Tensor G3/G4, Snapdragon 8 Gen 3/4)
- `/dev/kvm` legible
- `microdroid` (el microVM init de Android) disponible — Doki lo incluye

**Casos de uso**:
- Cómputo sensible en móvil (datos de salud, financieros)
- Multi-tenant en ChromeOS
- Inferencia de IA aislada en dispositivos edge

**Rendimiento**: Casi nativo. ~5-20 MB RAM de overhead por guest. Tiempo de boot ~50ms.

**Trade-offs**:
- Solo disponible en hardware específico
- Requiere soporte del lado del kernel (algunos ROMs lo desactivan)
- Sin passthrough de GPU (planeado para v1.0)

**Referencia de código**: `pkg/runtime/runners/pkvm/runner.go` (planeado)

**Estado**: No testeado. La detección funciona, no hay hardware compatible disponible en CI.

### Nivel 10: MicroVM

**Qué es**: VMs ligeras vía crosvm (Chromium OS VMM) o Firecracker (AWS). Bootea en microsegundos, expone un modelo de dispositivo mínimo.

**Requisitos**:

| Chip | Hipervisor | VMM | Generación |
|:-----|:-----------|:----|:-----------|
| Qualcomm Snapdragon 8 Gen 1/2/3/4 | Gunyah | crosvm | 2022+ |
| MediaTek Dimensity 7200/8200/9200/9300 | GenieZone | crosvm | 2023+ |
| Samsung Exynos 2200/2400 | Halla | crosvm | 2022+ |
| Google Tensor G1/G2/G3/G4 | KVM | crosvm | 2021+ |
| Intel Core/Xeon | KVM | Firecracker | Todos los KVM-capable |
| AMD Ryzen/EPYC | KVM | Firecracker | Todos los KVM-capable |

**Casos de uso**:
- Serverless multi-tenant (Firecracker en AWS Lambda)
- Cómputo edge con aislamiento fuerte
- Entornos de desarrollo que necesitan un kernel Linux "real"

**Rendimiento**: 5-20 MB RAM de overhead. Tiempo de boot ~5-50ms. Throughput de I/O dentro del 5% del nativo.

**Trade-offs**:
- Más memoria que contenedores (cada guest necesita su propio kernel)
- El boot es más lento que contenedores (todavía <50ms con crosvm)
- Passthrough de dispositivos limitado

**Referencia de código**: `internal/dokivm/`

**Estado**: No testeado. La detección funciona.

### Nivel 9: Sysbox

**Qué es**: [Sysbox](https://github.com/nestybox/sysbox) es un "runc runtime" que mejora los contenedores OCI con soporte de namespaces anidados. Permite correr un daemon Docker completo dentro de un contenedor, con aislamiento apropiado de UTS/PID/IPC/Mount.

**Requisitos**:
- `sysbox-runc` en `$PATH` (binario separado de `runc`)
- Linux kernel 4.18+
- User namespaces habilitados

**Casos de uso**:
- Docker-in-Docker (runners CI, granjas de build)
- Kubernetes-in-Kubernetes
- CI/CD multi-stage con operaciones privilegiadas

**Rendimiento**: Casi nativo para la mayoría de cargas. ~5% overhead para operaciones de namespace anidadas.

**Trade-offs**:
- Añade un límite de seguridad que puede ser complicado de debuggear
- Algunas operaciones `ptrace` no funcionan cruzando el límite anidado
- Requiere sysbox-runc instalado por separado

**Referencia de código**: `pkg/runtime/runners/sysbox/runner.go` (planeado)

**Estado**: No testeado. La detección funciona.

### Nivel 8: Namespaces

**Qué es**: Namespaces estándar de Linux — UTS, PID, IPC, Mount, Net, User, Cgroup. Es lo que Docker/Podman usan por defecto en modo rootful.

**Requisitos**:
- Linux kernel 3.8+ (la mayoría de las distros modernas)
- Acceso root (o user namespaces para rootless)
- `/proc/self/ns/` accesible

**Casos de uso**:
- Cargas de servidores en producción
- Despliegues multi-tenant confiables
- Donde sea que tengas root y quieras aislamiento nativo de contenedores

**Rendimiento**: Overhead insignificante. <1% CPU, <0.5% memoria. El mejor de todos los modos a nivel de kernel.

**Trade-offs**:
- Requiere root (o setup de user namespace)
- Los exploits de kernel pueden escapar (CVE-2022-0185, CVE-2022-0492)
- No aísla recursos del kernel como `/proc`, `/sys`

**Referencia de código**: `pkg/runtime/runtime.go:startWithNamespaces()`

**Estado**: Testeado.

### Nivel 7: gVisor

**Qué es**: [gVisor](https://gvisor.dev/) de Google es un kernel en user-space. El runtime `runsc` intercepta syscalls en el contenedor y los re-implementa en Go. ~70% de los syscalls nunca llega al kernel host.

**Requisitos**:
- `runsc` en `$PATH`
- Linux kernel 4.14+
- Sin acceso a raw sockets (gVisor no soporta todos los tipos de socket)

**Casos de uso**:
- Multi-tenant con código no confiable
- Defense-in-depth (incluso si el kernel tiene una vulnerabilidad, gVisor la captura)
- Sandboxing de servicios third-party

**Rendimiento**: ~20% overhead de CPU. Overhead de memoria mínimo. Throughput de red ~70% del nativo.

**Trade-offs**:
- Algunos syscalls no implementados (raw sockets, ciertos ioctls)
- Tamaño de imagen mayor (gVisor distribuye su propio kernel basado en Go)
- No todas las aplicaciones funcionan (cualquier cosa que use `perf`, `eBPF` directamente)

**Referencia de código**: `pkg/runtime/runners/gvisor/runner.go` (planeado)

**Estado**: No testeado. La detección funciona.

### Nivel 6: FEX-Emu

**Qué es**: FEXInterpreter (o Box64) traduce binarios x86/x86_64 a ARM64 en runtime. El contenedor corre una imagen x86, FEX traduce cada instrucción al vuelo.

**Requisitos**:
- `FEXInterpreter` o `box64` en `$PATH`
- Host ARM64
- Imagen x86 o x86_64

**Casos de uso**:
- Correr contenedores x86 en Apple Silicon (Mac mini, MacBook)
- Aplicaciones x86 legacy en servidores ARM (Graviton, Ampere)
- Desarrollo cross-architecture

**Rendimiento**: ~30% overhead de CPU para cargas compute-bound. I/O casi nativo. Overhead de memoria ~20%.

**Trade-offs**:
- No funciona para operaciones a nivel de kernel (KPTI, vDSO)
- Algunas instrucciones AVX/AVX2 no se traducen
- Mayor footprint de memoria (caché de traducción)

**Referencia de código**: `pkg/runtime/runners/fex/runner.go` (planeado)

**Estado**: No testeado. La detección funciona.

### Nivel 5: QEMU User

**Qué es**: Emulación user-mode de QEMU. Corre binarios de una arquitectura diferente vía `qemu-aarch64-static`, `qemu-x86_64-static`, etc.

**Requisitos**:
- `qemu-<arch>-static` en `$PATH` (o binfmt_misc registrado)
- Cualquier arquitectura de host

**Casos de uso**:
- Desarrollo cross-architecture (build en x86, test en ARM)
- Correr contenedores ARMv7 en ARM64 (el caso canónico de "legacy32")
- Correr contenedores ARM en servidores x86 (raro pero soportado)

**Rendimiento**: ~50% overhead de CPU. El más lento de los modos emulados.

**Trade-offs**:
- Más lento que FEX-Emu para x86→ARM
- Sin aceleración KVM (user-mode, no system-mode)
- Algunas features específicas de Linux (ej. `prctl(PR_SET_NAME)`) funcionan diferente

**Referencia de código**: `pkg/runtime/runners/qemu/runner.go` (planeado)

**Estado**: No testeado. La detección funciona.

### Nivel 4: Proot

**Qué es**: [PRoot](https://proot-me.github.io/) es una implementación userspace de `chroot`/`mount` que usa `ptrace` para interceptar syscalls. No requiere root.

**Requisitos**:
- `proot` en `$PATH` (o fallback al `doki-proot` distribuido con Doki en v0.9.1; v0.9.2+ usa `FindProotBinary()`)
- Termux / Android / cualquier Linux sin root

**Casos de uso**:
- Runtime por defecto en Android/Termux
- Servidores Linux rootless
- Testing de contenedores sin acceso root

**Rendimiento**: ~10% overhead de CPU por ptrace. Overhead de memoria mínimo.

**Trade-offs**:
- Más lento que namespaces nativos
- No funciona para algunos syscalls (raw `mount`, `pivot_root`)
- Específico de Termux: `LD_PRELOAD` debe ser eliminado (v0.9.2+ lo maneja)

**Referencia de código**: `pkg/runtime/runtime.go:retryWithQemu()` (el fallback), `internal/proot/manager.go:FindProotBinary()`

**Estado**: Testeado en Termux/Android.

### Nivel 3: Legacy32

**Qué es**: Correr contenedores ARMv7 en kernels ARM64 vía `binfmt_misc` y soporte multiarch. El contenedor cree que está en un sistema ARMv7; el kernel es ARM64 con compatibilidad ARMv7.

**Requisitos**:
- Kernel host ARM64
- `binfmt_misc` registrado para ARMv7 (`update-binfmts --display qemu-arm`)
- `qemu-arm-static` (para paths sin binfmt)

**Casos de uso**:
- Correr contenedores ARM de 32 bits en servidores ARM de 64 bits
- Compatibilidad con imágenes viejas solo-32-bits
- Dispositivos edge con firmware de 32 bits

**Rendimiento**: Overhead insignificante cuando `binfmt_misc` está configurado. El kernel ARM64 maneja los syscalls ARMv7 nativamente.

**Trade-offs**:
- Sin addressing de memoria de 32 bits real (siempre 64-bit)
- Algunas operaciones solo-32-bits (ej. syscalls `OABI`) no soportadas
- Punteros de 32 bits en algunos casos edge

**Referencia de código**: `pkg/runtime/runners/legacy32/runner.go` (planeado)

**Estado**: No testeado. La detección funciona.

### Nivel 2: Chroot

**Qué es**: `chroot(2)` plano para aislamiento de filesystem. Sin namespace de PID, sin namespace de red, sin namespace de user. Solo cambia el directorio root.

**Requisitos**:
- Acceso root
- Eso es todo

**Casos de uso**:
- Aislamiento rápido de filesystem para tests
- Etapas de build en CI (ej. building de paquetes Debian)
- Cuando ningún otro modo funciona

**Rendimiento**: Overhead insignificante.

**Trade-offs**:
- Sin aislamiento real — el proceso puede escapar vía `/proc`
- Requiere root
- No apto para multi-tenant

**Referencia de código**: `pkg/runtime/runtime.go:startWithChroot()`

**Estado**: No testeado.

### Nivel 1: Native

**Qué es**: Sin aislamiento en absoluto. El contenedor es solo un directorio + variables de entorno. El proceso corre directamente en el host.

**Requisitos**: Ninguno. Siempre disponible.

**Casos de uso**:
- Cargas confiables
- Cuando quieres máximo rendimiento y no te importa el aislamiento
- Fallback cuando nada más funciona
- Modo CLI de macOS

**Rendimiento**: Cero overhead.

**Trade-offs**:
- Sin aislamiento. El proceso puede hacer cualquier cosa que el usuario host pueda.
- No usar para código no confiable.

**Referencia de código**: `pkg/runtime/runtime.go:startWithNative()`

**Estado**: Testeado.

## Forzando un modo

```bash
# Fuerza proot incluso si hay namespaces disponibles
doki run --runtime proot alpine echo hola

# Fuerza microVM (fallará si no hay /dev/kvm)
doki run --runtime microvm alpine echo hola

# Lista modos disponibles
doki info --format json | jq '.Isolations'
```

## Niveles futuros (planeados para v0.10+)

- **Landlock** (v0.10): sandboxing a nivel de kernel encima de cualquier otro modo, restringe acceso a filesystem
- **Aislamiento de io_uring** (v0.10): ring de io_uring por contenedor con set de opcodes restringido
- **GPU passthrough** (v0.10): para cargas de AI/ML en microVM
- **Computación confidencial** (v1.0): SEV-SNP / TDX en AMD/Intel, TrustZone en ARM

## Referencia

- Fuente: `pkg/runtime/registry.go`, `pkg/runtime/runners/*/`
- Lógica de decisión: `pkg/runtime/runtime.go:detectMode()`
- Fallback de proot: `pkg/runtime/runtime.go:retryWithQemu()`
- Auto-detección: `pkg/runtime/registry.go:hostPlatform()`
