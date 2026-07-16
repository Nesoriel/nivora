package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

// ProviderRecorder decorates a product Provider and records only successful
// support-case references. All read operations remain transparent.
type ProviderRecorder struct {
	next     provider.Provider
	store    Store
	tenantID string
	now      func() time.Time
}

// NewProviderRecorder creates a Provider audit decorator.
func NewProviderRecorder(next provider.Provider, store Store, tenantID string) (*ProviderRecorder, error) {
	if next == nil {
		return nil, errors.New("next provider is required")
	}
	if store == nil {
		return nil, errors.New("conversation store is required")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, errors.New("tenant ID is required")
	}
	return &ProviderRecorder{next: next, store: store, tenantID: tenantID, now: time.Now}, nil
}

func (p *ProviderRecorder) Capabilities(ctx context.Context, auth provider.RequestAuth) (domain.CapabilitySet, error) {
	return p.next.Capabilities(ctx, auth)
}

func (p *ProviderRecorder) CustomerContext(ctx context.Context, auth provider.RequestAuth) (domain.CustomerContext, error) {
	return p.next.CustomerContext(ctx, auth)
}

func (p *ProviderRecorder) SearchKnowledge(ctx context.Context, auth provider.RequestAuth, query string, limit int) ([]domain.KnowledgeItem, error) {
	return p.next.SearchKnowledge(ctx, auth, query, limit)
}

func (p *ProviderRecorder) ListResources(ctx context.Context, auth provider.RequestAuth, limit int, status string) ([]domain.Resource, error) {
	return p.next.ListResources(ctx, auth, limit, status)
}

func (p *ProviderRecorder) DiagnoseResource(ctx context.Context, auth provider.RequestAuth, resourceID string) (domain.Diagnosis, error) {
	return p.next.DiagnoseResource(ctx, auth, resourceID)
}

func (p *ProviderRecorder) ListTransactions(ctx context.Context, auth provider.RequestAuth, resourceID string, limit int) ([]domain.Transaction, error) {
	return p.next.ListTransactions(ctx, auth, resourceID, limit)
}

func (p *ProviderRecorder) CreateCase(ctx context.Context, auth provider.RequestAuth, input domain.CreateCaseInput) (domain.SupportCase, error) {
	result, err := p.next.CreateCase(ctx, auth, input)
	if err != nil {
		return domain.SupportCase{}, err
	}
	now := p.now().UTC()
	createdAt := result.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	if err := p.store.RecordSupportCase(ctx, SupportCaseRecord{
		TenantID:       p.tenantID,
		ConversationID: input.ConversationID,
		ProviderCaseID: result.ID,
		Status:         result.Status,
		CreatedAt:      createdAt,
		UpdatedAt:      now,
	}); err != nil {
		return domain.SupportCase{}, err
	}
	return result, nil
}
