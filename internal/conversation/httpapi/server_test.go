package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/conversation"
	conversationhttp "github.com/Nesoriel/nivora/internal/conversation/httpapi"
	"github.com/Nesoriel/nivora/internal/conversation/sqlstore"
)

func TestTranscriptAPIRequiresInternalKeyAndUsesConfiguredTenant(t *testing.T) {
	store := openStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := store.BeginRun(ctx, conversation.RunRecord{RequestID: "req-1", TenantID: "tenant-a", ConversationID: "conv-1", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessage(ctx, conversation.MessageRecord{MessageID: "msg-1", RequestID: "req-1", TenantID: "tenant-a", ConversationID: "conv-1", Role: "user", Content: "hello", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	api, err := conversationhttp.New(store, "tenant-a", "secret")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	api.Register(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/conversations/conv-1/transcript", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/conversations/conv-1/transcript", nil)
	request.Header.Set("X-Nivora-Key", "secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !containsAll(body, `"tenant_id":"tenant-a"`, `"content":"hello"`) {
		t.Fatalf("unexpected response: %s", body)
	}
}

func openStore(t *testing.T) *sqlstore.Store {
	t.Helper()
	store, err := sqlstore.Open(context.Background(), "sqlite", "file:"+filepath.Join(t.TempDir(), "transcript.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !strings.Contains(value, fragment) {
			return false
		}
	}
	return true
}
