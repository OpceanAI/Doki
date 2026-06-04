package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Global metrics collector instance.
var metrics = NewMetricsCollector()

// MetricsCollector collects and exposes Prometheus-compatible metrics.
type MetricsCollector struct {
	// Counters
	requestsTotal    atomic.Uint64
	requestsErrors   atomic.Uint64
	containersCreated atomic.Uint64
	containersStarted atomic.Uint64
	containersStopped atomic.Uint64
	containersKilled  atomic.Uint64
	imagesPulled      atomic.Uint64
	imagesBuilt       atomic.Uint64
	networksCreated   atomic.Uint64
	volumesCreated    atomic.Uint64
	execCalls         atomic.Uint64

	// Gauges
	containersRunning atomic.Int64
	containersPaused  atomic.Int64
	imagesCount       atomic.Int64
	goroutines        atomic.Int64

	// Histograms (simplified as counters)
	requestDurationSum   atomic.Uint64
	requestDurationCount atomic.Uint64

	// Info
	startTime time.Time
	mu        sync.RWMutex
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime: time.Now(),
	}
}

// RecordRequest records an HTTP request.
func (m *MetricsCollector) RecordRequest(duration time.Duration) {
	m.requestsTotal.Add(1)
	m.requestDurationSum.Add(uint64(duration.Milliseconds()))
	m.requestDurationCount.Add(1)
}

// RecordError records an HTTP error.
func (m *MetricsCollector) RecordError() {
	m.requestsErrors.Add(1)
}

// RecordContainerCreated records a container creation.
func (m *MetricsCollector) RecordContainerCreated() {
	m.containersCreated.Add(1)
}

// RecordContainerStarted records a container start.
func (m *MetricsCollector) RecordContainerStarted() {
	m.containersStarted.Add(1)
	m.containersRunning.Add(1)
}

// RecordContainerStopped records a container stop.
func (m *MetricsCollector) RecordContainerStopped() {
	m.containersStopped.Add(1)
	m.containersRunning.Add(-1)
}

// RecordContainerKilled records a container kill.
func (m *MetricsCollector) RecordContainerKilled() {
	m.containersKilled.Add(1)
	m.containersRunning.Add(-1)
}

// RecordImagePulled records an image pull.
func (m *MetricsCollector) RecordImagePulled() {
	m.imagesPulled.Add(1)
	m.imagesCount.Add(1)
}

// RecordImageBuilt records an image build.
func (m *MetricsCollector) RecordImageBuilt() {
	m.imagesBuilt.Add(1)
	m.imagesCount.Add(1)
}

// RecordNetworkCreated records a network creation.
func (m *MetricsCollector) RecordNetworkCreated() {
	m.networksCreated.Add(1)
}

// RecordVolumeCreated records a volume creation.
func (m *MetricsCollector) RecordVolumeCreated() {
	m.volumesCreated.Add(1)
}

// RecordExec records an exec call.
func (m *MetricsCollector) RecordExec() {
	m.execCalls.Add(1)
}

// SetContainersRunning sets the number of running containers.
func (m *MetricsCollector) SetContainersRunning(n int64) {
	m.containersRunning.Store(n)
}

// SetContainersPaused sets the number of paused containers.
func (m *MetricsCollector) SetContainersPaused(n int64) {
	m.containersPaused.Store(n)
}

// SetImagesCount sets the number of images.
func (m *MetricsCollector) SetImagesCount(n int64) {
	m.imagesCount.Store(n)
}

// MetricsSnapshot is a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	RequestsTotal     uint64 `json:"requests_total"`
	RequestsErrors    uint64 `json:"requests_errors"`
	ContainersCreated uint64 `json:"containers_created"`
	ContainersStarted uint64 `json:"containers_started"`
	ContainersStopped uint64 `json:"containers_stopped"`
	ContainersKilled  uint64 `json:"containers_killed"`
	ContainersRunning int64  `json:"containers_running"`
	ContainersPaused  int64  `json:"containers_paused"`
	ImagesPulled      uint64 `json:"images_pulled"`
	ImagesBuilt       uint64 `json:"images_built"`
	ImagesCount       int64  `json:"images_count"`
	NetworksCreated   uint64 `json:"networks_created"`
	VolumesCreated    uint64 `json:"volumes_created"`
	ExecCalls         uint64 `json:"exec_calls"`
	Goroutines        int    `json:"goroutines"`
	UptimeSeconds     float64 `json:"uptime_seconds"`
	AllocBytes        uint64 `json:"alloc_bytes"`
	SysBytes          uint64 `json:"sys_bytes"`
	NumGC             uint32 `json:"num_gc"`
	AvgRequestMs      float64 `json:"avg_request_ms"`
}

// Snapshot returns a point-in-time snapshot of all metrics.
func (m *MetricsCollector) Snapshot() MetricsSnapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	avgReqMs := float64(0)
	if count := m.requestDurationCount.Load(); count > 0 {
		avgReqMs = float64(m.requestDurationSum.Load()) / float64(count)
	}

	return MetricsSnapshot{
		RequestsTotal:     m.requestsTotal.Load(),
		RequestsErrors:    m.requestsErrors.Load(),
		ContainersCreated: m.containersCreated.Load(),
		ContainersStarted: m.containersStarted.Load(),
		ContainersStopped: m.containersStopped.Load(),
		ContainersKilled:  m.containersKilled.Load(),
		ContainersRunning: m.containersRunning.Load(),
		ContainersPaused:  m.containersPaused.Load(),
		ImagesPulled:      m.imagesPulled.Load(),
		ImagesBuilt:       m.imagesBuilt.Load(),
		ImagesCount:       m.imagesCount.Load(),
		NetworksCreated:   m.networksCreated.Load(),
		VolumesCreated:    m.volumesCreated.Load(),
		ExecCalls:         m.execCalls.Load(),
		Goroutines:        runtime.NumGoroutine(),
		UptimeSeconds:     time.Since(m.startTime).Seconds(),
		AllocBytes:        mem.Alloc,
		SysBytes:          mem.Sys,
		NumGC:             mem.NumGC,
		AvgRequestMs:      avgReqMs,
	}
}

// MetricsHandler serves Prometheus-compatible metrics.
func MetricsHandlerV2(w http.ResponseWriter, r *http.Request) {
	snap := metrics.Snapshot()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)

	writeCounter(w, "doki_http_requests_total", snap.RequestsTotal, "Total HTTP requests")
	writeCounter(w, "doki_http_requests_errors_total", snap.RequestsErrors, "Total HTTP errors")
	writeCounter(w, "doki_containers_created_total", snap.ContainersCreated, "Total containers created")
	writeCounter(w, "doki_containers_started_total", snap.ContainersStarted, "Total containers started")
	writeCounter(w, "doki_containers_stopped_total", snap.ContainersStopped, "Total containers stopped")
	writeCounter(w, "doki_containers_killed_total", snap.ContainersKilled, "Total containers killed")
	writeGauge(w, "doki_containers_running", snap.ContainersRunning, "Currently running containers")
	writeGauge(w, "doki_containers_paused", snap.ContainersPaused, "Currently paused containers")
	writeCounter(w, "doki_images_pulled_total", snap.ImagesPulled, "Total images pulled")
	writeCounter(w, "doki_images_built_total", snap.ImagesBuilt, "Total images built")
	writeGauge(w, "doki_images_count", snap.ImagesCount, "Total images")
	writeCounter(w, "doki_networks_created_total", snap.NetworksCreated, "Total networks created")
	writeCounter(w, "doki_volumes_created_total", snap.VolumesCreated, "Total volumes created")
	writeCounter(w, "doki_exec_calls_total", snap.ExecCalls, "Total exec calls")
	writeGauge(w, "doki_goroutines", int64(snap.Goroutines), "Number of goroutines")
	writeGauge(w, "doki_uptime_seconds", int64(snap.UptimeSeconds), "Daemon uptime in seconds")
	writeGauge(w, "doki_memory_alloc_bytes", int64(snap.AllocBytes), "Allocated memory bytes")
	writeGauge(w, "doki_memory_sys_bytes", int64(snap.SysBytes), "System memory bytes")
	writeCounter(w, "doki_gc_cycles_total", uint64(snap.NumGC), "Total GC cycles")
	writeGauge(w, "doki_avg_request_ms", int64(snap.AvgRequestMs), "Average request duration in ms")
}

// HealthHandlerV2 returns comprehensive health status.
func HealthHandlerV2(w http.ResponseWriter, r *http.Request) {
	snap := metrics.Snapshot()

	status := "healthy"
	if snap.ContainersRunning < 0 {
		status = "unhealthy"
	}

	response := map[string]interface{}{
		"status":    status,
		"version":   common.Version,
		"api":       common.DokiAPIVersion,
		"uptime":    snap.UptimeSeconds,
		"goroutines": snap.Goroutines,
		"memory": map[string]interface{}{
			"alloc_bytes": snap.AllocBytes,
			"sys_bytes":   snap.SysBytes,
		},
		"containers": map[string]interface{}{
			"running": snap.ContainersRunning,
			"paused":  snap.ContainersPaused,
			"total":   snap.ContainersCreated,
		},
		"images": snap.ImagesCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func writeCounter(w http.ResponseWriter, name string, value uint64, help string) {
	w.Write([]byte("# HELP " + name + " " + help + "\n"))
	w.Write([]byte("# TYPE " + name + " counter\n"))
	w.Write([]byte(name + " " + formatUint64(value) + "\n"))
}

func writeGauge(w http.ResponseWriter, name string, value int64, help string) {
	w.Write([]byte("# HELP " + name + " " + help + "\n"))
	w.Write([]byte("# TYPE " + name + " gauge\n"))
	w.Write([]byte(name + " " + formatInt64(value) + "\n"))
}

func formatUint64(v uint64) string {
	return fmt.Sprintf("%d", v)
}

func formatInt64(v int64) string {
	return fmt.Sprintf("%d", v)
}
