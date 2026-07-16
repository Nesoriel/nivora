package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
	"github.com/Nesoriel/nivora/internal/requestctx"
)

// Streamer is the Agent runtime surface decorated by Recorder.
type Streamer interface {
	Stream(context.Context, domain.ChatRequest, provider.RequestAuth, func(domain.StreamEvent) error) error
}

// PromptMetadata returns the active approved Prompt metadata.
type PromptMetadata func() (version, source string)

// Recorder persists public messages and sanitized operational audit records.
type Recorder struct {
	next       Streamer
	store      Store
	version    string
	commit     string
	promptMeta PromptMetadata
	now        func() time.Time
}

// NewRecorder creates a durable Streamer decorator.
func NewRecorder(next Streamer, store Store, version, commit string, promptMeta PromptMetadata) (*Recorder, error) {
	if next == nil {
		return nil, errors.New("next streamer is required")
	}
	if store == nil {
		return nil, errors.New("conversation store is required")
	}
	if promptMeta == nil {
		promptMeta = func() (string, string) { return "unknown", "unknown" }
	}
	return &Recorder{
		next:       next,
		store:      store,
		version:    strings.TrimSpace(version),
		commit:     strings.TrimSpace(commit),
		promptMeta: promptMeta,
		now:        time.Now,
	}, nil
}

// Stream records the run before exposing output. Persistence failures fail the
// run closed so production traffic is never served without its required audit.
func (r *Recorder) Stream(ctx context.Context, request domain.ChatRequest, auth provider.RequestAuth, emit func(domain.StreamEvent) error) (runErr error) {
	requestID := strings.TrimSpace(requestctx.RequestID(ctx))
	if requestID == "" {
		requestID = request.ConversationID + ":run"
	}
	startedAt := r.now().UTC()
	promptVersion, promptSource := r.promptMeta()
	if err := r.store.BeginRun(ctx, RunRecord{
		RequestID:      requestID,
		TenantID:       request.Tenant.ID,
		ConversationID: request.ConversationID,
		Authenticated:  request.Principal.Authenticated,
		ScopeCount:     len(request.Principal.Scopes),
		NivoraVersion:  r.version,
		NivoraCommit:   r.commit,
		PromptVersion:  promptVersion,
		PromptSource:   promptSource,
		StartedAt:      startedAt,
	}); err != nil {
		return err
	}
	if err := r.store.AppendMessage(ctx, MessageRecord{
		MessageID:      requestID + ":user",
		RequestID:      requestID,
		TenantID:       request.Tenant.ID,
		ConversationID: request.ConversationID,
		Role:           "user",
		Content:        request.Question,
		CreatedAt:      startedAt,
	}); err != nil {
		_ = r.store.FinishRun(ctx, RunFinish{RequestID: requestID, Status: "failed", ErrorCode: "storage_write_failed", FinishedAt: r.now().UTC()})
		return err
	}

	var assistant strings.Builder
	seenToolStart := make(map[string]time.Time)
	wrappedEmit := func(event domain.StreamEvent) error {
		now := r.now().UTC()
		switch event.Type {
		case "message.delta":
			assistant.WriteString(event.Content)
		case "tool.started":
			seenToolStart[event.ToolCallID] = now
			if err := r.store.ToolStarted(ctx, ToolAuditRecord{
				RequestID:      requestID,
				TenantID:       request.Tenant.ID,
				ConversationID: request.ConversationID,
				ToolCallID:     event.ToolCallID,
				ToolName:       event.ToolName,
				Status:         "started",
				StartedAt:      now,
			}); err != nil {
				return err
			}
		case "tool.finished":
			started := seenToolStart[event.ToolCallID]
			if started.IsZero() {
				started = now
			}
			if err := r.store.ToolFinished(ctx, ToolAuditRecord{
				RequestID:      requestID,
				TenantID:       request.Tenant.ID,
				ConversationID: request.ConversationID,
				ToolCallID:     event.ToolCallID,
				ToolName:       event.ToolName,
				Status:         firstNonEmpty(event.AuditStatus, "finished"),
				ReferenceID:    event.AuditReferenceID,
				StartedAt:      started,
				FinishedAt:     now,
			}); err != nil {
				return err
			}
		}
		return emit(event)
	}

	runErr = r.next.Stream(ctx, request, auth, wrappedEmit)
	finishedAt := r.now().UTC()
	answer := strings.TrimSpace(assistant.String())
	if answer != "" {
		if err := r.store.AppendMessage(ctx, MessageRecord{
			MessageID:      requestID + ":assistant",
			RequestID:      requestID,
			TenantID:       request.Tenant.ID,
			ConversationID: request.ConversationID,
			Role:           "assistant",
			Content:        answer,
			CreatedAt:      finishedAt,
		}); err != nil {
			if runErr == nil {
				runErr = err
			}
		}
	}
	status := "completed"
	errorCode := ""
	if runErr != nil {
		status = "failed"
		errorCode = "agent_run_failed"
	}
	if err := r.store.FinishRun(ctx, RunFinish{
		RequestID:  requestID,
		Status:     status,
		ErrorCode:  errorCode,
		FinishedAt: finishedAt,
	}); err != nil && runErr == nil {
		runErr = err
	}
	return runErr
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
