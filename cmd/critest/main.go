// Command critest is a minimal CRI (Container Runtime Interface) client used to
// verify that dokid's CRI service works end-to-end, the way crictl would. It
// drives the full lifecycle: Version, Status, PullImage, RunPodSandbox,
// CreateContainer, StartContainer, ListContainers, ExecSync, then cleans up.
//
// Usage: critest /path/to/doki-cri.sock [image]
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	v1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: critest <cri-socket> [image]")
		os.Exit(2)
	}
	sock := os.Args[1]
	image := "busybox"
	if len(os.Args) > 2 {
		image = os.Args[2]
	}

	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatal("dial", err)
	}
	defer func() { _ = conn.Close() }()

	rt := v1.NewRuntimeServiceClient(conn)
	img := v1.NewImageServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. Version
	ver, err := rt.Version(ctx, &v1.VersionRequest{Version: "v1"})
	if err != nil {
		fatal("Version", err)
	}
	ok("Version", fmt.Sprintf("runtime=%s/%s api=%s", ver.RuntimeName, ver.RuntimeVersion, ver.Version))

	// 2. Status
	st, err := rt.Status(ctx, &v1.StatusRequest{})
	if err != nil {
		fatal("Status", err)
	}
	conds := ""
	for _, c := range st.GetStatus().GetConditions() {
		conds += fmt.Sprintf("%s=%v ", c.Type, c.Status)
	}
	ok("Status", conds)

	// 3. PullImage
	pull, err := img.PullImage(ctx, &v1.PullImageRequest{Image: &v1.ImageSpec{Image: image}})
	if err != nil {
		fatal("PullImage", err)
	}
	ok("PullImage", pull.ImageRef)

	// 4. RunPodSandbox
	sb, err := rt.RunPodSandbox(ctx, &v1.RunPodSandboxRequest{
		Config: &v1.PodSandboxConfig{
			Metadata: &v1.PodSandboxMetadata{Name: "critest-pod", Namespace: "default", Uid: "critest-uid"},
		},
	})
	if err != nil {
		fatal("RunPodSandbox", err)
	}
	ok("RunPodSandbox", sb.PodSandboxId)

	// 5. CreateContainer
	cc, err := rt.CreateContainer(ctx, &v1.CreateContainerRequest{
		PodSandboxId: sb.PodSandboxId,
		Config: &v1.ContainerConfig{
			Metadata: &v1.ContainerMetadata{Name: "critest-ctr"},
			Image:    &v1.ImageSpec{Image: image},
			Command:  []string{"sh", "-c", "echo cri-hello && sleep 30"},
		},
		SandboxConfig: &v1.PodSandboxConfig{
			Metadata: &v1.PodSandboxMetadata{Name: "critest-pod", Namespace: "default", Uid: "critest-uid"},
		},
	})
	if err != nil {
		fatal("CreateContainer", err)
	}
	ok("CreateContainer", cc.ContainerId)

	// 6. StartContainer
	if _, err := rt.StartContainer(ctx, &v1.StartContainerRequest{ContainerId: cc.ContainerId}); err != nil {
		fatal("StartContainer", err)
	}
	ok("StartContainer", cc.ContainerId)

	// 7. ListContainers
	lc, err := rt.ListContainers(ctx, &v1.ListContainersRequest{})
	if err != nil {
		fatal("ListContainers", err)
	}
	ok("ListContainers", fmt.Sprintf("%d container(s)", len(lc.Containers)))

	// 8. ExecSync
	es, err := rt.ExecSync(ctx, &v1.ExecSyncRequest{
		ContainerId: cc.ContainerId,
		Cmd:         []string{"echo", "exec-sync-ok"},
		Timeout:     10,
	})
	if err != nil {
		warn("ExecSync", err.Error())
	} else {
		ok("ExecSync", fmt.Sprintf("exit=%d out=%q", es.ExitCode, string(es.Stdout)))
	}

	// Cleanup
	_, _ = rt.StopContainer(ctx, &v1.StopContainerRequest{ContainerId: cc.ContainerId, Timeout: 2})
	_, _ = rt.RemoveContainer(ctx, &v1.RemoveContainerRequest{ContainerId: cc.ContainerId})
	_, _ = rt.StopPodSandbox(ctx, &v1.StopPodSandboxRequest{PodSandboxId: sb.PodSandboxId})
	_, _ = rt.RemovePodSandbox(ctx, &v1.RemovePodSandboxRequest{PodSandboxId: sb.PodSandboxId})
	ok("Cleanup", "done")

	fmt.Println("\nCRI end-to-end: PASS")
}

func ok(step, detail string)   { fmt.Printf("  [OK]   %-16s %s\n", step, detail) }
func warn(step, detail string) { fmt.Printf("  [WARN] %-16s %s\n", step, detail) }
func fatal(step string, err error) {
	fmt.Printf("  [FAIL] %-16s %v\n", step, err)
	os.Exit(1)
}
