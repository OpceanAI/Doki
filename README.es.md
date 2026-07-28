# El Motor de Contenedores Universal

![Doki Banner](whaley.gif)

Contenedores sin root para donde Docker no puede llegar.

<p>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"></a>
  <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-doki--init-black?style=flat&logo=rust&logoColor=white" alt="Rust"></a>
  <a href="https://github.com/OpceanAI/Doki/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-555?style=flat" alt="License"></a>
  <a href="https://github.com/OpceanAI/Doki/releases"><img src="https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat&color=6366F1" alt="Downloads"></a>
  <a href="https://github.com/OpceanAI/Doki/stargazers"><img src="https://img.shields.io/github/stars/OpceanAI/Doki?style=flat&color=6366F1" alt="Stars"></a>
</p>

API compatible con Docker y Podman -- OCI nativo -- Kubernetes CRI-ready.
Funciona en Linux, macOS y Android via Termux -- ARM64, ARMv7, x86_64.
Arquitectura rootless-first -- Sin daemon requerido -- Aislamiento microVM a nivel hardware.

---

## Resumen

Doki es un motor de contenedores disenado para cada kernel Linux, desde telefonos Android hasta servidores en la nube. Funciona sin root, sin systemd y sin hypervisor. Cuando tu hardware ofrece mas -- KVM, hypervisors integrados de Android, Linux namespaces -- Doki escala su aislamiento automaticamente.

| Metrica | Valor |
|:-------|:------|
| **Version** | v0.12.0 |
| **Tamano binario** | 13 MB |
| **Memoria (idle)** | 12 MB |
| **Tiempo de inicio** | <15ms |
| **Plataformas** | Linux, macOS, Android (Termux) |
| **Arquitecturas** | ARM64, ARMv7, x86_64 |
| **Dependencias runtime** | Cero |

### Disponibilidad de Binarios por Plataforma (v0.12.0)

| Plataforma | doki | dokid | doki-compose | doki-init | doki-kube | doki-kubectl |
|:---------|:----:|:-----:|:------------:|:---------:|:---------:|:------------:|
| Linux ARM64 | Si | Si | Si | Si | Si | Si |
| Linux ARMv7 | Si | Si | Si | Si | Si | Si |
| Linux x86_64 | Si | Si | Si | Si | Si | Si |
| Android ARM64 (Termux) | Si | Si | Si | Si | Si | Si |
| Android ARMv7 (Termux) | Si | Si | Si | Si | Si | Si |
| macOS ARM64 (Apple Silicon) | Si | -- | -- | -- | Si | Si |
| macOS x86_64 (Intel) | Si | -- | -- | -- | Si | Si |

**Nota:** Los binarios Android ARMv7 se construyen con `GOOS=linux` (Go 1.22+ requiere enlazador externo para `GOOS=android` en ARM de 32 bits). Los binarios se ejecutan via proot; la deteccion de Android usa sondas de sistema de archivos.

`dokid`, `doki-compose` y `doki-init` son solo Linux/Android -- dependen de Linux namespaces, cgroups v2 y syscalls de overlayfs. En macOS, `doki` corre solo en `ModeNative` y se conecta a un daemon remoto por red si es necesario. `doki-kube` y `doki-kubectl` estan disponibles en macOS como binarios cliente.

---

## Comparacion

| Metrica | Doki | Docker | Podman | containerd |
|:-------|:----:|:------:|:------:|:----------:|
| Tamano binario | **13 MB** | 58 MB | 45 MB | 42 MB |
| Memoria (idle) | **12 MB** | 85 MB | 60 MB | 55 MB |
| Tiempo de inicio | **<15ms** | ~50ms | ~30ms | ~40ms |
| Soporte Android | **Si** | No | No | No |
| Root requerido | **No** | Si | Opcional | Si |
| Daemon requerido | **No** | Si | No | Si |
| Aislamiento microVM | **Si** | No | No | No |
| Cero dependencias | **Si** | No | No | No |

---

## Que Reemplaza Doki

| En lugar de | Usa Doki | Porque |
|:-----------|:---------|:--------|
| Docker Desktop | `dokid` + `doki` | Misma API, sin overhead de VM, funciona en Android |
| Podman | `dokid` + `doki` | APIs de Pod, secret y manifest en el mismo socket, mas aislamiento escalable |
| containerd + crictl | `dokid` como CRI | Un solo binario en vez de 3 daemons |
| Docker Compose | `doki-compose` | Mismo YAML, mismos comandos, mismo workflow |
| Kubernetes (deploys pequenos) | `doki kube play` | Ejecuta YAML de K8s sin un cluster |
| Lima / Colima (macOS) | `dokid` | Daemon de contenedores nativo, sin VM Linux necesaria |
| Termux proot-distro | `doki run` | Imagenes OCI reales en vez de tarballs de chroot |
| kubectl + minikube | `doki-kubectl` + `doki-kube` | Plano de control K8s en un solo binario; ejecuta tu YAML en un nodo |

---

## Caracteristicas

### Android Nativo

El unico motor de contenedores que corre en Android via Termux sin root. Disenado para las limitaciones de sistemas operativos moviles desde cero. Namespace de red del host via fallback de proot cuando `/proc/sys/net` no esta disponible.

### Rootless por Defecto

Funciona como usuario regular. Escala a root o aislamiento microVM cuando esta disponible. No se requiere escalacion de privilegios para operaciones basicas.

### Compatible con Docker

Habla la API REST de Docker Engine sobre el mismo socket Unix. Apunta `DOCKER_HOST` a `dokid` y el CLI de docker, docker-py, dockerode y docker-compose se conectan sin cambios -- los flujos comunes de contenedores, imagenes, redes, volumenes, exec y build estan implementados y probados.

### Ultra Ligero

Binario de 13MB, 12MB RAM idle. 4x mas pequeno que Docker, 7x menos memoria.

### 12 Niveles de Aislamiento

Desde sandboxes WASM hasta aislamiento hardware pKVM. Seleccionado automaticamente en runtime basado en el hardware disponible. Un modo para cada dispositivo: telefonos sin root, servidores con KVM, Chromebooks con pKVM, o laptops que necesitan emulacion x86 en ARM. Nuevo en v0.11: deteccion de pKVM/Microdroid y backend VZ de macOS.

### Soporte Compose

Spec completa de Compose: redes, volumenes, secrets, health checks, depends_on con polling de 60s, 30+ campos incluyendo shm_size, pids_limit, ulimits. El motor de ejecucion de healthcheck ejecuta sondas periodicas y reporta el estado de salud del contenedor de extremo a extremo.

### Compatible con OCI

Push y pull a cualquier registro OCI. Resolucion automatica de multi-arquitectura. Compatible con Docker Hub, GHCR, ECR, GCR, Quay, GitLab, Harbor.

### Plano de Control Kubernetes

Un plano de control Kubernetes en un solo binario: un apiserver con watch de recursos real, un scheduler que filtra y puntua nodos, controladores reconciliadores (Deployment a ReplicaSet a Pod, Job, Endpoints, Service, garbage collection), kube-proxy (modos iptables/nftables/userspace) y CoreDNS. `dokid` expone un runtime CRI gRPC funcional, asi que un kubelet real o crictl puede manejarlo y `doki kube play` ejecuta contenedores reales desde YAML de Kubernetes. CLI compatible con kubectl. Enfocado en nodo unico; el cableado propio de kubelet-over-CRI del plano de control y almacenamiento persistente aun estan en proceso.

### DokiLink Mesh

Red de contenedores multi-host peer-to-peer. NAT traversal via STUN (RFC 8489) con TCP simultaneous open hole punching y fallback de relay TURN. Descubrimiento de peers DHT (Kademlia, 160-bit, k=8, alpha=3). Descubrimiento LAN mDNS con expiracion de 90 segundos y loop de limpieza. Cifrado TLS 1.3 con opcion NaCl secretbox. Identidad Ed25519 con modelo de confianza TOFU.

### Podman API v5

39 endpoints compatibles con clientes podman-remote. Gestion de Pods, secrets y manifests. Montado junto a la API de Docker en el mismo socket con TLS, middleware y rate limiting compartidos.

### Virtualizacion Nativa de macOS

Backend VZ via bridge cgo a Virtualization.framework para macOS 11+. Backend QEMU como fallback en Macs Intel o donde VZ no esta disponible. Backend Sandbox para aislamiento ligero sin overhead de VM.

### Diagnostico

Herramienta `doki deps` para verificacion de dependencias del host con `ls`, `check` (gate de CI), `go` (deps de modulos Go) e `install` (best-effort via gestor de paquetes detectado). `doki doctor` para chequeos de salud del entorno.

### Emulacion Cross-Architecture

Ejecuta contenedores x86 en ARM y viceversa sin soporte de kernel. Tres backends: QEMU user-mode (`qemu-x86_64-static`, `qemu-aarch64-static`), FEX-Emu (x86-sobre-ARM, optimizado para Termux/Android) y Box64 (emulador x86_64 ligero). Configurable via `doki emulator ls|set|detect` o `DOKI_EMULATION_MODE=qemu|fex|box64|auto`. Preferencias persistentes almacenadas en `~/.doki/emulation.json` con escrituras atomicas y override por variable de entorno. La auto-deteccion selecciona el mejor backend disponible para tu arquitectura host.

---

## Inicio Rapido

### Instalacion

```bash
curl -sL https://dok1.xyz | sh
```

### Primera Ejecucion

```bash
# Iniciar el daemon en el socket Unix por defecto
dokid &

# Iniciar el daemon con path de socket explicito
dokid --host unix:///var/run/doki.sock &

# Iniciar el daemon con listener TCP (para acceso remoto)
dokid --host tcp://0.0.0.0:2375 &

# Pull y ejecutar
doki pull alpine
doki run alpine echo "Hello from Doki"

# Ver que esta corriendo
doki ps
doki images
```

### Usar con Docker CLI

```bash
export DOCKER_HOST=unix:///var/run/doki.sock
docker ps
docker images
docker run alpine echo "via docker cli"
docker-compose up
```

### Usar con Docker SDKs

```python
import docker
client = docker.DockerClient(base_url="unix:///var/run/doki.sock")
client.containers.run("alpine", "echo hello")
```

```javascript
const Docker = require('dockerode');
const docker = new Docker({ socketPath: '/var/run/doki.sock' });
docker.listContainers().then(console.log);
```

### Usar con Kubernetes

```bash
# Iniciar el plano de control K8s
doki kube play my-app.yaml

# Gestionar con CLI compatible con kubectl
doki-kubectl get pods
doki-kubectl apply -f deployment.yaml
doki-kubectl describe pod web-abc123
doki-kubectl logs web-abc123
```

---

## Binarios

| Binario | Tamano | Descripcion |
|:-------|:----:|:------------|
| **doki** | 6.7 MB | CLI con 108+ comandos. Se conecta al daemon via socket Unix |
| **dokid** | 9.2 MB | Daemon. Docker Engine API v1.54 + Podman API v5 sobre socket Unix |
| **doki-compose** | 7.6 MB | Motor Compose con watch, publish, ejecucion de healthcheck y soporte completo de spec |
| **doki-init** | 2.9 MB | PID 1 para guests microVM (Go). Variante Rust disponible en el fuente |
| **doki-kube** | 8.1 MB | Plano de control Kubernetes (apiserver, kubelet, scheduler, controllers, kube-proxy, CoreDNS) |
| **doki-kubectl** | 4.3 MB | CLI compatible con kubectl para gestionar recursos Kubernetes |

---

## Arquitectura

### Pipeline

Cuando Doki ejecuta un contenedor, pasa por este pipeline:

1. **Resolucion de Imagen** -- Parsear referencia, contactar registro, autenticar, resolver manifiesto para la arquitectura actual, descargar capas
2. **Construccion de Rootfs** -- Extraer capas en orden, construir filesystem completo del contenedor con proteccion contra path traversal
3. **Seleccion de Modo de Ejecucion** -- Sondar sistema y seleccionar el mejor runner de 12 modos disponibles: WASM, pKVM, microVM, sysbox, namespaces, gVisor, FEX, QEMU, proot, legacy32, chroot o native
4. **Ejecucion de Proceso** -- Ejecutar comando del contenedor dentro del contexto de aislamiento elegido con variables de entorno aplicadas
5. **Gestion de Ciclo de Vida** -- Monitorear proceso, registrar codigos de salida, escribir logs, ejecutar health checks, aplicar politicas de reinicio

### Niveles de Aislamiento

Doki selecciona el modo de aislamiento mas fuerte disponible en tu hardware. Cada modo existe para un caso de uso especifico:

| Nivel | Modo | Aislamiento | Overhead | Por que / Cuando |
|:-----:|:-----|:----------|:---------|:-----------|
| **12** | WASM | Sandbox (user-space) | Minimo | Ejecutar contenedores WASI/Wasm para codigo no confiable. Ningun syscall llega al host. Usar para plugins, funciones serverless o microservicios poliglotas |
| **11** | pKVM/Microdroid | Nivel hardware (vm) | 5-20 MB RAM | VM protegida Android 15+. pKVM de Google aisla cargas de trabajo del host OS y entre si. Usar para computo sensible en Chromebooks/telefonos |
| **10** | MicroVM | Nivel hardware (vm) | 5-20 MB RAM | Hypervisors KVM, Gunyah, GenieZone, Halla. Aislamiento hardware completo con boot de microsegundos. Usar cuando necesitas seguridad nivel VM con velocidad de contenedor |
| **9** | Sysbox | Nivel kernel (DinD) | Moderado | Docker-in-Docker sin root via sysbox-runc. Usar cuando necesitas correr un daemon Docker completo dentro de un contenedor (CI runners, granjas de build) |
| **8** | Namespaces | Nivel kernel | Insignificante | Aislamiento estandar de Linux namespaces. Usar en servidores con acceso root. Mejor rendimiento para cargas multi-tenant confiables |
| **7** | gVisor | Kernel user-space | ~20% CPU | runsc de Google intercepta syscalls en el limite del user-space. Usar cuando quieres defensa en profundidad sin una VM -- 70% de los syscalls nunca llegan al host |
| **6** | FEX-Emu | Emulacion (x86 sobre ARM) | ~30% CPU | FEXInterpreter o Box64. Ejecuta binarios x86/x86_64 en ARM64 sin recompilacion. Usar para contenedores x86 legacy en Apple Silicon o servidores ARM |
| **5** | QEMU User | Emulacion (cross-arch) | ~50% CPU | QEMU user-mode para cualquier arch guest. Usar cuando necesitas ejecutar contenedores construidos para una arquitectura diferente (ej. arm32 en arm64, o cualquier arch en cualquier arch) |
| **4** | Proot | Userspace (ptrace) | ~10% CPU | Chroot basado en ptrace sin root. Por defecto en Android/Termux. Usar en dispositivos donde no tienes root ni namespaces -- telefonos, tablets, ChromeOS Linux |
| **3** | Legacy32 | Compat dual-arch | Insignificante | Ejecutar contenedores ARMv7 en kernels ARM64 via binfmt_misc y soporte multiarch. Usar cuando tu carga solo viene como ARM de 32 bits |
| **2** | Chroot | Nivel filesystem | Minimo | Aislamiento ligero de filesystem via chroot. Usar para pruebas rapidas, etapas de build, o cuando todos los demas modos no estan disponibles |
| **1** | Native | Ninguno | Cero | Ejecucion directa en el host. Siempre disponible como fallback. Usar cuando confias en la carga y quieres cero overhead |

### Deteccion de Niveles de Aislamiento

El registro de runners en `pkg/runtime/registry.go` sondar el host y selecciona el modo mas fuerte que funciona. Orden de sondaje (de arriba a abajo, el primero que pase gana):

| Nivel | Modo | Sonda de deteccion |
|:-----:|:-----|:----------------|
| 12 | WASM | `which wasmedge` o `which iwasm` |
| 11 | pKVM/Microdroid | `/dev/kvm` legible + Android 15+ |
| 10 | MicroVM | `/dev/kvm` legible + `crosvm`/`firecracker` en `$PATH` |
| 9 | Sysbox | `sysbox-runc` en `$PATH` |
| 8 | Namespaces | `unshare --user --map-root-user true` sale con 0 |
| 7 | gVisor | `runsc` en `$PATH` |
| 6 | FEX-Emu | `FEXInterpreter` o `box64` en `$PATH` |
| 5 | QEMU User | `qemu-aarch64-static` / `qemu-x86_64-static` etc. en `$PATH` |
| 4 | Proot | `proot` en `$PATH` (o incluido) |
| 3 | Legacy32 | `binfmt_misc` registrado + qemu multiarch |
| 2 | Chroot | siempre (usa `chroot(2)`) |
| 1 | Native | siempre (sin aislamiento) |

Override con `doki run --runtime <modo>`:

```bash
doki run --runtime proot alpine echo "always proot"
doki run --runtime native alpine echo "no isolation"
doki run --runtime wasm wasi-example.wasm
```

### Soporte MicroVM

DokiVM provee aislamiento a nivel hardware via maquinas virtuales ligeras.

| Fabricante | Serie de Chip | Hypervisor | VMM | Generacion |
|:-------------|:------------|:-----------|:----|:-----------|
| Qualcomm | Snapdragon 8 Gen 1/2/3/4 | Gunyah | crosvm | 2022+ |
| MediaTek | Dimensity 7200/8200/9200/9300 | GenieZone | crosvm | 2023+ |
| Samsung | Exynos 2200/2400 | Halla | crosvm | 2022+ |
| Google | Tensor G1/G2/G3/G4 | KVM | crosvm | 2021+ |
| Intel | Core / Xeon | KVM | Firecracker | Todos con KVM |
| AMD | Ryzen / EPYC | KVM | Firecracker | Todos con KVM |

### Virtualizacion macOS

En macOS, Doki provee tres backends de VM:

| Backend | Tecnologia | Requisitos | Mejor Para |
|:--------|:-----------|:-------------|:---------|
| **VZ** | Virtualization.framework via bridge cgo/ObjC | macOS 11+, Apple Silicon | Rendimiento nativo, soporte Rosetta, overhead minimo |
| **QEMU** | QEMU con acelerador HVF | macOS 10.15+, Intel o Apple Silicon | Fallback cuando VZ no disponible, emulacion x86 en ARM |
| **Sandbox** | Perfil sandbox-exec de macOS | macOS 10.7+ | Aislamiento ligero sin overhead completo de VM |

El backend VZ usa `VZVirtualMachineConfiguration`, `VZLinuxBootLoader`, `VZVirtioFileSystemDevice` con directorios compartidos, y `VZBridgedNetworkDevice`/`VZNATNetworkDevice`. Build tag `darwin && cgo`, `CGO_ENABLED=1` requerido.

---

## CLI

Doki provee **108 comandos** en 8 categorias.

### Gestion de Contenedores

| Comando | Descripcion |
|:--------|:------------|
| `doki run` | Crear e iniciar un contenedor (80+ flags) |
| `doki ps` | Listar contenedores |
| `doki create` | Crear sin iniciar |
| `doki start` | Iniciar contenedores detenidos |
| `doki stop` | Detener contenedores gracefulmente |
| `doki restart` | Detener e iniciar contenedores |
| `doki kill` | Enviar senal a contenedores |
| `doki rm` | Eliminar contenedores |
| `doki exec` | Ejecutar comando en contenedor en ejecucion |
| `doki logs` | Obtener logs del contenedor (soporte streaming) |
| `doki stats` | Estadisticas de recursos en vivo |
| `doki top` | Mostrar procesos del contenedor |
| `doki inspect` | Informacion detallada del contenedor |
| `doki build` | Construir imagen desde Dokifile |
| `doki commit` | Crear imagen desde contenedor |
| `doki attach` | Adjuntar a I/O del contenedor |
| `doki wait` | Bloquear hasta salida, retornar codigo |
| `doki cp` | Copiar archivos entre host y contenedor |

### Gestion de Imagenes

| Comando | Descripcion |
|:--------|:------------|
| `doki pull` | Pull desde cualquier registro OCI (resolucion automatica multi-arch) |
| `doki push` | Push a cualquier registro OCI |
| `doki images` | Listar imagenes con tamanos |
| `doki rmi` | Eliminar imagenes |
| `doki tag` | Etiquetar una imagen |
| `doki build` | Construir desde Dokifile (18 instrucciones, multi-stage) |
| `doki login` / `doki logout` | Autenticacion de registro |
| `doki search` | Buscar en Docker Hub |

### Red, Volumen, Sistema

| Red | Volumen | Sistema |
|:--------|:-------|:-------|
| `doki network ls` | `doki volume ls` | `doki info` |
| `doki network create` | `doki volume create` | `doki version` |
| `doki network rm` | `doki volume rm` | `doki system df` |
| `doki network inspect` | `doki volume inspect` | `doki system prune` |
| `doki network connect` | `doki volume prune` | `doki system events` |
| `doki network disconnect` | | `doki ping` |
| `doki network prune` | | |

### Podman y Kubernetes

| Podman | Kubernetes |
|:-------|:-----------|
| `doki pod create/ps/rm/start/stop` | `doki kube play` |
| `doki generate kube` | `doki kube down` |
| `doki play kube` | `doki kube generate` |
| `doki auto-update` | `doki apply -f` |
| `doki unshare` / `untag` | |
| `doki mount` / `unmount` | |
| `doki healthcheck` | |

### DokiLink Mesh

| Comando | Descripcion |
|:--------|:------------|
| `doki mesh status` | Mostrar ID de instalacion y clave publica Ed25519 |
| `doki mesh ls` | Listar peers conocidos |
| `doki link add <name> <addr> --pub <key>` | Agregar un peer estatico |
| `doki link rm <name>` | Eliminar un peer estatico |

### Diagnostico

| Comando | Descripcion |
|:--------|:------------|
| `doki doctor` | Verificar entorno del host y dependencias |
| `doki deps ls` | Listar todas las dependencias del sistema con estado |
| `doki deps check` | Gate de CI: salir non-zero si faltan deps requeridas |
| `doki deps go` | Auditar dependencias de modulos Go |
| `doki deps install <name>` | Instalacion best-effort via gestor de paquetes detectado |

### Emulacion Cross-Architecture

| Comando | Descripcion |
|:--------|:------------|
| `doki emu show` | Mostrar preferencia de emulador guardada y path de config |
| `doki emu detect` | Escanear PATH por backends QEMU/FEX/Box64 con versiones |
| `doki emu test` | Ejecutar deteccion y preguntar antes de guardar recomendacion |
| `doki emu set <mode>` | Establecer preferencia: `auto`, `qemu`, `fex`, `box64` |

El sistema de emulacion persiste preferencias en `~/.doki/emulation.json` con escrituras atomicas (tmp+rename, permisos 0600). Dos variables de entorno overridean la config en disco: `DOKI_EMULATION_MODE` y `DOKI_EMULATOR` (alias). La auto-deteccion prefiere FEX-Emu en hosts ARM64, luego Box64, luego QEMU user-mode como fallback universal. Imagenes de contenedores con arquitecturas foreignas (ej. `linux/amd64` en un host ARM64) se rutean automaticamente a traves del emulador seleccionado por el registro de runners.

---

## Constructor Dokifile

Doki lee Dokifiles (o Dockerfiles estandar) y construye imagenes compatibles con OCI. El parser soporta las 18 instrucciones de Dockerfile, builds multi-stage, heredocs y directivas de parser.

### Instrucciones Soportadas

```
FROM      RUN       CMD       LABEL     EXPOSE    ENV
ADD       COPY      ENTRYPOINT VOLUME   USER      WORKDIR
ARG       ONBUILD   STOPSIGNAL HEALTHCHECK SHELL  MAINTAINER
```

### Ejemplo

```dockerfile
FROM alpine:latest AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY . .
RUN gcc -static -o app main.c

FROM alpine:latest
COPY --from=builder /build/app /usr/local/bin/app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q --spider http://localhost:8080/ || exit 1
USER nobody
CMD ["/usr/local/bin/app"]
```

---

## Compose

Soporte completo de Compose Specification para aplicaciones multi-contenedor.

### Caracteristicas Soportadas

| Caracteristica | Descripcion |
|:--------|:------------|
| `services` | Definiciones de contenedores con configuracion completa |
| `networks` | Redes bridge/overlay personalizadas |
| `volumes` | Almacenamiento persistente con opciones de driver |
| `secrets` | Inyeccion de datos sensibles con sintaxis larga |
| `depends_on` | Orden de inicio: `service_started`, `service_healthy` (polling 60s), `service_completed_successfully` |
| `healthcheck` | Sondas de salud por servicio con motor de ejecucion periodico real |
| `deploy` | Limites de recursos (`cpus`, `memory`), `replicas`, `restart_policy` |
| `profiles` | Activacion condicional de servicios |
| `extends` | Herencia de servicios |
| `include` | Composicion multi-archivo |
| `watch` | Monitoreo de archivos via fsnotify para hot-reload durante desarrollo |
| `publish` | Integracion de service mesh para deployments basados en compose |
| Sintaxis larga | Puertos, volumenes, dispositivos, blkio_config, ulimits |

### Ejemplo

```yaml
name: production-stack

services:
  web:
    image: nginx:alpine
    ports: ["80:80", "443:443"]
    volumes:
      - web-data:/usr/share/nginx/html
    depends_on:
      api:
        condition: service_healthy
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 256M
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 10s
      retries: 3

  api:
    image: python:3-alpine
    command: uvicorn main:app --host 0.0.0.0
    environment:
      DATABASE_URL: postgresql://user:pass@db:5432/app
    depends_on:
      db:
        condition: service_started

  db:
    image: postgres:alpine
    volumes:
      - db-data:/var/lib/postgresql/data
    secrets:
      - db-password
```

---

## REST API

Doki expone la **Docker Engine API v1.54** y **Podman libpod API v5** sobre sockets Unix. Ambas APIs comparten el mismo servidor, heredando TLS, middleware y rate limiting.

### Docker Engine API -- Contenedores (16 endpoints)

| Metodo | Path | Descripcion |
|:-------|:-----|:------------|
| `GET` | `/containers/json` | Listar contenedores |
| `POST` | `/containers/create` | Crear contenedor |
| `GET` | `/containers/{id}/json` | Inspeccionar contenedor |
| `POST` | `/containers/{id}/start` | Iniciar contenedor |
| `POST` | `/containers/{id}/stop` | Detener contenedor |
| `POST` | `/containers/{id}/restart` | Reiniciar contenedor |
| `POST` | `/containers/{id}/kill` | Matar contenedor |
| `DELETE` | `/containers/{id}` | Eliminar contenedor |
| `GET` | `/containers/{id}/logs` | Obtener logs |
| `POST` | `/containers/{id}/exec` | Crear instancia exec |
| `POST` | `/containers/{id}/attach` | Adjuntar a contenedor |
| `POST` | `/containers/prune` | Eliminar contenedores detenidos |

### Docker Engine API -- Imagenes (7 endpoints)

| Metodo | Path | Descripcion |
|:-------|:-----|:------------|
| `GET` | `/images/json` | Listar imagenes |
| `POST` | `/images/create` | Pull imagen |
| `GET` | `/images/{name}/json` | Inspeccionar imagen |
| `POST` | `/images/{name}/push` | Push imagen |
| `DELETE` | `/images/{name}` | Eliminar imagen |
| `POST` | `/images/prune` | Eliminar imagenes no usadas |
| `GET` | `/images/search` | Buscar en registro |

### Docker Engine API -- Sistema y Otros

| Metodo | Path | Descripcion |
|:-------|:-----|:------------|
| `GET` | `/info` | Informacion del sistema |
| `GET` | `/version` | Informacion de version |
| `GET` | `/_ping` | Health check |
| `GET` | `/events` | Stream de eventos |
| `GET` | `/system/df` | Uso de disco |
| `GET` | `/metrics` | Metricas Prometheus |
| `GET` | `/health` | Salud del daemon |
| `POST` | `/auth` | Autenticacion |

### Podman API (39 endpoints)

| Categoria | Endpoints |
|:---------|:----------|
| **Pods** | `/libpod/pods/create`, `/libpod/pods/json`, `/libpod/pods/{id}/json`, `/libpod/pods/{id}/start`, `/libpod/pods/{id}/stop`, `/libpod/pods/{id}/restart`, `/libpod/pods/{id}/kill`, `/libpod/pods/{id}/pause`, `/libpod/pods/{id}/unpause`, `/libpod/pods/{id}/exists`, `/libpod/pods/{id}`, `/libpod/pods/prune` |
| **Secrets** | `/libpod/secrets/create`, `/libpod/secrets/json`, `/libpod/secrets/{id}/json`, `/libpod/secrets/{id}` |
| **Manifests** | `/libpod/manifests/create`, `/libpod/manifests/{name}/add`, `/libpod/manifests/{name}/remove`, `/libpod/manifests/{name}/json`, `/libpod/manifests/{name}/push`, `/libpod/manifests/json` |

### Kubernetes API

| Metodo | Path | Descripcion |
|:-------|:-----|:------------|
| `GET` | `/api/v1/pods` | Listar pods |
| `POST` | `/api/v1/pods` | Crear pod |
| `GET` | `/api/v1/services` | Listar services |
| `POST` | `/api/v1/services` | Crear service |
| `GET` | `/apis/apps/v1/deployments` | Listar deployments |
| `POST` | `/apis/apps/v1/deployments` | Crear deployment |
| `GET` | `/version` | Info de version del servidor |

Paths completos de grupos API: `api/v1`, `apis/apps/v1`, `apis/batch/v1`, `networking.k8s.io/v1`, `rbac.authorization.k8s.io/v1`.

### CRI gRPC (41 RPCs)

El plugin CRI implementa la interfaz completa de Kubernetes Container Runtime Interface:

| Servicio | RPCs | Descripcion |
|:--------|:-----|:------------|
| RuntimeService | 35 | RunPodSandbox, StopPodSandbox, RemovePodSandbox, PodSandboxStatus, ListPodSandbox, CreateContainer, StartContainer, StopContainer, RemoveContainer, ListContainers, ContainerStatus, UpdateContainerResources, ExecSync, Exec, Attach, PortForward y mas |
| ImageService | 6 | ListImages, ImageStatus, PullImage, RemoveImage, ImageFsInfo |

---

## Networking

### Tipos de Red

| Tipo | Descripcion |
|:-----|:------------|
| **Bridge** | Bridge `doki0` por defecto con NAT, resolucion DNS, mapeo de puertos |
| **Host** | Compartir namespace de red del host (maximo rendimiento). En Termux/Android, fallback a red del host via proot cuando `/proc/sys/net` no esta disponible |
| **None** | Solo loopback (aislamiento completo) |
| **CNI** | bridge, host-local, portmap, macvlan, ipvlan, dhcp, vlan |
| **Rootless** | Usa **pasta** para TCP/UDP sin root ni dispositivos TAP |
| **IPv6** | Dual-stack IPv4/IPv6 en redes bridge |

### Mapeo de Puertos

```bash
doki run -p 8080:80 nginx:alpine                    # Mapear host 8080 a contenedor 80
doki run -p 127.0.0.1:8080:80 nginx:alpine          # Bind a IP especifica
doki run -p 8080:80/tcp -p 8080:80/udp              # TCP y UDP
doki run -P nginx:alpine                            # Publicar todos los puertos EXPOSEd
doki run -p 8080-8090:80 nginx:alpine               # Rango de puertos
```

### Networking Especifico de Termux

En Android/Termux, el namespace de red del host es inaccesible para procesos no privilegiados. Doki detecta esto al inicio y:
- Hace fallback a modo de red `host` via proot, compartiendo el namespace de red de la app Termux
- DNS escucha en `127.0.0.11:8053` (puerto 53 bloqueado por SELinux)
- Resolvedores DNS upstream se leen de `getprop net.dns1..net.dns4`
- Mapeo de puertos usa socat en modo rootless (iptables no disponible)

Override de direccion de escucha DNS con `DOKI_DNS_LISTEN=IP:PORT` o `dns.listen` en `config.json`.

### Internos de Port Forwarding

El mapeo de puertos usa iptables DNAT en modo root y `socat` en modo rootless:

- Reglas DNAT usan `[]string` (sin parsing de shell) e incluyen la cadena `-A OUTPUT` para trafico local
- Forwards de socat rootless apuntan al IP bridge del contenedor directamente (no localhost)
- Pares veth rastreados via `Endpoint.VethHost`/`Endpoint.VethPeer` para teardown idempotente
- Tear down elimina ambos extremos veth via `ip link del` antes de eliminar el bridge

---

## DokiLink Mesh

DokiLink provee networking multi-host sin broker central. Los peers se descubren via mDNS (LAN), DHT (internet) o configuracion estatica. Todo el trafico se autentica via firmas Ed25519 y se cifra con TLS 1.3 o NaCl secretbox.

### NAT Traversal

El NAT traversal sigue una secuencia de cuatro etapas:

1. **STUN**: Ambos peers consultan un servidor STUN (RFC 8489) para descubrir su IP publica y mapeo de puertos
2. **Intercambio**: Los peers intercambian direcciones publicas via el protocolo gossip
3. **Hole Punching**: Ambos peers emiten paquetes TCP SYN simultaneos a la direccion publica del otro usando `TCPConn.SetDeadline` con timing coordinado
4. **Fallback**: Si el hole punching falla (NAT simetrico), el trafico se rutea a traves de un peer relay actuando como proxy TURN

### Descubrimiento de Peers DHT

Kademlia DHT con IDs de nodo de 160 bits provee descubrimiento de peers descentralizado:

| Parametro | Valor | Descripcion |
|:----------|:------|:------------|
| ID de Nodo | 160-bit | Hash SHA-1 de clave publica Ed25519 |
| k-buckets | k=8 | Maximo de peers por bucket de routing |
| Paralelismo | alpha=3 | Lookups concurrentes durante FIND_NODE |
| RPCs | PING, STORE, FIND_NODE, FIND_VALUE | Operaciones estandar de Kademlia |

### Descubrimiento mDNS

Descubrimiento de peers LAN via mDNS (multicast DNS):

- Los peers se anuncian via registros TXT `_doki-link._tcp.local`
- Las entradas expiran despues de 90 segundos si no se refrescan
- Loop de limpieza en background corre cada 30 segundos
- Auto-filtrado por ID de instalacion previene auto-descubrimiento
- Registros TXT anuncian `common.DokiVersion` para compatibilidad de version

### Capas de Cifrado

| Capa | Cifrado | Descripcion |
|:------|:-----------|:------------|
| **L0** | Ninguno | Solo loopback -- por defecto en Android/Termux |
| **L1** | TLS 1.3 | Por defecto, firmado por ECDSA P-256 CA por instalacion |
| **L2** | NaCl secretbox | Opt-in con `DOKI_LINK_PAYLOAD_ENC=1`, clave derivada de las claves publicas Ed25519 de ambos peers |

La derivacion de clave es independiente del orden (ambos peers computan la misma clave compartida via SHA-256 de pubkeys ordenadas). Nonces por conexion se siembran desde `crypto/rand`. Proteccion contra replay usa ventana de timestamp de 5 minutos con cache LRU de nonces (1024 entradas).

### Uso

```bash
# Mostrar ID de instalacion local y clave publica
doki mesh status

# Agregar un peer estatico
doki link add mybuddy 192.168.1.42:7432 \
  --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# Listar peers conocidos
doki mesh ls

# Publicar un contenedor alcanzable a traves del mesh
doki run -d -p 0.0.0.0:9090:80 --name web nginx:alpine
```

---

## DNS

Doki corre un servidor DNS interno que maneja resolucion de nombres entre contenedores y reenvia consultas externas a resolvedores upstream.

### Arquitectura

Los contenedores apuntan `/etc/resolv.conf` a `nameserver 127.0.0.11`. El servidor DNS interno de Doki resuelve nombres de contenedores locales a IPs del bridge y reenvia consultas externas upstream.

### Valores por Defecto

| Plataforma | Escucha por defecto | Por que |
|:---------|:----------------|:----|
| Linux | `127.0.0.11:53` | Puerto estandar no privilegiado |
| Android (Termux) | `127.0.0.11:8053` | Puerto 53 bloqueado por SELinux (EACCES) en non-root |
| macOS | no usado (ModeNative) | Sin red bridge |

### Resolucion de Nombres de Contenedores

```bash
$ doki network create backend
$ doki run -d --name db --network backend postgres:alpine
$ doki run -d --name api --network backend my-api:latest
$ doki exec api sh -c 'getent hosts db'
172.20.0.2      db.backend
```

### Comportamientos Clave

- **AAAA + PTR**: Lookups IPv6 forward y reverse funcionan junto a registros A
- **Registros SRV**: Soporte de protocolo de descubrimiento de servicios para `_<port>._tcp.<svc>.<ns>.svc.cluster.local`
- **ndots:0**: Nombres de contenedores como `forgejo` se resuelven directamente, sin loop de retry `forgejo.local`
- **Retry TCP**: Cuando UDP upstream retorna bit TC, la consulta se reintenta sobre TCP segun RFC 5966
- **sin busy-wait**: `ReadFromUDP` bloquea en el socket, sin loop de polling
- **Cache LRU**: 1024 entradas, TTL de 5 minutos, auto-registro al inicio del contenedor, re-registro al reinicio del daemon

---

## Almacenamiento

| Driver | Descripcion | Mejor para |
|:-------|:------------|:---------|
| **overlay2** | Overlay de kernel (mount directo via syscall) | Linux con root, mejor rendimiento |
| **fuse-overlayfs** | Overlay en userspace via FUSE | Rootless, Termux, Android |
| **btrfs** | Subvolumenes Btrfs con snapshots | Sistemas con root btrfs |
| **zfs** | Datasets ZFS con snapshots | Sistemas con pools ZFS |
| **vfs** | Copia simple de directorio | Pruebas, sistemas minimos |

---

## Seguridad

| Capa | Proteccion |
|:------|:-----------|
| **Seccomp** | 80+ syscalls permitidos, bloquea carga de modulos, BPF, AF_ALG, I/O de hardware |
| **AppArmor** | Perfiles basados en plantillas por contenedor |
| **User namespaces** | Remapeo de UID/GID, root mapea a usuario no privilegiado |
| **Capabilities** | Set minimo por defecto, grants explicitos, soporte `--cap-drop=ALL` |
| **TLS** | Autenticacion TLS mutua con certificados de cliente |
| **Rate limiting** | Token-bucket: 100 req/s, burst 200 |
| **Verificacion de imagenes** | Proteccion contra path traversal, validacion de symlinks, restricciones de hardlinks |
| **Landlock LSM** | Sandboxing no privilegiado Linux 5.13+ via Landlock ABI v9 |
| **Enforzamiento mTLS** | `RequireAndVerifyClientCert` cuando `ClientCAs` esta configurado |
| **Comparacion en tiempo constante** | Verificacion de pubkey TOFU usa `crypto/subtle.ConstantTimeCompare` |
| **Proteccion contra replay** | Mensajes gossip incluyen nonce aleatorio + ventana de timestamp de 5 minutos, cache LRU de nonces |
| **Prevencion OOM DoS** | Listener de gossip envuelto en `io.LimitReader(MaxGossipMessageBytes+1)` |

### Syscalls Bloqueados

```
init_module, finit_module, delete_module    # Carga de modulos
kexec_load, kexec_file_load                 # Ejecucion de kernel
iopl, ioperm                                # I/O de hardware
kcmp                                        # Fugas de info de kernel
process_vm_readv, process_vm_writev         # Acceso a memoria de procesos
```

### Syscalls Modernos Permitidos

```
io_uring_setup, io_uring_enter, io_uring_register  # I/O asincrono
pidfd_open, pidfd_send_signal, pidfd_getfd         # Descriptores de archivo PID
rseq, userfaultfd, copy_file_range                 # Caracteristicas modernas de kernel
landlock_create_ruleset, landlock_add_rule         # Sandboxing Landlock
```

---

## Configuracion

### Config del Daemon (`~/.doki/config.json`)

```json
{
  "data_dir": "/data/data/com.termux/files/usr/var/lib/doki",
  "socket": "/data/data/com.termux/files/usr/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "default_network": "bridge",
  "debug": false,
  "log_level": "info",
  "rootless": true,
  "dns": {
    "listen": "127.0.0.11:8053",
    "upstream": ["8.8.8.8:53", "8.4.4.4:53"],
    "cache_capacity": 256
  },
  "mesh": {
    "enabled": true,
    "listen": ":7432",
    "stun_servers": ["stun.l.google.com:19302"],
    "enable_mdns": true,
    "payload_encryption": false
  },
  "registry_mirrors": [],
  "insecure_registries": []
}
```

### Variables de Entorno

| Variable | Descripcion | Por Defecto |
|:---------|:------------|:--------|
| `DOKI_HOST` | Path del socket del daemon | Especifico de plataforma |
| `DOKI_DATA_DIR` | Directorio de datos | `~/.doki/data` |
| `DOKI_STORAGE_DRIVER` | Driver de almacenamiento | `fuse-overlayfs` |
| `DOKI_TLS` | Habilitar TLS | sin establecer |
| `DOKI_TLS_CERT` | Path del certificado TLS | sin establecer |
| `DOKI_TLS_KEY` | Path de la clave TLS | sin establecer |
| `DOKI_KERNEL` | Path del kernel MicroVM | Especifico de plataforma |
| `DOKI_NATIVE` | Forzar modo nativo | sin establecer |
| `DOKI_DNS_LISTEN` | Direccion de escucha del servidor DNS | `127.0.0.11:8053` (Android) / `127.0.0.11:53` (Linux) |
| `DOKI_DEBUG` | Habilitar modo debug (pprof en `:6060`) | sin establecer |
| `DOKI_RATE_LIMIT` | Peticiones por segundo | `100` |
| `DOKI_LOG_LEVEL` | Nivel de log (debug/info/warn/error) | `info` |
| `DOKI_LOG_FORMAT` | Formato de log (json/text) | auto-detect |
| `DOKI_LINK_MESH` | Habilitar mesh DokiLink (`1`/`0`) | `1` |
| `DOKI_LINK_ADDR` | Override direccion de escucha gossip del mesh | `:7432` |
| `DOKI_LINK_STUN` | Servidores STUN para NAT traversal (separados por coma) | `stun.l.google.com:19302` |
| `DOKI_LINK_RELAY` | Peer relay para fallback TURN | sin establecer |
| `DOKI_LINK_PAYLOAD_ENC` | Habilitar NaCl secretbox (cifrado L2) | sin establecer |
| `DOKI_USE_SOCAT` | Forzar socat para port forwarding | sin establecer |
| `DOKI_RUNTIME` | Forzar runner especifico (`proot`, `gVisor`, `native`, etc.) | auto-detect |
| `DOKI_EMULATION_MODE` | Preferencia de emulador cross-arch (`qemu`, `fex`, `box64`, `auto`) | `auto` |
| `DOKI_EMULATOR` | Alias para `DOKI_EMULATION_MODE` | sin establecer |

---

## Compilacion

### Requisitos

- Go 1.22 o posterior
- `make` (opcional)
- Para modo microVM: binario `crosvm` o `firecracker` (auto-detectado)
- Para backend VZ de macOS: CGO habilitado, SDK macOS 11+

### Targets de Build

```bash
# Android / Termux (ARM64)
make build-android-arm64
make install

# Android / Termux (ARMv7)
make build-android-armv7

# Linux (ARM64)
make build-linux-arm64

# Linux (ARMv7)
make build-linux-armv7

# Linux (x86_64)
make build-linux-amd64

# macOS (Apple Silicon)
make build-darwin-arm64

# macOS (Intel)
make build-darwin-amd64

# Todas las plataformas a la vez
make release

# Checksums SHA256
make sha256

# Testing y linting
make test      # go test ./...
make vet       # go vet ./...
make clean     # rm -rf releases/
```

### Build Manual

```bash
make release
# O equivalentemente:
go build -trimpath -ldflags="-s -w" -o releases/doki ./cmd/doki
go build -trimpath -ldflags="-s -w" -o releases/dokid ./cmd/dokid
go build -trimpath -ldflags="-s -w" -o releases/doki-compose ./cmd/doki-compose
go build -trimpath -ldflags="-s -w" -o releases/doki-init ./cmd/doki-init
go build -trimpath -ldflags="-s -w" -o releases/doki-kube ./cmd/doki-kube
go build -trimpath -ldflags="-s -w" -o releases/doki-kubectl ./cmd/doki-kubectl
```

---

## Estructura del Proyecto

```
Doki/
  cmd/
    doki/                 Binario CLI (108 comandos, 1600+ lineas)
    dokid/                Binario daemon (REST API, TLS, rate limiting)
    doki-compose/         CLI compatible con Docker Compose
    doki-init/            PID 1 minimo para contenedores (Go)
    doki-init-rust/       PID 1 minimo para guests microVM (Rust, 412K)
    doki-kube/            Plano de control Kubernetes (todo-en-uno)
    doki-kubectl/         Cliente CLI compatible con kubectl
    dokitest/             Suite de tests de integracion
    regtest/              Suite de tests de registro
  pkg/
    api/                  Servidor Docker Engine API v1.54
    podman/               Podman libpod v5 API (39 endpoints)
    compose/              Motor Compose con watch + publish + healthcheck
    apiserver/            Servidor API Kubernetes
    kubelet/              Agente kubelet Kubernetes (cliente CRI real)
    scheduler/            Scheduler Kubernetes
    controllers/          Controladores Kubernetes (10 controladores funcionales)
    kubeproxy/            Kubernetes kube-proxy (iptables/nftables/userspace)
    coredns/              DNS de cluster para Kubernetes
    kubectl/              Libreria cliente HTTP kubectl
    k8s-types/            80 tipos de API Kubernetes
    store/                Store de estado en memoria + SQLiteStore con persistencia crash-safe
    runtime/              Runtime OCI con 12 modos de ejecucion
    image/                Gestion de imagenes OCI (pull, push, build)
    registry/             Cliente OCI Distribution Spec
    network/              Networking de contenedores (bridge, CNI, DNS, pasta)
    storage/              Drivers de almacenamiento (overlay2, fuse, btrfs, zfs)
    builder/              Parser de Dokifile (18 instrucciones, multi-stage)
    cli/                  Libreria CLI (3200+ lineas)
    common/               Tipos compartidos, config, utilidades
    netlink/              DokiLink Mesh (gossip, proxy, NAT traversal, DHT, mDNS)
    emulation/            Emulacion cross-arch (deteccion + config QEMU/FEX/Box64)
    landlock/             Sandbox Landlock LSM (Linux 5.13+)
    macos/                VM nativa macOS (backends VZ + QEMU + Sandbox)
    security/             Perfiles Seccomp y AppArmor
    distro/               Gestion de distribuciones Linux
    cri/                  Servidor gRPC Kubernetes CRI (41 RPCs)
    oci/                  Generacion de spec OCI
    deps/                 Gestion de dependencias (herramienta doki deps)
    scheduler/            Scheduling de pods
  internal/
    dokivm/               Subsistema MicroVM (crosvm, firecracker, qemu)
    namespaces/           Gestion de namespaces Linux
    cgroups/              Gestion de recursos cgroups v2
    fuse/                 Operaciones de filesystem FUSE overlay
    proot/                Fallback proot para Android
    seccomp/              Motor de perfiles Seccomp
    apparmor/             Generador de perfiles AppArmor
  doki-os/                Config de kernel VM doki-OS + Makefile
```

---

## Compatibilidad

### Que Funciona

| Caracteristica | Estado | Notas |
|:--------|:------:|:------|
| `doki run` | Probado | Comandos basicos, shell scripts, --init, --user, --entrypoint, --restart |
| `doki pull` | Probado | Auto-resolucion multi-arch ARM64, descargas paralelas, auth por token |
| `doki push` | Probado | OCI Distribution Spec: blob upload, cross-repo mount, manifest PUT |
| `doki images` | Probado | Tamanos correctos, RepoDigests poblados |
| `doki ps` / `doki ps -a` | Probado | Nombres, puertos, imagen mostrados |
| `doki inspect` | Probado | Salida JSON completa |
| `doki stop` / `doki rm` | Probado | Por nombre o ID, sin deadlocks |
| `doki build` | Probado | Capas RUN, COPY --from, ARG, ENV, .dockerignore, cache de build |
| `doki logs` | Probado | Rotacion (10MB/3 archivos), formato de stream multiplexado Docker |
| `doki exec` | Probado | Corre dentro del contenedor via proot |
| `doki attach` | Probado | HTTP hijack, streaming bidireccional |
| `doki wait` | Probado | Multi-contenedor, retorna codigos de salida |
| `doki login` / `doki logout` | Probado | Auth por token, auth basica, cableado de credenciales |
| `doki network ls` | Probado | Bridge/host/none, creacion de bridge doki0 |
| `doki volume create/ls/rm` | Probado | Driver local, soporte tmpfs |
| `doki-compose up/down` | Probado | Spec completa de compose: redes, volumenes, secrets, healthcheck |
| `doki cp` | Probado | Copiar archivos host/contenedor con extraccion tar |
| Port forwarding (`-p`) | Probado | iptables DNAT (root) y socat (rootless) |
| Auto-seleccion de aislamiento | Probado | El registro elige el mejor runner disponible de 12 modos |
| Flag `--runtime` | Probado | Modo explicito via `doki run --runtime proot` |
| Kubernetes CRI gRPC | Funcional | Los 35+6 RPCs implementados en socket Unix |
| Kubelet con CRI real | Funcional | Loop de reconciliacion llama RunPodSandbox/CreateContainer/StartContainer |
| Kube-proxy | Funcional | Modos iptables/nftables/userspace, DNAT + MASQUERADE |
| Controladores K8s | Funcional | Deployment, ReplicaSet, Job, Endpoint, Service, Namespace, GC |
| SQLiteStore | Funcional | Estado persistente crash-safe |
| Podman API | Funcional | 39 endpoints, gestion de pod/secret/manifest |
| Ejecucion de healthcheck Compose | Funcional | Sondas periodicas, reporte de estado, condicion `service_healthy` |
| NAT traversal DokiLink Mesh | Funcional | STUN + TCP hole punching + fallback relay TURN |
| DHT DokiLink | Funcional | Kademlia 160-bit, k=8, descubrimiento de peers |
| mDNS DokiLink | Funcional | Descubrimiento LAN con expiracion 90s + limpieza 30s |
| Backend VZ macOS | Funcional | Virtualization.framework con bridge cgo |

### Que NO Funciona Todavia

| Caracteristica | Estado | Notas |
|:--------|:------:|:------|
| Aislamiento MicroVM | No probado | Codigo existe, no probado en hardware compatible |
| Aislamiento gVisor | No probado | Deteccion de runsc funciona, runtime no validado |
| Contenedores WASM | No probado | Deteccion wasmedge/iwasm funciona, runtime no validado |
| pKVM/Microdroid | No probado | Deteccion pKVM funciona, sin hardware compatible para probar |
| Sysbox | No probado | Deteccion sysbox-runc funciona, runtime no validado |
| FEX-Emu cross-arch | No probado | Deteccion FEXInterpreter/box64 funciona, runtime no validado |
| QEMU user-mode | No probado | Deteccion qemu-*-static funciona, runtime no validado |
| Modo Chroot | No probado | Funciona en principio, no validado |
| Modo Legacy32 | No probado | Deteccion binfmt_misc funciona, runtime no validado |
| Networking CNI | No probado | Plugin manager existe, no cableado |
| Aislamiento bridge de red | Parcial | Funciona rootful (iptables DNAT); en proot/native, contenedores comparten red del host |

---

## Novedades

### v0.12.0 (Julio 2026)

Doki 0.12 es el release de fidelidad del runtime: I/O interactiva real, streaming
genuino de exec/attach de Kubernetes, la API de Podman conectada al motor real,
semántica de reinicio/readiness en k4s, y una amplia tanda de endurecimiento de
seguridad. 79 archivos modificados, +13.786 / -1.256 líneas sobre v0.11.1.

#### Contenedores interactivos -- `run -it`, `attach`, `exec -it`

- **`pkg/runtime/stdio.go` + `internal/pty/pty_linux.go`**: un broker de stdio real. Las sesiones TTY obtienen una PTY (disciplina de línea, EOF con `Ctrl-D`, cambio de tamaño de ventana vía `TIOCSWINSZ`); las sesiones interactivas sin TTY usan tres tuberías con un stream multiplexado estilo Docker.
- **`doki run -it`, `doki attach`, `doki exec -it`** ahora se conectan en vivo al proceso, con distribución correcta de stdin/stdout/stderr hacia múltiples clientes (colas acotadas, descarte del cliente lento).
- La configuración de terminal de control (`setsid`/`TIOCSCTTY`) solo corre en modo namespaces; la disciplina de línea de proot funciona sin ella.
- Tests: `stdio_test.go`, `exec_stdin_test.go`.

#### Streaming real de Exec/Attach de CRI

- **`pkg/cri/wsstream.go`**: un servidor WebSocket RFC 6455 hecho a mano más el protocolo de canales `remotecommand` de Kubernetes (`v5.channel.k8s.io` con fallback a `v4`, canales 0-5: stdin/stdout/stderr/error/resize/close).
- **`pkg/cri/streamer.go`**: `Reserve()` emite tokens de streaming; `kubectl exec`/`attach` se conectan al proceso real del contenedor y devuelven un código de salida `metav1.Status`.
- Tests: `wsstream_test.go`.

#### API de Podman conectada al motor real (P1-P7)

- **`pkg/podman/containers.go`, `resources.go`, `api.go`**: los endpoints de libpod ahora delegan en los stores reales de runtime, imagen, red y volumen mediante una struct `Deps` inyectada -- create/start/stop/kill/logs/stats/exec reales, build, `play`/`generate kube` y CRUD de volúmenes.
- Códigos de estado honestos: `503` cuando un subsistema no está disponible, `501` para endpoints genuinamente diferidos (se acabaron los stubs silenciosos).
- Tests: `api_test.go`.

#### Fidelidad del runtime de k4s

- **Política de reinicio (K14)**: `Always` / `OnFailure` / `Never` con backoff exponencial y un `RestartCount` real en el estado del pod (`pkg/kubelet/probe.go`).
- **Readiness probes (K13)**: probes `exec`, `tcpSocket` y `httpGet` que respetan `initialDelaySeconds`/`timeoutSeconds`; la condición `Ready` del pod refleja el resultado real del probe.
- **Proyección de volúmenes ConfigMap/Secret**: las claves se escriben a disco y se montan en el contenedor.
- **apiserver**: subrecursos `scale`/`status` escribibles, selectores de etiqueta, JSON-patch, logs/exec reales del contenedor, forma de respuesta de `delete` correcta.
- Tests: `probe_test.go`, `projection_test.go`.

#### Actualización de recursos + ajuste de recursos del scheduler

- **`doki update`** aplica límites de CPU/memoria de cgroup v2 a un contenedor en ejecución (D1).
- **Scheduler (K16)**: los nodos se filtran por requests de CPU/memoria contra los pods ya comprometidos antes de puntuar (`pkg/scheduler/scheduler.go`).
- Tests: `scheduler_test.go`.

#### Endurecimiento de seguridad

- **Cinco fixes críticos**: digest write-what-where, envenenamiento de la caché de imágenes, path traversal en `docker cp`, filtración de secretos de build y `RUN` sin sandbox.
- **Escape por symlink clase CVE-2018-15664**: la extracción de tar pasa por `SecureJoin`, y los bind mounts se abren sobre un fd `O_NOFOLLOW` para vencer la carrera de symlink en rutas enmascaradas (HIGH-7).
- **Límites contra bombas de descompresión**: una capa se acota a 16 GiB / 2.000.000 de entradas.
- Se eliminan xattrs peligrosos y nodos de dispositivo de las capas de imagen; guarda SSRF en el realm de auth del registry; API de control con mismo origen; cuerpos de request de JSON/carga-de-imagen acotados; validación de proto/IP del firewall.
- Nuevas suites `*_security_test.go`: `archive_security_test.go`, `digest_security_test.go`, `securejoin_test.go`, `symlink_escape_test.go`.

#### Postura de seguridad honesta (C1)

- `/info` y el inspect del contenedor reportan las `SecurityOptions` **reales** para el modo de runtime activo (native / proot / namespaces) en vez de afirmaciones fijas; las advertencias de capabilities aparecen en la CLI.

#### Plomería y CLI

- **`pkg/events/bus.go`**: un stream de eventos real. **`pkg/stdcopy/stdcopy.go`**: demux de stream multiplexado de Docker para attach/logs sin TTY.
- Correcciones de CLI: flags cortas booleanas combinadas (`-it`), parseo de `--key=value`, orden de flags de `doki update` (compatible con Docker) y un fix del `daddr` vacío de `nft` en el firewall.
- **Builds de 32 bits**: `maxLayerUncompressedBytes` tipado como `int64` para que armv7 (`GOOS=linux GOARCH=arm`) compile sin overflow.

#### Documentación

- Se agregó el README en chino; el README en español se sincronizó 1:1 con el inglés.

### v0.11.0 (Junio 2026)

Doki 0.11 es el release de networking y madurez: DokiLink Mesh completo con NAT traversal y DHT, backend VZ cgo de macOS, Kubernetes 100% con CRI real, y API de Podman lista para produccion.

#### DokiLink Mesh -- NAT Traversal + DHT + mDNS

- **NAT traversal**: Cliente STUN (RFC 8489), TCP simultaneous open hole punching y servidor relay tipo TURN. Peers en diferentes redes se conectan sin IPs estaticas.
- **Descubrimiento de peers DHT**: Kademlia DHT con IDs de nodo de 160 bits, k-buckets (k=8), alpha=3 lookups paralelos. Routing descentralizado sin config estatica ni mDNS.
- **Expiracion mDNS de 90 segundos**: las entradas expiran despues de 90 segundos si no se refrescan, loop de limpieza cada 30s.
- **Correcciones crypto**: Derivacion de clave independiente del orden (ambos peers derivan la misma clave compartida). Nonces por conexion desde `crypto/rand`. Proteccion contra replay con ventana de timestamp de 5 minutos y cache LRU de nonces. `secretboxStreamConn.Close()` usa `atomic.Bool` para prevenir race de doble-cierre.
- **Hardening del mesh**: `Stop()` cierra `stopCh` para senalar todos los loops. Decodificador de gossip envuelto en `io.LimitReader` (prevencion OOM DoS). Registros TXT mDNS anuncian `common.DokiVersion`.
- **Seguridad**: Validacion de path traversal en TrustStore, SecretManager y ManifestManager. Comparacion en tiempo constante via `crypto/subtle.ConstantTimeCompare` para verificacion de pubkey TOFU. Enforzamiento mTLS con `RequireAndVerifyClientCert`.

#### Virtualizacion Nativa macOS

- **Backend VZ con cgo**: Bridge Objective-C a Virtualization.framework (`VZVirtualMachineConfiguration`, `VZLinuxBootLoader`, `VZVirtioFileSystemDevice`, `VZBridgedNetworkDevice`/`VZNATNetworkDevice`, `VZRosettaPlatform`). Build tag `darwin && cgo`.
- **Correcciones backend QEMU**: `sync.RWMutex` para thread safety, verificacion de binario, args aware de arch, timeout SIGTERM/SIGKILL.
- **Backend Sandbox**: Perfiles ajustados, process-exec y mach-lookup con scope.
- **Compatibilidad de build tags**: El paquete compila en todas las plataformas (darwin+cgo, darwin!cgo, !darwin).

#### Kubernetes 100%

- **Servidor CRI gRPC** (`pkg/cri/server.go`): CRI gRPC real implementando los 35 RuntimeServiceServer + 6 ImageServiceServer RPCs en socket Unix.
- **Kubelet con CRI real**: `NewKubeletWithCRI` marca socket CRI, llama `RunPodSandbox` / `CreateContainer` / `StartContainer`, obtiene PodIP real, estados de contenedores y digests de imagenes.
- **Kube-proxy real**: Cadenas iptables con DNAT/MASQUERADE, generacion de rulesets nftables, proxy TCP/UDP round-robin en userspace (funciona sin root).
- **Controladores funcionales** (`pkg/controllers/manager.go`): DeploymentController, ReplicaSetController, JobController (paralelismo/completaciones/backoff), EndpointController, ServiceController (asignacion de ClusterIP), NamespaceController (eliminacion en cascada), GarbageCollector (OwnerReferences).
- **Servidor API completo**: Paths de grupos API (`networking.k8s.io/v1`, `rbac.authorization.k8s.io/v1`), PATCH (merge-patch + strategic-merge), Watch (formato de eventos K8s).
- **SQLiteStore** (`pkg/store/sqlite.go`): Store persistente con persistencia crash-safe via SQLite.
- **Scheduler real**: Busy-wait reemplazado con sleep bloqueante, scoring de localidad de imagen, scoring least-requested.
- **CoreDNS real**: Race de buffer UDP corregida, soporte de registros SRV, NXDOMAIN para consultas no resolubles.

#### Cableado Podman

- Shim de Podman montado en dokid en `/libpod/*` en el mismo servidor, heredando TLS, middleware y rate limiting.
- Info del sistema usa `runtime.GOARCH`, `runtime.GOOS`, kernel/memoria detectados, `common.DokiVersion`.
- Ciclo de vida de contenedores delega a PodManager (start/stop/kill/restart/pause/unpause).
- Dispatch de contenedores retorna 404 si no encontrado, DELETE retorna 204.

#### Ejecucion de Healthcheck Compose

- `HealthChecker` (`pkg/runtime/healthcheck.go`) ejecuta sondas periodicas (CMD/CMD-SHELL/NONE), respeta Interval/Timeout/Retries/StartPeriod/StartInterval.
- Actualiza `state.HealthStatus.Status` (`starting` -> `healthy`/`unhealthy`).
- Condicion `service_healthy` de Compose funciona de extremo a extremo.

#### Diagnostico

- Herramienta `doki deps` con `ls` (listar deps del sistema), `check` (gate de CI), `go` (listar deps Go), `install <name>` (instalacion best-effort via gestor de paquetes detectado).

### v0.11.1 (Junio 2026)

Release de correccion de bugs y caracteristicas incrementales.

#### Emulacion Cross-Architecture (nuevo)

- **`pkg/emulation/config.go`** (198 lineas): Deteccion de QEMU user-mode, FEX-Emu y Box64 con config persistente en `~/.doki/emulation.json` (escrituras atomicas, permisos 0600).
- **`doki emu {show,detect,set,test}`** -- 4 nuevos subcomandos CLI para gestion de emuladores.
- **`DOKI_EMULATION_MODE`** / **`DOKI_EMULATOR`** variables de entorno overridean la preferencia guardada.
- `emulation.PreferredMode()`, `NormalizeMode()`, `Detect()`, `SelectBest()` API publica. Imagenes de arquitecturas foreignas se rutean automaticamente a traves del emulador seleccionado por el registro de runners.
- 4 tests unitarios (`TestNormalizeMode`, `TestSaveLoadPreferredMode`, `TestPreferredModeEnvWins`, `TestSelectBest`).

#### Refactor del Registro de Runners

- **`pkg/runtime/registry.go`**: Algoritmo `BestFor()` reescrito con soporte de variable de entorno `DOKI_RUNTIME`, `requestedRuntime()`, `runnerUsableOnHost()`, y `preferredEmulationRunner()` para routing cross-arch.
- 5 nuevos tests (`TestRegistryBestForUsesEnvRuntime`, `TestRegistryBestForChoosesHighestUsableLevel`, `TestRegistryBestForSkipsUnavailableHostRequirements`, `TestRegistryBestForCrossArchPrefersEmulation`, `TestRegistryBestForUsesQEMUPreference`).

#### Daemon

- **Flag `--host`** con direccionamiento estilo Docker: `unix:///path`, `tcp://addr:port`, path desnudo. Parsing `applyDaemonHost()`. Soporta variables de entorno `DOKI_HOST` y `DOCKER_HOST`.
- **`cmd/dokid/main_test.go`**: `TestApplyDaemonHost` cubre tres paths de parsing.

#### Bug Fix -- Issue #5: Networking Rootless en Termux

- **Warnings de chown** (`pkg/runtime/runtime.go`): `logChownError()` emite un solo mensaje INFO en modo rootless en vez de cientos de lineas WARN.
- **Fallback de pasta** (`pkg/network/manager.go`): `setupRootlessNetworking()` ahora intenta `pasta` -> `slirp4netns` -> host netns via proot (anteriormente se caia cuando pasta no se encontraba).
- **Codigo de salida del cliente** (`cmd/doki/main.go`): Manejo de errores de subcomando `dispatch()` usa `handleError()`. `ExitError{Code: 0}` ya no imprime "Error:".
- **UX especifica de Termux**: `termuxNetworkHint()` previene sugerencias enganosas de "pkg install passt". Mensaje INFO de fallback explica limitacion de `/dev/net/tun` + `CAP_NET_ADMIN`.
- **1 test de regresion**: `TestSetupRootlessNetworking_Fallback`.

#### Documentacion

- README restaurado al nivel de detalle de v0.10.0 (1243 lineas).
- 22 paginas de wiki reescritas en estilo limpio (cero emojis, cero SVGs, cero diagramas de caja ASCII).
- Dominio: `doki.opceanai.com` -> `dok1.xyz`.

#### Correcciones de Seguridad

- `emulation.json` almacenado con permisos 0600 y escrituras atomicas.
- `logChownError()` previene inyeccion de logs desde paths de archivos OCI.

### v0.10.0

Doki 0.10 es una expansion masiva: **Compatibilidad 1:1 con API de Podman, distribucion Kubernetes completa, soporte VM nativa de macOS, imagen VM doki-OS, y 20 nuevas dependencias** llevando el motor a 55,000+ lineas de codigo en 158 archivos.

#### Nuevas Plataformas y APIs

| Caracteristica | Descripcion |
|:--------|:------------|
| **Podman API v5** | 39 endpoints compatibles con clientes `podman-remote`. Gestion de Pod, secret y manifest |
| **Kubernetes 1.32** | Plano de control completo: apiserver, kubelet, scheduler, controllers (10), kube-proxy, CoreDNS |
| **macOS Nativo** | Backends VZ (Virtualization.framework) y QEMU para Apple Silicon e Intel Macs |
| **doki-OS** | Config de kernel Linux minimo (~4MB bzImage) para guests VM optimizados para contenedores |
| **Landlock LSM** | Sandboxing no privilegiado Linux 5.13+ via Landlock ABI v9 |

#### Nuevos Binarios

| Binario | Descripcion |
|:-------|:------------|
| `doki-kube` | Plano de control Kubernetes todo-en-uno |
| `doki-kubectl` | CLI compatible con kubectl (get, apply, delete, describe, logs) |

#### Calidad

- **staticcheck**: 0 warnings
- **errcheck production**: 0 errores sin chequear
- **go vet**: 0 warnings

### v0.9.3

Este release trajo **DokiLink-Lite** (mesh networking) y **190+ bug fixes** en 4 rondas de auditoria comprehensiva.

#### DokiLink-Lite (Mesh Networking)

Un proxy TCP/UDP + capa mesh que te deja reenviar el puerto publicado de un contenedor a otra instancia de Doki. Go stdlib puro + `crypto/tls` + `golang.org/x/crypto/nacl`.

| Caracteristica | Descripcion |
|:--------|:------------|
| Proxy TCP/UDP | Half-close, timeouts de idle, wrappers de transporte |
| TLS 1.3 (L1) | Cifrado por defecto, ECDSA P-256 CA por instalacion |
| NaCl secretbox (L2) | Opt-in via `DOKI_LINK_PAYLOAD_ENC=1`, clave derivada de Ed25519 |
| Identidad de instalacion | Keypair Ed25519 + ECDSA CA en `$DOKI_ROOT/keys/` |
| Confianza TOFU | Clave publica registrada en primer contacto, verificada en reconexion |
| Peers estaticos | `$DOKI_ROOT/mesh/peers.json` via `doki link add/rm` |
| mDNS (opt-in) | Construido con `-tags netlink_mdns`, descubrimiento solo-LAN |
| Gossip | Mensajes JSON firmados sobre TCP, tick de descubrimiento de peers de 15s |

#### Correcciones Criticas de Bugs

- kill no actualizaba estado -- hace poll con `signal(0)` y luego guarda estado de salida
- stop no actualizaba estado en fallo de SIGKILL -- siempre guarda codigo de salida 137
- Exec sin output -- retorna bytes stdout/stderr
- Flags de Compose despues del comando -- parser continua despues del subcomando
- Name de lista de contenedores siempre vacio -- `stateToInfo` establece `info.Name`
- Create ignora `?name=` -- lee parametro query de URL
- Config de inspect de imagen -- conversion PascalCase
- `ps --format` -- ejecucion de template

### v0.9.2

Este release fue un pase de **estabilidad + correccion** sobre v0.9.1.

#### Correcciones Criticas de Networking

1. **iptables DNAT faltaba flag `-A`** -- `-A OUTPUT` faltaba, causando error "Unknown option"
2. **Port forwarding a localhost** -- socat ahora apunta al IP bridge del contenedor
3. **Pares veth orphanos** -- `Endpoint` rastrea `VethHost`/`VethPeer`, teardown elimina ambos extremos
4. **proot faltando en hosts nuevos** -- `FindProotBinary()` hace fallback al PATH del sistema

#### Reescritura del Servidor DNS (18 bugs corregidos)

Cache LRU (1024 entradas, TTL 5 min), soporte AAAA + PTR, ndots:0, retry TCP en bit TC, upstream Android via `getprop net.dns*`, stripping de puertos, auto-registro, recuperacion en reinicio del daemon.

#### Fix LD_PRELOAD para Termux

`libtermux-exec-ld-preload.so` estaba rompiendo el syscall forwarding basado en ptrace de proot. Fix: `StripHostEnv()` elimina `LD_PRELOAD` y `LD_LIBRARY_PATH` de los entornos de contenedores.

### v0.9.1

- **OCI Push:** `doki push` -- blob upload, cross-repo mount, manifest PUT
- **Auth de Registro:** `doki login` acepta credenciales y propaga al cliente de registro
- **Extraccion tar nativa:** Tar nativo en Go con whiteouts, proteccion contra path traversal, auto-deteccion de compresion, extraccion paralela con rollback
- **4 nuevas distros:** Fedora, Gentoo, OpenSUSE, Rocky Linux -- 8 distros en total
- **Motor Compose mejorado:** Sintaxis larga Ports/Volumes, condiciones health de `depends_on` con polling 60s, 30+ campos nuevos
- **19 correcciones C de Proot:** SECCOMP_RET_ALLOW, bug de llave en fake_id0, fix uid/gid en stat.c, UB en link2symlink y mas
- **Mount kernel overlay2:** Usa `syscall.Mount("overlay")` directamente en vez de delegacion FUSE
- **Attach via HTTP hijack:** `doki attach` con streaming bidireccional
- **Listener DNS:** Servidor DNS interno en puerto 53 para resolucion entre contenedores
- **ARMv7 beta:** Compilacion y binarios para dispositivos ARM de 32 bits

### v0.9.0

- **doki-init-rust:** PID 1 reescrito en Rust (412K vs 2.9MB Go, -86%)
- **doki-proot:** Proot forkeado con modo daemon + protocolo JSON IPC. Binario de 14K
- **Sistema de distros:** `doki run --distro alpine/ubuntu/debian/arch` descarga desde Docker Hub
- **ARMv7 beta:** Paridad completa de caracteristicas para dispositivos ARM antiguos
- **Immich:** Stack completo corriendo (PostgreSQL 18 + pgvector + cube + earthdistance, Redis 7, Immich Server v2.7.5)

---

## Contribuir

Las contribuciones son bienvenidas. Areas donde mas se necesita ayuda:

| Area | Descripcion |
|:-----|:------------|
| **Backends MicroVM** | Soporte para hypervisors y plataformas adicionales |
| **Plugins CNI** | Implementacion de caracteristicas avanzadas de networking |
| **Seguridad** | Hardening, fuzzing y penetration testing |
| **Rendimiento** | Cache de capas, operaciones paralelas, optimizacion de memoria |
| **Testing** | Tests de integracion, tests end-to-end, stress tests |
| **Documentacion** | Tutoriales, ejemplos y referencia de API |

### Setup de Desarrollo

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
go build ./...
go test ./...
```

### Estilo de Commits

- Usar modo imperativo ("Add feature" no "Added feature")
- Mantener la primera linea bajo 72 caracteres
- Referenciar issues cuando aplique

---

## Licencia

Doki en si es Apache 2.0. DokiOS incluye componentes de terceros bajo sus respectivas licencias.

### Doki

```
Copyright 2024-2026 OpceanAI

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

### Componentes Incluidos

| Componente | Licencia | SPDX | Notas |
|:----------|:--------|:-----|:------|
| **Doki** | Apache 2.0 | `Apache-2.0` | Licenciado bajo Apache 2.0. Una licencia altamente comercial y permisiva con proteccion de patentes explicita para el usuario |
| **cloudflared** | Apache 2.0 | `Apache-2.0` | Tunel de Cloudflare. Otorga libertades comerciales con proteccion de patentes, igual que Doki |
| **fastfetch** | MIT | `MIT` | Licencia extremadamente corta y simple -- hacer casi lo que quieras con el codigo |
| **OpenSSH** | Estilo BSD | `SSH-OpenSSH` | Licencia de OpenSSH. Altamente permisiva, historicamente optimizada para seguridad y redistribucion libre |
| **zsh** | MIT / BSD | `MIT` o `BSD-2-Clause` | Licencia permisiva estilo MIT/BSD, manteniendo el entorno del shell libre de copyleft estricto |
| **bash** | GPL-3.0 | `GPL-3.0-only` | GNU GPLv3. La herramienta mas restrictiva legalmente del set: cualquier derivado debe compartir codigo fuente, con fuertes clausulas anti-tivoization |

---

## Enlaces

| Plataforma | Repositorio | Fuente de verdad |
|:---------|:-----------|:----------------|
| GitHub | [OpceanAI/Doki](https://github.com/OpceanAI/Doki) | Si (primario) |
| GitLab | [aguitauwu/doki](https://gitlab.com/aguitauwu/doki) | mirror |
| Codeberg | [aguitauwu/Doki](https://codeberg.org/aguitauwu/Doki) | mirror |
| Aguita | [root/Doki](https://git.aguita.site/root/Doki) | mirror |
| Sitio Web | [dok1.xyz](https://dok1.xyz) | docs / script de instalacion |
| README en ingles | [README.md](README.md) | original |
| README en chino | [README.zh.md](README.zh.md) | traduccion |

> Main es la unica fuente de verdad. Los mirrors se sincronizan por fuerza desde `main` despues de cada release. Si encuentras una divergencia, abre un issue en GitHub.

### Wikis

| Plataforma | Wiki |
|:---------|:-----|
| GitHub | [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki) |
| GitLab | [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home) |
| Codeberg | [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki) |

### Relacionados

| Repositorio | Descripcion |
|:-----------|:------------|
| [Doki-proot](https://github.com/OpceanAI/Doki-proot) | Proot forkeado con modo daemon JSON IPC para Doki |
