package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

func TestSearchKnowledgeForwardsAuthAndDecodes(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/internal/support/knowledge" {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer signed-context" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if got := request.Header.Get("X-Nivora-Provider-Key"); got != "provider-secret" {
			t.Fatalf("unexpected provider key: %q", got)
		}
		if got := request.URL.Query().Get("limit"); got != "6" {
			t.Fatalf("unexpected limit: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"id": "kb-1", "title": "Refunds", "content": "Check the ledger."}}})
	}))
	defer server.Close()

	client, err := New(server.URL, "provider-secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.SearchKnowledge(context.Background(), provider.RequestAuth{BearerToken: "signed-context"}, "refund", 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "kb-1" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestIdempotentProviderRequestRetriesTransientFailures(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := attempts.Add(1)
		if attempt < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	}))
	defer server.Close()

	client, err := New(server.URL, "provider-secret", server.Client(), WithRetry(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SearchKnowledge(context.Background(), provider.RequestAuth{}, "refund", 6); err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestCreateCaseIsNotRetriedAndSendsIdempotencyKey(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		attempts.Add(1)
		if got := request.Header.Get("Idempotency-Key"); got != "case-key" {
			t.Fatalf("unexpected idempotency key: %q", got)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := New(server.URL, "provider-secret", server.Client(), WithRetry(3, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateCase(context.Background(), provider.RequestAuth{}, domain.CreateCaseInput{
		Subject:        "Need help",
		Summary:        "A factual summary",
		IdempotencyKey: "case-key",
	})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected one POST attempt, got %d", got)
	}
}
