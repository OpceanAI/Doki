# Referencia de CLI

Doki v0.9.3 distribuye **244 comandos** en 9 categorías. Esta página es la referencia canónica; el [Inicio Rápido](Quick-Start.es) recorre los comunes.

## Flags globales

Estos flags están disponibles en la mayoría de comandos:

| Flag | Descripción |
|:-----|:-----------|
| `--host string` | Socket del daemon (default: `$DOKI_HOST` o específico de plataforma) |
| `--config string` | Path a `config.json` (default: `~/.doki/config.json`) |
| `-D, --debug` | Habilita logging debug |
| `--tls` | Usa TLS para conectar al daemon |
| `--tlscert string` | Path al cert TLS |
| `--tlskey string` | Path a la key TLS |
| `--tlsverify` | Verifica cert TLS remoto |
| `-H, --human` | Salida human-readable (default: true para `ps`, `images`, `df`) |
| `--format string` | Formato de salida: `text` (default) o `json` |
| `--quiet` | Suprime salida no esencial |

## Gestión de contenedores (17 comandos)

### `doki run [OPTIONS] IMAGE [COMMAND] [ARG...]`

Crea e inicia un contenedor. ~80 flags, los más comunes:

| Flag | Descripción |
|:-----|:-----------|
| `--name string` | Nombre del contenedor |
| `-d, --detach` | Corre en segundo plano |
| `-it` | TTY interactivo (combina `-i` y `-t`) |
| `--rm` | Elimina el contenedor cuando sale |
| `-e, --env list` | Variables de entorno (`KEY=VALUE`) |
| `--env-file list` | Lee env vars desde archivos |
| `-p, --publish list` | Mapeo de puerto (`host:container`) |
| `-P, --publish-all` | Publica todos los puertos `EXPOSE`d |
| `-v, --volume list` | Bind mount (`src:dst[:opts]`) |
| `--mount mount` | Espec de mount (estilo Compose) |
| `-w, --workdir string` | Directorio de trabajo dentro del contenedor |
| `-u, --user string` | Usuario (`uid[:gid]`) |
| `--entrypoint string` | Sobrescribe entrypoint |
| `--network network` | Red a la que attach |
| `--hostname string` | Hostname del contenedor |
| `--domainname string` | Domain name del contenedor |
| `--dns list` | Servidores DNS custom |
| `--dns-search list` | Dominios de búsqueda DNS |
| `--dns-opt list` | Opciones DNS |
| `--add-host list` | Entradas extra en `/etc/hosts` |
| `--restart string` | Política de restart: `no`, `always`, `unless-stopped`, `on-failure[:max]` |
| `--runtime string` | Modo de aislamiento (12 opciones, ver [Niveles de aislamiento](Isolation-Levels.es)) |
| `--init` | Corre `doki-init` como PID 1 (maneja señales, zombies) |
| `--privileged` | Otorga privilegios extendidos |
| `--read-only` | Monta rootfs como read-only |
| `--cap-add list` | Añade capabilities de Linux |
| `--cap-drop list` | Quita capabilities de Linux |
| `--security-opt list` | Opciones de seguridad (seccomp, apparmor) |
| `--sysctl map` | Sysctls (`net.core.somaxconn=512`) |
| `--ulimit ulimit` | Ulimit (`nofile=1024:2048`) |
| `--memory string` | Límite de memoria (`256m`, `1g`) |
| `--cpus string` | Límite de CPU (`0.5`, `2`) |
| `--cpuset-cpus string` | CPUs permitidas (`0-3`) |
| `--shm-size string` | Tamaño de `/dev/shm` (`64m`) |
| `--pids-limit int` | Máx PIDs (negativo = ilimitado) |
| `--blkio-weight uint16` | Peso de block I/O (10-1000) |
| `--device list` | Dispositivos del host a exponer |
| `--tmpfs list` | mounts tmpfs |
| `--log-driver string` | Driver de log: `json-file`, `journald`, `local`, `none` |
| `--log-opt list` | Opciones del driver de log |
| `--label list` | Labels del contenedor |
| `--label-file list` | Lee labels desde archivos |
| `--stop-signal string` | Señal para detener el contenedor (default `SIGTERM`) |
| `--stop-timeout int` | Timeout de stop en segundos |
| `--health-cmd string` | Comando del health check |
| `--health-interval duration` | Intervalo del health check (default 0s = deshabilitado) |
| `--health-timeout duration` | Timeout del health check |
| `--health-retries int` | Reintentos del health check |
| `--health-start-period duration` | Período de gracia inicial |
| `--platform string` | Fuerza plataforma (ej. `linux/amd64`) |
| `--pull string` | Política de pull: `always`, `missing`, `never` |
| `--cidfile string` | Escribe el ID del contenedor a un archivo |
| `--detach-keys string` | Sobrescribe detach keys |

**Ejemplos**:

```bash
# Corre un comando rápido
doki run --rm alpine echo hola

# Shell interactiva
doki run -it --rm alpine sh

# Servidor web de larga duración con port mapping
doki run -d --name web -p 8080:80 nginx:alpine

# Con límites de recursos
doki run -d --name api -m 512m --cpus 1.0 -p 3000:3000 my-api:latest

# Con política de restart
doki run -d --name worker --restart unless-stopped my-worker:latest

# Con health check
doki run -d --name web \
  --health-cmd "wget -q --spider http://localhost/" \
  --health-interval 30s \
  --health-timeout 3s \
  --health-retries 3 \
  nginx:alpine

# Con env file
doki run -d --env-file .env --name api my-api:latest

# Fuerza un nivel de aislamiento específico
doki run --runtime proot --rm alpine echo "usando proot"

# Cross-architecture (usa QEMU User)
doki run --platform linux/amd64 --rm amd64-only-image:latest
```

### `doki ps [OPTIONS]`

Lista contenedores.

| Flag | Descripción |
|:-----|:-----------|
| `-a, --all` | Muestra todos (default: corriendo) |
| `--filter, -f` | Filtra (`status=running`, `name=web`, `label=env=prod`) |
| `--format string` | Go template o `json` |
| `--no-trunc` | No truncar IDs |
| `-n, --last int` | Muestra los últimos n creados (todos los estados) |
| `-l, --latest` | Muestra el último creado |
| `-q, --quiet` | Solo IDs de contenedores |
| `-s, --size` | Muestra tamaños |

**Ejemplos**:

```bash
doki ps                      # corriendo
doki ps -a                  # todos
doki ps -f "status=exited"  # solo exited
doki ps -f "label=env=prod" # por label
doki ps --format json       # JSON
doki ps --format "{{.Names}}: {{.Status}}"
```

### `doki create [OPTIONS] IMAGE [COMMAND] [ARG...]`

Crea un contenedor sin iniciarlo. Mismos flags que `doki run` excepto `-d`/`-it`/políticas de restart. Usa `doki start <id>` para iniciarlo después.

### `doki start [OPTIONS] CONTAINER [CONTAINER...]`

Inicia uno o más contenedores detenidos.

| Flag | Descripción |
|:-----|:-----------|
| `-a, --attach` | Attach STDOUT/STDERR |
| `-i, --interactive` | Attach STDIN |

### `doki stop [OPTIONS] CONTAINER [CONTAINER...]`

Detiene ordenadamente contenedores corriendo. Envía `SIGTERM`, espera `--stop-timeout` (default 10s), luego `SIGKILL`.

| Flag | Descripción |
|:-----|:-----------|
| `-t, --time int` | Segundos a esperar antes del kill |
| `--signal string` | Señal de stop custom |

### `doki restart [OPTIONS] CONTAINER [CONTAINER...]`

Detiene e inicia. Mismos flags que `stop`.

### `doki kill [OPTIONS] CONTAINER`

Envía una señal a un contenedor corriendo.

| Flag | Descripción |
|:-----|:-----------|
| `-s, --signal string` | Señal a enviar (default `SIGKILL`) |

### `doki rm [OPTIONS] CONTAINER [CONTAINER...]`

Elimina contenedores.

| Flag | Descripción |
|:-----|:-----------|
| `-f, --force` | Fuerza eliminación de contenedor corriendo (SIGKILL primero) |
| `-v, --volumes` | Elimina volúmenes anónimos |
| `-l, --link` | Elimina el link especificado |

### `doki exec [OPTIONS] CONTAINER COMMAND [ARG...]`

Corre un comando en un contenedor corriendo.

| Flag | Descripción |
|:-----|:-----------|
| `-d, --detach` | Detached |
| `-i, --interactive` | Mantiene STDIN abierto |
| `-t, --tty` | Asigna un pseudo-TTY |
| `-e, --env list` | Variables de entorno |
| `-w, --workdir string` | Directorio de trabajo |
| `-u, --user string` | Usuario |
| `--privileged` | Privilegios extendidos |
| `--detach-keys string` | Sobrescribe detach keys |

**Ejemplos**:

```bash
doki exec web ls /var/log
doki exec -it web sh
doki exec -u postgres db psql -U postgres
doki exec -e DEBUG=1 api /bin/debug
```

### `doki logs [OPTIONS] CONTAINER`

Obtiene logs del contenedor.

| Flag | Descripción |
|:-----|:-----------|
| `-f, --follow` | Sigue la salida (como `tail -f`) |
| `--since string` | Muestra logs desde timestamp (`2024-01-01T00:00:00`) o relativo (`10m`) |
| `--until string` | Muestra logs antes del timestamp |
| `-t, --timestamps` | Muestra timestamps |
| `--tail string` | Número de líneas desde el final (`all` para todo) |
| `--details` | Muestra detalles extra |

**Ejemplos**:

```bash
doki logs web
doki logs -f web
doki logs --since 10m web
doki logs --tail 100 -f web
doki logs --since 2024-01-01T00:00:00 --until 2024-01-02T00:00:00 web
```

### `doki stats [OPTIONS] [CONTAINER...]`

Estadísticas de recursos en vivo.

| Flag | Descripción |
|:-----|:-----------|
| `-a, --all` | Todos los contenedores (default: corriendo) |
| `--no-stream` | Snapshot único, sin updates en vivo |
| `--format string` | Go template o `json` |

**Salida de ejemplo**:

```
CONTAINER    CPU %   MEM USAGE / LIMIT   MEM %   NET I/O       BLOCK I/O
web          0.05%   12.3 MiB / 1 GiB     1.20%   1.2kB / 0B    0B / 0B
db           1.23%   156 MiB / 512 MiB    30.4%   5.6MB / 4MB   1MB / 8MB
```

### `doki top CONTAINER [PS_OPTIONS]`

Muestra procesos corriendo en un contenedor. Pasa a `ps` dentro del contenedor.

```bash
doki top web
doki top web aux
```

### `doki inspect [OPTIONS] NAME|ID [NAME|ID...]`

Información detallada en JSON sobre un contenedor, imagen, red, volumen, etc.

| Flag | Descripción |
|:-----|:-----------|
| `-f, --format string` | Go template |
| `-s, --size` | Muestra tamaño (para imágenes) |
| `--type string` | Limita a tipo: `container`, `image`, `network`, `volume` |

**Ejemplos**:

```bash
doki inspect web
doki inspect --format '{{.State.Status}}' web
doki inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' web
```

### `doki build [OPTIONS] PATH | URL | -`

Construye una imagen desde un Dokifile.

| Flag | Descripción |
|:-----|:-----------|
| `-f, --file string` | Path al Dokifile (default `Dokifile` en el contexto) |
| `-t, --tag list` | Nombre y tag (`name:tag`) |
| `--build-arg list` | Variables de build-time |
| `--target string` | Stage target para builds multi-stage |
| `--no-cache` | Deshabilita caché de build |
| `--pull` | Siempre pull de la imagen base |
| `--platform list` | Plataformas target |
| `--secret id=id,src=path` | Secreto BuildKit |
| `--ssh string` | Socket de agente SSH o key |

**Ejemplo**:

```bash
doki build -t myapp:1.0 .
doki build -t myapp:1.0 -f Dockerfile.prod .
doki build --build-arg VERSION=1.2.3 -t myapp:1.2.3 .
doki build --target runner -t myapp:runner .
doki build --platform linux/amd64,linux/arm64 -t myapp:multi .
```

### `doki commit [OPTIONS] CONTAINER [IMAGE[:TAG]]`

Crea una imagen desde los cambios de un contenedor.

| Flag | Descripción |
|:-----|:-----------|
| `-a, --author string` | Autor |
| `-m, --message string` | Mensaje del commit |
| `-c, --change list` | Aplica instrucción de Dockerfile |
| `-p, --pause` | Pausa el contenedor durante el commit |

### `doki attach [OPTIONS] CONTAINER`

Attach al I/O de un contenedor corriendo. Usa `--detach-keys` (default `ctrl-p,ctrl-q`) para detacharte.

| Flag | Descripción |
|:-----|:-----------|
| `--detach-keys string` | Sobrescribe detach keys |
| `--no-stdin` | No attach STDIN |
| `--sig-proxy` | Proxy de señales (default true) |

### `doki wait CONTAINER [CONTAINER...]`

Bloquea hasta que uno o más contenedores se detengan, luego imprime sus exit codes.

```bash
$ doki run -d --name web nginx:alpine
abc123
$ doki wait web
0
```

## Gestión de imágenes (8 comandos)

### `doki pull [OPTIONS] NAME[:TAG|@DIGEST]`

Pull de una imagen desde un registry.

| Flag | Descripción |
|:-----|:-----------|
| `--platform list` | Plataformas específicas |
| `-a, --all-tags` | Todas las imágenes taggeadas en el repo |
| `--quiet` | Suprime output de progreso |

```bash
doki pull alpine
doki pull alpine:3.19
doki pull --platform linux/amd64 alpine
doki pull ghcr.io/owner/repo:tag
```

### `doki push NAME[:TAG]`

Push de una imagen a un registry.

| Flag | Descripción |
|:-----|:-----------|
| `--quiet` | Suprime output de progreso |

```bash
doki push myuser/myapp:1.0
doki push ghcr.io/owner/repo:tag
```

### `doki images [OPTIONS] [REPOSITORY[:TAG]]`

Lista imágenes.

| Flag | Descripción |
|:-----|:-----------|
| `-a, --all` | Muestra todas (default: imágenes intermedias ocultas) |
| `--digests` | Muestra digests |
| `-f, --filter` | Filtra |
| `--format string` | Go template o `json` |
| `--no-trunc` | No truncar |
| `-q, --quiet` | Solo IDs de imagen |

### `doki rmi [OPTIONS] IMAGE [IMAGE...]`

Elimina imágenes.

| Flag | Descripción |
|:-----|:-----------|
| `-f, --force` | Fuerza eliminación |
| `--no-prune` | No borra parents sin tag |

### `doki tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

Tagea una imagen.

```bash
doki tag myapp:1.0 myapp:latest
doki tag myapp:1.0 ghcr.io/owner/myapp:1.0
```

### `doki login [OPTIONS] [SERVER]`

Log in a un registry. Almacena credenciales en `~/.doki/config.json` o el keyring del sistema.

| Flag | Descripción |
|:-----|:-----------|
| `-u, --username string` | Usuario |
| `-p, --password string` | Password (preferir env var o prompt) |
| `--password-stdin` | Lee password desde stdin |

### `doki logout [SERVER]`

Log out. Elimina credenciales almacenadas.

### `doki search [OPTIONS] TERM`

Busca en Docker Hub.

| Flag | Descripción |
|:-----|:-----------|
| `--limit int` | Máx resultados (default 25) |
| `--filter stars=100` | Filtra por stars |
| `--format string` | Go template o `json` |

## Gestión de redes (7 comandos)

### `doki network ls`

Lista redes. Flags: `--filter`, `--format`, `--quiet`, `--no-trunc`.

### `doki network create [OPTIONS] NETWORK`

Crea una red.

| Flag | Descripción |
|:-----|:-----------|
| `--driver string` | Bridge, host, none, macvlan, ipvlan (default `bridge`) |
| `--gateway string` | Gateway para la subnet |
| `--subnet string` | Subnet en CIDR |
| `--ip-range string` | Rango de IPs a asignar |
| `--opt list` | Opciones del driver |
| `--ipv6` | Habilita IPv6 |
| `--label list` | Labels |
| `--internal` | Restringe acceso externo |

```bash
doki network create backend
doki network create --driver bridge --subnet 10.1.0.0/16 isolated
doki network create --ipv6 dualstack
```

### `doki network rm NETWORK [NETWORK...]`

Elimina redes.

### `doki network inspect NETWORK [NETWORK...]`

Info detallada. Flags: `--format`, `--verbose`.

### `doki network connect [OPTIONS] NETWORK CONTAINER`

Attach un contenedor a una red.

| Flag | Descripción |
|:-----|:-----------|
| `--ip string` | IPv4 específica |
| `--ip6 string` | IPv6 específica |
| `--alias list` | Aliases de red |
| `--link list` | Añade link a otro contenedor |

### `doki network disconnect [OPTIONS] NETWORK CONTAINER`

Detacha un contenedor de una red.

| Flag | Descripción |
|:-----|:-----------|
| `-f, --force` | Fuerza |

### `doki network prune [OPTIONS]`

Elimina todas las redes no usadas.

| Flag | Descripción |
|:-----|:-----------|
| `--filter` | Filtra |
| `-f, --force` | No prompt |

## Gestión de volúmenes (4 comandos)

### `doki volume ls`

Lista volúmenes. Flags: `--filter`, `--format`, `--quiet`.

### `doki volume create [OPTIONS] [NAME]`

Crea un volumen.

| Flag | Descripción |
|:-----|:-----------|
| `--driver string` | Driver de volumen (default `local`) |
| `--opt list` | Opciones del driver |
| `--label list` | Labels |

```bash
doki volume create db-data
doki volume create --driver local --opt type=nfs --opt o=addr=10.0.0.1,rw nfs-vol
```

### `doki volume rm VOLUME [VOLUME...]`

Elimina volúmenes. `-f` para forzar.

### `doki volume inspect VOLUME [VOLUME...]`

Info detallada.

## Sistema (8 comandos)

### `doki info`

Información del sistema (driver, cgroup, kernel, etc.).

### `doki version`

Muestra versión del cliente y (si `dokid` está corriendo) del server. Incluye GitCommit, BuildDate.

### `doki system df`

Uso de disco por imágenes, contenedores, volúmenes, build cache.

### `doki system prune [OPTIONS]`

Elimina todos los contenedores detenidos, imágenes dangling, redes no usadas, build cache.

| Flag | Descripción |
|:-----|:-----------|
| `-a, --all` | Elimina todas las imágenes no usadas (no solo dangling) |
| `--filter` | Filtra |
| `-f, --force` | No prompt |
| `--volumes` | Prunea volúmenes |

### `doki system events [OPTIONS]`

Eventos en tiempo real del servidor.

| Flag | Descripción |
|:-----|:-----------|
| `--since string` | Eventos desde |
| `--until string` | Eventos hasta |
| `--filter` | Filtra (`type=container`, `event=start`) |
| `--format string` | JSON o template |

### `doki ping`

Ping al daemon. Retorna `OK` en éxito.

### `doki info --format '{{.DriverStatus}}'`

Muestra estado del driver de storage.

### `doki container prune` / `doki image prune` / `doki volume prune` / `doki network prune`

Prunes específicos por tipo.

## Compatibilidad con Podman (15 comandos)

Para usuarios migrando desde Podman:

| Comando | Descripción |
|:--------|:-----------|
| `doki pod create` | Crea un pod (grupo de contenedores) |
| `doki pod ls` | Lista pods |
| `doki pod rm` | Elimina pod |
| `doki pod start` | Inicia pod |
| `doki pod stop` | Detiene pod |
| `doki pod inspect` | Inspecciona pod |
| `doki pod ps` | Lista pods con estado |
| `doki generate kube` | Genera YAML de Kubernetes desde contenedor corriendo |
| `doki play kube` | Corre YAML de Kubernetes |
| `doki auto-update` | Auto-update de contenedores desde registry |
| `doki unshare` | Corre comando en namespaces del contenedor |
| `doki untag` | Quita tag de imagen |
| `doki mount` / `unmount` | Monta/desmonta filesystem del contenedor |
| `doki healthcheck` | Corre health check manualmente |

## Compatibilidad con Kubernetes (4 comandos)

| Comando | Descripción |
|:--------|:-----------|
| `doki kube play` | Corre YAML de pod de Kubernetes |
| `doki kube down` | Detiene y elimina pod |
| `doki kube generate` | Genera YAML de K8s desde compose |
| `doki apply -f` | Aplica YAML de Kubernetes (alias de `kube play`) |

## DokiLink Mesh (6 comandos)

DokiLink-Lite proporciona networking mesh peer-to-peer con TLS 1.3 y encriptación NaCl secretbox opcional.

| Comando | Descripción |
|:--------|:-----------|
| `doki mesh ls` | Lista peers conectados al mesh |
| `doki mesh status` | Muestra estado del mesh y public key |
| `doki link add NAME ADDRESS` | Añade un peer confiable (TOFU) |
| `doki link rm NAME` | Elimina un peer |
| `doki link show NAME` | Muestra detalles de un peer |

**Ejemplos**:

```bash
# Muestra tu public key
doki mesh status

# Añade un peer
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# Lista peers
doki mesh ls

# Elimina un peer
doki link rm mybuddy
```

### Flags globales de DokiLink

| Flag | Descripción |
|:-----|:-----------|
| `--doki-link-addr string` | Dirección de escucha gossip (default `:7432`) |
| `--doki-link-payload-enc` | Habilita encriptación NaCl secretbox de payload (L2) |
| `--doki-link-mesh 0` | Deshabilita mesh completamente |
| `--doki-use-socat 1` | Fuerza fallback a socat para port forwarding |

## Compose (`doki-compose`, ~30 subcomandos)

`doki-compose` es un binario separado con su propio set de comandos. Top-level:

| Comando | Descripción |
|:--------|:-----------|
| `doki-compose up` | Crea e inicia servicios |
| `doki-compose down` | Detiene y elimina servicios, redes |
| `doki-compose ps` | Lista contenedores |
| `doki-compose logs` | Ve logs (multi-servicio) |
| `doki-compose exec` | Corre comando en servicio |
| `doki-compose run` | Corre comando one-off |
| `doki-compose start` / `stop` / `restart` | Ciclo de vida |
| `doki-compose pull` | Pull de todas las imágenes |
| `doki-compose build` | Construye todas las imágenes |
| `doki-compose config` | Valida y ve config |
| `doki-compose ls` | Lista proyectos compose |
| `doki-compose scale` | Escala servicios |
| `doki-compose top` | Muestra procesos corriendo |
| `doki-compose pause` / `unpause` | Pausa/despausa |
| `doki-compose kill` | Envía señal |
| `doki-compose rm` | Elimina contenedores detenidos |
| `doki-compose port` | Imprime mapeo de puerto |
| `doki-compose images` | Lista imágenes usadas |
| `doki-compose version` | Info de versión |

## Formatos de salida

La mayoría de comandos `ls`/`ps`/`images`/`volume` soportan `--format` con un Go template:

```bash
doki ps --format '{{.ID}}: {{.Image}} -> {{.Status}}'
doki images --format '{{.Repository}}:{{.Tag}} ({{.Size}})'
doki inspect --format '{{json .State}}' web
```

`--format json` emite JSON para pipear a `jq`.

## Códigos de salida

| Código | Significado |
|:------:|:------------|
| 0 | Éxito |
| 1 | Error general |
| 2 | Mal uso del comando |
| 125 | Error del daemon |
| 126 | Comando del contenedor no ejecutable |
| 127 | Comando del contenedor no encontrado |
| 130 | Contenedor recibió SIGINT (128+2) |
| 137 | Contenedor recibió SIGKILL (128+9) |
| 143 | Contenedor recibió SIGTERM (128+15) |

## Fuente

- CLI: `cmd/doki/main.go` (cobra)
- Comandos: `cmd/doki/*.go` (un archivo por grupo de comandos)
- Flags compartidos: `pkg/cli/flags.go`
- API del daemon: `pkg/api/*.go`
