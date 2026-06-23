# Referencia de CLI

<sub>[DOC: REFERENCIA-CLI]</sub>

Doki v0.11.0. Catálogo canónico de comandos. 244 comandos en 10 categorías. El [Inicio Rápido](Quick-Start.es) cubre el uso común.

<hr>

## Flags globales

<sub>[GLOBAL]</sub>

Disponibles en la mayoría de comandos.

```text
FLAG               DESCRIPCION
────────────────────────────────────────────────────────────
--host string      Socket del daemon (default: $DOKI_HOST o especifico de plataforma)
--config string    Path a config.json (default: ~/.doki/config.json)
-D, --debug        Habilita logging debug
--tls              Usa TLS para conectar al daemon
--tlscert string   Path al cert TLS
--tlskey string    Path a la key TLS
--tlsverify        Verifica cert TLS remoto
-H, --human        Salida human-readable (default: true para ps, images, df)
--format string    Formato de salida: text (default) o json
--quiet            Suprime salida no esencial
```

<hr>

## Catalogo de comandos

<sub>[CATALOGO]</sub>

Comandos agrupados por categoria. Sintaxis seguida de una breve descripcion.

```text
COMANDOS DE CONTENEDOR
────────────────────────────────────────────────────────────────────────────
doki run [OPTIONS] IMAGE [CMD] [ARG...]   Crea e inicia un contenedor
doki create [OPTIONS] IMAGE [CMD]         Crea contenedor sin iniciarlo
doki start [OPTIONS] CONTAINER [...]       Inicia uno o mas contenedores detenidos
doki stop [OPTIONS] CONTAINER [...]        Detiene ordenadamente (SIGTERM luego SIGKILL)
doki restart [OPTIONS] CONTAINER [...]     Detiene e inicia contenedores
doki kill [OPTIONS] CONTAINER              Envia senal a un contenedor corriendo
doki rm [OPTIONS] CONTAINER [...]           Elimina contenedores
doki exec [OPTIONS] CONTAINER CMD [ARG...] Corre comando en contenedor corriendo
doki logs [OPTIONS] CONTAINER              Obtiene logs del contenedor
doki stats [OPTIONS] [CONTAINER...]        Estadisticas de recursos en vivo
doki top CONTAINER [PS_OPTIONS]             Muestra procesos corriendo en contenedor
doki inspect [OPTIONS] NAME|ID [...]        Informacion detallada en JSON
doki build [OPTIONS] PATH|URL|-              Construye imagen desde un Dokifile
doki commit [OPTIONS] CONTAINER [IMAGE]      Crea imagen desde cambios del contenedor
doki attach [OPTIONS] CONTAINER              Attach al I/O del contenedor
doki wait CONTAINER [...]                    Bloquea hasta que se detengan, imprime exit codes
doki ps [OPTIONS]                            Lista contenedores
```

```text
COMANDOS DE IMAGEN
────────────────────────────────────────────────────────────────────────────
doki pull [OPTIONS] NAME[:TAG|@DIGEST]   Pull de imagen desde registry
doki push NAME[:TAG]                    Push de imagen a registry
doki images [OPTIONS] [REPOSITORY]      Lista imagenes
doki rmi [OPTIONS] IMAGE [...]           Elimina imagenes
doki tag SOURCE[:TAG] TARGET[:TAG]      Tagea una imagen
doki login [OPTIONS] [SERVER]           Log in a registry
doki logout [SERVER]                    Log out, elimina credenciales
doki search [OPTIONS] TERM               Busca en Docker Hub
```

```text
COMANDOS DE RED
────────────────────────────────────────────────────────────────────────────
doki network ls                          Lista redes
doki network create [OPTIONS] NETWORK    Crea una red
doki network rm NETWORK [...]             Elimina redes
doki network inspect NETWORK [...]        Info detallada de red
doki network connect [OPTIONS] NET CONT  Attachea contenedor a red
doki network disconnect [OPTIONS] NET C   Detacha contenedor de red
doki network prune [OPTIONS]            Elimina redes no usadas
```

```text
COMANDOS DE VOLUMEN
────────────────────────────────────────────────────────────────────────────
doki volume ls                           Lista volumenes
doki volume create [OPTIONS] [NAME]      Crea un volumen
doki volume rm VOLUME [...]              Elimina volumenes (-f para forzar)
doki volume inspect VOLUME [...]          Info detallada
```

```text
COMANDOS DE SISTEMA
────────────────────────────────────────────────────────────────────────────
doki info                                Informacion del sistema (driver, cgroup, kernel)
doki version                             Muestra version cliente y server
doki system df                           Uso de disco por imagenes, contenedores, cache
doki system prune [OPTIONS]             Elimina detenidos, dangling, no usados
doki system events [OPTIONS]            Eventos en tiempo real del server
doki ping                                Ping al daemon (retorna OK)
doki container prune                    Prunea contenedores detenidos
doki image prune                        Prunea imagenes no usadas
doki volume prune                       Prunea volumenes no usados
doki network prune                      Prunea redes no usadas
```

```text
COMANDOS DE POD
────────────────────────────────────────────────────────────────────────────
doki pod create                          Crea un pod (grupo de contenedores)
doki pod ls                              Lista pods
doki pod ps                              Lista pods con estado
doki pod rm                              Elimina pod
doki pod start                           Inicia pod
doki pod stop                            Detiene pod
doki pod inspect                         Inspecciona pod
doki generate kube                      Genera YAML de Kubernetes desde contenedor
doki play kube                           Corre YAML de Kubernetes
doki auto-update                         Auto-update de contenedores desde registry
doki unshare                             Corre comando en namespaces del contenedor
doki untag                               Quita tag de imagen
doki mount                               Monta filesystem del contenedor
doki unmount                             Desmonta filesystem del contenedor
doki healthcheck                         Corre health check manualmente
```

```text
COMANDOS DE COMPOSE
────────────────────────────────────────────────────────────────────────────
doki-compose up                          Crea e inicia servicios
doki-compose down                        Detiene y elimina servicios, redes
doki-compose ps                          Lista contenedores
doki-compose logs                        Ve logs (multi-servicio)
doki-compose exec                        Corre comando en servicio
doki-compose run                         Corre comando one-off
doki-compose start                       Ciclo de vida: start
doki-compose stop                        Ciclo de vida: stop
doki-compose restart                     Ciclo de vida: restart
doki-compose pull                        Pull de todas las imagenes
doki-compose build                       Construye todas las imagenes
doki-compose config                      Valida y ve config
doki-compose ls                          Lista proyectos compose
doki-compose scale                       Escala servicios
doki-compose top                         Muestra procesos corriendo
doki-compose pause                       Pausa servicios
doki-compose unpause                     Despausa servicios
doki-compose kill                        Envia senal
doki-compose rm                          Elimina contenedores detenidos
doki-compose port                        Imprime mapeo de puerto
doki-compose images                      Lista imagenes usadas
doki-compose version                     Info de version
```

```text
COMANDOS DE KUBERNETES
────────────────────────────────────────────────────────────────────────────
doki kube play                           Corre YAML de pod de Kubernetes
doki kube down                           Detiene y elimina pod
doki kube generate                       Genera YAML de K8s desde compose
doki apply -f FILE                       Aplica YAML de Kubernetes (alias de kube play)
```

```text
COMANDOS DE MESH/LINK
────────────────────────────────────────────────────────────────────────────
doki mesh ls                             Lista peers conectados al mesh
doki mesh status                         Muestra estado del mesh y public key
doki link add NAME ADDRESS               Anade peer confiable (TOFU)
doki link rm NAME                        Elimina un peer
doki link show NAME                      Muestra detalles de un peer
```

```text
COMANDOS DE DEPS
────────────────────────────────────────────────────────────────────────────
doki deps ls                             Lista dependencias de runtime detectadas
doki deps check [N...]                   Verifica presencia y version de dependencias
doki deps go [NAME]                      Imprime el path que Doki invocara para una dependencia
doki deps install [N...]                Instala dependencias faltantes via package manager
```

<hr>

## doki run

<sub>[RUN]</sub>

Crea e inicia un contenedor. Acepta aproximadamente 80 flags. Subconjunto comun abajo.

```text
FLAG                          DESCRIPCION
──────────────────────────────────────────────────────────────
--name string                 Nombre del contenedor
-d, --detach                  Corre en segundo plano
-it                           TTY interactivo (combina -i y -t)
--rm                          Elimina el contenedor cuando sale
-e, --env list                Variables de entorno (KEY=VALUE)
--env-file list               Lee env vars desde archivos
-p, --publish list            Mapeo de puerto (host:container)
-P, --publish-all             Publica todos los puertos EXPOSEd
-v, --volume list             Bind mount (src:dst[:opts])
--mount mount                 Espec de mount (estilo Compose)
-w, --workdir string          Directorio de trabajo dentro del contenedor
-u, --user string             Usuario (uid[:gid])
--entrypoint string           Sobrescribe entrypoint
--network network             Red a la que attach
--hostname string             Hostname del contenedor
--domainname string           Domain name del contenedor
--dns list                    Servidores DNS custom
--dns-search list             Dominios de busqueda DNS
--dns-opt list                Opciones DNS
--add-host list               Entradas extra en /etc/hosts
--restart string              Politica: no, always, unless-stopped, on-failure[:max]
--runtime string              Modo de aislamiento (12 opciones, ver Isolation-Levels.es)
--init                        Corre doki-init como PID 1 (senales, zombies)
--privileged                  Otorga privilegios extendidos
--read-only                   Monta rootfs como read-only
--cap-add list                Anade capabilities de Linux
--cap-drop list               Quita capabilities de Linux
--security-opt list           Opciones de seguridad (seccomp, apparmor)
--sysctl map                  Sysctls (net.core.somaxconn=512)
--ulimit ulimit               Ulimit (nofile=1024:2048)
--memory string               Limite de memoria (256m, 1g)
--cpus string                 Limite de CPU (0.5, 2)
--cpuset-cpus string          CPUs permitidas (0-3)
--shm-size string             Tamano de /dev/shm (64m)
--pids-limit int              Max PIDs (negativo = ilimitado)
--blkio-weight uint16         Peso de block I/O (10-1000)
--device list                 Dispositivos del host a exponer
--tmpfs list                  mounts tmpfs
--log-driver string           Driver: json-file, journald, local, none
--log-opt list                Opciones del driver de log
--label list                  Labels del contenedor
--label-file list             Lee labels desde archivos
--stop-signal string          Senal para detener (default SIGTERM)
--stop-timeout int            Timeout de stop en segundos
--health-cmd string           Comando del health check
--health-interval duration    Intervalo del health check (default 0s = deshabilitado)
--health-timeout duration     Timeout del health check
--health-retries int          Reintentos del health check
--health-start-period d       Periodo de gracia inicial
--platform string             Fuerza plataforma (ej. linux/amd64)
--pull string                 Politica de pull: always, missing, never
--cidfile string               Escribe ID del contenedor a archivo
--detach-keys string          Sobrescribe detach keys
```

Ejemplos:

```bash
doki run --rm alpine echo hola
doki run -it --rm alpine sh
doki run -d --name web -p 8080:80 nginx:alpine
doki run -d --name api -m 512m --cpus 1.0 -p 3000:3000 my-api:latest
doki run -d --name worker --restart unless-stopped my-worker:latest
doki run -d --name web \
  --health-cmd "wget -q --spider http://localhost/" \
  --health-interval 30s \
  --health-timeout 3s \
  --health-retries 3 \
  nginx:alpine
doki run -d --env-file .env --name api my-api:latest
doki run --runtime proot --rm alpine echo "usando proot"
doki run --platform linux/amd64 --rm amd64-only-image:latest
```

<hr>

## doki ps

<sub>[PS]</sub>

Lista contenedores.

```text
FLAG              DESCRIPCION
─────────────────────────────────────────
-a, --all         Muestra todos (default: corriendo)
--filter, -f      Filtra (status=running, name=web, label=env=prod)
--format string   Go template o json
--no-trunc        No truncar IDs
-n, --last int    Muestra los ultimos n creados (todos los estados)
-l, --latest      Muestra el ultimo creado
-q, --quiet       Solo IDs de contenedores
-s, --size        Muestra tamanos
```

Ejemplos:

```bash
doki ps
doki ps -a
doki ps -f "status=exited"
doki ps -f "label=env=prod"
doki ps --format json
doki ps --format "{{.Names}}: {{.Status}}"
```

<hr>

## doki start

<sub>[START]</sub>

Inicia contenedores detenidos.

```text
FLAG              DESCRIPCION
─────────────────────────────────────────
-a, --attach      Attach STDOUT/STDERR
-i, --interactive Attach STDIN
```

<hr>

## doki stop

<sub>[STOP]</sub>

Detencion ordenada. Envia SIGTERM, espera <kbd>--stop-timeout</kbd> (default 10s), luego SIGKILL.

```text
FLAG              DESCRIPCION
─────────────────────────────────────────
-t, --time int    Segundos a esperar antes del kill
--signal string   Senal de stop custom
```

<hr>

## doki kill

<sub>[KILL]</sub>

Envia senal a contenedor corriendo.

```text
FLAG                 DESCRIPCION
─────────────────────────────────────────
-s, --signal string  Senal a enviar (default SIGKILL)
```

<hr>

## doki rm

<sub>[RM]</sub>

Elimina contenedores.

```text
FLAG          DESCRIPCION
──────────────────────────────────
-f, --force   Fuerza eliminacion de contenedor corriendo (SIGKILL primero)
-v, --volumes  Elimina volumenes anonimos
-l, --link    Elimina el link especificado
```

<hr>

## doki exec

<sub>[EXEC]</sub>

Corre comando en contenedor corriendo.

```text
FLAG                DESCRIPCION
──────────────────────────────────────────
-d, --detach        Detached
-i, --interactive   Mantiene STDIN abierto
-t, --tty           Asigna pseudo-TTY
-e, --env list      Variables de entorno
-w, --workdir str   Directorio de trabajo
-u, --user string   Usuario
--privileged        Privilegios extendidos
--detach-keys str   Sobrescribe detach keys
```

Ejemplos:

```bash
doki exec web ls /var/log
doki exec -it web sh
doki exec -u postgres db psql -U postgres
doki exec -e DEBUG=1 api /bin/debug
```

<hr>

## doki logs

<sub>[LOGS]</sub>

Obtiene logs del contenedor.

```text
FLAG              DESCRIPCION
─────────────────────────────────────────
-f, --follow      Sigue la salida (tail -f)
--since string    Logs desde timestamp o relativo (10m)
--until string    Logs antes del timestamp
-t, --timestamps  Muestra timestamps
--tail string     Lineas desde el final (all para todo)
--details         Muestra detalles extra
```

Ejemplos:

```bash
doki logs web
doki logs -f web
doki logs --since 10m web
doki logs --tail 100 -f web
doki logs --since 2024-01-01T00:00:00 --until 2024-01-02T00:00:00 web
```

<hr>

## doki stats

<sub>[STATS]</sub>

Estadisticas de recursos en vivo.

```text
FLAG            DESCRIPCION
───────────────────────────────────────
-a, --all       Todos los contenedores (default: corriendo)
--no-stream     Snapshot unico, sin updates en vivo
--format str    Go template o json
```

Salida de ejemplo:

```text
CONTAINER    CPU %   MEM USAGE / LIMIT   MEM %   NET I/O       BLOCK I/O
web          0.05%   12.3 MiB / 1 GiB     1.20%   1.2kB / 0B    0B / 0B
db           1.23%   156 MiB / 512 MiB    30.4%   5.6MB / 4MB   1MB / 8MB
```

<hr>

## doki top

<sub>[TOP]</sub>

Muestra procesos corriendo en contenedor. Pasa a <kbd>ps</kbd> interno.

```bash
doki top web
doki top web aux
```

<hr>

## doki inspect

<sub>[INSPECT]</sub>

Informacion detallada en JSON sobre contenedor, imagen, red, volumen.

```text
FLAG              DESCRIPCION
─────────────────────────────────────────
-f, --format str  Go template
-s, --size        Muestra tamano (imagenes)
--type string     Limita a tipo: container, image, network, volume
```

Ejemplos:

```bash
doki inspect web
doki inspect --format '{{.State.Status}}' web
doki inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' web
```

<hr>

## doki create

<sub>[CREATE]</sub>

Crea contenedor sin iniciarlo. Mismos flags que <kbd>run</kbd> excepto <kbd>-d</kbd>, <kbd>-it</kbd>, politicas de restart. Usa <kbd>doki start</kbd> para iniciarlo.

<hr>

## doki restart

<sub>[RESTART]</sub>

Detiene e inicia. Mismos flags que <kbd>stop</kbd>.

<hr>

## doki build

<sub>[BUILD]</sub>

Construye imagen desde Dokifile.

```text
FLAG                DESCRIPCION
────────────────────────────────────────────────
-f, --file string   Path al Dokifile (default Dokifile en el contexto)
-t, --tag list      Nombre y tag (name:tag)
--build-arg list    Variables de build-time
--target string     Stage target para multi-stage builds
--no-cache          Deshabilita cache de build
--pull              Siempre pull de la imagen base
--platform list     Plataformas target
--secret id=id      Secreto BuildKit
--ssh string        Socket de agente SSH o key
```

Ejemplos:

```bash
doki build -t myapp:1.0 .
doki build -t myapp:1.0 -f Dockerfile.prod .
doki build --build-arg VERSION=1.2.3 -t myapp:1.2.3 .
doki build --target runner -t myapp:runner .
doki build --platform linux/amd64,linux/arm64 -t myapp:multi .
```

<hr>

## doki commit

<sub>[COMMIT]</sub>

Crea imagen desde cambios de un contenedor.

```text
FLAG                DESCRIPCION
─────────────────────────────────────────────
-a, --author str    Autor
-m, --message str   Mensaje del commit
-c, --change list   Aplica instruccion de Dockerfile
-p, --pause         Pausa el contenedor durante el commit
```

<hr>

## doki attach

<sub>[ATTACH]</sub>

Attach al I/O de contenedor corriendo. Detach con <kbd>ctrl-p,ctrl-q</kbd> o <kbd>--detach-keys</kbd> custom.

```text
FLAG               DESCRIPCION
────────────────────────────────────────────
--detach-keys str  Sobrescribe detach keys
--no-stdin         No attach STDIN
--sig-proxy        Proxy de senales (default true)
```

<hr>

## doki wait

<sub>[WAIT]</sub>

Bloquea hasta que los contenedores se detengan, imprime exit codes.

```bash
doki wait web
```

<hr>

## doki pull

<sub>[PULL]</sub>

Pull de imagen desde registry.

```text
FLAG             DESCRIPCION
───────────────────────────────────────
--platform list  Plataformas especificas
-a, --all-tags   Todas las imagenes taggeadas en el repo
--quiet          Suprime output de progreso
```

```bash
doki pull alpine
doki pull alpine:3.19
doki pull --platform linux/amd64 alpine
doki pull ghcr.io/owner/repo:tag
```

<hr>

## doki push

<sub>[PUSH]</sub>

Push de imagen a registry.

```text
FLAG     DESCRIPCION
─────────────────────────────
--quiet  Suprime output de progreso
```

```bash
doki push myuser/myapp:1.0
doki push ghcr.io/owner/repo:tag
```

<hr>

## doki images

<sub>[IMAGES]</sub>

Lista imagenes.

```text
FLAG             DESCRIPCION
───────────────────────────────────────
-a, --all        Muestra todas (default: intermedias ocultas)
--digests        Muestra digests
-f, --filter     Filtra
--format str     Go template o json
--no-trunc       No truncar
-q, --quiet      Solo IDs de imagen
```

<hr>

## doki rmi

<sub>[RMI]</sub>

Elimina imagenes.

```text
FLAG        DESCRIPCION
───────────────────────────────────
-f, --force Fuerza eliminacion
--no-prune  No borra parents sin tag
```

<hr>

## doki tag

<sub>[TAG]</sub>

Tagea una imagen.

```bash
doki tag myapp:1.0 myapp:latest
doki tag myapp:1.0 ghcr.io/owner/myapp:1.0
```

<hr>

## doki login

<sub>[LOGIN]</sub>

Log in a registry. Credenciales en <kbd>~/.doki/config.json</kbd> o keyring del sistema.

```text
FLAG                DESCRIPCION
─────────────────────────────────────────────────────
-u, --username str  Usuario
-p, --password str  Password (preferir env var o prompt)
--password-stdin    Lee password desde stdin
```

<hr>

## doki logout

<sub>[LOGOUT]</sub>

Log out, elimina credenciales almacenadas.

<hr>

## doki search

<sub>[SEARCH]</sub>

Busca en Docker Hub.

```text
FLAG             DESCRIPCION
───────────────────────────────────────
--limit int      Max resultados (default 25)
--filter str     Filtra (stars=100)
--format str     Go template o json
```

<hr>

## doki network create

<sub>[NET-CREATE]</sub>

Crea una red.

```text
FLAG            DESCRIPCION
────────────────────────────────────────────────
--driver str    Bridge, host, none, macvlan, ipvlan (default bridge)
--gateway str   Gateway para la subnet
--subnet str    Subnet en CIDR
--ip-range str  Rango de IPs a asignar
--opt list      Opciones del driver
--ipv6          Habilita IPv6
--label list    Labels
--internal      Restringe acceso externo
```

```bash
doki network create backend
doki network create --driver bridge --subnet 10.1.0.0/16 isolated
doki network create --ipv6 dualstack
```

<hr>

## doki network connect

<sub>[NET-CONNECT]</sub>

Attachea contenedor a red.

```text
FLAG          DESCRIPCION
───────────────────────────────────────
--ip str      IPv4 especifica
--ip6 str     IPv6 especifica
--alias list  Aliases de red
--link list   Anade link a otro contenedor
```

<hr>

## doki network disconnect

<sub>[NET-DISCONNECT]</sub>

```text
FLAG       DESCRIPCION
──────────────────────────
-f, --force Fuerza
```

<hr>

## doki volume create

<sub>[VOL-CREATE]</sub>

```text
FLAG           DESCRIPCION
───────────────────────────────────────
--driver str   Driver de volumen (default local)
--opt list     Opciones del driver
--label list   Labels
```

```bash
doki volume create db-data
doki volume create --driver local --opt type=nfs --opt o=addr=10.0.0.1,rw nfs-vol
```

<hr>

## doki system prune

<sub>[PRUNE]</sub>

Elimina contenedores detenidos, imagenes dangling, redes no usadas, build cache.

```text
FLAG          DESCRIPCION
───────────────────────────────────────
-a, --all     Elimina todas las imagenes no usadas (no solo dangling)
--filter      Filtra
-f, --force   No prompt
--volumes     Prunea volumenes
```

<hr>

## doki system events

<sub>[EVENTS]</sub>

Eventos en tiempo real del server.

```text
FLAG           DESCRIPCION
───────────────────────────────────────
--since str    Eventos desde
--until str    Eventos hasta
--filter       Filtra (type=container, event=start)
--format str   JSON o template
```

<hr>

## DokiLink Mesh

<sub>[MESH]</sub>

DokiLink-Lite proporciona networking mesh peer-to-peer con TLS 1.3 y encriptacion NaCl secretbox opcional.

Ejemplos:

```bash
doki mesh status
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"
doki mesh ls
doki link rm mybuddy
```

Flags globales de DokiLink:

```text
FLAG                       DESCRIPCION
───────────────────────────────────────────────────────────────
--doki-link-addr str       Direcccion de escucha gossip (default :7432)
--doki-link-payload-enc    Habilita encriptacion NaCl secretbox de payload (L2)
--doki-link-mesh 0        Deshabilita mesh completamente
--doki-use-socat 1        Fuerza fallback a socat para port forwarding
```

<hr>

## doki deps

<sub>[DEPS]</sub>

Inspeccion y manejo de dependencias. v0.11.0 introduce cuatro subcomandos.

```text
SUBCOMANDO              DESCRIPCION
──────────────────────────────────────────────────────────────────────────
doki deps ls            Lista dependencias de runtime detectadas y versiones
doki deps check [N...]  Verifica presencia y version de dependencias
doki deps go [NAME]     Imprime el path que Doki invocara para una dependencia
doki deps install [N..] Instala dependencias faltantes via package manager
```

Ejemplos:

```bash
doki deps ls
doki deps check proot qemu-arm-static
doki deps go proot
doki deps install proot
```

<hr>

## Formatos de salida

<sub>[FORMAT]</sub>

La mayoria de comandos <kbd>ls</kbd>, <kbd>ps</kbd>, <kbd>images</kbd>, <kbd>volume</kbd> aceptan <kbd>--format</kbd> con un Go template.

```bash
doki ps --format '{{.ID}}: {{.Image}} -> {{.Status}}'
doki images --format '{{.Repository}}:{{.Tag}} ({{.Size}})'
doki inspect --format '{{json .State}}' web
```

<kbd>--format json</kbd> emite JSON para pipear a <kbd>jq</kbd>.

<hr>

## Codigos de salida

<sub>[EXIT]</sub>

```text
CODE  SIGNIFICADO
───────────────────────────────────────
0     Exito
1     Error general
2     Mal uso del comando
125   Error del daemon
126   Comando del contenedor no ejecutable
127   Comando del contenedor no encontrado
130   Contenedor recibio SIGINT (128+2)
137   Contenedor recibio SIGKILL (128+9)
143   Contenedor recibio SIGTERM (128+15)
```

<hr>

## Fuente

<sub>[SOURCE]</sub>

- CLI: <kbd>cmd/doki/main.go</kbd> (cobra)
- Comandos: <kbd>cmd/doki/*.go</kbd> (un archivo por grupo de comandos)
- Flags compartidos: <kbd>pkg/cli/flags.go</kbd>
- API del daemon: <kbd>pkg/api/*.go</kbd>
- Deps: <kbd>pkg/deps/</kbd>