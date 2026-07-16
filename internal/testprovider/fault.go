package testprovider

import (
	"net/http"
	"sync/atomic"
	"time"
)

// FaultConfig controls deterministic failures in an isolated acceptance environment.
type FaultConfig struct {
	StatusCode int
	Count      int64
	Delay      time.Duration
}

// FaultInjector fails the first configured number of requests, then delegates.
type FaultInjector struct {
	next      http.Handler
	status    int
	remaining atomic.Int64
	delay     time.Duration
}

// WithFaults wraps a synthetic Provider with deterministic latency and status failures.
func WithFaults(next http.Handler, config FaultConfig) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	injector := &FaultInjector{next: next, status: config.StatusCode, delay: config.Delay}
	injector.remaining.Store(config.Count)
	return injector
}

func (f *FaultInjector) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if f.delay > 0 {
		timer := time.NewTimer(f.delay)
		defer timer.Stop()
		select {
		case <-request.Context().Done():
			return
		case <-timer.C:
		}
	}
	for {
		remaining := f.remaining.Load()
		if remaining <= 0 {
			break
		}
		if f.remaining.CompareAndSwap(remaining, remaining-1) {
			status := f.status
			if status < 400 || status > 599 {
				status = http.StatusServiceUnavailable
			}
			writeJSON(w, status, map[string]string{"error": "synthetic_provider_fault"})
			return
		}
	}
	f.next.ServeHTTP(w, request)
}
