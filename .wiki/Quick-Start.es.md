# Inicio Rapido

<sub>[TUTORIAL DE 5 MINUTOS / v0.11.0]</sub>

> Recorrido: instalar, verificar, daemon, pull, run, compose, cleanup.
> Probado en Termux/Android 12 aarch64 y Linux AMD64.

---

## 0. Prerrequisitos

- Doki instalado (ver [Instalacion](Installation.es))
- 100 MB libres en disco para la primera imagen
- `proot` instalado en Termux (`pkg install proot`)

---

## 1. Verificar la Instalacion

```bash
$ doki version
Client: Doki
 Version:       0.11.0
 API version:   1.54
 Go version:    go1.26.3
 OS/Arch:       android/arm64
```

```bash
$ doki doctor
[ok]   proot       /data/data/com.termux/files/usr/bin/proot
[ok]   iptables    /usr/sbin/iptables
[warn] pasta       no encontrado (opcional, networking sin root)
[ok]   fuse-overlayfs  /usr/bin/fuse-overlayfs
```

Si `doki doctor` reporta que falta `proot` en Termux:

```bash
pkg install proot
```

---

## 2. Arrancar el Daemon

<sub>[PRIMER PLANO / VISIBILIDAD DE LOGS]</sub>

```bash
$ dokid
INFO  dokid starting   version=0.11.0  mode=proot
INFO  storage driver   name=fuse-overlayfs
INFO  dns server       listen=127.0.0.11:8053
INFO  doki-link ready  listen=127.0.0.1:7432
INFO  runtime mode     mode=proot
INFO  dokid ready      images=18
```

<sub>[SEGUNDO PLANO / PRODUCCION]</sub>

```bash
$ dokid > /tmp/dokid.log 2>&1 &
$ echo $! > /tmp/dokid.pid
```

---

## 3. Descargar una Imagen

```bash
$ doki pull alpine
INFO  resolviendo    alpine:latest  para  linux/arm64
INFO  descargando    layer sha256:abcd...  4.0 MB / 4.0 MB
INFO  descargado     3 layers  total 4.0 MB
```

---

## 4. Ejecutar un Contenedor

```bash
$ doki run --rm alpine echo "hola desde Doki"
hola desde Doki
```

Shell interactiva:

```bash
$ doki run -it --rm alpine /bin/sh
/ # uname -a
Linux localhost 5.15.180-android13-8 aarch64 Android
/ # exit
```

---

## 5. Compatibilidad con Docker CLI

Apunta el Docker CLI estandar al socket de Doki. Sin modificaciones.

```bash
$ export DOCKER_HOST=unix://$HOME/.doki/doki.sock
$ docker ps
$ docker run --rm alpine uname -a
$ docker pull nginx:alpine
$ docker images
```

---

## 6. Stack Compose

```yaml
# docker-compose.yml
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: secret
    depends_on:
      - web
```

```bash
$ doki-compose up -d
INFO  pulling web    nginx:alpine
INFO  pulling db     postgres:16-alpine
INFO  starting web   container=web  port=8080:80
INFO  starting db    container=db
INFO  stack ready    services=2

$ doki-compose ps
NAME   STATUS   PORTS
web    running  0.0.0.0:8080->80/tcp
db     running  5432/tcp

$ doki-compose down
INFO  stopping web
INFO  stopping db
INFO  stack removed
```

---

## 7. Limpieza

```bash
# Eliminar contenedores detenidos.
$ doki container prune

# Eliminar imagenes colgantes.
$ doki image prune

# Limpieza completa del sistema.
$ doki system prune
```

---

## Siguientes Pasos

- [Referencia CLI](CLI-Reference.es) -- catalogo completo de comandos
- [Configuracion](Configuration.es) -- variables de entorno y config.json
- [Networking](Networking.es) -- bridge, DNS, DokiLink mesh
- [Arquitectura](Architecture.es) -- internales del daemon
