package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/telemetry"
)

func TestAnonymousPrincipalCannotRequestTransactionScope(t *testing.T) {
	server := New(testConfig(), fakeStreamer{}, fakeChecker{}, telemetry.New(), nil)
	payload, _ := json.Marshal(domain.ChatRequest{
		Question:  "show transactions",
		Tenant:    domain.TenantContext{ID: "lumio"},
		Principal: domain.Principal{Scopes: []string{domain.ScopeTransactionRead}},
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(payload))
	request.Header.Set("X-Nivora-Key", "secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("anonymous_scope_not_allowed")) {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
}

func TestAnonymousPrincipalRejectsBearerContext(t *testing.T) {
	server := New(testConfig(), fakeStreamer{}, fakeChecker{}, telemetry.New(), nil)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(anonymousPayload(t)))
	request.Header.Set("X-Nivora-Key", "secret")
	request.Header.Set("Authorization", "Bearer replayed-context")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("principal_context_mismatch")) {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
}

func TestUnknownRequestFieldsAreRejectedWithoutReflection(t *testing.T) {
	server := New(testConfig(), fakeStreamer{}, fakeChecker{}, telemetry.New(), nil)
	payload := []byte(`{"question":"hello","tenant":{"id":"lumio"},"principal":{"authenticated":false,"scopes":["knowledge:read"]},"provider_url":"http://attacker.invalid"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(payload))
	request.Header.Set("X-Nivora-Key", "secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte("invalid_request")) {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("attacker.invalid")) {
		t.Fatalf("response reflected untrusted data: %s", response.Body.String())
	}
}
