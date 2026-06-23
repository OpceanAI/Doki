# Instalacion

<sub>[SETUP POR PLATAFORMA / v0.11.0]</sub>

> Doki distribuye binarios estaticos sin dependencias de runtime.
> Elige tu plataforma, descarga, verifica, instala.

---

## Termux (Android)

```bash
# Instalar dependencias.
pkg install proot

# Instalador de una linea.
curl -fsSL https://raw.githubusercontent.com/OpceanAI/Doki/main/install.sh | bash

# O instalacion manual.
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-android-arm64
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/dokid-android-arm64
chmod +x doki-android-arm64 dokid-android-arm64
mv doki-android-arm64 $PREFIX/bin/doki
mv dokid-android-arm64 $PREFIX/bin/dokid

# Verificar.
doki doctor
```

Para aislamiento de red, instala `passt` (provee el binario `pasta`):

```bash
pkg install passt
```

Sin `pasta`, los contenedores usan networking compartido del host.

---

## Linux

<sub>[APT / DEBIAN + UBUNTU]</sub>

```bash
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-linux-amd64
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/dokid-linux-amd64
chmod +x doki-linux-amd64 dokid-linux-amd64
sudo mv doki-linux-amd64 /usr/local/bin/doki
sudo mv dokid-linux-amd64 /usr/local/bin/dokid
```

<sub>[PACMAN / ARCH]</sub>

```bash
# Desde AUR (mantenido por la comunidad).
yay -S doki-bin
```

<sub>[DNF / FEDORA]</sub>

```bash
# RPM (mantenido por la comunidad, verificar disponibilidad).
sudo dnf install https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-0.11.0-1.x86_64.rpm
```

---

## macOS

macOS es una plataforma cliente. `doki`, `doki-kube` y `doki-kubectl`
funcionan nativamente. `dokid` requiere un backend Linux o daemon
remoto.

```bash
# via Homebrew (mantenido por la comunidad, verificar disponibilidad).
brew install doki

# O instalacion manual.
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-darwin-arm64
chmod +x doki-darwin-arm64
sudo mv doki-darwin-arm64 /usr/local/bin/doki
```

---

## SBCs ARM (Raspberry Pi, Orange pi, etc.)

```bash
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/doki-linux-arm64
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/dokid-linux-arm64
chmod +x doki-linux-arm64 dokid-linux-arm64
sudo mv doki-linux-arm64 /usr/local/bin/doki
sudo mv dokid-linux-arm64 /usr/local/bin/dokid
```

---

## Compilar desde Fuente

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
make build-release sha256
```

Requiere Go 1.26+. Para el backend VZ de macOS, compilar con
`CGO_ENABLED=1` en darwin.

---

## Matriz de Binarios

```
BINARIO         ANDROID-ARM64  LINUX-ARM64  LINUX-AMD64  DARWIN-ARM64
─────────────────────────────────────────────────────────────────────
doki            si             si           si           si
dokid           si             si           si           ---
doki-compose    si             si           si           ---
doki-init       si             si           si           ---
doki-kube       si             si           si           si
doki-kubectl    si             si           si           si
```

---

## Verificar Checksums

```bash
curl -fsSLO https://github.com/OpceanAI/Doki/releases/download/v0.11.0/SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt --ignore-missing
```

---

## Post-Instalacion

```bash
# Verificar dependencias.
doki doctor

# Arrancar daemon.
dokid &

# Ejecutar primer contenedor.
doki run --rm alpine echo "ok"
```
