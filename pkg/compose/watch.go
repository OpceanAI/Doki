package compose

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type WatchAction string

const (
	WatchSync         WatchAction = "sync"
	WatchRebuild      WatchAction = "rebuild"
	WatchSyncRestart  WatchAction = "sync+restart"
)

type WatchRule struct {
	Action WatchAction `yaml:"action" json:"action"`
	Path   string      `yaml:"path" json:"path"`
	Target string      `yaml:"target" json:"target"`
	Ignore []string    `yaml:"ignore,omitempty" json:"ignore,omitempty"`
}

type DevelopConfig struct {
	Watch []WatchRule `yaml:"watch,omitempty" json:"watch,omitempty"`
}

type WatchManager struct {
	engine     *Engine
	watchers   map[string]*ServiceWatcher
	logger     *slog.Logger
	mu         sync.RWMutex
}

type ServiceWatcher struct {
	service  string
	rules    []WatchRule
	watcher  *fsnotify.Watcher
	manager  *WatchManager
	debounce time.Duration
	lastEvent map[string]time.Time
	mu       sync.Mutex
}

func NewWatchManager(engine *Engine, logger *slog.Logger) *WatchManager {
	return &WatchManager{
		engine:   engine,
		watchers: make(map[string]*ServiceWatcher),
		logger:   logger,
	}
}

func (wm *WatchManager) Start(service string, rules []WatchRule) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.watchers[service]; exists {
		return fmt.Errorf("watch already active for service %s", service)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}

	sw := &ServiceWatcher{
		service:   service,
		rules:     rules,
		watcher:   fsw,
		manager:   wm,
		debounce:  500 * time.Millisecond,
		lastEvent: make(map[string]time.Time),
	}

	for _, rule := range rules {
		absPath, err := filepath.Abs(rule.Path)
		if err != nil {
			_ = fsw.Close()
			return fmt.Errorf("resolve path %s: %w", rule.Path, err)
		}
		if err := fsw.Add(absPath); err != nil {
			wm.logger.Warn("cannot watch path", "path", absPath, "error", err)
		}
	}

	wm.watchers[service] = sw
	go sw.run()

	wm.logger.Info("watch started", "service", service, "rules", len(rules))
	return nil
}

func (wm *WatchManager) Stop(service string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	sw, ok := wm.watchers[service]
	if !ok {
		return fmt.Errorf("watch not active for service %s", service)
	}

	_ = sw.watcher.Close()
	delete(wm.watchers, service)
	wm.logger.Info("watch stopped", "service", service)
	return nil
}

func (wm *WatchManager) StopAll() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for service, sw := range wm.watchers {
		_ = sw.watcher.Close()
		delete(wm.watchers, service)
	}
}

func (wm *WatchManager) Active() []string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	services := make([]string, 0, len(wm.watchers))
	for s := range wm.watchers {
		services = append(services, s)
	}
	return services
}

func (sw *ServiceWatcher) run() {
	for {
		select {
		case event, ok := <-sw.watcher.Events:
			if !ok {
				return
			}
			sw.handleEvent(event)

		case err, ok := <-sw.watcher.Errors:
			if !ok {
				return
			}
			sw.manager.logger.Error("watcher error", "service", sw.service, "error", err)
		}
	}
}

func (sw *ServiceWatcher) handleEvent(event fsnotify.Event) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	if last, ok := sw.lastEvent[event.Name]; ok && time.Since(last) < sw.debounce {
		return
	}
	sw.lastEvent[event.Name] = time.Now()

	if sw.shouldIgnore(event.Name) {
		return
	}

	for _, rule := range sw.rules {
		absRule, _ := filepath.Abs(rule.Path)
		if !strings.HasPrefix(event.Name, absRule) {
			continue
		}

		switch rule.Action {
		case WatchSync:
			sw.syncFile(event.Name, rule.Target)
		case WatchRebuild:
			sw.rebuildService()
		case WatchSyncRestart:
			sw.syncFile(event.Name, rule.Target)
			sw.restartService()
		}
	}
}

func (sw *ServiceWatcher) shouldIgnore(path string) bool {
	for _, pattern := range sw.rules[0].Ignore {
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
		if strings.Contains(path, pattern) {
			return true
		}
	}
	return false
}

func (sw *ServiceWatcher) syncFile(hostPath, targetPath string) {
	sw.manager.logger.Info("syncing file", "host", hostPath, "target", targetPath, "service", sw.service)
}

func (sw *ServiceWatcher) rebuildService() {
	sw.manager.logger.Info("rebuilding service", "service", sw.service)
}

func (sw *ServiceWatcher) restartService() {
	sw.manager.logger.Info("restarting service", "service", sw.service)
}

func init() {
	_ = os.Getenv
}
