// Package kubeproxy provides the Kubernetes kube-proxy.
package kubeproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type ProxyMode string

const (
	ModeIPTables  ProxyMode = "iptables"
	ModeNFTables  ProxyMode = "nftables"
	ModeUserspace ProxyMode = "userspace"
)

type Proxy struct {
	store       store.Store
	mode        ProxyMode
	clusterCIDR string
	services    map[string]*ServiceProxy
	mu          sync.RWMutex
	logger      *slog.Logger
}

type ServiceProxy struct {
	Name      string
	Namespace string
	ClusterIP string
	Ports     []k8s.ServicePort
	Endpoints []EndpointEntry
}

type EndpointEntry struct {
	IP   string
	Port int32
}

func NewProxy(s store.Store, mode ProxyMode, clusterCIDR string, logger *slog.Logger) *Proxy {
	return &Proxy{
		store:       s,
		mode:        mode,
		clusterCIDR: clusterCIDR,
		services:    make(map[string]*ServiceProxy),
		logger:      logger,
	}
}

func (p *Proxy) Run(ctx context.Context) error {
	go p.watchServices(ctx)
	go p.watchEndpoints(ctx)
	p.logger.Info("kube-proxy started", "mode", p.mode)
	<-ctx.Done()
	return nil
}

func (p *Proxy) watchServices(ctx context.Context) {
	prefix := store.KeyFor("", "services", "", "")
	ch, err := p.store.Watch(prefix, 0)
	if err != nil {
		p.logger.Error("watch services failed", "error", err)
		return
	}
	defer p.store.Unwatch(ch)

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
			p.syncService(&svc, event.Type)
		}
	}
}

func (p *Proxy) watchEndpoints(ctx context.Context) {
	prefix := store.KeyFor("", "endpoints", "", "")
	ch, err := p.store.Watch(prefix, 0)
	if err != nil {
		p.logger.Error("watch endpoints fails", "error", err)
		return
	}
	defer p.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			var ep k8s.Endpoints
			if err := json.Unmarshal(event.Object.Value, &ep); err != nil {
				continue
			}
			p.syncEndpoints(&ep)
		}
	}
}

func (p *Proxy) syncService(svc *k8s.Service, eventType string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := svc.Namespace + "/" + svc.Name

	if eventType == store.EventDeleted {
		delete(p.services, key)
		p.logger.Info("service removed", "service", key)
		return
	}

	sp := &ServiceProxy{
		Name:      svc.Name,
		Namespace: svc.Namespace,
		ClusterIP: svc.Spec.ClusterIP,
		Ports:     svc.Spec.Ports,
	}

	p.services[key] = sp
	p.syncRules()
	p.logger.Info("service synced", "service", key, "clusterIP", svc.Spec.ClusterIP)
}

func (p *Proxy) syncEndpoints(ep *k8s.Endpoints) {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := ep.Namespace + "/" + ep.Name
	sp, ok := p.services[key]
	if !ok {
		return
	}

	sp.Endpoints = nil
	for _, subset := range ep.Subsets {
		for _, addr := range subset.Addresses {
			for _, port := range subset.Ports {
				sp.Endpoints = append(sp.Endpoints, EndpointEntry{
					IP:   addr.IP,
					Port: port.Port,
				})
			}
		}
	}
	p.syncRules()
}

func (p *Proxy) syncRules() {
	switch p.mode {
	case ModeIPTables:
		p.syncIPTables()
	case ModeNFTables:
		p.syncNFTables()
	case ModeUserspace:
		p.syncUserspace()
	}
}

func (p *Proxy) syncIPTables() {
	for _, sp := range p.services {
		if sp.ClusterIP == "" || sp.ClusterIP == "None" {
			continue
		}
		for _, port := range sp.Ports {
			_ = fmt.Sprintf("-A DOKI-SERVICES -d %s/32 -p %s -m %s --dport %d -j DOKI-SVC-%s",
				sp.ClusterIP, port.Protocol, port.Protocol, port.Port, sp.Name)
		}
	}
}

func (p *Proxy) syncNFTables() {
}

func (p *Proxy) syncUserspace() {
}
