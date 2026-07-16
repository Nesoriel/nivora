package acceptance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeClientDoesNotLeakConfiguredSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Nivora-Key") != "secret" {
			t.Fatal("service key was not sent")
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"tenant_not_allowed"}`))
	}))
	defer server.Close()
	client := ProbeClient{BaseURL: server.URL, SharedSecret: "secret", HTTPClient: server.Client()}
	result, err := client.Run(context.Background(), ProbeCase{
		ID:                 "tenant",
		Path:               "/v1/chat/stream",
		Body:               []byte(`{"question":"hello"}`),
		ExpectedStatus:     http.StatusBadRequest,
		RequiredSubstrings: []string{"tenant_not_allowed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("unexpected probe failure: %#v", result)
	}
	encoded := strings.Join(result.Failures, " ")
	if strings.Contains(encoded, "secret") {
		t.Fatal("probe result leaked configured secret")
	}
}

func TestProbeClientDetectsStatusAndContentFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("internal recipe"))
	}))
	defer server.Close()
	client := ProbeClient{BaseURL: server.URL, SharedSecret: "secret", HTTPClient: server.Client()}
	result, err := client.Run(context.Background(), ProbeCase{
		ID:                  "failure",
		Path:                "/probe",
		ExpectedStatus:      http.StatusBadRequest,
		ForbiddenSubstrings: []string{"internal recipe"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed || len(result.Failures) != 2 {
		t.Fatalf("expected two failures, got %#v", result)
	}
}
