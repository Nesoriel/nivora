package agent

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

type fakeProvider struct {
	lastCase domain.CreateCaseInput
}

func (f *fakeProvider) Capabilities(context.Context, provider.RequestAuth) (domain.CapabilitySet, error) {
	return allCapabilities(), nil
}

func (f *fakeProvider) CustomerContext(context.Context, provider.RequestAuth) (domain.CustomerContext, error) {
	return domain.CustomerContext{CustomerID: "customer-1"}, nil
}

func (f *fakeProvider) SearchKnowledge(context.Context, provider.RequestAuth, string, int) ([]domain.KnowledgeItem, error) {
	return nil, nil
}

func (f *fakeProvider) ListResources(context.Context, provider.RequestAuth, int, string) ([]domain.Resource, error) {
	return nil, nil
}

func (f *fakeProvider) DiagnoseResource(context.Context, provider.RequestAuth, string) (domain.Diagnosis, error) {
	return domain.Diagnosis{}, nil
}

func (f *fakeProvider) ListTransactions(context.Context, provider.RequestAuth, string, int) ([]domain.Transaction, error) {
	return nil, nil
}

func (f *fakeProvider) CreateCase(_ context.Context, _ provider.RequestAuth, input domain.CreateCaseInput) (domain.SupportCase, error) {
	f.lastCase = input
	return domain.SupportCase{ID: "case-1", Status: "open", CreatedAt: time.Now()}, nil
}

func TestAnonymousPrincipalOnlyGetsExplicitlyAuthorizedTools(t *testing.T) {
	t.Parallel()
	backend := &fakeProvider{}
	service := &Service{provider: backend}
	tools, err := service.buildTools(provider.RequestAuth{}, "conv-1", domain.Principal{
		Scopes: []string{domain.ScopeKnowledgeRead, domain.ScopeCaseCreate},
	}, allCapabilities())
	if err != nil {
		t.Fatal(err)
	}
	assertToolNames(t, tools, []string{"create_support_case", "search_knowledge"})
}

func TestAuthenticatedPrincipalGetsCapabilityIntersection(t *testing.T) {
	t.Parallel()
	backend := &fakeProvider{}
	service := &Service{provider: backend}
	tools, err := service.buildTools(provider.RequestAuth{BearerToken: "signed"}, "conv-1", domain.Principal{
		Authenticated: true,
		Scopes: []string{
			domain.ScopeKnowledgeRead,
			domain.ScopeCustomerRead,
			domain.ScopeResourceRead,
			domain.ScopeTransactionRead,
			domain.ScopeCaseCreate,
		},
	}, allCapabilities())
	if err != nil {
		t.Fatal(err)
	}
	assertToolNames(t, tools, []string{
		"create_support_case",
		"diagnose_resource",
		"get_customer_context",
		"list_customer_resources",
		"list_transactions",
		"search_knowledge",
	})
}

func TestCreateCaseToolUsesStableIdempotencyKey(t *testing.T) {
	backend := &fakeProvider{}
	service := &Service{provider: backend}
	tools, err := service.buildTools(provider.RequestAuth{}, "conv-1", domain.Principal{
		Scopes: []string{domain.ScopeCaseCreate},
	}, allCapabilities())
	if err != nil {
		t.Fatal(err)
	}

	var caseTool tool.InvokableTool
	for _, candidate := range tools {
		info, infoErr := candidate.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info.Name == "create_support_case" {
			caseTool = candidate.(tool.InvokableTool)
		}
	}
	if caseTool == nil {
		t.Fatal("create_support_case tool not found")
	}

	arguments := `{"subject":"Video failed","summary":"The generation failed.","resource_ids":["video-2","video-1"],"priority":"normal"}`
	if _, err := caseTool.InvokableRun(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	first := backend.lastCase.IdempotencyKey
	if first == "" {
		t.Fatal("expected idempotency key")
	}
	if _, err := caseTool.InvokableRun(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	if backend.lastCase.IdempotencyKey != first {
		t.Fatalf("idempotency key changed: %q != %q", backend.lastCase.IdempotencyKey, first)
	}
}

func allCapabilities() domain.CapabilitySet {
	return domain.CapabilitySet{
		Provider: "test",
		Version:  "1.0",
		Capabilities: []string{
			domain.CapabilityKnowledgeSearch,
			domain.CapabilityCustomerContextRead,
			domain.CapabilityResourceList,
			domain.CapabilityResourceDiagnose,
			domain.CapabilityTransactionRead,
			domain.CapabilityCaseCreate,
		},
	}
}

func assertToolNames(t *testing.T, tools []tool.BaseTool, expected []string) {
	t.Helper()
	var names []string
	for _, candidate := range tools {
		info, err := candidate.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, info.Name)
	}
	sort.Strings(names)
	sort.Strings(expected)
	if len(names) != len(expected) {
		t.Fatalf("unexpected tools: got %v want %v", names, expected)
	}
	for index := range names {
		if names[index] != expected[index] {
			t.Fatalf("unexpected tools: got %v want %v", names, expected)
		}
	}
}
