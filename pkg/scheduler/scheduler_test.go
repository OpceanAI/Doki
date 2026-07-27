package scheduler

import (
	"testing"

	k8s "github.com/OpceanAI/Doki/pkg/k8s-types"
)

func TestParseCPUMilli(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1", 1000},
		{"2", 2000},
		{"100m", 100},
		{"500m", 500},
		{"0.5", 500},
		{"0.25", 250},
		{"4", 4000},
	}
	for _, c := range cases {
		got, err := parseCPUMilli(c.in)
		if err != nil {
			t.Fatalf("parseCPUMilli(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseCPUMilli(%q) = %d, want %d", c.in, got, c.want)
		}
	}
	if _, err := parseCPUMilli(""); err == nil {
		t.Error("parseCPUMilli(\"\") should fail")
	}
}

func TestParseMemoryBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"1Ki", 1024},
		{"1Mi", 1024 * 1024},
		{"512Mi", 512 * 1024 * 1024},
		{"2Gi", 2 * 1024 * 1024 * 1024},
		{"1k", 1000},
		{"1M", 1000000},
		{"1G", 1000000000},
	}
	for _, c := range cases {
		got, err := parseMemoryBytes(c.in)
		if err != nil {
			t.Fatalf("parseMemoryBytes(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseMemoryBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func podWith(cpu, mem string) *k8s.Pod {
	return &k8s.Pod{
		Spec: k8s.PodSpec{
			Containers: []k8s.Container{{
				Name:  "app",
				Image: "busybox",
				Resources: k8s.ResourceRequirements{
					Requests: k8s.ResourceList{"cpu": cpu, "memory": mem},
				},
			}},
		},
	}
}

func nodeWith(cpu, mem string) k8s.Node {
	n := k8s.Node{}
	n.Name = "node-a"
	n.Status.Allocatable = k8s.ResourceList{"cpu": cpu, "memory": mem}
	return n
}

// A pod asking for more than the node has must be rejected outright, not
// merely ranked low. This is the K16 regression: before the hard predicate,
// filter() only looked at nodeSelector and tolerations, so an oversized pod
// was scheduled anyway and then failed at runtime.
func TestFilterResourceFitRejectsOversizedPod(t *testing.T) {
	s := &Scheduler{}
	node := nodeWith("2", "4Gi")

	if !s.filterResourceFit(podRequests(podWith("1", "1Gi")), node, resourceRequest{}) {
		t.Error("pod that fits was rejected")
	}
	if s.filterResourceFit(podRequests(podWith("4", "1Gi")), node, resourceRequest{}) {
		t.Error("pod requesting 4 CPUs fit on a 2-CPU node")
	}
	if s.filterResourceFit(podRequests(podWith("1", "8Gi")), node, resourceRequest{}) {
		t.Error("pod requesting 8Gi fit on a 4Gi node")
	}
}

// Already-committed requests must be subtracted; otherwise every pod is
// compared against full capacity and the node is overcommitted N times over.
func TestFilterResourceFitAccountsForCommitted(t *testing.T) {
	s := &Scheduler{}
	node := nodeWith("2", "4Gi")
	used := resourceRequest{cpuMilli: 1500, memBytes: 3 * 1024 * 1024 * 1024}

	if s.filterResourceFit(podRequests(podWith("1", "1Gi")), node, used) {
		t.Error("pod scheduled onto a node with only 500m CPU left")
	}
	if !s.filterResourceFit(podRequests(podWith("400m", "512Mi")), node, used) {
		t.Error("pod that fits in the remaining headroom was rejected")
	}
}

// A node that publishes no allocatable resources is treated as unconstrained,
// so single-node setups that never fill the field keep working.
func TestFilterResourceFitUnconstrainedNode(t *testing.T) {
	s := &Scheduler{}
	if !s.filterResourceFit(podRequests(podWith("64", "128Gi")), k8s.Node{}, resourceRequest{}) {
		t.Error("node without allocatable data should not reject pods")
	}
}

// Init containers run sequentially, so the effective request is the sum of the
// regular containers floored by the largest single init container.
func TestPodRequestsInitContainers(t *testing.T) {
	pod := podWith("100m", "64Mi")
	pod.Spec.InitContainers = []k8s.Container{{
		Name: "init",
		Resources: k8s.ResourceRequirements{
			Requests: k8s.ResourceList{"cpu": "2", "memory": "1Gi"},
		},
	}}
	got := podRequests(pod)
	if got.cpuMilli != 2000 {
		t.Errorf("cpuMilli = %d, want 2000 (init container floor)", got.cpuMilli)
	}
	if got.memBytes != 1024*1024*1024 {
		t.Errorf("memBytes = %d, want 1Gi", got.memBytes)
	}
}

// Kubernetes defaults requests to limits when only limits are specified.
func TestRequestsFallBackToLimits(t *testing.T) {
	got := requestsOf(nil, k8s.ResourceList{"cpu": "250m", "memory": "128Mi"})
	if got.cpuMilli != 250 {
		t.Errorf("cpuMilli = %d, want 250", got.cpuMilli)
	}
	if got.memBytes != 128*1024*1024 {
		t.Errorf("memBytes = %d, want 128Mi", got.memBytes)
	}
}
