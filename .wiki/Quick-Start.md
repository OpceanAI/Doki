# Quick Start

This tutorial takes ~5 minutes and walks you through: install → start daemon → pull image → run container → logs → compose stack → cleanup.

## 0. Prerequisites

- Doki installed (see [Installation](Installation))
- ~100 MB free disk space for the first image

## 1. Verify the Install

```bash
$ doki version
Client: Doki
 Version:    0.9.3
 API version: 1.48
 GitCommit:  faab400
 Built:      2026-06-08
```

If you see the version banner, you're good. If `dokid` is also installed, the same command shows daemon info too.

## 2. Start the Daemon

The `dokid` daemon listens on a Unix socket and exposes the Docker Engine API v1.54.

### Foreground (foreground, see logs)

```bash
$ dokid
INFO  daemon starting  root=/home/user/.doki  socket=/var/run/doki.sock
INFO  storage driver: fuse-overlayfs
INFO  dns server: 127.0.0.11:53
INFO  daemon ready
```

### Background (production / CI)

```bash
$ dokid > /tmp/dokid.log 2>&1 &
$ echo $! > /tmp/dokid.pid
```

### With Docker CLI compatibility

```bash
$ export DOCKER_HOST=unix:///var/run/doki.sock
$ docker ps
CONTAINER ID    IMAGE    COMMAND    CREATED    STATUS    PORTS    NAMES
```

The Docker CLI and SDKs work against Doki without modification.

## 3. Pull an Image

```bash
$ doki pull alpine
INFO  resolving alpine:latest for linux/arm64
INFO  downloading layer sha256:abcd... 4.0 MB / 4.0 MB [====] 1.2s
INFO  downloaded 3 layers, total 4.0 MB
INFO  pulled alpine:latest
```

By default, Doki auto-resolves the manifest for your host architecture. To pull for a different arch:

```bash
$ doki pull --platform linux/amd64 alpine
```

## 4. List Images

```bash
$ doki images
REPOSITORY    TAG       IMAGE ID       CREATED        SIZE
alpine        latest    sha256:abc...  2 minutes ago  4.0 MB
```

## 5. Run a Container

The classic hello-world:

```bash
$ doki run --rm alpine echo "Hello from Doki"
Hello from Doki
```

Run an interactive shell:

```bash
$ doki run -it --rm alpine sh
/ # ls
bin    dev    etc    home   lib    media  mnt    opt    proc   root   run    sbin   srv    sys    tmp    usr    var
/ # exit
```

Run a long-lived container in the background:

```bash
$ doki run -d --name webserver -p 8080:80 nginx:alpine
abc123def456
$ doki ps
CONTAINER ID    IMAGE           COMMAND                 CREATED         STATUS         PORTS                  NAMES
abc123def456    nginx:alpine    "/docker-entrypoint..." 5 seconds ago   Up 4 seconds   0.0.0.0:8080->80/tcp   webserver
```

Test it:

```bash
$ curl -s http://localhost:8080 | head -5
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
...
```

## 6. View Logs

```bash
$ doki logs webserver
/docker-entrypoint.sh: /docker-entrypoint.d/20-envsubst-on-templates.sh: No such file or directory
/docker-entrypoint.sh: Launching /docker-entrypoint.d/30-tune-worker-processes.sh
...
```

Follow the logs (like `tail -f`):

```bash
$ doki logs -f webserver
```

## 7. Stop and Remove

```bash
$ doki stop webserver
webserver

$ doki rm webserver
webserver

$ doki ps -a
CONTAINER ID    IMAGE    COMMAND    CREATED    STATUS    PORTS    NAMES
```

## 8. Multi-Container with Compose

Create `doki-compose.yml` (or `docker-compose.yml` — same format):

```yaml
name: quickstart

services:
  web:
    image: nginx:alpine
    ports: ["8080:80"]
    depends_on:
      api:
        condition: service_started

  api:
    image: python:3-alpine
    command: python -m http.server 8000
    expose: ["8000"]
```

Start it:

```bash
$ doki-compose up -d
[+] Running 2/2
 ✔ Container quickstart-api-1  Started
 ✔ Container quickstart-web-1  Started
```

Check status:

```bash
$ doki-compose ps
NAME                    COMMAND                  SERVICE    STATUS    PORTS
quickstart-api-1        "python -m http.serv..."  api        running   8000/tcp
quickstart-web-1        "/docker-entrypoint..."   web        running   0.0.0.0:8080->80/tcp
```

Tear it down:

```bash
$ doki-compose down
[+] Running 2/2
 ✔ Container quickstart-web-1  Removed
 ✔ Container quickstart-api-1  Removed
 ✔ Network quickstart_default   Removed
```

## 9. Inspect a Container

```bash
$ doki inspect webserver | jq '.[0].State'
{
  "Status": "running",
  "Running": true,
  "Paused": false,
  "Restarting": false,
  "OOMKilled": false,
  "Dead": false,
  "Pid": 12345,
  "ExitCode": 0,
  "StartedAt": "2026-06-04T20:00:00Z",
  "FinishedAt": "0001-01-01T00:00:00Z"
}
```

## 10. Cleanup

```bash
$ doki system prune -a
INFO  removing 3 stopped containers
INFO  removing 2 unused images
INFO  total reclaimed: 145.3 MB
```

## What Just Happened?

You went through the full Doki lifecycle:

| Step | Subsystem | Source code |
|:-----|:----------|:------------|
| Pull | `pkg/registry` + `pkg/image` | OCI Distribution Spec v2 client |
| Run | `pkg/runtime` + `pkg/storage` | OCI Runtime Spec executor |
| Port map | `pkg/network` | Bridge + iptables DNAT |
| Logs | `pkg/runtime` | Multiplexed stream over HTTP |
| Compose | `pkg/compose` | Compose spec engine |
| Inspect | `pkg/api` | Docker Engine API v1.54 |

Continue to [Architecture](Architecture) to understand each subsystem in depth.

## Common Pitfalls

| Problem | Solution |
|:--------|:---------|
| `doki: command not found` | Add `$PREFIX/bin` (Termux) or `/usr/local/bin` (Linux) to `$PATH` |
| `dokid: cannot connect to socket` | Run `dokid &` first |
| `permission denied` on `/var/run/doki.sock` | Add your user to the `docker` group, or set `DOKI_HOST` to a path you own |
| Container exits immediately | Check `doki logs <name>`; usually a missing `CMD` or wrong entrypoint |
| Pull is slow | Add a registry mirror in `config.json` |
| `port 53: permission denied` (Android) | Expected — Doki uses 8053 on Android, no action needed |

## Next Steps

- [Installation](Installation) — per-platform install
- [Architecture](Architecture) — how Doki works under the hood
- [Isolation Levels](Isolation-Levels) — pick the right runtime for your workload
- [CLI Reference](CLI-Reference) — all Docker, Podman, and Kubernetes commands
- [Configuration](Configuration) — `config.json` and env vars
