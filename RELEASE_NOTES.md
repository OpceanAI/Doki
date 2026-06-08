# Doki v0.9.3 — 190+ Bug Fixes, DokiLink-Lite Mesh Networking

## Breaking Changes

- None. v0.9.3 is fully backward-compatible with v0.9.2. The default
  port for gossip listeners is configurable via `DOKI_LINK_ADDR`
  (default `:7432`).

## What's New

### DokiLink-Lite (Mesh Networking)
- **TCP/UDP proxy in pure stdlib**: `pkg/netlink/proxy.go` and
  `pkg/netlink/udp.go` replace the old socat-based port forwarder.
  The new proxy supports half-close (graceful shutdown), idle timeouts,
  and per-connection transport wrappers.
- **TLS 1.3 wrapping for inter-host traffic**: `pkg/netlink/crypto.go`
  exposes `TLSWrapper` (default encryption layer, L1). Each link
  certificate is signed by a per-install ECDSA P-256 CA; certs include
  SAN DNS names (Go 1.15+ requirement).
- **NaCl secretbox for payload encryption** (L2, opt-in): `crypto.go`
  derives a 32-byte key from both peers' Ed25519 public keys (SHA-256,
  order-sensitive) and uses a 4-byte length-prefixed frame format for
  TCP and per-datagram nonces for UDP.
- **Install identity**: `pkg/netlink/keys.go` generates an Ed25519
  keypair and an ECDSA P-256 CA on first start, stored at
  `$DOKI_ROOT/keys/` with `0600` permissions.
- **Peer registry with TOFU trust**: `pkg/netlink/peer.go` and
  `TrustStore` persist known peers at `$DOKI_ROOT/trust/`. On first
  contact, the public key is recorded; subsequent connections verify
  signatures against this record.
- **Static peer discovery**: `pkg/netlink/discovery_static.go` reads
  `$DOKI_ROOT/mesh/peers.json` (managed by `doki link add/rm`).
- **mDNS discovery (opt-in)**: `pkg/netlink/discovery_mdns_on.go` is
  built with `-tags netlink_mdns` and depends on
  `github.com/hashicorp/mdns`. Default builds include a no-op stub.
- **Gossip protocol**: `pkg/netlink/mesh.go` exchanges ed25519-signed
  JSON messages (`hello`, `peer`, `container`) over TCP, capped at
  4 KiB per message. The 15-second tick discovers new peers
  automatically; 30-second tick refreshes from the static config.
- **New CLI**: `doki mesh {ls,status}` and `doki link {add,rm,show}`.
- **IPv6 ARPA support**: `arpaToIP` now handles `.ip6.arpa.` suffix
  for IPv6 reverse DNS lookups.
- **nftables OUTPUT chain**: Port mapping rules now also apply to
  locally-originated traffic (OUTPUT chain), not just PREROUTING.
- **Mesh data race fix**: `onMessage` now reads `pubKeyBytes()` inside
  the RLock scope, preventing races with peer map mutations.

### Bug Fixes (190+ total across Rounds 1-4)

#### Critical Fixes
- **kill not updating state**: After killing a container, the state was
  never updated to "exited" — it stayed "running" forever. Now polls
  with `signal(0)` after sending the signal, then saves exited state.
- **stop not updating state on SIGKILL failure**: When the 3-second
  SIGKILL timeout expired, Stop() returned nil without updating state.
  Now always saves exited state with exit code 137 before returning.
- **Stop ExitChan disconnect + goroutine Wait race**: Replaced ExitChan
  polling with `signal(0)` polling. Replaced goroutine+select with
  100ms sleep+signal check to avoid race with monitorProcess.
- **Exec no output**: Runtime.Exec wrote to os.Stdout (daemon stdout),
  not HTTP response. Changed to return (stdout, stderr []byte, error).
  API handler writes bytes to response.
- **flagsWithValue -f conflict**: `-f` in flagsWithValue caused
  `rm -f <container>` to skip the container ID. Removed `-f`, `-t`, `-s`
  from flagsWithValue map.
- **Compose flags after command ignored**: Parser broke out of loop
  after finding command, ignoring flags like `-p 8080:80` after
  `doki-compose up`. Now continues parsing flags after command.

#### High Fixes
- **ps --format not implemented**: Template parsing and execution for
  format strings now works. `{% raw %}{{.ID}}{% endraw %}` etc.
- **Search/history JSON tags incorrect**: Updated to match daemon
  response format (`repo_name`, `short_description` for Hub API;
  PascalCase for Docker API).
- **Push exit 0 on error**: Now parses JSON stream and checks for
  `"status":"error"` before returning.
- **Compose profile filter broken**: Services with profiles were included
  when no profiles were specified. Now excluded correctly.
- **Validate before extends**: Moved Validate() after resolveExtends()
  so inherited images pass validation.
- **ADD trailing slash**: Now detects trailing slash destination for
  correct directory placement.
- **ONBUILD type incorrect**: Stores `inst.Args[0]` (sub-instruction
  type) instead of `inst.Type` ("ONBUILD").
- **Build panic on empty tags**: Safe access with default "image" name.
- **mDNS compile errors**: Fixed WantUnicastResponse, mdns.Query(),
  AddrV6.String() calls.
- **SetupNetwork data race**: Added m.mu.Lock() around container
  access/modification in nw.Containers.
- **TCP DNS PTR missing arpaToIP**: Added conversion before
  ResolvePTR call.
- **Container list Name always empty**: stateToInfo now sets
  `info.Name = "/" + state.Name`.
- **Container inspect missing fields**: Added ImageID, ImageDigest,
  Name to inspect response.
- **Create ignores ?name= query parameter**: Now reads
  `r.URL.Query().Get("name")` and overrides body name.
- **Image inspect Config lowercase OCI**: Replaced with explicit
  PascalCase map (Entrypoint, Cmd, Env, etc.).
- **Image inspect Created int64 vs string**: Converted to RFC3339
  format via `time.Unix(record.Created, 0)`.
- **History.Created type mismatch**: Added `FlexString` type that
  handles both JSON string and int64.
- **compose ps -q truncation**: Now returns full container ID.
- **ps ID = Name same value**: CONTAINER ID now shows actual ID,
  NAMES shows the name.
- **images sha256: prefix**: Stripped from REPOSITORY column.
- **Pull exit code on failure**: Returns exit code 1 on error.

#### Medium Fixes
- **Rename returns 200 vs 204**: Changed to 204 No Content.
- **Stop already-stopped returns 204 vs 304**: Returns 304 Not
  Modified for already-stopped containers.
- **cp copies file as directory**: Fixed path handling so `cp file.txt
  container:/tmp/file.txt` writes to `/tmp/file.txt` not
  `/tmp/file.txt/file.txt`.
- **kill on non-running returns 0**: Now returns an error.
- **logs missing newlines**: Appends `\n` to each log line.
- **compose config non-deterministic order**: Services, volumes, and
  networks now sorted alphabetically.
- **compose logs non-deterministic order**: Services sorted before
  reading logs.
- **mesh onMessage data race**: Moved pubKeyBytes() inside RLock.
- **nftables OUTPUT chain empty**: Port mapping rules now also applied
  to OUTPUT chain for local traffic.
- **arpaToIP ignores IPv6**: Handles `.ip6.arpa.` with 32 nibbles.
- **Rm prints error twice**: Removed duplicate fmt.Fprintf calls.
- **Cp can't copy to non-existent path**: Fixed destination detection.
- **Compose pull doesn't deduplicate**: Tracked pulled images in map.
- **Compose logs --tail 1 returns empty**: Filters empty lines first.
- **Rename doesn't persist**: Calls SaveState after updating
  annotations.
- **Duplicate container names allowed**: Check for existing names
  before create, return 409 Conflict.
- **Attach is a stub**: Implemented log streaming with follow.
- **Update doesn't persist**: Calls SaveState after updating config.
- **--target CLI flag not implemented**: Added flag parsing and API
  pass-through.
- **inspect ImageID empty**: Set ImageDigest from imgRecord.ID.
- **compose ps ignores global -q flag**: Uses quietFlag variable.

### Other
- `dokid` startup logs the DokiLink install id and listen address at
  INFO level. Set `DOKI_LINK_MESH=0` to disable mesh entirely
  (e.g. on air-gapped hosts).
- `Makefile` produces `doki-v0.9.3-{arch}.tar.gz` under `releases/`
  with SHA256 checksums.
- `DOKI_USE_SOCAT=1` forces socat fallback for port forwarding.
- `DOKI_LINK_ADDR` overrides gossip listen address.

## Documentation

- `.wiki/Networking.md` and `.wiki/Networking.es.md` updated with
  DokiLink-Lite architecture, Mermaid sequence diagram of
  TCP-proxy + TLS + secretbox layers, and limitations of mesh
  discovery (no NAT traversal, no relay, mDNS limited to LAN).
- `README.md` and `README.es.md` updated with v0.9.3 as current
  version, DokiLink-Lite networking section, and full changelog.

## Install / Upgrade

```bash
# ARM64 (most Android devices, Apple Silicon, Linux ARM servers)
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.3/doki-v0.9.3-arm64.tar.gz | tar -xz
cd doki-v0.9.3-arm64
./install.sh

# ARMV7 (older 32-bit Android)
curl -L https://github.com/OpceanAI/Doki/releases/download/v0.9.3/doki-v0.9.3-armv7.tar.gz | tar -xz
cd doki-v0.9.3-armv7
./install.sh
```

## Verifying

```bash
dokid --version
doki version
doki mesh status
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"
```
