package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/OpceanAI/Doki/pkg/cli"
	"github.com/OpceanAI/Doki/pkg/common"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	socket := os.Getenv("DOKI_HOST")
	if socket == "" {
		socket = common.DefaultDaemonSocket()
	}
	if h := os.Getenv("DOCKER_HOST"); h != "" && socket == common.DefaultDaemonSocket() {
		socket = h
	}

	c := cli.New(socket)
	cmd := os.Args[1]
	args := os.Args[2:]

	// Handle help flag on any command.
	if cmd == "help" || cmd == "--help" || cmd == "-h" {
		if len(args) > 0 {
			printCommandHelp(args[0])
		} else {
			printHelp()
		}
		return
	}

	// Handle global flags.
	if cmd == "--version" || cmd == "-v" {
		c.SystemVersion()
		return
	}

	switch cmd {
	// Container commands.
	case "run":
		handleError(c.Run(args))
	case "ps":
		all := flagBool(args, "-a", "--all")
		quiet := flagBool(args, "-q", "--quiet")
		lastN := flagInt(args, "-n", "--last")
		filter := flagStr(args, "-f", "--filter")
		handleError(c.Ps(all, quiet, false, filter, "", lastN, false))
	case "create":
		img, cmdArgs, f := cli.ParseRunFlags(args)
		id, err := c.Create(img, cmdArgs, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(id[:12])
	case "start", "restart":
		timeout := flagInt(args, "-t", "--time")
		if cmd == "restart" {
			handleError(c.Restart(cleanIDs(args), timeout))
		} else {
			handleError(c.Start(cleanIDs(args)))
		}
	case "stop":
		timeout := flagInt(args, "-t", "--time")
		handleError(c.Stop(cleanIDs(args), timeout))
	case "kill":
		sig := flagStr(args, "-s", "--signal")
		handleError(c.Kill(cleanIDs(args), sig))
	case "rm":
		force := flagBool(args, "-f", "--force")
		volumes := flagBool(args, "-v", "--volumes")
		handleError(c.Rm(cleanIDs(args), force, volumes, false))
	case "pause":
		handleError(c.Pause(cleanIDs(args)))
	case "unpause":
		handleError(c.Unpause(cleanIDs(args)))
	case "exec":
		tty := flagBool(args, "-t", "--tty")
		detach := flagBool(args, "-d", "--detach")
		interactive := flagBool(args, "-i", "--interactive")
		env := flagStrSlice(args, "-e", "--env")
		workdir := flagStr(args, "-w", "--workdir")
		user := flagStr(args, "-u", "--user")
		if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			containerID := args[0]
			execArgs := args[1:]
			if len(execArgs) == 0 {
				fmt.Fprintf(os.Stderr, "doki exec: requires at least 1 argument (command)\n")
				os.Exit(1)
			}
			c.Exec(containerID, execArgs, tty, detach, interactive, env, workdir, user)
		}
	case "logs":
		follow := flagBool(args, "-f", "--follow")
		tail := flagInt(args, "-n", "--tail")
		timestamps := flagBool(args, "-t", "--timestamps")
		if len(args) > 0 {
			c.Logs(args[0], follow, timestamps, tail, "")
		}
	case "stats":
		noStream := flagBool(args, "--no-stream")
		handleError(c.Stats(cleanIDs(args), noStream))
	case "top":
		if len(args) > 0 {
			c.Top(args[0], "")
		}
	case "inspect":
		format := flagStr(args, "-f", "--format")
		handleError(c.Inspect(cleanIDs(args), format))
	case "commit":
		author := flagStr(args, "-a", "--author")
		message := flagStr(args, "-m", "--message")
		pause := !flagBool(args, "-p=false", "--pause=false")
		if len(args) >= 2 {
			repoTag := args[1]
			parts := strings.SplitN(repoTag, ":", 2)
			repo := parts[0]
			tag := ""
			if len(parts) > 1 {
				tag = parts[1]
			}
			c.Commit(args[0], repo, tag, author, message, pause, nil)
		}
	case "diff":
		if len(args) > 0 {
			c.Diff(args[0])
		}
	case "port":
		if len(args) >= 2 {
			c.Port(args[0], args[1])
		}
	case "rename":
		if len(args) >= 2 {
			c.Rename(args[0], args[1])
		}
	case "update":
		if len(args) > 0 {
			_, _, f := cli.ParseRunFlags(args[1:])
			c.Update(args[0], f)
		}
	case "wait":
		if len(args) > 0 {
			exitCode, _ := c.Wait(args[0])
			os.Exit(exitCode)
		}
	case "export":
		output := flagStr(args, "-o", "--output")
		if len(args) > 0 {
			c.Export(args[0], output)
		}
	case "cp":
		if len(args) >= 2 {
			c.Cp(args[0], args[1], args[2], false, false)
		}
	case "attach":
		if len(args) > 0 {
			c.Attach(args[0], "", "")
		}
	case "prune":
		all := flagBool(args, "-a", "--all")
		filter := flagStr(args, "-f", "--filter")
		handleError(c.Prune(all, filter))

	// Image commands.
	case "pull":
		allTags := flagBool(args, "-a", "--all-tags")
		quiet := flagBool(args, "-q", "--quiet")
		if len(args) > 0 {
			c.Pull(args[0], allTags, quiet)
		}
	case "push":
		quiet := flagBool(args, "-q", "--quiet")
		if len(args) > 0 {
			c.Push(args[0], false, quiet)
		}
	case "images":
		all := flagBool(args, "-a", "--all")
		quiet := flagBool(args, "-q", "--quiet")
		filter := flagStr(args, "-f", "--filter")
		handleError(c.Images(all, quiet, false, filter))
	case "rmi":
		force := flagBool(args, "-f", "--force")
		handleError(c.Rmi(cleanIDs(args), force, false))
	case "tag":
		if len(args) >= 2 {
			c.Tag(args[0], args[1])
		}
	case "history":
		noTrunc := flagBool(args, "--no-trunc")
		quiet := flagBool(args, "-q", "--quiet")
		if len(args) > 0 {
			c.History(args[0], noTrunc, quiet)
		}
	case "save":
		output := flagStr(args, "-o", "--output")
		names := cleanIDs(args)
		handleError(c.Save(names, output))
	case "load":
		input := flagStr(args, "-i", "--input")
		quiet := flagBool(args, "-q", "--quiet")
		handleError(c.Load(input, quiet))
	case "import":
		if len(args) > 0 {
			c.Import(args[0], "", "", nil, "")
		}
	case "build":
		tags := flagStrSlice(args, "-t", "--tag")
		f := flagStr(args, "-f", "--file")
		noCache := flagBool(args, "--no-cache")
		pull := flagBool(args, "--pull")
		quiet := flagBool(args, "-q", "--quiet")
		rmFlag := !flagBool(args, "--rm=false")
		// Context dir is the last non-flag argument.
		contextDir := "."
		for i := len(args) - 1; i >= 0; i-- {
			if !strings.HasPrefix(args[i], "-") {
				contextDir = args[i]
				break
			}
		}
		handleError(c.Build(contextDir, f, tags, nil, noCache, pull, quiet, rmFlag))
	case "search":
		limit := flagInt(args, "--limit")
		if len(args) > 0 {
			c.Search(args[0], limit, false, 0)
		}

	// Network commands.
	case "network":
		handleNetwork(c, args)
	// Volume commands.
	case "volume":
		handleVolume(c, args)

	// System commands.
	case "system":
		handleSystem(c, args)
	case "info":
		c.SystemInfo()
	case "version", "--version":
		c.SystemVersion()
	case "events":
		filter := flagStr(args, "-f", "--filter")
		c.Events(filter)
	case "login":
		server := flagStr(args, "-s", "--server")
		username := flagStr(args, "-u", "--username")
		password := flagStr(args, "-p", "--password")
		handleError(c.Login(server, username, password))
	case "logout":
		server := ""
		if len(args) > 0 {
			server = args[0]
		}
		handleError(c.Logout(server))
	case "ping":
		handleError(c.Ping())

	// Podman-specific.
	case "pod":
		handlePod(c, args)
	case "generate":
		handleGenerate(c, args)
	case "play":
		handlePlay(c, args)
	case "auto-update":
		c.AutoUpdate()
	case "unshare":
		c.Unshare(args)
	case "untag":
		for _, a := range args {
			c.Untag(a)
		}
	case "mount":
		c.Mount(cleanIDs(args))
	case "unmount":
		c.Unmount(cleanIDs(args))
	case "healthcheck":
		if len(args) > 0 {
			c.Healthcheck(args[0])
		}

	// Kubernetes.
	case "kube":
		handleKube(c, args)
	case "apply":
		if len(args) >= 2 && args[0] == "-f" {
			c.Apply(args[1])
		}

	default:
		fmt.Fprintf(os.Stderr, "doki: '%s' is not a doki command.\n", cmd)
		fmt.Fprintf(os.Stderr, "See 'doki --help'.\n")
	}
}

func handleNetwork(c *cli.DokiCLI, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: doki network COMMAND")
		return
	}
	switch args[0] {
	case "ls", "list":
		c.NetworkLs(false, false, "", "")
	case "create":
		name, driver, subnet, gw := "", "bridge", "", ""
		for i, a := range args[1:] {
			if a == "-d" || a == "--driver" {
				if i+2 < len(args) { driver = args[i+2] }
			} else if a == "--subnet" {
				if i+2 < len(args) { subnet = args[i+2] }
			} else if a == "--gateway" {
				if i+2 < len(args) { gw = args[i+2] }
			} else if !strings.HasPrefix(a, "-") {
				name = a
			}
		}
		c.NetworkCreate(name, driver, false, false, subnet, gw, nil)
	case "rm", "remove":
		c.NetworkRm(args[1:])
	case "inspect":
		c.NetworkInspect(args[1:])
	case "connect":
		if len(args) >= 3 {
			c.NetworkConnect(args[1], args[2], nil)
		}
	case "disconnect":
		if len(args) >= 3 {
			c.NetworkDisconnect(args[1], args[2], false)
		}
	case "prune":
		c.NetworkPrune("")
	}
}

func handleVolume(c *cli.DokiCLI, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: doki volume COMMAND")
		return
	}
	switch args[0] {
	case "ls", "list":
		c.VolumeLs(false, "")
	case "create":
		name, driver := "", "local"
		for i, a := range args[1:] {
			if a == "-d" || a == "--driver" {
				if i+2 < len(args) { driver = args[i+2] }
			} else if !strings.HasPrefix(a, "-") {
				name = a
			}
		}
		if name == "" {
			name = common.GenerateID(32)
		}
		c.VolumeCreate(name, driver, nil, nil)
	case "rm", "remove":
		c.VolumeRm(args[1:], false)
	case "inspect":
		c.VolumeInspect(args[1:])
	case "prune":
		c.VolumePrune("")
	}
}

func handleSystem(c *cli.DokiCLI, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: doki system COMMAND")
		return
	}
	switch args[0] {
	case "info":
		c.SystemInfo()
	case "df":
		verbose := false
		for _, a := range args {
			if a == "-v" || a == "--verbose" {
				verbose = true
			}
		}
		c.SystemDf(verbose)
	case "prune":
		all := false
		volumes := false
		for i, a := range args {
			if a == "-a" || a == "--all" {
				all = true
			}
			if a == "--volumes" {
				volumes = true
			}
			_ = i
		}
		c.SystemPrune(all, volumes, "")
	case "events":
		c.SystemEvents("", "", "")
	case "dial-stdio":
		c.SystemDialStdio()
	}
}

func handlePod(c *cli.DokiCLI, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: doki pod COMMAND")
		return
	}
	switch args[0] {
	case "create":
		c.PodCreate(args[1], nil)
	case "ps", "ls", "list":
		c.PodPs()
	case "rm":
		c.PodRm(args[1:], false)
	case "start":
		c.PodStart(args[1:])
	case "stop":
		c.PodStop(args[1:])
	}
}

func handleGenerate(c *cli.DokiCLI, args []string) {
	if len(args) == 0 {
		return
	}
	switch args[0] {
	case "kube":
		containerID := ""
		for i, a := range args {
			if a == "-s" || a == "--service" {
				if len(args) > i+1 {
					containerID = args[i+1]
				}
			} else if !strings.HasPrefix(a, "-") {
				containerID = a
			}
		}
		c.GenerateKube(containerID, true)
	}
}

func handlePlay(c *cli.DokiCLI, args []string) {
	if len(args) < 1 {
		return
	}
	c.KubePlay(args[0])
}

func handleKube(c *cli.DokiCLI, args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: doki kube COMMAND")
		return
	}
	switch args[0] {
	case "play":
		if len(args) > 1 {
			c.KubePlay(args[1])
		}
	case "down":
		if len(args) > 1 {
			c.KubeDown(args[1])
		}
	case "generate":
		c.KubeGenerate(args[1])
	}
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Doki - Universal Container Engine")
	fmt.Println()
	fmt.Println("Usage:  doki [OPTIONS] COMMAND")
	fmt.Println()
	fmt.Println("Management Commands:")
	fmt.Println("  container   Manage containers")
	fmt.Println("  image       Manage images")
	fmt.Println("  network     Manage networks")
	fmt.Println("  volume      Manage volumes")
	fmt.Println("  system      Manage Doki")
	fmt.Println("  pod         Manage pods")
	fmt.Println("  kube        Kubernetes integration")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  run         Create and run a new container from an image")
	fmt.Println("  exec        Execute a command in a running container")
	fmt.Println("  ps          List containers")
	fmt.Println("  build       Build an image from a Dokifile")
	fmt.Println("  pull        Download an image from a registry")
	fmt.Println("  push        Upload an image to a registry")
	fmt.Println("  images      List images")
	fmt.Println("  login       Log in to a registry")
	fmt.Println("  logout      Log out from a registry")
	fmt.Println("  search      Search Docker Hub for images")
	fmt.Println("  version     Show the Doki version information")
	fmt.Println("  info        Display system-wide information")
	fmt.Println("  play        Play Kubernetes YAML")
	fmt.Println("  generate    Generate Kubernetes YAML")
	fmt.Println("  unshare     Run command in user namespace")
	fmt.Println("  mount       Mount container root filesystem")
	fmt.Println("  unmount     Unmount container root filesystem")
	fmt.Println()
	fmt.Println("Run 'doki COMMAND --help' for more information.")
}

func printCommandHelp(cmd string) {
	fmt.Printf("Usage: doki %s [OPTIONS]\n", cmd)
	fmt.Println("See https://docs.docker.com/reference/ for detailed help.")
}

func flagBool(args []string, names ...string) bool {
	for _, n := range names {
		for _, a := range args {
			if a == n {
				return true
			}
		}
	}
	return false
}

func flagStr(args []string, names ...string) string {
	for _, n := range names {
		for i, a := range args {
			if a == n && i+1 < len(args) {
				return args[i+1]
			}
		}
	}
	return ""
}

func flagInt(args []string, names ...string) int {
	s := flagStr(args, names...)
	if s != "" {
		v, _ := strconv.Atoi(s)
		return v
	}
	return 0
}

func flagStrSlice(args []string, names ...string) []string {
	var result []string
	for _, n := range names {
		for i, a := range args {
			if a == n && i+1 < len(args) {
				result = append(result, args[i+1])
			}
		}
	}
	return result
}

func cleanIDs(args []string) []string {
	var ids []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			ids = append(ids, a)
		}
	}
	return ids
}
