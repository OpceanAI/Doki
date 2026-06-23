# DOKI

<sub>[RUNTIME DE CONTENEDORES SIN ROOT / GO 1.26 / APACHE-2.0]</sub>

> Arquitectura de runtime sin root disenada para virtualizar cargas
> POSIX bajo los limites de Android, Linux y macOS sin requerir
> capacidades elevadas, parches de kernel ni privilegios de daemon.

[![Go](https://img.shields.io/badge/go-1.26.3+-00ADD8?style=flat-square&color=24292e)](https://go.dev)
[![Licencia](https://img.shields.io/badge/licencia-Apache_2.0-555?style=flat-square&color=24292e)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.11.0-0F766E?style=flat-square&color=24292e)](https://github.com/OpceanAI/Doki/releases)
[![Descargas](https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat-square&color=24292e)](https://github.com/OpceanAI/Doki/releases)

---

## Resumen

Doki es un motor de contenedores para entornos donde Docker no
puede ejecutarse: telefonos Android, computadoras de placa unica
ARM, hosts Linux sin privilegios y maquinas de desarrollo macOS.
Implementa la Docker Engine API v1.54, la Podman libpod API,
distribucion de imagenes OCI, orquestacion Compose y un plano de
control Kubernetes en proceso con un servidor gRPC CRI real.

El proyecto no afirma paridad con Docker, Podman ni Kubernetes.
Rastrea conformidad contra especificaciones upstream y reporta
honestamente donde existen brechas.

---

## Taxonomia de Subsistemas

<sub>[SUBSISTEMA / CAPA DE INVERSION / HUELLA DETERMINISTA]</sub>

```
SUBSISTEMA           IMPLEMENTACION                HUELLA / LIMITE
─────────────────────────────────────────────────────────────────────
Docker Engine API    HTTP/JSON sobre Unix socket   conjunto v1.54
Podman libpod API    /libpod/* montado en daemon   shim libpod v5
Imagen OCI           distribution-spec v1.1        manifest, index, layers
Compose              parser YAML + resolver deps   subconjunto compose-spec
Kubernetes CRI       gRPC sobre Unix socket        35+6 RPCs, kubelet
Kube-proxy           iptables/nftables/userspace   DNAT, MASQUERADE, RR
DokiLink Mesh        gossip TCP + firmas Ed25519   TLS 1.3, NaCl secretbox
NAT Traversal        STUN RFC 8489 + relay         hole punching, TURN
DHT Discovery        Kademlia 160-bit, k=8, a=3    routing descentralizado
Almacenamiento       fuse-overlayfs / vfs / ov2    content-addressable
DNS                  servidor miekg/dns en :8053   A, SRV, PTR, NXDOMAIN
macOS VZ             puente cgo a Virtualization   VZVirtualMachine + ObjC
```

---

## Flujo de Ejecucion

<sub>[CICLO DE VIDA DEL CONTENEDOR / PIPELINE DEL DAEMON]</sub>

1. El usuario invoca `doki run alpine`.
2. El CLI envia `POST /containers/create` al daemon.
3. El daemon descarga las capas OCI y extrae el stream tar a un
   directorio rootfs. Se selecciona el runner (proot, native, gVisor,
   etc.).
4. El daemon responde `201 Created`. Se configuran la red y el port
   forwarding.
5. El CLI envia `POST /containers/{id}/start`.
6. El daemon hace fork del binario runner con la spec OCI. Para
   proot: `proot -S rootfs /bin/sh`.
7. El stdout del contenedor se streamea al CLI via
   `GET /containers/{id}/logs`.
8. Al salir, el daemon limpia el rootfs y elimina el contenedor si
   se especifico `--rm`.

---

## Inicializacion del Sistema

<sub>[VERIFICACION DE HOST / ARRANQUE DEL DAEMON]</sub>

```bash
# Verificar dependencias del host.
doki doctor

# Arrancar el daemon en segundo plano.
dokid &

# O en primer plano para ver logs.
dokid --log-level=info --log-format=text
```

<sub>[INTERACCION API A NIVEL DE WIRE]</sub>

```bash
# Apuntar el Docker CLI al socket de Doki.
export DOCKER_HOST=unix://$HOME/.doki/doki.sock

# Comandos Docker estandar funcionan sin modificacion.
docker ps
docker run --rm alpine echo "hola desde Doki"
docker pull nginx:alpine
docker images

# SDK de Python.
python3 -c "
import docker
c = docker.DockerClient(base_url='unix://$HOME/.doki/doki.sock')
print(c.info())
"
```

---

## Modos de Aislamiento

<sub>[REGISTRO DE RUNNERS / 12 BACKENDS DE EJECUCION]</sub>

```
MODO         ROOT   AISLAMIENTO            PLATAFORMA
────────────────────────────────────────────────────────────
proot        no     intercept ptrace        android, linux
native       no     exec directo            todas
namespaces   si     ns usuario+mount+pid    linux
chroot       no     aislamiento fs          linux, android
gvisor       no     kernel userspace        linux
microVM      si     crosvm/firecracker      linux, android(AVF)
wasm         no     runtime WebAssembly     todas
qemuuser     no     emulacion QEMU user     todas (cross-arch)
sysbox       si     nesting de contenedores linux
fex          no     FEX-Emu x86-sobre-ARM   arm
pkdroid      no     Android pKVM (AVF)      android 14+
legacy32     no     fallback 32-bit          armv7
```

El registro de runners selecciona el mejor modo disponible
automaticamente. Override explicito via `DOKI_RUNTIME=proot` o
flag `--runtime`.

---

## Matriz de Plataformas

<sub>[DISPONIBILIDAD DE BINARIOS / COMBINACIONES SOPORTADAS]</sub>

```
PLATAFORMA             doki   dokid   compose   kube   kubectl
────────────────────────────────────────────────────────────────
android arm64 termux    si     si      si       si     si
android armv7 termux    si     si      si       si     si
linux arm64             si     si      si       si     si
linux armv7             si     si      si       si     si
linux amd64             si     si      si       si     si
macos arm64             si     ---     ---      si     si
macos amd64             si     ---     ---      si     si
```

`dokid` y `doki-compose` requieren primitivas de procesos y red de
Linux/Android. macOS opera como plataforma cliente, pareado con un
daemon remoto o backend de VM.

---

## Objetivos de Compatibilidad

<sub>[SPEC UPSTREAM / ESTADO DE CONFORMIDAD]</sub>

```
INTERFAZ               ESPECIFICACION                 ESTADO
──────────────────────────────────────────────────────────────────────
Docker Engine API      docs.docker.com/reference/api  endpoints core
Compose Spec           compose-spec.io                ciclo de vida
Imagen OCI             github.com/opencontainers/...  pull/push/digests
Distribucion OCI       github.com/opencontainers/...  multi-arch, auth
Kubernetes CRI         kubernetes.io/docs/.../cri     gRPC 41 RPCs
Podman libpod          podman.io                      /libpod/* montado
```

---

## Configuracion

<sub>[VARIABLES DE ENTORNO / PARAMETROS DE RUNTIME]</sub>

```bash
# Path del socket CLI (override del default).
export DOKI_HOST=unix://$HOME/.doki/doki.sock

# Socket compatible con Docker (fallback cuando DOKI_HOST no esta).
export DOCKER_HOST=unix://$HOME/.doki/doki.sock

# Root de datos del daemon.
export DOKI_DATA_DIR=/var/lib/doki

# Override del driver de almacenamiento.
export DOKI_STORAGE_DRIVER=fuse-overlayfs

# Deshabilitar DokiLink mesh.
export DOKI_LINK_MESH=0

# Habilitar TLS en el daemon.
export DOKI_TLS=1

# Servidores STUN para NAT traversal (separados por coma).
export DOKI_LINK_STUN=stun.l.google.com:19302

# Peer relay para fallback tipo TURN.
export DOKI_LINK_RELAY=203.0.113.5:7432
```

<sub>[ARCHIVO DE CONFIG DEL DAEMON / config.json]</sub>

```json
{
  "data_dir": "/var/lib/doki",
  "socket": "/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "log_level": "info",
  "rate_limit": 100,
  "rate_burst": 200,
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
  }
}
```

---

## DokiLink Mesh

<sub>[NETWORKING PEER-TO-PEER / GOSSIP ENCRIPTADO]</sub>

DokiLink provee networking multi-host sin broker central. Los pares
se descubren via mDNS (LAN), DHT (internet) o configuracion estatica.
Todo el trafico se autentica con firmas Ed25519 y se cifra
opcionalmente con TLS 1.3 o NaCl secretbox.

El NAT traversal sigue una secuencia de cuatro etapas: (1) ambos
pares consultan un servidor STUN para descubrir su IP y puerto
publicos, (2) los pares intercambian sus direcciones publicas via
el protocolo gossip, (3) ambos pares envian SYN TCP simultaneos a
la direccion publica del otro (hole punching), (4) si el NAT permite
el SYN entrante a traves del pinhole, se establece un canal directo
encriptado. Si el hole punching falla, el trafico usa un peer relay
que actua como proxy TURN.

La derivacion de clave es independiente del orden (ambos pares
computan la misma clave compartida via SHA-256 de pubkeys
ordenadas). Los nonces por conexion se siembran desde crypto/rand.
La proteccion contra replay usa una ventana de timestamp de 5
minutos con cache LRU de nonces (1024 entradas).

---

## Diagnostico

<sub>[VERIFICACION DE DEPENDENCIAS DEL HOST]</sub>

```bash
# Verificar herramientas del host.
doki doctor

# Listar todas las dependencias del sistema con estado.
doki deps ls

# Puerta de CI: exit non-zero si faltan deps requeridas.
doki deps check

# Auditar dependencias del modulo Go.
doki deps go

# Instalacion best-effort via gestor de paquetes detectado.
doki deps install pasta
```

---

## Desarrollo

<sub>[BUILD / TEST / RELEASE]</sub>

```bash
# Ejecutar el suite completo de tests.
go test ./... -count=1

# Analisis estatico.
go vet ./...
staticcheck ./...

# Construir binarios para todas las plataformas.
make build-release sha256

# Cross-compile un solo target.
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -trimpath -o doki ./cmd/doki
```

---

## Documentacion

<sub>[MATERIAL DE REFERENCIA]</sub>

- [Wiki](https://github.com/OpceanAI/Doki/wiki) -- Instalacion, Quick Start,
  Arquitectura, Networking, Seguridad, Storage, Referencia CLI
- [Release Notes](RELEASE_NOTES.md) -- Changelog por version
- [Compatibility Roadmap](docs/COMPATIBILITY_ROADMAP.md) -- Tracking de conformidad
- [Technical Report](docs/technical-report/) -- Documentacion de ingenieria

---

## Postura del Proyecto

Doki es ambicioso pero honesto. Las afirmaciones estan vinculadas a
tests, no a marketing. La prioridad es hacer que los contenedores
sin root en Android funcionen correctamente, luego expandir
compatibilidad una interfaz upstream a la vez.

---

## Licencia

Apache-2.0. Ver [LICENSE](LICENSE).
