// Package coredns provides cluster DNS for Kubernetes.
package coredns

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type ClusterDNS struct {
	store      store.Store
	domain     string
	listenAddr string
	services   map[string]*serviceRecord
	pods       map[string]string
	mu         sync.RWMutex
	logger     *slog.Logger
}

type serviceRecord struct {
	Name      string
	Namespace string
	ClusterIP string
	Ports     []k8s.ServicePort
	Type      string
}

func NewClusterDNS(s store.Store, domain, listenAddr string, logger *slog.Logger) *ClusterDNS {
	return &ClusterDNS{
		store:      s,
		domain:     domain,
		listenAddr: listenAddr,
		services:   make(map[string]*serviceRecord),
		pods:       make(map[string]string),
		logger:     logger,
	}
}

func (d *ClusterDNS) Run(ctx context.Context) error {
	go d.watchServices(ctx)
	go d.watchPods(ctx)
	go d.serve(ctx)
	d.logger.Info("cluster DNS started", "domain", d.domain, "listen", d.listenAddr)
	<-ctx.Done()
	return nil
}

func (d *ClusterDNS) watchServices(ctx context.Context) {
	prefix := store.KeyFor("", "services", "", "")
	ch, _ := d.store.Watch(prefix, 0)
	defer d.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var svc k8s.Service
			if err := json.Unmarshal(event.Object.Value, &svc); err != nil {
				continue
			}
			d.syncService(&svc, event.Type)
		}
	}
}

func (d *ClusterDNS) watchPods(ctx context.Context) {
	prefix := store.KeyFor("", "pods", "", "")
	ch, _ := d.store.Watch(prefix, 0)
	defer d.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var pod k8s.Pod
			if err := json.Unmarshal(event.Object.Value, &pod); err != nil {
				continue
			}
			d.syncPod(&pod, event.Type)
		}
	}
}

func (d *ClusterDNS) syncService(svc *k8s.Service, eventType string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := svc.Namespace + "/" + svc.Name
	if eventType == store.EventDeleted {
		delete(d.services, key)
		return
	}

	d.services[key] = &serviceRecord{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		ClusterIP: svc.Spec.ClusterIP,
		Ports:     svc.Spec.Ports,
		Type:      svc.Spec.Type,
	}
}

func (d *ClusterDNS) syncPod(pod *k8s.Pod, eventType string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if pod.Status.PodIP == "" {
		return
	}

	podKey := pod.Namespace + "/" + pod.Name
	if eventType == store.EventDeleted {
		delete(d.pods, podKey)
		return
	}

	d.pods[podKey] = pod.Status.PodIP
}

func (d *ClusterDNS) serve(ctx context.Context) {
	addr := d.listenAddr
	if addr == "" {
		addr = "10.96.0.10:53"
	}

	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		d.logger.Error("DNS listen failed", "addr", addr, "error", err)
		return
	}
	defer func() { _ = pc.Close() }()

	go func() {
		<-ctx.Done()
		_ = pc.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, remoteAddr, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		data := make([]byte, n)
		copy(data, buf[:n])
		go d.handleDNSQuery(pc, remoteAddr, data)
	}
}

func (d *ClusterDNS) handleDNSQuery(pc net.PacketConn, addr net.Addr, data []byte) {
	if len(data) < 12 {
		return
	}

	qname := extractQName(data)
	if qname == "" {
		return
	}

	qtype, qEnd := extractQTypeAndEnd(data)
	if qEnd < 0 {
		return
	}

	var response []byte
	switch qtype {
	case 1: // A
		ip := d.resolve(qname)
		if ip == "" {
			response = buildNXDOMAINResponse(data, qEnd)
		} else {
			response = buildAResponse(data, qEnd, ip)
		}
	case 28: // AAAA
		response = buildNXDOMAINResponse(data, qEnd)
	case 33: // SRV
		response = d.buildSRVResponse(data, qEnd, qname)
	default:
		response = buildNXDOMAINResponse(data, qEnd)
	}

	if response != nil {
		_, _ = pc.WriteTo(response, addr)
	}
}

func (d *ClusterDNS) resolve(qname string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	qname = strings.TrimSuffix(qname, ".")
	suffix := "." + d.domain

	if strings.HasSuffix(qname, suffix) {
		name := strings.TrimSuffix(qname, suffix)
		parts := strings.Split(name, ".")

		switch {
		case len(parts) == 3 && parts[2] == "svc":
			svcName, ns := parts[0], parts[1]
			key := ns + "/" + svcName
			if rec, ok := d.services[key]; ok {
				return rec.ClusterIP
			}

		case len(parts) == 4 && parts[3] == "pod":
			podName, ns := parts[0], parts[1]
			key := ns + "/" + podName
			if ip, ok := d.pods[key]; ok {
				return ip
			}

		case len(parts) == 2 && parts[1] == "svc":
			svcName := parts[0]
			for _, rec := range d.services {
				if rec.Name == svcName {
					return rec.ClusterIP
				}
			}
		}
	}

	return ""
}

func extractQName(data []byte) string {
	if len(data) < 13 {
		return ""
	}
	pos := 12
	var labels []string
	for pos < len(data) {
		length := int(data[pos])
		if length == 0 {
			break
		}
		pos++
		if pos+length > len(data) {
			return ""
		}
		labels = append(labels, string(data[pos:pos+length]))
		pos += length
	}
	return strings.Join(labels, ".")
}

// extractQTypeAndEnd returns the query's QTYPE and the byte offset of the
// first byte following the question section. Returns (0, -1) if the query
// is malformed.
func extractQTypeAndEnd(data []byte) (uint16, int) {
	if len(data) < 12 {
		return 0, -1
	}
	pos := 12
	for pos < len(data) {
		length := int(data[pos])
		if length == 0 {
			pos++
			break
		}
		pos += length + 1
	}
	if pos+4 > len(data) {
		return 0, -1
	}
	qtype := uint16(data[pos])<<8 | uint16(data[pos+1])
	return qtype, pos + 4
}

// buildResponseBase copies the query header and full question section into a
// new response buffer, sets the response flags (QR=1, AA=1), and the given
// ANCOUNT and RCODE. The returned buffer is ready to have answer records
// appended. qEnd is the offset of the first byte after the question section.
func buildResponseBase(query []byte, qEnd int, ancount int, rcode byte) []byte {
	response := make([]byte, qEnd)
	copy(response, query[:qEnd])
	// Byte 2: QR=1, OPCODE=preserved, AA=1, TC=0, RD=preserved
	response[2] = (query[2] & 0x79) | 0x84
	// Byte 3: RA=0, Z=0, RCODE=rcode
	response[3] = rcode & 0x0F
	// QDCOUNT: preserved from query (typically 1)
	// ANCOUNT
	response[6] = byte(ancount >> 8)
	response[7] = byte(ancount)
	// NSCOUNT = 0
	response[8] = 0x00
	response[9] = 0x00
	// ARCOUNT = 0
	response[10] = 0x00
	response[11] = 0x00
	return response
}

// buildAResponse constructs a DNS A record response for the given IPv4.
func buildAResponse(query []byte, qEnd int, ip string) []byte {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return buildNXDOMAINResponse(query, qEnd)
	}
	octets := make([]byte, 4)
	for i, p := range parts {
		var val int
		n, _ := fmt.Sscanf(p, "%d", &val)
		if n != 1 || val < 0 || val > 255 {
			return buildNXDOMAINResponse(query, qEnd)
		}
		octets[i] = byte(val)
	}

	response := buildResponseBase(query, qEnd, 1, 0)
	response = append(response,
		0xC0, 0x0C, // name pointer to offset 12
		0x00, 0x01, // type A
		0x00, 0x01, // class IN
		0x00, 0x00, 0x00, 0x1E, // TTL 30
		0x00, 0x04, // RDLENGTH 4
	)
	response = append(response, octets...)
	return response
}

// buildNXDOMAINResponse constructs a DNS response with RCODE=3 (NXDOMAIN)
// and no answer records.
func buildNXDOMAINResponse(query []byte, qEnd int) []byte {
	return buildResponseBase(query, qEnd, 0, 3)
}

// encodeName encodes a dotted domain name into DNS wire format (a sequence
// of length-prefixed labels terminated by a zero byte).
func encodeName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	var buf []byte
	for _, label := range strings.Split(name, ".") {
		if len(label) == 0 {
			continue
		}
		buf = append(buf, byte(len(label)))
		buf = append(buf, []byte(label)...)
	}
	return append(buf, 0)
}

// resolveSRV parses an SRV query name of the form
// _<port>._<proto>.<svc>.<ns>.svc.<domain> and returns the matching service
// port and the target FQDN.
func (d *ClusterDNS) resolveSRV(qname string) (port uint16, target string, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	qname = strings.TrimSuffix(qname, ".")
	suffix := "." + d.domain
	if !strings.HasSuffix(qname, suffix) {
		return 0, "", false
	}
	name := strings.TrimSuffix(qname, suffix)
	parts := strings.Split(name, ".")
	if len(parts) != 5 || parts[4] != "svc" {
		return 0, "", false
	}
	if !strings.HasPrefix(parts[0], "_") || !strings.HasPrefix(parts[1], "_") {
		return 0, "", false
	}
	portStr := parts[0][1:]
	proto := parts[1][1:]
	if proto != "tcp" && proto != "udp" {
		return 0, "", false
	}
	svcName, ns := parts[2], parts[3]
	rec, exists := d.services[ns+"/"+svcName]
	if !exists {
		return 0, "", false
	}

	target = svcName + "." + ns + ".svc." + d.domain + "."
	if portNum, err := strconv.Atoi(portStr); err == nil {
		for _, p := range rec.Ports {
			if p.Port == int32(portNum) {
				return uint16(portNum), target, true
			}
		}
	} else {
		for _, p := range rec.Ports {
			if p.Name == portStr {
				return uint16(p.Port), target, true
			}
		}
	}
	return 0, "", false
}

// buildSRVResponse constructs a DNS SRV record response, or an NXDOMAIN
// response if the query does not match a known service.
func (d *ClusterDNS) buildSRVResponse(query []byte, qEnd int, qname string) []byte {
	port, target, ok := d.resolveSRV(qname)
	if !ok {
		return buildNXDOMAINResponse(query, qEnd)
	}

	targetName := encodeName(target)
	rdlength := 6 + len(targetName) // priority + weight + port + target

	response := buildResponseBase(query, qEnd, 1, 0)
	response = append(response,
		0xC0, 0x0C, // name pointer to offset 12
		0x00, 0x21, // type SRV (33)
		0x00, 0x01, // class IN
		0x00, 0x00, 0x00, 0x1E, // TTL 30
		byte(rdlength>>8), byte(rdlength), // RDLENGTH
		0x00, 0x00, // priority 0
		0x00, 0x00, // weight 0
		byte(port>>8), byte(port), // port
	)
	response = append(response, targetName...)
	return response
}
