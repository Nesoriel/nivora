package runtrace

import "context"

// Metadata contains support-safe identifiers and release metadata for one run.
// It must never contain bearer tokens, service secrets, raw Provider payloads,
// unrestricted message content, or model chain of thought.
type Metadata struct {
	RequestID      string
	ConversationID string
	TenantID       string
	Version        string
	Commit         string
	PromptVersion  string
	PromptSource   string
	Authenticated  bool
	ScopeCount     int
	ToolCount      int
}

// Tracer starts one root trace around an Agent run.
type Tracer interface {
	Start(context.Context, Metadata) (context.Context, func(error))
}

type noopTracer struct{}

// Noop returns a tracer that performs no external work.
func Noop() Tracer {
	return noopTracer{}
}

func (noopTracer) Start(ctx context.Context, _ Metadata) (context.Context, func(error)) {
	return ctx, func(error) {}
}
