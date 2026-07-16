package failover

import (
	"context"
	"errors"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeModel struct {
	response *schema.Message
	err      error
}

func (f *fakeModel) Generate(context.Context, []*schema.Message, ...einomodel.Option) (*schema.Message, error) {
	return f.response, f.err
}

func (f *fakeModel) Stream(context.Context, []*schema.Message, ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, f.err
}

func (f *fakeModel) WithTools([]*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return f, nil
}

func TestGenerateFallsBackToNextModel(t *testing.T) {
	t.Parallel()

	model, err := New(
		&fakeModel{err: errors.New("primary unavailable")},
		&fakeModel{response: &schema.Message{Role: schema.Assistant, Content: "ok"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	output, err := model.Generate(context.Background(), []*schema.Message{{Role: schema.User, Content: "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.Content != "ok" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestGenerateReturnsJoinedFailures(t *testing.T) {
	t.Parallel()

	model, err := New(
		&fakeModel{err: errors.New("primary unavailable")},
		&fakeModel{err: errors.New("backup unavailable")},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = model.Generate(context.Background(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}
