package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"sync/atomic"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

var (
	reqCount  uint64
	errCount  uint64
	startTime = time.Now()
)

// MetricsHandler serves Prometheus-compatible metrics.
func MetricsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version="+common.Version)

	goroutines := goruntime.NumGoroutine()
	var mem goruntime.MemStats
	goruntime.ReadMemStats(&mem)

	uptime := time.Since(startTime).Seconds()

	_, _ = w.Write([]byte("# HELP doki_uptime_seconds Time since daemon started\n"))
	_, _ = w.Write([]byte("# TYPE doki_uptime_seconds gauge\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("doki_uptime_seconds %.0f\n", uptime)))

	_, _ = w.Write([]byte("# HELP doki_requests_total Total number of HTTP requests\n"))
	_, _ = w.Write([]byte("# TYPE doki_requests_total counter\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("doki_requests_total %d\n", atomic.LoadUint64(&reqCount))))

	_, _ = w.Write([]byte("# HELP doki_errors_total Total number of HTTP errors\n"))
	_, _ = w.Write([]byte("# TYPE doki_errors_total counter\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("doki_errors_total %d\n", atomic.LoadUint64(&errCount))))

	_, _ = w.Write([]byte("# HELP doki_goroutines Number of goroutines\n"))
	_, _ = w.Write([]byte("# TYPE doki_goroutines gauge\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("doki_goroutines %d\n", goroutines)))

	_, _ = w.Write([]byte("# HELP doki_memory_alloc_bytes Allocated memory in bytes\n"))
	_, _ = w.Write([]byte("# TYPE doki_memory_alloc_bytes gauge\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("doki_memory_alloc_bytes %d\n", mem.Alloc)))

	_, _ = w.Write([]byte("# HELP doki_memory_sys_bytes System memory in bytes\n"))
	_, _ = w.Write([]byte("# TYPE doki_memory_sys_bytes gauge\n"))
	_, _ = w.Write([]byte(fmt.Sprintf("doki_memory_sys_bytes %d\n", mem.Sys)))
}

// RecordRequest increments the request counter.
func RecordRequest() { atomic.AddUint64(&reqCount, 1) }

// RecordError increments the error counter.
func RecordError() { atomic.AddUint64(&errCount, 1) }

// AG8: HealthHandler returns comprehensive daemon health status.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	status := "healthy"
	checks := make([]map[string]string, 0)

	// Check storage.
	storageStatus := "ok"
	if dataDir := os.Getenv("DOKI_DATA_DIR"); dataDir != "" {
		if _, err := os.Stat(dataDir); err != nil {
			storageStatus = "unhealthy: " + err.Error()
			status = "unhealthy"
		}
	}
	checks = append(checks, map[string]string{
		"name": "storage", "status": storageStatus,
	})

	// Check proot availability.
	prootStatus := "ok"
	if _, err := exec.LookPath("proot"); err != nil {
		prootStatus = "unavailable"
	}
	checks = append(checks, map[string]string{
		"name": "proot", "status": prootStatus,
	})

	// Check registry connectivity (non-blocking).
	registryStatus := "unknown"

	// Collect overall info.
	response := map[string]interface{}{
		"status":     status,
		"version":    common.Version,
		"uptime":     time.Since(startTime).String(),
		"goroutines": goruntime.NumGoroutine(),
		"checks":     checks,
		"registry":   registryStatus,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Warn("encode", "err", err)
	}
}

// PprofHandler returns a pprof index page for debugging.
func PprofHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<html><body>
<h1>Doki pprof</h1>
<a href="/debug/pprof/goroutine">goroutine</a><br>
<a href="/debug/pprof/heap">heap</a><br>
<a href="/debug/pprof/threadcreate">threadcreate</a><br>
<a href="/debug/pprof/block">block</a><br>
<a href="/debug/pprof/mutex">mutex</a><br>
</body></html>`))
}
