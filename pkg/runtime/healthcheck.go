package runtime

import (
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

const (
	healthStarting  = "starting"
	healthHealthy   = "healthy"
	healthUnhealthy = "unhealthy"
	healthNone      = "none"

	defaultHCInterval = 30 * time.Second
	defaultHCTimeout  = 30 * time.Second
	defaultHCRetries  = 3
	maxHCLogEntries   = 5
)

// HealthChecker periodically executes a health check probe for a container
// and updates the container's HealthStatus in its persisted state.
type HealthChecker struct {
	containerID string
	config      *HealthCheckConfig
	runtime     *Runtime

	stopCh chan struct{}
	done   chan struct{}

	mu      sync.Mutex
	stopped bool
}

// NewHealthChecker creates a HealthChecker for the given container.
func NewHealthChecker(rt *Runtime, id string, cfg *HealthCheckConfig) *HealthChecker {
	return &HealthChecker{
		containerID: id,
		config:      cfg,
		runtime:     rt,
		stopCh:      make(chan struct{}),
		done:        make(chan struct{}),
	}
}

// Start launches the health check goroutine.
func (hc *HealthChecker) Start() {
	go hc.run()
}

// Stop signals the health check goroutine to exit and blocks until it does.
// Safe to call multiple times.
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	if hc.stopped {
		hc.mu.Unlock()
		return
	}
	hc.stopped = true
	close(hc.stopCh)
	hc.mu.Unlock()
	<-hc.done
}

// run is the main health check loop.
func (hc *HealthChecker) run() {
	defer close(hc.done)

	cfg := hc.config

	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultHCInterval
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultHCTimeout
	}
	retries := cfg.Retries
	if retries <= 0 {
		retries = defaultHCRetries
	}
	startPeriod := cfg.StartPeriod
	startInterval := cfg.StartInterval
	if startInterval <= 0 {
		startInterval = interval
	}

	// "NONE" → container is always healthy.
	if len(cfg.Test) > 0 && cfg.Test[0] == "NONE" {
		hc.setHealthStatus(healthHealthy, 0)
		return
	}

	// Parse the test command.
	cmd, ok := parseHealthCheckTest(cfg.Test)
	if !ok {
		slog.Default().Warn("healthcheck: invalid test", "container", hc.containerID, "test", cfg.Test)
		return
	}

	startTime := time.Now()

	// Initialise status to "starting".
	hc.setHealthStatus(healthStarting, 0)

	for {
		// Determine the current probe interval: use startInterval during
		// startPeriod, then the regular interval afterwards.
		currentInterval := interval
		inStartPeriod := time.Since(startTime) < startPeriod
		if inStartPeriod {
			currentInterval = startInterval
		}

		select {
		case <-hc.stopCh:
			return
		case <-time.After(currentInterval):
		}

		// Bail out if the container is no longer running.
		state, err := hc.runtime.State(hc.containerID)
		if err != nil || state.Status != common.StateRunning {
			return
		}

		// Execute the probe.
		exitCode, output := hc.runProbe(cmd, timeout)

		// Record the result and update status.
		hc.recordProbe(exitCode, output, inStartPeriod, retries)
	}
}

// runProbe executes a single health check command via runtime.Exec and
// returns the exit code and combined output. A timeout is enforced.
func (hc *HealthChecker) runProbe(cmd []string, timeout time.Duration) (int, string) {
	type execResult struct {
		stdout, stderr []byte
		err            error
	}

	resultCh := make(chan execResult, 1)
	go func() {
		stdout, stderr, err := hc.runtime.Exec(hc.containerID, cmd, nil, "", "")
		resultCh <- execResult{stdout, stderr, err}
	}()

	select {
	case res := <-resultCh:
		exitCode := 0
		output := strings.TrimSpace(string(res.stdout) + string(res.stderr))
		if res.err != nil {
			if exitErr, ok := res.err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
			if output != "" {
				output += "\n"
			}
			output += res.err.Error()
		}
		return exitCode, output

	case <-time.After(timeout):
		return -1, "healthcheck probe timed out"

	case <-hc.stopCh:
		return -1, "healthcheck stopped"
	}
}

// recordProbe records the probe result in the health log and updates the
// health status according to Docker semantics:
//   - A single success → "healthy" (and resets the failing streak).
//   - During startPeriod, failures don't count toward unhealthy.
//   - After startPeriod, `retries` consecutive failures → "unhealthy".
func (hc *HealthChecker) recordProbe(exitCode int, output string, inStartPeriod bool, retries int) {
	hc.runtime.mu.Lock()
	defer hc.runtime.mu.Unlock()

	state, err := hc.runtime.loadState(hc.containerID)
	if err != nil || state.HealthStatus == nil {
		return
	}

	hs := state.HealthStatus
	now := time.Now()
	probeDuration := hc.config.Timeout
	if probeDuration <= 0 {
		probeDuration = defaultHCTimeout
	}

	hs.Log = append(hs.Log, common.HealthCheckResult{
		Start:    now.Add(-probeDuration),
		End:      now,
		ExitCode: exitCode,
		Output:   output,
	})
	if len(hs.Log) > maxHCLogEntries {
		hs.Log = hs.Log[len(hs.Log)-maxHCLogEntries:]
	}

	success := exitCode == 0
	if success {
		hs.FailingStreak = 0
		hs.Status = healthHealthy
	} else {
		if inStartPeriod {
			// Failures during start period don't count toward unhealthy.
			hs.FailingStreak = 0
			if hs.Status != healthHealthy {
				hs.Status = healthStarting
			}
		} else {
			hs.FailingStreak++
			if hs.FailingStreak >= retries {
				hs.Status = healthUnhealthy
			} else if hs.Status != healthHealthy {
				hs.Status = healthStarting
			}
		}
	}

	if err := hc.runtime.saveState(state); err != nil {
		slog.Default().Warn("healthcheck: saveState failed", "container", hc.containerID, "error", err)
	}
}

// setHealthStatus updates the container's HealthStatus without recording a log entry.
func (hc *HealthChecker) setHealthStatus(status string, failingStreak int) {
	hc.runtime.mu.Lock()
	defer hc.runtime.mu.Unlock()

	state, err := hc.runtime.loadState(hc.containerID)
	if err != nil {
		return
	}
	if state.HealthStatus == nil {
		state.HealthStatus = &common.HealthStatus{
			Log: []common.HealthCheckResult{},
		}
	}
	state.HealthStatus.Status = status
	state.HealthStatus.FailingStreak = failingStreak

	if err := hc.runtime.saveState(state); err != nil {
		slog.Default().Warn("healthcheck: saveState failed", "container", hc.containerID, "error", err)
	}
}

// parseHealthCheckTest parses the Docker HEALTHCHECK Test field.
//   - ["CMD", ...]        → run the remaining args directly.
//   - ["CMD-SHELL", str]  → run str via /bin/sh -c.
//   - ["NONE"]            → no probe (handled by caller).
//   - No prefix           → treated as a direct command (backward compat).
func parseHealthCheckTest(test []string) ([]string, bool) {
	if len(test) == 0 {
		return nil, false
	}
	switch test[0] {
	case "CMD":
		if len(test) < 2 {
			return nil, false
		}
		return test[1:], true
	case "CMD-SHELL":
		if len(test) < 2 {
			return nil, false
		}
		return []string{"/bin/sh", "-c", test[1]}, true
	case "NONE":
		return nil, false
	default:
		return test, true
	}
}
