# Networking

<sub>[BRIDGE / DNS / PORTMAP / DOKILINK MESH / NAT TRAVERSAL / DHT / MDNS]</sub>

> Stack de networking de Doki v0.11.0. Redes bridge, soporte de
> plugins CNI, port mapping, servidor DNS interno, iptables
> DNAT/SNAT, fallback rootless (pasta/slirp4netns/host), IPv6, y
> mesh DokiLink con TLS 1.3, NaCl secretbox, firmas Ed25519, NAT
> traversal (STUN/TURN/hole punching), DHT Kademlia, y
> descubrimiento mDNS con expiracion de 90 segundos.

---

## Tipos de red

<sub>[DRIVERS / SELECCION]</sub>

```text
TYPE       DESCRIPTION                                         DRIVER
─────────────────────────────────────────────────────────────────────
bridge     Bridge doki0 por defecto: NAT, DNS, port mapping     Linux bridge + iptables
host       Comparte el namespace de red del host                (sin driver)
none       Solo loopback                                        (sin driver)
overlay    Multi-host (planeado)                                vxlan
macvlan    Acceso directo a la NIC del host                     macvlan
ipvlan     Aislamiento L3                                       ipvlan
```

---

## Bridge por defecto: `doki0`

<sub>[PRIMER ARRANQUE / AUTO-CREACION]</sub>

Al primer arranque, el daemon crea un bridge Linux llamado `doki0`.

```text
PROPERTY        DEFAULT
────────────────────────────────────────────────────────
Subnet          10.0.0.0/24
Gateway         10.0.0.1
MTU             1500
IP allocation   Secuencial (.2, .3, ...)
iptables        MASQUERADE en outbound, DNAT en port forward
```

Sobrescribe en `config.json`:

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

## Attach del contenedor

<sub>[PAR VETH / ASIGNACION IP / REGISTRO DNS]</sub>

Cuando se inicia un contenedor con `--network bridge`, el daemon:

```text
1. Crea par veth  (veth<random>  <->  eth0 dentro del contenedor)
2. Attach del veth del lado host a doki0
3. Asigna IP de la subnet
4. Instala reglas iptables (chain DOKI)
5. Registra el nombre del contenedor en el DNS interno
```

### Camino de paquetes del bridge

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
  Camino de retorno:  INTERNET -> host eth0 -> DNAT (DOKI) -> doki0 -> vethABC -> eth0
```

---

## Red Host

`--network host` salta el bridge y le da al contenedor el namespace de red del host. El contenedor ve todas las interfaces e IPs del host. Port mapping es no-op (`-p` ignorado).

El rendimiento es el mejor de todos los modos. Seguridad: el menos aislado; el contenedor puede sniffear todo el trafico del host.

---

## None

`--network none` le da al contenedor solo `lo`. Sin red externa. Util para procesamiento batch y cargas sensibles a seguridad.

---

## Port Mapping

<sub>[DNAT / SNAT / PROXY ROOTLESS]</sub>

### Sintaxis

```bash
-p HOST_IP:HOST_PORT:CONTAINER_PORT/PROTOCOL
-p HOST_PORT:CONTAINER_PORT
-p CONTAINER_PORT   # puerto host aleatorio (usa -P para publicar todos los EXPOSE)
```

### Ejemplos

```bash
# Mapa host 8080 a container 80
doki run -p 8080:80 nginx:alpine

# Bind a IP especifica del host
doki run -p 127.0.0.1:8080:80 nginx:alpine

# Multiples protocolos
doki run -p 8080:80/tcp -p 8080:80/udp my-server:latest

# Publica todos los puertos EXPOSE
doki run -P nginx:alpine

# Rango de puertos
doki run -p 8080-8090:80 my-server:latest
```

### Como funciona (rootful)

```text
1. iptables -t nat -A DOKI -p tcp --dport 8080 -j DNAT --to-destination 10.0.0.2:80
2. iptables -t nat -A POSTROUTING -s 10.0.0.2 -j MASQUERADE   (camino de retorno)
3. socat / pkg/netlink proxy para el relevo TCP real en modo rootless
```

### Fix iptables DNAT de v0.9.3

La construccion de la regla DNAT en `pkg/network/manager.go` usaba `strings.Split` y le faltaba el flag `-A` (append) en v0.9.2:

```diff
- args := strings.Split("OUTPUT -p tcp --dport 8080 -j DNAT --to-destination 10.0.0.2:80", " ")
- exec.Command("iptables", args...).Run()  // error descartado
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

Dos cosas arregladas:

```text
1. Flag -A: v0.9.2 tenia OUTPUT como primer arg, que iptables
   interpretaba como el nombre de la tabla. El fix usa []string e
   incluye -A correctamente.
2. Chequeo de error: v0.9.2 llamaba .Run() y descartaba el error.
   El fix usa .CombinedOutput() y envuelve el error.
```

La chain DOKI ahora tambien se auto-crea en `pkg/network/cni.go:ensureChains()` (idempotente -- seguro llamarla en cada start de contenedor).

### Fix de port-forwarding de v0.9.3

El proxy `socat` rootless se conectaba a `localhost:containerPort` en lugar de `containerIP:containerPort`:

```diff
- socatArgs := []string{
-     "TCP-LISTEN:8080,reuseaddr,fork",
-     "TCP:localhost:80",   // mal: localhost desde host != contenedor
- }
+ socatArgs := []string{
+     "TCP-LISTEN:8080,reuseaddr,fork",
+     "TCP:10.0.0.2:80",    // IP bridge del contenedor
+ }
```

### Soporte UDP (v0.9.3)

Port forwarding UDP ahora esta soportado via `socat -u`:

```go
if port.Type == "udp" {
    socatArgs = append(socatArgs[:2],
        append([]string{"UDP-LISTEN:8080,reuseaddr,fork"},
               "UDP:10.0.0.2:80")...)
}
```

---

## DNS interno

<sub>[A / AAAA / PTR / FORWARD UPSTREAM / CACHE LRU]</sub>

Doki corre un servidor DNS interno que maneja:

```text
- Resolucion de nombres entre contenedores  (db -> 10.0.0.2)
- Registros A    (IPv4)
- Registros AAAA (IPv6)
- Registros PTR  (DNS inverso)
- Forwarding a resolvers upstream
```

### Arquitectura

```text
  Container /etc/resolv.conf
  nameserver 127.0.0.11
            |
            | A / AAAA / PTR
            v
  +-----------------------------+
  | DNS interno de Doki         |
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
Linux               127.0.0.11:53      Puerto estandar sin privilegios
Android (Termux)    127.0.0.11:8053    Puerto 53 bloqueado por SELinux
macOS               no se usa          ModeNative no tiene bridge
```

Sobrescribe con `DOKI_DNS_LISTEN=IP:PORT`.

### Resolucion de nombres

```bash
$ doki network create backend
$ doki run -d --name db  --network backend postgres:alpine
$ doki run -d --name api --network backend my-api:latest

# Desde dentro del contenedor api:
$ doki exec api sh -c 'getent hosts db'
172.20.0.2      db.backend

# Desde el host (via el CLI doki):
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

Los aliases se pueden setear con `doki network connect --alias db postgres backend`.

### Cache LRU

```text
- 1024 entradas
- TTL de 5 minutos por entrada
- Re-registrada al reiniciar el contenedor
```

### Fixes clave de v0.9.3

```text
FILE                        BUG                                     FIX
─────────────────────────────────────────────────────────────────────────
pkg/network/dns.go          Busy-wait en SetReadDeadline            ReadFromUDP bloquea naturalmente
pkg/common/resolv.go        Almacenaba :port en nameservers         Stripped, appended :53 para dialling
pkg/network/manager.go      DNS no registrado al iniciar container  SetupNetwork llama a AddEntry
cmd/dokid/main.go           Usaba :53 en Android (bloqueado)        Usa :8053 en Android
pkg/network/dns.go          Sin AAAA, sin PTR                       Anadidos ambos
pkg/common/resolv.go        ndots:5 causaba loops de retry          ndots:0 por defecto
pkg/network/dns.go          Solo UDP                                Reintento TCP en bit TC (RFC 5966)
pkg/network/manager.go      DNS perdido al reiniciar daemon         recoverContainers llama a ReRegisterDNS
```

---

## Plugins CNI

<sub>[CONTAINER NETWORK INTERFACE / PLUGGABLE]</sub>

CNI (Container Network Interface) es una spec para networking pluggable. Doki soporta estos plugins:

```text
PLUGIN       PURPOSE
─────────────────────────────────────────
bridge       Bridge Linux (default)
host-local   Asignacion local de IP
portmap      Port mapping
macvlan      Acceso directo a NIC del host
ipvlan       Aislamiento L3
dhcp         Asignacion de IP basada en DHCP
vlan         Tagging 802.1Q VLAN
```

CNI se habilita con `DOKI_CNI=/path/to/cni/conf`. El modo bridge por defecto no usa CNI directamente (mas rapido, sin overhead de plugin).

Estado: el gestor de plugins existe, no esta completamente conectado al runtime. Ver Limitaciones conocidas en el README del proyecto.

---

## Networking rootless

<sub>[PASTA / SLIRP4NETNS / FALLBACK HOST]</sub>

Para usuarios sin root, Doki selecciona un fallback en este orden:

```text
PRIORITY   DRIVER          CONDITION
─────────────────────────────────────────────────────────
1          pasta           binario pasta en $PATH  (override DOKI_PASTA)
2          slirp4netns     binario slirp4netns en $PATH
3          host            ultimo recurso: compartir netns del host
```

[pasta](https://passt.top/) es el sucesor de slirp4netns. Provee:

```text
- Conectividad TCP/UDP sin root ni dispositivos TAP
- Modo ICS (Internet Connection Sharing)
- Servidor DHCP integrado para el contenedor
- Los bind mounts funcionan normalmente
```

Uso:

```bash
# pasta se auto-detecta en $PATH
# o setea DOKI_PASTA=/path/to/pasta
doki run --rm --network bridge alpine ping -c 1 8.8.8.8
```

Pasta escucha en la interfaz externa del host y hace NAT del trafico para el contenedor. El rendimiento es ~95% del nativo (vs 70% para slirp4netns).

---

## IPv6

<sub>[DUAL-STACK / RED EXPLICITA]</sub>

Habilita en el bridge por defecto:

```json
{
  "network": {
    "ipv6": true
  }
}
```

O crea una red IPv6 explicitamente:

```bash
doki network create --ipv6 --subnet fd00::/64 ipv6-net
```

Doki asigna direcciones v4 y v6 cuando `ipv6: true`.

---

## Teardown de veth (fix v0.9.3)

<sub>[PREVENCION DE LEAKS]</sub>

Cuando se elimina un contenedor, su par veth debe ser eliminado para evitar filtrar interfaces en el host. v0.9.3 anadio tracking:

```go
// pkg/network/manager.go
type Endpoint struct {
    // ...campos existentes...
    VethHost string  // nombre de interfaz del lado host (ej. "vethabc123")
    VethPeer string  // nombre de interfaz del lado container (ej. "eth0")
}
```

`teardownBridgeNetwork()` ahora hace:

```go
// Elimina ambos extremos veth
exec.Command("ip", "link", "del", endpoint.VethHost).Run()
// (VethPeer se va automaticamente con el par)

// Luego elimina el bridge
exec.Command("ip", "link", "del", bridgeName).Run()
```

Antes de v0.9.3: `ip link` mostraba decenas de interfaces `veth*` despues de correr unos cuantos contenedores.

---

## Consideraciones de seguridad

<sub>[AMENAZA / MITIGACION]</sub>

```text
CONCERN                                  MITIGATION
─────────────────────────────────────────────────────────────────────────────
Container sniffeando trafico del host    Usa --network bridge (default), no host
Container escapando via manip. iptables  La chain DOKI esta namespaced, separada de reglas del sistema
DNS spoofing                             Las respuestas DNS estan bound a la IP del contenedor
Port hijacking                           El primer contenedor en reclamar un puerto del host gana;
                                         el segundo falla con EADDRINUSE
ARP spoofing                             Doki habilita arp_ignore/arp_announce en los veth
```

---

## Rendimiento

<sub>[THROUGHPUT / OVERHEAD DE LATENCIA]</sub>

```text
MODE                          THROUGHPUT      LATENCY OVERHEAD
─────────────────────────────────────────────────────────────
bridge (rootful)              95% nativo      <0.1ms
bridge (rootless, pasta)      90% nativo      ~0.2ms
host                          100% nativo     0ms
none                          (sin red)       n/a
```

---

## Fuente

<sub>[PAQUETES / INDICE DE ARCHIVOS]</sub>

```text
pkg/network/manager.go              bridge, port forwarding, veth, teardown
pkg/network/cni.go                  gestor de plugins CNI, chain DOKI
pkg/network/dns.go                  servidor DNS interno
pkg/network/android_dns.go          descubrimiento DNS en Android
pkg/network/rootless.go             integracion pasta / slirp4netns
pkg/common/resolv.go                parsing de resolv.conf
pkg/netlink/proxy.go                reenviador TCP (reemplaza socat)
pkg/netlink/udp.go                  reenviador UDP con mapa de sesiones
pkg/netlink/keys.go                 identidad de instalacion, Ed25519 + ECDSA P-256
pkg/netlink/crypto.go               envolturas TLS 1.3 y NaCl secretbox
pkg/netlink/peer.go                 registro de pares y almacén de confianza
pkg/netlink/mesh.go                 protocolo gossip, registro de pares, router mesh
pkg/netlink/discovery_static.go     cargador de peers.json
pkg/netlink/discovery_mdns_*.go     servicio mDNS (opt-in, 90s expiry)
pkg/netlink/stun.go                 STUN binding requests
pkg/netlink/turn.go                 cliente de relevo TURN
pkg/netlink/punch.go                coordinador de NAT hole punching
pkg/netlink/dht.go                  nodo DHT Kademlia + tabla de routing
```

---

## DokiLink Mesh (Multi-Host)

<sub>[v0.11.0 / TLS 1.3 / NACL SECRETBOX / ED25519 / NAT TRAVERSAL / DHT / MDNS]</sub>

DokiLink es un proxy TCP/UDP mas una capa de descubrimiento mesh.
Reenvia un puerto publicado de un contenedor a otra instancia Doki
en la misma LAN, a traves de VLANs via pares estaticos, o a traves
de NATs via el stack STUN/TURN introducido en v0.11.0.

El protocolo de cable es intencionalmente minimalista: sin gVisor,
sin stack completo de WireGuard. La alcanzabilidad cross-NAT la
proveen STUN binding requests, fallback de relevo TURN, y hole
punching sincronizado. La DHT Kademlia resuelve endpoints de pares
mas alla de la LAN. mDNS anuncia pares en la LAN con expiracion de
90 segundos.

### Capas de cifrado

```text
LAYER          WHEN              LIBRARY                             NOTES
─────────────────────────────────────────────────────────────────────────────────────
L0 (none)      solo loopback     --                                  default en Android/Termux
L1 (TLS 1.3)   cualquier inter   crypto/tls stdlib                   default, firmado por CA por instalacion
L2 (secretbox) solo payload      golang.org/x/crypto/nacl/secretbox  opt-in via DOKI_LINK_PAYLOAD_ENC=1,
                                                                     clave derivada de las pubkeys
                                                                     Ed25519 de ambos pares
```

Todos los registros de gossip llevan una firma Ed25519 sobre el
cuerpo canonico del registro. La verificacion falla cerrado: los
registros sin firma o con mismatch se descartan y el par se marca
como no confiable.

### Intercambio de gossip

Cada 15 segundos, cada instancia Doki marca a sus pares conocidos
en el puerto 7432 usando TLS 1.3 con mensajes firmados Ed25519. El
marcador envia sus registros de pares (ID, direccion, clave
publica); el oyente verifica la firma contra su TrustStore, fusiona
los pares nuevos en su registro mesh, y responde con sus propios
registros. Ambos lados actualizan su TrustStore y recomputan la
tabla de routing mesh. El re-anuncio mDNS ocurre cuando un registro
expira despues de 90 segundos.

### Descubrimiento de pares

El descubrimiento empieza desde la identidad de instalacion (par de
claves Ed25519 mas una CA auto-firmada). Dos canales alimentan el
TrustStore: configuracion estatica via `peers.json`, y browsing mDNS
(solo LAN, TTL 90s, requiere el build tag `netlink_mdns`). El
TrustStore alimenta el registro mesh, que controla el gossip cada 15
segundos. Los anuncios de contenedores se propagan por el mismo canal
de gossip. Para endpoints fuera de LAN, las busquedas DHT via
Kademlia resuelven las direcciones de los pares sin configuracion
estatica.

### NAT Traversal

<sub>[STUN / TURN / HOLE PUNCHING / v0.11.0]</sub>

El NAT traversal sigue una secuencia de cuatro etapas:

1. **Bind**: cada par envia una peticion STUN binding a un servidor
   STUN publico. El servidor responde con el atributo
   XOR-MAPPED-ADDRESS, revelando la IP y puerto publicos del par
   segun los ve el NAT.
2. **Exchange**: los pares publican sus endpoints publicos al DHT.
   Cada par recupera la direccion publica del otro via una busqueda
   DHT.
3. **Punch**: ambos pares envian simultaneamente paquetes SYN a la
   direccion publica del otro. Esto abre un pinhole en la tabla de
   estado del NAT de cada lado, permitiendo que el SYN entrante pase.
4. **Outcome**: si ambos NAT son tipo cone, se establece un canal
   directo TLS 1.3. Si cualquiera es simetrico, el trafico cae al
   relay TURN. Si todos los metodos fallan, el par se marca
   inalcanzable y se poda del mesh despues de 90 segundos.

STUN binding esta implementado en `pkg/netlink/stun.go` (RFC 5389
STUN clasico, sin ICE candidate trickle). El relevo TURN usa
`pkg/netlink/turn.go` (RFC 5766 ALLOCATE/CHANNELBIND). La
coordinacion de punch vive en `pkg/netlink/punch.go` y lanza un
burst fijo de 8 paquetes por par con timeout de 500ms.

### DHT (Kademlia)

<sub>[TABLA DE ROUTING / k=20 / ALPHA=3]</sub>

```text
PARAMETER       VALUE
────────────────────────────────────────
Algoritmo       Kademlia (XOR estilo Bamford)
k               20  (tamano de bucket)
alpha           3   (lookups paralelos)
Node ID         SHA-256 de la pubkey Ed25519 de instalacion
Key             ID de instalacion (base32, 12 chars)
Value           { endpoint publico, last seen, firma }
Store TTL       24h, refrescado en gossip
Routing table   256 buckets, solo pares vivos
```

La DHT se usa para un proposito: resolver un ID de instalacion a
un endpoint `{IP, port}` alcanzable cuando ni mDNS ni `peers.json`
cubren al par. No es un key-value store generico. El trafico de
DHT viaja en el mismo listener TLS 1.3 en `:7432` y se firma con
la clave Ed25519 del nodo.

`pkg/netlink/dht.go` implementa FIND_NODE / FIND_VALUE / STORE /
PING. El refresh de buckets corre cada 60 segundos.

### Descubrimiento mDNS

<sub>[90s EXPIRY / SOLO LAN / BUILD TAG]</sub>

mDNS anuncia la instalacion local en `_dokilink._tcp` y busca pares
en la misma LAN. Esta gateado por el build tag `netlink_mdns`; las
builds por defecto incluyen un stub que no-op.

```text
PROPERTY              VALUE
────────────────────────────────────────────
Service type           _dokilink._tcp
TTL                    90 segundos
Refresh interval       30 segundos (re-announce antes de expirar TTL)
Prune threshold        90s sin re-announce -> registro evicted
Transport              multicast UDP 224.0.0.251:5353
Scope                  solo link-local
Build tag              -tags netlink_mdns
Files                  pkg/netlink/discovery_mdns_on.go
                       pkg/netlink/discovery_mdns_off.go  (stub)
```

Para escenarios cross-VLAN o cross-NAT, usa `peers.json` estatico o
la DHT.

### CLI

```bash
# Inspeccionar la identidad local de instalacion.
doki mesh status
# install id:    fndwnv3mn7dt
# public key:    K0dm12xvxzUTBZ3lJkOcOyBrGPPNlCWpTJhcEv0BQys=
# ca fingerprint: cc4165e0ef4c
# ca expires:    2027-06-07

# Agregar un par estatico.
doki link add mybuddy 192.168.1.42:7432 --pub "$(doki mesh status | awk '/public key/ {print $3}')"

# Listar pares.
doki mesh ls
# PEER ID    NAME       ADDRESS            LAST SEEN
# mybuddy    mybuddy    192.168.1.42:7432  2s

# Eliminar un par.
doki link rm mybuddy

# Disparar un intento explicito de NAT traversal para un par remoto.
doki link punch mybuddy
# probing STUN_A ... ok   reflexive 203.0.113.7:49152
# probing STUN_B ... ok   reflexive 198.51.100.4:49153
# exchanged endpoints via DHT
# punch burst (8 packets) -> direct path established
```

### Limitaciones (lea antes de desplegar)

```text
1. mDNS es solo LAN y build-taggeado. Las builds por defecto
   incluyen un stub. Use pares estaticos o la DHT para cross-VLAN.
2. El framing secretbox por datagrama tiene 24 bytes de nonce
   + 16 bytes de overhead. Las cargas UDP pesadas pagan un costo
   fijo por paquete.
3. NAT traversal falla en double-symmetric NATs. El relevo TURN es
   el fallback; configure DOKI_TURN_SERVER o los relays se saltan.
4. El contenedor ve la red del host en modo proot+Termux. El
   reenvio DokiLink es un relevo TCP/UDP en el loopback del host,
   no un puente de namespaces de red. Los contenedores en modo
   proot ya pueden alcanzar cualquier servicio del host, asi que
   el proxy no anade una frontera de seguridad en ese modo.
5. El protocolo de cable es JSON, limitado a 4 KiB por mensaje.
   gRPC / protobuf es una feature futura de v0.12+.
6. Vida util de la CA: 365 dias, certificados de enlace 90 dias.
   Re-emita con `doki mesh status` para confirmar expiracion.
   La herramienta de rotacion esta planificada para v0.12.
7. El Store TTL de la DHT es 24h. Un par que se queda silencioso
   >24h se evicta de las tablas de routing remotas; debe
   re-anunciarse al reiniciar.
```

---

## Vease tambien

- [Arquitectura](Architecture.es) -- capas del daemon, pipeline, registro de runners
- [Niveles de Aislamiento](Isolation-Levels.es) -- 12 modos de runner desde WASM a microVM
- [Seguridad](Security.es) -- seccomp, AppArmor, capabilities, TLS
- [Configuracion](Configuration.es) -- schema de config.json y env vars
