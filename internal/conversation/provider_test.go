package conversation

import (
	"context"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

type caseProvider struct{}

func (caseProvider) Capabilities(context.Context, provider.RequestAuth) (domain.CapabilitySet, error) {
	return domain.CapabilitySet{}, nil
}
func (caseProvider) CustomerContext(context.Context, provider.RequestAuth) (domain.CustomerContext, error) {
	return domain.CustomerContext{}, nil
}
func (caseProvider) SearchKnowledge(context.Context, provider.RequestAuth, string, int) ([]domain.KnowledgeItem, error) {
	return nil, nil
}
func (caseProvider) ListResources(context.Context, provider.RequestAuth, int, string) ([]domain.Resource, error) {
	return nil, nil
}
func (caseProvider) DiagnoseResource(context.Context, provider.RequestAuth, string) (domain.Diagnosis, error) {
	return domain.Diagnosis{}, nil
}
func (caseProvider) ListTransactions(context.Context, provider.RequestAuth, string, int) ([]domain.Transaction, error) {
	return nil, nil
}
func (caseProvider) CreateCase(context.Context, provider.RequestAuth, domain.CreateCaseInput) (domain.SupportCase, error) {
	return domain.SupportCase{ID: "case-42", Status: "open", CreatedAt: time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)}, nil
}

type caseStore struct {
	Store
	record SupportCaseRecord
}

func (s *caseStore) RecordSupportCase(_ context.Context, record SupportCaseRecord) error {
	s.record = record
	return nil
}

func TestProviderRecorderStoresCaseReference(t *testing.T) {
	store := &caseStore{Store: Nop()}
	recorder, err := NewProviderRecorder(caseProvider{}, store, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	result, err := recorder.CreateCase(context.Background(), provider.RequestAuth{}, domain.CreateCaseInput{ConversationID: "conv-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "case-42" || store.record.ProviderCaseID != "case-42" || store.record.ConversationID != "conv-1" || store.record.TenantID != "tenant-a" {
		t.Fatalf("unexpected case audit: result=%#v record=%#v", result, store.record)
	}
}
