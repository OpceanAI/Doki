# CLI Reference

<sub>[DOC: CLI-REFERENCE]</sub>

Doki v0.11.0. Canonical command catalog. 244 commands across 10 categories. The [Quick Start](Quick-Start) covers common usage.

<hr>

## Global Flags

<sub>[GLOBAL]</sub>

Available on most commands.

```text
FLAG               DESCRIPTION
────────────────────────────────────────────────────────────
--host string      Daemon socket (default: $DOKI_HOST or platform-specific)
--config string    Path to config.json (default: ~/.doki/config.json)
-D, --debug        Enable debug logging
--tls              Use TLS to connect to daemon
--tlscert string   Path to TLS cert
--tlskey string    Path to TLS key
--tlsverify        Verify remote TLS cert
-H, --human        Human-readable output (default: true for ps, images, df)
--format string    Output format: text (default) or json
--quiet            Suppress non-essential output
```

<hr>

## Command Catalog

<sub>[CATALOG]</sub>

Commands grouped by category. Syntax followed by one-line description.

```text
CONTAINER COMMANDS
────────────────────────────────────────────────────────────────────────────
doki run [OPTIONS] IMAGE [CMD] [ARG...]   Create and start a container
doki create [OPTIONS] IMAGE [CMD]         Create container without starting
doki start [OPTIONS] CONTAINER [...]      Start one or more stopped containers
doki stop [OPTIONS] CONTAINER [...]        Gracefully stop containers (SIGTERM then SIGKILL)
doki restart [OPTIONS] CONTAINER [...]    Stop and start containers
doki kill [OPTIONS] CONTAINER              Send a signal to a running container
doki rm [OPTIONS] CONTAINER [...]           Remove containers
doki exec [OPTIONS] CONTAINER CMD [ARG...] Run a command in a running container
doki logs [OPTIONS] CONTAINER              Fetch container logs
doki stats [OPTIONS] [CONTAINER...]        Live resource usage statistics
doki top CONTAINER [PS_OPTIONS]             Display running processes in container
doki inspect [OPTIONS] NAME|ID [...]        Detailed JSON information
doki build [OPTIONS] PATH|URL|-              Build an image from a Dokifile
doki commit [OPTIONS] CONTAINER [IMAGE]      Create image from container changes
doki attach [OPTIONS] CONTAINER              Attach to running container I/O
doki wait CONTAINER [...]                    Block until containers stop, print exit codes
doki ps [OPTIONS]                            List containers
```

```text
IMAGE COMMANDS
────────────────────────────────────────────────────────────────────────────
doki pull [OPTIONS] NAME[:TAG|@DIGEST]   Pull an image from a registry
doki push NAME[:TAG]                    Push an image to a registry
doki images [OPTIONS] [REPOSITORY]      List images
doki rmi [OPTIONS] IMAGE [...]           Remove images
doki tag SOURCE[:TAG] TARGET[:TAG]      Tag an image
doki login [OPTIONS] [SERVER]           Log in to a registry
doki logout [SERVER]                    Log out, remove credentials
doki search [OPTIONS] TERM               Search Docker Hub for images
```

```text
NETWORK COMMANDS
────────────────────────────────────────────────────────────────────────────
doki network ls                          List networks
doki network create [OPTIONS] NETWORK    Create a network
doki network rm NETWORK [...]             Remove networks
doki network inspect NETWORK [...]        Detailed network info
doki network connect [OPTIONS] NET CONT  Attach container to network
doki network disconnect [OPTIONS] NET C  Detach container from network
doki network prune [OPTIONS]            Remove all unused networks
```

```text
VOLUME COMMANDS
────────────────────────────────────────────────────────────────────────────
doki volume ls                           List volumes
doki volume create [OPTIONS] [NAME]      Create a volume
doki volume rm VOLUME [...]              Remove volumes (-f to force)
doki volume inspect VOLUME [...]          Detailed info
```

```text
SYSTEM COMMANDS
────────────────────────────────────────────────────────────────────────────
doki info                                System-wide information (driver, cgroup, kernel)
doki version                             Show client and server version
doki system df                           Disk usage by images, containers, volumes, cache
doki system prune [OPTIONS]             Remove stopped containers, dangling images
doki system events [OPTIONS]            Real-time events from server
doki ping                                Ping the daemon (returns OK)
doki container prune                    Prune stopped containers
doki image prune                        Prune unused images
doki volume prune                       Prune unused volumes
doki network prune                      Prune unused networks
```

```text
POD COMMANDS
────────────────────────────────────────────────────────────────────────────
doki pod create                          Create a pod (group of containers)
doki pod ls                              List pods
doki pod ps                              List pods with status
doki pod rm                              Remove pod
doki pod start                           Start pod
doki pod stop                            Stop pod
doki pod inspect                         Inspect pod
doki generate kube                       Generate Kubernetes YAML from container
doki play kube                           Run Kubernetes YAML
doki auto-update                         Auto-update containers from registry
doki unshare                             Run command in container namespaces
doki untag                               Remove tag from image
doki mount                               Mount container filesystem
doki unmount                             Unmount container filesystem
doki healthcheck                         Run health check manually
```

```text
COMPOSE COMMANDS
────────────────────────────────────────────────────────────────────────────
doki-compose up                          Create and start services
doki-compose down                        Stop and remove services, networks
doki-compose ps                          List containers
doki-compose logs                        View logs (multi-service)
doki-compose exec                        Run command in service
doki-compose run                         Run one-off command
doki-compose start                       Lifecycle: start
doki-compose stop                        Lifecycle: stop
doki-compose restart                     Lifecycle: restart
doki-compose pull                        Pull all images
doki-compose build                       Build all images
doki-compose config                      Validate and view config
doki-compose ls                          List compose projects
doki-compose scale                       Scale services
doki-compose top                         Display running processes
doki-compose pause                       Pause services
doki-compose unpause                     Unpause services
doki-compose kill                        Send signal
doki-compose rm                          Remove stopped containers
doki-compose port                        Print port mapping
doki-compose images                      List images used
doki-compose version                     Version info
```

```text
KUBERNETES COMMANDS
────────────────────────────────────────────────────────────────────────────
doki kube play                           Run Kubernetes pod YAML
doki kube down                           Stop and remove pod
doki kube generate                       Generate K8s YAML from compose
doki apply -f FILE                       Apply Kubernetes YAML (alias of kube play)
```

```text
MESH/LINK COMMANDS
────────────────────────────────────────────────────────────────────────────
doki mesh ls                             List connected mesh peers
doki mesh status                         Show mesh status and public key
doki link add NAME ADDRESS               Add a trusted peer (TOFU)
doki link rm NAME                        Remove a peer
doki link show NAME                      Show peer details
```

```text
DEPS COMMANDS
────────────────────────────────────────────────────────────────────────────
doki deps ls                             List detected runtime dependencies
doki deps check [NAME...]                 Check presence and version of dependencies
doki deps go [NAME]                      Print the path Doki will invoke for a dependency
doki deps install [NAME...]               Install missing dependencies via platform package manager
```

<hr>

## doki run

<sub>[RUN]</sub>

Create and start a container. Accepts approximately 80 flags. Common subset below.

```text
FLAG                          DESCRIPTION
──────────────────────────────────────────────────────────────
--name string                 Container name
-d, --detach                  Run in background
-it                           Interactive TTY (combines -i and -t)
--rm                          Remove container when it exits
-e, --env list                Environment variables (KEY=VALUE)
--env-file list               Read env vars from files
-p, --publish list            Port mapping (host:container)
-P, --publish-all             Publish all EXPOSEd ports
-v, --volume list             Bind mount (src:dst[:opts])
--mount mount                 Mount spec (Compose-style)
-w, --workdir string          Working directory inside container
-u, --user string             User (uid[:gid])
--entrypoint string           Override entrypoint
--network network             Network to attach
--hostname string             Container hostname
--domainname string           Container domain name
--dns list                    Custom DNS servers
--dns-search list             DNS search domains
--dns-opt list                DNS options
--add-host list               Extra /etc/hosts entries
--restart string              Restart policy: no, always, unless-stopped, on-failure[:max]
--runtime string              Isolation mode (12 options, see Isolation-Levels)
--init                        Run doki-init as PID 1 (signals, zombies)
--privileged                  Grant extended privileges
--read-only                   Mount rootfs as read-only
--cap-add list                Add Linux capabilities
--cap-drop list               Drop Linux capabilities
--security-opt list           Security options (seccomp, apparmor)
--sysctl map                  Sysctls (net.core.somaxconn=512)
--ulimit ulimit               Ulimit (nofile=1024:2048)
--memory string               Memory limit (256m, 1g)
--cpus string                 CPU limit (0.5, 2)
--cpuset-cpus string          CPUs to allow (0-3)
--shm-size string             /dev/shm size (64m)
--pids-limit int              Max PIDs (negative = unlimited)
--blkio-weight uint16         Block I/O weight (10-1000)
--device list                 Host devices to expose
--tmpfs list                  tmpfs mounts
--log-driver string           Log driver: json-file, journald, local, none
--log-opt list                Log driver options
--label list                  Container labels
--label-file list             Read labels from files
--stop-signal string          Signal to stop container (default SIGTERM)
--stop-timeout int            Stop timeout in seconds
--health-cmd string           Health check command
--health-interval duration    Health check interval (default 0s = disabled)
--health-timeout duration     Health check timeout
--health-retries int          Health check retries
--health-start-period d       Initial grace period
--platform string             Force platform (e.g. linux/amd64)
--pull string                 Pull policy: always, missing, never
--cidfile string               Write container ID to file
--detach-keys string          Override detach keys
```

Examples:

```bash
doki run --rm alpine echo hello
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
doki run --runtime proot --rm alpine echo "using proot"
doki run --platform linux/amd64 --rm amd64-only-image:latest
```

<hr>

## doki ps

<sub>[PS]</sub>

List containers.

```text
FLAG              DESCRIPTION
─────────────────────────────────────────
-a, --all         Show all (default: running)
--filter, -f      Filter (status=running, name=web, label=env=prod)
--format string   Go template or json
--no-trunc        Don't truncate IDs
-n, --last int    Show n last created (all states)
-l, --latest      Show latest created
-q, --quiet       Only container IDs
-s, --size        Show sizes
```

Examples:

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

Start stopped containers.

```text
FLAG              DESCRIPTION
─────────────────────────────────────────
-a, --attach      Attach STDOUT/STDERR
-i, --interactive Attach STDIN
```

<hr>

## doki stop

<sub>[STOP]</sub>

Graceful stop. Sends SIGTERM, waits <kbd>--stop-timeout</kbd> (default 10s), then SIGKILL.

```text
FLAG              DESCRIPTION
─────────────────────────────────────────
-t, --time int    Seconds to wait before kill
--signal string   Custom stop signal
```

<hr>

## doki kill

<sub>[KILL]</sub>

Send signal to running container.

```text
FLAG              DESCRIPTION
─────────────────────────────────────────
-s, --signal string  Signal to send (default SIGKILL)
```

<hr>

## doki rm

<sub>[RM]</sub>

Remove containers.

```text
FLAG         DESCRIPTION
──────────────────────────────────
-f, --force  Force remove running container (SIGKILL first)
-v, --volumes Remove anonymous volumes
-l, --link   Remove the specified link
```

<hr>

## doki exec

<sub>[EXEC]</sub>

Run command in running container.

```text
FLAG                DESCRIPTION
──────────────────────────────────────────
-d, --detach        Detached
-i, --interactive   Keep STDIN open
-t, --tty           Allocate pseudo-TTY
-e, --env list      Environment variables
-w, --workdir str   Working directory
-u, --user string   User
--privileged        Extended privileges
--detach-keys str   Override detach keys
```

Examples:

```bash
doki exec web ls /var/log
doki exec -it web sh
doki exec -u postgres db psql -U postgres
doki exec -e DEBUG=1 api /bin/debug
```

<hr>

## doki logs

<sub>[LOGS]</sub>

Fetch container logs.

```text
FLAG              DESCRIPTION
─────────────────────────────────────────
-f, --follow      Follow log output (tail -f)
--since string    Logs since timestamp or relative (10m)
--until string    Logs before timestamp
-t, --timestamps  Show timestamps
--tail string     Lines from end (all for full)
--details         Show extra details
```

Examples:

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

Live resource usage.

```text
FLAG            DESCRIPTION
───────────────────────────────────────
-a, --all       All containers (default: running)
--no-stream     Single snapshot, no live updates
--format str    Go template or json
```

Sample output:

```text
CONTAINER    CPU %   MEM USAGE / LIMIT   MEM %   NET I/O       BLOCK I/O
web          0.05%   12.3 MiB / 1 GiB     1.20%   1.2kB / 0B    0B / 0B
db           1.23%   156 MiB / 512 MiB    30.4%   5.6MB / 4MB   1MB / 8MB
```

<hr>

## doki top

<sub>[TOP]</sub>

Display running processes in container. Passes through to internal <kbd>ps</kbd>.

```bash
doki top web
doki top web aux
```

<hr>

## doki inspect

<sub>[INSPECT]</sub>

Detailed JSON for container, image, network, volume.

```text
FLAG              DESCRIPTION
─────────────────────────────────────────
-f, --format str  Go template
-s, --size        Show size (images)
--type string     Limit to type: container, image, network, volume
```

Examples:

```bash
doki inspect web
doki inspect --format '{{.State.Status}}' web
doki inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' web
```

<hr>

## doki create

<sub>[CREATE]</sub>

Create container without starting. Same flags as <kbd>run</kbd> except <kbd>-d</kbd>, <kbd>-it</kbd>, restart policies. Use <kbd>doki start</kbd> to launch.

<hr>

## doki restart

<sub>[RESTART]</sub>

Stop then start. Same flags as <kbd>stop</kbd>.

<hr>

## doki build

<sub>[BUILD]</sub>

Build image from Dokifile.

```text
FLAG                DESCRIPTION
────────────────────────────────────────────────
-f, --file string   Path to Dokifile (default Dokifile in context)
-t, --tag list      Name and tag (name:tag)
--build-arg list    Build-time variables
--target string     Target stage for multi-stage builds
--no-cache          Disable build cache
--pull              Always pull base image
--platform list     Target platforms
--secret id=id      BuildKit secret
--ssh string        SSH agent socket or key
```

Examples:

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

Create image from container changes.

```text
FLAG                DESCRIPTION
─────────────────────────────────────────────
-a, --author str    Author
-m, --message str   Commit message
-c, --change list   Apply Dockerfile instruction
-p, --pause         Pause container during commit
```

<hr>

## doki attach

<sub>[ATTACH]</sub>

Attach to running container I/O. Detach with <kbd>ctrl-p,ctrl-q</kbd> or custom <kbd>--detach-keys</kbd>.

```text
FLAG               DESCRIPTION
────────────────────────────────────────────
--detach-keys str  Override detach keys
--no-stdin         Don't attach STDIN
--sig-proxy        Proxy signals (default true)
```

<hr>

## doki wait

<sub>[WAIT]</sub>

Block until containers stop, print exit codes.

```bash
doki wait web
```

<hr>

## doki pull

<sub>[PULL]</sub>

Pull image from registry.

```text
FLAG             DESCRIPTION
───────────────────────────────────────
--platform list  Specific platforms
-a, --all-tags   All tagged images in repo
--quiet          Suppress progress output
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

Push image to registry.

```text
FLAG     DESCRIPTION
─────────────────────────────
--quiet  Suppress progress output
```

```bash
doki push myuser/myapp:1.0
doki push ghcr.io/owner/repo:tag
```

<hr>

## doki images

<sub>[IMAGES]</sub>

List images.

```text
FLAG             DESCRIPTION
───────────────────────────────────────
-a, --all        Show all (default: intermediate hidden)
--digests        Show digests
-f, --filter     Filter
--format str     Go template or json
--no-trunc       Don't truncate
-q, --quiet      Only image IDs
```

<hr>

## doki rmi

<sub>[RMI]</sub>

Remove images.

```text
FLAG        DESCRIPTION
───────────────────────────────────
-f, --force Force remove
--no-prune  Don't delete untagged parents
```

<hr>

## doki tag

<sub>[TAG]</sub>

Tag an image.

```bash
doki tag myapp:1.0 myapp:latest
doki tag myapp:1.0 ghcr.io/owner/myapp:1.0
```

<hr>

## doki login

<sub>[LOGIN]</sub>

Log in to registry. Credentials stored in <kbd>~/.doki/config.json</kbd> or system keyring.

```text
FLAG                DESCRIPTION
─────────────────────────────────────────────────────
-u, --username str  Username
-p, --password str  Password (prefer env var or prompt)
--password-stdin    Read password from stdin
```

<hr>

## doki logout

<sub>[LOGOUT]</sub>

Log out, remove stored credentials.

<hr>

## doki search

<sub>[SEARCH]</sub>

Search Docker Hub.

```text
FLAG             DESCRIPTION
───────────────────────────────────────
--limit int      Max results (default 25)
--filter str     Filter (stars=100)
--format str     Go template or json
```

<hr>

## doki network create

<sub>[NET-CREATE]</sub>

Create a network.

```text
FLAG            DESCRIPTION
────────────────────────────────────────────────
--driver str    Bridge, host, none, macvlan, ipvlan (default bridge)
--gateway str   Gateway for subnet
--subnet str    Subnet in CIDR
--ip-range str  Range of IPs to allocate
--opt list      Driver options
--ipv6          Enable IPv6
--label list    Labels
--internal      Restrict external access
```

```bash
doki network create backend
doki network create --driver bridge --subnet 10.1.0.0/16 isolated
doki network create --ipv6 dualstack
```

<hr>

## doki network connect

<sub>[NET-CONNECT]</sub>

Attach container to network.

```text
FLAG          DESCRIPTION
───────────────────────────────────────
--ip str      Specific IPv4
--ip6 str     Specific IPv6
--alias list  Network aliases
--link list   Add link to another container
```

<hr>

## doki network disconnect

<sub>[NET-DISCONNECT]</sub>

```text
FLAG       DESCRIPTION
──────────────────────────
-f, --force Force
```

<hr>

## doki volume create

<sub>[VOL-CREATE]</sub>

```text
FLAG           DESCRIPTION
───────────────────────────────────────
--driver str   Volume driver (default local)
--opt list     Driver options
--label list   Labels
```

```bash
doki volume create db-data
doki volume create --driver local --opt type=nfs --opt o=addr=10.0.0.1,rw nfs-vol
```

<hr>

## doki system prune

<sub>[PRUNE]</sub>

Remove stopped containers, dangling images, unused networks, build cache.

```text
FLAG          DESCRIPTION
───────────────────────────────────────
-a, --all     Remove all unused images (not just dangling)
--filter      Filter
-f, --force   Don't prompt
--volumes     Prune volumes
```

<hr>

## doki system events

<sub>[EVENTS]</sub>

Real-time events from server.

```text
FLAG           DESCRIPTION
───────────────────────────────────────
--since str    Events since
--until str    Events until
--filter       Filter (type=container, event=start)
--format str   JSON or template
```

<hr>

## DokiLink Mesh

<sub>[MESH]</sub>

DokiLink-Lite provides peer-to-peer mesh networking with TLS 1.3 and optional NaCl secretbox encryption.

Examples:

```bash
doki mesh status
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"
doki mesh ls
doki link rm mybuddy
```

Global DokiLink flags:

```text
FLAG                       DESCRIPTION
───────────────────────────────────────────────────────────────
--doki-link-addr str       Gossip listen address (default :7432)
--doki-link-payload-enc    Enable NaCl secretbox payload encryption (L2)
--doki-link-mesh 0        Disable mesh entirely
--doki-use-socat 1        Force socat fallback for port forwarding
```

<hr>

## doki deps

<sub>[DEPS]</sub>

Dependency inspection and management. v0.11.0 introduces four subcommands.

```text
SUBCOMMAND              DESCRIPTION
──────────────────────────────────────────────────────────────────────────
doki deps ls            List detected runtime dependencies and versions
doki deps check [N...]  Check presence and version of dependencies
doki deps go [NAME]     Print the path Doki will invoke for a dependency
doki deps install [N..] Install missing dependencies via platform package manager
```

Examples:

```bash
doki deps ls
doki deps check proot qemu-arm-static
doki deps go proot
doki deps install proot
```

<hr>

## Output Formats

<sub>[FORMAT]</sub>

Most <kbd>ls</kbd>, <kbd>ps</kbd>, <kbd>images</kbd>, <kbd>volume</kbd> commands accept <kbd>--format</kbd> with a Go template.

```bash
doki ps --format '{{.ID}}: {{.Image}} -> {{.Status}}'
doki images --format '{{.Repository}}:{{.Tag}} ({{.Size}})'
doki inspect --format '{{json .State}}' web
```

<kbd>--format json</kbd> emits JSON for piping to <kbd>jq</kbd>.

<hr>

## Exit Codes

<sub>[EXIT]</sub>

```text
CODE  MEANING
───────────────────────────────────────
0     Success
1     General error
2     Misuse of command
125   daemon error
126   Container command not executable
127   Container command not found
130   Container received SIGINT (128+2)
137   Container received SIGKILL (128+9)
143   Container received SIGTERM (128+15)
```

<hr>

## Source

<sub>[SOURCE]</sub>

- CLI: <kbd>cmd/doki/main.go</kbd> (cobra)
- Commands: <kbd>cmd/doki/*.go</kbd> (one file per command group)
- Shared flags: <kbd>pkg/cli/flags.go</kbd>
- Daemon API: <kbd>pkg/api/*.go</kbd>
- Deps: <kbd>pkg/deps/</kbd>