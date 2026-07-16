package conversation_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Nesoriel/nivora/internal/conversation"
	"github.com/Nesoriel/nivora/internal/conversation/sqlstore"
	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
	"github.com/Nesoriel/nivora/internal/requestctx"
)

type fakeStreamer struct{}

func (fakeStreamer) Stream(_ context.Context, _ domain.ChatRequest, _ provider.RequestAuth, emit func(domain.StreamEvent) error) error {
	if err := emit(domain.StreamEvent{Type: "tool.started", ToolCallID: "call-1", ToolName: "search_knowledge"}); err != nil {
		return err
	}
	if err := emit(domain.StreamEvent{Type: "tool.finished", ToolCallID: "call-1", ToolName: "search_knowledge"}); err != nil {
		return err
	}
	if err := emit(domain.StreamEvent{Type: "message.delta", Content: "verified "}); err != nil {
		return err
	}
	if err := emit(domain.StreamEvent{Type: "message.delta", Content: "answer"}); err != nil {
		return err
	}
	return emit(domain.StreamEvent{Type: "done"})
}

func TestRecorderPersistsOnlyPublicMessages(t *testing.T) {
	store := openStore(t)
	recorder, err := conversation.NewRecorder(fakeStreamer{}, store, "v1", "abc", func() (string, string) {
		return "prompt-v2", "cozeloop"
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := requestctx.WithRequestID(context.Background(), "req-1")
	request := domain.ChatRequest{
		Question:       "customer question",
		ConversationID: "conv-1",
		Tenant:         domain.TenantContext{ID: "tenant-a"},
		Principal:      domain.Principal{Scopes: []string{domain.ScopeKnowledgeRead}},
	}
	var forwarded []domain.StreamEvent
	if err := recorder.Stream(ctx, request, provider.RequestAuth{}, func(event domain.StreamEvent) error {
		forwarded = append(forwarded, event)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	transcript, err := store.Transcript(context.Background(), "tenant-a", "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 2 || transcript[0].Content != "customer question" || transcript[1].Content != "verified answer" {
		t.Fatalf("unexpected transcript: %#v", transcript)
	}
	if len(forwarded) != 5 {
		t.Fatalf("unexpected forwarded events: %#v", forwarded)
	}
}

func openStore(t *testing.T) *sqlstore.Store {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "recorder.db") + "?_pragma=busy_timeout(5000)"
	store, err := sqlstore.Open(context.Background(), "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
