package eval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
)

func TestLoadJSONLRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()
	dataset := strings.NewReader("" +
		`{"id":"case-1","question":"hello","tenant":{"id":"test"},"principal":{"scopes":["knowledge:read"]}}` + "\n" +
		`{"id":"case-1","question":"again","tenant":{"id":"test"},"principal":{"scopes":["knowledge:read"]}}` + "\n")
	if _, err := LoadJSONL(dataset); err == nil {
		t.Fatal("expected duplicate id error")
	}
}

func TestEvaluateChecksToolsContentAndLatency(t *testing.T) {
	t.Parallel()
	result := Evaluate(Case{
		ID: "case-1",
		Expected: Expectations{
			RequiredSubstrings:  []string{"refund confirmed"},
			ForbiddenSubstrings: []string{"system prompt"},
			RequiredTools:       []string{"list_transactions"},
			ForbiddenTools:      []string{"create_support_case"},
			MaxLatencyMS:        1000,
		},
	}, Observation{
		Answer:    "Refund confirmed.",
		Tools:     []string{"list_transactions"},
		Completed: true,
		Duration:  500 * time.Millisecond,
	})
	if !result.Passed {
		t.Fatalf("unexpected failure: %#v", result.Failures)
	}
}

func TestClientParsesNivoraSSE(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Nivora-Key") != "secret" {
			t.Fatal("missing shared secret")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: tool.started\ndata: {\"type\":\"tool.started\",\"tool_name\":\"search_knowledge\"}\n\n")
		_, _ = fmt.Fprint(w, "event: message.delta\ndata: {\"type\":\"message.delta\",\"content\":\"hello\"}\n\n")
		_, _ = fmt.Fprint(w, "event: done\ndata: {\"type\":\"done\"}\n\n")
	}))
	defer server.Close()

	client := Client{BaseURL: server.URL, SharedSecret: "secret", HTTPClient: server.Client()}
	observation, err := client.Run(context.Background(), Case{
		ID:       "case-1",
		Question: "hello",
		Tenant:   domain.TenantContext{ID: "test"},
		Principal: domain.Principal{
			Scopes: []string{domain.ScopeKnowledgeRead},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Completed || observation.Answer != "hello" {
		t.Fatalf("unexpected observation: %#v", observation)
	}
	if len(observation.Tools) != 1 || observation.Tools[0] != "search_knowledge" {
		t.Fatalf("unexpected tools: %#v", observation.Tools)
	}
}
