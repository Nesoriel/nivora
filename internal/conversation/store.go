package conversation

import (
	"context"
	"errors"
	"time"
)

var ErrIdempotencyConflict = errors.New("conversation idempotency conflict")

// RunRecord is the durable, support-safe metadata for one Agent run.
type RunRecord struct {
	RequestID      string
	TenantID       string
	ConversationID string
	Authenticated  bool
	ScopeCount     int
	NivoraVersion  string
	NivoraCommit   string
	PromptVersion  string
	PromptSource   string
	StartedAt      time.Time
}

// MessageRecord stores only customer-visible user or assistant content.
type MessageRecord struct {
	MessageID      string    `json:"message_id"`
	RequestID      string    `json:"request_id"`
	TenantID       string    `json:"tenant_id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

// ToolAuditRecord deliberately excludes Tool arguments and raw results.
type ToolAuditRecord struct {
	RequestID      string
	TenantID       string
	ConversationID string
	ToolCallID     string
	ToolName       string
	Status         string
	ReferenceID    string
	StartedAt      time.Time
	FinishedAt     time.Time
}

// SupportCaseRecord stores only the external reference needed for handoff.
type SupportCaseRecord struct {
	TenantID       string
	ConversationID string
	ProviderCaseID string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RunFinish records the externally observable result of one run.
type RunFinish struct {
	RequestID  string
	Status     string
	ErrorCode  string
	FinishedAt time.Time
}

// RetentionResult reports restart-safe cleanup counts.
type RetentionResult struct {
	Runs          int64
	Messages      int64
	ToolAudits    int64
	SupportCases  int64
	Conversations int64
}

// Store owns Nivora conversation and audit state. It never stores chain of
// thought, bearer contexts, service secrets, or unrestricted Provider payloads.
type Store interface {
	BeginRun(context.Context, RunRecord) error
	AppendMessage(context.Context, MessageRecord) error
	ToolStarted(context.Context, ToolAuditRecord) error
	ToolFinished(context.Context, ToolAuditRecord) error
	RecordSupportCase(context.Context, SupportCaseRecord) error
	FinishRun(context.Context, RunFinish) error
	Transcript(context.Context, string, string) ([]MessageRecord, error)
	DeleteBefore(context.Context, time.Time) (RetentionResult, error)
	Check(context.Context) error
	Close() error
}

type nopStore struct{}

// Nop returns a disabled store implementation.
func Nop() Store { return nopStore{} }

func (nopStore) BeginRun(context.Context, RunRecord) error                         { return nil }
func (nopStore) AppendMessage(context.Context, MessageRecord) error                 { return nil }
func (nopStore) ToolStarted(context.Context, ToolAuditRecord) error                  { return nil }
func (nopStore) ToolFinished(context.Context, ToolAuditRecord) error                 { return nil }
func (nopStore) RecordSupportCase(context.Context, SupportCaseRecord) error           { return nil }
func (nopStore) FinishRun(context.Context, RunFinish) error                          { return nil }
func (nopStore) Transcript(context.Context, string, string) ([]MessageRecord, error)  { return nil, nil }
func (nopStore) DeleteBefore(context.Context, time.Time) (RetentionResult, error)     { return RetentionResult{}, nil }
func (nopStore) Check(context.Context) error                                         { return nil }
func (nopStore) Close() error                                                        { return nil }
