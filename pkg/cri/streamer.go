package cri

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// defaultStreamer is the built-in streaming server that returns a
// local URL for exec/attach/portforward. It implements the Streamer
// interface defined in server.go.
type defaultStreamer struct {
	mu     sync.Mutex
	tokens map[string]*streamEntry
	nextID atomic.Uint64
	host   string
	port   int
}

type streamEntry struct {
	resourceID string
	params     map[string]string
	ctx        context.Context
}

func newDefaultStreamer() *defaultStreamer {
	return &defaultStreamer{
		tokens: make(map[string]*streamEntry),
		host:   "127.0.0.1",
	}
}

func (s *defaultStreamer) Reserve(ctx context.Context, engine, resourceID string, params map[string]string) (int, error) {
	token := fmt.Sprintf("%x", s.nextID.Add(1))
	s.mu.Lock()
	s.tokens[token] = &streamEntry{
		resourceID: resourceID,
		params:     params,
		ctx:        ctx,
	}
	s.mu.Unlock()
	return s.port, nil
}

func (s *defaultStreamer) Addr() (string, int) {
	return s.host, s.port
}

func (s *defaultStreamer) start() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("streamer listen: %w", err)
	}
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	s.host = "127.0.0.1"
	if portStr != "" {
		if p, pe := fmt.Sscanf(portStr, "%d", &s.port); pe != nil || p != 1 {
			// parse failed, use 0
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.serveHTTP)
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(listener) }()
	return nil
}

func (s *defaultStreamer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	token := strings.TrimSuffix(path, "/")
	s.mu.Lock()
	_, ok := s.tokens[token]
	if ok {
		delete(s.tokens, token)
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "unknown streaming token", http.StatusNotFound)
		return
	}
	// For kubernetes streaming, the kubelet dials this URL and
	// upgrades to WebSocket or SPDY. Here we return 200 with a JSON
	// payload informing the caller that the main API server handles
	// the actual attach.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `{"status":"ok","token":"%s"}`+"\n", token)
}
