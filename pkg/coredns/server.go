package coredns

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type ClusterDNS struct {
	store         store.Store
	domain        string
	listenAddr    string
	services      map[string]*serviceRecord
	pods          map[string]string
	mu            sync.RWMutex
	logger        *slog.Logger
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
		go d.handleDNSQuery(pc, remoteAddr, buf[:n])
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

	ip := d.resolve(qname)
	if ip == "" {
		return
	}

	response := d.buildResponse(data, ip)
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

func (d *ClusterDNS) buildResponse(query []byte, ip string) []byte {
	if len(query) < 12 {
		return nil
	}

	response := make([]byte, len(query))
	copy(response, query)

	response[2] = 0x81
	response[3] = 0x80
	response[6] = 0x00
	response[7] = 0x01
	response[10] = 0x00
	response[11] = 0x01

	pos := 12
	for pos < len(query) {
		length := int(query[pos])
		if length == 0 {
			pos++
			break
		}
		pos += length + 1
	}
	pos += 4

	answer := []byte{
		0xC0, 0x0C,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x1E,
		0x00, 0x04,
	}
	response = append(response[:pos], answer...)

	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		for _, p := range parts {
			var b byte
			_, _ = fmt.Sscanf(p, "%d", &b)
			response = append(response, b)
		}
	}

	return response
}

func init() {
	_ = time.Now
}
