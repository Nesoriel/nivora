package httpserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nesoriel/nivora/internal/config"
	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
	"github.com/Nesoriel/nivora/internal/telemetry"
)

type fakeStreamer struct{}

func (fakeStreamer) Stream(_ context.Context, _ domain.ChatRequest, _ provider.RequestAuth, emit func(domain.StreamEvent) error) error {
	if err := emit(domain.StreamEvent{Type: "message.delta", Content: "hello"}); err != nil {
		return err
	}
	return emit(domain.StreamEvent{Type: "done"})
}

type fakeChecker struct {
	err error
}

func (f fakeChecker) Check(context.Context) error {
	return f.err
}

type blockingStreamer struct {
	started chan struct{}
	release chan struct{}
}

func (b blockingStreamer) Stream(ctx context.Context, _ domain.ChatRequest, _ provider.RequestAuth, emit func(domain.StreamEvent) error) error {
	close(b.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return emit(domain.StreamEvent{Type: "done"})
	}
}

func testConfig() config.Config {
	return config.Config{
		SharedSecret:         "secret",
		ProviderSharedSecret: "provider-secret",
		ArkAPIKey:            "configured",
		ArkModels:            []string{"configured"},
		ProviderBaseURL:      "http://provider.invalid",
		TenantID:             "lumio",
		RequestTimeout:       time.Second,
		ReadinessTimeout:     time.Second,
		ReadinessCacheTTL:    time.Second,
		QueueTimeout:         10 * time.Millisecond,
		MaxConcurrentRuns:    4,
		MaxHistoryTurns:      4,
		MaxQuestionBytes:     4096,
	}
}

func anonymousPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := json.Marshal(domain.ChatRequest{
		Question: "hello",
		Tenant:   domain.TenantContext{ID: "lumio", Brand: domain.Brand{Name: "Lumio"}},
		Principal: domain.Principal{
			Scopes: []string{domain.ScopeKnowledgeRead},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestChatRequiresInternalKey(t *testing.T) {
	t.Parallel()
	server := New(testConfig(), fakeStreamer{}, fakeChecker{}, telemetry.New(), slog.Default())
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(anonymousPayload(t)))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestChatStreamsStableEvents(t *testing.T) {
	t.Parallel()
	server := New(testConfig(), fakeStreamer{}, fakeChecker{}, telemetry.New(), slog.Default())
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(anonymousPayload(t)))
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

func TestAuthenticatedPrincipalRequiresProviderContext(t *testing.T) {
	t.Parallel()
	server := New(testConfig(), fakeStreamer{}, fakeChecker{}, telemetry.New(), slog.Default())
	payload, _ := json.Marshal(domain.ChatRequest{
		Question: "hello",
		Tenant:   domain.TenantContext{ID: "lumio"},
		Principal: domain.Principal{
			Authenticated: true,
			Scopes:        []string{domain.ScopeCustomerRead},
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(payload))
	request.Header.Set("X-Nivora-Key", "secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestReadinessChecksProviderDependency(t *testing.T) {
	t.Parallel()
	server := New(testConfig(), fakeStreamer{}, fakeChecker{err: errors.New("provider unavailable")}, telemetry.New(), slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestChatRejectsWhenConcurrencyGateIsFull(t *testing.T) {
	cfg := testConfig()
	cfg.MaxConcurrentRuns = 1
	cfg.QueueTimeout = 5 * time.Millisecond
	streamer := blockingStreamer{started: make(chan struct{}), release: make(chan struct{})}
	server := New(cfg, streamer, fakeChecker{}, telemetry.New(), slog.Default())

	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(anonymousPayload(t)))
		request.Header.Set("X-Nivora-Key", "secret")
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		firstDone <- response
	}()
	<-streamer.started

	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/stream", bytes.NewReader(anonymousPayload(t)))
	secondRequest.Header.Set("X-Nivora-Key", "secret")
	secondResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondResponse, secondRequest)
	if secondResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", secondResponse.Code, secondResponse.Body.String())
	}
	close(streamer.release)
	<-firstDone
}
