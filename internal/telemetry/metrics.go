package telemetry

import (
	"fmt"
	"io"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Metrics stores a small production-safe metric set without introducing a
// second metrics runtime into the service.
type Metrics struct {
	activeRuns        atomic.Int64
	totalRuns         atomic.Int64
	successfulRuns    atomic.Int64
	failedRuns        atomic.Int64
	queueRejected     atomic.Int64
	readinessFailures atomic.Int64
	toolMu            sync.Mutex
	toolCalls         map[string]int64
}

func New() *Metrics {
	return &Metrics{toolCalls: make(map[string]int64)}
}

func (m *Metrics) RunStarted() {
	m.totalRuns.Add(1)
	m.activeRuns.Add(1)
}

func (m *Metrics) RunFinished(success bool) {
	m.activeRuns.Add(-1)
	if success {
		m.successfulRuns.Add(1)
	} else {
		m.failedRuns.Add(1)
	}
}

func (m *Metrics) QueueRejected() {
	m.queueRejected.Add(1)
}

func (m *Metrics) ReadinessFailed() {
	m.readinessFailures.Add(1)
}

func (m *Metrics) ToolStarted(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unknown"
	}
	m.toolMu.Lock()
	m.toolCalls[name]++
	m.toolMu.Unlock()
}

// WritePrometheus writes the stable metric surface consumed by Prometheus or a
// compatible managed collector.
func (m *Metrics) WritePrometheus(w io.Writer) error {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	lines := []struct {
		name       string
		help       string
		metricType string
		value      uint64
	}{
		{"nivora_agent_active_runs", "Current active agent runs.", "gauge", uint64(maxInt64(m.activeRuns.Load(), 0))},
		{"nivora_agent_runs_total", "Total accepted agent runs.", "counter", uint64(maxInt64(m.totalRuns.Load(), 0))},
		{"nivora_agent_runs_success_total", "Agent runs that completed successfully.", "counter", uint64(maxInt64(m.successfulRuns.Load(), 0))},
		{"nivora_agent_runs_failed_total", "Agent runs that failed.", "counter", uint64(maxInt64(m.failedRuns.Load(), 0))},
		{"nivora_agent_queue_rejected_total", "Agent runs rejected by overload protection.", "counter", uint64(maxInt64(m.queueRejected.Load(), 0))},
		{"nivora_readiness_failures_total", "Dependency readiness checks that failed.", "counter", uint64(maxInt64(m.readinessFailures.Load(), 0))},
		{"nivora_process_goroutines", "Current Go goroutine count.", "gauge", uint64(runtime.NumGoroutine())},
		{"nivora_process_heap_alloc_bytes", "Current allocated heap bytes.", "gauge", memory.HeapAlloc},
		{"nivora_process_heap_objects", "Current allocated heap object count.", "gauge", memory.HeapObjects},
	}
	for _, metric := range lines {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %d\n", metric.name, metric.help, metric.name, metric.metricType, metric.name, metric.value); err != nil {
			return err
		}
	}

	m.toolMu.Lock()
	toolCalls := make(map[string]int64, len(m.toolCalls))
	for name, value := range m.toolCalls {
		toolCalls[name] = value
	}
	m.toolMu.Unlock()

	names := make([]string, 0, len(toolCalls))
	for name := range toolCalls {
		names = append(names, name)
	}
	sort.Strings(names)
	if _, err := io.WriteString(w, "# HELP nivora_tool_calls_total Tool calls started by the agent.\n# TYPE nivora_tool_calls_total counter\n"); err != nil {
		return err
	}
	for _, name := range names {
		if _, err := fmt.Fprintf(w, "nivora_tool_calls_total{tool=\"%s\"} %d\n", escapeLabel(name), toolCalls[name]); err != nil {
			return err
		}
	}
	return nil
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func escapeLabel(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\n", "\\n")
	return strings.ReplaceAll(value, "\"", "\\\"")
}
