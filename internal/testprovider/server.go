package testprovider

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Nesoriel/nivora/internal/domain"
)

// Config controls a synthetic Provider for isolated acceptance environments.
type Config struct {
	SharedSecret string
	BearerToken  string
	Delay        time.Duration
}

// Server implements Provider API v1 with synthetic, non-production data.
type Server struct {
	config Config
	mux    *http.ServeMux
	mu     sync.Mutex
	cases  map[string]domain.SupportCase
}

// New creates a deterministic synthetic Provider.
func New(config Config) *Server {
	server := &Server{config: config, mux: http.NewServeMux(), cases: make(map[string]domain.SupportCase)}
	server.mux.HandleFunc("GET /api/internal/support/capabilities", server.capabilities)
	server.mux.HandleFunc("GET /api/internal/support/context", server.context)
	server.mux.HandleFunc("GET /api/internal/support/knowledge", server.knowledge)
	server.mux.HandleFunc("GET /api/internal/support/resources", server.resources)
	server.mux.HandleFunc("GET /api/internal/support/diagnosis", server.diagnosis)
	server.mux.HandleFunc("GET /api/internal/support/transactions", server.transactions)
	server.mux.HandleFunc("POST /api/internal/support/cases", server.createCase)
	return server
}

// Handler returns the synthetic Provider handler.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !constantTimeEqual(request.Header.Get("X-Nivora-Provider-Key"), s.config.SharedSecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		if s.config.BearerToken != "" {
			token := strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer ")
			if !constantTimeEqual(token, s.config.BearerToken) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_context"})
				return
			}
		}
		if s.config.Delay > 0 {
			timer := time.NewTimer(s.config.Delay)
			defer timer.Stop()
			select {
			case <-request.Context().Done():
				return
			case <-timer.C:
			}
		}
		s.mux.ServeHTTP(w, request)
	})
}

func (s *Server) capabilities(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, domain.CapabilitySet{
		Provider: "nivora-synthetic-provider",
		Version:  "1.0",
		Capabilities: []string{
			domain.CapabilityKnowledgeSearch,
			domain.CapabilityCustomerContextRead,
			domain.CapabilityResourceList,
			domain.CapabilityResourceDiagnose,
			domain.CapabilityTransactionRead,
			domain.CapabilityCaseCreate,
		},
	})
}

func (s *Server) context(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, domain.CustomerContext{
		CustomerID: "synthetic-customer",
		Attributes: map[string]any{"credit_balance": 100},
	})
}

func (s *Server) knowledge(w http.ResponseWriter, request *http.Request) {
	limit := queryLimit(request, 6)
	items := []domain.KnowledgeItem{
		{ID: "refund-policy", Title: "退款与积分返还说明", Content: "生成失败后，以交易流水中的退款记录为准。", Score: 0.96, Source: "synthetic://refund-policy/v1"},
		{ID: "human-support-guide", Title: "人工客服说明", Content: "无法自行解决时，可以创建人工客服工单。", Score: 0.92, Source: "synthetic://human-support/v1"},
	}
	if limit < len(items) {
		items = items[:limit]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) resources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": []domain.Resource{{
		ID:        "video-failed-1",
		Type:      "video_generation",
		Title:     "Synthetic failed video",
		Status:    "failed",
		CreatedAt: time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
	}}})
}

func (s *Server) diagnosis(w http.ResponseWriter, request *http.Request) {
	resourceID := request.URL.Query().Get("resource_id")
	if resourceID != "video-failed-1" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "resource_not_found"})
		return
	}
	writeJSON(w, http.StatusOK, domain.Diagnosis{
		ResourceID:  resourceID,
		Status:      "failed",
		Category:    "upstream_generation_failed",
		Message:     "The synthetic generation failed before delivery.",
		Charged:     10,
		Refunded:    10,
		Suggestions: []string{"Retry with the same approved parameters."},
	})
}

func (s *Server) transactions(w http.ResponseWriter, request *http.Request) {
	resourceID := request.URL.Query().Get("resource_id")
	writeJSON(w, http.StatusOK, map[string]any{"items": []domain.Transaction{
		{ID: "tx-charge-1", ResourceID: resourceID, Type: "charge", Amount: -10, CreatedAt: time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)},
		{ID: "tx-refund-1", ResourceID: resourceID, Type: "refund", Amount: 10, CreatedAt: time.Date(2026, 7, 16, 9, 1, 0, 0, time.UTC)},
	}})
}

func (s *Server) createCase(w http.ResponseWriter, request *http.Request) {
	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key_required"})
		return
	}
	var input domain.CreateCaseInput
	if err := json.NewDecoder(http.MaxBytesReader(w, request.Body, 64*1024)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, exists := s.cases[idempotencyKey]; exists {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	caseRecord := domain.SupportCase{
		ID:        "synthetic-case-" + strconv.Itoa(len(s.cases)+1),
		Status:    "open",
		CreatedAt: time.Now().UTC(),
	}
	s.cases[idempotencyKey] = caseRecord
	writeJSON(w, http.StatusCreated, caseRecord)
}

func queryLimit(request *http.Request, fallback int) int {
	value, err := strconv.Atoi(request.URL.Query().Get("limit"))
	if err != nil || value < 1 {
		return fallback
	}
	return value
}

func constantTimeEqual(got, expected string) bool {
	if got == "" || expected == "" || len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
