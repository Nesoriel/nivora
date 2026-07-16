package cozeloop

import (
	"testing"

	"github.com/coze-dev/cozeloop-go/spec/tracespec"
)

func TestFilterTagsRemovesMessageAndSecretPayloads(t *testing.T) {
	filtered := filterTags(map[string]any{
		tracespec.Input:        "customer secret",
		tracespec.Output:       "private provider payload",
		tracespec.ModelName:    "ep-safe",
		tracespec.InputTokens:  12,
		"authorization":       "Bearer should-never-leave",
		"provider_raw_result": map[string]any{"recipe": "hidden"},
	})
	if _, exists := filtered[tracespec.Input]; exists {
		t.Fatal("input content must be removed")
	}
	if _, exists := filtered[tracespec.Output]; exists {
		t.Fatal("output content must be removed")
	}
	if _, exists := filtered["authorization"]; exists {
		t.Fatal("authorization must be removed")
	}
	if filtered[tracespec.ModelName] != "ep-safe" || filtered[tracespec.InputTokens] != 12 {
		t.Fatalf("expected safe telemetry to remain: %#v", filtered)
	}
}
