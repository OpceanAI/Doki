package api

import (
	"fmt"
	"net/http"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

var (
	reqCount     uint64
	errCount     uint64
	startTime    = time.Now()
)

// MetricsHandler serves Prometheus-compatible metrics.
func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	goroutines := runtime.NumGoroutine()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := time.Since(startTime).Seconds()

	w.Write([]byte("# HELP doki_uptime_seconds Time since daemon started\n"))
	w.Write([]byte("# TYPE doki_uptime_seconds gauge\n"))
	w.Write([]byte(fmt.Sprintf("doki_uptime_seconds %.0f\n", uptime)))

	w.Write([]byte("# HELP doki_requests_total Total number of HTTP requests\n"))
	w.Write([]byte("# TYPE doki_requests_total counter\n"))
	w.Write([]byte(fmt.Sprintf("doki_requests_total %d\n", atomic.LoadUint64(&reqCount))))

	w.Write([]byte("# HELP doki_errors_total Total number of HTTP errors\n"))
	w.Write([]byte("# TYPE doki_errors_total counter\n"))
	w.Write([]byte(fmt.Sprintf("doki_errors_total %d\n", atomic.LoadUint64(&errCount))))

	w.Write([]byte("# HELP doki_goroutines Number of goroutines\n"))
	w.Write([]byte("# TYPE doki_goroutines gauge\n"))
	w.Write([]byte(fmt.Sprintf("doki_goroutines %d\n", goroutines)))

	w.Write([]byte("# HELP doki_memory_alloc_bytes Allocated memory in bytes\n"))
	w.Write([]byte("# TYPE doki_memory_alloc_bytes gauge\n"))
	w.Write([]byte(fmt.Sprintf("doki_memory_alloc_bytes %d\n", mem.Alloc)))

	w.Write([]byte("# HELP doki_memory_sys_bytes System memory in bytes\n"))
	w.Write([]byte("# TYPE doki_memory_sys_bytes gauge\n"))
	w.Write([]byte(fmt.Sprintf("doki_memory_sys_bytes %d\n", mem.Sys)))
}

// RecordRequest increments the request counter.
func RecordRequest() { atomic.AddUint64(&reqCount, 1) }

// RecordError increments the error counter.
func RecordError() { atomic.AddUint64(&errCount, 1) }

// HealthHandler returns daemon health status.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy","version":"` + common.Version + `","uptime":"` +
		time.Since(startTime).String() + `"}`))
}

// PprofHandler returns a pprof index page for debugging.
func PprofHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<html><body>
<h1>Doki pprof</h1>
<a href="/debug/pprof/goroutine">goroutine</a><br>
<a href="/debug/pprof/heap">heap</a><br>
<a href="/debug/pprof/threadcreate">threadcreate</a><br>
<a href="/debug/pprof/block">block</a><br>
<a href="/debug/pprof/mutex">mutex</a><br>
</body></html>`))
}
