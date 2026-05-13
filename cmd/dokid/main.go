package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	r "runtime"
	"strconv"
	"strings"
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
	tlsEnabled     bool
	tlsCertFile    string
	tlsKeyFile     string
	tlsCAFile      string
	tlsVerify      bool
	tlsAutoCert    bool
	socketPath     string
	tcpAddr        string
	configPath     string
	logLevel       string
	debugMode      bool
	rateLimitPerSec float64
	rateLimitBurst  int
	Version        = "0.3.0"
	GitCommit      = "unknown"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	setLogLevel(logLevel)
	log.Printf("Doki Daemon v%s starting...", Version)
	log.Printf("Go %s / %s %s", r.Version(), r.GOOS, r.GOARCH)

	rotateDaemonLog()

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

	netMgr, err := network.NewManager(
		filepath.Join(dataDir, "networks"),
		network.NewFirewallManager(network.DetectFirewallBackend()),
		network.NewDNSServer(),
	)
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
	rateLimiter := api.NewRateLimit(rateLimitPerSec, rateLimitBurst)
	defer rateLimiter.Stop()
	server.SetMiddleware(rateLimiter.RateLimitMiddleware)
	log.Printf("Rate limiter: %.0f req/s, burst %d", rateLimitPerSec, rateLimitBurst)

	if debugMode {
		go startPprofServer(6060)
	}

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
		if tlsAutoCert && (tlsCertFile == "" || tlsKeyFile == "") {
			certDir := filepath.Join(dataDir, "tls")
			common.EnsureDir(certDir)
			tlsCertFile = filepath.Join(certDir, "cert.pem")
			tlsKeyFile = filepath.Join(certDir, "key.pem")
			if !common.PathExists(tlsCertFile) || !common.PathExists(tlsKeyFile) {
				if err := api.GenerateSelfSignedCert(tlsCertFile, tlsKeyFile); err != nil {
					log.Printf("WARNING: auto TLS cert generation failed: %v", err)
				} else {
					log.Printf("Auto-generated self-signed TLS cert: %s", tlsCertFile)
				}
			}
		}
		tlsCfg, err := api.NewTLSConfig(&api.TLSConfig{
			Enabled: true, CertFile: tlsCertFile, KeyFile: tlsKeyFile,
			CAFile: tlsCAFile, Verify: tlsVerify, MinTLS: tls.VersionTLS12,
		})
		if err != nil {
			log.Fatalf("TLS: %v", err)
		}
		for i, ln := range listeners {
			listeners[i] = api.TLSListener(ln, tlsCfg)
		}
		log.Printf("TLS enabled (mutual=%v)", tlsVerify)
	}

	// AG7: Recover container state on startup.
	recoverContainers(rt, dataDir, imgStore, netMgr)

	srv := &http.Server{
		Handler:      server,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
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

	api.GracefulShutdown(context.Background(), srv, 30*time.Second)
}

func loadConfig() *common.DokiConfig {
	cfg := common.DefaultConfig()
	if s := socketPath; s != "" {
		cfg.SocketPath = s
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if dataDir := os.Getenv("DOKI_DATA_DIR"); dataDir != "" {
		cfg.DataDir = dataDir
		cfg.ExecRoot = filepath.Join(dataDir, "runtimes")
	}
	if drv := os.Getenv("DOKI_STORAGE_DRIVER"); drv != "" {
		cfg.StorageDriver = drv
	}
	if configPath != "" {
		if loaded, err := common.LoadConfig(); err == nil {
			applyLoadedConfig(cfg, loaded)
		}
	} else if loaded, err := common.LoadConfig(); err == nil {
		applyLoadedConfig(cfg, loaded)
	}
	return cfg
}

func applyLoadedConfig(cfg, loaded *common.DokiConfig) {
	if loaded.StorageDriver != "" {
		cfg.StorageDriver = loaded.StorageDriver
	}
	if loaded.LogLevel != "" {
		cfg.LogLevel = loaded.LogLevel
	}
	if len(loaded.DNS) > 0 {
		cfg.DNS = loaded.DNS
	}
	if loaded.DataDir != "" {
		cfg.DataDir = loaded.DataDir
		cfg.ExecRoot = filepath.Join(loaded.DataDir, "runtimes")
	}
	if loaded.Debug {
		cfg.Debug = true
	}
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

func rotateDaemonLog() {
	logPath := "dokid.log"
	fi, err := os.Stat(logPath)
	if err != nil || fi.Size() < 10*1024*1024 {
		return
	}
	for i := 3; i >= 1; i-- {
		oldPath := logPath + "." + strconv.Itoa(i)
		newPath := logPath + "." + strconv.Itoa(i+1)
		if i == 3 {
			os.Remove(newPath)
		}
		os.Rename(oldPath, newPath)
	}
	os.Rename(logPath, logPath+".1")
}

func setLogLevel(level string) {
	switch strings.ToLower(level) {
	case "debug":
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	case "warn", "warning":
		log.SetFlags(log.Ldate | log.Ltime)
	case "error":
		log.SetFlags(log.Ldate | log.Ltime)
		log.SetOutput(os.Stderr)
	default:
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	}
}

func startPprofServer(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	addr := fmt.Sprintf(":%d", port)
	log.Printf("Debug pprof server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("Pprof server: %v", err)
	}
}

func init() {
	flag.StringVar(&socketPath, "socket", "", "Unix socket path")
	flag.StringVar(&tcpAddr, "tcp", "", "TCP listen address")
	flag.StringVar(&configPath, "config", "", "Config file path")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug/info/warn/error)")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug mode (pprof on :6060)")
	flag.BoolVar(&tlsEnabled, "tls", false, "Enable TLS")
	flag.StringVar(&tlsCertFile, "tls-cert", "", "TLS certificate path")
	flag.StringVar(&tlsKeyFile, "tls-key", "", "TLS key path")
	flag.StringVar(&tlsCAFile, "tls-ca", "", "TLS CA certificate path")
	flag.BoolVar(&tlsVerify, "tls-verify", false, "Verify client certificates")
	flag.Float64Var(&rateLimitPerSec, "rate-limit", 100, "Rate limit requests per second")
	flag.IntVar(&rateLimitBurst, "rate-burst", 200, "Rate limit burst size")
	flag.Parse()

	if s := os.Getenv("DOKI_SOCKET"); s != "" || os.Getenv("DOCKER_HOST") != "" {
		if s == "" {
			s = os.Getenv("DOCKER_HOST")
			s = strings.TrimPrefix(s, "unix://")
		}
		socketPath = s
	} else if socketPath == "" {
		if _, err := os.Stat("/data/data/com.termux/files/usr"); err == nil {
			socketPath = "/data/data/com.termux/files/usr/var/run/doki.sock"
		} else {
			socketPath = filepath.Join(os.TempDir(), "doki.sock")
		}
	}
	if s := os.Getenv("DOKI_TCP_ADDR"); s != "" && tcpAddr == "" {
		tcpAddr = s
	}
	if os.Getenv("DOKI_TLS") == "1" && !tlsEnabled {
		tlsEnabled = true
		tlsCertFile = os.Getenv("DOKI_TLS_CERT")
		tlsKeyFile = os.Getenv("DOKI_TLS_KEY")
		tlsCAFile = os.Getenv("DOKI_TLS_CA")
		if os.Getenv("DOKI_TLS_VERIFY") == "1" {
			tlsVerify = true
		}
		if os.Getenv("DOKI_TLS_AUTO_CERT") != "0" {
			tlsAutoCert = true
		}
	}
	if os.Getenv("DOKI_DEBUG") == "1" {
		debugMode = true
	}
	if s := os.Getenv("DOKI_RATE_LIMIT"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			rateLimitPerSec = v
		}
	}
	common.Version = Version
	common.GitCommit = GitCommit
	_ = json.Marshal
}

// recoverContainers scans the containers directory and recovers state on startup.
func recoverContainers(rt *dr.Runtime, dataDir string, imgStore *image.Store, netMgr *network.Manager) {
	containerDir := filepath.Join(dataDir, "containers")
	entries, err := os.ReadDir(containerDir)
	if err != nil {
		return
	}
	recovered := 0
	dead := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		statePath := filepath.Join(containerDir, entry.Name(), "state.json")
		pidPath := filepath.Join(containerDir, entry.Name(), "init.pid")
		data, err := os.ReadFile(statePath)
		if err != nil {
			continue
		}
		var state struct {
			ID     string `json:"id"`
			Pid    int    `json:"pid"`
			Status string `json:"status"`
		}
		if json.Unmarshal(data, &state) != nil {
			continue
		}
		if state.Status != "running" {
			continue
		}
		if pidData, err := os.ReadFile(pidPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil && pid > 0 {
				state.Pid = pid
			}
		}
		if state.Pid > 0 && processExists(state.Pid) {
			recovered++
			log.Printf("Recovered container %s (pid=%d)", common.ShortID(state.ID), state.Pid)
		} else {
			dead++
			log.Printf("Container %s is dead (pid=%d not found), marking as exited", common.ShortID(state.ID), state.Pid)
			if st, err := rt.State(state.ID); err == nil && st != nil {
				rt.Stop(state.ID, 0)
			}
		}
	}
	if recovered > 0 || dead > 0 {
		log.Printf("State recovery: %d recovered, %d dead", recovered, dead)
	}
}

func processExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}
