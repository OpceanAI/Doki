# Podman REST API (libpod v5) - Complete Reference for doki-pod

## Overview

**Total API Endpoints:** 184  
**API Base Path:** `/v{version}/` (versioned) and `/` (non-versioned for Docker compat)  
**Two API namespaces:**
- **Docker Compatibility API:** `/containers/`, `/images/`, etc. (no `/libpod/` prefix)
- **Libpod Native API:** `/libpod/containers/`, `/libpod/pods/`, etc. (Podman-specific features)

---

## 1. System & Infrastructure Endpoints

### 1.1 Ping & Version

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/_ping` | Return protocol info in headers (API-Version, Libpod-API-Version, BuildKit-Version) |
| GET | `/version` | Display version information (compat) |
| GET | `/libpod/version` | Component version information (libpod) |
| GET | `/info` | Display system information (compat) |
| GET | `/libpod/info` | Display Podman-related system information |

### 1.2 Events

| Method | Path | Description |
|--------|------|-------------|
| GET | `/events` | Monitor real-time events (compat) |
| GET | `/libpod/events` | Monitor Podman events (libpod) |

**Query Parameters:**
- `since`, `until` - Timestamp filters
- `filters` - JSON encoded filters (event, container, image, pod, network, volume, etc.)

### 1.3 Auth

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth` | Validate credentials with a registry |

---

## 2. Container Endpoints

### 2.1 Docker Compatibility API (`/containers/`)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/containers/create` | Create a container |
| GET | `/containers/json` | List containers |
| DELETE | `/containers/{name}` | Remove a container |
| GET | `/containers/{name}/json` | Inspect container |
| POST | `/containers/{name}/kill` | Kill container |
| GET | `/containers/{name}/logs` | Get container logs |
| POST | `/containers/{name}/pause` | Pause container |
| POST | `/containers/{name}/restart` | Restart container |
| POST | `/containers/{name}/start` | Start container |
| GET | `/containers/{name}/stats` | Get container stats (streaming) |
| POST | `/containers/{name}/stop` | Stop container |
| GET | `/containers/{name}/top` | List processes |
| POST | `/containers/{name}/unpause` | Unpause container |
| POST | `/containers/{name}/wait` | Wait on container |
| POST | `/containers/{name}/attach` | Attach to container |
| POST | `/containers/{name}/resize` | Resize TTY |
| GET | `/containers/{name}/export` | Export container |
| POST | `/containers/{name}/rename` | Rename container |
| POST | `/containers/{name}/update` | Update container config |
| POST | `/containers/prune` | Delete stopped containers |

### 2.2 Libpod Native API (`/libpod/containers/`)

**Podman-specific endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/containers/create` | Create container (uses SpecGenerator) |
| GET | `/libpod/containers/json` | List containers (with namespace info) |
| DELETE | `/libpod/containers/{name}` | Delete container |
| GET | `/libpod/containers/{name}/json` | Inspect container (libpod format) |
| POST | `/libpod/containers/{name}/kill` | Kill container |
| POST | `/libpod/containers/{name}/mount` | **Mount container to filesystem** |
| POST | `/libpod/containers/{name}/unmount` | **Unmount container** |
| GET | `/libpod/containers/{name}/logs` | Get logs |
| POST | `/libpod/containers/{name}/pause` | Pause container |
| POST | `/libpod/containers/{name}/restart` | Restart container |
| POST | `/libpod/containers/{name}/start` | Start container |
| GET | `/libpod/containers/{name}/stats` | Get stats (deprecated, use /stats) |
| GET | `/libpod/containers/stats` | **Get stats for multiple containers** |
| GET | `/libpod/containers/{name}/top` | List processes (with streaming) |
| POST | `/libpod/containers/{name}/unpause` | Unpause container |
| POST | `/libpod/containers/{name}/wait` | Wait on container |
| GET | `/libpod/containers/{name}/exists` | **Check if container exists** |
| POST | `/libpod/containers/{name}/stop` | Stop container |
| POST | `/libpod/containers/{name}/attach` | Attach to container |
| POST | `/libpod/containers/prune` | Delete stopped containers |
| GET | `/libpod/containers/showmounted` | **Show mounted containers** |

**Podman-specific container operations:**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/containers/{name}/checkpoint` | **Checkpoint container (CRIU)** |
| POST | `/libpod/containers/{name}/restore` | **Restore container from checkpoint** |
| GET | `/libpod/containers/{name}/healthcheck` | **Execute healthcheck manually** |
| POST | `/libpod/containers/{name}/init` | **Initialize container without starting** |
| GET | `/libpod/containers/{name}/changes` | **Report filesystem changes** |
| POST | `/libpod/containers/{name}/exec` | Create exec session |
| POST | `/libpod/containers/{name}/archive` | **Get/put archive from/to container** |

**Key Query Parameters for Libpod Containers:**
- `namespace` - Include namespace information
- `pod` - Include pod details
- `sync` - Sync container state with OCI runtime
- `external` - Include external containers

---

## 3. Pod Endpoints (Podman-Specific)

**Pods are a Podman-exclusive feature not available in Docker.**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/pods/create` | Create a pod |
| GET | `/libpod/pods/json` | List pods |
| DELETE | `/libpod/pods/{name}` | Remove pod |
| GET | `/libpod/pods/{name}/json` | Inspect pod |
| GET | `/libpod/pods/{name}/exists` | Check if pod exists |
| POST | `/libpod/pods/{name}/kill` | Kill pod |
| POST | `/libpod/pods/{name}/pause` | Pause pod |
| POST | `/libpod/pods/{name}/unpause` | Unpause pod |
| POST | `/libpod/pods/{name}/restart` | Restart pod |
| POST | `/libpod/pods/{name}/start` | Start pod |
| POST | `/libpod/pods/{name}/stop` | Stop pod |
| GET | `/libpod/pods/{name}/top` | List processes in pod |
| GET | `/libpod/pods/stats` | **Statistics for pods** |
| POST | `/libpod/pods/prune` | Prune unused pods |

**PodSpecGenerator Fields:**
- `name` - Pod name
- `hostname` - Pod hostname
- `labels` - Key-value labels
- `cgroup_parent` - Cgroup parent
- `shared_namespaces` - Namespaces to share (ipc, net, pid, uts)
- `infra_container` - Infra container configuration
- `containers` - Container specifications
- `networks` - Network configuration
- `portmappings` - Port mappings
- `dns_server`, `dns_search`, `dns_option` - DNS configuration
- `hostadd` - Host entries
- `security_opt` - Security options
- `volumes` - Volume mounts
- `restart_policy` - Restart policy

---

## 4. Image Endpoints

### 4.1 Docker Compatibility API (`/images/`)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/images/json` | List images |
| POST | `/images/create` | Pull/create image |
| GET | `/images/get` | Export multiple images |
| POST | `/images/load` | Load images from archive |
| POST | `/images/prune` | Remove unused images |
| GET | `/images/search` | Search registry for image |
| DELETE | `/images/{name}` | Remove image |
| GET | `/images/{name}/json` | Inspect image |
| GET | `/images/{name}/history` | Show image history |
| POST | `/images/{name}/push` | Push image to registry |
| POST | `/images/{name}/tag` | Tag an image |
| GET | `/images/{name}/get` | Export single image |

### 4.2 Libpod Native API (`/libpod/images/`)

**Podman-specific endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/images/json` | List images |
| POST | `/libpod/images/pull` | Pull image |
| POST | `/libpod/images/remove` | Remove image(s) |
| GET | `/libpod/images/{name}/json` | Inspect image |
| GET | `/libpod/images/{name}/history` | Image history |
| POST | `/libpod/images/{name}/push` | Push image |
| POST | `/libpod/images/{name}/tag` | Tag image |
| POST | `/libpod/images/{name}/untag` | **Remove tag from image** |
| GET | `/libpod/images/{name}/exists` | **Check if image exists** |
| GET | `/libpod/images/{name}/get` | Export image |
| GET | `/libpod/images/{name}/changes` | **Report filesystem changes** |
| GET | `/libpod/images/{name}/tree` | **Show image tree (layer info)** |
| GET | `/libpod/images/{name}/resolve` | **Resolve image name to ID** |
| POST | `/libpod/images/import` | **Import image from archive** |
| POST | `/libpod/images/export` | **Export multiple images** |
| POST | `/libpod/images/load` | Load images |
| POST | `/libpod/images/prune` | Remove unused images |
| GET | `/libpod/images/search` | Search for images |
| POST | `/libpod/images/scp/{name}` | **Secure copy image to remote host** |

**Key Query Parameters:**
- `tlsVerify` - Require HTTPS and verify signatures
- `all` - Show all images (including intermediate)
- `digests` - Show digests
- `compress` - Compress image on export
- `format` - Export format

---

## 5. Build Endpoints

### 5.1 Docker Compatibility API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/build` | Build image from Dockerfile |

### 5.2 Libpod Native API

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/build` | Build image (with Podman-specific options) |
| POST | `/libpod/local/build` | **Build image locally (no remote context)** |

**Podman-specific Build Parameters:**
- `allplatforms` - Build for all available platforms
- `additionalbuildcontexts` - Additional build contexts
- `nohosts` - Don't create /etc/hosts
- `compatvolumes` - Volume modification on ADD/COPY
- `createdannotation` - Add creation annotation
- `sourcedateepoch` - Timestamp for reproducible builds
- `rewritetimestamp` - Rewrite timestamps
- `inheritlabels` - Inherit labels from base image
- `inheritannotations` - Inherit annotations from base image

---

## 6. Commit Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/commit` | Create image from container (compat) |
| POST | `/libpod/commit` | Create image from container (libpod) |

---

## 7. Exec Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/containers/{name}/exec` | Create exec session (compat) |
| POST | `/exec/{id}/start` | Start exec session |
| POST | `/exec/{id}/resize` | Resize exec TTY |
| GET | `/exec/{id}/json` | Inspect exec session |
| POST | `/libpod/containers/{name}/exec` | Create exec session (libpod) |
| POST | `/libpod/exec/{id}/start` | Start exec session |
| POST | `/libpod/exec/{id}/resize` | Resize exec TTY |
| GET | `/libpod/exec/{id}/json` | Inspect exec session |

---

## 8. Volume Endpoints

### 8.1 Docker Compatibility API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/volumes` | List volumes |
| POST | `/volumes/create` | Create volume |
| DELETE | `/volumes/{name}` | Remove volume |
| POST | `/volumes/prune` | Remove unused volumes |

### 8.2 Libpod Native API

**Podman-specific endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/volumes/json` | List volumes |
| POST | `/libpod/volumes/create` | Create volume |
| DELETE | `/libpod/volumes/{name}` | Remove volume |
| GET | `/libpod/volumes/{name}/json` | Inspect volume |
| GET | `/libpod/volumes/{name}/exists` | **Check if volume exists** |
| GET | `/libpod/volumes/{name}/export` | **Export volume as tar** |
| POST | `/libpod/volumes/{name}/import` | **Import tar into volume** |
| POST | `/libpod/volumes/prune` | Remove unused volumes |

---

## 9. Network Endpoints

### 9.1 Docker Compatibility API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/networks` | List networks |
| POST | `/networks/create` | Create network |
| DELETE | `/networks/{name}` | Remove network |
| POST | `/networks/{name}/connect` | Connect container to network |
| POST | `/networks/{name}/disconnect` | Disconnect container from network |
| POST | `/networks/prune` | Remove unused networks |

### 9.2 Libpod Native API

**Podman-specific endpoints:**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/networks/json` | List networks |
| POST | `/libpod/networks/create` | Create network |
| DELETE | `/libpod/networks/{name}` | Remove network |
| GET | `/libpod/networks/{name}/json` | Inspect network |
| GET | `/libpod/networks/{name}/exists` | **Check if network exists** |
| POST | `/libpod/networks/{name}/connect` | Connect container |
| POST | `/libpod/networks/{name}/disconnect` | Disconnect container |
| POST | `/libpod/networks/{name}/update` | **Update network configuration** |
| POST | `/libpod/networks/prune` | Remove unused networks |

**Podman Network Features:**
- `network_interface` - Specify network interface name
- `dns_enabled` - Enable DNS for network
- `ipv6_enabled` - Enable IPv6
- `internal` - Restrict external access
- `labels` - Network labels
- `options` - Driver-specific options

---

## 10. Manifest Endpoints (Podman-Specific)

**Manifest lists allow managing multi-architecture images.**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/manifests/{name}` | Create manifest list |
| PUT | `/libpod/manifests/{name}` | **Modify manifest list (add/remove images)** |
| DELETE | `/libpod/manifests/{name}` | Delete manifest list |
| GET | `/libpod/manifests/{name}/json` | Inspect manifest list |
| GET | `/libpod/manifests/{name}/exists` | Check if manifest exists |
| POST | `/libpod/manifests/{name}/add` | **Add image to manifest (deprecated)** |
| POST | `/libpod/manifests/{name}/push` | **Push manifest (deprecated)** |
| POST | `/libpod/manifests/{name}/registry/{destination}` | **Push manifest to registry** |

**ManifestModifyOptions:**
- `operation` - "add" or "remove"
- `images` - List of images to add/remove
- `all` - Add all images if given list
- `annotation` - Annotations to add
- `os`, `architecture`, `variant` - Platform specifications

---

## 11. Secret Endpoints (Podman-Specific)

**Secrets management for sensitive data.**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/secrets` | List secrets (compat) |
| POST | `/secrets/create` | Create secret (compat) |
| DELETE | `/secrets/{name}` | Remove secret (compat) |
| GET | `/libpod/secrets/json` | List secrets |
| POST | `/libpod/secrets/create` | Create secret |
| DELETE | `/libpod/secrets/{name}` | Remove secret |
| GET | `/libpod/secrets/{name}/json` | Inspect secret |
| GET | `/libpod/secrets/{name}/exists` | Check if secret exists |

**Secret Creation Parameters:**
- `name` - Secret name
- `driver` - Secret driver (default: "file")
- `driveropts` - Driver options (JSON)
- `labels` - Labels (JSON)
- `replace` - Replace existing secret
- `ignore` - Ignore if exists

---

## 12. Generate Endpoints (Podman-Specific)

**Generate structured data from containers/pods.**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/generate/{name}/systemd` | **Generate systemd unit files** |
| GET | `/libpod/generate/kube` | **Generate Kubernetes YAML** |

### 12.1 Generate Systemd

**Query Parameters:**
- `useName` - Use names instead of IDs
- `new` - Create new container vs start existing
- `noHeader` - Omit version header
- `startTimeout` - Start timeout in seconds
- `stopTimeout` - Stop timeout in seconds
- `restartPolicy` - no, on-success, on-failure, on-abnormal, on-watchdog, on-abort, always
- `containerPrefix` - Unit name prefix for containers
- `podPrefix` - Unit name prefix for pods
- `separator` - Name separator
- `restartSec` - Restart delay
- `wants` - Systemd Wants list
- `after` - Systemd After list
- `requires` - Systemd Requires list
- `additionalEnvVariables` - Environment variables
- `templateUnitFile` - Add template specifier

### 12.2 Generate Kube

**Query Parameters:**
- `names` - Container/pod names (array)
- `service` - Generate Service object
- `type` - Kubernetes kind (pod, deployment, etc.)
- `replicas` - Replica count for Deployment
- `noTrunc` - Don't truncate annotations
- `podmanOnly` - Include Podman-only annotations

---

## 13. Play Kube Endpoints (Podman-Specific)

**Deploy Kubernetes YAML files.**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/play/kube` | **Play Kubernetes YAML file** |
| DELETE | `/libpod/play/kube` | **Tear down resources from kube play** |

**Play Kube Parameters:**
- `annotations` - Annotations (JSON)
- `logDriver` - Logging driver
- `logOptions` - Log driver options
- `network` - Network mode/name
- `noHosts` - Don't setup /etc/hosts
- `noTrunc` - Don't truncate annotations
- `publishPorts` - Port mappings
- `publishAllPorts` - Publish all containerPort/hostPort
- `replace` - Replace existing pods
- `serviceContainer` - Start service container
- `start` - Start pod after creation
- `staticIPs` - Static IP addresses
- `staticMACs` - Static MAC addresses
- `tlsVerify` - Require HTTPS
- `userns` - User namespace mode
- `wait` - Clean up on SIGTERM
- `build` - Build images from context

**Content-Type:**
- `plain/text` - YAML format
- `application/x-tar` - Tar with play.yaml and build contexts

---

## 14. Kube Apply Endpoint (Podman-Specific)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/kube/apply` | **Apply Kubernetes YAML to running system** |

---

## 15. Quadlet Endpoints (Podman-Specific)

**Quadlet is a systemd integration feature for declarative container management.**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/quadlets` | **Install quadlet files** |
| DELETE | `/libpod/quadlets` | **Remove quadlet files (batch)** |
| GET | `/libpod/quadlets/json` | **List quadlets** |
| DELETE | `/libpod/quadlets/{name}` | **Remove specific quadlet** |
| GET | `/libpod/quadlets/{name}/exists` | **Check if quadlet exists** |
| GET | `/libpod/quadlets/{name}/file` | **Get quadlet file contents** |

**Quadlet Install Parameters:**
- `replace` - Replace existing files
- `reload-systemd` - Reload systemd after install

**Content-Type:**
- `application/x-tar` - Tar archive with quadlet file
- `multipart/form-data` - Form data upload

---

## 16. Artifact Endpoints (Podman-Specific)

**OCI artifacts management.**

| Method | Path | Description |
|--------|------|-------------|
| POST | `/libpod/artifacts/add` | **Add file as artifact** |
| POST | `/libpod/artifacts/local/add` | **Add local file as artifact** |
| POST | `/libpod/artifacts/pull` | **Pull artifact from registry** |
| POST | `/libpod/artifacts/{name}/push` | **Push artifact to registry** |
| GET | `/libpod/artifacts/{name}/json` | **Inspect artifact** |
| GET | `/libpod/artifacts/{name}/extract` | **Extract artifact as tar** |
| DELETE | `/libpod/artifacts/{name}` | **Remove artifact** |
| GET | `/libpod/artifacts/json` | **List artifacts** |
| POST | `/libpod/artifacts/remove` | **Remove artifacts (batch)** |

---

## 17. System Management Endpoints

### 17.1 Docker Compatibility API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/system/df` | Show disk usage |

### 17.2 Libpod Native API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/system/df` | Show disk usage |
| POST | `/libpod/system/prune` | **Prune unused data** |
| POST | `/libpod/system/check` | **Check storage consistency** |

**System Prune Parameters:**
- `all` - Remove all unused data
- `volumes` - Prune volumes
- `external` - Remove images used by external containers
- `build` - Remove build cache
- `filters` - Filters (until, label)

**System Check Parameters:**
- `quick` - Skip time-consuming checks
- `repair` - Remove inconsistent images
- `repair_lossy` - Remove inconsistent containers and images
- `unreferenced_layer_max_age` - Max age of unreferenced layers

---

## 18. Healthcheck Endpoints (Podman-Specific)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/libpod/containers/{name}/healthcheck` | **Manually execute healthcheck** |

---

## 19. Distribution Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/distribution/{name}/json` | Get image information from registry |

---

## 20. Swarm Endpoints (Docker Compatibility)

**Podman provides stub implementations for Docker Swarm API compatibility.**

---

## 21. Plugins Endpoints (Docker Compatibility)

**Podman provides stub implementations for Docker plugin API compatibility.**

---

## Podman-Specific Features (Not in Docker)

### 1. **Pods**
- Group of containers sharing namespaces
- Infra container for pod lifecycle
- Pod-level operations (start, stop, pause, kill)
- Pod statistics and monitoring

### 2. **Manifest Lists**
- Multi-architecture image management
- Add/remove images from manifest
- Push manifest lists to registries

### 3. **Quadlet (Systemd Integration)**
- Declarative container management via systemd
- .container, .pod, .volume, .network, .kube, .image, .build files
- Automatic systemd unit generation
- Lifecycle management via systemctl

### 4. **Generate Commands**
- Generate systemd unit files from containers/pods
- Generate Kubernetes YAML from containers/pods
- Template support and customization

### 5. **Play Kube**
- Deploy Kubernetes YAML files
- Support for Pods, Deployments, Services, ConfigMaps, Secrets, PVCs
- Build images from context
- Replace existing resources

### 6. **Artifacts**
- OCI artifact management
- Add files as artifacts
- Push/pull artifacts to/from registries
- Extract artifact contents

### 7. **Secrets Management**
- Secure storage for sensitive data
- Multiple secret drivers (file, pass, etc.)
- Secret injection into containers

### 8. **Checkpoint/Restore**
- CRIU integration for container migration
- Live migration support
- Checkpoint export/import

### 9. **Auto-Update**
- Automatic container image updates
- Label-based update policies
- Systemd integration for auto-updates

### 10. **Container Mount/Unmount**
- Mount container filesystem to host
- Direct filesystem access
- Unmount when done

### 11. **Volume Export/Import**
- Export volumes as tar archives
- Import tar archives into volumes
- Volume backup and migration

### 12. **Image SCP**
- Secure copy images between hosts
- SSH-based transfer
- No registry required

### 13. **Healthcheck Manual Execution**
- Manually trigger healthchecks
- Get healthcheck results on demand

### 14. **Network Update**
- Update network configuration
- Modify network settings without recreation

### 15. **System Check**
- Storage consistency verification
- Repair inconsistent state
- Layer cleanup

---

## Podman CLI Commands (for reference)

### Container Management
- `podman attach` - Attach to running container
- `podman create` - Create new container
- `podman exec` - Execute command in container
- `podman init` - Initialize container
- `podman kill` - Kill container
- `podman logs` - Display container logs
- `podman pause` - Pause container
- `podman port` - List port mappings
- `podman ps` - List containers
- `podman rename` - Rename container
- `podman restart` - Restart container
- `podman rm` - Remove container
- `podman run` - Run command in container
- `podman start` - Start container
- `podman stats` - Display resource usage
- `podman stop` - Stop container
- `podman top` - Display running processes
- `podman unpause` - Unpause container
- `podman update` - Update container config
- `podman wait` - Wait on container

### Pod Management
- `podman pod create` - Create pod
- `podman pod inspect` - Inspect pod
- `podman pod kill` - Kill pod
- `podman pod pause` - Pause pod
- `podman pod ps` - List pods
- `podman pod restart` - Restart pod
- `podman pod rm` - Remove pod
- `podman pod start` - Start pod
- `podman pod stats` - Pod statistics
- `podman pod stop` - Stop pod
- `podman pod top` - List pod processes
- `podman pod unpause` - Unpause pod

### Image Management
- `podman build` - Build image
- `podman commit` - Create image from container
- `podman diff` - Inspect changes
- `podman history` - Show image history
- `podman images` - List images
- `podman import` - Import tarball as image
- `podman load` - Load image from archive
- `podman pull` - Pull image from registry
- `podman push` - Push image to registry
- `podman rmi` - Remove image
- `podman save` - Save image to archive
- `podman search` - Search registry
- `podman tag` - Tag image
- `podman untag` - Remove tag

### Volume Management
- `podman volume create` - Create volume
- `podman volume inspect` - Inspect volume
- `podman volume ls` - List volumes
- `podman volume prune` - Remove unused volumes
- `podman volume rm` - Remove volume
- `podman volume export` - Export volume
- `podman volume import` - Import volume

### Network Management
- `podman network connect` - Connect container to network
- `podman network create` - Create network
- `podman network disconnect` - Disconnect container
- `podman network inspect` - Inspect network
- `podman network ls` - List networks
- `podman network prune` - Remove unused networks
- `podman network rm` - Remove network
- `podman network update` - Update network

### Manifest Management
- `podman manifest add` - Add image to manifest
- `podman manifest annotate` - Annotate manifest
- `podman manifest create` - Create manifest
- `podman manifest exists` - Check if manifest exists
- `podman manifest inspect` - Inspect manifest
- `podman manifest push` - Push manifest
- `podman manifest remove` - Remove from manifest
- `podman manifest rm` - Remove manifest

### Secret Management
- `podman secret create` - Create secret
- `podman secret inspect` - Inspect secret
- `podman secret ls` - List secrets
- `podman secret rm` - Remove secret

### Kubernetes Integration
- `podman kube down` - Tear down kube resources
- `podman kube generate` - Generate kube YAML
- `podman kube play` - Play kube YAML
- `podman kube apply` - Apply kube YAML

### System Management
- `podman system check` - Check storage
- `podman system connection` - Manage connections
- `podman system df` - Show disk usage
- `podman system events` - Monitor events
- `podman system info` - Display system info
- `podman system migrate` - Migrate containers
- `podman system prune` - Remove unused data
- `podman system renumber` - Renumber locks
- `podman system reset` - Reset storage
- `podman system service` - Start API service

### Quadlet Management
- `podman quadlet install` - Install quadlet
- `podman quadlet list` - List quadlets
- `podman quadlet remove` - Remove quadlet

### Other Commands
- `podman auto-update` - Auto-update containers
- `podman cp` - Copy files to/from container
- `podman export` - Export container filesystem
- `podman generate kube` - Generate kube YAML
- `podman generate systemd` - Generate systemd units
- `podman healthcheck run` - Run healthcheck
- `podman login` - Login to registry
- `podman logout` - Logout from registry
- `podman mount` - Mount container
- `podman unmount` - Unmount container
- `podman unshare` - Run in user namespace
- `podman version` - Display version

---

## Key Data Structures

### SpecGenerator (Container Creation)
```go
type SpecGenerator struct {
    ContainerBasicConfig
    ContainerStorageConfig
    ContainerSecurityConfig
    ContainerNetworkConfig
    ContainerCgroupConfig
    ContainerHealthCheckConfig
    ContainerResourceConfig
}
```

**Key Fields:**
- `name`, `image`, `command`, `entrypoint`
- `env`, `labels`, `annotations`
- `volumes`, `mounts`
- `portmappings`, `networks`
- `resource_limits`, `cgroup_parent`
- `privileged`, `readonly_filesystem`
- `healthconfig`, `restart_policy`
- `pod` - Join existing pod
- `init_container_type` - Init container mode
- `systemd` - Systemd mode
- `sdnotifyMode` - sd-notify handling

### PodSpecGenerator
```go
type PodSpecGenerator struct {
    InfraContainer    *InfraContainerConfig
    Name              string
    Hostname          string
    Labels            map[string]string
    CgroupParent      string
    SharedNamespaces  []string
    Containers        []ContainerConfig
    Networks          map[string]PerNetworkOptions
    PortMappings      []PortMapping
    DNSServer         []net.IP
    DNSSearch         []string
    DNSOption         []string
    HostAdd           []string
    SecurityOpt       []string
    Volumes           []*NamedVolume
    RestartPolicy     string
}
```

---

## API Versioning

Podman uses semantic versioning for the API:
- **Major version:** Breaking changes
- **Minor version:** New features (backward compatible)
- **Patch version:** Bug fixes

**Current API Version:** 5.x (as of Podman 5.x)

**Version Headers:**
- `API-Version` - Max Docker compat version
- `Libpod-API-Version` - Max Podman API version
- `Libpod-Buildah-Version` - Buildah version

---

## Authentication & Security

**Registry Authentication:**
- `X-Registry-Auth` header - Base64 encoded auth config
- `X-Registry-Config` header - Registry configuration

**TLS:**
- `tlsVerify` parameter - Require HTTPS
- Client certificate support
- CA certificate bundles

---

## Streaming Endpoints

**Endpoints that return streaming responses:**
- `/containers/{name}/attach` - Container I/O streams
- `/containers/{name}/logs` - Log streams
- `/containers/{name}/stats` - Resource usage stats
- `/events` - Event stream
- `/libpod/containers/{name}/top` - Process list (with streaming)

**Stream Format:**
- Content-Type: `application/vnd.docker.raw-stream` or `application/vnd.docker.multiplexed-stream`
- 8-byte header: `[STREAM_TYPE, 0, 0, 0, SIZE1, SIZE2, SIZE3, SIZE4]`
- STREAM_TYPE: 0=stdin, 1=stdout, 2=stderr

---

## Error Handling

**Standard HTTP Status Codes:**
- 200 - Success
- 201 - Created
- 204 - No Content
- 304 - Not Modified (already started/stopped)
- 400 - Bad Request
- 401 - Unauthorized
- 404 - Not Found
- 409 - Conflict (state conflict)
- 500 - Internal Server Error

**Error Response Format:**
```json
{
    "cause": "error cause",
    "message": "detailed error message",
    "response": 500
}
```

---

## Implementation Notes for doki-pod

1. **Dual API Support:** Implement both Docker compat and libpod native APIs
2. **Pod Infrastructure:** Implement pod infra containers for pod lifecycle
3. **Namespace Sharing:** Support shared namespaces within pods
4. **Systemd Integration:** Implement quadlet for declarative management
5. **Kubernetes Compatibility:** Support play kube and generate kube
6. **Manifest Lists:** Implement multi-architecture image support
7. **Secrets Management:** Implement secure secret storage
8. **Checkpoint/Restore:** Integrate CRIU for migration
9. **Auto-Update:** Implement label-based update policies
10. **Streaming:** Implement proper stream multiplexing for attach/logs/stats

---

## References

- **Podman API Documentation:** https://docs.podman.io/en/latest/_static/api.html
- **Swagger Specification:** https://storage.googleapis.com/libpod-master-releases/swagger-latest.yaml
- **Podman CLI Reference:** https://docs.podman.io/en/latest/markdown/podman.1.html
- **Podman GitHub:** https://github.com/containers/podman
- **Quadlet Documentation:** https://docs.podman.io/en/latest/markdown/podman-systemd.unit.5.html
