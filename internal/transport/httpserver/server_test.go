package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/config"
	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

type fakeStreamer struct{}

func (fakeStreamer) Stream(_ context.Context, _ domain.ChatRequest, _ provider.RequestAuth, emit func(domain.StreamEvent) error) error {
	if err := emit(domain.StreamEvent{Type: "message.delta", Content: "hello"}); err != nil {
		return err
	}
	return emit(domain.StreamEvent{Type: "done"})
}

func testConfig() config.Config {
	return config.Config{
		SharedSecret:     "secret",
		ArkAPIKey:        "configured",
		ArkModel:         "configured",
		ProviderBaseURL:  "http://provider.invalid",
		TenantID:         "lumio",
		RequestTimeout:   time.Second,
		MaxHistoryTurns:  4,
		MaxQuestionBytes: 4096,
	}
}

func TestChatRequiresInternalKey(t *testing.T) {
	t.Parallel()
	server := New(testConfig(), fakeStreamer{}, slog.Default())
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", strings.NewReader(`{"question":"hello","tenant":{"id":"lumio","brand":{"name":"Lumio"}}}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestChatStreamsStableEvents(t *testing.T) {
	t.Parallel()
	server := New(testConfig(), fakeStreamer{}, slog.Default())
	payload, _ := json.Marshal(domain.ChatRequest{Question: "hello", Tenant: domain.TenantContext{ID: "lumio", Brand: domain.Brand{Name: "Lumio"}}})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(payload))
	request.Header.Set("X-Nivora-Key", "secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	scanner := bufio.NewScanner(strings.NewReader(response.Body.String()))
	var dataLines int
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			dataLines++
		}
	}
	if dataLines != 2 {
		t.Fatalf("expected 2 SSE data lines, got %d: %s", dataLines, response.Body.String())
	}
}
