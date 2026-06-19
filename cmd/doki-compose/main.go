package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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

	var (
		fileFlag    string
		projFlag    string
		profileFlag string
		detachFlag  bool
		envFileFlag string
		quietFlag   bool
	)

	allArgs := os.Args[1:]
	command := ""
	cmdArgs := []string{}

	for i := 0; i < len(allArgs); i++ {
		a := allArgs[i]
		if command == "" {
			switch {
			case a == "-f" || a == "--file":
				if i+1 < len(allArgs) {
					fileFlag = allArgs[i+1]
					i++
				}
			case strings.HasPrefix(a, "-f="):
				fileFlag = strings.TrimPrefix(a, "-f=")
			case strings.HasPrefix(a, "--file="):
				fileFlag = strings.TrimPrefix(a, "--file=")
			case a == "-p" || a == "--project-name":
				if i+1 < len(allArgs) {
					projFlag = allArgs[i+1]
					i++
				}
			case strings.HasPrefix(a, "-p="):
				projFlag = strings.TrimPrefix(a, "-p=")
			case a == "--profile":
				if i+1 < len(allArgs) {
					profileFlag = allArgs[i+1]
					i++
				}
			case a == "-d" || a == "--detach":
				detachFlag = true
			case a == "--env-file":
				if i+1 < len(allArgs) {
					envFileFlag = allArgs[i+1]
					i++
				}
			case a == "-q" || a == "--quiet":
				quietFlag = true
			case a == "--help" || a == "-h":
				command = "help"
			default:
				if !strings.HasPrefix(a, "-") {
					command = a
					// Don't break - continue parsing flags after command
				}
			}
		} else {
			// Parse flags and args after command
			switch {
			case a == "-d" || a == "--detach":
				detachFlag = true
			case a == "-q" || a == "--quiet":
				quietFlag = true
			case a == "-f" || a == "--file":
				if i+1 < len(allArgs) {
					fileFlag = allArgs[i+1]
					i++
				}
			case strings.HasPrefix(a, "-f="):
				fileFlag = strings.TrimPrefix(a, "-f=")
			case strings.HasPrefix(a, "--file="):
				fileFlag = strings.TrimPrefix(a, "--file=")
			case a == "-p" || a == "--project-name":
				if i+1 < len(allArgs) {
					projFlag = allArgs[i+1]
					i++
				}
			case a == "--profile":
				if i+1 < len(allArgs) {
					profileFlag = allArgs[i+1]
					i++
				}
			case a == "--env-file":
				if i+1 < len(allArgs) {
					envFileFlag = allArgs[i+1]
					i++
				}
			case a == "--help" || a == "-h":
				// Ignore help after command
			default:
				// Non-flag args go to cmdArgs (service names, etc.)
				cmdArgs = append(cmdArgs, a)
			}
		}
	}

	if command == "" {
		printUsage()
		os.Exit(0)
	}

	cfg := common.DefaultConfig()
	dataDir := cfg.DataDir

	storeMgr, err := storage.NewManager(dataDir, cfg.StorageDriver)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Storage error: %v\n", err)
		os.Exit(1)
	}

	imgStore, err := image.NewStore(filepath.Join(dataDir, "images"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Image store error: %v\n", err)
		os.Exit(1)
	}

	netMgr, err := network.NewManager(
		filepath.Join(dataDir, "networks"),
		network.NewFirewallManager(network.DetectFirewallBackend()),
		network.NewDNSServer(nil),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Network error: %v\n", err)
		os.Exit(1)
	}

	rt := runtime.NewRuntime(cfg.ExecRoot, storeMgr)

	projectName := projFlag
	if projectName == "" {
		projectName = detectProjectName(fileFlag)
	}

	engine := compose.NewEngine(projectName, rt, imgStore, netMgr)

	needsFile := command != "help" && command != "version"
	if needsFile {
		loadPath := "."
		if fileFlag != "" {
			loadPath = fileFlag
		}
		if err := engine.Load(loadPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	if profileFlag != "" {
		_ = os.Setenv("COMPOSE_PROFILES", profileFlag)
	}
	if envFileFlag != "" {
		_ = os.Setenv("COMPOSE_ENV_FILE", envFileFlag)
	}

	switch command {
	case "up":
		fmt.Println("Starting services...")
		if err := engine.Up(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if !detachFlag {
			fmt.Println("All services started. Use -d to run in background.")
		}

	case "down":
		if err := engine.Down(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services stopped")

	case "stop":
		services := filterServices(cmdArgs)
		if err := engine.StopServices(services); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(services) > 0 {
			fmt.Printf("Services %s stopped\n", strings.Join(services, ", "))
		} else {
			fmt.Println("All services stopped")
		}

	case "start":
		services := filterServices(cmdArgs)
		fmt.Println("Starting services...")
		if err := engine.StartServices(services); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services started")

	case "restart":
		services := filterServices(cmdArgs)
		fmt.Println("Restarting services...")
		if err := engine.RestartServices(services); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All services restarted")

	case "kill":
		sig := "SIGKILL"
		services := []string{}
		for i := 0; i < len(cmdArgs); i++ {
			a := cmdArgs[i]
			if a == "-s" || a == "--signal" {
				if i+1 < len(cmdArgs) {
					sig = cmdArgs[i+1]
					i++
				}
			} else if strings.HasPrefix(a, "-s=") {
				sig = strings.TrimPrefix(a, "-s=")
			} else if strings.HasPrefix(a, "--signal=") {
				sig = strings.TrimPrefix(a, "--signal=")
			} else if !strings.HasPrefix(a, "-") {
				services = append(services, a)
			}
		}
		if err := engine.KillServices(sig, services); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Services killed")

	case "pause":
		if err := engine.PauseServices(filterServices(cmdArgs)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Services paused")

	case "unpause":
		if err := engine.UnpauseServices(filterServices(cmdArgs)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Services unpaused")

	case "logs":
		tail := 0
		follow := false
		services := []string{}
		for i := 0; i < len(cmdArgs); i++ {
			a := cmdArgs[i]
			switch {
			case a == "--tail" || a == "-n":
				if i+1 < len(cmdArgs) {
					tail, _ = strconv.Atoi(cmdArgs[i+1])
					i++
				}
			case strings.HasPrefix(a, "--tail="):
				tail, _ = strconv.Atoi(strings.TrimPrefix(a, "--tail="))
			case a == "-f" || a == "--follow":
				follow = true
			case a == "--since":
				if i+1 < len(cmdArgs) {
					i++
				}
			case !strings.HasPrefix(a, "-"):
				services = append(services, a)
			}
		}
		_ = follow
		logs, err := engine.Logs(tail)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		svcNames := make([]string, 0, len(logs))
		for svcName := range logs {
			if len(services) > 0 && !containsStr(services, svcName) {
				continue
			}
			svcNames = append(svcNames, svcName)
		}
		sort.Strings(svcNames)
		for _, svcName := range svcNames {
			if !quietFlag {
				fmt.Printf("=== %s ===\n", svcName)
			}
			fmt.Print(logs[svcName])
			if !strings.HasSuffix(logs[svcName], "\n") {
				fmt.Println()
			}
		}

	case "pull":
		fmt.Println("Pulling images...")
		if err := engine.Pull(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("All images pulled")

	case "ps":
		containers, err := engine.Ps()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(containers) == 0 {
			fmt.Println("No running services")
			return
		}
		quiet := quietFlag // Use global flag
		for _, a := range cmdArgs {
			if a == "-q" || a == "--quiet" {
				quiet = true
			}
		}
		if quiet {
			for _, c := range containers {
				fmt.Println(c.ID)
			}
			return
		}
		fmt.Println("CONTAINER ID   IMAGE                        COMMAND              STATUS         NAMES")
		for _, c := range containers {
			name := ""
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			cmd := c.Command
			if len(cmd) > 20 {
				cmd = cmd[:17] + "..."
			}
			shortID := c.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			fmt.Printf("%-14s %-28s %-20s %-14s %s\n", shortID, c.Image, cmd, c.Status, name)
		}

	case "build":
		fmt.Println("Building services...")
		if err := engine.Build(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Build completed")

	case "config":
		showServices := false
		showVolumes := false
		showNetworks := false
		for _, a := range cmdArgs {
			switch a {
			case "--services":
				showServices = true
			case "--volumes":
				showVolumes = true
			case "--networks":
				showNetworks = true
			}
		}
		if showServices || showVolumes || showNetworks {
			engine.PrintConfigSubset(showServices, showVolumes, showNetworks)
		} else {
			output, err := engine.Config()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Print(output)
		}

	case "rm":
		if err := engine.Down(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Removed services")

	case "images":
		imgs, err := engine.ProjectImages()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("REPOSITORY                   TAG         IMAGE ID")
		for _, img := range imgs {
			shortID := img.ID
			if len(shortID) > 12 {
				shortID = shortID[:12]
			}
			for _, tag := range img.RepoTags {
				parts := strings.SplitN(tag, ":", 2)
				repo := tag
				tagName := "-"
				if len(parts) == 2 {
					repo = parts[0]
					tagName = parts[1]
				}
				fmt.Printf("%-28s %-12s %s\n", repo, tagName, shortID)
			}
		}

	case "exec":
		if len(cmdArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: doki-compose exec SERVICE COMMAND [ARGS...]\n")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "exec: use 'doki exec' on the container directly\n")
		os.Exit(1)

	case "run":
		if len(cmdArgs) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: doki-compose run SERVICE [COMMAND]\n")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "run: use 'doki run' on the container directly\n")
		os.Exit(1)

	case "create":
		fmt.Println("Creating services...")
		if err := engine.Create(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Services created")

	case "top":
		if err := engine.Top(filterServices(cmdArgs)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "port":
		if len(cmdArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: doki-compose port SERVICE PRIVATE_PORT\n")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "port: not yet fully implemented\n")
		os.Exit(1)

	case "push":
		fmt.Fprintf(os.Stderr, "push: use 'doki push' directly\n")
		os.Exit(1)

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

func filterServices(args []string) []string {
	var services []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			services = append(services, a)
		}
	}
	return services
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Printf("Doki Compose %s\n\n", common.Version)
	fmt.Println("Usage: doki-compose [OPTIONS] COMMAND [ARGS...]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  up        Create and start containers")
	fmt.Println("  down      Stop and remove containers")
	fmt.Println("  stop      Stop containers")
	fmt.Println("  start     Start stopped containers")
	fmt.Println("  restart   Restart containers")
	fmt.Println("  kill      Kill containers")
	fmt.Println("  pause     Pause containers")
	fmt.Println("  unpause   Unpause containers")
	fmt.Println("  logs      View output from containers")
	fmt.Println("  pull      Pull service images")
	fmt.Println("  ps        List running containers")
	fmt.Println("  build     Build or rebuild services")
	fmt.Println("  config    Parse, resolve and view compose file")
	fmt.Println("  rm        Remove stopped service containers")
	fmt.Println("  images    List images used by services")
	fmt.Println("  exec      Execute a command in a running container")
	fmt.Println("  run       Run a one-off command")
	fmt.Println("  create    Create services")
	fmt.Println("  top       Display running processes")
	fmt.Println("  port      Print the public port for a port binding")
	fmt.Println("  push      Push service images")
	fmt.Println("  version   Show version information")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -f, --file PATH          Compose file(s) (default: auto-detect)")
	fmt.Println("  -p, --project-name NAME  Project name (default: directory name)")
	fmt.Println("  --profile PROFILE        Run with the given profile")
	fmt.Println("  -d, --detach             Run in background")
	fmt.Println("  --env-file PATH          Specify environment file")
	fmt.Println("  -q, --quiet              Suppress output")
}

func detectProjectName(fileFlag string) string {
	if fileFlag != "" {
		base := filepath.Base(fileFlag)
		base = strings.TrimSuffix(base, ".yaml")
		base = strings.TrimSuffix(base, ".yml")
		base = strings.TrimSuffix(base, ".json")
		if base != "." && base != "compose" && base != "docker-compose" && base != "doki-compose" && base != "doki" {
			return base
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "doki"
	}
	return filepath.Base(wd)
}
