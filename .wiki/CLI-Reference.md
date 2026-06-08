# CLI Reference

Doki v0.9.3 ships **244 commands** across 9 categories. This page is the canonical reference; the [Quick Start](Quick-Start) walks through the common ones.

## Global Flags

These flags are available on most commands:

| Flag | Description |
|:-----|:------------|
| `--host string` | Daemon socket (default: `$DOKI_HOST` or platform-specific) |
| `--config string` | Path to `config.json` (default: `~/.doki/config.json`) |
| `-D, --debug` | Enable debug logging |
| `--tls` | Use TLS to connect to daemon |
| `--tlscert string` | Path to TLS cert |
| `--tlskey string` | Path to TLS key |
| `--tlsverify` | Verify remote TLS cert |
| `-H, --human` | Human-readable output (default: true for `ps`, `images`, `df`) |
| `--format string` | Output format: `text` (default) or `json` |
| `--quiet` | Suppress non-essential output |

## Container Management (17 commands)

### `doki run [OPTIONS] IMAGE [COMMAND] [ARG...]`

Create and start a container. ~80 flags, the most common:

| Flag | Description |
|:-----|:------------|
| `--name string` | Container name |
| `-d, --detach` | Run in background |
| `-it` | Interactive TTY (combines `-i` and `-t`) |
| `--rm` | Remove container when it exits |
| `-e, --env list` | Environment variables (`KEY=VALUE`) |
| `--env-file list` | Read env vars from files |
| `-p, --publish list` | Port mapping (`host:container`) |
| `-P, --publish-all` | Publish all `EXPOSE`d ports |
| `-v, --volume list` | Bind mount (`src:dst[:opts]`) |
| `--mount mount` | Mount spec (Compose-style) |
| `-w, --workdir string` | Working directory inside container |
| `-u, --user string` | User (`uid[:gid]`) |
| `--entrypoint string` | Override entrypoint |
| `--network network` | Network to attach |
| `--hostname string` | Container hostname |
| `--domainname string` | Container domain name |
| `--dns list` | Custom DNS servers |
| `--dns-search list` | DNS search domains |
| `--dns-opt list` | DNS options |
| `--add-host list` | Extra `/etc/hosts` entries |
| `--restart string` | Restart policy: `no`, `always`, `unless-stopped`, `on-failure[:max]` |
| `--runtime string` | Isolation mode (12 options, see [Isolation Levels](Isolation-Levels)) |
| `--init` | Run `doki-init` as PID 1 (handles signals, zombies) |
| `--privileged` | Grant extended privileges |
| `--read-only` | Mount rootfs as read-only |
| `--cap-add list` | Add Linux capabilities |
| `--cap-drop list` | Drop Linux capabilities |
| `--security-opt list` | Security options (seccomp, apparmor) |
| `--sysctl map` | Sysctls (`net.core.somaxconn=512`) |
| `--ulimit ulimit` | Ulimit (`nofile=1024:2048`) |
| `--memory string` | Memory limit (`256m`, `1g`) |
| `--cpus string` | CPU limit (`0.5`, `2`) |
| `--cpuset-cpus string` | CPUs to allow (`0-3`) |
| `--shm-size string` | `/dev/shm` size (`64m`) |
| `--pids-limit int` | Max PIDs (negative = unlimited) |
| `--blkio-weight uint16` | Block I/O weight (10-1000) |
| `--device list` | Host devices to expose |
| `--tmpfs list` | tmpfs mounts |
| `--log-driver string` | Log driver: `json-file`, `journald`, `local`, `none` |
| `--log-opt list` | Log driver options |
| `--label list` | Container labels |
| `--label-file list` | Read labels from files |
| `--stop-signal string` | Signal to stop container (default `SIGTERM`) |
| `--stop-timeout int` | Stop timeout in seconds |
| `--health-cmd string` | Health check command |
| `--health-interval duration` | Health check interval (default 0s = disabled) |
| `--health-timeout duration` | Health check timeout |
| `--health-retries int` | Health check retries |
| `--health-start-period duration` | Initial grace period |
| `--platform string` | Force platform (e.g. `linux/amd64`) |
| `--pull string` | Pull policy: `always`, `missing`, `never` |
| `--cidfile string` | Write container ID to file |
| `--detach-keys string` | Override detach keys |

**Examples**:

```bash
# Run a quick command
doki run --rm alpine echo hello

# Interactive shell
doki run -it --rm alpine sh

# Long-lived web server with port mapping
doki run -d --name web -p 8080:80 nginx:alpine

# With resource limits
doki run -d --name api -m 512m --cpus 1.0 -p 3000:3000 my-api:latest

# With restart policy
doki run -d --name worker --restart unless-stopped my-worker:latest

# With health check
doki run -d --name web \
  --health-cmd "wget -q --spider http://localhost/" \
  --health-interval 30s \
  --health-timeout 3s \
  --health-retries 3 \
  nginx:alpine

# With env file
doki run -d --env-file .env --name api my-api:latest

# Force a specific isolation level
doki run --runtime proot --rm alpine echo "using proot"

# Cross-architecture (uses QEMU User)
doki run --platform linux/amd64 --rm amd64-only-image:latest
```

### `doki ps [OPTIONS]`

List containers.

| Flag | Description |
|:-----|:------------|
| `-a, --all` | Show all (default: running) |
| `--filter, -f` | Filter (`status=running`, `name=web`, `label=env=prod`) |
| `--format string` | Go template or `json` |
| `--no-trunc` | Don't truncate IDs |
| `-n, --last int` | Show n last created (all states) |
| `-l, --latest` | Show latest created |
| `-q, --quiet` | Only container IDs |
| `-s, --size` | Show sizes |

**Examples**:

```bash
doki ps                      # running
doki ps -a                  # all
doki ps -f "status=exited"  # only exited
doki ps -f "label=env=prod" # by label
doki ps --format json       # JSON
doki ps --format "{{.Names}}: {{.Status}}"
```

### `doki create [OPTIONS] IMAGE [COMMAND] [ARG...]`

Create a container without starting it. Same flags as `doki run` except `-d`/`-it`/restart policies. Use `doki start <id>` to start it later.

### `doki start [OPTIONS] CONTAINER [CONTAINER...]`

Start one or more stopped containers.

| Flag | Description |
|:-----|:------------|
| `-a, --attach` | Attach STDOUT/STDERR |
| `-i, --interactive` | Attach STDIN |

### `doki stop [OPTIONS] CONTAINER [CONTAINER...]`

Gracefully stop running containers. Sends `SIGTERM`, waits `--stop-timeout` (default 10s), then `SIGKILL`.

| Flag | Description |
|:-----|:------------|
| `-t, --time int` | Seconds to wait before kill |
| `--signal string` | Custom stop signal |

### `doki restart [OPTIONS] CONTAINER [CONTAINER...]`

Stop and start. Same flags as `stop`.

### `doki kill [OPTIONS] CONTAINER`

Send a signal to a running container.

| Flag | Description |
|:-----|:------------|
| `-s, --signal string` | Signal to send (default `SIGKILL`) |

### `doki rm [OPTIONS] CONTAINER [CONTAINER...]`

Remove containers.

| Flag | Description |
|:-----|:------------|
| `-f, --force` | Force remove running container (SIGKILL first) |
| `-v, --volumes` | Remove anonymous volumes |
| `-l, --link` | Remove the specified link |

### `doki exec [OPTIONS] CONTAINER COMMAND [ARG...]`

Run a command in a running container.

| Flag | Description |
|:-----|:------------|
| `-d, --detach` | Detached |
| `-i, --interactive` | Keep STDIN open |
| `-t, --tty` | Allocate a pseudo-TTY |
| `-e, --env list` | Environment variables |
| `-w, --workdir string` | Working directory |
| `-u, --user string` | User |
| `--privileged` | Extended privileges |
| `--detach-keys string` | Override detach keys |

**Examples**:

```bash
doki exec web ls /var/log
doki exec -it web sh
doki exec -u postgres db psql -U postgres
doki exec -e DEBUG=1 api /bin/debug
```

### `doki logs [OPTIONS] CONTAINER`

Fetch container logs.

| Flag | Description |
|:-----|:------------|
| `-f, --follow` | Follow log output (like `tail -f`) |
| `--since string` | Show logs since timestamp (`2024-01-01T00:00:00`) or relative (`10m`) |
| `--until string` | Show logs before timestamp |
| `-t, --timestamps` | Show timestamps |
| `--tail string` | Number of lines from end (`all` for full) |
| `--details` | Show extra details |

**Examples**:

```bash
doki logs web
doki logs -f web
doki logs --since 10m web
doki logs --tail 100 -f web
doki logs --since 2024-01-01T00:00:00 --until 2024-01-02T00:00:00 web
```

### `doki stats [OPTIONS] [CONTAINER...]`

Live resource usage statistics.

| Flag | Description |
|:-----|:------------|
| `-a, --all` | All containers (default: running) |
| `--no-stream` | Single snapshot, no live updates |
| `--format string` | Go template or `json` |

**Example output**:

```
CONTAINER    CPU %   MEM USAGE / LIMIT   MEM %   NET I/O       BLOCK I/O
web          0.05%   12.3 MiB / 1 GiB     1.20%   1.2kB / 0B    0B / 0B
db           1.23%   156 MiB / 512 MiB    30.4%   5.6MB / 4MB   1MB / 8MB
```

### `doki top CONTAINER [PS_OPTIONS]`

Display running processes in a container. Passes through to `ps` inside the container.

```bash
doki top web
doki top web aux
```

### `doki inspect [OPTIONS] NAME|ID [NAME|ID...]`

Detailed JSON information about a container, image, network, volume, etc.

| Flag | Description |
|:-----|:------------|
| `-f, --format string` | Go template |
| `-s, --size` | Show size (for images) |
| `--type string` | Limit to type: `container`, `image`, `network`, `volume` |

**Examples**:

```bash
doki inspect web
doki inspect --format '{{.State.Status}}' web
doki inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' web
```

### `doki build [OPTIONS] PATH | URL | -`

Build an image from a Dokifile.

| Flag | Description |
|:-----|:------------|
| `-f, --file string` | Path to Dokifile (default `Dokifile` in context) |
| `-t, --tag list` | Name and tag (`name:tag`) |
| `--build-arg list` | Build-time variables |
| `--target string` | Target stage for multi-stage builds |
| `--no-cache` | Disable build cache |
| `--pull` | Always pull base image |
| `--platform list` | Target platforms |
| `--secret id=id,src=path` | BuildKit secret |
| `--ssh string` | SSH agent socket or key |

**Example**:

```bash
doki build -t myapp:1.0 .
doki build -t myapp:1.0 -f Dockerfile.prod .
doki build --build-arg VERSION=1.2.3 -t myapp:1.2.3 .
doki build --target runner -t myapp:runner .
doki build --platform linux/amd64,linux/arm64 -t myapp:multi .
```

### `doki commit [OPTIONS] CONTAINER [IMAGE[:TAG]]`

Create an image from a container's changes.

| Flag | Description |
|:-----|:------------|
| `-a, --author string` | Author |
| `-m, --message string` | Commit message |
| `-c, --change list` | Apply Dockerfile instruction |
| `-p, --pause` | Pause container during commit |

### `doki attach [OPTIONS] CONTAINER`

Attach to a running container's I/O. Use `--detach-keys` (default `ctrl-p,ctrl-q`) to detach.

| Flag | Description |
|:-----|:------------|
| `--detach-keys string` | Override detach keys |
| `--no-stdin` | Don't attach STDIN |
| `--sig-proxy` | Proxy signals (default true) |

### `doki wait CONTAINER [CONTAINER...]`

Block until one or more containers stop, then print their exit codes.

```bash
$ doki run -d --name web nginx:alpine
abc123
$ doki wait web
0
```

## Image Management (8 commands)

### `doki pull [OPTIONS] NAME[:TAG|@DIGEST]`

Pull an image from a registry.

| Flag | Description |
|:-----|:------------|
| `--platform list` | Specific platforms |
| `-a, --all-tags` | All tagged images in repo |
| `--quiet` | Suppress progress output |

```bash
doki pull alpine
doki pull alpine:3.19
doki pull --platform linux/amd64 alpine
doki pull ghcr.io/owner/repo:tag
```

### `doki push NAME[:TAG]`

Push an image to a registry.

| Flag | Description |
|:-----|:------------|
| `--quiet` | Suppress progress output |

```bash
doki push myuser/myapp:1.0
doki push ghcr.io/owner/repo:tag
```

### `doki images [OPTIONS] [REPOSITORY[:TAG]]`

List images.

| Flag | Description |
|:-----|:------------|
| `-a, --all` | Show all (default: intermediate images hidden) |
| `--digests` | Show digests |
| `-f, --filter` | Filter |
| `--format string` | Go template or `json` |
| `--no-trunc` | Don't truncate |
| `-q, --quiet` | Only image IDs |

### `doki rmi [OPTIONS] IMAGE [IMAGE...]`

Remove images.

| Flag | Description |
|:-----|:------------|
| `-f, --force` | Force remove |
| `--no-prune` | Don't delete untagged parents |

### `doki tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]`

Tag an image.

```bash
doki tag myapp:1.0 myapp:latest
doki tag myapp:1.0 ghcr.io/owner/myapp:1.0
```

### `doki login [OPTIONS] [SERVER]`

Log in to a registry. Stores credentials in `~/.doki/config.json` or system keyring.

| Flag | Description |
|:-----|:------------|
| `-u, --username string` | Username |
| `-p, --password string` | Password (prefer env var or prompt) |
| `--password-stdin` | Read password from stdin |

### `doki logout [SERVER]`

Log out. Removes stored credentials.

### `doki search [OPTIONS] TERM`

Search Docker Hub for images.

| Flag | Description |
|:-----|:------------|
| `--limit int` | Max results (default 25) |
| `--filter stars=100` | Filter by stars |
| `--format string` | Go template or `json` |

## Network Management (7 commands)

### `doki network ls`

List networks. Flags: `--filter`, `--format`, `--quiet`, `--no-trunc`.

### `doki network create [OPTIONS] NETWORK`

Create a network.

| Flag | Description |
|:-----|:------------|
| `--driver string` | Bridge, host, none, macvlan, ipvlan (default `bridge`) |
| `--gateway string` | Gateway for subnet |
| `--subnet string` | Subnet in CIDR |
| `--ip-range string` | Range of IPs to allocate |
| `--opt list` | Driver options |
| `--ipv6` | Enable IPv6 |
| `--label list` | Labels |
| `--internal` | Restrict external access |

```bash
doki network create backend
doki network create --driver bridge --subnet 10.1.0.0/16 isolated
doki network create --ipv6 dualstack
```

### `doki network rm NETWORK [NETWORK...]`

Remove networks.

### `doki network inspect NETWORK [NETWORK...]`

Detailed network info. Flags: `--format`, `--verbose`.

### `doki network connect [OPTIONS] NETWORK CONTAINER`

Attach a container to a network.

| Flag | Description |
|:-----|:------------|
| `--ip string` | Specific IPv4 |
| `--ip6 string` | Specific IPv6 |
| `--alias list` | Network aliases |
| `--link list` | Add link to another container |

### `doki network disconnect [OPTIONS] NETWORK CONTAINER`

Detach a container from a network.

| Flag | Description |
|:-----|:------------|
| `-f, --force` | Force |

### `doki network prune [OPTIONS]`

Remove all unused networks.

| Flag | Description |
|:-----|:------------|
| `--filter` | Filter |
| `-f, --force` | Don't prompt |

## Volume Management (4 commands)

### `doki volume ls`

List volumes. Flags: `--filter`, `--format`, `--quiet`.

### `doki volume create [OPTIONS] [NAME]`

Create a volume.

| Flag | Description |
|:-----|:------------|
| `--driver string` | Volume driver (default `local`) |
| `--opt list` | Driver options |
| `--label list` | Labels |

```bash
doki volume create db-data
doki volume create --driver local --opt type=nfs --opt o=addr=10.0.0.1,rw nfs-vol
```

### `doki volume rm VOLUME [VOLUME...]`

Remove volumes. `-f` to force.

### `doki volume inspect VOLUME [VOLUME...]`

Detailed info.

## System (8 commands)

### `doki info`

System-wide information (driver, cgroup, kernel, etc.).

### `doki version`

Show client and (if `dokid` is running) server version. Includes GitCommit, BuildDate.

### `doki system df`

Disk usage by images, containers, volumes, build cache.

### `doki system prune [OPTIONS]`

Remove all stopped containers, dangling images, unused networks, build cache.

| Flag | Description |
|:-----|:------------|
| `-a, --all` | Remove all unused images (not just dangling) |
| `--filter` | Filter |
| `-f, --force` | Don't prompt |
| `--volumes` | Prune volumes |

### `doki system events [OPTIONS]`

Real-time events from the server.

| Flag | Description |
|:-----|:------------|
| `--since string` | Events since |
| `--until string` | Events until |
| `--filter` | Filter (`type=container`, `event=start`) |
| `--format string` | JSON or template |

### `doki ping`

Ping the daemon. Returns `OK` on success.

### `doki info --format '{{.DriverStatus}}'`

Show storage driver status.

### `doki container prune` / `doki image prune` / `doki volume prune` / `doki network prune`

Type-specific prunes.

## Podman Compatibility (15 commands)

For users migrating from Podman:

| Command | Description |
|:--------|:------------|
| `doki pod create` | Create a pod (group of containers) |
| `doki pod ls` | List pods |
| `doki pod rm` | Remove pod |
| `doki pod start` | Start pod |
| `doki pod stop` | Stop pod |
| `doki pod inspect` | Inspect pod |
| `doki pod ps` | List pods with status |
| `doki generate kube` | Generate Kubernetes YAML from running container |
| `doki play kube` | Run Kubernetes YAML |
| `doki auto-update` | Auto-update containers from registry |
| `doki unshare` | Run command in container's namespaces |
| `doki untag` | Remove tag from image |
| `doki mount` / `unmount` | Mount/unmount container filesystem |
| `doki healthcheck` | Run health check manually |

## Kubernetes Compatibility (4 commands)

| Command | Description |
|:--------|:------------|
| `doki kube play` | Run Kubernetes pod YAML |
| `doki kube down` | Stop and remove pod |
| `doki kube generate` | Generate K8s YAML from compose |
| `doki apply -f` | Apply Kubernetes YAML (alias of `kube play`) |

## DokiLink Mesh (6 commands)

DokiLink-Lite provides peer-to-peer mesh networking with TLS 1.3 and optional NaCl secretbox encryption.

| Command | Description |
|:--------|:------------|
| `doki mesh ls` | List connected mesh peers |
| `doki mesh status` | Show mesh status and public key |
| `doki link add NAME ADDRESS` | Add a trusted peer (TOFU) |
| `doki link rm NAME` | Remove a peer |
| `doki link show NAME` | Show peer details |

**Examples**:

```bash
# Show your public key
doki mesh status

# Add a peer
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# List peers
doki mesh ls

# Remove a peer
doki link rm mybuddy
```

### Global DokiLink flags

| Flag | Description |
|:-----|:------------|
| `--doki-link-addr string` | Gossip listen address (default `:7432`) |
| `--doki-link-payload-enc` | Enable NaCl secretbox payload encryption (L2) |
| `--doki-link-mesh 0` | Disable mesh entirely |
| `--doki-use-socat 1` | Force socat fallback for port forwarding |

## Compose (`doki-compose`, ~30 subcommands)

`doki-compose` is a separate binary with its own command set. Top-level:

| Command | Description |
|:--------|:------------|
| `doki-compose up` | Create and start services |
| `doki-compose down` | Stop and remove services, networks |
| `doki-compose ps` | List containers |
| `doki-compose logs` | View logs (multi-service) |
| `doki-compose exec` | Run command in service |
| `doki-compose run` | Run one-off command |
| `doki-compose start` / `stop` / `restart` | Lifecycle |
| `doki-compose pull` | Pull all images |
| `doki-compose build` | Build all images |
| `doki-compose config` | Validate and view config |
| `doki-compose ls` | List compose projects |
| `doki-compose scale` | Scale services |
| `doki-compose top` | Display running processes |
| `doki-compose pause` / `unpause` | Pause/unpause |
| `doki-compose kill` | Send signal |
| `doki-compose rm` | Remove stopped containers |
| `doki-compose port` | Print port mapping |
| `doki-compose images` | List images used |
| `doki-compose version` | Version info |

## Output Formats

Most `ls`/`ps`/`images`/`volume` commands support `--format` with a Go template:

```bash
doki ps --format '{{.ID}}: {{.Image}} -> {{.Status}}'
doki images --format '{{.Repository}}:{{.Tag}} ({{.Size}})'
doki inspect --format '{{json .State}}' web
```

`--format json` outputs JSON for piping to `jq`.

## Exit Codes

| Code | Meaning |
|:----:|:--------|
| 0 | Success |
| 1 | General error |
| 2 | Misuse of command |
| 125 | daemon error |
| 126 | Container command not executable |
| 127 | Container command not found |
| 130 | Container received SIGINT (128+2) |
| 137 | Container received SIGKILL (128+9) |
| 143 | Container received SIGTERM (128+15) |

## Source

- CLI: `cmd/doki/main.go` (cobra)
- Commands: `cmd/doki/*.go` (one file per command group)
- Shared flags: `pkg/cli/flags.go`
- Daemon API: `pkg/api/*.go`
