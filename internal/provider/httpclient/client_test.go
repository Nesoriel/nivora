package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
