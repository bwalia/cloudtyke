// Package chaos injects faults into ARM traffic: throttling (429 +
// Retry-After), transient 5xx errors, added latency and slow async
// operations. Configured at runtime through /_cloudtyke/chaos.
package chaos

import (
	"encoding/json"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Config is the chaos configuration.
type Config struct {
	ThrottleRate      float64 `json:"throttleRate"`      // 0..1 probability of 429
	ErrorRate         float64 `json:"errorRate"`         // 0..1 probability of 500/503
	LatencyMs         int     `json:"latencyMs"`         // added to every ARM request
	AsyncDelaySeconds float64 `json:"asyncDelaySeconds"` // how long async operations stay InProgress
	WritesOnly        bool    `json:"writesOnly"`        // only affect PUT/PATCH/DELETE/POST
}

// Engine holds the active chaos configuration.
type Engine struct {
	mu   sync.RWMutex
	cfg  Config
	base time.Duration
}

// New returns an engine with the default async delay.
func New(baseAsyncDelay time.Duration) *Engine { return &Engine{base: baseAsyncDelay} }

// Get returns the current configuration.
func (e *Engine) Get() Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// Set replaces the configuration.
func (e *Engine) Set(c Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cfg = c
}

// AsyncDelay is used by the async operation manager.
func (e *Engine) AsyncDelay() time.Duration {
	c := e.Get()
	if c.AsyncDelaySeconds > 0 {
		return time.Duration(c.AsyncDelaySeconds * float64(time.Second))
	}
	return e.base
}

// Middleware wraps the ARM handler.
func (e *Engine) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := e.Get()
		affected := !c.WritesOnly || r.Method != http.MethodGet
		// never break operation polling: clients would otherwise loop forever
		if strings.Contains(strings.ToLower(r.URL.Path), "operation") {
			affected = false
		}
		if affected {
			if c.LatencyMs > 0 {
				time.Sleep(time.Duration(c.LatencyMs) * time.Millisecond)
			}
			if c.ThrottleRate > 0 && rand.Float64() < c.ThrottleRate {
				w.Header().Set("Retry-After", "2")
				write(w, 429, "SubscriptionRequestsThrottled", "Number of requests for subscription exceeded the limit. Please try again after '2' seconds. (CloudTyke chaos)")
				return
			}
			if c.ErrorRate > 0 && rand.Float64() < c.ErrorRate {
				if rand.IntN(2) == 0 {
					write(w, 500, "InternalServerError", "Encountered internal server error. (CloudTyke chaos)")
				} else {
					w.Header().Set("Retry-After", "1")
					write(w, 503, "ServiceUnavailable", "The service is temporarily unavailable. (CloudTyke chaos)")
				}
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func write(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": msg}})
}
