package telemetry

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePrometheusIncludesProcessAndAgentMetrics(t *testing.T) {
	metrics := New()
	metrics.RunStarted()
	metrics.ToolStarted("search_knowledge")
	metrics.RunFinished(true)

	var output bytes.Buffer
	if err := metrics.WritePrometheus(&output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, metric := range []string{
		"nivora_agent_runs_total 1",
		"nivora_agent_runs_success_total 1",
		"nivora_process_goroutines",
		"nivora_process_heap_alloc_bytes",
		"nivora_process_heap_objects",
		`nivora_tool_calls_total{tool="search_knowledge"} 1`,
	} {
		if !strings.Contains(text, metric) {
			t.Fatalf("missing metric %q in:\n%s", metric, text)
		}
	}
}
