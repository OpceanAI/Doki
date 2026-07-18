# 通用容器引擎

![Doki Banner](whaley.gif)

为 Docker 无法触及的地方提供无 root 容器。

<p>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go&logoColor=white" alt="Go"></a>
  <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-doki--init-black?style=flat&logo=rust&logoColor=white" alt="Rust"></a>
  <a href="https://github.com/OpceanAI/Doki/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Apache_2.0-555?style=flat" alt="License"></a>
  <a href="https://github.com/OpceanAI/Doki/releases"><img src="https://img.shields.io/github/downloads/OpceanAI/Doki/total?style=flat&color=6366F1" alt="Downloads"></a>
  <a href="https://github.com/OpceanAI/Doki/stargazers"><img src="https://img.shields.io/github/stars/OpceanAI/Doki?style=flat&color=6366F1" alt="Stars"></a>
</p>

兼容 Docker 和 Podman API -- OCI 原生 -- Kubernetes CRI 就绪。
运行于 Linux、macOS 和 Android (通过 Termux) -- ARM64、ARMv7、x86_64。
无 root 优先架构 -- 无需守护进程 -- 硬件级 microVM 隔离。

---

## 概述

Doki 是一个为每个 Linux 内核设计的容器引擎,从 Android 手机到云服务器。它无需 root、无需 systemd、无需 hypervisor。当你的硬件提供更多支持时 -- KVM、Android 内置 hypervisor、Linux namespaces -- Doki 会自动提升其隔离级别。

| 指标 | 值 |
|:-------|:------|
| **版本** | v0.12.0 |
| **二进制大小** | 13 MB |
| **内存 (空闲)** | 12 MB |
| **启动时间** | <15ms |
| **平台** | Linux、macOS、Android (Termux) |
| **架构** | ARM64、ARMv7、x86_64 |
| **运行时依赖** | 零 |

### 各平台二进制可用性 (v0.12.0)

| 平台 | doki | dokid | doki-compose | doki-init | doki-kube | doki-kubectl |
|:---------|:----:|:-----:|:------------:|:---------:|:---------:|:------------:|
| Linux ARM64 | 是 | 是 | 是 | 是 | 是 | 是 |
| Linux ARMv7 | 是 | 是 | 是 | 是 | 是 | 是 |
| Linux x86_64 | 是 | 是 | 是 | 是 | 是 | 是 |
| Android ARM64 (Termux) | 是 | 是 | 是 | 是 | 是 | 是 |
| Android ARMv7 (Termux) | 是 | 是 | 是 | 是 | 是 | 是 |
| macOS ARM64 (Apple Silicon) | 是 | -- | -- | -- | 是 | 是 |
| macOS x86_64 (Intel) | 是 | -- | -- | -- | 是 | 是 |

**注意:** Android ARMv7 二进制使用 `GOOS=linux` 构建 (Go 1.22+ 在 32 位 ARM 上需要外部链接器用于 `GOOS=android`)。二进制通过 proot 运行;Android 检测使用文件系统探测。

`dokid`、`doki-compose` 和 `doki-init` 仅限 Linux/Android -- 它们依赖 Linux namespaces、cgroups v2 和 overlayfs 系统调用。在 macOS 上,`doki` 仅在 `ModeNative` 下运行,并在需要时通过网络连接到远程守护进程。`doki-kube` 和 `doki-kubectl` 在 macOS 上作为客户端二进制可用。

---

## 对比

| 指标 | Doki | Docker | Podman | containerd |
|:-------|:----:|:------:|:------:|:----------:|
| 二进制大小 | **13 MB** | 58 MB | 45 MB | 42 MB |
| 内存 (空闲) | **12 MB** | 85 MB | 60 MB | 55 MB |
| 启动时间 | **<15ms** | ~50ms | ~30ms | ~40ms |
| Android 支持 | **是** | 否 | 否 | 否 |
| 需要 root | **否** | 是 | 可选 | 是 |
| 需要守护进程 | **否** | 是 | 否 | 是 |
| microVM 隔离 | **是** | 否 | 否 | 否 |
| 零依赖 | **是** | 否 | 否 | 否 |

---

## Doki 替代什么

| 替代 | 使用 Doki | 原因 |
|:-----------|:---------|:--------|
| Docker Desktop | `dokid` + `doki` | 相同 API,无 VM 开销,可在 Android 上运行 |
| Podman | `dokid` + `doki` | Pod、secret 和 manifest API 在同一 socket 上,加上可扩展隔离 |
| containerd + crictl | `dokid` 作为 CRI | 单个二进制而非 3 个守护进程 |
| Docker Compose | `doki-compose` | 相同 YAML,相同命令,相同工作流 |
| Kubernetes (小型部署) | `doki kube play` | 无需集群即可运行 K8s YAML |
| Lima / Colima (macOS) | `dokid` | 原生容器守护进程,无需 Linux VM |
| Termux proot-distro | `doki run` | 实际 OCI 镜像而非 chroot tarball |
| kubectl + minikube | `doki-kubectl` + `doki-kube` | 单二进制 K8s 控制平面;在单节点上运行你的 YAML |

---

## 特性

### Android 原生

唯一通过 Termux 在 Android 上无 root 运行的容器引擎。从头开始为移动操作系统的限制而设计。当 `/proc/sys/net` 不可用时,通过 proot 回退使用主机网络命名空间。

### 默认无 Root

以普通用户身份运行。当可用时,可扩展到 root 或 microVM 隔离。基本操作无需权限提升。

### Docker 兼容

通过相同的 Unix socket 使用 Docker Engine REST API。将 `DOCKER_HOST` 指向 `dokid`,docker CLI、docker-py、dockerode 和 docker-compose 无需修改即可连接 -- 常见的容器、镜像、网络、卷、exec 和构建流程已实现并经过测试。

### 超轻量

13MB 二进制,12MB RAM 空闲。比 Docker 小 4 倍,内存使用少 7 倍。

### 12 级隔离

从 WASM 沙箱到 pKVM 硬件隔离。根据可用硬件在运行时自动选择。每种设备都有合适的模式:无 root 的手机、带 KVM 的服务器、带 pKVM 的 Chromebook,或需要在 ARM 上进行 x86 仿真的笔记本。v0.11 新增:pKVM/Microdroid 检测和 macOS VZ 后端。

### Compose 支持

完整 Compose 规范:网络、卷、secrets、健康检查、带 60 秒轮询的 depends_on、30+ 字段包括 shm_size、pids_limit、ulimits。健康检查执行引擎运行周期性探测并端到端报告容器健康状态。

### OCI 兼容

推送到任何 OCI 仓库或从任何 OCI 仓库拉取。多架构自动解析。兼容 Docker Hub、GHCR、ECR、GCR、Quay、GitLab、Harbor。

### Kubernetes 控制平面

单二进制 Kubernetes 控制平面:带真实资源 watch 的 apiserver、过滤和评分节点的调度器、协调控制器 (Deployment 到 ReplicaSet 到 Pod、Job、Endpoints、Service、垃圾回收)、kube-proxy (iptables/nftables/userspace 模式) 和 CoreDNS。`dokid` 暴露可用的 CRI gRPC 运行时,因此真实的 kubelet 或 crictl 可以驱动它,`doki kube play` 从 Kubernetes YAML 运行实际容器。kubectl 兼容 CLI。专注于单节点;控制平面自身的 kubelet-over-CRI 连接和持久存储仍在完善中。

### DokiLink Mesh

点对点多主机容器网络。通过 STUN (RFC 8489) 进行 NAT 遍历,使用 TCP 同时打开打洞和 TURN 中继回退。DHT 对等发现 (Kademlia,160 位,k=8,alpha=3)。mDNS LAN 发现,90 秒过期和清理循环。TLS 1.3 加密,可选 NaCl secretbox。Ed25519 身份,TOFU 信任模型。

### Podman API v5

39 个端点兼容 podman-remote 客户端。Pod、secret 和 manifest 管理。与 Docker API 一起挂载在同一 socket 上,共享 TLS、中间件和速率限制。

### macOS 原生虚拟化

通过 cgo 桥接到 Virtualization.framework 的 VZ 后端,适用于 macOS 11+。QEMU 后端作为 Intel Mac 或 VZ 不可用时的回退。Sandbox 后端用于轻量级隔离,无 VM 开销。

### 诊断

`doki deps` 工具用于主机依赖验证,包含 `ls`、`check` (CI 门控)、`go` (Go 模块依赖) 和 `install` (通过检测到的包管理器尽力安装)。`doki doctor` 用于环境健康检查。

### 跨架构仿真

在 ARM 上运行 x86 容器,反之亦然,无需内核支持。三个后端:QEMU 用户模式 (`qemu-x86_64-static`、`qemu-aarch64-static`)、FEX-Emu (x86-on-ARM,针对 Termux/Android 优化) 和 Box64 (轻量级 x86_64 仿真器)。通过 `doki emulator ls|set|detect` 或 `DOKI_EMULATION_MODE=qemu|fex|box64|auto` 配置。持久化偏好存储在 `~/.doki/emulation.json` 中,支持原子写入和环境变量覆盖。自动检测为你的主机架构选择最佳可用后端。

---

## 快速开始

### 安装

```bash
curl -sL https://dok1.xyz | sh
```

### 首次运行

```bash
# 在默认 Unix socket 上启动守护进程
dokid &

# 使用显式 socket 路径启动守护进程
dokid --host unix:///var/run/doki.sock &

# 使用 TCP 监听器启动守护进程 (用于远程访问)
dokid --host tcp://0.0.0.0:2375 &

# 拉取并运行
doki pull alpine
doki run alpine echo "Hello from Doki"

# 检查正在运行的内容
doki ps
doki images
```

### 与 Docker CLI 一起使用

```bash
export DOCKER_HOST=unix:///var/run/doki.sock
docker ps
docker images
docker run alpine echo "via docker cli"
docker-compose up
```

### 与 Docker SDK 一起使用

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

### 与 Kubernetes 一起使用

```bash
# 启动 K8s 控制平面
doki kube play my-app.yaml

# 使用 kubectl 兼容 CLI 管理
doki-kubectl get pods
doki-kubectl apply -f deployment.yaml
doki-kubectl describe pod web-abc123
doki-kubectl logs web-abc123
```

---

## 二进制文件

| 二进制 | 大小 | 描述 |
|:-------|:----:|:------------|
| **doki** | 6.7 MB | 带 108+ 命令的 CLI。通过 Unix socket 连接到守护进程 |
| **dokid** | 9.2 MB | 守护进程。Docker Engine API v1.54 + Podman API v5 通过 Unix socket |
| **doki-compose** | 7.6 MB | Compose 引擎,带 watch、publish、健康检查执行和完整规范支持 |
| **doki-init** | 2.9 MB | microVM 客户机的 PID 1 (Go)。源代码中有 Rust 变体 |
| **doki-kube** | 8.1 MB | Kubernetes 控制平面 (apiserver、kubelet、scheduler、controllers、kube-proxy、CoreDNS) |
| **doki-kubectl** | 4.3 MB | 用于管理 Kubernetes 资源的 kubectl 兼容 CLI |

---

## 架构

### 流水线

当 Doki 运行容器时,它会经过这个流水线:

1. **镜像解析** -- 解析引用,联系仓库,认证,解析当前架构的清单,下载层
2. **Rootfs 构建** -- 按顺序解压层,构建完整的容器文件系统,带路径遍历保护
3. **执行模式选择** -- 探测系统并从 12 种可用模式中选择最佳运行器:WASM、pKVM、microVM、sysbox、namespaces、gVisor、FEX、QEMU、proot、legacy32、chroot 或 native
4. **进程执行** -- 在选定的隔离上下文中执行容器命令,应用环境变量
5. **生命周期管理** -- 监控进程,记录退出代码,写入日志,执行健康检查,执行重启策略

### 隔离级别

Doki 选择你硬件上可用的最强隔离模式。每种模式都针对特定用例:

| 级别 | 模式 | 隔离 | 开销 | 原因 / 何时使用 |
|:-----:|:-----|:----------|:---------|:-----------|
| **12** | WASM | 沙箱 (用户空间) | 最小 | 为不受信任的代码运行 WASI/Wasm 容器。无系统调用泄漏到主机。用于插件、无服务器功能或多语言微服务 |
| **11** | pKVM/Microdroid | 硬件级 (vm) | 5-20 MB RAM | Android 15+ 受保护 VM。Google 的 pKVM 将工作负载与主机 OS 彼此隔离。用于 Chromebook/手机上的敏感计算 |
| **10** | MicroVM | 硬件级 (vm) | 5-20 MB RAM | KVM、Gunyah、GenieZone、Halla hypervisor。完全硬件隔离,微秒级启动。当你需要 VM 级安全性与容器速度时使用 |
| **9** | Sysbox | 内核级 (DinD) | 中等 | 通过 sysbox-runc 实现无 root Docker-in-Docker。当你需要在容器内运行完整 Docker 守护进程时使用 (CI 运行器、构建农场) |
| **8** | Namespaces | 内核级 | 可忽略 | 标准 Linux namespace 隔离。用于有 root 访问权限的服务器。受信任的多租户工作负载的最佳性能 |
| **7** | gVisor | 用户空间内核 | ~20% CPU | Google 的 runsc 在用户空间边界拦截系统调用。当你想要纵深防御而不使用 VM 时使用 -- 70% 的系统调用永远不会到达主机 |
| **6** | FEX-Emu | 仿真 (ARM 上的 x86) | ~30% CPU | FEXInterpreter 或 Box64。在 ARM64 上运行 x86/x86_64 二进制,无需重新编译。用于 Apple Silicon 或 ARM 服务器上的遗留 x86 容器 |
| **5** | QEMU User | 仿真 (跨架构) | ~50% CPU | 适用于任何客户机架构的 QEMU 用户模式。当你需要运行为不同架构构建的容器时使用 (例如,arm64 上的 arm32,或任何架构上的任何架构) |
| **4** | Proot | 用户空间 (ptrace) | ~10% CPU | 基于 Ptrace 的 chroot,无需 root。Android/Termux 上的默认值。用于缺少 root 和 namespaces 的设备 -- 手机、平板、ChromeOS Linux |
| **3** | Legacy32 | 双架构兼容 | 可忽略 | 通过 binfmt_misc 和多架构支持在 ARM64 内核上运行 ARMv7 容器。当你的工作负载仅作为 32 位 ARM 提供时使用 |
| **2** | Chroot | 文件系统级 | 最小 | 通过 chroot 实现轻量级文件系统隔离。用于快速测试、构建阶段,或当所有其他模式都不可用时 |
| **1** | Native | 无 | 零 | 直接主机执行。始终作为回退可用。当你信任工作负载并想要零开销时使用 |

### 隔离级别检测

`pkg/runtime/registry.go` 中的运行器注册表探测主机并选择可用的最强模式。探测顺序 (自上而下,第一个通过的获胜):

| 级别 | 模式 | 检测探测 |
|:-----:|:-----|:----------------|
| 12 | WASM | `which wasmedge` 或 `which iwasm` |
| 11 | pKVM/Microdroid | `/dev/kvm` 可读 + Android 15+ |
| 10 | MicroVM | `/dev/kvm` 可读 + `$PATH` 中有 `crosvm`/`firecracker` |
| 9 | Sysbox | `$PATH` 中有 `sysbox-runc` |
| 8 | Namespaces | `unshare --user --map-root-user true` 退出 0 |
| 7 | gVisor | `$PATH` 中有 `runsc` |
| 6 | FEX-Emu | `$PATH` 中有 `FEXInterpreter` 或 `box64` |
| 5 | QEMU User | `$PATH` 中有 `qemu-aarch64-static` / `qemu-x86_64-static` 等 |
| 4 | Proot | `$PATH` 中有 `proot` (或已提供) |
| 3 | Legacy32 | `binfmt_misc` 已注册 + 多架构 qemu |
| 2 | Chroot | 始终 (使用 `chroot(2)`) |
| 1 | Native | 始终 (无隔离) |

使用 `doki run --runtime <mode>` 覆盖:

```bash
doki run --runtime proot alpine echo "always proot"
doki run --runtime native alpine echo "no isolation"
doki run --runtime wasm wasi-example.wasm
```

### MicroVM 支持

DokiVM 通过轻量级虚拟机提供硬件级隔离。

| 制造商 | 芯片系列 | Hypervisor | VMM | 世代 |
|:-------------|:------------|:-----------|:----|:-----------|
| Qualcomm | Snapdragon 8 Gen 1/2/3/4 | Gunyah | crosvm | 2022+ |
| MediaTek | Dimensity 7200/8200/9200/9300 | GenieZone | crosvm | 2023+ |
| Samsung | Exynos 2200/2400 | Halla | crosvm | 2022+ |
| Google | Tensor G1/G2/G3/G4 | KVM | crosvm | 2021+ |
| Intel | Core / Xeon | KVM | Firecracker | 所有支持 KVM 的 |
| AMD | Ryzen / EPYC | KVM | Firecracker | 所有支持 KVM 的 |

### macOS 虚拟化

在 macOS 上,Doki 提供三个 VM 后端:

| 后端 | 技术 | 要求 | 最适合 |
|:--------|:-----------|:-------------|:---------|
| **VZ** | 通过 cgo/ObjC 桥接的 Virtualization.framework | macOS 11+,Apple Silicon | 原生性能,Rosetta 支持,最小开销 |
| **QEMU** | 带 HVF 加速器的 QEMU | macOS 10.15+,Intel 或 Apple Silicon | VZ 不可用时的回退,ARM 上的 x86 仿真 |
| **Sandbox** | macOS sandbox-exec 配置文件 | macOS 10.7+ | 无完整 VM 开销的轻量级隔离 |

VZ 后端使用 `VZVirtualMachineConfiguration`、`VZLinuxBootLoader`、`VZVirtioFileSystemDevice` (带共享目录) 和 `VZBridgedNetworkDevice`/`VZNATNetworkDevice`。构建标签 `darwin && cgo`,需要 `CGO_ENABLED=1`。

---

## CLI

Doki 提供 **108 个命令**,分为 8 个类别。

### 容器管理

| 命令 | 描述 |
|:--------|:------------|
| `doki run` | 创建并启动容器 (80+ 标志) |
| `doki ps` | 列出容器 |
| `doki create` | 创建但不启动 |
| `doki start` | 启动已停止的容器 |
| `doki stop` | 优雅停止容器 |
| `doki restart` | 停止并启动容器 |
| `doki kill` | 向容器发送信号 |
| `doki rm` | 移除容器 |
| `doki exec` | 在运行中的容器内运行命令 |
| `doki logs` | 获取容器日志 (支持流式传输) |
| `doki stats` | 实时资源统计 |
| `doki top` | 显示容器进程 |
| `doki inspect` | 详细容器信息 |
| `doki build` | 从 Dokifile 构建镜像 |
| `doki commit` | 从容创建镜像 |
| `doki attach` | 附加到容器 I/O |
| `doki wait` | 阻塞直到退出,返回代码 |
| `doki cp` | 在主机和容器之间复制文件 |

### 镜像管理

| 命令 | 描述 |
|:--------|:------------|
| `doki pull` | 从任何 OCI 仓库拉取 (多架构自动解析) |
| `doki push` | 推送到任何 OCI 仓库 |
| `doki images` | 列出带大小的镜像 |
| `doki rmi` | 移除镜像 |
| `doki tag` | 标记镜像 |
| `doki build` | 从 Dokifile 构建 (18 条指令,多阶段) |
| `doki login` / `doki logout` | 仓库认证 |
| `doki search` | 搜索 Docker Hub |

### 网络、卷、系统

| 网络 | 卷 | 系统 |
|:--------|:-------|:-------|
| `doki network ls` | `doki volume ls` | `doki info` |
| `doki network create` | `doki volume create` | `doki version` |
| `doki network rm` | `doki volume rm` | `doki system df` |
| `doki network inspect` | `doki volume inspect` | `doki system prune` |
| `doki network connect` | `doki volume prune` | `doki system events` |
| `doki network disconnect` | | `doki ping` |
| `doki network prune` | | |

### Podman 和 Kubernetes

| Podman | Kubernetes |
|:-------|:-----------|
| `doki pod create/ps/rm/start/stop` | `doki kube play` |
| `doki generate kube` | `doki kube down` |
| `doki play kube` | `doki kube generate` |
| `doki auto-update` | `doki apply -f` |
| `doki unshare` / `untag` | |
| `doki mount` / `unmount` | |
| `doki healthcheck` | |

### DokiLink Mesh

| 命令 | 描述 |
|:--------|:------------|
| `doki mesh status` | 显示安装 ID 和 Ed25519 公钥 |
| `doki mesh ls` | 列出已知对等体 |
| `doki link add <name> <addr> --pub <key>` | 添加静态对等体 |
| `doki link rm <name>` | 移除静态对等体 |

### 诊断

| 命令 | 描述 |
|:--------|:------------|
| `doki doctor` | 验证主机环境和依赖 |
| `doki deps ls` | 列出所有系统依赖及其状态 |
| `doki deps check` | CI 门控:如果缺少必需依赖则非零退出 |
| `doki deps go` | 审计 Go 模块依赖 |
| `doki deps install <name>` | 通过检测到的包管理器尽力安装 |

### 跨架构仿真

| 命令 | 描述 |
|:--------|:------------|
| `doki emu show` | 显示保存的仿真器偏好和配置路径 |
| `doki emu detect` | 扫描 PATH 中的 QEMU/FEX/Box64 后端及版本 |
| `doki emu test` | 运行检测并在保存建议前询问 |
| `doki emu set <mode>` | 设置偏好:`auto`、`qemu`、`fex`、`box64` |

仿真系统将偏好持久化在 `~/.doki/emulation.json` 中,支持原子写入 (tmp+rename,0600 权限)。两个环境变量覆盖磁盘配置:`DOKI_EMULATION_MODE` 和 `DOKI_EMULATOR` (别名)。自动检测在 ARM64 主机上首选 FEX-Emu,然后是 Box64,最后是 QEMU 用户模式作为通用回退。具有外部架构的容器镜像 (例如,ARM64 主机上的 `linux/amd64`) 由运行器注册表自动路由通过选定的仿真器。

---

## Dokifile 构建器

Doki 读取 Dokifile (或标准 Dockerfile) 并构建 OCI 兼容镜像。解析器支持所有 18 条 Dockerfile 指令、多阶段构建、heredoc 和解析器指令。

### 支持的指令

```
FROM      RUN       CMD       LABEL     EXPOSE    ENV
ADD       COPY      ENTRYPOINT VOLUME   USER      WORKDIR
ARG       ONBUILD   STOPSIGNAL HEALTHCHECK SHELL  MAINTAINER
```

### 示例

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

---

## Compose

完整 Compose 规范支持多容器应用。

### 支持的特性

| 特性 | 描述 |
|:--------|:------------|
| `services` | 带完整配置的容器定义 |
| `networks` | 自定义 bridge/overlay 网络 |
| `volumes` | 带驱动选项的持久存储 |
| `secrets` | 带长语法的敏感数据注入 |
| `depends_on` | 启动顺序:`service_started`、`service_healthy` (60 秒轮询)、`service_completed_successfully` |
| `healthcheck` | 每个服务的健康探测,带真实周期性执行引擎 |
| `deploy` | 资源限制 (`cpus`、`memory`)、`replicas`、`restart_policy` |
| `profiles` | 条件服务激活 |
| `extends` | 服务继承 |
| `include` | 多文件组合 |
| `watch` | 通过 fsnotify 进行文件监视,用于开发期间的热重载 |
| `publish` | 基于 compose 的部署的服务网格集成 |
| 长语法 | 端口、卷、设备、blkio_config、ulimits |

### 示例

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

---

## REST API

Doki 通过 Unix socket 暴露 **Docker Engine API v1.54** 和 **Podman libpod API v5**。两个 API 共享同一服务器,继承 TLS、中间件和速率限制。

### Docker Engine API -- 容器 (16 个端点)

| 方法 | 路径 | 描述 |
|:-------|:-----|:------------|
| `GET` | `/containers/json` | 列出容器 |
| `POST` | `/containers/create` | 创建容器 |
| `GET` | `/containers/{id}/json` | 检查容器 |
| `POST` | `/containers/{id}/start` | 启动容器 |
| `POST` | `/containers/{id}/stop` | 停止容器 |
| `POST` | `/containers/{id}/restart` | 重启容器 |
| `POST` | `/containers/{id}/kill` | 终止容器 |
| `DELETE` | `/containers/{id}` | 移除容器 |
| `GET` | `/containers/{id}/logs` | 获取日志 |
| `POST` | `/containers/{id}/exec` | 创建 exec 实例 |
| `POST` | `/containers/{id}/attach` | 附加到容器 |
| `POST` | `/containers/prune` | 移除已停止的容器 |

### Docker Engine API -- 镜像 (7 个端点)

| 方法 | 路径 | 描述 |
|:-------|:-----|:------------|
| `GET` | `/images/json` | 列出镜像 |
| `POST` | `/images/create` | 拉取镜像 |
| `GET` | `/images/{name}/json` | 检查镜像 |
| `POST` | `/images/{name}/push` | 推送镜像 |
| `DELETE` | `/images/{name}` | 移除镜像 |
| `POST` | `/images/prune` | 移除未使用的镜像 |
| `GET` | `/images/search` | 搜索仓库 |

### Docker Engine API -- 系统和其他

| 方法 | 路径 | 描述 |
|:-------|:-----|:------------|
| `GET` | `/info` | 系统信息 |
| `GET` | `/version` | 版本信息 |
| `GET` | `/_ping` | 健康检查 |
| `GET` | `/events` | 事件流 |
| `GET` | `/system/df` | 磁盘使用 |
| `GET` | `/metrics` | Prometheus 指标 |
| `GET` | `/health` | 守护进程健康 |
| `POST` | `/auth` | 认证 |

### Podman API (39 个端点)

| 类别 | 端点 |
|:---------|:----------|
| **Pods** | `/libpod/pods/create`、`/libpod/pods/json`、`/libpod/pods/{id}/json`、`/libpod/pods/{id}/start`、`/libpod/pods/{id}/stop`、`/libpod/pods/{id}/restart`、`/libpod/pods/{id}/kill`、`/libpod/pods/{id}/pause`、`/libpod/pods/{id}/unpause`、`/libpod/pods/{id}/exists`、`/libpod/pods/{id}`、`/libpod/pods/prune` |
| **Secrets** | `/libpod/secrets/create`、`/libpod/secrets/json`、`/libpod/secrets/{id}/json`、`/libpod/secrets/{id}` |
| **Manifests** | `/libpod/manifests/create`、`/libpod/manifests/{name}/add`、`/libpod/manifests/{name}/remove`、`/libpod/manifests/{name}/json`、`/libpod/manifests/{name}/push`、`/libpod/manifests/json` |

### Kubernetes API

| 方法 | 路径 | 描述 |
|:-------|:-----|:------------|
| `GET` | `/api/v1/pods` | 列出 pods |
| `POST` | `/api/v1/pods` | 创建 pod |
| `GET` | `/api/v1/services` | 列出 services |
| `POST` | `/api/v1/services` | 创建 service |
| `GET` | `/apis/apps/v1/deployments` | 列出 deployments |
| `POST` | `/apis/apps/v1/deployments` | 创建 deployment |
| `GET` | `/version` | 服务器版本信息 |

完整 API 组路径:`api/v1`、`apis/apps/v1`、`apis/batch/v1`、`networking.k8s.io/v1`、`rbac.authorization.k8s.io/v1`。

### CRI gRPC (41 个 RPC)

CRI 插件实现完整的 Kubernetes 容器运行时接口:

| 服务 | RPC | 描述 |
|:--------|:-----|:------------|
| RuntimeService | 35 | RunPodSandbox、StopPodSandbox、RemovePodSandbox、PodSandboxStatus、ListPodSandbox、CreateContainer、StartContainer、StopContainer、RemoveContainer、ListContainers、ContainerStatus、UpdateContainerResources、ExecSync、Exec、Attach、PortForward 等 |
| ImageService | 6 | ListImages、ImageStatus、PullImage、RemoveImage、ImageFsInfo |

---

## 网络

### 网络类型

| 类型 | 描述 |
|:-----|:------------|
| **Bridge** | 默认 `doki0` 桥接,带 NAT、DNS 解析、端口映射 |
| **Host** | 共享主机网络命名空间 (最大性能)。在 Termux/Android 上,当 `/proc/sys/net` 不可用时,通过 proot 回退到主机网络 |
| **None** | 仅环回 (完全隔离) |
| **CNI** | bridge、host-local、portmap、macvlan、ipvlan、dhcp、vlan |
| **Rootless** | 使用 **pasta** 进行 TCP/UDP,无需 root 或 TAP 设备 |
| **IPv6** | bridge 网络上的双栈 IPv4/IPv6 |

### 端口映射

```bash
doki run -p 8080:80 nginx:alpine                    # 将主机 8080 映射到容器 80
doki run -p 127.0.0.1:8080:80 nginx:alpine          # 绑定到特定 IP
doki run -p 8080:80/tcp -p 8080:80/udp              # TCP 和 UDP
doki run -P nginx:alpine                            # 发布所有 EXPOSEd 端口
doki run -p 8080-8090:80 nginx:alpine               # 端口范围
```

### Termux 特定网络

在 Android/Termux 上,非特权进程无法访问主机网络命名空间。Doki 在启动时检测到此情况并:
- 通过 proot 回退到 `host` 网络模式,共享 Termux 应用的网络命名空间
- DNS 监听在 `127.0.0.11:8053` (端口 53 被 SELinux 阻止)
- 上游 DNS 解析器从 `getprop net.dns1..net.dns4` 读取
- 端口映射在无 root 模式下使用 socat (iptables 不可用)

使用 `DOKI_DNS_LISTEN=IP:PORT` 或 `config.json` 中的 `dns.listen` 覆盖 DNS 监听地址。

### 端口转发内部

端口映射在 root 模式下使用 iptables DNAT,在无 root 模式下使用 `socat`:

- DNAT 规则使用 `[]string` (无 shell 解析) 并包含 `-A OUTPUT` 链用于本地流量
- 无 root socat 转发直接针对容器桥接 IP (非 localhost)
- Veth 对通过 `Endpoint.VethHost`/`Endpoint.VethPeer` 跟踪,用于幂等拆卸
- 拆卸在移除桥接之前通过 `ip link del` 删除两个 veth 端

---

## DokiLink Mesh

DokiLink 提供无中央代理的多主机容器网络。对等体通过 mDNS (LAN)、DHT (互联网) 或静态配置相互发现。所有流量通过 Ed25519 签名认证,并使用 TLS 1.3 或 NaCl secretbox 加密。

### NAT 遍历

NAT 遍历遵循四阶段序列:

1. **STUN**: 两个对等体查询 STUN 服务器 (RFC 8489) 以发现其公共 IP 和端口映射
2. **交换**: 对等体通过 gossip 协议交换公共地址
3. **打洞**: 两个对等体使用 `TCPConn.SetDeadline` 和协调计时向彼此的公共地址发送同时 TCP SYN 数据包
4. **回退**: 如果打洞失败 (对称 NAT),流量通过充当 TURN 代理的中继对等体路由

### DHT 对等发现

带 160 位节点 ID 的 Kademlia DHT 提供去中心化对等发现:

| 参数 | 值 | 描述 |
|:----------|:------|:------------|
| 节点 ID | 160 位 | Ed25519 公钥的 SHA-1 哈希 |
| k-buckets | k=8 | 每个路由桶的最大对等体数 |
| 并行度 | alpha=3 | FIND_NODE 期间的并发查找 |
| RPC | PING、STORE、FIND_NODE、FIND_VALUE | 标准 Kademlia 操作 |

### mDNS 发现

通过 mDNS (多播 DNS) 进行 LAN 对等发现:

- 对等体通过 `_doki-link._tcp.local` TXT 记录通告
- 条目在 90 秒后过期 (如果未刷新)
- 后台清理循环每 30 秒运行一次
- 通过安装 ID 自我过滤防止自我发现
- TXT 记录通告 `common.DokiVersion` 用于版本兼容性

### 加密层

| 层 | 加密 | 描述 |
|:------|:-----------|:------------|
| **L0** | 无 | 仅环回 -- Android/Termux 上的默认值 |
| **L1** | TLS 1.3 | 默认,由每个安装的 ECDSA P-256 CA 签名 |
| **L2** | NaCl secretbox | 使用 `DOKI_LINK_PAYLOAD_ENC=1` 选择加入,密钥从两个对等体的 Ed25519 公钥派生 |

密钥派生与顺序无关 (两个对等体通过排序公钥的 SHA-256 计算相同的共享密钥)。每个连接的 nonce 从 `crypto/rand` 播种。重放保护使用 5 分钟时间戳窗口和 LRU nonce 缓存 (1024 个条目)。

### 使用

```bash
# 显示本地安装 ID 和公钥
doki mesh status

# 添加静态对等体
doki link add mybuddy 192.168.1.42:7432 \
  --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# 列出已知对等体
doki mesh ls

# 发布可通过 mesh 访问的容器
doki run -d -p 0.0.0.0:9090:80 --name web nginx:alpine
```

---

## DNS

Doki 运行内部 DNS 服务器,处理容器间名称解析并将外部查询转发到上游解析器。

### 架构

容器将 `/etc/resolv.conf` 指向 `nameserver 127.0.0.11`。Doki 内部 DNS 服务器将本地容器名称解析为桥接 IP,并将外部查询转发到上游。

### 默认值

| 平台 | 默认监听 | 原因 |
|:---------|:----------------|:----|
| Linux | `127.0.0.11:53` | 标准非特权端口 |
| Android (Termux) | `127.0.0.11:8053` | 端口 53 在非 root 上被 SELinux (EACCES) 阻止 |
| macOS | 不使用 (ModeNative) | 无桥接网络 |

### 容器名称解析

```bash
$ doki network create backend
$ doki run -d --name db --network backend postgres:alpine
$ doki run -d --name api --network backend my-api:latest
$ doki exec api sh -c 'getent hosts db'
172.20.0.2      db.backend
```

### 关键行为

- **AAAA + PTR**: IPv6 正向和反向查找与 A 记录一起工作
- **SRV 记录**: 服务发现协议支持 `_<port>._tcp.<svc>.<ns>.svc.cluster.local`
- **ndots:0**: 像 `forgejo` 这样的容器名称直接解析,无 `forgejo.local` 重试循环
- **TCP 重试**: 当上游 UDP 返回 TC 位时,根据 RFC 5966 通过 TCP 重试查询
- **无忙等待**: `ReadFromUDP` 阻塞在 socket 上,无轮询循环
- **LRU 缓存**: 1024 个条目,5 分钟 TTL,容器启动时自动注册,守护进程重启时重新注册

---

## 存储

| 驱动 | 描述 | 最适合 |
|:-------|:------------|:---------|
| **overlay2** | 内核 overlay (直接系统调用挂载) | 带 root 的 Linux,最佳性能 |
| **fuse-overlayfs** | 通过 FUSE 的用户空间 overlay | 无 root、Termux、Android |
| **btrfs** | 带快照的 Btrfs 子卷 | 带 btrfs root 的系统 |
| **zfs** | 带快照的 ZFS 数据集 | 带 ZFS 池的系统 |
| **vfs** | 简单目录复制 | 测试、最小系统 |

---

## 安全性

| 层 | 保护 |
|:------|:-----------|
| **Seccomp** | 80+ 允许的系统调用,阻止模块加载、BPF、AF_ALG、硬件 I/O |
| **AppArmor** | 每个容器基于模板的配置文件 |
| **用户命名空间** | UID/GID 重映射,root 映射到非特权用户 |
| **能力** | 最小默认集,显式授予,支持 `--cap-drop=ALL` |
| **TLS** | 带客户端证书的双向 TLS 认证 |
| **速率限制** | 令牌桶:100 请求/秒,突发 200 |
| **镜像验证** | 路径遍历保护、符号链接验证、硬链接限制 |
| **Landlock LSM** | Linux 5.13+ 无特权沙箱,通过 Landlock ABI v9 |
| **mTLS 强制** | 当配置了 `ClientCAs` 时使用 `RequireAndVerifyClientCert` |
| **常数时间比较** | TOFU 公钥验证使用 `crypto/subtle.ConstantTimeCompare` |
| **重放保护** | Gossip 消息包括随机 nonce + 5 分钟时间戳窗口,LRU nonce 缓存 |
| **OOM DoS 防护** | Gossip 监听器包装在 `io.LimitReader(MaxGossipMessageBytes+1)` 中 |

### 阻止的系统调用

```
init_module, finit_module, delete_module    # 模块加载
kexec_load, kexec_file_load                 # 内核执行
iopl, ioperm                                # 硬件 I/O
kcmp                                        # 内核信息泄漏
process_vm_readv, process_vm_writev         # 进程内存访问
```

### 允许的现代系统调用

```
io_uring_setup, io_uring_enter, io_uring_register  # 异步 I/O
pidfd_open, pidfd_send_signal, pidfd_getfd         # PID 文件描述符
rseq, userfaultfd, copy_file_range                 # 现代内核特性
landlock_create_ruleset, landlock_add_rule         # Landlock 沙箱
```

---

## 配置

### 守护进程配置 (`~/.doki/config.json`)

```json
{
  "data_dir": "/data/data/com.termux/files/usr/var/lib/doki",
  "socket": "/data/data/com.termux/files/usr/var/run/doki.sock",
  "storage_driver": "fuse-overlayfs",
  "default_network": "bridge",
  "debug": false,
  "log_level": "info",
  "rootless": true,
  "dns": {
    "listen": "127.0.0.11:8053",
    "upstream": ["8.8.8.8:53", "8.4.4.4:53"],
    "cache_capacity": 256
  },
  "mesh": {
    "enabled": true,
    "listen": ":7432",
    "stun_servers": ["stun.l.google.com:19302"],
    "enable_mdns": true,
    "payload_encryption": false
  },
  "registry_mirrors": [],
  "insecure_registries": []
}
```

### 环境变量

| 变量 | 描述 | 默认值 |
|:---------|:------------|:--------|
| `DOKI_HOST` | 守护进程 socket 路径 | 平台特定 |
| `DOKI_DATA_DIR` | 数据目录 | `~/.doki/data` |
| `DOKI_STORAGE_DRIVER` | 存储驱动 | `fuse-overlayfs` |
| `DOKI_TLS` | 启用 TLS | 未设置 |
| `DOKI_TLS_CERT` | TLS 证书路径 | 未设置 |
| `DOKI_TLS_KEY` | TLS 密钥路径 | 未设置 |
| `DOKI_KERNEL` | MicroVM 内核路径 | 平台特定 |
| `DOKI_NATIVE` | 强制原生模式 | 未设置 |
| `DOKI_DNS_LISTEN` | DNS 服务器监听地址 | `127.0.0.11:8053` (Android) / `127.0.0.11:53` (Linux) |
| `DOKI_DEBUG` | 启用调试模式 (pprof 在 `:6060`) | 未设置 |
| `DOKI_RATE_LIMIT` | 每秒请求数 | `100` |
| `DOKI_LOG_LEVEL` | 日志级别 (debug/info/warn/error) | `info` |
| `DOKI_LOG_FORMAT` | 日志格式 (json/text) | 自动检测 |
| `DOKI_LINK_MESH` | 启用 DokiLink mesh (`1`/`0`) | `1` |
| `DOKI_LINK_ADDR` | 覆盖 mesh gossip 监听地址 | `:7432` |
| `DOKI_LINK_STUN` | NAT 遍历的 STUN 服务器 (逗号分隔) | `stun.l.google.com:19302` |
| `DOKI_LINK_RELAY` | TURN 回退的中继对等体 | 未设置 |
| `DOKI_LINK_PAYLOAD_ENC` | 启用 NaCl secretbox (L2 加密) | 未设置 |
| `DOKI_USE_SOCAT` | 强制使用 socat 进行端口转发 | 未设置 |
| `DOKI_RUNTIME` | 强制特定运行器 (`proot`、`gVisor`、`native` 等) | 自动检测 |
| `DOKI_EMULATION_MODE` | 跨架构仿真器偏好 (`qemu`、`fex`、`box64`、`auto`) | `auto` |
| `DOKI_EMULATOR` | `DOKI_EMULATION_MODE` 的别名 | 未设置 |

---

## 构建

### 要求

- Go 1.22 或更高版本
- `make` (可选)
- 对于 microVM 模式:`crosvm` 或 `firecracker` 二进制 (自动检测)
- 对于 macOS VZ 后端:启用 CGO,macOS 11+ SDK

### 构建目标

```bash
# Android / Termux (ARM64)
make build-android-arm64
make install

# Android / Termux (ARMv7)
make build-android-armv7

# Linux (ARM64)
make build-linux-arm64

# Linux (ARMv7)
make build-linux-armv7

# Linux (x86_64)
make build-linux-amd64

# macOS (Apple Silicon)
make build-darwin-arm64

# macOS (Intel)
make build-darwin-amd64

# 所有平台一次
make release

# SHA256 校验和
make sha256

# 测试和 lint
make test      # go test ./...
make vet       # go vet ./...
make clean     # rm -rf releases/
```

### 手动构建

```bash
make release
# 或等效地:
go build -trimpath -ldflags="-s -w" -o releases/doki ./cmd/doki
go build -trimpath -ldflags="-s -w" -o releases/dokid ./cmd/dokid
go build -trimpath -ldflags="-s -w" -o releases/doki-compose ./cmd/doki-compose
go build -trimpath -ldflags="-s -w" -o releases/doki-init ./cmd/doki-init
go build -trimpath -ldflags="-s -w" -o releases/doki-kube ./cmd/doki-kube
go build -trimpath -ldflags="-s -w" -o releases/doki-kubectl ./cmd/doki-kubectl
```

---

## 项目结构

```
Doki/
  cmd/
    doki/                 CLI 二进制 (108 个命令,1600+ 行)
    dokid/                守护进程二进制 (REST API、TLS、速率限制)
    doki-compose/         Docker Compose 兼容 CLI
    doki-init/            容器的最小 PID 1 (Go)
    doki-init-rust/       microVM 客户机的最小 PID 1 (Rust,412K)
    doki-kube/            Kubernetes 控制平面 (多合一)
    doki-kubectl/         kubectl 兼容 CLI 客户端
    dokitest/             集成测试套件
    regtest/              仓库测试套件
  pkg/
    api/                  Docker Engine API v1.54 服务器
    podman/               Podman libpod v5 API (39 个端点)
    compose/              Compose 引擎,带 watch + publish + healthcheck
    apiserver/            Kubernetes API 服务器
    kubelet/              Kubernetes kubelet 代理 (真实 CRI 客户端)
    scheduler/            Kubernetes 调度器
    controllers/          Kubernetes 控制器 (10 个功能控制器)
    kubeproxy/            Kubernetes kube-proxy (iptables/nftables/userspace)
    coredns/              Kubernetes 的集群 DNS
    kubectl/              kubectl HTTP 客户端库
    k8s-types/            80 个 Kubernetes API 类型
    store/                内存状态存储 + SQLiteStore,带崩溃安全持久化
    runtime/              带 12 种执行模式的 OCI 运行时
    image/                OCI 镜像管理 (pull、push、build)
    registry/             OCI Distribution Spec 客户端
    network/              容器网络 (bridge、CNI、DNS、pasta)
    storage/              存储驱动 (overlay2、fuse、btrfs、zfs)
    builder/              Dokifile 解析器 (18 条指令,多阶段)
    cli/                  CLI 库 (3200+ 行)
    common/               共享类型、配置、实用程序
    netlink/              DokiLink Mesh (gossip、proxy、NAT 遍历、DHT、mDNS)
    emulation/            跨架构仿真 (QEMU/FEX/Box64 检测 + 配置)
    landlock/             Landlock LSM 沙箱 (Linux 5.13+)
    macos/                macOS 原生 VM (VZ + QEMU + Sandbox 后端)
    security/             Seccomp 和 AppArmor 配置文件
    distro/               Linux 发行版管理
    cri/                  Kubernetes CRI gRPC 服务器 (41 个 RPC)
    oci/                  OCI 规范生成
    deps/                 依赖管理 (doki deps 工具)
    scheduler/            Pod 调度
  internal/
    dokivm/               MicroVM 子系统 (crosvm、firecracker、qemu)
    namespaces/           Linux namespace 管理
    cgroups/              cgroups v2 资源管理
    fuse/                 FUSE overlay 文件系统操作
    proot/                Android 的 Proot 回退
    seccomp/              Seccomp 配置文件引擎
    apparmor/             AppArmor 配置文件生成器
  doki-os/                doki-OS VM 内核配置 + Makefile
```

---

## 兼容性

### 什么有效

| 特性 | 状态 | 说明 |
|:--------|:------:|:------|
| `doki run` | 已测试 | 基本命令、shell 脚本、--init、--user、--entrypoint、--restart |
| `doki pull` | 已测试 | ARM64 多架构自动解析、并行下载、令牌认证 |
| `doki push` | 已测试 | OCI Distribution Spec:blob 上传、跨仓库挂载、清单 PUT |
| `doki images` | 已测试 | 正确的大小、填充的 RepoDigests |
| `doki ps` / `doki ps -a` | 已测试 | 显示名称、端口、镜像 |
| `doki inspect` | 已测试 | 完整 JSON 输出 |
| `doki stop` / `doki rm` | 已测试 | 按名称或 ID,无死锁 |
| `doki build` | 已测试 | RUN 层、COPY --from、ARG、ENV、.dockerignore、构建缓存 |
| `doki logs` | 已测试 | 轮转 (10MB/3 个文件)、Docker 多路复用流格式 |
| `doki exec` | 已测试 | 通过 proot 在容器内运行 |
| `doki attach` | 已测试 | HTTP 劫持、双向流 |
| `doki wait` | 已测试 | 多容器,返回退出代码 |
| `doki login` / `doki logout` | 已测试 | 令牌认证、基本认证、凭据连接 |
| `doki network ls` | 已测试 | Bridge/host/none、doki0 桥接创建 |
| `doki volume create/ls/rm` | 已测试 | 本地驱动、tmpfs 支持 |
| `doki-compose up/down` | 已测试 | 完整 compose 规范:网络、卷、secrets、healthcheck |
| `doki cp` | 已测试 | 使用 tar 解压在主机/容器之间复制文件 |
| 端口转发 (`-p`) | 已测试 | iptables DNAT (root) 和 socat (无 root) |
| 隔离自动选择 | 已测试 | 注册表从 12 种模式中选择最佳可用运行器 |
| `--runtime` 标志 | 已测试 | 通过 `doki run --runtime proot` 显式模式 |
| Kubernetes CRI gRPC | 功能性 | 所有 35+6 RPC 在 Unix socket 上实现 |
| 带真实 CRI 的 Kubelet | 功能性 | 协调循环调用 RunPodSandbox/CreateContainer/StartContainer |
| Kube-proxy | 功能性 | iptables/nftables/userspace 模式、DNAT + MASQUERADE |
| K8s 控制器 | 功能性 | Deployment、ReplicaSet、Job、Endpoint、Service、Namespace、GC |
| SQLiteStore | 功能性 | 崩溃安全持久状态 |
| Podman API | 功能性 | 39 个端点、pod/secret/manifest 管理 |
| Compose healthcheck 执行 | 功能性 | 周期性探测、状态报告、`service_healthy` 条件 |
| DokiLink Mesh NAT 遍历 | 功能性 | STUN + TCP 打洞 + TURN 中继回退 |
| DokiLink DHT | 功能性 | Kademlia 160 位、k=8、对等发现 |
| DokiLink mDNS | 功能性 | LAN 发现,90 秒过期 + 30 秒清理 |
| macOS VZ 后端 | 功能性 | 带 cgo 桥接的 Virtualization.framework |

### 什么尚不可用

| 特性 | 状态 | 说明 |
|:--------|:------:|:------|
| MicroVM 隔离 | 未测试 | 代码存在,未在兼容硬件上测试 |
| gVisor 隔离 | 未测试 | runsc 检测有效,运行时未验证 |
| WASM 容器 | 未测试 | wasmedge/iwasm 检测有效,运行时未验证 |
| pKVM/Microdroid | 未测试 | pKVM 检测有效,无兼容硬件可测试 |
| Sysbox | 未测试 | sysbox-runc 检测有效,运行时未验证 |
| FEX-Emu 跨架构 | 未测试 | FEXInterpreter/box64 检测有效,运行时未验证 |
| QEMU 用户模式 | 未测试 | qemu-*-static 检测有效,运行时未验证 |
| Chroot 模式 | 未测试 | 原则上有效,未验证 |
| Legacy32 模式 | 未测试 | binfmt_misc 检测有效,运行时未验证 |
| CNI 网络 | 未测试 | 插件管理器存在,未连接 |
| 网络桥接隔离 | 部分 | 在 rootful 下有效 (iptables DNAT);在 proot/native 中,容器共享主机网络 |

---

## 新特性

### v0.11.0 (2026 年 6 月)

Doki 0.11 是网络和成熟度版本:完整 DokiLink Mesh,带 NAT 遍历和 DHT,macOS VZ cgo 后端,Kubernetes 100% 带真实 CRI,以及生产就绪的 Podman API。

#### DokiLink Mesh -- NAT 遍历 + DHT + mDNS

- **NAT 遍历**:STUN 客户端 (RFC 8489)、TCP 同时打开打洞和 TURN 风格中继服务器。不同网络上的对等体无需静态 IP 即可连接。
- **DHT 对等发现**:带 160 位节点 ID 的 Kademlia DHT、k-buckets (k=8)、alpha=3 并行查找。无需静态配置或 mDNS 的去中心化路由。
- **mDNS 90 秒过期**:条目在 90 秒后过期 (如果未刷新),清理循环每 30 秒运行一次。
- **加密修复**:与顺序无关的密钥派生 (两个对等体派生相同的共享密钥)。每个连接的 nonce 来自 `crypto/rand`。带 5 分钟时间戳窗口和 LRU nonce 缓存的重放保护。`secretboxStreamConn.Close()` 使用 `atomic.Bool` 防止双关闭竞争。
- **Mesh 加固**:`Stop()` 关闭 `stopCh` 以向所有循环发信号。Gossip 解码器包装在 `io.LimitReader` 中 (OOM DoS 防护)。mDNS TXT 记录通告 `common.DokiVersion`。
- **安全性**:TrustStore、SecretManager 和 ManifestManager 中的路径遍历验证。TOFU 公钥验证通过 `crypto/subtle.ConstantTimeCompare` 进行常数时间比较。使用 `RequireAndVerifyClientCert` 强制 mTLS。

#### macOS 原生虚拟化

- **带 cgo 的 VZ 后端**:到 Virtualization.framework 的 Objective-C 桥接 (`VZVirtualMachineConfiguration`、`VZLinuxBootLoader`、`VZVirtioFileSystemDevice`、`VZBridgedNetworkDevice`/`VZNATNetworkDevice`、`VZRosettaPlatform`)。构建标签 `darwin && cgo`。
- **QEMU 后端修复**:`sync.RWMutex` 用于线程安全、二进制验证、架构感知参数、SIGTERM/SIGKILL 超时。
- **Sandbox 后端**:收紧的配置文件,作用域 process-exec 和 mach-lookup。
- **构建标签兼容性**:包在所有平台上编译 (darwin+cgo、darwin!cgo、!darwin)。

#### Kubernetes 100%

- **CRI gRPC 服务器** (`pkg/cri/server.go`):真实 gRPC CRI,在 Unix socket 上实现所有 35 个 RuntimeServiceServer + 6 个 ImageServiceServer RPC。
- **带真实 CRI 的 Kubelet**:`NewKubeletWithCRI` 拨号 CRI socket,调用 `RunPodSandbox` / `CreateContainer` / `StartContainer`,获取真实 PodIP、容器状态和镜像摘要。
- **真实 Kube-proxy**:带 DNAT/MASQUERADE 的 iptables 链、nftables 规则集生成、用户空间 TCP/UDP 轮询代理 (无需 root 即可工作)。
- **功能性控制器** (`pkg/controllers/manager.go`):DeploymentController、ReplicaSetController、JobController (并行度/完成数/退避)、EndpointController、ServiceController (ClusterIP 分配)、NamespaceController (级联删除)、GarbageCollector (OwnerReferences)。
- **完整 API 服务器**:API 组路径 (`networking.k8s.io/v1`、`rbac.authorization.k8s.io/v1`)、PATCH (merge-patch + strategic-merge)、Watch (K8s 事件格式)。
- **SQLiteStore** (`pkg/store/sqlite.go`):通过 SQLite 实现崩溃安全持久化的持久存储。
- **真实调度器**:忙等待替换为阻塞睡眠、镜像局部性评分、最少请求评分。
- **真实 CoreDNS**:UDP 缓冲区竞争修复、SRV 记录支持、对无法解析的查询返回 NXDOMAIN。

#### Podman 连接

- Podman shim 挂载在 dokid 的 `/libpod/*` 上,在同一服务器上,继承 TLS、中间件和速率限制。
- 系统信息使用 `runtime.GOARCH`、`runtime.GOOS`、检测到的内核/内存、`common.DokiVersion`。
- 容器生命周期委托给 PodManager (start/stop/kill/restart/pause/unpause)。
- 容器调度在找不到时返回 404,DELETE 返回 204。

#### Compose Healthcheck 执行

- `HealthChecker` (`pkg/runtime/healthcheck.go`) 运行周期性探测 (CMD/CMD-SHELL/NONE),尊重 Interval/Timeout/Retries/StartPeriod/StartInterval。
- 更新 `state.HealthStatus.Status` (`starting` -> `healthy`/`unhealthy`)。
- Compose `service_healthy` 条件端到端工作。

#### 诊断

- `doki deps` 工具,带 `ls` (列出系统依赖)、`check` (CI 门控)、`go` (列出 Go 依赖)、`install <name>` (通过检测到的包管理器尽力安装)。

### v0.11.1 (2026 年 6 月)

错误修复和增量特性版本。

#### 跨架构仿真 (新)

- **`pkg/emulation/config.go`** (198 行):QEMU 用户模式、FEX-Emu 和 Box64 检测,持久化配置在 `~/.doki/emulation.json` (原子写入,0600 权限)。
- **`doki emu {show,detect,set,test}`** -- 4 个新的 CLI 子命令用于仿真器管理。
- **`DOKI_EMULATION_MODE`** / **`DOKI_EMULATOR`** 环境变量覆盖保存的偏好。
- `emulation.PreferredMode()`、`NormalizeMode()`、`Detect()`、`SelectBest()` 公共 API。外部架构镜像由运行器注册表自动路由通过选定的仿真器。
- 4 个单元测试 (`TestNormalizeMode`、`TestSaveLoadPreferredMode`、`TestPreferredModeEnvWins`、`TestSelectBest`)。

#### 运行器注册表重构

- **`pkg/runtime/registry.go`**:`BestFor()` 算法重写,带 `DOKI_RUNTIME` 环境变量支持、`requestedRuntime()`、`runnerUsableOnHost()` 和 `preferredEmulationRunner()` 用于跨架构路由。
- 5 个新测试 (`TestRegistryBestForUsesEnvRuntime`、`TestRegistryBestForChoosesHighestUsableLevel`、`TestRegistryBestForSkipsUnavailableHostRequirements`、`TestRegistryBestForCrossArchPrefersEmulation`、`TestRegistryBestForUsesQEMUPreference`)。

#### 守护进程

- **`--host` 标志**,带 Docker 风格寻址:`unix:///path`、`tcp://addr:port`、裸路径。`applyDaemonHost()` 解析。支持 `DOKI_HOST` 和 `DOCKER_HOST` 环境变量。
- **`cmd/dokid/main_test.go`**:`TestApplyDaemonHost` 覆盖三个解析路径。

#### 错误修复 -- Issue #5:Termux 上的无 Root 网络

- **chown 警告** (`pkg/runtime/runtime.go`):`logChownError()` 在无 root 模式下发出单个 INFO 消息,而非数百行 WARN。
- **pasta 回退** (`pkg/network/manager.go`):`setupRootlessNetworking()` 现在尝试 `pasta` -> `slirp4netns` -> 通过 proot 的主机 netns (之前在找不到 pasta 时崩溃)。
- **客户端退出代码** (`cmd/doki/main.go`):`dispatch()` 子命令错误处理使用 `handleError()`。`ExitError{Code: 0}` 不再打印 "Error:"。
- **Termux 特定 UX**:`termuxNetworkHint()` 防止误导性的 "pkg install passt" 建议。回退 INFO 消息解释 `/dev/net/tun` + `CAP_NET_ADMIN` 限制。
- **1 个回归测试**:`TestSetupRootlessNetworking_Fallback`。

#### 文档

- README 恢复到 v0.10.0 详细级别 (1243 行)。
- 22 个 wiki 页面以简洁风格重写 (零表情符号、零 SVG、零 ASCII 框图)。
- 域名:`doki.opceanai.com` -> `dok1.xyz`。

#### 安全修复

- `emulation.json` 以 0600 权限和原子写入存储。
- `logChownError()` 防止来自 OCI 文件路径的日志注入。

### v0.10.0

Doki 0.10 是一次大规模扩展:**Podman 1:1 API 兼容性、完整 Kubernetes 发行版、macOS 原生 VM 支持、doki-OS VM 镜像,以及 20 个新依赖**,使引擎达到 55,000+ 行代码,跨 158 个文件。

#### 新平台和 API

| 特性 | 描述 |
|:--------|:------------|
| **Podman API v5** | 39 个端点兼容 `podman-remote` 客户端。Pod、secret 和 manifest 管理 |
| **Kubernetes 1.32** | 完整控制平面:apiserver、kubelet、scheduler、controllers (10)、kube-proxy、CoreDNS |
| **macOS 原生** | VZ (Virtualization.framework) 和 QEMU 后端,适用于 Apple Silicon 和 Intel Mac |
| **doki-OS** | 用于容器优化 VM 客户机的最小 Linux 内核配置 (~4MB bzImage) |
| **Landlock LSM** | Linux 5.13+ 无特权沙箱,通过 Landlock ABI v9 |

#### 新二进制

| 二进制 | 描述 |
|:-------|:------------|
| `doki-kube` | 多合一 Kubernetes 控制平面 |
| `doki-kubectl` | kubectl 兼容 CLI (get、apply、delete、describe、logs) |

#### 质量

- **staticcheck**:0 警告
- **errcheck 生产**:0 未检查错误
- **go vet**:0 警告

### v0.9.3

此版本发布了 **DokiLink-Lite** (mesh 网络) 和 **190+ 错误修复**,跨 4 轮全面审计。

#### DokiLink-Lite (Mesh 网络)

TCP/UDP 代理 + mesh 层,让你可以转发容器的发布端口到另一个 Doki 实例。纯 Go 标准库 + `crypto/tls` + `golang.org/x/crypto/nacl`。

| 特性 | 描述 |
|:--------|:------------|
| TCP/UDP 代理 | 半关闭、空闲超时、传输包装器 |
| TLS 1.3 (L1) | 默认加密,每个安装 ECDSA P-256 CA |
| NaCl secretbox (L2) | 通过 `DOKI_LINK_PAYLOAD_ENC=1` 选择加入,Ed25519 派生密钥 |
| 安装身份 | Ed25519 密钥对 + ECDSA CA 在 `$DOKI_ROOT/keys/` |
| TOFU 信任 | 首次接触时记录公钥,重连时验证 |
| 静态对等体 | `$DOKI_ROOT/mesh/peers.json` 通过 `doki link add/rm` |
| mDNS (选择加入) | 使用 `-tags netlink_mdns` 构建,仅 LAN 发现 |
| Gossip | 带签名的 JSON 消息通过 TCP,15 秒对等发现 tick |

#### 关键错误修复

- kill 未更新状态 -- 使用 `signal(0)` 轮询然后保存已退出状态
- stop 在 SIGKILL 失败时未更新状态 -- 始终保存退出代码 137
- Exec 无输出 -- 返回 stdout/stderr 字节
- 命令后的 Compose 标志 -- 解析器在子命令后继续
- 容器列表 Name 始终为空 -- `stateToInfo` 设置 `info.Name`
- Create 忽略 `?name=` -- 读取 URL 查询参数
- 镜像检查 Config -- PascalCase 转换
- `ps --format` -- 模板执行

### v0.9.2

此版本是对 v0.9.1 的 **稳定性 + 正确性** 传递。

#### 关键网络修复

1. **iptables DNAT 缺少 `-A` 标志** -- `-A OUTPUT` 缺失,导致 "Unknown option" 错误
2. **端口转发到 localhost** -- socat 现在针对容器桥接 IP
3. **孤立的 veth 对** -- `Endpoint` 跟踪 `VethHost`/`VethPeer`,拆卸删除两端
4. **新主机上缺少 proot** -- `FindProotBinary()` 回退到系统 PATH

#### DNS 服务器重写 (修复 18 个错误)

LRU 缓存 (1024 个条目,5 分钟 TTL)、AAAA + PTR 支持、ndots:0、TC 位时的 TCP 重试、通过 `getprop net.dns*` 的 Android 上游、端口剥离、自动注册、守护进程重启时恢复。

#### Termux 的 LD_PRELOAD 修复

`libtermux-exec-ld-preload.so` 破坏了 proot 的基于 ptrace 的系统调用转发。修复:`StripHostEnv()` 从容器环境中剥离 `LD_PRELOAD` 和 `LD_LIBRARY_PATH`。

### v0.9.1

- **OCI 推送**:`doki push` -- blob 上传、跨仓库挂载、清单 PUT
- **仓库认证**:`doki login` 接受凭据并传播到仓库客户端
- **原生 tar 解压**:Go 原生 tar,带 whiteout、路径遍历保护、压缩自动检测、并行解压和回滚
- **4 个新发行版**:Fedora、Gentoo、OpenSUSE、Rocky Linux -- 共 8 个发行版
- **改进的 Compose 引擎**:长语法 Ports/Volumes、带 60 秒轮询的 `depends_on` 健康条件、30+ 新字段
- **19 个 Proot C 修复**:SECCOMP_RET_ALLOW、fake_id0 大括号错误、stat.c uid/gid 修复、link2symlink UB 等
- **Overlay2 内核挂载**:直接使用 `syscall.Mount("overlay")` 而非 FUSE 委托
- **通过 HTTP 劫持附加**:`doki attach` 带双向流
- **DNS 监听器**:内部 DNS 服务器在端口 53 上用于容器间解析
- **ARMv7 beta**:为旧 ARM 设备编译和二进制

### v0.9.0

- **doki-init-rust**:PID 1 用 Rust 重写 (412K vs 2.9MB Go,-86%)
- **doki-proot**:Forked proot,带守护进程模式 + JSON IPC 协议。14K 二进制
- **发行版系统**:`doki run --distro alpine/ubuntu/debian/arch` 从 Docker Hub 下载
- **ARMv7 beta**:旧 ARM 设备的完整特性对等
- **Immich**:完整堆栈运行 (PostgreSQL 18 + pgvector + cube + earthdistance、Redis 7、Immich Server v2.7.5)

---

## 贡献

欢迎贡献。最需要帮助的领域:

| 领域 | 描述 |
|:-----|:------------|
| **MicroVM 后端** | 支持额外的 hypervisor 和平台 |
| **CNI 插件** | 实现高级网络特性 |
| **安全性** | 加固、模糊测试和渗透测试 |
| **性能** | 层缓存、并行操作、内存优化 |
| **测试** | 集成测试、端到端测试、压力测试 |
| **文档** | 教程、示例和 API 参考 |

### 开发设置

```bash
git clone https://github.com/OpceanAI/Doki.git
cd Doki
go build ./...
go test ./...
```

### 提交风格

- 使用祈使语气 ("Add feature" 而非 "Added feature")
- 保持第一行在 72 个字符以内
- 适用时引用 issue

---

## 许可证

Doki 本身是 Apache 2.0。DokiOS 在其各自许可证下捆绑第三方组件。

### Doki

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

### 捆绑组件

| 组件 | 许可证 | SPDX | 说明 |
|:----------|:--------|:-----|:------|
| **Doki** | Apache 2.0 | `Apache-2.0` | 根据 Apache 2.0 许可。具有明确的专利保护的高度商业化和宽松的许可证 |
| **cloudflared** | Apache 2.0 | `Apache-2.0` | Cloudflare 隧道。授予与 Doki 相同的商业自由和专利保护 |
| **fastfetch** | MIT | `MIT` | 非常简短、简单的开源许可证 -- 几乎可以对代码做任何事 |
| **OpenSSH** | BSD 风格 | `SSH-OpenSSH` | OpenSSH 许可证。高度宽松,历史上针对安全和自由分发进行了优化 |
| **zsh** | MIT / BSD | `MIT` 或 `BSD-2-Clause` | 宽松的 MIT/BSD 风格许可证,使 shell 环境免受严格的 copyleft |
| **bash** | GPL-3.0 | `GPL-3.0-only` | GNU GPLv3。集合中最 legally 限制的工具:任何衍生作品必须共享源代码,具有强大的反 tivoization 条款 |

---

## 链接

| 平台 | 仓库 | 真实来源 |
|:---------|:-----------|:----------------|
| GitHub | [OpceanAI/Doki](https://github.com/OpceanAI/Doki) | 是 (主要) |
| GitLab | [aguitauwu/doki](https://gitlab.com/aguitauwu/doki) | 镜像 |
| Codeberg | [aguitauwu/Doki](https://codeberg.org/aguitauwu/Doki) | 镜像 |
| Aguita | [katu/doki](https://git.aguita.site/katu/doki) | 镜像 |
| 网站 | [dok1.xyz](https://dok1.xyz) | 文档 / 安装脚本 |
| 英文 README | [README.md](README.md) | 原始版本 |

> Main 是唯一的真实来源。镜像在每个版本后从 `main` 强制同步。如果你发现分歧,请在 GitHub 上开 issue。

### Wikis

| 平台 | Wiki |
|:---------|:-----|
| GitHub | [OpceanAI/Doki/wiki](https://github.com/OpceanAI/Doki/wiki) |
| GitLab | [aguitauwu/doki/-/wikis](https://gitlab.com/aguitauwu/doki/-/wikis/home) |
| Codeberg | [aguitauwu/Doki/wiki](https://codeberg.org/aguitauwu/Doki/wiki) |

### 相关

| 仓库 | 描述 |
|:-----------|:------------|
| [Doki-proot](https://github.com/OpceanAI/Doki-proot) | 带 JSON IPC 守护进程模式的 Forked proot,用于 Doki |
