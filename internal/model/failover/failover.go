package failover

import (
	"context"
	"errors"
	"fmt"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Model tries configured chat models in order. Generate falls back on request
// failure; Stream falls back only when a stream cannot be established, never
// after partial output has reached the caller.
type Model struct {
	models []einomodel.ToolCallingChatModel
}

// New creates an immutable failover model.
func New(models ...einomodel.ToolCallingChatModel) (*Model, error) {
	filtered := make([]einomodel.ToolCallingChatModel, 0, len(models))
	for _, candidate := range models {
		if candidate != nil {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return nil, errors.New("at least one chat model is required")
	}
	return &Model{models: filtered}, nil
}

// Generate tries each configured endpoint until one succeeds or the context ends.
func (m *Model) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	var failures []error
	for index, candidate := range m.models {
		output, err := candidate.Generate(ctx, input, opts...)
		if err == nil {
			return output, nil
		}
		failures = append(failures, fmt.Errorf("model %d generate: %w", index, err))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(failures...)
}

// Stream tries the next endpoint only when stream creation itself fails.
func (m *Model) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	var failures []error
	for index, candidate := range m.models {
		stream, err := candidate.Stream(ctx, input, opts...)
		if err == nil {
			return stream, nil
		}
		failures = append(failures, fmt.Errorf("model %d stream: %w", index, err))
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(failures...)
}

// WithTools binds the same tool contract to every endpoint.
func (m *Model) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	bound := make([]einomodel.ToolCallingChatModel, 0, len(m.models))
	for index, candidate := range m.models {
		modelWithTools, err := candidate.WithTools(tools)
		if err != nil {
			return nil, fmt.Errorf("bind tools to model %d: %w", index, err)
		}
		bound = append(bound, modelWithTools)
	}
	return New(bound...)
}
