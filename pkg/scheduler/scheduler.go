// Package scheduler provides the Kubernetes scheduler.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/k8s-types"
	"github.com/OpceanAI/Doki/pkg/store"
)

type Scheduler struct {
	store  store.Store
	queue  *SchedulingQueue
	logger *slog.Logger
	mu     sync.Mutex
}

type SchedulingQueue struct {
	mu   sync.Mutex
	pods []*k8s.Pod
}

func NewScheduler(s store.Store, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store:  s,
		queue:  &SchedulingQueue{pods: make([]*k8s.Pod, 0)},
		logger: logger,
	}
}

func (s *Scheduler) Run(ctx context.Context) error {
	go s.watchUnscheduled(ctx)
	go s.scheduleLoop(ctx)
	s.logger.Info("scheduler started")
	<-ctx.Done()
	return nil
}

func (s *Scheduler) watchUnscheduled(ctx context.Context) {
	prefix := store.KeyFor("", "pods", "", "")
	ch, err := s.store.Watch(prefix, 0)
	if err != nil {
		s.logger.Error("failed to watch pods", "error", err)
		return
	}
	defer s.store.Unwatch(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if event.Type != store.EventAdded && event.Type != store.EventModified {
				continue
			}
			var pod k8s.Pod
			if err := json.Unmarshal(event.Object.Value, &pod); err != nil {
				continue
			}
			if pod.Spec.NodeName == "" && pod.DeletionTimestamp == nil {
				s.queue.Add(&pod)
			}
		}
	}
}

func (s *Scheduler) scheduleLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pod := s.queue.Pop()
		if pod == nil {
			// No pods to schedule — block briefly instead of
			// busy-waiting at 100% CPU. Use a short sleep that
			// is interrupted by context cancellation.
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		if err := s.scheduleOne(pod); err != nil {
			s.logger.Error("schedule failed", "pod", pod.Name, "error", err)
			// Re-queue with backoff.
			time.AfterFunc(5*time.Second, func() {
				s.queue.Add(pod)
			})
		}
	}
}

func (s *Scheduler) scheduleOne(pod *k8s.Pod) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	nodes := s.getNodes()
	if len(nodes) == 0 {
		return fmt.Errorf("no nodes available")
	}

	committed := s.committedRequests()
	feasible := s.filter(pod, nodes, committed)
	if len(feasible) == 0 {
		return fmt.Errorf("no feasible nodes for pod %s/%s", pod.Namespace, pod.Name)
	}

	best := s.score(pod, feasible, committed)

	pod.Spec.NodeName = best.Name
	data, err := json.Marshal(pod)
	if err != nil {
		return fmt.Errorf("marshal pod: %w", err)
	}
	key := store.KeyFor("", "pods", pod.Namespace, pod.Name)
	return s.store.Put(key, &store.StoredObject{Value: data})
}

func (s *Scheduler) getNodes() []k8s.Node {
	objects, err := s.store.List(store.KeyFor("", "nodes", "", ""))
	if err != nil {
		return nil
	}

	nodes := make([]k8s.Node, 0, len(objects))
	for _, obj := range objects {
		var node k8s.Node
		if err := json.Unmarshal(obj.Value, &node); err != nil {
			continue
		}
		for _, cond := range node.Status.Conditions {
			if cond.Type == "Ready" && cond.Status == "True" && !node.Spec.Unschedulable {
				nodes = append(nodes, node)
				break
			}
		}
	}
	return nodes
}

func (s *Scheduler) filter(pod *k8s.Pod, nodes []k8s.Node, committed map[string]resourceRequest) []k8s.Node {
	want := podRequests(pod)
	feasible := make([]k8s.Node, 0)
	for _, node := range nodes {
		if s.filterNodeSelector(pod, node) &&
			s.filterTolerations(pod, node) &&
			s.filterResourceFit(want, node, committed[node.Name]) {
			feasible = append(feasible, node)
		}
	}
	return feasible
}

// resourceRequest is a normalized compute request: CPU in millicores, memory
// in bytes. Normalizing at parse time is what stops "100m" and "1" from being
// compared as if they were the same unit.
type resourceRequest struct {
	cpuMilli int64
	memBytes int64
}

func (r resourceRequest) add(o resourceRequest) resourceRequest {
	return resourceRequest{cpuMilli: r.cpuMilli + o.cpuMilli, memBytes: r.memBytes + o.memBytes}
}

func (r resourceRequest) max(o resourceRequest) resourceRequest {
	out := r
	if o.cpuMilli > out.cpuMilli {
		out.cpuMilli = o.cpuMilli
	}
	if o.memBytes > out.memBytes {
		out.memBytes = o.memBytes
	}
	return out
}

// requestsOf reads cpu/memory out of a ResourceList, preferring requests and
// falling back to limits (Kubernetes defaults requests to limits when only
// limits are given).
func requestsOf(req, lim k8s.ResourceList) resourceRequest {
	var out resourceRequest
	pick := func(name string) string {
		if v, ok := req[name]; ok && v != "" {
			return v
		}
		return lim[name]
	}
	if v := pick("cpu"); v != "" {
		if n, err := parseCPUMilli(v); err == nil {
			out.cpuMilli = n
		}
	}
	if v := pick("memory"); v != "" {
		if n, err := parseMemoryBytes(v); err == nil {
			out.memBytes = n
		}
	}
	return out
}

// podRequests computes the effective resource request of a pod: the sum of its
// regular containers, floored by the largest single init container (init
// containers run sequentially, so they never stack with each other).
func podRequests(pod *k8s.Pod) resourceRequest {
	var sum resourceRequest
	for _, c := range pod.Spec.Containers {
		sum = sum.add(requestsOf(c.Resources.Requests, c.Resources.Limits))
	}
	var initMax resourceRequest
	for _, c := range pod.Spec.InitContainers {
		initMax = initMax.max(requestsOf(c.Resources.Requests, c.Resources.Limits))
	}
	return sum.max(initMax)
}

// committedRequests sums the requests of every pod already bound to a node and
// not yet terminal, keyed by node name. Without this the scheduler would
// compare each new pod against the node's full capacity and happily overcommit
// it a hundred times over.
func (s *Scheduler) committedRequests() map[string]resourceRequest {
	out := make(map[string]resourceRequest)
	objects, err := s.store.List(store.KeyFor("", "pods", "", ""))
	if err != nil {
		return out
	}
	for _, obj := range objects {
		var p k8s.Pod
		if err := json.Unmarshal(obj.Value, &p); err != nil {
			continue
		}
		if p.Spec.NodeName == "" {
			continue
		}
		if p.Status.Phase == "Succeeded" || p.Status.Phase == "Failed" {
			continue
		}
		out[p.Spec.NodeName] = out[p.Spec.NodeName].add(podRequests(&p))
	}
	return out
}

// filterResourceFit is the hard predicate K16 asks for: a pod that requests
// more than a node has left is infeasible, not merely low-scoring. Nodes that
// do not publish allocatable resources are treated as unconstrained, since
// rejecting them would break single-node setups that never fill the field.
func (s *Scheduler) filterResourceFit(want resourceRequest, node k8s.Node, used resourceRequest) bool {
	if want.cpuMilli == 0 && want.memBytes == 0 {
		return true
	}
	if v, ok := node.Status.Allocatable["cpu"]; ok && v != "" && want.cpuMilli > 0 {
		if capacity, err := parseCPUMilli(v); err == nil {
			if used.cpuMilli+want.cpuMilli > capacity {
				return false
			}
		}
	}
	if v, ok := node.Status.Allocatable["memory"]; ok && v != "" && want.memBytes > 0 {
		if capacity, err := parseMemoryBytes(v); err == nil {
			if used.memBytes+want.memBytes > capacity {
				return false
			}
		}
	}
	return true
}

func (s *Scheduler) filterNodeSelector(pod *k8s.Pod, node k8s.Node) bool {
	if pod.Spec.NodeName != "" {
		return node.Name == pod.Spec.NodeName
	}
	for key, value := range pod.Spec.NodeSelector {
		if nodeVal, ok := node.Labels[key]; !ok || nodeVal != value {
			return false
		}
	}
	return true
}

func (s *Scheduler) filterTolerations(pod *k8s.Pod, node k8s.Node) bool {
	for _, taint := range node.Spec.Taints {
		tolerated := false
		for _, toleration := range pod.Spec.Tolerations {
			if toleration.Key == taint.Key {
				if toleration.Operator == "Exists" || toleration.Value == taint.Value {
					if toleration.Effect == "" || toleration.Effect == taint.Effect {
						tolerated = true
						break
					}
				}
			}
		}
		if !tolerated && taint.Effect == "NoSchedule" {
			return false
		}
	}
	return true
}

func (s *Scheduler) score(pod *k8s.Pod, nodes []k8s.Node, committed map[string]resourceRequest) k8s.Node {
	type scored struct {
		node  k8s.Node
		score int64
	}

	scores := make([]scored, 0, len(nodes))
	for _, node := range nodes {
		score := int64(0)
		score += s.scoreImageLocality(pod, node)
		score += s.scoreLeastRequested(pod, node, committed[node.Name])
		scores = append(scores, scored{node: node, score: score})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	return scores[0].node
}

// scoreImageLocality scores nodes based on whether they already have
// the pod's container images pulled locally. A node that already has
// the image gets a higher score (faster startup).
func (s *Scheduler) scoreImageLocality(pod *k8s.Pod, node k8s.Node) int64 {
	if len(node.Status.Images) == 0 {
		return 0
	}
	imageSet := make(map[string]bool)
	for _, img := range node.Status.Images {
		for _, name := range img.Names {
			imageSet[name] = true
		}
	}
	var score int64
	for _, c := range pod.Spec.Containers {
		if imageSet[c.Image] {
			score += 10
		}
	}
	return score
}

// scoreLeastRequested prefers the node with the most headroom left after the
// pods already bound to it, rather than the one with the largest raw capacity.
func (s *Scheduler) scoreLeastRequested(pod *k8s.Pod, node k8s.Node, used resourceRequest) int64 {
	var cpuFree, memFreeMiB int64
	if v, ok := node.Status.Allocatable["cpu"]; ok {
		if n, err := parseCPUMilli(v); err == nil {
			cpuFree = n - used.cpuMilli
		}
	}
	if v, ok := node.Status.Allocatable["memory"]; ok {
		if n, err := parseMemoryBytes(v); err == nil {
			memFreeMiB = (n - used.memBytes) / (1024 * 1024)
		}
	}
	if cpuFree < 0 {
		cpuFree = 0
	}
	if memFreeMiB < 0 {
		memFreeMiB = 0
	}
	return cpuFree/10 + memFreeMiB/100
}

// parseCPUMilli parses a Kubernetes CPU quantity into millicores. "1" is 1000,
// "500m" is 500, "0.5" is 500. The previous parser stripped the "m" suffix and
// applied a multiplier of 1, so "100m" read as 100 CPUs — a 1000x overcount
// that made every CPU comparison meaningless.
func parseCPUMilli(q string) (int64, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	if strings.HasSuffix(q, "m") {
		n, err := strconv.ParseInt(strings.TrimSuffix(q, "m"), 10, 64)
		if err != nil {
			return 0, err
		}
		return n, nil
	}
	cores, err := strconv.ParseFloat(q, 64)
	if err != nil {
		return 0, err
	}
	return int64(cores * 1000), nil
}

// parseMemoryBytes parses a Kubernetes memory quantity into bytes, accepting
// both binary (Ki/Mi/Gi/Ti/Pi) and decimal (k/M/G/T/P) suffixes as the
// Kubernetes quantity format defines them.
func parseMemoryBytes(q string) (int64, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	suffixes := []struct {
		suffix     string
		multiplier int64
	}{
		{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50},
		{"k", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12}, {"P", 1e15},
	}
	for _, s := range suffixes {
		if strings.HasSuffix(q, s.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(q, s.suffix), 64)
			if err != nil {
				return 0, err
			}
			return int64(n * float64(s.multiplier)), nil
		}
	}
	n, err := strconv.ParseFloat(q, 64)
	if err != nil {
		return 0, err
	}
	return int64(n), nil
}

func (q *SchedulingQueue) Add(pod *k8s.Pod) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.pods = append(q.pods, pod)
}

func (q *SchedulingQueue) Pop() *k8s.Pod {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pods) == 0 {
		return nil
	}
	pod := q.pods[0]
	q.pods = q.pods[1:]
	return pod
}

func (q *SchedulingQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pods)
}
