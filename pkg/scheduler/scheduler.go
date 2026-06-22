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

	feasible := s.filter(pod, nodes)
	if len(feasible) == 0 {
		return fmt.Errorf("no feasible nodes for pod %s/%s", pod.Namespace, pod.Name)
	}

	best := s.score(pod, feasible)

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

func (s *Scheduler) filter(pod *k8s.Pod, nodes []k8s.Node) []k8s.Node {
	feasible := make([]k8s.Node, 0)
	for _, node := range nodes {
		if s.filterNodeSelector(pod, node) &&
			s.filterTolerations(pod, node) {
			feasible = append(feasible, node)
		}
	}
	return feasible
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

func (s *Scheduler) score(pod *k8s.Pod, nodes []k8s.Node) k8s.Node {
	type scored struct {
		node  k8s.Node
		score int64
	}

	scores := make([]scored, 0, len(nodes))
	for _, node := range nodes {
		score := int64(0)
		score += s.scoreImageLocality(pod, node)
		score += s.scoreLeastRequested(pod, node)
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

// scoreLeastRequested scores nodes based on resource availability.
// Nodes with more available CPU and memory get higher scores.
func (s *Scheduler) scoreLeastRequested(pod *k8s.Pod, node k8s.Node) int64 {
	cpuScore := int64(0)
	memScore := int64(0)
	for _, addr := range node.Status.Addresses {
		_ = addr
	}
	// Parse capacity and allocatable from node status.
	for resourceName, qty := range node.Status.Allocatable {
		switch resourceName {
		case "cpu":
			n, _ := parseQuantity(qty)
			cpuScore = n
		case "memory":
			n, _ := parseQuantity(qty)
			memScore = n / (1024 * 1024)
		}
	}
	// Prefer nodes with more available resources.
	return cpuScore*100 + memScore/100
}

// parseQuantity parses a Kubernetes resource quantity string (e.g.,
// "4", "8Gi", "512Mi") into an int64 value.
func parseQuantity(q string) (int64, error) {
	if q == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	// Strip suffixes.
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(q, "Ki"):
		multiplier = 1024
		q = q[:len(q)-2]
	case strings.HasSuffix(q, "Mi"):
		multiplier = 1024 * 1024
		q = q[:len(q)-2]
	case strings.HasSuffix(q, "Gi"):
		multiplier = 1024 * 1024 * 1024
		q = q[:len(q)-2]
	case strings.HasSuffix(q, "Ti"):
		multiplier = 1024 * 1024 * 1024 * 1024
		q = q[:len(q)-2]
	case strings.HasSuffix(q, "m"):
		multiplier = 1
		q = q[:len(q)-1]
	}
	n, err := strconv.ParseInt(q, 10, 64)
	if err != nil {
		return 0, err
	}
	return n * multiplier, nil
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
