package builder

import (
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// ProgressEvent represents a build progress event.
type ProgressEvent struct {
	ID      string        `json:"id"`
	Type    string        `json:"type"` // "step", "output", "error", "complete"
	Message string        `json:"message"`
	Stage   string        `json:"stage,omitempty"`
	Step    int           `json:"step,omitempty"`
	Total   int           `json:"total,omitempty"`
	Elapsed time.Duration `json:"elapsed,omitempty"`
}

// ProgressCallback is called for each progress event.
type ProgressCallback func(event ProgressEvent)

// ProgressTracker tracks build progress and reports it.
type ProgressTracker struct {
	mu       sync.Mutex
	log      *slog.Logger
	callback ProgressCallback
	writer   io.Writer
	start    time.Time
	stage    string
	step     int
	total    int
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(log *slog.Logger, callback ProgressCallback) *ProgressTracker {
	return &ProgressTracker{
		log:      log,
		callback: callback,
		start:    time.Now(),
	}
}

// NewProgressTrackerWithWriter creates a progress tracker that writes to a writer.
func NewProgressTrackerWithWriter(w io.Writer) *ProgressTracker {
	return &ProgressTracker{
		log:    slog.Default().With("component", "builder.progress"),
		writer: w,
		start:  time.Now(),
	}
}

// SetStage sets the current build stage.
func (p *ProgressTracker) SetStage(name string, totalSteps int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stage = name
	p.step = 0
	p.total = totalSteps
	p.emit(ProgressEvent{
		Type:    "stage",
		Message: fmt.Sprintf("Building stage: %s", name),
		Stage:   name,
		Total:   totalSteps,
	})
}

// Step advances to the next step.
func (p *ProgressTracker) Step(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.step++
	p.emit(ProgressEvent{
		Type:    "step",
		Message: message,
		Stage:   p.stage,
		Step:    p.step,
		Total:   p.total,
		Elapsed: time.Since(p.start),
	})
}

// Output logs build output.
func (p *ProgressTracker) Output(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emit(ProgressEvent{
		Type:    "output",
		Message: message,
		Stage:   p.stage,
	})
}

// Error logs a build error.
func (p *ProgressTracker) Error(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emit(ProgressEvent{
		Type:    "error",
		Message: message,
		Stage:   p.stage,
		Elapsed: time.Since(p.start),
	})
}

// Complete marks the build as complete.
func (p *ProgressTracker) Complete(message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.emit(ProgressEvent{
		Type:    "complete",
		Message: message,
		Elapsed: time.Since(p.start),
	})
}

func (p *ProgressTracker) emit(event ProgressEvent) {
	if p.callback != nil {
		p.callback(event)
	}
	if p.writer != nil {
		p.writeToWriter(event)
	}
	if p.log != nil {
		switch event.Type {
		case "error":
			p.log.Error("build", "msg", event.Message, "stage", event.Stage)
		case "complete":
			p.log.Info("build complete", "msg", event.Message, "elapsed", event.Elapsed)
		default:
			p.log.Info("build", "type", event.Type, "msg", event.Message, "stage", event.Stage)
		}
	}
}

func (p *ProgressTracker) writeToWriter(event ProgressEvent) {
	var prefix string
	switch event.Type {
	case "step":
		prefix = fmt.Sprintf("[%d/%d] ", event.Step, event.Total)
	case "error":
		prefix = "ERROR: "
	case "complete":
		prefix = "==> "
	}
	fmt.Fprintf(p.writer, "%s%s\n", prefix, event.Message)
}

// BuildProgressWriter is an io.Writer that captures build output and reports progress.
type BuildProgressWriter struct {
	tracker *ProgressTracker
	buf     []byte
}

// NewBuildProgressWriter creates a new build progress writer.
func NewBuildProgressWriter(tracker *ProgressTracker) *BuildProgressWriter {
	return &BuildProgressWriter{tracker: tracker}
}

func (w *BuildProgressWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	// Flush complete lines.
	for {
		i := indexOf(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.tracker.Output(line)
	}
	return len(p), nil
}

func indexOf(data []byte, b byte) int {
	for i, v := range data {
		if v == b {
			return i
		}
	}
	return -1
}

// StepTimer tracks the duration of a build step.
type StepTimer struct {
	start   time.Time
	name    string
	tracker *ProgressTracker
}

// NewStepTimer creates a new step timer.
func NewStepTimer(name string, tracker *ProgressTracker) *StepTimer {
	tracker.Step(name)
	return &StepTimer{
		start:   time.Now(),
		name:    name,
		tracker: tracker,
	}
}

// Stop stops the timer and reports the duration.
func (t *StepTimer) Stop() {
	elapsed := time.Since(t.start)
	t.tracker.Output(fmt.Sprintf("  -> %s completed in %s", t.name, elapsed.Round(time.Millisecond)))
}
