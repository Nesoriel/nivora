package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/pkg/knowledge"
)

type backend struct{}

func (backend) Search(context.Context, knowledge.Query) ([]knowledge.Candidate, error) {
	return []knowledge.Candidate{{
		TenantID:       "tenant-a",
		DocumentID:     "doc-1",
		ChunkID:        "chunk-1",
		SourceTitle:    "Support guide",
		SourceVersion:  "v1",
		Content:        "Approved answer.",
		ApprovalStatus: "approved",
		EffectiveAt:    time.Now().Add(-time.Hour),
		Score:          0.9,
	}}, nil
}

func TestSearchRequiresServiceKey(t *testing.T) {
	service, _ := knowledge.NewService(backend{})
	server, _ := New(service, "secret")
	request := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewBufferString(`{"tenant_id":"tenant-a","query":"help"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestSearchReturnsApprovedItems(t *testing.T) {
	service, _ := knowledge.NewService(backend{})
	server, _ := New(service, "secret")
	request := httptest.NewRequest(http.MethodPost, "/v1/search", bytes.NewBufferString(`{"tenant_id":"tenant-a","query":"help","min_score":0.8}`))
	request.Header.Set("X-Nivora-Knowledge-Key", "secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Contains(response.Body.Bytes(), []byte(`"chunk_id":"chunk-1"`)) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}
