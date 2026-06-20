# Inicio Rápido

Este tutorial toma ~5 minutos y te lleva por: instalar → iniciar el daemon → pull de imagen → correr contenedor → logs → compose stack → limpieza.

## 0. Prerrequisitos

- Doki instalado (ver [Instalación](Installation.es))
- ~100 MB de espacio libre en disco para la primera imagen

## 1. Verifica la instalación

```bash
$ doki version
Client: Doki
 Version:    0.9.3
 API version: 1.48
 GitCommit:  faab400
 Built:      2026-06-08
```

Si ves el banner de versión, todo bien. Si `dokid` también está instalado, el mismo comando muestra info del daemon.

## 2. Inicia el daemon

El daemon `dokid` escucha en un Unix socket y expone la Docker Engine API v1.54.

### En primer plano (ves los logs)

```bash
$ dokid
INFO  daemon starting  root=/home/user/.doki  socket=/var/run/doki.sock
INFO  storage driver: fuse-overlayfs
INFO  dns server: 127.0.0.11:53
INFO  daemon ready
```

### En segundo plano (producción / CI)

```bash
$ dokid > /tmp/dokid.log 2>&1 &
$ echo $! > /tmp/dokid.pid
```

### Con compatibilidad del CLI de Docker

```bash
$ export DOCKER_HOST=unix:///var/run/doki.sock
$ docker ps
CONTAINER ID    IMAGE    COMMAND    CREATED    STATUS    PORTS    NAMES
```

El CLI y los SDKs de Docker funcionan contra Doki sin modificación.

## 3. Pull de una imagen

```bash
$ doki pull alpine
INFO  resolving alpine:latest for linux/arm64
INFO  downloading layer sha256:abcd... 4.0 MB / 4.0 MB [====] 1.2s
INFO  downloaded 3 layers, total 4.0 MB
INFO  pulled alpine:latest
```

Por defecto, Doki auto-resuelve el manifest para la arquitectura de tu host. Para pull de otra arquitectura:

```bash
$ doki pull --platform linux/amd64 alpine
```

## 4. Lista imágenes

```bash
$ doki images
REPOSITORY    TAG       IMAGE ID       CREATED        SIZE
alpine        latest    sha256:abc...  2 minutes ago  4.0 MB
```

## 5. Corre un contenedor

El clásico hello-world:

```bash
$ doki run --rm alpine echo "Hola desde Doki"
Hola desde Doki
```

Corre una shell interactiva:

```bash
$ doki run -it --rm alpine sh
/ # ls
bin    dev    etc    home   lib    media  mnt    opt    proc   root   run    sbin   srv    sys    tmp    usr    var
/ # exit
```

Corre un contenedor de larga duración en segundo plano:

```bash
$ doki run -d --name webserver -p 8080:80 nginx:alpine
abc123def456
$ doki ps
CONTAINER ID    IMAGE           COMMAND                 CREATED         STATUS         PORTS                  NAMES
abc123def456    nginx:alpine    "/docker-entrypoint..." 5 seconds ago   Up 4 seconds   0.0.0.0:8080->80/tcp   webserver
```

Pruébalo:

```bash
$ curl -s http://localhost:8080 | head -5
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
...
```

## 6. Ver logs

```bash
$ doki logs webserver
/docker-entrypoint.sh: /docker-entrypoint.d/20-envsubst-on-templates.sh: No such file or directory
/docker-entrypoint.sh: Launching /docker-entrypoint.d/30-tune-worker-processes.sh
...
```

Sigue los logs (como `tail -f`):

```bash
$ doki logs -f webserver
```

## 7. Detener y eliminar

```bash
$ doki stop webserver
webserver

$ doki rm webserver
webserver

$ doki ps -a
CONTAINER ID    IMAGE    COMMAND    CREATED    STATUS    PORTS    NAMES
```

## 8. Multi-contenedor con Compose

Crea `doki-compose.yml` (o `docker-compose.yml` — el mismo formato):

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

Inícialo:

```bash
$ doki-compose up -d
[+] Running 2/2
 ✔ Container quickstart-api-1  Started
 ✔ Container quickstart-web-1  Started
```

Verifica el estado:

```bash
$ doki-compose ps
NAME                    COMMAND                  SERVICE    STATUS    PORTS
quickstart-api-1        "python -m http.serv..."  api        running   8000/tcp
quickstart-web-1        "/docker-entrypoint..."   web        running   0.0.0.0:8080->80/tcp
```

Derríbalo:

```bash
$ doki-compose down
[+] Running 2/2
 ✔ Container quickstart-web-1  Removed
 ✔ Container quickstart-api-1  Removed
 ✔ Network quickstart_default   Removed
```

## 9. Inspecciona un contenedor

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

## 10. Limpieza

```bash
$ doki system prune -a
INFO  removing 3 stopped containers
INFO  removing 2 unused images
INFO  total reclaimed: 145.3 MB
```

## ¿Qué acaba de pasar?

Pasaste por el ciclo de vida completo de Doki:

| Paso | Subsistema | Código fuente |
|:-----|:-----------|:--------------|
| Pull | `pkg/registry` + `pkg/image` | Cliente OCI Distribution Spec v2 |
| Run | `pkg/runtime` + `pkg/storage` | Ejecutor OCI Runtime Spec |
| Port map | `pkg/network` | Bridge + iptables DNAT |
| Logs | `pkg/runtime` | Stream multiplexado sobre HTTP |
| Compose | `pkg/compose` | Motor de la spec de Compose |
| Inspect | `pkg/api` | Docker Engine API v1.54 |

Continúa a [Arquitectura](Architecture.es) para entender cada subsistema en profundidad.

## Problemas comunes

| Problema | Solución |
|:---------|:---------|
| `doki: command not found` | Añade `$PREFIX/bin` (Termux) o `/usr/local/bin` (Linux) al `$PATH` |
| `dokid: cannot connect to socket` | Primero corre `dokid &` |
| `permission denied` en `/var/run/doki.sock` | Añade tu usuario al grupo `docker`, o configura `DOKI_HOST` a un path propio |
| El contenedor sale inmediatamente | Revisa `doki logs <nombre>`; usualmente falta `CMD` o entrypoint incorrecto |
| Pull es lento | Añade un mirror de registry en `config.json` |
| `port 53: permission denied` (Android) | Esperado — Doki usa 8053 en Android, no requiere acción |

## Siguientes pasos

- [Instalación](Installation.es) — instalación por plataforma
- [Arquitectura](Architecture.es) — cómo funciona Doki por dentro
- [Niveles de aislamiento](Isolation-Levels.es) — elige el runtime correcto para tu carga
- [Referencia de CLI](CLI-Reference.es) — los 244 comandos
- [Configuración](Configuration.es) — `config.json` y variables de entorno
