package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/OpceanAI/Doki/pkg/cli"
	"github.com/OpceanAI/Doki/pkg/common"
)

// Command represents a CLI command with optional sub-commands.
type Command struct {
	Name        string
	Aliases     []string
	Handler     func(c *cli.DokiCLI, args []string) error
	Help        string
	SubCommands map[string]*Command
}

func main() {
	socket := os.Getenv("DOKI_HOST")
	if socket == "" {
		socket = common.DefaultDaemonSocket()
	}
	if h := os.Getenv("DOCKER_HOST"); h != "" && socket == common.DefaultDaemonSocket() {
		socket = h
	}

	c := cli.New(socket)

	if len(os.Args) < 2 {
		printMainHelp()
		return
	}

	cmdName := os.Args[1]
	args := os.Args[2:]

	if cmdName == "help" || cmdName == "--help" || cmdName == "-h" {
		if len(args) > 0 {
			printCommandHelp(args[0])
		} else {
			printMainHelp()
		}
		return
	}

	dispatch(c, cmdName, args)
}

func dispatch(c *cli.DokiCLI, name string, args []string) {
	if sub, ok := groupCommands[name]; ok {
		if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
			fmt.Println(sub.Help)
			return
		}
		dispatch(c, args[0], args[1:])
		return
	}

	cmd, ok := commands[name]
	if !ok {
		for _, c2 := range commands {
			for _, a := range c2.Aliases {
				if a == name {
					cmd = c2
					ok = true
					break
				}
			}
			if ok {
				break
			}
		}
	}
	if !ok {
		fmt.Fprintf(os.Stderr, "doki: '%s' is not a doki command.\n", name)
		fmt.Fprintf(os.Stderr, "See 'doki --help'.\n")
		os.Exit(1)
	}

	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(cmd.Help)
		return
	}

	handleError(cmd.Handler(c, args))
}

var commands = map[string]*Command{
	// --- Container commands ---
	"run": {
		Name: "run",
		Help: `Usage: doki run [OPTIONS] IMAGE [COMMAND] [ARG...]

Create and run a new container from an image.

Options:
  -d, --detach       Run container in background
  -i, --interactive  Keep STDIN open
  -t, --tty          Allocate pseudo-TTY
  --rm               Remove container on exit
  --name NAME        Container name
  -p, --publish      Publish port (host:container)
  -v, --volume       Bind mount a volume
  -e, --env          Set environment variable
  -w, --workdir      Working directory
  -u, --user         Username or UID
  --network          Network mode (bridge/host/none)
  --restart          Restart policy
  --pull             Pull policy (always/missing/never)
  --distro NAME      Use predefined distro (alpine, ubuntu, debian, arch)
  --install PKGS     Install packages before running (comma-separated)

Examples:
  doki run alpine echo hello
  doki run -d --name web -p 8080:80 nginx:alpine
  doki run --rm alpine /bin/sh -c "echo test"
  doki run --distro ubuntu bash
  doki run --distro alpine --install gcc,make gcc --version`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("doki run: requires at least 1 argument (image)")
			}

			distro := flagStr(args, "--distro")
			if distro != "" {
				return runWithDistro(c, distro, args)
			}

			return c.Run(args)
		},
	},
	"ps": {
		Name: "ps", Aliases: []string{"list"},
		Help: `Usage: doki ps [OPTIONS]

List containers.

Options:
  -a, --all      Show all containers (default shows just running)
  -q, --quiet    Only display container IDs
  -n, --last N   Show N last created containers
  -f, --filter   Filter output based on conditions`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			all := flagBool(args, "-a", "--all")
			quiet := flagBool(args, "-q", "--quiet")
			lastN := flagInt(args, "-n", "--last")
			filter := flagStr(args, "-f", "--filter")
			return c.Ps(all, quiet, false, filter, "", lastN, false)
		},
	},
	"create": {
		Name: "create",
		Help: `Usage: doki create [OPTIONS] IMAGE [COMMAND] [ARG...]

Create a new container without starting it.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			img, cmdArgs, f := cli.ParseRunFlags(args)
			id, err := c.Create(img, cmdArgs, f)
			if err != nil {
				return err
			}
			fmt.Println(id[:12])
			return nil
		},
	},
	"start": {
		Name: "start",
		Help: `Usage: doki start CONTAINER [CONTAINER...]

Start one or more stopped containers.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Start(cleanIDs(args))
		},
	},
	"stop": {
		Name: "stop",
		Help: `Usage: doki stop [OPTIONS] CONTAINER [CONTAINER...]

Stop one or more running containers.

Options:
  -t, --time SECONDS  Seconds to wait for stop before killing (default 10)`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			timeout := flagInt(args, "-t", "--time")
			return c.Stop(cleanIDs(args), timeout)
		},
	},
	"restart": {
		Name: "restart",
		Help: `Usage: doki restart [OPTIONS] CONTAINER [CONTAINER...]

Restart one or more containers.

Options:
  -t, --time SECONDS  Seconds to wait for stop before killing (default 10)`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			timeout := flagInt(args, "-t", "--time")
			return c.Restart(cleanIDs(args), timeout)
		},
	},
	"kill": {
		Name: "kill",
		Help: `Usage: doki kill [OPTIONS] CONTAINER [CONTAINER...]

Kill one or more running containers.

Options:
  -s, --signal SIGNAL  Signal to send (default "KILL")`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			sig := flagStr(args, "-s", "--signal")
			return c.Kill(cleanIDs(args), sig)
		},
	},
	"rm": {
		Name: "rm", Aliases: []string{"remove"},
		Help: `Usage: doki rm [OPTIONS] CONTAINER [CONTAINER...]

Remove one or more containers.

Options:
  -f, --force   Force removal of running container
  -v, --volumes Remove anonymous volumes`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			force := flagBool(args, "-f", "--force")
			volumes := flagBool(args, "-v", "--volumes")
			return c.Rm(cleanIDs(args), force, volumes, false)
		},
	},
	"pause": {
		Name: "pause",
		Help: `Usage: doki pause CONTAINER [CONTAINER...]

Pause all processes within one or more containers.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Pause(cleanIDs(args))
		},
	},
	"unpause": {
		Name: "unpause",
		Help: `Usage: doki unpause CONTAINER [CONTAINER...]

Unpause all processes within one or more containers.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Unpause(cleanIDs(args))
		},
	},
	"exec": {
		Name: "exec",
		Help: `Usage: doki exec [OPTIONS] CONTAINER COMMAND [ARG...]

Execute a command in a running container.

Options:
  -i, --interactive  Keep STDIN open
  -t, --tty          Allocate pseudo-TTY
  -d, --detach       Detached mode
  -e, --env          Set environment variables
  -w, --workdir      Working directory inside the container
  -u, --user         Username or UID`,
		Handler: func(c *cli.DokiCLI, args []string) error {
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
					return fmt.Errorf("doki exec: requires at least 1 argument (command)")
				}
				return c.Exec(containerID, execArgs, tty, detach, interactive, env, workdir, user)
			}
			return nil
		},
	},
	"logs": {
		Name: "logs",
		Help: `Usage: doki logs [OPTIONS] CONTAINER

Fetch the logs of a container.

Options:
  -f, --follow    Follow log output
  -n, --tail N    Number of lines to show
  -t, --timestamps  Show timestamps`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			follow := flagBool(args, "-f", "--follow")
			tail := flagInt(args, "-n", "--tail")
			timestamps := flagBool(args, "-t", "--timestamps")
			if len(args) > 0 {
				return c.Logs(args[0], follow, timestamps, tail, "")
			}
			return nil
		},
	},
	"stats": {
		Name: "stats",
		Help: `Usage: doki stats [OPTIONS] [CONTAINER...]

Display a live stream of container resource usage statistics.

Options:
  --no-stream  Disable streaming stats and only pull the first result`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			noStream := flagBool(args, "--no-stream")
			return c.Stats(cleanIDs(args), noStream)
		},
	},
	"top": {
		Name: "top",
		Help: `Usage: doki top CONTAINER

Display the running processes of a container.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) > 0 {
				return c.Top(args[0], "")
			}
			return nil
		},
	},
	"inspect": {
		Name: "inspect",
		Help: `Usage: doki inspect [OPTIONS] CONTAINER|IMAGE [CONTAINER|IMAGE...]

Return low-level information on Doki objects.

Options:
  -f, --format FORMAT  Format the output using a Go template`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			format := flagStr(args, "-f", "--format")
			return c.Inspect(cleanIDs(args), format)
		},
	},
	"commit": {
		Name: "commit",
		Help: `Usage: doki commit [OPTIONS] CONTAINER [REPOSITORY[:TAG]]

Create a new image from a container's changes.

Options:
  -a, --author AUTHOR   Author
  -m, --message MESSAGE  Commit message`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			author := flagStr(args, "-a", "--author")
			message := flagStr(args, "-m", "--message")
			pause := true
			if flagBool(args, "--pause=false", "-p=false") {
				pause = false
			}
			if len(args) >= 2 {
				repoTag := args[1]
				parts := strings.SplitN(repoTag, ":", 2)
				repo := parts[0]
				tag := ""
				if len(parts) > 1 {
					tag = parts[1]
				}
				return c.Commit(args[0], repo, tag, author, message, pause, nil)
			}
			return nil
		},
	},
	"diff": {
		Name: "diff",
		Help: `Usage: doki diff CONTAINER

Inspect changes to files or directories on a container's filesystem.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) > 0 {
				return c.Diff(args[0])
			}
			return nil
		},
	},
	"port": {
		Name: "port",
		Help: `Usage: doki port CONTAINER PRIVATE_PORT

List port mappings for the container.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) >= 2 {
				return c.Port(args[0], args[1])
			}
			return nil
		},
	},
	"rename": {
		Name: "rename",
		Help: `Usage: doki rename CONTAINER NEW_NAME

Rename a container.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) >= 2 {
				return c.Rename(args[0], args[1])
			}
			return nil
		},
	},
	"update": {
		Name: "update",
		Help: `Usage: doki update [OPTIONS] CONTAINER [CONTAINER...]

Update configuration of one or more containers.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) > 0 {
				_, _, f := cli.ParseRunFlags(args[1:])
				return c.Update(args[0], f)
			}
			return nil
		},
	},
	"wait": {
		Name: "wait",
		Help: `Usage: doki wait CONTAINER [CONTAINER...]

Block until one or more containers stop, then print their exit codes.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			containerIDs := cleanIDs(args)
			if len(containerIDs) > 0 {
				var lastExit int
				for _, id := range containerIDs {
					exitCode, err := c.Wait(id)
					if err != nil {
						return err
					}
					lastExit = exitCode
				}
				os.Exit(lastExit)
			}
			return nil
		},
	},
	"export": {
		Name: "export",
		Help: `Usage: doki export [OPTIONS] CONTAINER

Export a container's filesystem as a tar archive.

Options:
  -o, --output FILE  Write to a file instead of stdout`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			output := flagStr(args, "-o", "--output")
			if len(args) > 0 {
				return c.Export(args[0], output)
			}
			return nil
		},
	},
	"cp": {
		Name: "cp",
		Help: `Usage: doki cp CONTAINER:SRC_PATH DEST_PATH|-
       doki cp SRC_PATH|- CONTAINER:DEST_PATH

Copy files/folders between a container and the local filesystem.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) >= 2 {
				if strings.Contains(args[0], ":") {
					parts := strings.SplitN(args[0], ":", 2)
					destPath := args[1]
					if len(args) >= 3 {
						destPath = args[2]
					}
					return c.Cp(parts[0], parts[1], destPath, false, false)
				} else if strings.Contains(args[1], ":") {
					parts := strings.SplitN(args[1], ":", 2)
					return c.CpToContainer(parts[0], args[0], parts[1], false, false)
				}
				return fmt.Errorf("doki cp: must specify container path with CONTAINER:PATH")
			}
			return nil
		},
	},
	"attach": {
		Name: "attach",
		Help: `Usage: doki attach [OPTIONS] CONTAINER

Attach local standard input, output, and error streams to a running container.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) > 0 {
				return c.Attach(args[0], "", "")
			}
			return nil
		},
	},
	"prune": {
		Name: "prune",
		Help: `Usage: doki container prune [OPTIONS]

Remove all stopped containers.

Options:
  -a, --all     Remove all unused containers, not just stopped ones
  -f, --filter  Provide filter values`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			all := flagBool(args, "-a", "--all")
			filter := flagStr(args, "-f", "--filter")
			return c.Prune(all, filter)
		},
	},

	// --- Image commands ---
	"pull": {
		Name: "pull",
		Help: `Usage: doki pull [OPTIONS] NAME[:TAG|@DIGEST]

Pull an image or a repository from a registry.

Options:
  -a, --all-tags  Download all tagged images in the repository
  -q, --quiet     Suppress verbose output`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			allTags := flagBool(args, "-a", "--all-tags")
			quiet := flagBool(args, "-q", "--quiet")
			if len(args) > 0 {
				return c.Pull(args[0], allTags, quiet)
			}
			return nil
		},
	},
	"push": {
		Name: "push",
		Help: `Usage: doki push [OPTIONS] NAME[:TAG]

Push an image or a repository to a registry.

Options:
  -a, --all-tags  Push all tagged images in the repository
  -q, --quiet     Suppress verbose output`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			allTags := flagBool(args, "-a", "--all-tags")
			quiet := flagBool(args, "-q", "--quiet")
			if len(args) > 0 {
				return c.Push(args[0], allTags, quiet)
			}
			return nil
		},
	},
	"images": {
		Name: "images", Aliases: []string{"image list"},
		Help: `Usage: doki images [OPTIONS] [REPOSITORY[:TAG]]

List images.

Options:
  -a, --all      Show all images
  -q, --quiet    Only show image IDs
  -f, --filter   Filter output based on conditions`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			all := flagBool(args, "-a", "--all")
			quiet := flagBool(args, "-q", "--quiet")
			filter := flagStr(args, "-f", "--filter")
			return c.Images(all, quiet, false, filter)
		},
	},
	"rmi": {
		Name: "rmi", Aliases: []string{"image rm", "image remove"},
		Help: `Usage: doki rmi [OPTIONS] IMAGE [IMAGE...]

Remove one or more images.

Options:
  -f, --force  Force removal of the image`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			force := flagBool(args, "-f", "--force")
			return c.Rmi(cleanIDs(args), force, false)
		},
	},
	"tag": {
		Name: "tag",
		Help: `Usage: doki tag SOURCE_IMAGE[:TAG] TARGET_IMAGE[:TAG]

Create a tag TARGET_IMAGE that refers to SOURCE_IMAGE.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) >= 2 {
				return c.Tag(args[0], args[1])
			}
			return nil
		},
	},
	"history": {
		Name: "history",
		Help: `Usage: doki history [OPTIONS] IMAGE

Show the history of an image.

Options:
  --no-trunc  Don't truncate output
  -q, --quiet  Only show image IDs`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			noTrunc := flagBool(args, "--no-trunc")
			quiet := flagBool(args, "-q", "--quiet")
			if len(args) > 0 {
				return c.History(args[0], noTrunc, quiet)
			}
			return nil
		},
	},
	"save": {
		Name: "save",
		Help: `Usage: doki save [OPTIONS] IMAGE [IMAGE...]

Save one or more images to a tar archive.

Options:
  -o, --output FILE  Write to a file instead of stdout`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			output := flagStr(args, "-o", "--output")
			names := cleanIDs(args)
			return c.Save(names, output)
		},
	},
	"load": {
		Name: "load",
		Help: `Usage: doki load [OPTIONS]

Load an image from a tar archive or STDIN.

Options:
  -i, --input FILE  Read from tar archive file
  -q, --quiet       Suppress the load output`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			input := flagStr(args, "-i", "--input")
			quiet := flagBool(args, "-q", "--quiet")
			return c.Load(input, quiet)
		},
	},
	"import": {
		Name: "import",
		Help: `Usage: doki import [OPTIONS] file|URL|- [REPOSITORY[:TAG]]

Import the contents from a tarball to create a filesystem image.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) > 0 {
				return c.Import(args[0], "", "", nil, "")
			}
			return nil
		},
	},
	"build": {
		Name: "build",
		Help: `Usage: doki build [OPTIONS] PATH

Build an image from a Dokifile or Dockerfile.

Options:
  -f, --file FILE    Path to Dokifile (default: Dokifile)
  -t, --tag TAG      Name and optionally a tag (name:tag)
  --no-cache         Do not use cache when building
  --pull             Always pull base images
  -q, --quiet        Suppress build output`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			tags := flagStrSlice(args, "-t", "--tag")
			f := flagStr(args, "-f", "--file")
			noCache := flagBool(args, "--no-cache")
			pull := flagBool(args, "--pull")
			quiet := flagBool(args, "-q", "--quiet")
			rmFlag := !flagBool(args, "--rm=false")
			contextDir := "."
			for i := len(args) - 1; i >= 0; i-- {
				if !strings.HasPrefix(args[i], "-") {
					contextDir = args[i]
					break
				}
			}
			return c.Build(contextDir, f, tags, nil, noCache, pull, quiet, rmFlag)
		},
	},
	"search": {
		Name: "search",
		Help: `Usage: doki search [OPTIONS] TERM

Search Docker Hub for images.

Options:
  --limit N  Maximum number of results (default 25)`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			limit := flagInt(args, "--limit")
			if len(args) > 0 {
				return c.Search(args[0], limit, false, 0)
			}
			return nil
		},
	},

	// --- Network commands ---
	"network": {
		Name: "network",
		Help: `Usage: doki network COMMAND [OPTIONS]

Commands:
  ls          List networks
  create      Create a network
  rm          Remove a network
  inspect     Display network details
  connect     Connect a container to a network
  disconnect  Disconnect a container from a network
  prune       Remove unused networks`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handleNetwork(c, args)
		},
	},

	// --- Volume commands ---
	"volume": {
		Name: "volume",
		Help: `Usage: doki volume COMMAND [OPTIONS]

Commands:
  ls          List volumes
  create      Create a volume
  rm          Remove a volume
  inspect     Display volume details
  prune       Remove unused volumes`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handleVolume(c, args)
		},
	},

	// --- System commands ---
	"system": {
		Name: "system",
		Help: `Usage: doki system COMMAND [OPTIONS]

Commands:
  info        Display system-wide information
  df          Show disk usage
  prune       Remove unused data
  events      Get real-time events
  dial-stdio  Proxy stdio to daemon`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handleSystem(c, args)
		},
	},
	"info": {
		Name: "info",
		Help: `Usage: doki info

Display system-wide information.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.SystemInfo()
		},
	},
	"version": {
		Name: "version",
		Help: `Usage: doki version

Show the Doki version information.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.SystemVersion()
		},
	},
	"events": {
		Name: "events",
		Help: `Usage: doki events [OPTIONS]

Get real-time events from the server.

Options:
  -f, --filter  Filter output based on conditions`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			filter := flagStr(args, "-f", "--filter")
			return c.Events(filter)
		},
	},
	"login": {
		Name: "login",
		Help: `Usage: doki login [OPTIONS] [SERVER]

Log in to a registry.

Options:
  -s, --server SERVER  Registry address
  -u, --username USER  Username
  -p, --password PASS  Password`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			server := flagStr(args, "-s", "--server")
			username := flagStr(args, "-u", "--username")
			password := flagStr(args, "-p", "--password")
			return c.Login(server, username, password)
		},
	},
	"logout": {
		Name: "logout",
		Help: `Usage: doki logout [SERVER]

Log out from a registry.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			server := ""
			if len(args) > 0 {
				server = args[0]
			}
			return c.Logout(server)
		},
	},
	"ping": {
		Name: "ping",
		Help: `Usage: doki ping

Check if the Doki daemon is running.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Ping()
		},
	},

	// --- Podman-specific ---
	"pod": {
		Name: "pod",
		Help: `Usage: doki pod COMMAND [OPTIONS]

Commands:
  create      Create a pod
  ps          List pods
  rm          Remove a pod
  start       Start a pod
  stop        Stop a pod`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handlePod(c, args)
		},
	},
	"generate": {
		Name: "generate",
		Help: `Usage: doki generate COMMAND

Generate Kubernetes YAML from Doki objects.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handleGenerate(c, args)
		},
	},
	"play": {
		Name: "play",
		Help: `Usage: doki play FILE

Play Kubernetes YAML.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handlePlay(c, args)
		},
	},
	"auto-update": {
		Name: "auto-update",
		Help: `Usage: doki auto-update

Auto update containers according to their auto-update policy.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.AutoUpdate()
		},
	},
	"unshare": {
		Name: "unshare",
		Help: `Usage: doki unshare [COMMAND]

Run a command in a modified user namespace.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Unshare(args)
		},
	},
	"untag": {
		Name: "untag",
		Help: `Usage: doki untag IMAGE [IMAGE...]

Remove a tag from an image.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			for _, a := range args {
				if err := c.Untag(a); err != nil {
					return err
				}
			}
			return nil
		},
	},
	"mount": {
		Name: "mount",
		Help: `Usage: doki mount CONTAINER [CONTAINER...]

Mount a container's root filesystem.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Mount(cleanIDs(args))
		},
	},
	"unmount": {
		Name: "unmount",
		Help: `Usage: doki unmount CONTAINER [CONTAINER...]

Unmount a container's root filesystem.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return c.Unmount(cleanIDs(args))
		},
	},
	"healthcheck": {
		Name: "healthcheck",
		Help: `Usage: doki healthcheck CONTAINER

Check the health status of a container.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) > 0 {
				return c.Healthcheck(args[0])
			}
			return nil
		},
	},

	// --- Kubernetes ---
	"kube": {
		Name: "kube",
		Help: `Usage: doki kube COMMAND

Commands:
  play     Play Kubernetes YAML
  down     Tear down Kubernetes resources
  generate Generate Kubernetes YAML from Doki objects`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			return handleKube(c, args)
		},
	},
	"apply": {
		Name: "apply",
		Help: `Usage: doki apply -f FILE

Apply Kubernetes YAML.`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			if len(args) >= 2 && args[0] == "-f" {
				return c.Apply(args[1])
			}
			return nil
		},
	},
	"scout": {
		Name: "scout",
		Help: `Usage: doki scout IMAGE

Scan an image for known vulnerabilities (stub).`,
		Handler: func(c *cli.DokiCLI, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return c.Scout(target)
		},
	},
	"scan": {
		Name: "scan",
		Help: "Alias for doki scout.",
		Handler: func(c *cli.DokiCLI, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return c.Scout(target)
		},
	},
}

// groupCommands are top-level management command groups that proxy to existing commands.
var groupCommands = map[string]*Command{
	"container": {
		Name: "container",
		Help: `Usage: doki container COMMAND

Manage containers.

Commands:
  ls          List containers
  rm          Remove one or more containers`,
	},
	"image": {
		Name: "image",
		Help: `Usage: doki image COMMAND

Manage images.

Commands:
  ls          List images`,
	},
}

// Build groupCommands sub-command proxies by referencing the original commands.
func init() {
	groupCommands["container"].SubCommands = map[string]*Command{
		"ls":  commands["ps"],
		"rm":  commands["rm"],
	}
	groupCommands["image"].SubCommands = map[string]*Command{
		"ls": commands["images"],
	}
}

func handleNetwork(c *cli.DokiCLI, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "ls", "list":
		return c.NetworkLs(false, false, "", "")
	case "create":
		name, driver, subnet, gw := "", "bridge", "", ""
		sub := args[1:]
		for i := 0; i < len(sub); i++ {
			a := sub[i]
			switch {
			case a == "-d" || a == "--driver":
				i++
				if i < len(sub) {
					driver = sub[i]
				}
			case a == "--subnet":
				i++
				if i < len(sub) {
					subnet = sub[i]
				}
			case a == "--gateway":
				i++
				if i < len(sub) {
					gw = sub[i]
				}
			case !strings.HasPrefix(a, "-"):
				name = a
			}
		}
		return c.NetworkCreate(name, driver, false, false, subnet, gw, nil)
	case "rm", "remove":
		return c.NetworkRm(args[1:])
	case "inspect":
		return c.NetworkInspect(args[1:])
	case "connect":
		if len(args) >= 3 {
			return c.NetworkConnect(args[1], args[2], nil)
		}
		return nil
	case "disconnect":
		if len(args) >= 3 {
			return c.NetworkDisconnect(args[1], args[2], false)
		}
		return nil
	case "prune":
		return c.NetworkPrune("")
	default:
		return fmt.Errorf("doki network: '%s' is not a valid subcommand", args[0])
	}
}

func handleVolume(c *cli.DokiCLI, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "ls", "list":
		return c.VolumeLs(false, "")
	case "create":
		name, driver := "", "local"
		sub := args[1:]
		for i := 0; i < len(sub); i++ {
			a := sub[i]
			switch {
			case a == "-d" || a == "--driver":
				i++
				if i < len(sub) {
					driver = sub[i]
				}
			case !strings.HasPrefix(a, "-"):
				name = a
			}
		}
		if name == "" {
			name = common.GenerateID(32)
		}
		return c.VolumeCreate(name, driver, nil, nil)
	case "rm", "remove":
		return c.VolumeRm(args[1:], false)
	case "inspect":
		return c.VolumeInspect(args[1:])
	case "prune":
		return c.VolumePrune("")
	default:
		return fmt.Errorf("doki volume: '%s' is not a valid subcommand", args[0])
	}
}

func handleSystem(c *cli.DokiCLI, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "info":
		return c.SystemInfo()
	case "df":
		verbose := false
		for _, a := range args {
			if a == "-v" || a == "--verbose" {
				verbose = true
			}
		}
		return c.SystemDf(verbose)
	case "prune":
		all := false
		volumes := false
		for _, a := range args {
			if a == "-a" || a == "--all" {
				all = true
			}
			if a == "--volumes" {
				volumes = true
			}
		}
		return c.SystemPrune(all, volumes, "")
	case "events":
		return c.SystemEvents("", "", "")
	case "dial-stdio":
		return c.SystemDialStdio()
	default:
		return fmt.Errorf("doki system: '%s' is not a valid subcommand", args[0])
	}
}

func handlePod(c *cli.DokiCLI, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "create":
		_, err := c.PodCreate(args[1], nil)
		return err
	case "ps", "ls", "list":
		return c.PodPs()
	case "rm":
		return c.PodRm(args[1:], false)
	case "start":
		return c.PodStart(args[1:])
	case "stop":
		return c.PodStop(args[1:])
	}
	return nil
}

func handleGenerate(c *cli.DokiCLI, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "kube":
		containerID := ""
		for i, a := range args {
			if a == "-s" || a == "--service" {
				if len(args) > i+1 {
					containerID = args[i+1]
				}
			} else if !strings.HasPrefix(a, "-") && a != "kube" {
				containerID = a
			}
		}
		return c.GenerateKube(containerID, true)
	}
	return nil
}

func handlePlay(c *cli.DokiCLI, args []string) error {
	if len(args) < 1 {
		return nil
	}
	return c.KubePlay(args[0])
}

func handleKube(c *cli.DokiCLI, args []string) error {
	if len(args) == 0 {
		return nil
	}
	switch args[0] {
	case "play":
		if len(args) > 1 {
			return c.KubePlay(args[1])
		}
	case "down":
		if len(args) > 1 {
			return c.KubeDown(args[1])
		}
	case "generate":
		return c.KubeGenerate(args[1])
	}
	return nil
}

func handleError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printMainHelp() {
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
	if c, ok := commands[cmd]; ok {
		fmt.Println(c.Help)
	} else if g, ok := groupCommands[cmd]; ok {
		fmt.Println(g.Help)
	} else {
		fmt.Printf("Usage: doki %s [OPTIONS]\n", cmd)
		fmt.Println("See 'doki --help' for available commands.")
	}
}

func flagBool(args []string, names ...string) bool {
	for _, n := range names {
		for _, a := range args {
			if a == n {
				return true
			}
			if strings.HasPrefix(a, n+"=") {
				val := strings.TrimPrefix(a, n+"=")
				if val == "false" || val == "0" {
					return false
				}
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
			if strings.HasPrefix(a, n+"=") {
				return strings.TrimPrefix(a, n+"=")
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
			if strings.HasPrefix(a, n+"=") {
				result = append(result, strings.TrimPrefix(a, n+"="))
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

// forEachID iterates over non-flag arguments and calls fn for each.
func forEachID(args []string, fn func(id string) error) {
	ids := cleanIDs(args)
	for _, id := range ids {
		if err := fn(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	}
}

// printTable prints aligned tabular output.
func printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 14, 0, 1, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// streamResponse copies a streaming response body to stdout and closes it.
func streamResponse(body io.ReadCloser) {
	defer body.Close()
	io.Copy(os.Stdout, body)
}

// runWithDistro runs a command inside a predefined distro rootfs using proot.
func runWithDistro(c *cli.DokiCLI, distroName string, args []string) error {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/data/data/com.termux/files/home"
	}

	// Resolve version: alpine:3.19 -> alpine, 3.19
	parts := strings.SplitN(distroName, ":", 2)
	name := parts[0]

	rootfsPath := filepath.Join(home, ".doki", "distros", name, "rootfs")

	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		// Try to docker pull the image
		fmt.Fprintf(os.Stderr, "Distro '%s' not installed. Pulling image...\n", name)
		// TODO: implement distro pull via image.Store
		// For now, create minimal rootfs
		if err := createMinimalRootfs(rootfsPath); err != nil {
			return fmt.Errorf("failed to prepare distro rootfs: %w", err)
		}
	}

	return runInProot(rootfsPath, args)
}

// createMinimalRootfs creates a minimal rootfs structure for a distro.
func createMinimalRootfs(path string) error {
	dirs := []string{
		"bin", "sbin", "dev", "proc", "sys", "tmp", "run",
		"etc", "var", "home", "root", "opt",
		"usr/bin", "usr/sbin", "usr/lib", "usr/share", "usr/local/bin",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(path, d), 0755); err != nil {
			return err
		}
	}
	return nil
}

// runInProot executes a command inside a rootfs using doki-proot or system proot.
func runInProot(rootfs string, args []string) error {
	prootBin := "proot"
	// Prefer doki-proot if available
	if _, err := os.Stat("doki-proot"); err == nil {
		prootBin = "doki-proot"
	}

	// Remove --distro flag and its value from args
	var cleanArgs []string
	skipNext := false
	for _, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if a == "--distro" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(a, "--distro=") {
			continue
		}
		cleanArgs = append(cleanArgs, a)
	}

	if len(cleanArgs) == 0 {
		cleanArgs = []string{"/bin/sh"}
	}

	prootArgs := []string{
		"-r", rootfs,
		"-b", "/proc",
		"-b", "/sys",
		"-b", "/dev",
		"--kill-on-exit",
		"--link2symlink",
	}
	prootArgs = append(prootArgs, cleanArgs...)

	cmd := exec.Command(prootBin, prootArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	return cmd.Run()
}
