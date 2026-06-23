# Networking

<sub>[BRIDGE / DNS / PORTMAP / DOKILINK MESH / NAT TRAVERSAL / DHT / MDNS]</sub>

> Doki v0.11.0 networking stack. Bridge networks, CNI plugin
> support, port mapping, an internal DNS server, iptables
> DNAT/SNAT, rootless fallback (pasta/slirp4netns/host), IPv6, and
> the DokiLink mesh with TLS 1.3, NaCl secretbox, Ed25519
> signatures, NAT traversal (STUN/TURN/hole punching), Kademlia
> DHT, and mDNS discovery with 90-second expiry.

---

## Network Types

<sub>[DRIVERS / SELECTION]</sub>

```text
TYPE       DESCRIPTION                                         DRIVER
─────────────────────────────────────────────────────────────────────
bridge     Default doki0 bridge: NAT, DNS, port mapping        Linux bridge + iptables
host       Share host network namespace                        (no driver)
none       Loopback only                                       (no driver)
overlay    Multi-host (planned)                                vxlan
macvlan    Direct host NIC access                              macvlan
ipvlan     L3 isolation                                        ipvlan
```

---

## Default Bridge: `doki0`

<sub>[FIRST-START / AUTO-CREATION]</sub>

On first start, the daemon creates a Linux bridge named `doki0`.

```text
PROPERTY        DEFAULT
────────────────────────────────────────────────────────
Subnet          10.0.0.0/24
Gateway         10.0.0.1
MTU             1500
IP allocation   Sequential (.2, .3, ...)
iptables        MASQUERADE on outbound, DNAT on port forward
```

Override in `config.json`:

```json
{
  "network": {
    "bridge": "doki0",
    "default_subnet": "10.1.0.0/24",
    "mtu": 1500,
    "ipv6": false
  }
}
```

---

## Container Attachment

<sub>[VETH PAIR / IP ASSIGN / DNS REGISTER]</sub>

When a container is started with `--network bridge`, the daemon:

```text
1. Create veth pair  (veth<random>  <->  eth0 inside container)
2. Attach host-side veth to doki0
3. Assign IP from the subnet
4. Install iptables rules (DOKI chain)
5. Register container name in the internal DNS
```

### Bridge Packet Path

```text
[CONTAINER]            [HOST]                       [WIRE]
                                                         |
  eth0 10.0.0.2 ----+                                  |
                    | veth peer                         |
                    v                                   v
                 vethABC123  ---(kernel bridge)--+   doki0 (10.0.0.1)
                                                  |    |
                                                  |    | POSTROUTING
                                                  |    | MASQUERADE
                                                  |    v
                                                  |  host eth0 (192.168.1.5)
                                                  |    |
                                                  +----+-----> INTERNET
                                                         |
  Return path:  INTERNET -> host eth0 -> DNAT (DOKI) -> doki0 -> vethABC -> eth0
```

---

## Host Network

`--network host` skips the bridge and gives the container the host's network namespace. The container sees all host interfaces and IPs. Port mapping is a no-op (`-p` ignored).

Performance is the best of all modes. Security is the least isolated: the container can sniff all host traffic.

---

## None

`--network none` gives the container only `lo`. No external network. Useful for batch processing and security-sensitive workloads.

---

## Port Mapping

<sub>[DNAT / SNAT / ROOTLESS PROXY]</sub>

### Syntax

```bash
-p HOST_IP:HOST_PORT:CONTAINER_PORT/PROTOCOL
-p HOST_PORT:CONTAINER_PORT
-p CONTAINER_PORT   # random host port (use -P to publish all EXPOSE)
```

### Examples

```bash
# Map host 8080 to container 80
doki run -p 8080:80 nginx:alpine

# Bind to specific host IP
doki run -p 127.0.0.1:8080:80 nginx:alpine

# Multiple protocols
doki run -p 8080:80/tcp -p 8080:80/udp my-server:latest

# Publish all EXPOSEd ports
doki run -P nginx:alpine

# Port range
doki run -p 8080-8090:80 my-server:latest
```

### How it works (rootful)

```text
1. iptables -t nat -A DOKI -p tcp --dport 8080 -j DNAT --to-destination 10.0.0.2:80
2. iptables -t nat -A POSTROUTING -s 10.0.0.2 -j MASQUERADE   (return path)
3. socat / pkg/netlink proxy for the actual TCP relay in rootless mode
```

### v0.9.3 iptables DNAT fix

The DNAT rule construction in `pkg/network/manager.go` was using `strings.Split` and missing the `-A` (append) flag in v0.9.2:

```diff
- args := strings.Split("OUTPUT -p tcp --dport 8080 -j DNAT --to-destination 10.0.0.2:80", " ")
- exec.Command("iptables", args...).Run()  // error discarded
+ args := []string{
+     "-A", "OUTPUT",
+     "-p", "tcp",
+     "--dport", "8080",
+     "-j", "DNAT",
+     "--to-destination", "10.0.0.2:80",
+ }
+ out, err := exec.Command("iptables", args...).CombinedOutput()
+ if err != nil {
+     return fmt.Errorf("iptables DNAT: %s: %w", out, err)
+ }
```

Two things fixed:

```text
1. -A flag:  v0.9.2 had OUTPUT as the first arg, which iptables
   interpreted as the table name. The fix uses []string and
   includes -A correctly.
2. Error check: v0.9.2 called .Run() and discarded the error.
   The fix uses .CombinedOutput() and wraps the error.
```

The DOKI chain is now also auto-created in `pkg/network/cni.go:ensureChains()` (idempotent -- safe to call on every container start).

### v0.9.3 port-forwarding fix

The rootless `socat` proxy was connecting to `localhost:containerPort` instead of `containerIP:containerPort`:

```diff
- socatArgs := []string{
-     "TCP-LISTEN:8080,reuseaddr,fork",
-     "TCP:localhost:80",   // wrong: localhost from host != container
- }
+ socatArgs := []string{
+     "TCP-LISTEN:8080,reuseaddr,fork",
+     "TCP:10.0.0.2:80",    // container bridge IP
+ }
```

### UDP support (v0.9.3)

UDP port forwarding is now supported via `socat -u`:

```go
if port.Type == "udp" {
    socatArgs = append(socatArgs[:2],
        append([]string{"UDP-LISTEN:8080,reuseaddr,fork"},
               "UDP:10.0.0.2:80")...)
}
```

---

## Internal DNS

<sub>[A / AAAA / PTR / UPSTREAM FORWARD / LRU CACHE]</sub>

Doki runs an internal DNS server that handles:

```text
- Inter-container name resolution  (db -> 10.0.0.2)
- A records    (IPv4)
- AAAA records (IPv6)
- PTR records  (reverse DNS)
- Forwarding to upstream resolvers
```

### Architecture

```text
  Container /etc/resolv.conf
  nameserver 127.0.0.11
            |
            | A / AAAA / PTR
            v
  +-----------------------------+
  | Doki internal DNS           |
  |   :53   Linux               |
  |   :8053 Android (Termux)    |
  +-----------------------------+
        |              |
        | local        | upstream
        v              v
  container-name   /etc/resolv.conf
  -> bridge IP     getprop net.dns*
                   8.8.8.8 fallback
                         |
                         v
                     INTERNET
```

### Defaults (v0.9.3)

```text
PLATFORM            DEFAULT LISTEN     REASON
─────────────────────────────────────────────────────────
Linux               127.0.0.11:53      Standard unprivileged port
Android (Termux)    127.0.0.11:8053    Port 53 blocked by SELinux
macOS               not used           ModeNative has no bridge
```

Override with `DOKI_DNS_LISTEN=IP:PORT`.

### Name Resolution

```bash
$ doki network create backend
$ doki run -d --name db  --network backend postgres:alpine
$ doki run -d --name api --network backend my-api:latest

# From inside the api container:
$ doki exec api sh -c 'getent hosts db'
172.20.0.2      db.backend

# From the host (via the doki CLI):
$ doki network inspect backend
[
  {
    "Name": "backend",
    "Id": "abc123",
    "Containers": {
      "db":  {"EndpointID": "...", "IPv4Address": "172.20.0.2"},
      "api": {"EndpointID": "...", "IPv4Address": "172.20.0.3"}
    }
  }
]
```

Aliases can be set with `doki network connect --alias db postgres backend`.

### LRU Cache

```text
- 1024 entries
- 5-minute TTL per entry
- Re-registered on container restart
```

### Key v0.9.3 fixes

```text
FILE                        BUG                                     FIX
─────────────────────────────────────────────────────────────────────────
pkg/network/dns.go          Busy-wait on SetReadDeadline            ReadFromUDP blocks naturally
pkg/common/resolv.go        Stored :port in nameservers             Stripped, appended :53 for dialling
pkg/network/manager.go      DNS not registered on container start   SetupNetwork calls AddEntry
cmd/dokid/main.go           Used :53 on Android (blocked)           Uses :8053 on Android
pkg/network/dns.go          No AAAA, no PTR                         Added both
pkg/common/resolv.go        ndots:5 caused retry loops              ndots:0 default
pkg/network/dns.go          UDP-only                                TCP retry on TC bit (RFC 5966)
pkg/network/manager.go      Lost DNS on daemon restart              recoverContainers calls ReRegisterDNS
```

---

## CNI Plugins

<sub>[CONTAINER NETWORK INTERFACE / PLUGGABLE]</sub>

CNI (Container Network Interface) is a spec for pluggable networking. Doki supports these plugins:

```text
PLUGIN       PURPOSE
─────────────────────────────────────────
bridge       Linux bridge (default)
host-local   Local IP allocation
portmap      Port mapping
macvlan      Direct host NIC access
ipvlan       L3 isolation
dhcp         DHCP-based IP allocation
vlan         802.1Q VLAN tagging
```

CNI is enabled with `DOKI_CNI=/path/to/cni/conf`. The default bridge mode does not use CNI directly (faster, no plugin overhead).

Status: plugin manager exists, not fully wired into the runtime. See Known Limitations in the project README.

---

## Rootless Networking

<sub>[PASTA / SLIRP4NETNS / HOST FALLBACK]</sub>

For users without root, Doki selects a fallback in this order:

```text
PRIORITY   DRIVER          CONDITION
─────────────────────────────────────────────────────────
1          pasta           pasta binary in $PATH  (DOKI_PASTA override)
2          slirp4netns     slirp4netns binary in $PATH
3          host            last resort: share host netns
```

[pasta](https://passt.top/) is the successor to slirp4netns. It provides:

```text
- TCP/UDP connectivity without root or TAP devices
- ICS (Internet Connection Sharing) mode
- Built-in DHCP server for the container
- Bind mounts work normally
```

Usage:

```bash
# pasta is auto-detected in $PATH
# or set DOKI_PASTA=/path/to/pasta
doki run --rm --network bridge alpine ping -c 1 8.8.8.8
```

Pasta listens on the host's external interface and NATs traffic for the container. Performance is ~95% of native (vs 70% for slirp4netns).

---

## IPv6

<sub>[DUAL-STACK / EXPLICIT NET]</sub>

Enable on the default bridge:

```json
{
  "network": {
    "ipv6": true
  }
}
```

Or create an IPv6 network explicitly:

```bash
doki network create --ipv6 --subnet fd00::/64 ipv6-net
```

Doki assigns both v4 and v6 addresses when `ipv6: true`.

---

## Veth Teardown (v0.9.3 fix)

<sub>[LEAK PREVENTION]</sub>

When a container is removed, its veth pair must be deleted to avoid leaking interfaces on the host. v0.9.3 added tracking:

```go
// pkg/network/manager.go
type Endpoint struct {
    // ...existing fields...
    VethHost string  // host-side interface name (e.g. "vethabc123")
    VethPeer string  // container-side interface name (e.g. "eth0")
}
```

`teardownBridgeNetwork()` now does:

```go
// Delete both veth ends
exec.Command("ip", "link", "del", endpoint.VethHost).Run()
// (VethPeer goes away automatically with the pair)

// Then delete the bridge
exec.Command("ip", "link", "del", bridgeName).Run()
```

Before v0.9.3: `ip link` would show dozens of `veth*` interfaces after running a few containers.

---

## Security Considerations

<sub>[THREAT / MITIGATION]</sub>

```text
CONCERN                                  MITIGATION
─────────────────────────────────────────────────────────────────────────────
Container sniffing host traffic          Use --network bridge (default), not host
Container escaping via iptables manip.   DOKI chain is namespaced, separate from system rules
DNS spoofing                             DNS responses are bound to the container's IP
Port hijacking                           First container to claim a host port wins;
                                         second fails with EADDRINUSE
ARP spoofing                             Doki enables arp_ignore/arp_announce on veth
```

---

## Performance

<sub>[THROUGHPUT / LATENCY OVERHEAD]</sub>

```text
MODE                          THROUGHPUT      LATENCY OVERHEAD
─────────────────────────────────────────────────────────────
bridge (rootful)              95% native      <0.1ms
bridge (rootless, pasta)      90% native      ~0.2ms
host                          100% native     0ms
none                          (no network)    n/a
```

---

## Source

<sub>[PACKAGES / FILE INDEX]</sub>

```text
pkg/network/manager.go              bridge, port forwarding, veth, teardown
pkg/network/cni.go                  CNI plugin manager, DOKI chain
pkg/network/dns.go                  internal DNS server
pkg/network/android_dns.go          Android DNS discovery
pkg/network/rootless.go             pasta / slirp4netns integration
pkg/common/resolv.go                resolv.conf parsing
pkg/netlink/proxy.go                TCP forwarder (replaces socat)
pkg/netlink/udp.go                  UDP forwarder with session map
pkg/netlink/keys.go                 install identity, Ed25519 + ECDSA P-256
pkg/netlink/crypto.go               TLS 1.3 and NaCl secretbox wrappers
pkg/netlink/peer.go                 peer record and trust store
pkg/netlink/mesh.go                 gossip protocol, peer registry, mesh router
pkg/netlink/discovery_static.go     peers.json loader
pkg/netlink/discovery_mdns_*.go     mDNS service (opt-in, 90s expiry)
pkg/netlink/stun.go                 STUN binding requests
pkg/netlink/turn.go                 TURN relay client
pkg/netlink/punch.go                NAT hole punching coordinator
pkg/netlink/dht.go                  Kademlia DHT node + routing table
```

---

## DokiLink Mesh (Multi-Host)

<sub>[v0.11.0 / TLS 1.3 / NACL SECRETBOX / ED25519 / NAT TRAVERSAL / DHT / MDNS]</sub>

DokiLink is a TCP/UDP proxy plus mesh discovery layer. It forwards
a container's published port to another Doki instance on the same
LAN, across VLANs via static peers, or across NATs via the STUN/TURN
stack introduced in v0.11.0.

The wire protocol is intentionally minimal: no gVisor, no full
WireGuard stack. Cross-NAT reachability is provided by STUN binding
requests, TURN relay fallback, and synchronized hole punching. The
Kademlia DHT resolves peer endpoints beyond the LAN. mDNS advertises
peers on the LAN with a 90-second expiry.

### Encryption Layers

```text
LAYER          WHEN              LIBRARY                             NOTES
─────────────────────────────────────────────────────────────────────────────────────
L0 (none)      loopback-only     --                                  default on Android/Termux
L1 (TLS 1.3)   any inter-host    crypto/tls stdlib                   default, signed by per-install CA
L2 (secretbox) payload-only      golang.org/x/crypto/nacl/secretbox  opt-in via DOKI_LINK_PAYLOAD_ENC=1,
                                                                     key derived from both peers'
                                                                     Ed25519 pubkeys
```

All gossip records carry an Ed25519 signature over the canonical
record body. Verification fails closed: unsigned or mismatched
records are dropped and the peer is marked untrusted.

### Gossip Exchange

```text
  Doki A                                  Doki B
  :7432                                   :7432
  ┌──────────────┐                        ┌──────────────┐
  │ gossip tick  │  every 15s             │              │
  │ (mesh.go)    │──── dial TLS 1.3 ────> │ listener     │
  │              │     ed25519 signed     │              │
  │              │<── peer records ──────│ │ TrustStore   │
  │              │     ed25519 signed     │              │
  │ TrustStore   │                        │ TrustStore   │
  │  updated     │                        │  updated     │
  │  mesh router │                        │  mesh router │
  │  recomputes  │                        │  recomputes  │
  └──────────────┘                        └──────────────┘
        |                                        |
        | record expired (90s mDNS TTL)          | same
        v                                        v
  mDNS re-announce                         mDNS re-announce
```

### Peer Discovery

```text
  install identity (Ed25519 + CA)
            |
            +----------+----------+
            |                     |
       static                mDNS browser
       peers.json            (90s expiry, LAN-only,
            |                 -tags netlink_mdns)
            |                     |
            +----------+----------+
                       |
                  TrustStore
                       |
                  Mesh registry
                       |
                  Gossip every 15s
                       |
                  Container announcements
                       |
                  DHT lookup (Kademlia)
                  for off-LAN endpoints
```

### NAT Traversal

<sub>[STUN / TURN / HOLE PUNCHING / v0.11.0]</sub>

```text
LEGEND
  A, B    Doki peers behind NAT
  STUN    public reflexive endpoint discovery
  TURN    relay fallback for symmetric NAT
  DHT     endpoint exchange channel (Kademlia)

  1. BIND
     A --binding req--> STUN_A
     A <--mapped addr-- STUN_A      (A's public IP:port)
     B --binding req--> STUN_B
     B <--mapped addr-- STUN_B      (B's public IP:port)

  2. EXCHANGE
     A --put(B, pubA:portA)--> DHT
     B --get(A)---------------> DHT  --> pubA:portA
     B --put(A, pubB:portB)--> DHT
     A --get(B)---------------> DHT  --> pubB:portB

  3. PUNCH  (both ends send first to open NAT state)
     A === SYN/UDP to pubB:portB ===>  B's NAT  (may drop, opens A's hole)
     B === SYN/UDP to pubA:portA ===>  A's NAT  (may drop, opens B's hole)

  4. OUTCOME
     cone + cone       --> direct path established, TLS 1.3 wraps it
     symmetric either  --> fall back to TURN relay (turn.go)
     both fail         --> peer marked unreachable, mesh prunes in 90s
```

STUN binding is implemented in `pkg/netlink/stun.go` (RFC 5389
classic STUN, no ICE candidate trickle). TURN relay uses
`pkg/netlink/turn.go` (RFC 5766 ALLOCATE/CHANNELBIND). Punch
coordination lives in `pkg/netlink/punch.go` and runs a fixed
8-packet burst per peer with a 500ms timeout.

### DHT (Kademlia)

<sub>[ROUTING TABLE / k=20 / ALPHA=3]</sub>

```text
PARAMETER       VALUE
────────────────────────────────────────
Algorithm       Kademlia (Bamford-style XOR)
k               20  (bucket size)
alpha           3   (parallel lookups)
Node ID         SHA-256 of install Ed25519 pubkey
Key             install ID (12-char base32)
Value           { public endpoint, last seen, signature }
Store TTL       24h, refreshed on gossip
Routing table   256 buckets, alive peers only
```

The DHT is used for one purpose: resolving an install ID to a
reachable `{IP, port}` endpoint when neither mDNS nor `peers.json`
covers the peer. It is not a generic key-value store. DHT traffic
rides the same TLS 1.3 listener on `:7432` and is signed with the
node's Ed25519 key.

`pkg/netlink/dht.go` implements FIND_NODE / FIND_VALUE / STORE /
PING. Bucket refresh runs every 60 seconds.

### mDNS Discovery

<sub>[90s EXPIRY / LAN-ONLY / BUILD TAG]</sub>

mDNS advertises the local install on `_dokilink._tcp` and browses
for peers on the same LAN. It is gated by the `netlink_mdns` build
tag; default builds ship a stub that no-ops.

```text
PROPERTY              VALUE
────────────────────────────────────────────
Service type           _dokilink._tcp
TTL                    90 seconds
Refresh interval       30 seconds (re-announce before TTL expiry)
Prune threshold        90s without re-announce -> record evicted
Transport              multicast UDP 224.0.0.251:5353
Scope                  link-local only
Build tag              -tags netlink_mdns
Files                  pkg/netlink/discovery_mdns_on.go
                       pkg/netlink/discovery_mdns_off.go  (stub)
```

For cross-VLAN or cross-NAT scenarios, use static `peers.json` or
the DHT.

### CLI

```bash
# Inspect the local install identity.
doki mesh status
# install id:    fndwnv3mn7dt
# public key:    K0dm12xvxzUTBZ3lJkOcOyBrGPPNlCWpTJhcEv0BQys=
# ca fingerprint: cc4165e0ef4c
# ca expires:    2027-06-07

# Add a static peer.
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# List peers.
doki mesh ls
# PEER ID    NAME       ADDRESS            LAST SEEN
# mybuddy    mybuddy    192.168.1.42:7432  2s

# Remove a peer.
doki link rm mybuddy

# Trigger an explicit NAT traversal attempt for a remote peer.
doki link punch mybuddy
# probing STUN_A ... ok   reflexive 203.0.113.7:49152
# probing STUN_B ... ok   reflexive 198.51.100.4:49153
# exchanged endpoints via DHT
# punch burst (8 packets) -> direct path established
```

### Limitations (read before deploying)

```text
1. mDNS is LAN-only and build-tagged. Default builds include a
   stub. Use static peers or the DHT for cross-VLAN.
2. Per-datagram secretbox framing has a 24-byte nonce + 16-byte
   overhead. UDP-heavy workloads pay a fixed per-packet cost.
3. NAT traversal fails on double-symmetric NATs. TURN relay is
   the fallback; configure DOKI_TURN_SERVER or relays are skipped.
4. Container sees host network in proot+Termux mode. DokiLink
   forwarding is a TCP/UDP relay on the host loopback, not a
   network-namespace bridge. Containers in proot mode can already
   reach any service on the host, so the proxy does not add a
   security boundary in that mode.
5. Wire protocol is JSON, capped at 4 KiB per message. gRPC /
   protobuf is a future v0.12+ feature.
6. CA lifetime is 365 days, link certs 90 days. Re-issue with
   `doki mesh status` to confirm expiry. Rotation tooling is
   planned for v0.12.
7. DHT store TTL is 24h. A peer that goes silent for >24h is
   evicted from remote routing tables; it must re-announce on
   restart.
```

---

## See Also

- [Architecture](Architecture) -- daemon layers, pipeline, runner registry
- [Isolation Levels](Isolation-Levels) -- 12 runner modes from WASM to microVM
- [Security](Security) -- seccomp, AppArmor, capabilities, TLS
- [Configuration](Configuration) -- config.json schema and env vars
