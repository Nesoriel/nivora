package vikingdb

import (
	"context"
	"testing"
	"time"

	volcretriever "github.com/cloudwego/eino-ext/components/retriever/volc_vikingdb"
	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/Nesoriel/nivora/pkg/knowledge"
)

type fakeRetriever struct {
	documents []*schema.Document
	options   *einoretriever.Options
}

func (f *fakeRetriever) Retrieve(_ context.Context, _ string, options ...einoretriever.Option) ([]*schema.Document, error) {
	f.options = einoretriever.GetCommonOptions(&einoretriever.Options{}, options...)
	return f.documents, nil
}

func TestBackendPushesTenantAndApprovalFilters(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	fields := map[string]any{
		"tenant_id":       "tenant-a",
		"document_id":     "doc-1",
		"chunk_id":        "chunk-1",
		"source_title":    "Refund policy",
		"source_version":  "v4",
		"source_uri":      "https://support.example/refunds",
		"approval_status": "approved",
		"effective_at":    now.Add(-time.Hour).Format(time.RFC3339),
		"expires_at":      now.Add(time.Hour).Unix(),
	}
	document := (&schema.Document{
		ID:      "viking-primary-key",
		Content: "Refunds are verified from the transaction ledger.",
		MetaData: map[string]any{
			volcretriever.ExtraKeyVikingDBFields: fields,
		},
	}).WithScore(0.91)
	fake := &fakeRetriever{documents: []*schema.Document{document}}
	backend, err := NewWithRetriever(fake, FieldMapping{}, 3)
	if err != nil {
		t.Fatal(err)
	}

	candidates, err := backend.Search(context.Background(), knowledge.Query{
		TenantID: "tenant-a",
		Text:     "refund",
		Limit:    6,
		MinScore: 0.8,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].DocumentID != "doc-1" || candidates[0].SourceVersion != "v4" {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
	if fake.options == nil || fake.options.DSLInfo == nil {
		t.Fatal("expected VikingDB filter DSL")
	}
	conditions, ok := fake.options.DSLInfo["conditions"].([]map[string]any)
	if !ok || len(conditions) != 2 {
		t.Fatalf("unexpected filter: %#v", fake.options.DSLInfo)
	}
	if conditions[0]["field"] != "tenant_id" || conditions[0]["value"] != "tenant-a" {
		t.Fatalf("tenant filter missing: %#v", conditions)
	}
	if conditions[1]["field"] != "approval_status" || conditions[1]["value"] != "approved" {
		t.Fatalf("approval filter missing: %#v", conditions)
	}
	if fake.options.TopK == nil || *fake.options.TopK != 18 {
		t.Fatalf("expected oversampled top-k 18, got %#v", fake.options.TopK)
	}
}

func TestBackendLeavesMalformedMetadataForOuterFailClosedValidation(t *testing.T) {
	document := (&schema.Document{
		ID:      "chunk-only",
		Content: "Untrusted content",
		MetaData: map[string]any{
			volcretriever.ExtraKeyVikingDBFields: map[string]any{
				"tenant_id":       "wrong-tenant",
				"approval_status": "draft",
			},
		},
	}).WithScore(0.99)
	fake := &fakeRetriever{documents: []*schema.Document{document}}
	backend, _ := NewWithRetriever(fake, FieldMapping{}, 1)
	service, _ := knowledge.NewService(backend)

	items, err := service.Search(context.Background(), knowledge.Query{
		TenantID: "tenant-a",
		Text:     "question",
		MinScore: 0.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("malformed cross-tenant knowledge leaked: %#v", items)
	}
}
