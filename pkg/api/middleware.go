package api

import (
	"context"
	"encoding/hex"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
)

// Middleware provides HTTP middleware for the API server.
type Middleware struct {
	logger *slog.Logger
}

// NewMiddleware creates a new middleware chain.
func NewMiddleware() *Middleware {
	if apiLogger == nil {
		apiLogger = slog.Default().With("component", "api")
	}
	return &Middleware{logger: apiLogger}
}

// Logging logs each request.
func (m *Middleware) Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := w.Header().Get("X-Request-ID")
		m.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"request_id", rid,
		)
		next.ServeHTTP(w, r)
		m.logger.Info("response",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// Recovery catches panics and returns 500.
func (m *Middleware) Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				m.logger.Error("panic recovered",
					"err", err,
					"path", r.URL.Path,
				)
				http.Error(w, `{"message":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// CORS adds CORS headers.
func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,HEAD,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Registry-Auth")
		w.Header().Set("Api-Version", common.DokiAPIVersion)
		w.Header().Set("Server", "Doki/"+common.Version)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequestID adds a unique request ID header.
func (m *Middleware) RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = commonGenID(16)
		}
		w.Header().Set("X-Request-ID", rid)
		next.ServeHTTP(w, r)
	})
}

func commonGenID(n int) string {
	b := make([]byte, n/2)
	for i := range b {
		b[i] = byte(time.Now().UnixNano()>>(i*8)) ^ byte(i*31)
	}
	return hex.EncodeToString(b)
}

// RateLimit implements simple token bucket rate limiting.
type RateLimit struct {
	burst      int
	ratePerSec float64
	tokens     chan struct{}
	stop       chan struct{}
	stopOnce   sync.Once
}

// NewRateLimit creates a rate limiter.
func NewRateLimit(requestsPerSec float64, burst int) *RateLimit {
	if requestsPerSec < 1 {
		requestsPerSec = 1
	}
	rl := &RateLimit{
		burst:      burst,
		ratePerSec: requestsPerSec,
		tokens:     make(chan struct{}, burst),
		stop:       make(chan struct{}),
	}
	for i := 0; i < burst; i++ {
		rl.tokens <- struct{}{}
	}
	go rl.refill()
	return rl
}

func (rl *RateLimit) refill() {
	interval := time.Duration(float64(time.Second) / rl.ratePerSec)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			select {
			case rl.tokens <- struct{}{}:
			default:
			}
		case <-rl.stop:
			return
		}
	}
}

func (rl *RateLimit) Stop() { rl.stopOnce.Do(func() { close(rl.stop) }) }

// Allow checks if a request is allowed.
func (rl *RateLimit) Allow() bool {
	select {
	case <-rl.tokens:
		return true
	default:
		return false
	}
}

// RateLimitMiddleware applies rate limiting.
func (rl *RateLimit) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow() {
			http.Error(w, `{"message":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GracefulShutdown handles OS signals for graceful shutdown.
func GracefulShutdown(ctx context.Context, server *http.Server, timeout time.Duration) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sig)

	for {
		select {
		case s := <-sig:
			switch s {
			case syscall.SIGHUP:
				log.Println("[doki] received SIGHUP - reloading configuration")
				// Reload config.
			default:
				log.Println("[doki] shutting down gracefully...")
				shutdownCtx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				if err := server.Shutdown(shutdownCtx); err != nil {
					slog.Warn("shutdown error", "err", err)
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// SetLogger replaces the API package's logger. Subsequent log lines
// emitted by Middleware and handlers go through slog.
func SetLogger(l *slog.Logger) {
	apiLogger = l
}

var apiLogger *slog.Logger

func slogFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(slogKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	if apiLogger != nil {
		return apiLogger
	}
	return slog.Default()
}

type slogKey struct{}

// WithLogger attaches a per-request logger.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, slogKey{}, l)
}

// WaitForSignal blocks until SIGINT, SIGTERM, or SIGHUP is received,
// then calls the cancel function. Used by main to bridge os/signal to
// a context cancel.
func WaitForSignal(cancel context.CancelFunc) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	s := <-ch
	slogFromContext(context.Background()).Info("signal received", "signal", s.String())
	cancel()
}
