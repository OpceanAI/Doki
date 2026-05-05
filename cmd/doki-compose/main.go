package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/compose"
	"github.com/OpceanAI/Doki/pkg/image"
	"github.com/OpceanAI/Doki/pkg/network"
	"github.com/OpceanAI/Doki/pkg/runtime"
	"github.com/OpceanAI/Doki/pkg/storage"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	command := os.Args[1]

	cfg := common.DefaultConfig()
	dataDir := cfg.DataDir

	storeMgr, err := storage.NewManager(dataDir, cfg.StorageDriver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}

	imgStore, err := image.NewStore(common.ImageDir())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Image store error: %v\n", err)
		os.Exit(1)
	}

	netMgr, err := network.NewManager(
		common.NetworkDir(),
		network.NewFirewallManager(network.DetectFirewallBackend()),
		network.NewDNSServer(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Network error: %v\n", err)
		os.Exit(1)
	}

	rt := runtime.NewRuntime(cfg.ExecRoot, storeMgr)

	engine := compose.NewEngine("doki", rt, imgStore, netMgr)

	switch command {
	case "up":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Starting services...")
		if err := engine.Up(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services started")

	case "down":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := engine.Down(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services stopped")

	case "stop":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if err := engine.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services stopped")

	case "restart":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Restarting services...")
		if err := engine.Restart(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services restarted")

	case "logs":
		path := "."
		tail := 0
		if len(os.Args) > 2 {
			path = os.Args[2]
		}
		if len(os.Args) > 3 {
			if n, err := strconv.Atoi(os.Args[3]); err == nil {
				tail = n
			}
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		logs, err := engine.Logs(tail)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		for svcName, log := range logs {
			fmt.Printf("=== %s ===\n%s\n", svcName, log)
		}

	case "pull":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Pulling images...")
		if err := engine.Pull(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All images pulled")

	case "ps":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		containers, err := engine.Ps()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if len(containers) == 0 {
			fmt.Println("No running services")
			return
		}

		fmt.Println("CONTAINER ID   NAME                 STATE")
		for _, c := range containers {
			name := ""
			if len(c.Names) > 0 {
				name = c.Names[0]
			}
			fmt.Printf("%-14s %-20s %s\n", c.ID, name, c.Status)
		}

	case "build":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Building services...")
		if err := engine.Build(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Build completed")

	case "config":
		path := "."
		if len(os.Args) > 2 {
			path = os.Args[2]
		}

		if err := engine.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		output, err := engine.Config()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(output)

	case "help", "--help", "-h":
		printUsage()

	case "version":
		fmt.Printf("doki-compose version %s\n", common.Version)

	default:
		fmt.Fprintf(os.Stderr, "doki-compose: '%s' is not a valid command.\n", command)
		fmt.Fprintf(os.Stderr, "See 'doki-compose --help'.\n")
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf("Doki Compose %s\n\n", common.Version)
	fmt.Println("Usage: doki-compose [COMMAND]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  up        Create and start containers")
	fmt.Println("  down      Stop and remove containers")
	fmt.Println("  stop      Stop containers")
	fmt.Println("  restart   Restart containers")
	fmt.Println("  logs      View output from containers")
	fmt.Println("  pull      Pull service images")
	fmt.Println("  ps        List containers")
	fmt.Println("  build     Build or rebuild services")
	fmt.Println("  config    Parse and resolve compose file")
	fmt.Println("  version   Show version information")
	fmt.Println()
	fmt.Println("Run 'doki-compose [COMMAND] --help' for more information.")
}
