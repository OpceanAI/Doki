# Storage

<sub>[DRIVERS / LAYERS / VOLUMES / v0.11.0]</sub>

> Five storage drivers, auto-detected by `DetectBestDriver()`. Selection
> is a function of kernel, filesystem, and whether the caller holds
> root. There is no "best" driver in the abstract; there is only the
> driver the host can actually mount.

<hr>

## Driver Matrix

<sub>[COMPARISON / ROOT / PERFORMANCE / PLATFORM]</sub>

```text
DRIVER          ROOT     PERFORMANCE              PLATFORM             STATUS
----------      ----     -----------              --------             ------
overlay2        yes      best (kernel native)     Linux                tested
fuse-overlayfs  no       ~90% of overlay2         Linux/Android/Termux tested
btrfs           no*      best (with snapshots)    Linux (btrfs root)   untested
zfs             no*      best (with snapshots)    Linux (zfs pool)     untested
vfs             no       slowest (copy on read)   any (macOS fallback) tested

* subvolumes / datasets; mount itself does not need root
```

<hr>

## Auto-Detection

<sub>[DETECTBESTDRIVER / PKG/STORAGE/DRIVER.GO]</sub>

```go
// pkg/storage/driver.go
func DetectBestDriver(root string) string {
    if canUseOverlay2() {  // modprobe overlay OR /proc/filesystems has overlay
        return DriverOverlay2
    }
    if isBtrfs(root) {     // stat -f -c %T returns btrfs
        return "btrfs"
    }
    if _, err := exec.LookPath("zfs"); err == nil {
        return "zfs"
    }
    return "fuse-overlayfs"  // always works
}
```

```text
HOST CONDITION              DRIVER
-----------------------     ---------------
Linux + root + overlay      overlay2
Termux / Android            fuse-overlayfs
macOS                       vfs
Btrfs root                  btrfs
ZFS pool present            zfs
```

Override the probe with `DOKI_STORAGE_DRIVER=<driver>` or
`storage_driver` in `config.json`. The probe is advisory, not
authoritative.

<hr>

## Content-Addressable Store

<sub>[CAS / SHA256 / DEDUP]</sub>

Every layer is stored by SHA256 digest. Two images that share a base
layer store that layer exactly once. Deduplication is a side effect of
addressing, not a separate pass.

```text
~/.doki/data/
  layers/           one dir per layer SHA (sha256:abc.../, sha256:def.../, ...)
  merged/           mount points (overlay2)
  diff/             upper dirs (overlay2)
  work/             work dirs (overlay2)
  images/           image metadata
  containers/       container state (one dir per container ID)
    <id>/
      state.json    container status, PID, config
      rootfs/       extracted rootfs
      logs/         container log output
  volumes/          named volumes
```

<hr>

## Layer Extraction

<sub>[PULL PIPELINE / PKG/REGISTRY + PKG/STORAGE/LAYER.GO]</sub>

Sequence executed by `doki pull alpine`:

```text
1. FETCH MANIFEST    pkg/registry -> GET /v2/alpine/manifests/latest
2. PARSE CONFIG      extract layer list + image config
3. DOWNLOAD LAYERS   4 concurrent, Range support for resumption
4. VERIFY CHECKSUMS  SHA256 each blob after download
5. STORE IN CAS      data/layers/sha256:<digest>/
6. EXTRACT ON DEMAND layers stacked into data/containers/<id>/rootfs/
```

Extraction is Go-native. The `tar` binary is not invoked.

```text
CHECK                       ENFORCEMENT
-----------------------     --------------------------------
compression                 gzip / bzip2 / xz / zstd (auto)
path traversal              rejects ".." and absolute paths
symlink validation          rejects targets outside rootfs
hardlink restriction        hardlinks only within same layer
whiteout                    ".wh." prefix removes lower files
rollback                    parallel extract aborts on error
```

<hr>

## overlay2

<sub>[KERNEL OVERLAYFS / FASTEST]</sub>

Fastest driver. Uses the Linux kernel `overlayfs` mount syscall.

```text
REQUIREMENT              VALUE
---------------------    ----------------------------
kernel                   3.18+ (4.0+ recommended)
kernel config            CONFIG_OVERLAY_FS=y
privilege                root (for the mount syscall)
backing fs               must support xattrs
quota fs                 btrfs / XFS / ZFS / ext4 (proj)
```

```go
opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
syscall.Mount("overlay", mergeDir, "overlay", 0, opts)
```

Some FUSE filesystems do not expose extended attributes and cannot back
overlay2. Quotas are per-container and driver-specific.

<hr>

## fuse-overlayfs

<sub>[ROOTLESS / USERSPACE OVERLAY]</sub>

Userspace overlay via FUSE. The rootless alternative when the mount
syscall is unavailable.

```text
REQUIREMENT              VALUE
---------------------    ----------------------------
binary                   fuse-overlayfs in $PATH
module                   FUSE kernel module or fusermount
overhead                 ~90% of overlay2 (metadata-bound)
data path                near-native
```

```bash
# Termux default.
$ pkg install fuse-overlayfs
$ doki run --rm alpine echo hello
```

<hr>

## btrfs

<sub>[SUBVOLUMES / COW SNAPSHOTS]</sub>

Uses Btrfs subvolumes and snapshots.

```text
REQUIREMENT              VALUE
---------------------    ----------------------------
root fs                  btrfs (or a btrfs subvolume at data root)
tools                    btrfs CLI
snapshot                 instant (CoW)
quota                    btrfs qgroup limit (native)
backup                   btrfs send / receive
```

```bash
# Create a subvolume for Doki.
btrfs subvolume create /var/lib/doki
# Configure Doki.
echo '{"storage_driver": "btrfs"}' > /etc/doki/config.json
```

<hr>

## zfs

<sub>[DATASETS / SNAPSHOTS / ENCRYPTION]</sub>

Uses ZFS datasets and snapshots.

```text
REQUIREMENT              VALUE
---------------------    ----------------------------
pool                     ZFS pool mounted
tools                    zfs CLI
linux package            zfs-dkms or zfsutils-linux
snapshot                 datasets + clones
encryption               native
compression              lz4 / zstd
backup                   zfs send / receive
```

```bash
# Create a dataset for Doki.
zfs create -o mountpoint=/var/lib/doki tank/doki
# Configure Doki.
echo '{"storage_driver": "zfs"}' > /etc/doki/config.json
```

<hr>

## vfs

<sub>[FALLBACK / COPY ON READ]</sub>

Directory copy. No overlay. No snapshots. No CoW.

```text
PROPERTY              VALUE
------------------    --------------------------------
use case              testing / macOS / no-overlay host
start time            proportional to image size
storage cost          higher than overlay (full copy)
per-container         own complete copy of image
```

Each `doki run` copies the entire image. This is the worst driver by
every measurable axis. It exists so that Doki runs anywhere.

<hr>

## Volumes

<sub>[NAMED / ANONYMOUS / PERSISTENCE]</sub>

Named volumes are stored separately from the container rootfs.

```bash
$ doki volume create db-data
db-data
$ doki run -d -v db-data:/var/lib/postgresql/data postgres:alpine
```

Volume data survives container removal. Anonymous volumes (declared via
`VOLUME` in a Dockerfile) are removed with the container unless `-v` is
passed to `doki rm`.

<hr>

## Volume Drivers

<sub>[LOCAL / TMPFS / NFS]</sub>

```text
DRIVER    BACKING                             NOTES
------    -------                             ----
local     data/volumes/<name>/                default
tmpfs     RAM-backed                          Linux only
nfs       NFS mount                           requires nfs-utils
```

### tmpfs

```bash
$ doki run -d --tmpfs /tmp:size=64m,mode=1777 my-image:latest
$ doki run -d --mount type=tmpfs,destination=/tmp,tmpfs-size=67108864 my-image:latest
```

### nfs

```bash
$ doki volume create --driver local \
  --opt type=nfs \
  --opt o=addr=10.0.0.1,rw \
  --opt device=:/path/to/export \
  nfs-vol

$ doki run -d -v nfs-vol:/data my-image:latest
```

<hr>

## Image Cache

<sub>[SYSTEM DF / PRUNE]</sub>

Pulled images are cached in the content-addressable store. Reclaim
space with the pruning commands.

```bash
# Show disk usage.
$ doki system df
TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          5         3         1.2 GB    800 MB (66%)
Containers      10        2         50 MB     40 MB (80%)
Local Volumes   4         2         200 MB    100 MB (50%)
Build Cache     0         0         0 B       0 B

# Prune unused.
$ doki image prune -a
$ doki container prune
$ doki volume prune
$ doki system prune -a --volumes
```

<hr>

## Build Cache

<sub>[LAYER CACHE / INVALIDATION]</sub>

`doki build` keys a layer cache by Dokifile instruction. Each `RUN`,
`COPY`, `ADD` instruction produces a cached layer.

```text
INSTRUCTION    CACHE HIT WHEN
----------     -----------------------------------------------
RUN            command string is byte-identical
COPY           source file checksums match
ADD            URL + checksum identical (URLs are re-fetched)
ENV/ARG/LABEL  any change invalidates dependent downstream layers
```

Override with `--no-cache`. Inspect with `doki build --progress=plain`.

<hr>

## Quotas

<sub>[PER-CONTAINER / BACKING FS]</sub>

```text
FS       MECHANISM
-----    -----------------------------
btrfs    btrfs qgroup limit
XFS      xfs_quota (project quotas)
ZFS      zfs set quota
ext4     project quota
```

```bash
doki run -d --storage-opt size=10G my-image:latest
```

The flag is driver-specific. A backing filesystem without quota support
silently ignores it.

<hr>

## Backup & Migration

<sub>[SAVE / LOAD / EXPORT / IMPORT / COMMIT]</sub>

### Image

```bash
$ doki save -o myapp.tar myapp:1.0
$ doki load -i myapp.tar
```

### Container filesystem

```bash
$ doki export web > web.tar
$ doki import web.tar
```

### Container snapshot (btrfs / ZFS only)

```bash
$ doki commit web myapp:snapshot
```

Full-state backup including volumes is planned (`doki system backup`).

<hr>

## Source

<sub>[PKG/STORAGE/*]</sub>

```text
FILE                              ROLE
------------------------------    -----------------------------------
pkg/storage/driver.go             entry point, driver detection
pkg/storage/drivers.go            btrfs, zfs, vfs implementations
pkg/storage/overlay.go            overlay2
pkg/storage/fuse.go               fuse-overlayfs
pkg/storage/layer.go              layer extraction, CAS
pkg/storage/volume.go             volume management
pkg/storage/cache.go              image cache, build cache
pkg/storage/mount.go              Linux mount helpers
pkg/storage/mount_darwin.go       macOS mount shim (v0.9.3)
```
