package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/OpceanAI/Doki/pkg/apiserver"
	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/controllers"
	"github.com/OpceanAI/Doki/pkg/coredns"
	"github.com/OpceanAI/Doki/pkg/kubelet"
	"github.com/OpceanAI/Doki/pkg/kubeproxy"
	"github.com/OpceanAI/Doki/pkg/scheduler"
	"github.com/OpceanAI/Doki/pkg/store"
)

func main() {
	mode := flag.String("mode", "all", "Component: all, apiserver, kubelet, scheduler, controller, proxy, dns")
	apiAddr := flag.String("api-addr", ":6443", "API server listen address")
	nodeName := flag.String("node-name", "doki-node-1", "Node name")
	clusterDomain := flag.String("cluster-domain", "cluster.local", "Cluster DNS domain")
	clusterCIDR := flag.String("cluster-cidr", "10.244.0.0/16", "Cluster pod CIDR")
	serviceCIDR := flag.String("service-cidr", "10.96.0.0/12", "Cluster service CIDR")
	dnsAddr := flag.String("dns-addr", "10.96.0.10:53", "Cluster DNS listen address")
	storePath := flag.String("store-path", "", "State store path (empty = in-memory)")
	flag.Parse()

	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "-help" || os.Args[1] == "--help" || os.Args[1] == "help") {
		fmt.Printf("Usage: doki-kube [options]\nOptions:\n")
		flag.PrintDefaults()
		fmt.Printf("\nSubcommands:\n  help    Show this help\n  version Show version\n")
		os.Exit(0)
	}

	if flag.NArg() == 1 && flag.Arg(0) == "version" {
		fmt.Printf("doki-kube version %s (API %s)\n", common.DokiVersion, common.DokiAPIVersion)
		os.Exit(0)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var s store.Store
	if *storePath != "" {
		s = store.NewMemoryStore()
	} else {
		s = store.NewMemoryStore()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down")
		cancel()
		_ = s.Close()
	}()

	logger.Info("doki-kube starting",
		"version", common.DokiVersion,
		"mode", *mode,
		"node", *nodeName,
	)

	switch *mode {
	case "apiserver":
		api := apiserver.NewAPIServer(*apiAddr, s)
		logger.Info("apiserver listening", "addr", *apiAddr)
		if err := api.Start(); err != nil {
			logger.Error("apiserver failed", "error", err)
			os.Exit(1)
		}

	case "kubelet":
		kl := kubelet.NewKubelet(*nodeName, s, logger)
		if err := kl.Run(ctx); err != nil {
			logger.Error("kubelet failed", "error", err)
			os.Exit(1)
		}

	case "scheduler":
		sched := scheduler.NewScheduler(s, logger)
		if err := sched.Run(ctx); err != nil {
			logger.Error("scheduler failed", "error", err)
			os.Exit(1)
		}

	case "controller":
		cm := controllers.NewManager(s, logger)
		if err := cm.Run(ctx); err != nil {
			logger.Error("controllers failed", "error", err)
			os.Exit(1)
		}

	case "proxy":
		proxy := kubeproxy.NewProxy(s, kubeproxy.ModeIPTables, *clusterCIDR, logger)
		if err := proxy.Run(ctx); err != nil {
			logger.Error("proxy failed", "error", err)
			os.Exit(1)
		}

	case "dns":
		dns := coredns.NewClusterDNS(s, *clusterDomain, *dnsAddr, logger)
		if err := dns.Run(ctx); err != nil {
			logger.Error("dns failed", "error", err)
			os.Exit(1)
		}

	case "all":
		api := apiserver.NewAPIServer(*apiAddr, s)
		go func() {
			logger.Info("apiserver listening", "addr", *apiAddr)
			_ = api.Start()
		}()

		kl := kubelet.NewKubelet(*nodeName, s, logger)
		go func() { _ = kl.Run(ctx) }()

		sched := scheduler.NewScheduler(s, logger)
		go func() { _ = sched.Run(ctx) }()

		cm := controllers.NewManager(s, logger)
		go func() { _ = cm.Run(ctx) }()

		proxy := kubeproxy.NewProxy(s, kubeproxy.ModeIPTables, *clusterCIDR, logger)
		go func() { _ = proxy.Run(ctx) }()

		dns := coredns.NewClusterDNS(s, *clusterDomain, *dnsAddr, logger)
		go func() { _ = dns.Run(ctx) }()

		logger.Info("all components started",
			"api", *apiAddr,
			"node", *nodeName,
			"domain", *clusterDomain,
			"clusterCIDR", *clusterCIDR,
			"serviceCIDR", *serviceCIDR,
			"dns", *dnsAddr,
		)

		<-ctx.Done()

	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		fmt.Fprintf(os.Stderr, "valid modes: all, apiserver, kubelet, scheduler, controller, proxy, dns\n")
		os.Exit(1)
	}
}
