<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="680" height="200" viewBox="0 0 680 200">
    <defs>
      <linearGradient id="bg" x1="0%" y1="0%" x2="100%" y2="100%">
        <stop offset="0%" style="stop-color:#0f0f23"/>
        <stop offset="50%" style="stop-color:#1a1a3e"/>
        <stop offset="100%" style="stop-color:#0f0f23"/>
      </linearGradient>
      <linearGradient id="accent" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" style="stop-color:#6366f1"/>
        <stop offset="50%" style="stop-color:#818cf8"/>
        <stop offset="100%" style="stop-color:#6366f1"/>
      </linearGradient>
      <linearGradient id="textGrad" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" style="stop-color:#e0e7ff"/>
        <stop offset="100%" style="stop-color:#c7d2fe"/>
      </linearGradient>
      <filter id="glow">
        <feGaussianBlur stdDeviation="3" result="coloredBlur"/>
        <feMerge>
          <feMergeNode in="coloredBlur"/>
          <feMergeNode in="SourceGraphic"/>
        </feMerge>
      </filter>
      <filter id="softGlow">
        <feGaussianBlur stdDeviation="2" result="coloredBlur"/>
        <feMerge>
          <feMergeNode in="coloredBlur"/>
          <feMergeNode in="SourceGraphic"/>
        </feMerge>
      </filter>
    </defs>
    <rect width="680" height="200" rx="16" fill="url(#bg)" stroke="#6366f1" stroke-width="1" stroke-opacity="0.3"/>
    <!-- Animated dots -->
    <circle cx="50" cy="40" r="1.5" fill="#6366f1" opacity="0.4">
      <animate attributeName="opacity" values="0.2;0.8;0.2" dur="3s" repeatCount="indefinite"/>
    </circle>
    <circle cx="620" cy="30" r="1" fill="#818cf8" opacity="0.3">
      <animate attributeName="opacity" values="0.1;0.6;0.1" dur="4s" repeatCount="indefinite"/>
    </circle>
    <circle cx="100" cy="170" r="1" fill="#6366f1" opacity="0.3">
      <animate attributeName="opacity" values="0.2;0.7;0.2" dur="2.5s" repeatCount="indefinite"/>
    </circle>
    <circle cx="580" cy="160" r="1.5" fill="#818cf8" opacity="0.4">
      <animate attributeName="opacity" values="0.3;0.9;0.3" dur="3.5s" repeatCount="indefinite"/>
    </circle>
    <circle cx="340" cy="25" r="1" fill="#6366f1" opacity="0.3">
      <animate attributeName="opacity" values="0.1;0.5;0.1" dur="5s" repeatCount="indefinite"/>
    </circle>
    <!-- Container icon -->
    <g transform="translate(310, 30)" filter="url(#glow)">
      <rect x="0" y="0" width="60" height="40" rx="6" fill="none" stroke="url(#accent)" stroke-width="2"/>
      <rect x="8" y="8" width="12" height="12" rx="2" fill="#6366f1" opacity="0.8">
        <animate attributeName="opacity" values="0.5;1;0.5" dur="2s" repeatCount="indefinite"/>
      </rect>
      <rect x="24" y="8" width="12" height="12" rx="2" fill="#818cf8" opacity="0.6">
        <animate attributeName="opacity" values="0.3;0.8;0.3" dur="2.5s" repeatCount="indefinite"/>
      </rect>
      <rect x="40" y="8" width="12" height="12" rx="2" fill="#6366f1" opacity="0.4">
        <animate attributeName="opacity" values="0.2;0.7;0.2" dur="3s" repeatCount="indefinite"/>
      </rect>
      <line x1="8" y1="28" x2="52" y2="28" stroke="#6366f1" stroke-width="1.5" opacity="0.5"/>
    </g>
    <!-- Title -->
    <text x="340" y="115" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="42" font-weight="800" fill="url(#textGrad)" letter-spacing="8">DOKI</text>
    <!-- Subtitle -->
    <text x="340" y="145" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="13" fill="#94a3b8" letter-spacing="4">THE UNIVERSAL CONTAINER ENGINE</text>
    <!-- Divider line -->
    <line x1="240" y1="158" x2="440" y2="158" stroke="url(#accent)" stroke-width="1" opacity="0.4"/>
    <!-- Badges -->
    <text x="340" y="178" text-anchor="middle" font-family="monospace" font-size="10" fill="#64748b" letter-spacing="1">v0.9.1 · Go 1.26+ · Rust · Docker API v1.44 · Apache 2.0</text>
    <!-- Bottom accent line -->
    <line x1="0" y1="199" x2="680" y2="199" stroke="url(#accent)" stroke-width="1" opacity="0.2"/>
  </svg>
</p>

<p align="center">
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"></a>
  <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-doki--init-black?style=flat&logo=rust&logoColor=white" alt="Rust"></a>
  <a href="https://www.docker.com"><img src="https://img.shields.io/badge/API-Docker_v1.44-2496ED?style=flat&logo=docker&logoColor=white" alt="Docker API"></a>
  <a href="https://github.com/OpceanAI/Doki/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-555?style=flat" alt="License"></a>
  <a href="https://github.com/OpceanAI/Doki/releases"><img src="https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat&color=6366F1" alt="Downloads"></a>
  <a href="https://github.com/OpceanAI/Doki/stargazers"><img src="https://img.shields.io/github/stars/OpceanAI/Doki?style=flat&color=6366F1" alt="Stars"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#architecture">Architecture</a> &middot;
  <a href="#cli">CLI</a> &middot;
  <a href="#performance">Performance</a> &middot;
  <a href="#contributing">Contributing</a>
</p>

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <defs>
      <linearGradient id="lineGrad" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" style="stop-color:#6366f1;stop-opacity:0"/>
        <stop offset="50%" style="stop-color:#6366f1;stop-opacity:1"/>
        <stop offset="100%" style="stop-color:#6366f1;stop-opacity:0"/>
      </linearGradient>
    </defs>
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

# The Universal Container Engine

<p align="center">
  Docker and Podman compatible API &middot; OCI native &middot; Kubernetes CRI-ready<br>
  Runs on Linux, macOS, and Android via Termux &middot; ARM64, ARMv7 & x86_64<br>
  Rootless-first architecture &middot; No daemon required &middot; Hardware-level microVM isolation
</p>

<br>

<p align="center">
  <img src="https://img.shields.io/badge/Linux-000?style=for-the-badge&logo=linux&logoColor=fff" alt="Linux">
  <img src="https://img.shields.io/badge/macOS-000?style=for-the-badge&logo=apple&logoColor=fff" alt="macOS">
  <img src="https://img.shields.io/badge/Android-000?style=for-the-badge&logo=android&logoColor=3DDC84" alt="Android">
  <img src="https://img.shields.io/badge/Termux-000?style=for-the-badge" alt="Termux">
  <img src="https://img.shields.io/badge/Rootless-000?style=for-the-badge" alt="Rootless">
  <img src="https://img.shields.io/badge/CRI--Ready-000?style=for-the-badge" alt="CRI-Ready">
</p>

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <defs>
      <linearGradient id="lineGrad2" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" style="stop-color:#6366f1;stop-opacity:0"/>
        <stop offset="50%" style="stop-color:#6366f1;stop-opacity:1"/>
        <stop offset="100%" style="stop-color:#6366f1;stop-opacity:0"/>
      </linearGradient>
    </defs>
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad2)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Overview

Doki is a container engine designed for every Linux kernel, from Android phones to cloud servers. It works without root, without systemd, and without a hypervisor. When your hardware offers more -- KVM, Android's built-in hypervisors, Linux namespaces -- Doki scales up its isolation automatically.

| | |
|---|---|
| **Binary size** | 13 MB |
| **Memory (idle)** | 12 MB |
| **Start time** | <15ms |
| **Platforms** | Linux, macOS, Android (Termux) |
| **Architectures** | ARM64, ARMv7, x86_64 |
| **Runtime deps** | Zero |

<br>

## Comparison

| Metric | Doki | Docker | Podman | containerd |
|:-------|:----:|:------:|:------:|:----------:|
| Binary size | **13 MB** | 58 MB | 45 MB | 42 MB |
| Memory (idle) | **12 MB** | 85 MB | 60 MB | 55 MB |
| Start time | **<15ms** | ~50ms | ~30ms | ~40ms |
| Android support | **Yes** | No | No | No |
| Root required | **No** | Yes | Optional | Yes |
| Daemon required | **No** | Yes | No | Yes |
| microVM isolation | **Yes** | No | No | No |
| Zero dependencies | **Yes** | No | No | No |

<br>

## What Doki Replaces

| Instead of | Use Doki | Because |
|:-----------|:---------|:--------|
| Docker Desktop | `dokid` + `doki` | Same API, no VM overhead, works on Android |
| Podman | `dokid` + `doki` | Same pod abstraction, plus microVM isolation |
| containerd + crictl | `dokid` as CRI | Single binary instead of 3 daemons |
| Docker Compose | `doki-compose` | Same YAML, same commands, same workflow |
| Kubernetes (small deploys) | `doki kube play` | Run K8s YAML without a cluster |
| Lima / Colima (macOS) | `dokid` | Native container daemon, no Linux VM needed |
| Termux proot-distro | `doki run` | Actual OCI images instead of chroot tarballs |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Features

<table>
  <tr>
    <td width="50%" valign="top">
      <h3>Android Native</h3>
      <p>The only container engine that runs on Android via Termux without root. Designed for the constraints of mobile operating systems from the ground up.</p>
    </td>
    <td width="50%" valign="top">
      <h3>Rootless by Default</h3>
      <p>Works as a regular user. Scales to root or microVM isolation when available. No privilege escalation required for basic operations.</p>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h3>Docker Compatible</h3>
      <p>Same REST API v1.44. Drop-in replacement for Docker CLI and SDKs. docker-compose, docker-py, CI/CD pipelines all work without modification.</p>
    </td>
    <td width="50%" valign="top">
      <h3>Ultra Lightweight</h3>
      <p>13MB binary, 12MB RAM idle. 4x smaller than Docker, 7x less memory. doki-init rewritten in Rust: 412K vs 2.9MB Go, -86%.</p>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h3>4 Isolation Levels</h3>
      <p>MicroVM to Namespaces to Proot to Native. Auto-selected at runtime based on available hardware capabilities. Scales up automatically.</p>
    </td>
    <td width="50%" valign="top">
      <h3>Compose Support</h3>
      <p>Full Compose spec: networks, volumes, secrets, health checks, depends_on with 60s poll, 30+ fields including shm_size, pids_limit, ulimits.</p>
    </td>
  </tr>
  <tr>
    <td width="50%" valign="top">
      <h3>OCI Compliant</h3>
      <p>Push and pull to any OCI registry. Multi-architecture auto-resolution. Compatible with Docker Hub, GHCR, ECR, GCR, Quay, GitLab, Harbor.</p>
    </td>
    <td width="50%" valign="top">
      <h3>CRI-Ready</h3>
      <p>Kubernetes CRI plugin. Run K8s YAML without a cluster. PodSandbox, container management, and image service implemented.</p>
    </td>
  </tr>
</table>

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Quick Start

### Install

```bash
curl -sL https://doki.opceanai.com | sh
```

### First Run

```bash
# Start the daemon
dokid &

# Pull and run
doki pull alpine
doki run alpine echo "Hello from Doki"

# Check what's running
doki ps
doki images
```

### Use with Docker CLI

```bash
export DOCKER_HOST=unix:///var/run/doki.sock
docker ps
docker images
docker run alpine echo "via docker cli"
docker-compose up
```

### Use with Docker SDKs

```python
import docker
client = docker.DockerClient(base_url="unix:///var/run/doki.sock")
client.containers.run("alpine", "echo hello")
```

```javascript
const Docker = require('dockerode');
const docker = new Docker({ socketPath: '/var/run/doki.sock' });
docker.listContainers().then(console.log);
```

<br>

## Binaries

| Binary | Size | Description |
|:-------|:----:|:------------|
| **doki** | 9.2 MB | CLI with ~108 commands. Connects to daemon via Unix socket |
| **dokid** | 13 MB | Daemon. Docker Engine API v1.44 over Unix socket |
| **doki-compose** | 11 MB | Compose engine. Full spec support with health conditions |
| **doki-init-rust** | 412 KB | PID 1 for microVM guests. Rust, -86% vs Go |
| **doki-proot** | 14 KB | Forked proot with JSON IPC daemon mode |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Architecture

### Pipeline

When Doki runs a container, it goes through this pipeline:

1. **Image Resolution** -- Parse reference, contact registry, authenticate, resolve manifest for current architecture, download layers
2. **Rootfs Construction** -- Extract layers in order, build complete container filesystem with path traversal protection
3. **Execution Mode Selection** -- Probe system for available isolation: microVM, namespaces, proot, or native
4. **Process Execution** -- Execute container command within chosen isolation context with environment variables applied
5. **Lifecycle Management** -- Monitor process, record exit codes, write logs, execute health checks, enforce restart policies

### Isolation Levels

| Level | Mode | Isolation | Overhead | Requirements |
|:-----:|:-----|:----------|:---------|:-------------|
| **4** | MicroVM | Hardware-level | 5-20 MB RAM | KVM, Gunyah, GenieZone, Halla |
| **3** | Namespaces | Kernel-level | Negligible | Root + Linux namespaces |
| **2** | Proot | Userspace | ~10% CPU | proot binary |
| **1** | Native | None | Zero | Always available |

### MicroVM Support

DokiVM provides hardware-level isolation via lightweight virtual machines.

| Manufacturer | Chip Series | Hypervisor | VMM | Generation |
|:-------------|:------------|:-----------|:----|:-----------|
| Qualcomm | Snapdragon 8 Gen 1/2/3/4 | Gunyah | crosvm | 2022+ |
| MediaTek | Dimensity 7200/8200/9200/9300 | GenieZone | crosvm | 2023+ |
| Samsung | Exynos 2200/2400 | Halla | crosvm | 2022+ |
| Google | Tensor G1/G2/G3/G4 | KVM | crosvm | 2021+ |
| Intel | Core / Xeon | KVM | Firecracker | All KVM-capable |
| AMD | Ryzen / EPYC | KVM | Firecracker | All KVM-capable |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## CLI

Doki provides **108 commands** across 8 categories.

### Container Management

| Command | Description |
|:--------|:------------|
| `doki run` | Create and start a container (80+ flags) |
| `doki ps` | List containers |
| `doki create` | Create without starting |
| `doki start` | Start stopped containers |
| `doki stop` | Gracefully stop containers |
| `doki restart` | Stop and start containers |
| `doki kill` | Send signal to containers |
| `doki rm` | Remove containers |
| `doki exec` | Run command in running container |
| `doki logs` | Fetch container logs (streaming support) |
| `doki stats` | Live resource statistics |
| `doki top` | Display container processes |
| `doki inspect` | Detailed container info |
| `doki build` | Build image from Dokifile |
| `doki commit` | Create image from container |
| `doki attach` | Attach to container I/O |
| `doki wait` | Block until exit, return code |

### Image Management

| Command | Description |
|:--------|:------------|
| `doki pull` | Pull from any OCI registry (multi-arch auto-resolve) |
| `doki push` | Push to any OCI registry |
| `doki images` | List images with sizes |
| `doki rmi` | Remove images |
| `doki tag` | Tag an image |
| `doki build` | Build from Dokifile (18 instructions, multi-stage) |
| `doki login` / `doki logout` | Registry authentication |
| `doki search` | Search Docker Hub |

### Network, Volume, System

| Network | Volume | System |
|:--------|:-------|:-------|
| `doki network ls` | `doki volume ls` | `doki info` |
| `doki network create` | `doki volume create` | `doki version` |
| `doki network rm` | `doki volume rm` | `doki system df` |
| `doki network inspect` | `doki volume inspect` | `doki system prune` |
| `doki network connect` | `doki volume prune` | `doki system events` |
| `doki network disconnect` | | `doki ping` |
| `doki network prune` | | |

### Podman and Kubernetes

| Podman | Kubernetes |
|:-------|:-----------|
| `doki pod create/ps/rm/start/stop` | `doki kube play` |
| `doki generate kube` | `doki kube down` |
| `doki play kube` | `doki kube generate` |
| `doki auto-update` | `doki apply -f` |
| `doki unshare` / `untag` | |
| `doki mount` / `unmount` | |
| `doki healthcheck` | |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Dokifile Builder

Doki reads Dokifiles (or standard Dockerfiles) and builds OCI-compatible images. The parser supports all 18 Dockerfile instructions, multi-stage builds, heredocs, and parser directives.

### Supported Instructions

```
FROM      RUN       CMD       LABEL     EXPOSE    ENV
ADD       COPY      ENTRYPOINT VOLUME   USER      WORKDIR
ARG       ONBUILD   STOPSIGNAL HEALTHCHECK SHELL  MAINTAINER
```

### Example

```dockerfile
FROM alpine:latest AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY . .
RUN gcc -static -o app main.c

FROM alpine:latest
COPY --from=builder /build/app /usr/local/bin/app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q --spider http://localhost:8080/ || exit 1
USER nobody
CMD ["/usr/local/bin/app"]
```

<br>

## Compose

Full Compose Specification support for multi-container applications.

### Supported Features

| Feature | Description |
|:--------|:------------|
| `services` | Container definitions with full configuration |
| `networks` | Custom bridge/overlay networks |
| `volumes` | Persistent storage with driver options |
| `secrets` | Sensitive data injection with long syntax |
| `depends_on` | Startup ordering: `service_started`, `service_healthy` (60s poll), `service_completed_successfully` |
| `healthcheck` | Health probes per service |
| `deploy` | Resource limits (`cpus`, `memory`), `replicas`, `restart_policy` |
| `profiles` | Conditional service activation |
| `extends` | Service inheritance |
| `include` | Multi-file composition |
| Long syntax | Ports, volumes, devices, blkio_config, ulimits |

### Example

```yaml
name: production-stack

services:
  web:
    image: nginx:alpine
    ports: ["80:80", "443:443"]
    volumes:
      - web-data:/usr/share/nginx/html
    depends_on:
      api:
        condition: service_healthy
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 256M
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 10s
      retries: 3

  api:
    image: python:3-alpine
    command: uvicorn main:app --host 0.0.0.0
    environment:
      DATABASE_URL: postgresql://user:pass@db:5432/app
    depends_on:
      db:
        condition: service_started

  db:
    image: postgres:alpine
    volumes:
      - db-data:/var/lib/postgresql/data
    secrets:
      - db-password
```

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## REST API

Doki exposes the **Docker Engine API v1.44** over a Unix socket. 53 endpoints.

### Key Endpoints

<details>
<summary><b>Containers (16 endpoints)</b></summary>

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/containers/json` | List containers |
| `POST` | `/containers/create` | Create container |
| `GET` | `/containers/{id}/json` | Inspect container |
| `POST` | `/containers/{id}/start` | Start container |
| `POST` | `/containers/{id}/stop` | Stop container |
| `POST` | `/containers/{id}/restart` | Restart container |
| `POST` | `/containers/{id}/kill` | Kill container |
| `DELETE` | `/containers/{id}` | Remove container |
| `GET` | `/containers/{id}/logs` | Fetch logs |
| `POST` | `/containers/{id}/exec` | Create exec instance |
| `POST` | `/containers/{id}/attach` | Attach to container |
| `POST` | `/containers/prune` | Remove stopped containers |

</details>

<details>
<summary><b>Images (8 endpoints)</b></summary>

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/images/json` | List images |
| `POST` | `/images/create` | Pull image |
| `GET` | `/images/{name}/json` | Inspect image |
| `POST` | `/images/{name}/push` | Push image |
| `DELETE` | `/images/{name}` | Remove image |
| `POST` | `/images/prune` | Remove unused images |
| `GET` | `/images/search` | Search registry |

</details>

<details>
<summary><b>System & Other</b></summary>

| Method | Path | Description |
|:-------|:-----|:------------|
| `GET` | `/info` | System information |
| `GET` | `/version` | Version information |
| `GET` | `/_ping` | Health check |
| `GET` | `/events` | Event stream |
| `GET` | `/system/df` | Disk usage |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/health` | Daemon health |
| `POST` | `/auth` | Authentication |

</details>

<br>

## Networking

| Type | Description |
|:-----|:------------|
| **Bridge** | Default `doki0` bridge with NAT, DNS resolution, port mapping |
| **Host** | Share host network namespace (max performance) |
| **None** | Loopback only (complete isolation) |
| **CNI** | bridge, host-local, portmap, macvlan, ipvlan, dhcp, vlan |
| **Rootless** | Uses **pasta** for TCP/UDP without root or TAP devices |
| **IPv6** | Dual-stack IPv4/IPv6 on bridge networks |

### Port Mapping

```bash
doki run -p 8080:80 nginx:alpine                    # Map host 8080 to container 80
doki run -p 127.0.0.1:8080:80 nginx:alpine          # Bind to specific IP
doki run -p 8080:80/tcp -p 8080:80/udp              # TCP and UDP
doki run -P nginx:alpine                            # Publish all EXPOSEd ports
doki run -p 8080-8090:80 nginx:alpine               # Port range
```

<br>

## Storage

| Driver | Description | Best for |
|:-------|:------------|:---------|
| **overlay2** | Kernel overlay (direct syscall mount) | Linux with root, best performance |
| **fuse-overlayfs** | Userspace overlay via FUSE | Rootless, Termux, Android |
| **btrfs** | Btrfs subvolumes with snapshots | Systems with btrfs root |
| **zfs** | ZFS datasets with snapshots | Systems with ZFS pools |
| **vfs** | Simple directory copy | Testing, minimal systems |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Security

| Layer | Protection |
|:------|:-----------|
| **Seccomp** | 80+ allowed syscalls, blocks module loading, BPF, AF_ALG, hardware I/O |
| **AppArmor** | Template-based profiles per container |
| **User namespaces** | UID/GID remapping, root maps to unprivileged user |
| **Capabilities** | Minimal default set, explicit grants, `--cap-drop=ALL` support |
| **TLS** | Mutual TLS authentication with client certificates |
| **Rate limiting** | Token-bucket: 100 req/s, burst 200 |
| **Image verification** | Path traversal protection, symlink validation, hardlink restrictions |

### Blocked Syscalls

```
init_module, finit_module, delete_module    # Module loading
kexec_load, kexec_file_load                 # Kernel execution
iopl, ioperm                                # Hardware I/O
kcmp                                        # Kernel info leaks
process_vm_readv, process_vm_writev         # Process memory access
```

### Allowed Modern Syscalls

```
io_uring_setup, io_uring_enter, io_uring_register  # Async I/O
pidfd_open, pidfd_send_signal, pidfd_getfd         # PID file descriptors
rseq, userfaultfd, copy_file_range                 # Modern kernel features
landlock_create_ruleset, landlock_add_rule         # Landlock sandboxing
```

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Performance

Measured on **Qualcomm Snapdragon 685, Android 14, Termux**. Cold pull, ARM64 native binaries.

| Image | Size | Pull | Start | RAM |
|:------|-----:|-----:|------:|----:|
| busybox:latest | 1.8 MB | 1.4s | **8ms** | 0.6 MB |
| alpine:latest | 4.0 MB | 2.1s | **10ms** | 1.2 MB |
| python:3-alpine | 17.3 MB | 8.2s | **4ms** | 3.1 MB |
| redis:alpine | 15.2 MB | 7.1s | **6ms** | 2.8 MB |
| nginx:alpine | 24.6 MB | 11.5s | **12ms** | 5.8 MB |
| node:22-alpine | 48.7 MB | 22.8s | **15ms** | 12.3 MB |
| mariadb:latest | 156 MB | 62.4s | **20ms** | 31.2 MB |
| nextcloud:latest | 423 MB | 87.3s | **45ms** | 45.7 MB |

<br>

## Registries

Compatible with any OCI or Docker Registry HTTP API v2.

| Registry | Pull | Push | Auth |
|:---------|:----:|:----:|:-----|
| Docker Hub | Yes | Yes | Token |
| GitHub Container Registry | Yes | Yes | PAT |
| Quay.io | Yes | Yes | Robot |
| Google Container Registry | Yes | Yes | JSON key |
| Amazon ECR | Yes | Yes | IAM |
| Azure Container Registry | Yes | Yes | SP |
| GitLab Registry | Yes | Yes | Token |
| Harbor | Yes | Yes | Basic |
| Any OCI registry | Yes | Yes | Configurable |

### Verified Images

`alpine`, `busybox`, `python:3-alpine`, `node:22-alpine`, `nginx:alpine`, `redis:alpine`, `mariadb`, `postgres:alpine`, `nextcloud`, `ubuntu`, `debian`, `golang`, `rust`, `ruby`, `php`, `traefik`, `caddy`, `vault`

<br>

## Supported Distros

```bash
doki run --distro alpine   echo hello
doki run --distro ubuntu   bash
doki run --distro debian   --install curl,vim bash
doki run --distro arch
doki run --distro fedora
doki run --distro rocky
doki run --distro gentoo
doki run --distro opensuse
```

| Distro | Image | Size |
|:-------|:------|:-----|
| Alpine | `alpine:latest` | ~3MB |
| Ubuntu | `ubuntu:latest` | ~29MB |
| Debian | `debian:stable-slim` | ~27MB |
| Arch | `archlinux:latest` | ~150MB |
| Fedora | `fedora:latest` | ~95MB |
| Gentoo | `gentoo/stage3:latest` | ~200MB |
| OpenSUSE | `opensuse/tumbleweed:latest` | ~80MB |
| Rocky Linux | `rockylinux:latest` | ~100MB |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Configuration

### Daemon Config (`~/.doki/config.json`)

```json
{
  "root": "/data/data/com.termux/files/usr/var/lib/doki",
  "socket_path": "/data/data/com.termux/files/usr/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "default_network": "bridge",
  "debug": false,
  "log_level": "info",
  "rootless": true,
  "dns": ["8.8.8.8", "8.8.4.4"],
  "registry_mirrors": [],
  "insecure_registries": []
}
```

### Environment Variables

| Variable | Description | Default |
|:---------|:------------|:--------|
| `DOKI_HOST` | Daemon socket path | Platform-specific |
| `DOKI_DATA_DIR` | Data directory | `~/.doki/data` |
| `DOKI_STORAGE_DRIVER` | Storage driver | `fuse-overlayfs` |
| `DOKI_TLS` | Enable TLS | unset |
| `DOKI_TLS_CERT` | TLS certificate path | unset |
| `DOKI_TLS_KEY` | TLS key path | unset |
| `DOKI_KERNEL` | MicroVM kernel path | Platform-specific |
| `DOKI_NATIVE` | Force native mode | unset |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Building

### Requirements

- Go 1.22 or later
- `make` (optional)
- For microVM mode: `crosvm` or `firecracker` binary (auto-detected)

### Build Targets

```bash
# Android / Termux (ARM64)
make build-android
make install

# Android / Termux (ARMv7)
make build-armv7

# Linux (x86_64)
make build-linux
make install

# Linux (ARM64)
make build-linux-arm64

# macOS (ARM64)
make build-darwin-arm64

# Testing & linting
make test      # go test ./...
make vet       # go vet ./...
make lint      # golangci-lint run ./...
make clean     # rm -rf bin/
```

### Manual Build

```bash
go build -trimpath -ldflags="-s -w" -o bin/doki ./cmd/doki
go build -trimpath -ldflags="-s -w" -o bin/dokid ./cmd/dokid
go build -trimpath -ldflags="-s -w" -o bin/doki-compose ./cmd/doki-compose
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/doki-init ./cmd/doki-init
```

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Project Structure

```
Doki/
  cmd/
    doki/                 CLI binary (108 commands, 2200+ lines)
    dokid/                Daemon binary (REST API, TLS, gRPC, rate limiting)
    doki-compose/         Docker Compose compatible CLI
    doki-init-rust/       Minimal PID 1 for microVM guests (Rust, 412K)
  pkg/
    api/                  Docker Engine API v1.44 server (53 endpoints)
    runtime/              OCI runtime with 4 execution modes
    image/                OCI image management (pull, push, build)
    registry/             OCI Distribution Spec client
    network/              Container networking (bridge, CNI, DNS)
    storage/              Storage drivers (overlay2, fuse, btrfs, zfs)
    builder/              Dokifile parser (18 instructions, multi-stage)
    compose/              Compose engine
    cri/                  Kubernetes CRI plugin
    cli/                  CLI library (2200+ lines)
    common/               Shared types, config, utilities
  internal/
    dokivm/               MicroVM subsystem (crosvm, firecracker, qemu)
    namespaces/           Linux namespace management
    cgroups/              cgroups v2 resource management
    fuse/                 FUSE overlay filesystem operations
    proot/                proot fallback for Android
    seccomp/              Seccomp profile engine (80+ syscalls)
    apparmor/             AppArmor profile generator
  kernels/                Pre-compiled VM kernels (ARM64 + x86_64)
```

**40 Go source files. 14,500+ lines of code. 5 compiled binaries. Zero external dependencies.**

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Known Limitations

### What Works

| Feature | Status | Notes |
|:--------|:------:|:------|
| `doki run` | Tested | Basic commands, shell scripts, --init, --user, --entrypoint, --restart |
| `doki pull` | Tested | ARM64 multi-arch auto-resolve, parallel downloads, token auth |
| `doki push` | Tested | OCI Distribution Spec: blob upload, cross-repo mount, manifest PUT |
| `doki images` | Tested | Correct sizes, RepoDigests populated |
| `doki ps` / `doki ps -a` | Tested | Names, ports, image shown |
| `doki inspect` | Tested | Full JSON output |
| `doki stop` / `doki rm` | Tested | By name or ID, no deadlocks |
| `doki build` | Tested | RUN layers, COPY --from, ARG, ENV, .dockerignore, build cache |
| `doki logs` | Tested | Rotation (10MB/3 files), Docker multiplexed stream format |
| `doki exec` | Tested | Runs inside container via proot |
| `doki attach` | Tested | HTTP hijack, bidirectional streaming |
| `doki wait` | Tested | Multi-container, returns exit codes |
| `doki login` / `doki logout` | Tested | Token auth, Basic auth, credential wiring |
| `doki network ls` | Tested | Bridge/host/none, doki0 bridge creation |
| `doki volume create/ls/rm` | Tested | Local driver, tmpfs support |
| `doki-compose up/down` | Tested | Full compose spec: networks, volumes, secrets, healthcheck |
| Port forwarding (`-p`) | Tested | FirewallManager wired |

### What Does NOT Work Yet

| Feature | Status | Notes |
|:--------|:------:|:------|
| `doki cp` | Stub | Copy files host/container not implemented |
| MicroVM isolation | Untested | Code exists, not tested on compatible hardware |
| Kubernetes CRI | Stub | gRPC server not implemented |
| CNI networking | Untested | Plugin manager exists, not wired |
| Network bridge isolation | No | Containers share host network in proot/native mode |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## What's New

### v0.9.1 (Current)

- **OCI Push:** `doki push` -- blob upload, cross-repo mount, manifest PUT to any OCI registry
- **Registry Auth:** `doki login` accepts credentials and propagates to registry client
- **Native tar extraction:** Go-native tar with whiteouts, path traversal protection, compression auto-detection (gzip/bzip2/xz/zstd), parallel extraction with rollback
- **4 new distros:** Fedora, Gentoo, OpenSUSE, Rocky Linux -- 8 distros total
- **Improved Compose engine:** Long syntax Ports/Volumes, `depends_on` health conditions with 60s poll, 30+ new fields
- **19 Proot C fixes:** SECCOMP_RET_ALLOW, fake_id0 brace bug, stat.c uid/gid fix, link2symlink UB, and more
- **Updated seccomp:** io_uring, pidfd, rseq, userfaultfd, copy_file_range now allowed
- **Overlay2 kernel mount:** Uses `syscall.Mount("overlay")` directly instead of FUSE delegation
- **Attach via HTTP hijack:** `doki attach` with bidirectional streaming
- **Multi-container wait:** Waits for multiple containers simultaneously
- **DNS listener:** Internal DNS server on port 53 for inter-container resolution
- **Buffer pool & String intern pool:** Reduced GC pressure and memory deduplication
- **PProf endpoint:** `/debug/pprof/` for profiling
- **Systemd socket activation:** Linux socket activation support
- **ARMv7 beta:** Compilation and binaries for 32-bit ARM devices

### v0.9.0

- **doki-init-rust:** PID 1 rewritten in Rust (412K vs 2.9MB Go, -86%)
- **doki-proot:** Forked proot with daemon mode + JSON IPC protocol. 14K binary
- **Distro system:** `doki run --distro alpine/ubuntu/debian/arch` downloads from Docker Hub
- **ARMv7 beta:** Full feature parity for older ARM devices
- **Immich:** Full stack running (PostgreSQL 18 + pgvector + cube + earthdistance, Redis 7, Immich Server v2.7.5)

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## Contributing

Contributions are welcome. Areas where help is most needed:

| Area | Description |
|:-----|:------------|
| **MicroVM backends** | Support for additional hypervisors and platforms |
| **CNI plugins** | Implementation of advanced networking features |
| **Security** | Hardening, fuzzing, and penetration testing |
| **Performance** | Layer caching, parallel operations, memory optimization |
| **Testing** | Integration tests, end-to-end tests, stress tests |
| **Documentation** | Tutorials, examples, and API reference |

### Development Setup

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
go build ./...
go test ./...
```

### Commit Style

- Use imperative mood ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="600" height="80" viewBox="0 0 600 80">
    <line x1="0" y1="40" x2="600" y2="40" stroke="url(#lineGrad)" stroke-width="1"/>
    <circle cx="300" cy="40" r="4" fill="#6366f1">
      <animate attributeName="r" values="3;5;3" dur="2s" repeatCount="indefinite"/>
    </circle>
  </svg>
</p>

## License

Apache License 2.0

```
Copyright 2024-2026 OpceanAI

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

<br>

## Links

| Platform | Repository |
|:---------|:-----------|
| Website | [doki.opceanai.com](https://doki.opceanai.com) |
| GitHub | [OpceanAI/Doki](https://github.com/OpceanAI/Doki) |
| GitLab | [aguitauwu/doki](https://gitlab.com/aguitauwu/doki) |
| Codeberg | [aguitauwu/Doki](https://codeberg.org/aguitauwu/Doki) |

### Related

| Repository | Description |
|:-----------|:------------|
| [Doki-proot](https://github.com/OpceanAI/Doki-proot) | Forked proot with JSON IPC daemon mode for Doki |

<br>

<p align="center">
  <svg xmlns="http://www.w3.org/2000/svg" width="400" height="60" viewBox="0 0 400 60">
    <defs>
      <linearGradient id="footerGrad" x1="0%" y1="0%" x2="100%" y2="0%">
        <stop offset="0%" style="stop-color:#6366f1;stop-opacity:0"/>
        <stop offset="50%" style="stop-color:#6366f1;stop-opacity:0.6"/>
        <stop offset="100%" style="stop-color:#6366f1;stop-opacity:0"/>
      </linearGradient>
    </defs>
    <line x1="0" y1="30" x2="400" y2="30" stroke="url(#footerGrad)" stroke-width="1"/>
    <text x="200" y="50" text-anchor="middle" font-family="system-ui, -apple-system, sans-serif" font-size="11" fill="#64748b" letter-spacing="2">THE CONTAINER ENGINE FOR THE OTHER 3 BILLION DEVICES</text>
  </svg>
</p>

<p align="center">
  <a href="https://github.com/OpceanAI">
    <img src="https://img.shields.io/badge/Made_by_OpceanAI-2026-000?style=flat" alt="OpceanAI">
  </a>
</p>
