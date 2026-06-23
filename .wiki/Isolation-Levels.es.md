# Niveles de aislamiento

<sub>[DOC: NIVELES-AISLAMIENTO]</sub>

Doki v0.11.0. Doce modos de runner. Desde un sandbox WASM sin syscalls hasta microVMs a nivel de hardware. El registro de runners en <kbd>pkg/runtime/registry.go</kbd> prueba el host y selecciona el modo mas fuerte que funcione. Se puede forzar un modo especifico con <kbd>doki run --runtime &lt;mode&gt;</kbd>.

<hr>

## Matriz de modos

<sub>[MATRIX]</sub>

Columnas: modo, root requerido, nivel de aislamiento, plataformas, requisitos de kernel, overhead.

```text
MODO        ROOT  AISLAMIENTO           PLATAFORMAS            KERNEL REQ     OVERHEAD
────────────────────────────────────────────────────────────────────────────────────────
wasm        no    sandbox user-space    cualquier runtime wasm n/a            minimo
pkvm        si    hardware vm           android 15+ (tensor/sd) pKVM cap      5-20 MB RAM
microvm    si    hardware vm           kvm/gunyah/geniezone   4.18+          5-20 MB RAM
sysbox      no    kernel DinD           linux 4.18+            user ns        moderado
namespaces  si    kernel                linux 3.8+             cualquiera     insignificante
gvisor      no    kernel user-space     linux 4.14+            cualquiera     ~20% CPU
fex         no    emulacion x86 a ARM64 host arm64             n/a            ~30% CPU
qemu-user   no    emulacion cross-arch  cualquiera con static  binfmt_misc    ~50% CPU
proot       no    userspace ptrace      termux/android/linux   n/a            ~10% CPU
legacy32   no    compat dual-arch      host arm64             binfmt_misc    insignificante
chroot      si    filesystem            cualquier unix         n/a            minimo
native      no    ninguno               cualquier              n/a            cero
```

<hr>

## Prioridad de seleccion automatica

<sub>[PRIORITY]</sub>

La deteccion corre en orden estricto. El primer modo cuyos requisitos se cumplen gana. El orden va de aislamiento mas fuerte a mas debil.

```text
PRIORIDAD  MODO        PROBE
─────────────────────────────────────────────────────────────
1          wasm        which wasmedge || which iwasm
2          pkvm        /dev/kvm legible Y kernel android 15+
3          microvm    /dev/kvm legible O nodo gunyah/geniezone
4          sysbox      which sysbox-runc
5          gvisor      which runsc
6          namespaces  uid == 0 Y /proc/self/ns accesible
7          fex         which FEXInterpreter OR which box64
8          qemu-user   which qemu-<arch>-static
9          proot       which proot OR fallback doki-proot
10         legacy32   uname -m == aarch64 Y binfmt_misc registrado
11         chroot      uid == 0
12         native      siempre disponible, ultimo recurso
```

Override con <kbd>--runtime</kbd>. El override salta la probe y fuerza el modo. Fallo al inicializar aborta el run.

```bash
doki run --runtime proot alpine echo hola
doki run --runtime microvm alpine echo hola
doki info --format json | jq '.Isolations'
```

<hr>

## WASM

<sub>[MODO 12]</sub>

Corre modulos WASI via <kbd>wasmedge</kbd> o <kbd>iwasm</kbd>. El modulo nunca hace un syscall real. Todo I/O es mediado por el host WASM. Cold start ~1-5ms. Requiere una imagen OCI con media type <kbd>application/wasm</kbd> o config <kbd>wasm-oci</kbd>. Superficie de syscall limitada. Sin <kbd>fork</kbd> o <kbd>execve</kbd> reales. Algunas librerias fallan. Networking requiere extensiones de socket WASI. Deteccion via <kbd>which wasmedge</kbd>. Runtime no validado en cargas de produccion. Codigo: <kbd>pkg/runtime/runners/wasm/runner.go</kbd> (planeado).

<hr>

## pKVM / Microdroid

<sub>[MODO 11]</sub>

Protected Kernel-based Virtual Machine. Hipervisor de Google en Android 15+. El kernel host corre en EL1. Los guests VM corren en un mundo protegido separado. Memoria cifrada y aislada a nivel de hardware. Requiere kernel pKVM-capable (Tensor G3/G4, Snapdragon 8 Gen 3/4), <kbd>/dev/kvm</kbd> legible, <kbd>microdroid</kbd> (Doki lo incluye). Boot time ~50ms. Overhead RAM 5-20 MB por guest. Sin passthrough de GPU (planeado v1.0). Deteccion funciona. Sin hardware compatible en CI. Codigo: <kbd>pkg/runtime/runners/pkvm/runner.go</kbd> (planeado).

<hr>

## MicroVM

<sub>[MODO 10]</sub>

VMs ligeras via crosvm (Chromium OS VMM) o Firecracker (AWS). Bootea en microsegundos. Modelo de dispositivo minimo. Requiere <kbd>/dev/kvm</kbd> legible o nodo vendor-specific (Gunyah/GenieZone/Halla). Overhead RAM 5-20 MB. Boot time 5-50ms. Throughput I/O dentro del 5% del nativo. Cada guest necesita su propio kernel. Passthrough de dispositivos limitado.

```text
CHIP                              HIPERVISOR  VMM         GEN
──────────────────────────────────────────────────────────────────
Qualcomm Snapdragon 8 Gen 1-4     Gunyah      crosvm      2022+
MediaTek Dimensity 7200-9300      GenieZone   crosvm      2023+
Samsung Exynos 2200/2400          Halla       crosvm      2022+
Google Tensor G1-G4               KVM         crosvm      2021+
Intel Core/Xeon                   KVM         Firecracker cualquiera kvm-capable
AMD Ryzen/EPYC                    KVM         Firecracker cualquiera kvm-capable
```

Codigo: <kbd>internal/dokivm/</kbd>. Deteccion funciona. No testeado.

<hr>

## Sysbox

<sub>[MODO 9]</sub>

Sysbox es un runtime compatible con runc que anade soporte de namespaces anidados a contenedores OCI. Permite correr un daemon Docker completo dentro de un contenedor con aislamiento apropiado de UTS/PID/IPC/Mount. Requiere <kbd>sysbox-runc</kbd> en <kbd>$PATH</kbd>, Linux kernel 4.18+, user namespaces habilitados. Rendimiento casi nativo. ~5% overhead para operaciones de namespace anidadas. Algunas operaciones <kbd>ptrace</kbd> fallan cruzando el limite anidado. Requiere sysbox-runc instalado por separado. Codigo: <kbd>pkg/runtime/runners/sysbox/runner.go</kbd> (planeado). Deteccion funciona. No testeado.

<hr>

## Namespaces

<sub>[MODO 8]</sub>

Namespaces estandar de Linux: UTS, PID, IPC, Mount, Net, User, Cgroup. El modo rootful por defecto que usan Docker y Podman. Requiere Linux kernel 3.8+, root o user namespaces, <kbd>/proc/self/ns/</kbd> accesible. Overhead insignificante: menos de 1% CPU, menos de 0.5% memoria. El mejor de los modos a nivel de kernel. Exploits de kernel pueden escapar (CVE-2022-0185, CVE-2022-0492). No aisla recursos del kernel como <kbd>/proc</kbd> y <kbd>/sys</kbd>. Codigo: <kbd>pkg/runtime/runtime.go:startWithNamespaces()</kbd>. Testeado.

<hr>

## gVisor

<sub>[MODO 7]</sub>

Kernel user-space de Google. El runtime <kbd>runsc</kbd> intercepta syscalls y los re-implementa en Go. ~70% de los syscalls nunca llega al kernel host. Requiere <kbd>runsc</kbd> en <kbd>$PATH</kbd>, Linux kernel 4.14+, sin acceso a raw sockets. ~20% overhead de CPU. Throughput de red ~70% del nativo. Algunos syscalls no implementados (raw sockets, ciertos ioctls). Tamano de imagen mayor. No todas las aplicaciones funcionan (cualquier cosa que use <kbd>perf</kbd> o <kbd>eBPF</kbd> directamente). Codigo: <kbd>pkg/runtime/runners/gvisor/runner.go</kbd> (planeado). Deteccion funciona. No testeado.

<hr>

## FEX-Emu

<sub>[MODO 6]</sub>

FEXInterpreter o Box64 traduce binarios x86/x86_64 a ARM64 en runtime. El contenedor corre una imagen x86. FEX traduce cada instruccion al vuelo. Requiere <kbd>FEXInterpreter</kbd> o <kbd>box64</kbd> en <kbd>$PATH</kbd>, host ARM64, imagen x86 o x86_64. ~30% overhead de CPU para cargas compute-bound. I/O casi nativo. Overhead de memoria ~20% por la cache de traduccion. No maneja operaciones a nivel de kernel (KPTI, vDSO). Algunas instrucciones AVX/AVX2 no se traducen. Codigo: <kbd>pkg/runtime/runners/fex/runner.go</kbd> (planeado). Deteccion funciona. No testeado.

<hr>

## QEMU User

<sub>[MODO 5]</sub>

Emulacion user-mode de QEMU. Corre binarios de una arquitectura diferente via <kbd>qemu-aarch64-static</kbd>, <kbd>qemu-x86_64-static</kbd>, etc. Requiere <kbd>qemu-&lt;arch&gt;-static</kbd> en <kbd>$PATH</kbd> o binfmt_misc registrado. Cualquier arquitectura de host. ~50% overhead de CPU. El mas lento de los modos emulados. Sin aceleracion KVM (user-mode, no system-mode). Algunas features especificas de Linux (ej. <kbd>prctl(PR_SET_NAME)</kbd>) funcionan diferente. Codigo: <kbd>pkg/runtime/runners/qemu/runner.go</kbd> (planeado). Deteccion funciona. No testeado.

<hr>

## Proot

<sub>[MODO 4]</sub>

PRoot es una implementacion userspace de <kbd>chroot</kbd>/<kbd>mount</kbd> que usa <kbd>ptrace</kbd> para interceptar syscalls. No requiere root. Requiere <kbd>proot</kbd> en <kbd>$PATH</kbd> o fallback al <kbd>doki-proot</kbd> distribuido con Doki (v0.9.2); v0.9.3+ usa <kbd>FindProotBinary()</kbd>. Runtime por defecto en Android/Termux. ~10% overhead de CPU por ptrace. Mas lento que namespaces nativos. Algunos syscalls fallan (raw <kbd>mount</kbd>, <kbd>pivot_root</kbd>). En Termux, <kbd>LD_PRELOAD</kbd> debe eliminarse (v0.9.3+ lo maneja). Codigo: <kbd>pkg/runtime/runtime.go:retryWithQemu()</kbd>, <kbd>internal/proot/manager.go:FindProotBinary()</kbd>. Testeado en Termux/Android.

<hr>

## Legacy32

<sub>[MODO 3]</sub>

Corre contenedores ARMv7 en kernels ARM64 via <kbd>binfmt_misc</kbd> y soporte multiarch. El contenedor cree que corre en ARMv7. El kernel es ARM64 con compatibilidad ARMv7. Requiere kernel host ARM64, <kbd>binfmt_misc</kbd> registrado para ARMv7, <kbd>qemu-arm-static</kbd> para paths sin binfmt. Overhead insignificante cuando <kbd>binfmt_misc</kbd> esta configurado. El kernel ARM64 maneja syscalls ARMv7 nativamente. Sin addressing de memoria 32-bit real (siempre 64-bit). Algunas operaciones solo-32-bits (syscalls OABI) no soportadas. Codigo: <kbd>pkg/runtime/runners/legacy32/runner.go</kbd> (planeado). Deteccion funciona. No testeado.

<hr>

## Chroot

<sub>[MODO 2]</sub>

<kbd>chroot(2)</kbd> plano para aislamiento de filesystem. Sin namespace de PID, sin namespace de red, sin namespace de user. Cambia el directorio root unicamente. Requiere root. Overhead insignificante. Sin aislamiento real. El proceso puede escapar via <kbd>/proc</kbd>. Requiere root. No apto para multi-tenant. Codigo: <kbd>pkg/runtime/runtime.go:startWithChroot()</kbd>. No testeado.

<hr>

## Native

<sub>[MODO 1]</sub>

Sin aislamiento. El contenedor es un directorio mas variables de entorno. El proceso corre directamente en el host. Sin requisitos. Siempre disponible. Cero overhead. Sin aislamiento. El proceso puede hacer cualquier cosa que el usuario host pueda. No usar para codigo no confiable. Fallback cuando nada mas funciona. Tambien el modo CLI de macOS. Codigo: <kbd>pkg/runtime/runtime.go:startWithNative()</kbd>. Testeado.

<hr>

## Niveles futuros

<sub>[FUTURE]</sub>

Planeados para v0.11.0+ y siguientes.

```text
NIVEL                 VERSION  DESCRIPCION
──────────────────────────────────────────────────────────────────────────
landlock              v0.11   sandbox kernel sobre cualquier modo, restringe fs
io_uring isolation    v0.11   ring por contenedor con set de opcodes restringido
gpu passthrough       v0.11   para cargas AI/ML en microVM
confidential compute  v1.0    SEV-SNP / TDX en AMD/Intel, TrustZone en ARM
```

<hr>

## Referencia

<sub>[SOURCE]</sub>

- Fuente: <kbd>pkg/runtime/registry.go</kbd>, <kbd>pkg/runtime/runners/*/</kbd>
- Logica de decision: <kbd>pkg/runtime/runtime.go:detectMode()</kbd>
- Fallback de proot: <kbd>pkg/runtime/runtime.go:retryWithQemu()</kbd>
- Auto-deteccion: <kbd>pkg/runtime/registry.go:hostPlatform()</kbd>