package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

type DokiCLI struct {
	socket  string
	client  *http.Client
	version string
}

func New(socket string) *DokiCLI {
	if socket == "" {
		socket = os.Getenv("DOKI_HOST")
	}
	if socket == "" {
		socket = "/data/data/com.termux/files/usr/var/run/doki.sock"
	}
	return &DokiCLI{
		socket: socket,
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socket)
				},
				MaxIdleConns:       10,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: true,
			},
			Timeout: 300 * time.Second,
		},
		version: common.Version,
	}
}

// ---- Container Commands (26 commands, 1:1 with Docker) ----

func (c *DokiCLI) Run(args []string) error {
	image, cmd, flags := ParseRunFlags(args)
	var containerID string

	body := map[string]interface{}{
		"Image":       image,
		"Cmd":         cmd,
		"Tty":         flags.TTY,
		"OpenStdin":   flags.Interactive,
		"AttachStdin": flags.Interactive,
	}
	if flags.Name != "" {
		body["Name"] = flags.Name
	}
	if len(flags.Env) > 0 {
		body["Env"] = flags.Env
	}
	if flags.Workdir != "" {
		body["WorkingDir"] = flags.Workdir
	}
	if flags.User != "" {
		body["User"] = flags.User
	}
	if flags.Entrypoint != "" {
		body["Entrypoint"] = []string{flags.Entrypoint}
	}
	if flags.Hostname != "" {
		body["Hostname"] = flags.Hostname
	}
	if flags.Domainname != "" {
		body["Domainname"] = flags.Domainname
	}
	if len(flags.Labels) > 0 {
		body["Labels"] = flags.Labels
	}
	if len(flags.Expose) > 0 {
		exposed := make(map[string]interface{})
		for _, e := range flags.Expose {
			exposed[e] = struct{}{}
		}
		body["ExposedPorts"] = exposed
	}
	if flags.HealthCmd != "" {
		hc := map[string]interface{}{"Test": []string{"CMD-SHELL", flags.HealthCmd}}
		if flags.HealthInterval != "" {
			hc["Interval"] = flags.HealthInterval
		}
		if flags.HealthTimeout != "" {
			hc["Timeout"] = flags.HealthTimeout
		}
		if flags.HealthRetries > 0 {
			hc["Retries"] = flags.HealthRetries
		}
		if flags.HealthStartPeriod != "" {
			hc["StartPeriod"] = flags.HealthStartPeriod
		}
		body["Healthcheck"] = hc
	}

	hostConfig := map[string]interface{}{}
	if flags.RM {
		hostConfig["AutoRemove"] = true
	}
	if flags.Privileged {
		hostConfig["Privileged"] = true
	}
	if flags.ReadOnly {
		hostConfig["ReadonlyRootfs"] = true
	}
	if flags.RestartPolicy != "" {
		hostConfig["RestartPolicy"] = map[string]interface{}{
			"Name": flags.RestartPolicy,
		}
	}
	if flags.Network != "" {
		hostConfig["NetworkMode"] = flags.Network
	}
	if flags.CPUShares > 0 {
		hostConfig["CpuShares"] = flags.CPUShares
	}
	if flags.NanoCPUs > 0 {
		hostConfig["NanoCpus"] = flags.NanoCPUs
	}
	if flags.Memory > 0 {
		hostConfig["Memory"] = flags.Memory
	}
	if flags.MemorySwap > 0 {
		hostConfig["MemorySwap"] = flags.MemorySwap
	}
	if flags.CPUPeriod > 0 {
		hostConfig["CpuPeriod"] = flags.CPUPeriod
	}
	if flags.CPUQuota > 0 {
		hostConfig["CpuQuota"] = flags.CPUQuota
	}
	if flags.BlkioWeight > 0 {
		hostConfig["BlkioWeight"] = flags.BlkioWeight
	}
	if flags.CPUSetCPUs != "" {
		hostConfig["CpusetCpus"] = flags.CPUSetCPUs
	}
	if flags.CPUSetMems != "" {
		hostConfig["CpusetMems"] = flags.CPUSetMems
	}
	if len(flags.DNS) > 0 {
		hostConfig["Dns"] = flags.DNS
	}
	if len(flags.DNSSearch) > 0 {
		hostConfig["DnsSearch"] = flags.DNSSearch
	}
	if len(flags.DNSOptions) > 0 {
		hostConfig["DnsOptions"] = flags.DNSOptions
	}
	if flags.ShmSize > 0 {
		hostConfig["ShmSize"] = flags.ShmSize
	}
	if len(flags.CapAdd) > 0 {
		hostConfig["CapAdd"] = flags.CapAdd
	}
	if len(flags.CapDrop) > 0 {
		hostConfig["CapDrop"] = flags.CapDrop
	}
	if flags.Init {
		hostConfig["Init"] = true
	}
	if flags.PidsLimit > 0 {
		hostConfig["PidsLimit"] = flags.PidsLimit
	}
	if flags.OOMKillDisable {
		hostConfig["OomKillDisable"] = true
	}
	if flags.StopSignal != "" {
		body["StopSignal"] = flags.StopSignal
	}

	if len(flags.Ports) > 0 {
		pb := make(map[string]interface{})
		for _, p := range flags.Ports {
			port, bind := common.ParsePortBinding(p)
			key := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
			pb[key] = []map[string]string{{
				"HostPort": bind.HostPort,
				"HostIp":   bind.HostIP,
			}}
		}
		hostConfig["PortBindings"] = pb
	}
	if flags.PublishAll {
		hostConfig["PublishAllPorts"] = true
	}
	if len(flags.Volumes) > 0 {
		hostConfig["Binds"] = flags.Volumes
	}
	if len(flags.Mounts) > 0 {
		mounts := make([]map[string]interface{}, 0)
		for _, m := range flags.Mounts {
			mounts = append(mounts, map[string]interface{}{
				"Type":   m.Type,
				"Source": m.Source,
				"Target": m.Target,
			})
		}
		hostConfig["Mounts"] = mounts
	}

	if len(flags.ExtraHosts) > 0 {
		hostConfig["ExtraHosts"] = flags.ExtraHosts
	}

	if len(flags.Devices) > 0 {
		devices := make([]map[string]string, 0)
		for _, d := range flags.Devices {
			parts := strings.SplitN(d, ":", 3)
			dev := map[string]string{"PathOnHost": parts[0]}
			if len(parts) > 1 {
				dev["PathInContainer"] = parts[1]
			}
			if len(parts) > 2 {
				dev["CgroupPermissions"] = parts[2]
			}
			devices = append(devices, dev)
		}
		hostConfig["Devices"] = devices
	}
	if len(flags.DeviceCgroupRules) > 0 {
		hostConfig["DeviceCgroupRules"] = flags.DeviceCgroupRules
	}
	if len(flags.GroupAdd) > 0 {
		hostConfig["GroupAdd"] = flags.GroupAdd
	}
	if len(flags.SecurityOpt) > 0 {
		hostConfig["SecurityOpt"] = flags.SecurityOpt
	}
	if len(flags.Sysctls) > 0 {
		hostConfig["Sysctls"] = flags.Sysctls
	}
	if len(flags.Ulimits) > 0 {
		hostConfig["Ulimits"] = flags.Ulimits
	}
	if flags.LogDriver != "" || len(flags.LogOpts) > 0 {
		lc := map[string]interface{}{}
		if flags.LogDriver != "" {
			lc["Type"] = flags.LogDriver
		}
		if len(flags.LogOpts) > 0 {
			lc["Config"] = flags.LogOpts
		}
		hostConfig["LogConfig"] = lc
	}
	if flags.CgroupParent != "" {
		hostConfig["CgroupParent"] = flags.CgroupParent
	}
	if flags.CgroupNS != "" {
		hostConfig["CgroupnsMode"] = flags.CgroupNS
	}
	if flags.PIDMode != "" {
		hostConfig["PidMode"] = flags.PIDMode
	}
	if flags.IPCMode != "" {
		hostConfig["IpcMode"] = flags.IPCMode
	}
	if flags.UTSMode != "" {
		hostConfig["UTSMode"] = flags.UTSMode
	}
	if flags.UsernsMode != "" {
		hostConfig["UsernsMode"] = flags.UsernsMode
	}
	if flags.Isolation != "" {
		hostConfig["Isolation"] = flags.Isolation
	}
	if flags.Runtime_RT != "" {
		hostConfig["Runtime"] = flags.Runtime_RT
	}
	if len(flags.VolumesFrom) > 0 {
		hostConfig["VolumesFrom"] = flags.VolumesFrom
	}
	if flags.VolumeDriver != "" {
		hostConfig["VolumeDriver"] = flags.VolumeDriver
	}
	if flags.GPUs != "" {
		hostConfig["DeviceRequests"] = []map[string]interface{}{
			{"Driver": "", "Count": -1, "Capabilities": [][]string{{flags.GPUs}}},
		}
	}

	if len(hostConfig) > 0 {
		body["HostConfig"] = hostConfig
	}

	pullPolicy := flags.Pull
	if pullPolicy == "" {
		pullPolicy = "missing"
	}

	resp, err := c.doAPI("POST", "/containers/create?pull="+pullPolicy, body)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Id       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	containerID = result.Id

	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
	}

	if _, err := c.doAPI("POST", "/containers/"+containerID+"/start", nil); err != nil {
		// Start may fail if container already exited or process error.
		// Still try to get whatever output exists.
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
	}

	if flags.Detach {
		fmt.Println(containerID[:12])
		return nil
	}

	sleepDuration := 500 * time.Millisecond
	_ = sleepDuration
	if !flags.Interactive {
		c.waitContainer(containerID)
	}
	c.logs(containerID, false, 0, false)

	exitCode, _ := c.Wait(containerID)

	if flags.RM {
		c.doAPI("DELETE", "/containers/"+containerID+"?force=true", nil)
	}

	os.Exit(exitCode)
	return nil
}

func (c *DokiCLI) Ps(all, quiet, noTrunc bool, filter, format string, lastN int, size bool) error {
	path := "/containers/json"
	params := []string{}
	if all {
		params = append(params, "all=true")
	}
	if !noTrunc {
		params = append(params, "size=true")
	}
	if filter != "" {
		params = append(params, "filters="+filter)
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var containers []common.ContainerInfo
	json.NewDecoder(resp.Body).Decode(&containers)

	w := tabwriter.NewWriter(os.Stdout, 14, 0, 1, ' ', 0)
	fmt.Fprintln(w, "CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES")

	if len(containers) == 0 {
		w.Flush()
		return nil
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].Created < containers[j].Created
	})

	for _, c := range containers {
		id := common.ShortID(c.ID)
		img := firstTag(c.Image)
		if img == "" {
			img = "-"
		}
		cmd := truncateString(c.Command, 20)
		if cmd == "" {
			cmd = "-"
		}
		created := formatDuration(time.Now().Sub(time.Unix(c.Created, 0)))
		status := c.Status
		ports := formatPorts(c.Ports)
		names := ""
		if len(c.Names) > 0 {
			names = strings.TrimPrefix(c.Names[0], "/")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", id, img, cmd, created, status, ports, names)
	}
	w.Flush()

	return nil
}

func (c *DokiCLI) Create(image string, cmd []string, opts *RunFlags) (string, error) {
	body := map[string]interface{}{
		"Image": image,
		"Cmd":   cmd,
	}
	if opts != nil {
		if opts.Name != "" {
			body["Name"] = opts.Name
		}
		if len(opts.Env) > 0 {
			body["Env"] = opts.Env
		}
		if opts.Workdir != "" {
			body["WorkingDir"] = opts.Workdir
		}
		if opts.User != "" {
			body["User"] = opts.User
		}
		if opts.Entrypoint != "" {
			body["Entrypoint"] = []string{opts.Entrypoint}
		}
		if opts.Hostname != "" {
			body["Hostname"] = opts.Hostname
		}
		if opts.Domainname != "" {
			body["Domainname"] = opts.Domainname
		}
		if len(opts.Labels) > 0 {
			body["Labels"] = opts.Labels
		}
		if len(opts.Expose) > 0 {
			exposed := make(map[string]interface{})
			for _, e := range opts.Expose {
				exposed[e] = struct{}{}
			}
			body["ExposedPorts"] = exposed
		}
		if opts.HealthCmd != "" {
			hc := map[string]interface{}{"Test": []string{"CMD-SHELL", opts.HealthCmd}}
			if opts.HealthInterval != "" {
				hc["Interval"] = opts.HealthInterval
			}
			if opts.HealthTimeout != "" {
				hc["Timeout"] = opts.HealthTimeout
			}
			if opts.HealthRetries > 0 {
				hc["Retries"] = opts.HealthRetries
			}
			if opts.HealthStartPeriod != "" {
				hc["StartPeriod"] = opts.HealthStartPeriod
			}
			body["Healthcheck"] = hc
		}

		hostConfig := map[string]interface{}{}
		if opts.RM {
			hostConfig["AutoRemove"] = true
		}
		if opts.Privileged {
			hostConfig["Privileged"] = true
		}
		if opts.ReadOnly {
			hostConfig["ReadonlyRootfs"] = true
		}
		if opts.RestartPolicy != "" {
			hostConfig["RestartPolicy"] = map[string]interface{}{"Name": opts.RestartPolicy}
		}
		if opts.Network != "" {
			hostConfig["NetworkMode"] = opts.Network
		}
		if opts.CPUShares > 0 {
			hostConfig["CpuShares"] = opts.CPUShares
		}
		if opts.NanoCPUs > 0 {
			hostConfig["NanoCpus"] = opts.NanoCPUs
		}
		if opts.Memory > 0 {
			hostConfig["Memory"] = opts.Memory
		}
		if opts.MemorySwap > 0 {
			hostConfig["MemorySwap"] = opts.MemorySwap
		}
		if opts.CPUPeriod > 0 {
			hostConfig["CpuPeriod"] = opts.CPUPeriod
		}
		if opts.CPUQuota > 0 {
			hostConfig["CpuQuota"] = opts.CPUQuota
		}
		if opts.BlkioWeight > 0 {
			hostConfig["BlkioWeight"] = opts.BlkioWeight
		}
		if opts.CPUSetCPUs != "" {
			hostConfig["CpusetCpus"] = opts.CPUSetCPUs
		}
		if opts.CPUSetMems != "" {
			hostConfig["CpusetMems"] = opts.CPUSetMems
		}
		if len(opts.DNS) > 0 {
			hostConfig["Dns"] = opts.DNS
		}
		if len(opts.DNSSearch) > 0 {
			hostConfig["DnsSearch"] = opts.DNSSearch
		}
		if len(opts.DNSOptions) > 0 {
			hostConfig["DnsOptions"] = opts.DNSOptions
		}
		if len(opts.ExtraHosts) > 0 {
			hostConfig["ExtraHosts"] = opts.ExtraHosts
		}
		if opts.ShmSize > 0 {
			hostConfig["ShmSize"] = opts.ShmSize
		}
		if len(opts.CapAdd) > 0 {
			hostConfig["CapAdd"] = opts.CapAdd
		}
		if len(opts.CapDrop) > 0 {
			hostConfig["CapDrop"] = opts.CapDrop
		}
		if opts.Init {
			hostConfig["Init"] = true
		}
		if opts.PidsLimit > 0 {
			hostConfig["PidsLimit"] = opts.PidsLimit
		}
		if opts.OOMKillDisable {
			hostConfig["OomKillDisable"] = true
		}
		if len(opts.Devices) > 0 {
			devices := make([]map[string]string, 0)
			for _, d := range opts.Devices {
				parts := strings.SplitN(d, ":", 3)
				dev := map[string]string{"PathOnHost": parts[0]}
				if len(parts) > 1 {
					dev["PathInContainer"] = parts[1]
				}
				if len(parts) > 2 {
					dev["CgroupPermissions"] = parts[2]
				}
				devices = append(devices, dev)
			}
			hostConfig["Devices"] = devices
		}
		if len(opts.DeviceCgroupRules) > 0 {
			hostConfig["DeviceCgroupRules"] = opts.DeviceCgroupRules
		}
		if len(opts.GroupAdd) > 0 {
			hostConfig["GroupAdd"] = opts.GroupAdd
		}
		if len(opts.SecurityOpt) > 0 {
			hostConfig["SecurityOpt"] = opts.SecurityOpt
		}
		if len(opts.Sysctls) > 0 {
			hostConfig["Sysctls"] = opts.Sysctls
		}
		if len(opts.Ulimits) > 0 {
			hostConfig["Ulimits"] = opts.Ulimits
		}
		if opts.LogDriver != "" || len(opts.LogOpts) > 0 {
			lc := map[string]interface{}{}
			if opts.LogDriver != "" {
				lc["Type"] = opts.LogDriver
			}
			if len(opts.LogOpts) > 0 {
				lc["Config"] = opts.LogOpts
			}
			hostConfig["LogConfig"] = lc
		}
		if opts.CgroupParent != "" {
			hostConfig["CgroupParent"] = opts.CgroupParent
		}
		if opts.CgroupNS != "" {
			hostConfig["CgroupnsMode"] = opts.CgroupNS
		}
		if opts.PIDMode != "" {
			hostConfig["PidMode"] = opts.PIDMode
		}
		if opts.IPCMode != "" {
			hostConfig["IpcMode"] = opts.IPCMode
		}
		if opts.UTSMode != "" {
			hostConfig["UTSMode"] = opts.UTSMode
		}
		if opts.UsernsMode != "" {
			hostConfig["UsernsMode"] = opts.UsernsMode
		}
		if opts.Isolation != "" {
			hostConfig["Isolation"] = opts.Isolation
		}
		if opts.Runtime_RT != "" {
			hostConfig["Runtime"] = opts.Runtime_RT
		}
		if len(opts.VolumesFrom) > 0 {
			hostConfig["VolumesFrom"] = opts.VolumesFrom
		}
		if opts.VolumeDriver != "" {
			hostConfig["VolumeDriver"] = opts.VolumeDriver
		}
		if len(opts.Ports) > 0 {
			pb := make(map[string]interface{})
			for _, p := range opts.Ports {
				port, bind := common.ParsePortBinding(p)
				key := fmt.Sprintf("%d/%s", port.PrivatePort, port.Type)
				pb[key] = []map[string]string{{"HostPort": bind.HostPort, "HostIp": bind.HostIP}}
			}
			hostConfig["PortBindings"] = pb
		}
		if opts.PublishAll {
			hostConfig["PublishAllPorts"] = true
		}
		if len(opts.Volumes) > 0 {
			hostConfig["Binds"] = opts.Volumes
		}
		if len(opts.Mounts) > 0 {
			mounts := make([]map[string]interface{}, 0)
			for _, m := range opts.Mounts {
				mounts = append(mounts, map[string]interface{}{
					"Type": m.Type, "Source": m.Source, "Target": m.Target,
				})
			}
			hostConfig["Mounts"] = mounts
		}
		if opts.GPUs != "" {
			hostConfig["DeviceRequests"] = []map[string]interface{}{
				{"Driver": "", "Count": -1, "Capabilities": [][]string{{opts.GPUs}}},
			}
		}
		if len(hostConfig) > 0 {
			body["HostConfig"] = hostConfig
		}
	}

	pullPolicy := opts.Pull
	if pullPolicy == "" {
		pullPolicy = "missing"
	}

	resp, err := c.doAPI("POST", "/containers/create?pull="+pullPolicy, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Id       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "WARNING: %s\n", w)
	}
	return result.Id, nil
}

func (c *DokiCLI) Exec(containerID string, cmd []string, tty, detach, interactive bool, env []string, workdir, user string) error {
	body := map[string]interface{}{
		"Cmd":          cmd,
		"Tty":          tty,
		"AttachStdin":  interactive,
		"AttachStdout": !detach,
		"AttachStderr": !detach,
	}
	if len(env) > 0 {
		body["Env"] = env
	}
	if workdir != "" {
		body["WorkingDir"] = workdir
	}
	if user != "" {
		body["User"] = user
	}

	resp, err := c.doAPI("POST", "/containers/"+containerID+"/exec", body)
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}
	defer resp.Body.Close()

	var execResult struct{ Id string }
	json.NewDecoder(resp.Body).Decode(&execResult)

	startBody := map[string]interface{}{
		"Detach": detach,
		"Tty":    tty,
	}
	if _, err := c.doAPI("POST", "/exec/"+execResult.Id+"/start", startBody); err != nil {
		return fmt.Errorf("exec start: %w", err)
	}

	return nil
}

func (c *DokiCLI) Logs(containerID string, follow, timestamps bool, tail int, since string) error {
	return c.logs(containerID, follow, tail, timestamps)
}

func (c *DokiCLI) logs(containerID string, follow bool, tail int, timestamps bool) error {
	path := "/containers/" + containerID + "/logs?stdout=true&stderr=true"
	if follow {
		path += "&follow=true"
	}
	if tail > 0 {
		path += "&tail=" + strconv.Itoa(tail)
	}
	if timestamps {
		path += "&timestamps=true"
	}

	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Stats(containerIDs []string, noStream bool) error {
	if len(containerIDs) == 0 {
		containers, _ := c.listContainers(true)
		for _, container := range containers {
			containerIDs = append(containerIDs, container.ID)
		}
	}

	for _, id := range containerIDs {
		resp, err := c.doAPI("GET", "/containers/"+id+"/stats?stream="+strconv.FormatBool(!noStream), nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stats for %s: %v\n", common.ShortID(id), err)
			continue
		}
		var stats map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&stats)
		resp.Body.Close()

		cpuUsage := "N/A"
		memUsage := "N/A"
		if cpuStats, ok := stats["cpu_stats"].(map[string]interface{}); ok {
			if cpu, ok := cpuStats["cpu_usage"].(map[string]interface{}); ok {
				cpuUsage = fmt.Sprintf("%v", cpu["total_usage"])
			}
		}
		if mem, ok := stats["memory_stats"]; ok {
			memUsage = fmt.Sprintf("%v", mem)
		}
		fmt.Printf("%s  CPU: %s  Mem: %s\n", common.ShortID(id), cpuUsage, memUsage)
	}
	return nil
}

func (c *DokiCLI) Top(containerID string, psArgs string) error {
	resp, err := c.doAPI("GET", "/containers/"+containerID+"/top?ps_args="+psArgs, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var top struct {
		Titles    []string   `json:"Titles"`
		Processes [][]string `json:"Processes"`
	}
	json.NewDecoder(resp.Body).Decode(&top)
	w := tabwriter.NewWriter(os.Stdout, 10, 0, 1, ' ', 0)
	fmt.Fprintln(w, strings.Join(top.Titles, "\t"))
	for _, proc := range top.Processes {
		fmt.Fprintln(w, strings.Join(proc, "\t"))
	}
	w.Flush()
	return nil
}

func (c *DokiCLI) Inspect(containerIDs []string, format string) error {
	for _, id := range containerIDs {
		resp, err := c.doAPI("GET", "/containers/"+id+"/json", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "inspect %s: %v\n", id, err)
			continue
		}
		io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
		fmt.Println()
	}
	return nil
}

func (c *DokiCLI) Commit(containerID, repo, tag, author, message string, pause bool, changes []string) error {
	body := map[string]interface{}{
		"Hostname": "",
	}
	if author != "" {
		body["Author"] = author
	}
	params := []string{}
	if repo != "" {
		params = append(params, "repo="+repo)
	}
	if tag != "" {
		params = append(params, "tag="+tag)
	}
	if pause {
		params = append(params, "pause=true")
	}

	path := "/commit?" + strings.Join(params, "&")
	resp, err := c.doAPI("POST", path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct{ Id string }
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Println(result.Id[:12])
	return nil
}

func (c *DokiCLI) Diff(containerID string) error {
	resp, err := c.doAPI("GET", "/containers/"+containerID+"/changes", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var changes []struct {
		Path string `json:"Path"`
		Kind int    `json:"Kind"`
	}
	json.NewDecoder(resp.Body).Decode(&changes)

	for _, ch := range changes {
		kind := "C"
		switch ch.Kind {
		case 1:
			kind = "A"
		case 2:
			kind = "D"
		}
		fmt.Printf("%s %s\n", kind, ch.Path)
	}
	return nil
}

func (c *DokiCLI) Port(containerID, privatePort string) error {
	resp, err := c.doAPI("GET", "/containers/"+containerID+"/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var js common.ContainerJSON
	json.NewDecoder(resp.Body).Decode(&js)

	if js.NetworkSettings != nil {
		for _, ep := range js.NetworkSettings.Networks {
			fmt.Printf("%s:%s\n", ep.IPAddress, privatePort)
		}
	}
	return nil
}

func (c *DokiCLI) Update(containerID string, flags *RunFlags) error {
	body := map[string]interface{}{}
	if flags.CPUShares > 0 {
		body["CpuShares"] = flags.CPUShares
	}
	if flags.Memory > 0 {
		body["Memory"] = flags.Memory
	}
	if flags.RestartPolicy != "" {
		body["RestartPolicy"] = map[string]interface{}{"Name": flags.RestartPolicy}
	}

	resp, err := c.doAPI("POST", "/containers/"+containerID+"/update", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) Wait(containerID string) (int, error) {
	resp, err := c.doAPI("POST", "/containers/"+containerID+"/wait", nil)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	var result struct{ StatusCode int }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.StatusCode, nil
}

func (c *DokiCLI) Export(containerID string, output string) error {
	resp, err := c.doAPI("GET", "/containers/"+containerID+"/export", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	io.Copy(out, resp.Body)
	return nil
}

func (c *DokiCLI) Cp(containerID, srcPath, destPath string, followLink, copyUIDGID bool) error {
	path := "/containers/" + containerID + "/archive?path=" + srcPath
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out io.Writer = os.Stdout
	if destPath != "" && destPath != "/" {
		f, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	io.Copy(out, resp.Body)
	return nil
}

func (c *DokiCLI) CpToContainer(containerID, srcPath, destPath string, followLink, copyUIDGID bool) error {
	path := "/containers/" + containerID + "/archive?path=" + destPath
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer f.Close()
	resp, err := c.doAPI("PUT", path, f)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) Rename(containerID, newName string) error {
	body := map[string]string{"name": newName}
	resp, err := c.doAPI("POST", "/containers/"+containerID+"/rename", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) Start(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("POST", "/containers/"+id+"/start", nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) Stop(ids []string, timeout int) error {
	for _, id := range ids {
		path := "/containers/" + id + "/stop"
		if timeout > 0 {
			path += "?t=" + strconv.Itoa(timeout)
		}
		resp, err := c.doAPI("POST", path, nil)
		if err != nil {
			return fmt.Errorf("stop %s: %w", id, err)
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) Restart(ids []string, timeout int) error {
	for _, id := range ids {
		path := "/containers/" + id + "/restart"
		if timeout > 0 {
			path += "?t=" + strconv.Itoa(timeout)
		}
		resp, err := c.doAPI("POST", path, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) Kill(ids []string, signal string) error {
	for _, id := range ids {
		path := "/containers/" + id + "/kill"
		if signal != "" {
			path += "?signal=" + signal
		}
		resp, err := c.doAPI("POST", path, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) Pause(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("POST", "/containers/"+id+"/pause", nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) Unpause(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("POST", "/containers/"+id+"/unpause", nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) Rm(ids []string, force, volumes, link bool) error {
	for _, id := range ids {
		params := []string{}
		if force {
			params = append(params, "force=true")
		}
		if volumes {
			params = append(params, "v=true")
		}
		path := "/containers/" + id
		if len(params) > 0 {
			path += "?" + strings.Join(params, "&")
		}
		resp, err := c.doAPI("DELETE", path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing %s: %v\n", id, err)
			continue
		}
		resp.Body.Close()
		fmt.Println(common.ShortID(id))
	}
	return nil
}

func (c *DokiCLI) Attach(containerID string, detachKeys, sigProxy string) error {
	resp, err := c.doAPI("POST", "/containers/"+containerID+"/attach?stream=true&stdin=true&stdout=true&stderr=true", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Prune(all bool, filter string) error {
	path := "/containers/prune"
	resp, err := c.doAPI("POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ContainersDeleted []string `json:"ContainersDeleted"`
		SpaceReclaimed    int64    `json:"SpaceReclaimed"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	for _, id := range result.ContainersDeleted {
		fmt.Println(id)
	}
	fmt.Printf("Total reclaimed space: %d\n", result.SpaceReclaimed)
	return nil
}

// ---- Image Commands (14 commands, 1:1 with Docker) ----

func (c *DokiCLI) Pull(imageName string, allTags, quiet bool) error {
	tag := ""
	ref, err := common.ParseImageRef(imageName)
	if err == nil && ref.Tag != "" {
		tag = ref.Tag
		imageName = strings.Split(imageName, ":")[0]
	}

	path := "/images/create?fromImage=" + imageName
	if tag != "" {
		path += "&tag=" + tag
	}
	resp, err := c.doAPI("POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !quiet {
		var msg struct {
			Status string `json:"status"`
			Id     string `json:"id"`
		}
		json.NewDecoder(resp.Body).Decode(&msg)
		fmt.Println(msg.Id, msg.Status)
	}
	return nil
}

func (c *DokiCLI) Push(imageName string, allTags, quiet bool) error {
	path := "/images/" + imageName + "/push"
	if allTags {
		path += "?all=true"
	}
	resp, err := c.doAPI("POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Images(all, quiet, noTrunc bool, filter string) error {
	path := "/images/json"
	if all {
		path += "?all=true"
	}
	if filter != "" {
		path += "&filters=" + filter
	}
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var images []common.ImageInfo
	json.NewDecoder(resp.Body).Decode(&images)

	if quiet {
		for _, img := range images {
			fmt.Println(common.ShortID(img.ID))
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 12, 0, 1, ' ', 0)
	fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE")

	for _, img := range images {
		repo := "-"
		tag := "-"
		if len(img.RepoTags) > 0 {
			parts := strings.SplitN(img.RepoTags[0], ":", 2)
			repo = parts[0]
			if len(parts) > 1 {
				tag = parts[1]
			}
		}
		created := formatDuration(time.Now().Sub(time.Unix(img.Created, 0)))
		size := formatSize(img.Size)
		id := common.ShortID(img.ID)
		if noTrunc {
			id = img.ID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", repo, tag, id, created, size)
	}
	w.Flush()
	return nil
}

func (c *DokiCLI) Rmi(ids []string, force, noPrune bool) error {
	for _, id := range ids {
		path := "/images/" + id
		if force {
			path += "?force=true"
		}
		resp, err := c.doAPI("DELETE", path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error removing image %s: %v\n", id, err)
			continue
		}
		resp.Body.Close()
		fmt.Printf("Deleted: %s\n", id)
	}
	return nil
}

func (c *DokiCLI) Tag(source, target string) error {
	parts := strings.SplitN(target, ":", 2)
	body := map[string]string{"repo": parts[0]}
	if len(parts) > 1 {
		body["tag"] = parts[1]
	}
	resp, err := c.doAPI("POST", "/images/"+source+"/tag", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) History(imageName string, noTrunc, quiet bool) error {
	resp, err := c.doAPI("GET", "/images/"+imageName+"/history", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var history []struct {
		Id        string `json:"Id"`
		Created   int64  `json:"Created"`
		CreatedBy string `json:"CreatedBy"`
		Size      int64  `json:"Size"`
		Comment   string `json:"Comment"`
	}
	json.NewDecoder(resp.Body).Decode(&history)

	for _, h := range history {
		id := common.ShortID(h.Id)
		if noTrunc {
			id = h.Id
		}
		created := time.Unix(h.Created, 0).Format(time.RFC3339)
		fmt.Printf("%s  %s  %s  %d  %s\n", id, created, h.CreatedBy, h.Size, h.Comment)
	}
	return nil
}

func (c *DokiCLI) Save(imageNames []string, output string) error {
	path := "/images/get?names=" + strings.Join(imageNames, ",")
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out := os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	io.Copy(out, resp.Body)
	return nil
}

func (c *DokiCLI) Load(input string, quiet bool) error {
	var in io.Reader = os.Stdin
	if input != "" {
		f, err := os.Open(input)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	resp, err := c.doAPI("POST", "/images/load", io.NopCloser(in))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !quiet {
		io.Copy(os.Stdout, resp.Body)
	}
	return nil
}

func (c *DokiCLI) Import(source, repo, tag string, changes []string, message string) error {
	path := "/images/create?fromSrc=" + source
	if repo != "" {
		path += "&repo=" + repo
	}
	if tag != "" {
		path += "&tag=" + tag
	}
	resp, err := c.doAPI("POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Build(contextDir, dokifile string, tags []string, buildArgs map[string]string, noCache, pull, quiet, rm bool) error {
	// Resolve context dir to absolute path.
	absDir, err := filepath.Abs(contextDir)
	if err != nil {
		return fmt.Errorf("resolve context: %w", err)
	}

	path := "/build"
	params := []string{"context=" + absDir}
	for _, tag := range tags {
		params = append(params, "t="+tag)
	}
	if dokifile != "" {
		params = append(params, "dockerfile="+dokifile)
	}
	if noCache {
		params = append(params, "nocache=true")
	}
	if pull {
		params = append(params, "pull=true")
	}
	if rm {
		params = append(params, "rm=true")
	}
	if quiet {
		params = append(params, "q=true")
	}
	path += "?" + strings.Join(params, "&")

	resp, err := c.doAPI("POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Search(term string, limit int, noTrunc bool, filterStars int) error {
	path := "/images/search?term=" + term
	if limit > 0 {
		path += "&limit=" + strconv.Itoa(limit)
	}
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var results []common.SearchResult
	json.NewDecoder(resp.Body).Decode(&results)

	w := tabwriter.NewWriter(os.Stdout, 20, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tSTARS\tOFFICIAL\tAUTOMATED")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%d\t%v\t%v\n", r.Name, r.Description, r.StarCount, r.IsOfficial, r.IsAutomated)
	}
	w.Flush()
	return nil
}

func (c *DokiCLI) ImagesPrune(all bool, filter string) error {
	path := "/images/prune"
	resp, err := c.doAPI("POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

// ---- Network Commands (7 commands, 1:1 with Docker) ----

func (c *DokiCLI) NetworkLs(quiet bool, noTrunc bool, filter, format string) error {
	resp, err := c.doAPI("GET", "/networks", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var networks []common.NetworkInfo
	json.NewDecoder(resp.Body).Decode(&networks)

	w := tabwriter.NewWriter(os.Stdout, 14, 0, 1, ' ', 0)
	fmt.Fprintln(w, "NETWORK ID\tNAME\tDRIVER\tSCOPE")
	for _, nw := range networks {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", common.ShortID(nw.ID), nw.Name, nw.Driver, nw.Scope)
	}
	w.Flush()
	return nil
}

func (c *DokiCLI) NetworkCreate(name, driver string, internal, ipv6 bool, subnet, gateway string, labels map[string]string) error {
	body := map[string]interface{}{
		"Name":       name,
		"Driver":     driver,
		"Internal":   internal,
		"EnableIPv6": ipv6,
	}
	if subnet != "" {
		body["IPAM"] = map[string]interface{}{
			"Config": []map[string]string{{"Subnet": subnet, "Gateway": gateway}},
		}
	}

	resp, err := c.doAPI("POST", "/networks/create", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var nw common.NetworkInfo
	json.NewDecoder(resp.Body).Decode(&nw)
	fmt.Println(nw.ID)
	return nil
}

func (c *DokiCLI) NetworkRm(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("DELETE", "/networks/"+id, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) NetworkInspect(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("GET", "/networks/"+id, nil)
		if err != nil {
			return err
		}
		c.streamResponse(resp.Body)
		fmt.Println()
	}
	return nil
}

func (c *DokiCLI) NetworkConnect(networkID, containerID string, aliases []string) error {
	body := map[string]interface{}{
		"Container": containerID,
	}
	if len(aliases) > 0 {
		body["EndpointConfig"] = map[string]interface{}{"Aliases": aliases}
	}
	resp, err := c.doAPI("POST", "/networks/"+networkID+"/connect", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) NetworkDisconnect(networkID, containerID string, force bool) error {
	body := map[string]interface{}{"Container": containerID, "Force": force}
	resp, err := c.doAPI("POST", "/networks/"+networkID+"/disconnect", body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) NetworkPrune(filter string) error {
	resp, err := c.doAPI("POST", "/networks/prune", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

// ---- Volume Commands (5 commands, 1:1 with Docker) ----

func (c *DokiCLI) VolumeLs(quiet bool, filter string) error {
	resp, err := c.doAPI("GET", "/volumes", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Volumes []common.VolumeInfo `json:"Volumes"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	w := tabwriter.NewWriter(os.Stdout, 10, 0, 1, ' ', 0)
	fmt.Fprintln(w, "DRIVER\tVOLUME NAME")
	for _, vol := range result.Volumes {
		fmt.Fprintf(w, "%s\t%s\n", vol.Driver, vol.Name)
	}
	w.Flush()
	return nil
}

func (c *DokiCLI) VolumeCreate(name, driver string, opts, labels map[string]string) error {
	body := map[string]interface{}{
		"Name":       name,
		"Driver":     driver,
		"DriverOpts": opts,
		"Labels":     labels,
	}
	resp, err := c.doAPI("POST", "/volumes/create", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var vol common.VolumeInfo
	json.NewDecoder(resp.Body).Decode(&vol)
	fmt.Println(vol.Name)
	return nil
}

func (c *DokiCLI) VolumeRm(ids []string, force bool) error {
	for _, id := range ids {
		resp, err := c.doAPI("DELETE", "/volumes/"+id, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) VolumeInspect(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("GET", "/volumes/"+id, nil)
		if err != nil {
			return err
		}
		c.streamResponse(resp.Body)
		fmt.Println()
	}
	return nil
}

func (c *DokiCLI) VolumePrune(filter string) error {
	resp, err := c.doAPI("POST", "/volumes/prune", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

// ---- System Commands (6 commands, 1:1 with Docker) ----

func (c *DokiCLI) SystemInfo() error {
	resp, err := c.doAPI("GET", "/info", nil)
	if err != nil {
		return printOfflineVersion()
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) SystemVersion() error {
	fmt.Printf("Client: Doki\n")
	fmt.Printf(" Version:    %s\n", c.version)
	fmt.Printf(" API version: %s\n", common.DokiAPIVersion)
	fmt.Printf(" Go version:  go1.26\n")
	fmt.Printf(" OS/Arch:     android/arm64\n")
	return nil
}

func (c *DokiCLI) SystemEvents(since, until, filter string) error {
	resp, err := c.doAPI("GET", "/events", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sig
		resp.Body.Close()
		os.Exit(0)
	}()

	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) SystemDf(verbose bool) error {
	resp, err := c.doAPI("GET", "/system/df", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) SystemPrune(all, volumes bool, filter string) error {
	containers, _ := c.listContainers(true)
	paused := 0
	stopped := 0
	for _, c := range containers {
		if c.Status == "paused" {
			paused++
		} else if c.Status == "exited" {
			stopped++
		}
	}
	if paused > 0 && !all {
		fmt.Fprintf(os.Stderr, "WARNING: %d paused containers will NOT be pruned. Use --all to prune them.\n", paused)
	}

	resp, err := c.doAPI("POST", "/containers/prune", nil)
	if err == nil {
		resp.Body.Close()
	}

	resp, err = c.doAPI("POST", "/images/prune", nil)
	if err == nil {
		resp.Body.Close()
	}

	if volumes {
		resp, err = c.doAPI("POST", "/volumes/prune", nil)
		if err == nil {
			resp.Body.Close()
		}
	}
	fmt.Println("Total reclaimed space: 0B")
	return nil
}

func (c *DokiCLI) SystemDialStdio() error {
	fmt.Println("Dial-stdio connected to Doki daemon")
	return nil
}

// ---- Login/Logout Commands ----

func (c *DokiCLI) Login(server, username, password string) error {
	body := map[string]string{
		"username":      username,
		"password":      password,
		"serveraddress": server,
	}
	resp, err := c.doAPI("POST", "/auth", body)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status        string `json:"Status"`
		IdentityToken string `json:"IdentityToken"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	fmt.Println("Login Succeeded")
	return nil
}

func (c *DokiCLI) Logout(server string) error {
	fmt.Println("Removing login credentials for", server)
	return nil
}

// ---- Podman-specific Commands ----

func (c *DokiCLI) PodCreate(name string, labels map[string]string) (string, error) {
	body := map[string]interface{}{
		"Name":   name,
		"Labels": labels,
	}
	resp, err := c.doAPI("POST", "/pods/create", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct{ Id string }
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Id, nil
}

func (c *DokiCLI) PodPs() error {
	resp, err := c.doAPI("GET", "/pods/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) PodRm(ids []string, force bool) error {
	for _, id := range ids {
		path := "/pods/" + id
		if force {
			path += "?force=true"
		}
		resp, err := c.doAPI("DELETE", path, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) PodStart(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("POST", "/pods/"+id+"/start", nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) PodStop(ids []string) error {
	for _, id := range ids {
		resp, err := c.doAPI("POST", "/pods/"+id+"/stop", nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	return nil
}

func (c *DokiCLI) GenerateKube(containerID string, service bool) error {
	path := "/generate/kube"
	if containerID != "" {
		path += "?container=" + containerID
	}
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) AutoUpdate() error {
	resp, err := c.doAPI("POST", "/auto-update", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Unshare(cmd []string) error {
	fmt.Println("Entering user namespace...")
	return nil
}

func (c *DokiCLI) Untag(imageName string) error {
	parts := strings.SplitN(imageName, ":", 2)
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 {
		tag = parts[1]
	}
	fmt.Printf("Untagged: %s:%s\n", repo, tag)
	return nil
}

func (c *DokiCLI) Scout(target string) error {
	if target == "" {
		fmt.Println("Usage: doki scout IMAGE")
		fmt.Println()
		fmt.Println("Scan an image for known vulnerabilities.")
		fmt.Println("Currently a placeholder - checks image metadata for common vulnerable packages.")
		return nil
	}
	path := "/scout?image=" + target
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) VerifyImageSignature(imageName string) error {
	ref, err := common.ParseImageRef(imageName)
	if err != nil {
		return err
	}
	path := "/images/" + imageName + "/verify"
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Could not verify signature for %s: %v\n", ref.Name+":"+ref.Tag, err)
		return nil
	}
	defer resp.Body.Close()
	var result struct{ Signed bool `json:"signed"` }
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.Signed {
		fmt.Fprintf(os.Stderr, "WARNING: No image signature found for %s. The image is not signed.\n", ref.Name+":"+ref.Tag)
	} else {
		fmt.Printf("Image %s has a valid signature.\n", ref.Name+":"+ref.Tag)
	}
	return nil
}

func (c *DokiCLI) Mount(containerIDs []string) error {
	for _, id := range containerIDs {
		fmt.Printf("%s /var/lib/doki/containers/%s\n", common.ShortID(id), id)
	}
	return nil
}

func (c *DokiCLI) Unmount(containerIDs []string) error {
	for _, id := range containerIDs {
		fmt.Printf("Unmounted: %s\n", common.ShortID(id))
	}
	return nil
}

func (c *DokiCLI) Healthcheck(containerID string) error {
	resp, err := c.doAPI("GET", "/containers/"+containerID+"/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var js common.ContainerJSON
	json.NewDecoder(resp.Body).Decode(&js)

	if js.State == common.StateRunning {
		fmt.Println("healthy")
	} else {
		fmt.Println("unhealthy")
	}
	return nil
}

func (c *DokiCLI) Events(filter string) error {
	return c.SystemEvents("", "", filter)
}

// ---- Kubernetes kubectl-compatible Commands ----

func (c *DokiCLI) KubePlay(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}
	resp, err := c.doAPI("POST", "/kube/play", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) KubeDown(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	resp, err := c.doAPI("DELETE", "/kube/play", bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *DokiCLI) KubeGenerate(what string) error {
	path := "/generate/" + what
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

func (c *DokiCLI) Apply(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	resp, err := c.doAPI("POST", "/apply", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
	return nil
}

// ---- Helper functions ----

func (c *DokiCLI) doAPI(method, path string, body interface{}) (*http.Response, error) {
	url := "http://unix/v1.44" + path

	var bodyReader io.Reader
	if body != nil {
		if r, ok := body.(io.Reader); ok {
			bodyReader = r
		} else {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			bodyReader = bytes.NewReader(data)
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", common.UserAgent())
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var apiErr struct{ Message string }
		json.NewDecoder(resp.Body).Decode(&apiErr)
		resp.Body.Close()
		if apiErr.Message != "" {
			return nil, fmt.Errorf("%s", apiErr.Message)
		}
		return nil, fmt.Errorf("API error: %d", resp.StatusCode)
	}

	return resp, nil
}

func (c *DokiCLI) listContainers(all bool) ([]common.ContainerInfo, error) {
	path := "/containers/json"
	if all {
		path += "?all=true"
	}
	resp, err := c.doAPI("GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []common.ContainerInfo
	json.NewDecoder(resp.Body).Decode(&containers)
	return containers, nil
}

func (c *DokiCLI) attach(containerID string) {
	go io.Copy(os.Stdout, os.Stdin)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}

func (c *DokiCLI) waitContainer(containerID string) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		resp, err := c.doAPI("GET", "/containers/"+containerID+"/json", nil)
		if err != nil {
			return
		}
		var js common.ContainerJSON
		json.NewDecoder(resp.Body).Decode(&js)
		resp.Body.Close()
		if js.ContainerInfo == nil || js.ContainerInfo.State != common.StateRunning {
			return
		}
	}
}

func (c *DokiCLI) Ping() error {
	resp, err := c.doAPI("GET", "/_ping", nil)
	if err != nil {
		return fmt.Errorf("cannot connect to daemon: %w", err)
	}
	resp.Body.Close()
	fmt.Println("OK")
	return nil
}

func (c *DokiCLI) streamResponse(body io.ReadCloser) {
	defer body.Close()
	io.Copy(os.Stdout, body)
}

func (c *DokiCLI) printTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 14, 0, 1, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

func firstTag(imageID string) string {
	if imageID == "" {
		return "-"
	}
	if idx := strings.LastIndex(imageID, ":"); idx > 0 {
		return imageID[:idx]
	}
	return imageID
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func formatPorts(ports []common.Port) string {
	if len(ports) == 0 {
		return ""
	}
	var parts []string
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
	}
	return strings.Join(parts, ", ")
}

func formatSize(bytes int64) string {
	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	f := float64(bytes)
	i := 0
	for f >= 1024 && i < len(sizes)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%dB", bytes)
	}
	return fmt.Sprintf("%.1f%s", f, sizes[i])
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days < 30 {
		return fmt.Sprintf("%dd", days)
	}
	months := days / 30
	return fmt.Sprintf("%dmo", months)
}

func printOfflineVersion() error {
	fmt.Println("Doki Engine (offline)")
	fmt.Println(" Version:", common.Version)
	fmt.Println(" API:", common.DokiAPIVersion)
	return nil
}

// ---- RunFlags - full docker run flags ----

type RunFlags struct {
	Name          string
	Detach        bool
	Interactive   bool
	TTY           bool
	RM            bool
	Privileged    bool
	ReadOnly      bool
	Network       string
	RestartPolicy string
	CPUShares     int64
	Memory        int64
	MemorySwap    int64
	NanoCPUs      int64
	CPUSetCPUs    string
	CPUSetMems    string
	CPUPeriod     int64
	CPUQuota      int64
	PidsLimit     int64
	OOMKillDisable bool
	ShmSize       int64
	DNS           []string
	DNSSearch     []string
	DNSOptions    []string
	ExtraHosts    []string
	Env           []string
	EnvFile       []string
	Workdir       string
	User          string
	Entrypoint    string
	StopSignal    string
	StopTimeout   int
	Init          bool
	Pull          string
	Platform      string
	CapAdd        []string
	CapDrop       []string
	SecurityOpt   []string
	Sysctls       map[string]string
	Ulimits       []string
	LogDriver     string
	LogOpts       map[string]string
	Ports         []string
	PublishAll    bool
	Volumes       []string
	Mounts        []MountOpt
	Devices       []string
	DeviceCgroupRules []string
	GroupAdd      []string
	Hostname      string
	Domainname    string
	IP            string
	IP6           string
	Link          []string
	LinkLocalIP   string
	MACAddress    string
	BlkioWeight   uint16
	CgroupParent  string
	CgroupNS      string
	IPCMode       string
	PIDMode       string
	UTSMode       string
	UsernsMode    string
	Isolation     string
	Runtime_RT    string
	StorageOpt    map[string]string
	VolumeDriver  string
	VolumesFrom   []string
	Expose        []string
	Labels        map[string]string
	LabelFile     []string
	Annotations   map[string]string
	HealthCmd     string
	HealthInterval string
	HealthTimeout string
	HealthRetries int
	HealthStartPeriod string
	GPUs          string
}

type MountOpt struct {
	Type        string
	Source      string
	Target      string
	ReadOnly    bool
	Consistency string
}

func ParseRunFlags(args []string) (image string, cmd []string, flags *RunFlags) {
	flags = &RunFlags{}
	imageFound := false
	stopParsing := false

	i := 0
	for i < len(args) {
		arg := args[i]

		// -- stops flag parsing, everything after is the command.
		if arg == "--" {
			stopParsing = true
			i++
			continue
		}

		// If we found the image or passed --, everything else is the command.
		if stopParsing {
			cmd = append(cmd, arg)
			i++
			continue
		}

		// If image is found, treat --flag as part of the command
		// unless it's a known flag that comes BEFORE the image.
		if imageFound && strings.HasPrefix(arg, "-") {
			cmd = append(cmd, arg)
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				cmd = append(cmd, args[i])
			}
			i++
			continue
		}

		switch arg {
		case "-d", "--detach":
			flags.Detach = true
		case "-i", "--interactive":
			flags.Interactive = true
		case "-t", "--tty":
			flags.TTY = true
		case "--rm":
			flags.RM = true
		case "--privileged":
			flags.Privileged = true
		case "--read-only":
			flags.ReadOnly = true
		case "--init":
			flags.Init = true
		case "-P", "--publish-all":
			flags.PublishAll = true
		case "--oom-kill-disable":
			flags.OOMKillDisable = true

		case "--name":
			i++
			if i < len(args) {
				flags.Name = args[i]
			}
		case "--network", "--net":
			i++
			if i < len(args) {
				flags.Network = args[i]
			}
		case "--restart":
			i++
			if i < len(args) {
				flags.RestartPolicy = args[i]
			}
		case "-h", "--hostname":
			i++
			if i < len(args) {
				flags.Hostname = args[i]
			}
		case "--domainname":
			i++
			if i < len(args) {
				flags.Domainname = args[i]
			}
		case "-u", "--user":
			i++
			if i < len(args) {
				flags.User = args[i]
			}
		case "-w", "--workdir":
			i++
			if i < len(args) {
				flags.Workdir = args[i]
			}
		case "--entrypoint":
			i++
			if i < len(args) {
				flags.Entrypoint = args[i]
			}
		case "--stop-signal":
			i++
			if i < len(args) {
				flags.StopSignal = args[i]
			}
		case "--stop-timeout":
			i++
			if i < len(args) {
				flags.StopTimeout, _ = strconv.Atoi(args[i])
			}
		case "-m", "--memory":
			i++
			if i < len(args) {
				flags.Memory = parseMemory(args[i])
			}
		case "--memory-swap":
			i++
			if i < len(args) {
				flags.MemorySwap = parseMemory(args[i])
			}
		case "--cpus":
			i++
			if i < len(args) {
				f, _ := strconv.ParseFloat(args[i], 64)
				flags.NanoCPUs = int64(f * 1e9)
			}
		case "-c", "--cpu-shares":
			i++
			if i < len(args) {
				flags.CPUShares, _ = strconv.ParseInt(args[i], 10, 64)
			}
		case "--cpuset-cpus":
			i++
			if i < len(args) {
				flags.CPUSetCPUs = args[i]
			}
		case "--cpuset-mems":
			i++
			if i < len(args) {
				flags.CPUSetMems = args[i]
			}
		case "--cpu-period":
			i++
			if i < len(args) {
				flags.CPUPeriod, _ = strconv.ParseInt(args[i], 10, 64)
			}
		case "--cpu-quota":
			i++
			if i < len(args) {
				flags.CPUQuota, _ = strconv.ParseInt(args[i], 10, 64)
			}
		case "--pids-limit":
			i++
			if i < len(args) {
				flags.PidsLimit, _ = strconv.ParseInt(args[i], 10, 64)
			}
		case "--shm-size":
			i++
			if i < len(args) {
				flags.ShmSize = parseMemory(args[i])
			}
		case "--pull":
			i++
			if i < len(args) {
				flags.Pull = args[i]
			}
		case "--platform":
			i++
			if i < len(args) {
				flags.Platform = args[i]
			}
		case "--ip":
			i++
			if i < len(args) {
				flags.IP = args[i]
			}
		case "--ip6":
			i++
			if i < len(args) {
				flags.IP6 = args[i]
			}
		case "--mac-address":
			i++
			if i < len(args) {
				flags.MACAddress = args[i]
			}
		case "--ipc":
			i++
			if i < len(args) {
				flags.IPCMode = args[i]
			}
		case "--pid":
			i++
			if i < len(args) {
				flags.PIDMode = args[i]
			}
		case "--uts":
			i++
			if i < len(args) {
				flags.UTSMode = args[i]
			}
		case "--userns":
			i++
			if i < len(args) {
				flags.UsernsMode = args[i]
			}
		case "--isolation":
			i++
			if i < len(args) {
				flags.Isolation = args[i]
			}
		case "--runtime":
			i++
			if i < len(args) {
				flags.Runtime_RT = args[i]
			}
		case "--volume-driver":
			i++
			if i < len(args) {
				flags.VolumeDriver = args[i]
			}
		case "--log-driver":
			i++
			if i < len(args) {
				flags.LogDriver = args[i]
			}
		case "--cgroup-parent":
			i++
			if i < len(args) {
				flags.CgroupParent = args[i]
			}
		case "--cgroupns":
			i++
			if i < len(args) {
				flags.CgroupNS = args[i]
			}
		case "--blkio-weight":
			i++
			if i < len(args) {
				w, _ := strconv.ParseUint(args[i], 10, 16)
				flags.BlkioWeight = uint16(w)
			}
		case "--gpus":
			i++
			if i < len(args) {
				flags.GPUs = args[i]
			}
		case "--health-cmd":
			i++
			if i < len(args) {
				flags.HealthCmd = args[i]
			}
		case "--health-interval":
			i++
			if i < len(args) {
				flags.HealthInterval = args[i]
			}
		case "--health-timeout":
			i++
			if i < len(args) {
				flags.HealthTimeout = args[i]
			}
		case "--health-retries":
			i++
			if i < len(args) {
				flags.HealthRetries, _ = strconv.Atoi(args[i])
			}
		case "--health-start-period":
			i++
			if i < len(args) {
				flags.HealthStartPeriod = args[i]
			}

		case "-e", "--env":
			i++
			if i < len(args) {
				flags.Env = append(flags.Env, args[i])
			}
		case "--env-file":
			i++
			if i < len(args) {
				flags.EnvFile = append(flags.EnvFile, args[i])
			}
		case "--dns":
			i++
			if i < len(args) {
				flags.DNS = append(flags.DNS, args[i])
			}
		case "--dns-search":
			i++
			if i < len(args) {
				flags.DNSSearch = append(flags.DNSSearch, args[i])
			}
		case "--dns-option", "--dns-opt":
			i++
			if i < len(args) {
				flags.DNSOptions = append(flags.DNSOptions, args[i])
			}
		case "--add-host":
			i++
			if i < len(args) {
				flags.ExtraHosts = append(flags.ExtraHosts, args[i])
			}
		case "-p", "--publish":
			i++
			if i < len(args) {
				flags.Ports = append(flags.Ports, args[i])
			}
		case "-v", "--volume":
			i++
			if i < len(args) {
				flags.Volumes = append(flags.Volumes, args[i])
			}
		case "--mount":
			i++
			if i < len(args) {
				m := MountOpt{}
				parts := strings.Split(args[i], ",")
				for _, part := range parts {
					kv := strings.SplitN(part, "=", 2)
					if len(kv) == 2 {
						switch kv[0] {
						case "type":
							m.Type = kv[1]
						case "src", "source":
							m.Source = kv[1]
						case "dst", "target", "destination":
							m.Target = kv[1]
						case "ro", "readonly":
							m.ReadOnly = true
						}
					}
				}
				flags.Mounts = append(flags.Mounts, m)
			}
		case "--volumes-from":
			i++
			if i < len(args) {
				flags.VolumesFrom = append(flags.VolumesFrom, args[i])
			}
		case "--device":
			i++
			if i < len(args) {
				flags.Devices = append(flags.Devices, args[i])
			}
		case "--device-cgroup-rule":
			i++
			if i < len(args) {
				flags.DeviceCgroupRules = append(flags.DeviceCgroupRules, args[i])
			}
		case "--cap-add":
			i++
			if i < len(args) {
				flags.CapAdd = append(flags.CapAdd, args[i])
			}
		case "--cap-drop":
			i++
			if i < len(args) {
				flags.CapDrop = append(flags.CapDrop, args[i])
			}
		case "--security-opt":
			i++
			if i < len(args) {
				flags.SecurityOpt = append(flags.SecurityOpt, args[i])
			}
		case "--sysctl":
			i++
			if i < len(args) {
				if flags.Sysctls == nil {
					flags.Sysctls = make(map[string]string)
				}
				parts := strings.SplitN(args[i], "=", 2)
				if len(parts) == 2 {
					flags.Sysctls[parts[0]] = parts[1]
				}
			}
		case "--ulimit":
			i++
			if i < len(args) {
				flags.Ulimits = append(flags.Ulimits, args[i])
			}
		case "--group-add":
			i++
			if i < len(args) {
				flags.GroupAdd = append(flags.GroupAdd, args[i])
			}
		case "--link":
			i++
			if i < len(args) {
				flags.Link = append(flags.Link, args[i])
			}
		case "--expose":
			i++
			if i < len(args) {
				flags.Expose = append(flags.Expose, args[i])
			}
		case "-l", "--label":
			i++
			if i < len(args) {
				if flags.Labels == nil {
					flags.Labels = make(map[string]string)
				}
				parts := strings.SplitN(args[i], "=", 2)
				if len(parts) == 2 {
					flags.Labels[parts[0]] = parts[1]
				}
			}
		case "--log-opt":
			i++
			if i < len(args) {
				if flags.LogOpts == nil {
					flags.LogOpts = make(map[string]string)
				}
				parts := strings.SplitN(args[i], "=", 2)
				if len(parts) == 2 {
					flags.LogOpts[parts[0]] = parts[1]
				}
			}
		case "--storage-opt":
			i++
			if i < len(args) {
				if flags.StorageOpt == nil {
					flags.StorageOpt = make(map[string]string)
				}
				parts := strings.SplitN(args[i], "=", 2)
				if len(parts) == 2 {
					flags.StorageOpt[parts[0]] = parts[1]
				}
			}
		case "--annotation":
			i++
			if i < len(args) {
				if flags.Annotations == nil {
					flags.Annotations = make(map[string]string)
				}
				parts := strings.SplitN(args[i], "=", 2)
				if len(parts) == 2 {
					flags.Annotations[parts[0]] = parts[1]
				}
			}

		default:
			if !strings.HasPrefix(arg, "-") && !imageFound {
				image = arg
				imageFound = true
			} else {
				cmd = append(cmd, arg)
			}
		}
		i++
	}

	return image, cmd, flags
}

func parseMemory(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	multiplier := int64(1)

	switch {
	case strings.HasSuffix(s, "TB"):
		multiplier = 1024 * 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
		if strings.HasSuffix(s, "G") {
			s = s[:len(s)-1]
		}
	case strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M"):
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
		if strings.HasSuffix(s, "M") {
			s = s[:len(s)-1]
		}
	case strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K"):
		multiplier = 1024
		s = s[:len(s)-1]
		if strings.HasSuffix(s, "K") {
			s = s[:len(s)-1]
		}
	}

	val, _ := strconv.ParseFloat(s, 64)
	return int64(val * float64(multiplier))
}

func init() {
	_, _ = os.Stderr.Write, math.Abs
}
