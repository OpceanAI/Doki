package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	r "runtime"
	"strings"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/internal/dokivm"
	"github.com/OpceanAI/Doki/pkg/api"
	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	dr "github.com/OpceanAI/Doki/pkg/runtime"
	"github.com/OpceanAI/Doki/pkg/storage"
)

var (
	tlsEnabled  bool
	tlsCertFile string
	tlsKeyFile  string
	tlsCAFile   string
	tlsVerify   bool
	socketPath  string
	tcpAddr     string
	Version     = "0.3.0"
	GitCommit   = "unknown"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("Doki Daemon v%s starting...", Version)
	log.Printf("Go %s / %s %s", r.Version(), r.GOOS, r.GOARCH)

	cfg := loadConfig()
	dataDir := cfg.DataDir
	execRoot := cfg.ExecRoot

	for _, dir := range []string{
		dataDir, execRoot,
		filepath.Join(dataDir, "images"),
		filepath.Join(dataDir, "containers"),
		filepath.Join(dataDir, "volumes"),
		filepath.Join(dataDir, "networks"),
		filepath.Join(dataDir, "layers"),
		filepath.Join(dataDir, "rootfs"),
		filepath.Join(dataDir, "tmp"),
	} {
		common.EnsureDir(dir)
	}

	storeMgr, err := storage.NewManager(dataDir, cfg.StorageDriver)
	if err != nil {
		log.Fatalf("Storage: %v", err)
	}
	log.Printf("Storage driver: %s", storeMgr.Name())

	gc := storage.NewGarbageCollector(storeMgr, storage.GCConfig{
		Enabled: true, Interval: 1 * time.Hour, MaxAge: 72 * time.Hour,
	})
	gc.Start()
	defer gc.Stop()

	imgStore, err := image.NewStore(filepath.Join(dataDir, "images"))
	if err != nil {
		log.Fatalf("Image store: %v", err)
	}

	netMgr, err := network.NewManager(filepath.Join(dataDir, "networks"))
	if err != nil {
		log.Fatalf("Network: %v", err)
	}
	log.Printf("Firewall backend: %s", network.DetectFirewallBackend())

	rt := dr.NewRuntime(execRoot, storeMgr)
	log.Printf("Runtime mode: %s", modeString(rt.Mode()))

	server := api.NewServer(cfg, rt, imgStore, netMgr)
	server.RegisterHandler("/metrics", http.HandlerFunc(api.MetricsHandler))
	server.RegisterHandler("/health", http.HandlerFunc(api.HealthHandler))

	mw := api.NewMiddleware()
	server.SetMiddleware(mw.Logging, mw.CORS, mw.Recovery, mw.RequestID)
	rateLimiter := api.NewRateLimit(100, 200)
	defer rateLimiter.Stop()
	server.SetMiddleware(rateLimiter.RateLimitMiddleware)

	var listeners []net.Listener
	os.Remove(socketPath)
	unixLn, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Unix socket %s: %v", socketPath, err)
	}
	listeners = append(listeners, unixLn)
	log.Printf("Listening on unix://%s", socketPath)

	if tcpAddr != "" {
		tcpLn, err := net.Listen("tcp", tcpAddr)
		if err != nil {
			log.Printf("TCP %s: %v", tcpAddr, err)
		} else {
			listeners = append(listeners, tcpLn)
			log.Printf("Listening on tcp://%s", tcpAddr)
		}
	}

	if tlsEnabled {
		tlsCfg, err := api.NewTLSConfig(&api.TLSConfig{
			Enabled: true, CertFile: tlsCertFile, KeyFile: tlsKeyFile,
			CAFile: tlsCAFile, Verify: tlsVerify, MinTLS: tls.VersionTLS12,
		})
		if err != nil {
			log.Fatalf("TLS: %v", err)
		}
		for i, ln := range listeners {
			listeners[i] = tls.NewListener(ln, tlsCfg)
		}
		log.Printf("TLS enabled (mutual=%v)", tlsVerify)
	}

	srv := &http.Server{
		Handler:      server,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	for _, ln := range listeners {
		go func(l net.Listener) {
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("Serve error: %v", err)
			}
		}(ln)
	}

	log.Printf("Doki daemon v%s ready (API v%s)", Version, common.DokiAPIVersion)
	log.Printf("Mode: %s | Images: %d", modeString(rt.Mode()), countImages(imgStore))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for s := range sig {
		switch s {
		case syscall.SIGHUP:
			log.Println("SIGHUP - reloading config")
		case syscall.SIGINT, syscall.SIGTERM:
			log.Println("Shutting down...")
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			srv.Shutdown(ctx)
			storeMgr.Cleanup()
			log.Println("Doki daemon stopped")
			os.Exit(0)
		}
	}
}

func loadConfig() *common.DokiConfig {
	cfg := common.DefaultConfig()
	if s := socketPath; s != "" {
		cfg.SocketPath = s
	}
	if dataDir := os.Getenv("DOKI_DATA_DIR"); dataDir != "" {
		cfg.DataDir = dataDir
		cfg.ExecRoot = filepath.Join(dataDir, "runtimes")
	}
	if drv := os.Getenv("DOKI_STORAGE_DRIVER"); drv != "" {
		cfg.StorageDriver = drv
	}
	if loaded, err := common.LoadConfig(); err == nil {
		if loaded.StorageDriver != "" {
			cfg.StorageDriver = loaded.StorageDriver
		}
		if loaded.LogLevel != "" {
			cfg.LogLevel = loaded.LogLevel
		}
		if len(loaded.DNS) > 0 {
			cfg.DNS = loaded.DNS
		}
	}
	return cfg
}

func modeString(m dr.ExecutionMode) string {
	switch m {
	case dr.ModeMicroVM:
		info := dokivm.DetectHypervisor()
		return fmt.Sprintf("microVM (%s via %s)", info.Backend, info.Type)
	case dr.ModeNative:
		return "native (host)"
	case dr.ModeProot:
		return "proot"
	case dr.ModeNamespaces:
		return "namespaces (root)"
	}
	return "unknown"
}

func countImages(imgStore *image.Store) int {
	images, _ := imgStore.List()
	return len(images)
}

func init() {
	if s := os.Getenv("DOKI_SOCKET"); s != "" || os.Getenv("DOCKER_HOST") != "" {
		if s == "" {
			s = os.Getenv("DOCKER_HOST")
			s = strings.TrimPrefix(s, "unix://")
		}
		socketPath = s
	} else if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
		socketPath = "/data/data/com.termux/files/usr/var/run/doki.sock"
	} else {
		socketPath = filepath.Join(os.TempDir(), "doki.sock")
	}
	if s := os.Getenv("DOKI_TCP_ADDR"); s != "" {
		tcpAddr = s
	}
	if os.Getenv("DOKI_TLS") == "1" {
		tlsEnabled = true
		tlsCertFile = os.Getenv("DOKI_TLS_CERT")
		tlsKeyFile = os.Getenv("DOKI_TLS_KEY")
		tlsCAFile = os.Getenv("DOKI_TLS_CA")
		if os.Getenv("DOKI_TLS_VERIFY") == "1" {
			tlsVerify = true
		}
	}
	common.Version = Version
	common.GitCommit = GitCommit
	_ = json.Marshal
}
