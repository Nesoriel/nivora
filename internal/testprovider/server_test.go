package testprovider

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyntheticProviderRequiresBearerForCustomerContext(t *testing.T) {
	server := New(Config{SharedSecret: "provider-secret", BearerToken: "context"})
	request := httptest.NewRequest(http.MethodGet, "/api/internal/support/context", nil)
	request.Header.Set("X-Nivora-Provider-Key", "provider-secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestSyntheticProviderAllowsAnonymousKnowledge(t *testing.T) {
	server := New(Config{SharedSecret: "provider-secret", BearerToken: "context"})
	request := httptest.NewRequest(http.MethodGet, "/api/internal/support/knowledge?q=help", nil)
	request.Header.Set("X-Nivora-Provider-Key", "provider-secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestSyntheticProviderCaseCreationIsIdempotent(t *testing.T) {
	server := New(Config{SharedSecret: "provider-secret", BearerToken: "context"})
	call := func() string {
		request := httptest.NewRequest(http.MethodPost, "/api/internal/support/cases", bytes.NewBufferString(`{"conversation_id":"conv-1","subject":"help","summary":"verified"}`))
		request.Header.Set("X-Nivora-Provider-Key", "provider-secret")
		request.Header.Set("Idempotency-Key", "stable-key")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusCreated && response.Code != http.StatusOK {
			t.Fatalf("unexpected status %d body=%s", response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	first := call()
	second := call()
	if first != second {
		t.Fatalf("idempotent response changed: first=%s second=%s", first, second)
	}
}
