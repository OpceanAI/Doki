# Instalación

Doki distribuye 4 binarios (`doki`, `dokid`, `doki-compose`, `doki-init`) para 4 combinaciones de plataforma/arquitectura. Elige tu plataforma abajo.

## Instalación rápida (Linux/macOS/Android vía Termux)

```bash
curl -sL https://doki.opceanai.com | sh
```

Esto descarga el binario correcto para tu plataforma a `~/.local/bin/`. Añádelo a tu `$PATH` si no está ya.

## Termux (Android)

Termux es el entorno Android soportado principal. Doki corre rootless en Termux, sin necesidad de root.

### Desde F-Droid (recomendado)

1. Instala [Termux desde F-Droid](https://f-droid.org/packages/com.termux/) (NO desde Google Play — la versión de Play está desactualizada)
2. Abre Termux y corre:

```bash
pkg update && pkg upgrade
pkg install proot curl
curl -sL https://doki.opceanai.com | sh
```

### Desde GitHub Releases

```bash
pkg install proot curl
mkdir -p $PREFIX/bin
curl -L -o $PREFIX/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-android-arm64
curl -L -o $PREFIX/bin/dokid https://github.com/OpceanAI/Doki/releases/latest/download/dokid-android-arm64
curl -L -o $PREFIX/bin/doki-compose https://github.com/OpceanAI/Doki/releases/latest/download/doki-compose-android-arm64
curl -L -o $PREFIX/bin/doki-init https://github.com/OpceanAI/Doki/releases/latest/download/doki-init-android-arm64
chmod +x $PREFIX/bin/doki*
```

### Verificando

```bash
$ doki version
Client: Doki
 Version:    0.9.3
 API version: 1.48
 GitCommit:  faab400
 Built:      2026-06-08

$ doki run --rm alpine echo "hola desde doki"
hola desde doki
```

### Notas específicas de Termux

- `LD_PRELOAD` y `LD_LIBRARY_PATH` se eliminan del entorno de proot automáticamente (v0.9.2+)
- El DNS escucha en `127.0.0.11:8053` (el puerto 53 está bloqueado por SELinux sin root)
- El runtime por defecto es proot; sobrescribe con `doki run --runtime native`
- Driver de storage: `fuse-overlayfs` (no necesita root)
- Dispositivos ARMv7 (32-bit): usa los binarios `android-armv7` (compilados con `GOOS=linux`, corre via proot)

## Linux

### Debian / Ubuntu

```bash
sudo apt update
sudo apt install -y curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Fedora / RHEL / Rocky

```bash
sudo dnf install -y curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Arch / Manjaro

```bash
sudo pacman -Syu curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Alpine

```bash
sudo apk add curl fuse-overlayfs iptables
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Gentoo

```bash
sudo emerge -av sys-fs/fuse-overlayfs net-firewall/iptables net-misc/curl
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

### Notas específicas de Linux

- El modo root requiere `iptables` y `kmod` (para `modprobe overlay`)
- El modo rootless usa `fuse-overlayfs` (instala desde tu package manager) y `pasta` (descárgalo de [passt](https://passt.top/) o usa el binario distribuido con Doki)
- El binario `doki-init` es el PID 1 para guests microVM; no lo necesitas para contenedores normales
- Para dispositivos ARMv7 (ARM de 32 bits), usa los binarios `android-armv7` (Termux) o `linux-armv7` (Raspberry Pi, postmarketOS)

## macOS

Doki distribuye un binario `doki` solo-CLI para macOS Apple Silicon (arm64). El daemon y otros binarios son solo para Linux porque dependen de `internal/namespaces` y mounts de overlayfs que no existen en Darwin.

### Homebrew (planeado)

Una fórmula de Homebrew está en proceso. Por ahora, instala manualmente.

### Instalación manual

```bash
curl -L -o /usr/local/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-darwin-arm64
chmod +x /usr/local/bin/doki
```

### Notas específicas de macOS

- El CLI corre solo en `ModeNative` — sin aislamiento, sin red bridge
- Para usar el daemon, corre `dokid` en un servidor Linux y apunta tu `doki` local a él vía `DOKI_HOST=tcp://servidor:2375`
- macOS no es un target de build para `dokid`/`doki-compose`/`doki-init` — esos fallarán al construir con GOOS=darwin

## Windows / WSL2

Windows nativo no está soportado. Usa WSL2 con las instrucciones de instalación de Ubuntu de arriba.

```powershell
wsl --install
wsl --set-default-version 2
# Luego sigue los pasos de instalación de Debian/Ubuntu dentro de WSL
```

## Chromebook (contenedor Linux de ChromeOS)

El contenedor Linux (Beta) de ChromeOS es esencialmente una VM Debian. El CLI de Doki corre dentro de él; para escenarios de contenedor-en-contenedor usa el runtime `proot`.

```bash
sudo apt update
sudo apt install -y curl fuse-overlayfs proot
curl -L -o /tmp/doki.tar.gz https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.tar.gz
sudo tar -xzf /tmp/doki.tar.gz -C /usr/local/bin/
```

Para aislamiento pKVM en Chromebooks con el firmware correcto, Doki lo auto-detectará y lo usará (nivel 11 en la página de [Niveles de aislamiento](Isolation-Levels.es)).

## Raspberry Pi / placas ARM

Usa el binario `linux-armv7` (Raspbian 32-bit) o `linux-arm64` (Raspberry Pi OS 64-bit, Ubuntu ARM64).

```bash
# Detecta tu arquitectura
uname -m
# aarch64 -> arm64, armv7l -> armv7

# Para Pi OS 64-bit
curl -L -o /usr/local/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64
chmod +x /usr/local/bin/doki

# Para Raspbian 32-bit
curl -L -o /usr/local/bin/doki https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-armv7
chmod +x /usr/local/bin/doki
```

Habilita cgroups v2 en `/boot/firmware/cmdline.txt` añadiendo:

```
cgroup_memory=1 cgroup_enable=memory
```

Luego reinicia.

## postmarketOS / PinePhone / Librem

Distros de Linux móvil basadas en Alpine o Arch. Usa los pasos de instalación de Alpine o Arch. Proot es el runtime por defecto.

## Compilando desde el código fuente

Consulta [Compilando desde el código fuente](#compilando-desde-el-c%C3%B3digo-fuente) abajo o la sección [Building](../README.md#building) del README.

## Verificando descargas

Cada binario tiene un archivo `.sha256`:

```bash
$ curl -L -O https://github.com/OpceanAI/Doki/releases/latest/download/doki-linux-arm64.sha256
$ sha256sum -c doki-linux-arm64.sha256
doki-linux-arm64: OK
```

## Troubleshooting

| Síntoma | Solución |
|:--------|:---------|
| `command not found: doki` | Añade `$PREFIX/bin` (Termux) o `/usr/local/bin` (Linux) al `$PATH` |
| `execve: Function not implemented` (Termux) | Arreglado en v0.9.2+; actualiza a la última release |
| `port 53: permission denied` (Termux) | Esto es esperado; Doki usa el puerto 8053 por defecto en Android |
| `requires external cgo linking` (armv7) | Arreglado en v0.9.3; los builds armv7 usan workaround `GOOS=linux` |
| `iptables: Unknown option` | Actualiza a v0.9.2+; el bug del DNAT fue arreglado |
| `cannot find proot` | `apt install proot` (Debian/Ubuntu) o `pkg install proot` (Termux) |

## Siguientes pasos

- Continúa a [Inicio Rápido](Quick-Start.es) para un tutorial de 5 minutos
- Lee [Arquitectura](Architecture.es) para entender el daemon
- Elige el [Nivel de aislamiento](Isolation-Levels.es) correcto para tu carga
