package cozeloop

import (
	"context"
	"log/slog"

	ccb "github.com/cloudwego/eino-ext/callbacks/cozeloop"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/schema"
	cozeloopgo "github.com/coze-dev/cozeloop-go"
	"github.com/coze-dev/cozeloop-go/spec/tracespec"

	"github.com/Nesoriel/nivora/internal/runtrace"
)

// Runtime owns the optional CozeLoop client and Eino callback registration.
type Runtime struct {
	client cozeloopgo.Client
	tracer runtrace.Tracer
	logger *slog.Logger
}

// New initializes CozeLoop only when enabled. Callers should degrade to
// Disabled when this function returns an error; trace export must never become
// a hard dependency for customer conversations.
func New(enabled bool, logger *slog.Logger) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if !enabled {
		return Disabled(logger), nil
	}
	client, err := cozeloopgo.NewClient()
	if err != nil {
		return nil, err
	}
	parser := &safeParser{base: ccb.NewDefaultDataParser(false)}
	callbacks.AppendGlobalHandlers(ccb.NewLoopHandler(
		client,
		ccb.WithCallbackDataParser(parser),
		ccb.WithAggrMessageOutput(false),
	))
	return &Runtime{client: client, tracer: &tracer{client: client}, logger: logger}, nil
}

// Disabled returns a no-op runtime.
func Disabled(logger *slog.Logger) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{tracer: runtrace.Noop(), logger: logger}
}

// Client exposes the official CozeLoop SDK client for PromptHub integration.
func (r *Runtime) Client() cozeloopgo.Client {
	if r == nil {
		return nil
	}
	return r.client
}

// Tracer returns the safe root-run tracer.
func (r *Runtime) Tracer() runtrace.Tracer {
	if r == nil || r.tracer == nil {
		return runtrace.Noop()
	}
	return r.tracer
}

// Close flushes and closes the optional client during graceful shutdown.
func (r *Runtime) Close(ctx context.Context) {
	if r == nil || r.client == nil {
		return
	}
	r.client.Close(ctx)
}

type tracer struct {
	client cozeloopgo.Client
}

func (t *tracer) Start(ctx context.Context, metadata runtrace.Metadata) (context.Context, func(error)) {
	if t == nil || t.client == nil {
		return ctx, func(error) {}
	}
	ctx, span := t.client.StartSpan(ctx, "nivora_support_run", "agent", nil)
	span.SetTags(ctx, map[string]any{
		"request_id":       metadata.RequestID,
		"conversation_id":  metadata.ConversationID,
		"tenant_id":        metadata.TenantID,
		"nivora_version":   metadata.Version,
		"nivora_commit":    metadata.Commit,
		"prompt_version":   metadata.PromptVersion,
		"prompt_source":    metadata.PromptSource,
		"authenticated":    metadata.Authenticated,
		"authorized_scopes": metadata.ScopeCount,
		"authorized_tools":  metadata.ToolCount,
	})
	return ctx, func(runErr error) {
		if runErr != nil {
			span.SetError(ctx, runErr)
		}
		span.Finish(ctx)
	}
}

type safeParser struct {
	base ccb.CallbackDataParser
}

func (p *safeParser) ParseInput(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) map[string]any {
	return filterTags(p.base.ParseInput(ctx, info, input))
}

func (p *safeParser) ParseOutput(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) map[string]any {
	return filterTags(p.base.ParseOutput(ctx, info, output))
}

func (p *safeParser) ParseStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) map[string]any {
	return filterTags(p.base.ParseStreamInput(ctx, info, input))
}

func (p *safeParser) ParseStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) map[string]any {
	return filterTags(p.base.ParseStreamOutput(ctx, info, output))
}

var allowedTraceTags = map[string]struct{}{
	tracespec.ModelName:         {},
	tracespec.ModelProvider:     {},
	tracespec.InputTokens:       {},
	tracespec.OutputTokens:      {},
	tracespec.Tokens:            {},
	tracespec.LatencyFirstResp:  {},
	tracespec.Stream:            {},
	tracespec.PromptKey:         {},
	tracespec.PromptVersion:     {},
	tracespec.PromptProvider:    {},
	tracespec.ToolCallID:        {},
}

func filterTags(tags map[string]any) map[string]any {
	if len(tags) == 0 {
		return nil
	}
	filtered := make(map[string]any, len(tags))
	for key, value := range tags {
		if _, allowed := allowedTraceTags[key]; allowed {
			filtered[key] = value
		}
	}
	return filtered
}
