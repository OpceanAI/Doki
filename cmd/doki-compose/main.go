package main

import (
	"fmt"
	"os"

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

	// Initialize core services.
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

	netMgr, err := network.NewManager(common.NetworkDir())
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
		fmt.Println("doki-compose build: Building images for services...")

	case "config":
		fmt.Println("doki-compose config: Validating and printing compose file...")

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
	fmt.Println("  ps        List containers")
	fmt.Println("  build     Build or rebuild services")
	fmt.Println("  config    Parse and resolve compose file")
	fmt.Println("  version   Show version information")
	fmt.Println()
	fmt.Println("Run 'doki-compose [COMMAND] --help' for more information.")
}
