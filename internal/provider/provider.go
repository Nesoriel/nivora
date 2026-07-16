package provider

import (
	"context"

	"github.com/Nesoriel/nivora/internal/domain"
)

// RequestAuth carries the short-lived customer context issued by a provider.
type RequestAuth struct {
	BearerToken string
}

// Provider is the boundary between Nivora and a product backend.
type Provider interface {
	Capabilities(context.Context, RequestAuth) (domain.CapabilitySet, error)
	CustomerContext(context.Context, RequestAuth) (domain.CustomerContext, error)
	SearchKnowledge(context.Context, RequestAuth, string, int) ([]domain.KnowledgeItem, error)
	ListResources(context.Context, RequestAuth, int, string) ([]domain.Resource, error)
	DiagnoseResource(context.Context, RequestAuth, string) (domain.Diagnosis, error)
	ListTransactions(context.Context, RequestAuth, string, int) ([]domain.Transaction, error)
	CreateCase(context.Context, RequestAuth, domain.CreateCaseInput) (domain.SupportCase, error)
}
