# Quick Start

<sub>[5-MINUTE TUTORIAL / v0.11.0]</sub>

> Walkthrough: install, verify, daemon, pull, run, compose, cleanup.
> Tested on Termux/Android 12 aarch64 and Linux AMD64.

---

## 0. Prerequisites

- Doki installed (see [Installation](Installation))
- 100 MB free disk for first image
- `proot` installed on Termux (`pkg install proot`)

---

## 1. Verify the Install

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
[warn] pasta       not found (optional, rootless networking)
[ok]   fuse-overlayfs  /usr/bin/fuse-overlayfs
```

If `doki doctor` reports no `proot` on Termux, install it:

```bash
pkg install proot
```

---

## 2. Start the Daemon

<sub>[FOREGROUND / LOG VISIBILITY]</sub>

```bash
$ dokid
INFO  dokid starting   version=0.11.0  mode=proot
INFO  storage driver   name=fuse-overlayfs
INFO  dns server       listen=127.0.0.11:8053
INFO  doki-link ready  listen=127.0.0.1:7432
INFO  runtime mode     mode=proot
INFO  dokid ready      images=18
```

<sub>[BACKGROUND / PRODUCTION]</sub>

```bash
$ dokid > /tmp/dokid.log 2>&1 &
$ echo $! > /tmp/dokid.pid
```

---

## 3. Pull an Image

```bash
$ doki pull alpine
INFO  resolving    alpine:latest  for  linux/arm64
INFO  downloading  layer sha256:abcd...  4.0 MB / 4.0 MB
INFO  downloaded   3 layers  total 4.0 MB
```

---

## 4. Run a Container

```bash
$ doki run --rm alpine echo "hello from Doki"
hello from Doki
```

Interactive shell:

```bash
$ doki run -it --rm alpine /bin/sh
/ # uname -a
Linux localhost 5.15.180-android13-8 aarch64 Android
/ # exit
```

---

## 5. Docker CLI Compatibility

Point the standard Docker CLI at the Doki socket. No modifications
required.

```bash
$ export DOCKER_HOST=unix://$HOME/.doki/doki.sock
$ docker ps
$ docker run --rm alpine uname -a
$ docker pull nginx:alpine
$ docker images
```

---

## 6. Compose Stack

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

## 7. Cleanup

```bash
# Remove all stopped containers.
$ doki container prune

# Remove dangling images.
$ doki image prune

# Full system cleanup.
$ doki system prune
```

---

## Next Steps

- [CLI Reference](CLI-Reference) -- full command catalog
- [Configuration](Configuration) -- environment variables and config.json
- [Networking](Networking) -- bridge, DNS, DokiLink mesh
- [Architecture](Architecture) -- daemon internals
