// Package main is the Doki daemon.
// Doki daemon entry point.
//
// Responsibilities:
//   - parse flags and env
//   - structured logging (slog JSON in prod, text in dev)
//   - data dir bootstrap
//   - storage / image / network / runtime init
//   - HTTP API server (Docker-compatible API)
//   - graceful shutdown
//   - state recovery on restart
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
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
	"github.com/OpceanAI/Doki/pkg/netlink"
	"github.com/OpceanAI/Doki/pkg/network"
	dr "github.com/OpceanAI/Doki/pkg/runtime"
	runners_chroot "github.com/OpceanAI/Doki/pkg/runtime/runners/chroot"
	runners_fex "github.com/OpceanAI/Doki/pkg/runtime/runners/fex"
	runners_gvisor "github.com/OpceanAI/Doki/pkg/runtime/runners/gvisor"
	runners_legacy32 "github.com/OpceanAI/Doki/pkg/runtime/runners/legacy32"
	runners_microvm "github.com/OpceanAI/Doki/pkg/runtime/runners/microvm"
	runners_namespaces "github.com/OpceanAI/Doki/pkg/runtime/runners/namespaces"
	runners_native "github.com/OpceanAI/Doki/pkg/runtime/runners/native"
	runners_pkdroid "github.com/OpceanAI/Doki/pkg/runtime/runners/pkdroid"
	runners_proot "github.com/OpceanAI/Doki/pkg/runtime/runners/proot"
	runners_qemuuser "github.com/OpceanAI/Doki/pkg/runtime/runners/qemuuser"
	runners_sysbox "github.com/OpceanAI/Doki/pkg/runtime/runners/sysbox"
	runners_wasm "github.com/OpceanAI/Doki/pkg/runtime/runners/wasm"
	"github.com/OpceanAI/Doki/pkg/storage"
)

var (
	tlsEnabled      bool
	tlsCertFile     string
	tlsKeyFile      string
	tlsCAFile       string
	tlsVerify       bool
	tlsAutoCert     bool
	socketPath      string
	tcpAddr         string
	configPath      string
	logLevel        string
	logFormat       string
	debugMode       bool
	rateLimitPerSec float64
	rateLimitBurst  int
	dnsListen       string
	showVersion     bool
)

func init() {
	if r.GOOS == "android" {
		dnsListen = "127.0.0.11:8053"
	} else {
		dnsListen = "127.0.0.11:53"
	}
}

// rootCtx is cancelled on signal for graceful shutdown.
var rootCtx, rootCancel = context.WithCancel(context.Background())

func main() {
	flag.StringVar(&socketPath, "socket", "", "Unix socket path")
	flag.StringVar(&tcpAddr, "tcp", "", "TCP listen address")
	flag.StringVar(&configPath, "config", "", "Config file path")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug/info/warn/error)")
	flag.StringVar(&logFormat, "log-format", "auto", "Log format: json|text|auto")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug mode (pprof on :6060)")
	flag.BoolVar(&tlsEnabled, "tls", false, "Enable TLS")
	flag.StringVar(&tlsCertFile, "tls-cert", "", "TLS certificate path")
	flag.StringVar(&tlsKeyFile, "tls-key", "", "TLS key path")
	flag.StringVar(&tlsCAFile, "tls-ca", "", "TLS CA certificate path")
	flag.BoolVar(&tlsVerify, "tls-verify", false, "Verify client certificates")
	flag.Float64Var(&rateLimitPerSec, "rate-limit", 100, "Rate limit requests per second")
	flag.IntVar(&rateLimitBurst, "rate-burst", 200, "Rate limit burst size")
	flag.StringVar(&dnsListen, "dns-listen", dnsListen, "DNS server listen address (default: 127.0.0.11:8053 on Android, 127.0.0.11:53 on Linux)")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Parse()

	if showVersion {
		fmt.Printf("dokid version %s (API %s, min API %s)\n", common.Version, common.DokiAPIVersion, common.DokiMinClient)
		os.Exit(0)
	}

	applyEnvOverrides()

	logger := newLogger()
	slog.SetDefault(logger)
	api.SetLogger(logger)

	logger.Info("dokid starting",
		"version", common.Version,
		"commit", common.GitCommit,
		"api", common.DokiAPIVersion,
		"min_api", common.DokiMinClient,
		"build_date", common.BuildDate,
		"go", r.Version(),
		"goos", r.GOOS,
		"goarch", r.GOARCH,
		"pid", os.Getpid(),
	)

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
		if err := common.EnsureDir(dir); err != nil {
			logger.Error("ensure dir", "path", dir, "err", err)
			os.Exit(1)
		}
	}

	storeMgr, err := storage.NewManager(dataDir, cfg.StorageDriver)
	if err != nil {
		logger.Error("storage init", "driver", cfg.StorageDriver, "err", err)
		os.Exit(1)
	}
	logger.Info("storage driver ready", "name", storeMgr.Name())

	gc := storage.NewGarbageCollector(storeMgr, storage.GCConfig{
		Enabled: true, Interval: 1 * time.Hour, MaxAge: 72 * time.Hour,
	})
	gc.Start()
	defer gc.Stop()

	imgStore, err := image.NewStore(filepath.Join(dataDir, "images"))
	if err != nil {
		logger.Error("image store", "err", err)
		os.Exit(1)
	}

	// DNS server is created with a real listen address and started here
	// (Phase 0 fix: previously NewDNSServer was called but Start() was never
	// invoked, so containers resolved external names against the host's
	// resolver which is unreachable from inside a network namespace).
	dnsServer := network.NewDNSServer(cfg.DNS)
	dnsReady := false
	if err := dnsServer.Start(dnsListen); err != nil {
		logger.Warn("dns server start failed, continuing without DNS", "listen", dnsListen, "err", err)
	} else {
		dnsReady = true
		defer dnsServer.Stop()
		logger.Info("dns server ready",
			"listen", dnsServer.Addr(),
			"upstream", cfg.DNS,
			"cache_capacity", dnsServer.CacheCapacity(),
		)
	}

	netMgr, err := network.NewManager(
		filepath.Join(dataDir, "networks"),
		network.NewFirewallManager(network.DetectFirewallBackend()),
		dnsServer,
	)
	if err != nil {
		logger.Error("network manager", "err", err)
		os.Exit(1)
	}
	logger.Info("firewall backend", "name", network.DetectFirewallBackend())

	// DokiLink-Lite mesh: load or generate install identity and start
	// the gossip listener. The mesh is opt-out: setting
	// DOKI_LINK_MESH=0 disables it entirely (e.g. on air-gapped hosts).
	meshStop := func() {}
	if os.Getenv("DOKI_LINK_MESH") != "0" {
		keysDir := filepath.Join(dataDir, "keys")
		identity, idErr := netlink.NewIdentity(keysDir)
		if idErr != nil {
			logger.Warn("doki-link: identity init", "err", idErr)
		} else {
			trustDir := filepath.Join(dataDir, "trust")
			trust, _ := netlink.NewTrustStore(trustDir)
			if err := trust.Load(); err != nil {
				logger.Warn("doki-link: trust load", "err", err)
			}
			peersPath := filepath.Join(dataDir, "mesh", "peers.json")
			sp, _ := netlink.NewStaticPeers(peersPath)
			mesh, meshErr := netlink.NewMesh(netlink.MeshConfig{
				Identity:   identity,
				Trust:      trust,
				Static:     sp,
				ListenAddr: meshListenAddr(),
				Logger:     logger,
			})
			if meshErr != nil {
				logger.Warn("doki-link: mesh init", "err", meshErr)
			} else {
				ctx, cancel := context.WithCancel(context.Background())
				if startErr := mesh.Start(ctx); startErr != nil {
					logger.Warn("doki-link: mesh start", "err", startErr)
					cancel()
				} else {
					logger.Info("doki-link ready",
						"install_id", identity.ShortID(),
						"listen", meshListenAddr(),
					)
					meshStop = func() {
						_ = mesh.Stop()
						cancel()
					}
				}
			}
		}
	}
	defer meshStop()

	// Create runner registry and register all available runners.
	registry := dr.NewRegistry()
	registry.Register(runners_native.New(execRoot))
	registry.Register(runners_proot.New(execRoot))
	registry.Register(runners_namespaces.New(execRoot))
	registry.Register(runners_microvm.New(execRoot))
	registry.Register(runners_gvisor.New(execRoot))
	registry.Register(runners_wasm.New(execRoot))
	registry.Register(runners_pkdroid.New(execRoot))
	registry.Register(runners_sysbox.New(execRoot))
	registry.Register(runners_qemuuser.New(execRoot))
	registry.Register(runners_chroot.New(execRoot))
	registry.Register(runners_fex.New(execRoot))
	registry.Register(runners_legacy32.New(execRoot))

	dnsAddr := ""
	if dnsReady {
		dnsAddr = dnsServer.Addr()
	}
	rt := dr.NewRuntime(execRoot, storeMgr,
		dr.WithRegistry(registry),
		dr.WithDNSAddr(dnsAddr),
	)
	logger.Info("runtime mode", "mode", modeString(rt.Mode()))
	logger.Info("available runtimes", "count", len(registry.Available()))
	logger.Info("container DNS", "addr", dnsAddr)

	server, err := api.NewServer(cfg, rt, imgStore, netMgr)
	if err != nil {
		logger.Error("failed to create API server", "err", err)
		os.Exit(1)
	}

	mw := api.NewMiddleware()
	rateLimiter := api.NewRateLimit(rateLimitPerSec, rateLimitBurst)
	defer rateLimiter.Stop()
	server.SetMiddleware(
		mw.RequestID,
		mw.Recovery,
		mw.CORS,
		rateLimiter.RateLimitMiddleware,
		mw.Logging,
	)
	logger.Info("rate limiter", "req_per_sec", rateLimitPerSec, "burst", rateLimitBurst)

	if debugMode {
		go startPprofServer(6060)
	}

	listeners, err := buildListeners(socketPath, tcpAddr)
	if err != nil {
		logger.Error("listeners", "err", err)
		os.Exit(1)
	}
	for _, ln := range listeners {
		logger.Info("listener ready", "addr", ln.Addr().String(), "network", ln.Addr().Network())
	}

	if tlsEnabled {
		if tlsAutoCert && (tlsCertFile == "" || tlsKeyFile == "") {
			certDir := filepath.Join(dataDir, "tls")
			if err := common.EnsureDir(certDir); err != nil {
				logger.Error("tls dir", "err", err)
				os.Exit(1)
			}
			tlsCertFile = filepath.Join(certDir, "cert.pem")
			tlsKeyFile = filepath.Join(certDir, "key.pem")
			if !common.PathExists(tlsCertFile) || !common.PathExists(tlsKeyFile) {
				if err := api.GenerateSelfSignedCert(tlsCertFile, tlsKeyFile); err != nil {
					logger.Warn("auto TLS cert generation failed", "err", err)
				} else {
					logger.Info("auto-generated self-signed TLS cert", "path", tlsCertFile)
				}
			}
		}
		tlsCfg, err := api.NewTLSConfig(&api.TLSConfig{
			Enabled: true, CertFile: tlsCertFile, KeyFile: tlsKeyFile,
			CAFile: tlsCAFile, Verify: tlsVerify, MinTLS: tls.VersionTLS12,
		})
		if err != nil {
			logger.Error("tls config", "err", err)
			os.Exit(1)
		}
		for i, ln := range listeners {
			listeners[i] = api.TLSListener(ln, tlsCfg)
		}
		logger.Info("tls enabled", "mutual", tlsVerify)
	}

	// AG7: Recover container state on startup.
	recoverContainers(logger, rt, dataDir, imgStore, netMgr)

	srv := &http.Server{
		Handler:      server,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	for _, ln := range listeners {
		go func(l net.Listener) {
			if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
				logger.Error("serve", "addr", l.Addr().String(), "err", err)
			}
		}(ln)
	}

	logger.Info("dokid ready",
		"version", common.Version,
		"api", common.DokiAPIVersion,
		"mode", modeString(rt.Mode()),
		"images", countImages(imgStore),
	)

	go api.WaitForSignal(rootCancel)
	<-rootCtx.Done()
	logger.Info("shutdown signal received")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("http shutdown", "err", err)
	}
	rootCancel()
	logger.Info("dokid stopped")
}

// newLogger builds the structured logger based on -log-level and -log-format.
// In auto mode: text on TTY (for dev), JSON otherwise (for prod / container logs).
func newLogger() *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(logLevel) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	opts := &slog.HandlerOptions{Level: lvl, AddSource: debugMode}

	format := strings.ToLower(logFormat)
	if format == "auto" {
		if isTerminal(os.Stderr) {
			format = "text"
		} else {
			format = "json"
		}
	}
	var h slog.Handler
	switch format {
	case "json":
		h = slog.NewJSONHandler(os.Stderr, opts)
	default:
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	return slog.New(h).With("app", "dokid", "component", "main")
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func applyEnvOverrides() {
	if socketPath == "" {
		if s := os.Getenv("DOKI_SOCKET"); s != "" {
			socketPath = s
		} else if h := os.Getenv("DOCKER_HOST"); h != "" {
			socketPath = strings.TrimPrefix(h, "unix://")
		} else {
			socketPath = common.DefaultDaemonSocket()
		}
	}
	if tcpAddr == "" {
		if s := os.Getenv("DOKI_TCP_ADDR"); s != "" {
			tcpAddr = s
		}
	}
	if !tlsEnabled && os.Getenv("DOKI_TLS") == "1" {
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
	if !debugMode && os.Getenv("DOKI_DEBUG") == "1" {
		debugMode = true
	}
	if s := os.Getenv("DOKI_RATE_LIMIT"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			rateLimitPerSec = v
		}
	}
	if s := os.Getenv("DOKI_LOG_FORMAT"); s != "" && logFormat == "auto" {
		logFormat = s
	}
	if s := os.Getenv("DOKI_LOG_LEVEL"); s != "" {
		logLevel = s
	}
	if s := os.Getenv("DOKI_DNS_LISTEN"); s != "" {
		dnsListen = s
	}
}

func loadConfig() *common.DokiConfig {
	cfg := common.DefaultConfig()
	if loaded, err := common.LoadConfig(); err == nil {
		applyLoadedConfig(cfg, loaded)
	}
	if configPath != "" {
		if loaded, err := common.LoadConfigFrom(configPath); err == nil {
			applyLoadedConfig(cfg, loaded)
		}
	}
	applyConfigOverrides(cfg)
	return cfg
}

func applyConfigOverrides(cfg *common.DokiConfig) {
	if s := socketPath; s != "" {
		cfg.SocketPath = s
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if dataDir := os.Getenv("DOKI_DATA_DIR"); dataDir != "" {
		cfg.DataDir = dataDir
		cfg.Root = dataDir
		cfg.ExecRoot = filepath.Join(dataDir, "runtimes")
	}
	if drv := os.Getenv("DOKI_STORAGE_DRIVER"); drv != "" {
		cfg.StorageDriver = drv
	}
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

func buildListeners(unixPath, tcp string) ([]net.Listener, error) {
	var out []net.Listener
	if err := os.Remove(unixPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket %s: %w", unixPath, err)
	}
	ln, err := net.Listen("unix", unixPath)
	if err != nil {
		return nil, fmt.Errorf("listen unix %s: %w", unixPath, err)
	}
	if err := os.Chmod(unixPath, 0660); err != nil {
		// best effort
		slog.Default().Warn("chmod unix socket", "path", unixPath, "err", err)
	}
	out = append(out, ln)
	if tcp != "" {
		tcpLn, err := net.Listen("tcp", tcp)
		if err != nil {
			return nil, fmt.Errorf("listen tcp %s: %w", tcp, err)
		}
		out = append(out, tcpLn)
	}
	return out, nil
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
			_ = os.Remove(newPath)
		}
		_ = os.Rename(oldPath, newPath)
	}
	_ = os.Rename(logPath, logPath+".1")
}

func startPprofServer(port int) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	addr := fmt.Sprintf(":%d", port)
	slog.Default().Info("pprof server listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Default().Error("pprof server", "err", err)
	}
}

// recoverContainers scans the containers directory and reconciles in-memory
// state with what's actually running. Running PIDs are re-registered; missing
// PIDs are marked as exited.
func recoverContainers(logger *slog.Logger, rt *dr.Runtime, dataDir string, _ *image.Store, netMgr *network.Manager) {
	containerDir := filepath.Join(dataDir, "containers")
	entries, err := os.ReadDir(containerDir)
	if err != nil {
		return
	}
	recovered, dead := 0, 0
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
		var st struct {
			ID     string `json:"id"`
			Pid    int    `json:"pid"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(data, &st); err != nil {
			continue
		}
		if st.Status != "running" {
			continue
		}
		if pidData, err := os.ReadFile(pidPath); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil && pid > 0 {
				st.Pid = pid
			}
		}
		if st.Pid > 0 && processExists(st.Pid) {
			recovered++
			logger.Info("container recovered", "id", common.ShortID(st.ID), "pid", st.Pid)
			netMgr.ReRegisterDNS(st.ID)
		} else {
			dead++
			logger.Warn("container dead, marking exited", "id", common.ShortID(st.ID), "pid", st.Pid)
			if stt, err := rt.State(st.ID); err == nil && stt != nil {
				_ = rt.Stop(st.ID, 0)
			}
		}
	}
	if recovered > 0 || dead > 0 {
		logger.Info("state recovery complete", "recovered", recovered, "dead", dead)
	}
}

func processExists(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

// meshListenAddr returns the address the DokiLink gossip listener
// binds to. DOKI_LINK_ADDR overrides the default of ":7432". On
// Android/Termux the listener must be loopback-only (no raw socket
// access) so we use 127.0.0.1:7432.
func meshListenAddr() string {
	if a := os.Getenv("DOKI_LINK_ADDR"); a != "" {
		return a
	}
	if common.IsTermux() || common.IsProotMode() {
		return "127.0.0.1:7432"
	}
	return ":7432"
}
