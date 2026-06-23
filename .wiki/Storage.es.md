# Storage

<sub>[DRIVERS / CAPAS / VOLUMENES / v0.11.0]</sub>

> Cinco drivers de storage, auto-detectados por `DetectBestDriver()`.
> La seleccion es funcion de kernel, filesystem y si el caller tiene
> root. No existe un driver "mejor" en abstracto; solo existe el driver
> que el host puede montar realmente.

<hr>

## Matriz de Drivers

<sub>[COMPARACION / ROOT / RENDIMIENTO / PLATAFORMA]</sub>

```text
DRIVER          ROOT     RENDIMIENTO             PLATAFORMA           ESTADO
----------      ----     -----------             --------             ------
overlay2        si       mejor (kernel nativo)   Linux               testeado
fuse-overlayfs  no       ~90% de overlay2        Linux/Android/Termux testeado
btrfs           no*      mejor (con snapshots)   Linux (root btrfs)  no testeado
zfs             no*      mejor (con snapshots)   Linux (pool zfs)    no testeado
vfs             no       el mas lento (copia on read)  cualquiera (fallback macOS)  testeado

* subvolumenes / datasets; el mount en si no necesita root
```

<hr>

## Auto-Deteccion

<sub>[DETECTBESTDRIVER / PKG/STORAGE/DRIVER.GO]</sub>

```go
// pkg/storage/driver.go
func DetectBestDriver(root string) string {
    if canUseOverlay2() {  // modprobe overlay OR /proc/filesystems tiene overlay
        return DriverOverlay2
    }
    if isBtrfs(root) {     // stat -f -c %T devuelve btrfs
        return "btrfs"
    }
    if _, err := exec.LookPath("zfs"); err == nil {
        return "zfs"
    }
    return "fuse-overlayfs"  // siempre funciona
}
```

```text
CONDICION DEL HOST          DRIVER
-----------------------     ---------------
Linux + root + overlay      overlay2
Termux / Android            fuse-overlayfs
macOS                       vfs
Root Btrfs                  btrfs
Pool ZFS presente           zfs
```

Sobrescribe el probe con `DOKI_STORAGE_DRIVER=<driver>` o
`storage_driver` en `config.json`. El probe es advisory, no
autoritativo.

<hr>

## Store Content-Addressable

<sub>[CAS / SHA256 / DEDUP]</sub>

Cada capa se almacena por digest SHA256. Dos imagenes que comparten una
capa base almacenan esa capa exactamente una vez. La deduplicacion es
un efecto lateral del direccionamiento, no un paso separado.

```text
~/.doki/
└── data/
    ├── layers/                       un dir por SHA de capa
    │   ├── sha256:abc.../
    │   ├── sha256:def.../
    │   └── sha256:ghi.../
    ├── merged/                       puntos de mount  (overlay2)
    ├── diff/                         dirs upper       (overlay2)
    ├── work/                         dirs de work     (overlay2)
    ├── images/                       metadata de imagenes
    ├── containers/                   estado de contenedores
    │   └── <id>/
    │       ├── state.json
    │       ├── rootfs/               rootfs extraido
    │       └── logs/
    └── volumes/                      volumenes con nombre
```

<hr>

## Extraccion de Capas

<sub>[PIPELINE DE PULL / PKG/REGISTRY + PKG/STORAGE/LAYER.GO]</sub>

Secuencia ejecutada por `doki pull alpine`:

```text
1. FETCH MANIFEST    pkg/registry -> GET /v2/alpine/manifests/latest
2. PARSEA CONFIG     extrae lista de capas + config de imagen
3. DESCARGA CAPAS    4 concurrentes, soporte Range para resumption
4. VERIFICA CHECKSUMS  SHA256 de cada blob despues de descarga
5. ALMACENA EN CAS   data/layers/sha256:<digest>/
6. EXTRAE BAJO DEMANDA  capas apiladas en data/containers/<id>/rootfs/
```

La extraccion es nativa en Go. El binario `tar` no se invoca.

```text
CHECK                       ENFORCEMENT
-----------------------     --------------------------------
compresion                  gzip / bzip2 / xz / zstd (auto)
path traversal              rechaza ".." y paths absolutos
validacion de symlinks      rechaza targets fuera del rootfs
resticcion de hardlinks     hardlinks solo dentro de la misma capa
whiteout                    prefijo ".wh." elimina archivos inferiores
rollback                    extraccion paralela aborta en error
```

<hr>

## overlay2

<sub>[KERNEL OVERLAYFS / EL MAS RAPIDO]</sub>

El driver mas rapido. Usa el syscall de mount `overlayfs` del kernel
Linux.

```text
REQUISITO                VALOR
---------------------    ----------------------------
kernel                   3.18+ (4.0+ recomendado)
config kernel            CONFIG_OVERLAY_FS=y
privilegio               root (para el syscall mount)
fs subyacente            debe soportar xattrs
fs de cuota              btrfs / XFS / ZFS / ext4 (proj)
```

```go
opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerDir, upperDir, workDir)
syscall.Mount("overlay", mergeDir, "overlay", 0, opts)
```

Algunos filesystems FUSE no exponen extended attributes y no pueden
respaldar overlay2. Las cuotas son por contenedor y especificas del
driver.

<hr>

## fuse-overlayfs

<sub>[ROOTLESS / OVERLAY EN USERSPACE]</sub>

Overlay en userspace via FUSE. La alternativa rootless cuando el
syscall de mount no esta disponible.

```text
REQUISITO                VALOR
---------------------    ----------------------------
binario                  fuse-overlayfs en $PATH
modulo                   modulo kernel FUSE o fusermount
overhead                 ~90% de overlay2 (limitado por metadata)
path de datos            casi nativo
```

```bash
# Default en Termux.
$ pkg install fuse-overlayfs
$ doki run --rm alpine echo hola
```

<hr>

## btrfs

<sub>[SUBVOLUMENES / COW SNAPSHOTS]</sub>

Usa subvolumenes y snapshots de Btrfs.

```text
REQUISITO                VALOR
---------------------    ----------------------------
fs root                  btrfs (o un subvolumen btrfs en el data root)
tools                    CLI de btrfs
snapshot                 instantaneo (CoW)
cuota                    btrfs qgroup limit (nativo)
backup                   btrfs send / receive
```

```bash
# Crea un subvolumen para Doki.
btrfs subvolume create /var/lib/doki
# Configura Doki.
echo '{"storage_driver": "btrfs"}' > /etc/doki/config.json
```

<hr>

## zfs

<sub>[DATASETS / SNAPSHOTS / ENCRYPTACION]</sub>

Usa datasets y snapshots de ZFS.

```text
REQUISITO                VALOR
---------------------    ----------------------------
pool                     pool ZFS montado
tools                    CLI de zfs
paquete linux            zfs-dkms o zfsutils-linux
snapshot                 datasets + clones
encryptacion             nativa
compresion               lz4 / zstd
backup                   zfs send / receive
```

```bash
# Crea un dataset para Doki.
zfs create -o mountpoint=/var/lib/doki tank/doki
# Configura Doki.
echo '{"storage_driver": "zfs"}' > /etc/doki/config.json
```

<hr>

## vfs

<sub>[FALLBACK / COPIA ON READ]</sub>

Copia de directorios. Sin overlay. Sin snapshots. Sin CoW.

```text
PROPIEDAD              VALOR
------------------    --------------------------------
caso de uso            testing / macOS / host sin overlay
tiempo de start        proporcional al tamano de imagen
costo de storage       mayor que overlay (copia completa)
por contenedor         copia completa propia de la imagen
```

Cada `doki run` copia la imagen completa. Es el peor driver por cada
eje medible. Existe para que Doki corra en cualquier lado.

<hr>

## Volumenes

<sub>[NAMED / ANONIMOS / PERSISTENCIA]</sub>

Los volumenes con nombre se almacenan separados del rootfs del
contenedor.

```bash
$ doki volume create db-data
db-data
$ doki run -d -v db-data:/var/lib/postgresql/data postgres:alpine
```

Los datos del volumen sobreviven a la eliminacion del contenedor. Los
volumenes anonimos (declarados via `VOLUME` en un Dockerfile) se
eliminan con el contenedor a menos que se pase `-v` a `doki rm`.

<hr>

## Drivers de Volumen

<sub>[LOCAL / TMPFS / NFS]</sub>

```text
DRIVER    RESPALDO                           NOTAS
------    -------                            ----
local     data/volumes/<name>/               default
tmpfs     respaldado por RAM                 solo Linux
nfs       mount NFS                          requiere nfs-utils
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

## Cache de Imagenes

<sub>[SYSTEM DF / PRUNE]</sub>

Las imagenes pulleadas se cachean en el store content-addressable.
Libera espacio con los comandos de prune.

```bash
# Muestra uso de disco.
$ doki system df
TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          5         3         1.2 GB    800 MB (66%)
Containers      10        2         50 MB     40 MB (80%)
Local Volumes   4         2         200 MB    100 MB (50%)
Build Cache     0         0         0 B       0 B

# Prunea lo no usado.
$ doki image prune -a
$ doki container prune
$ doki volume prune
$ doki system prune -a --volumes
```

<hr>

## Cache de Build

<sub>[CACHE DE CAPAS / INVALIDACION]</sub>

`doki build` keyea una cache de capas por instruccion del Dokifile.
Cada instruccion `RUN`, `COPY`, `ADD` produce una capa cacheada.

```text
INSTRUCCION    CACHE HIT CUANDO
----------     -----------------------------------------------
RUN            el string del comando es byte-identico
COPY           los checksums del archivo fuente coinciden
ADD            URL + checksum identicos (URLs se re-fetchean)
ENV/ARG/LABEL  cualquier cambio invalida capas dependientes downstream
```

Sobrescribe con `--no-cache`. Inspecciona con `doki build --progress=plain`.

<hr>

## Cuotas

<sub>[POR CONTENEDOR / FS SUBYACENTE]</sub>

```text
FS       MECANISMO
-----    -----------------------------
btrfs    btrfs qgroup limit
XFS      xfs_quota (project quotas)
ZFS      zfs set quota
ext4     project quota
```

```bash
doki run -d --storage-opt size=10G my-image:latest
```

El flag es especifico del driver. Un filesystem subyacente sin soporte
de cuota lo ignora silenciosamente.

<hr>

## Backup & Migracion

<sub>[SAVE / LOAD / EXPORT / IMPORT / COMMIT]</sub>

### Imagen

```bash
$ doki save -o myapp.tar myapp:1.0
$ doki load -i myapp.tar
```

### Filesystem de contenedor

```bash
$ doki export web > web.tar
$ doki import web.tar
```

### Snapshot de contenedor (solo btrfs / ZFS)

```bash
$ doki commit web myapp:snapshot
```

Backup full-state incluyendo volumenes esta planeado
(`doki system backup`).

<hr>

## Fuente

<sub>[PKG/STORAGE/*]</sub>

```text
FILE                              ROL
------------------------------    -----------------------------------
pkg/storage/driver.go             entry point, deteccion de driver
pkg/storage/drivers.go            implementaciones btrfs, zfs, vfs
pkg/storage/overlay.go            overlay2
pkg/storage/fuse.go               fuse-overlayfs
pkg/storage/layer.go              extraccion de capas, CAS
pkg/storage/volume.go             gestion de volumenes
pkg/storage/cache.go              cache de imagenes, cache de build
pkg/storage/mount.go              helpers de mount de Linux
pkg/storage/mount_darwin.go       shim de mount de macOS (v0.9.3)
```
