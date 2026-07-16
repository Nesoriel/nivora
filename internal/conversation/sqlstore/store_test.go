package sqlstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/conversation"
)

func TestSQLiteStorePersistsTranscriptAndRejectsCrossIdentityReplay(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	run := conversation.RunRecord{
		RequestID:      "req-1",
		TenantID:       "tenant-a",
		ConversationID: "conv-1",
		NivoraVersion:  "v1",
		NivoraCommit:   "abc",
		PromptVersion:  "p1",
		PromptSource:   "bundled",
		StartedAt:      now,
	}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.BeginRun(ctx, run); err != nil {
		t.Fatalf("same run replay should be idempotent: %v", err)
	}
	conflict := run
	conflict.TenantID = "tenant-b"
	if err := store.BeginRun(ctx, conflict); !errors.Is(err, conversation.ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}

	messages := []conversation.MessageRecord{
		{MessageID: "req-1:user", RequestID: "req-1", TenantID: "tenant-a", ConversationID: "conv-1", Role: "user", Content: "hello", CreatedAt: now},
		{MessageID: "req-1:assistant", RequestID: "req-1", TenantID: "tenant-a", ConversationID: "conv-1", Role: "assistant", Content: "verified answer", CreatedAt: now.Add(time.Second)},
	}
	for _, message := range messages {
		if err := store.AppendMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
		if err := store.AppendMessage(ctx, message); err != nil {
			t.Fatalf("message replay should be idempotent: %v", err)
		}
	}

	transcript, err := store.Transcript(ctx, "tenant-a", "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 2 || transcript[1].Content != "verified answer" {
		t.Fatalf("unexpected transcript: %#v", transcript)
	}
	otherTenant, err := store.Transcript(ctx, "tenant-b", "conv-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(otherTenant) != 0 {
		t.Fatalf("cross-tenant transcript leaked: %#v", otherTenant)
	}
}

func TestSQLiteStoreRecordsSanitizedToolsCasesAndRetention(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := store.BeginRun(ctx, conversation.RunRecord{RequestID: "req-old", TenantID: "tenant-a", ConversationID: "conv-old", StartedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessage(ctx, conversation.MessageRecord{MessageID: "old:user", RequestID: "req-old", TenantID: "tenant-a", ConversationID: "conv-old", Role: "user", Content: "old", CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := store.ToolStarted(ctx, conversation.ToolAuditRecord{RequestID: "req-old", TenantID: "tenant-a", ConversationID: "conv-old", ToolCallID: "tool-1", ToolName: "create_support_case", Status: "started", StartedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := store.ToolFinished(ctx, conversation.ToolAuditRecord{RequestID: "req-old", TenantID: "tenant-a", ConversationID: "conv-old", ToolCallID: "tool-1", ToolName: "create_support_case", Status: "finished", StartedAt: old, FinishedAt: old.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordSupportCase(ctx, conversation.SupportCaseRecord{TenantID: "tenant-a", ConversationID: "conv-old", ProviderCaseID: "case-1", Status: "open", CreatedAt: old, UpdatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(ctx, conversation.RunFinish{RequestID: "req-old", Status: "completed", FinishedAt: old.Add(2 * time.Second)}); err != nil {
		t.Fatal(err)
	}

	result, err := store.DeleteBefore(ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if result.Runs != 1 || result.Messages != 1 || result.ToolAudits != 1 || result.SupportCases != 1 || result.Conversations != 1 {
		t.Fatalf("unexpected retention result: %#v", result)
	}
	transcript, err := store.Transcript(ctx, "tenant-a", "conv-old")
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript) != 0 {
		t.Fatalf("retention left messages: %#v", transcript)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "nivora.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	store, err := Open(context.Background(), "sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
