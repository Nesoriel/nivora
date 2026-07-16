package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/pkg/knowledge"
)

const maxBodyBytes = 64 * 1024

// Searcher is implemented by the approved knowledge service.
type Searcher interface {
	Search(ctx interface{ Done() <-chan struct{} }, query knowledge.Query) ([]knowledge.Item, error)
}

// KnowledgeSearcher uses the concrete context-aware service signature.
type KnowledgeSearcher interface {
	Search(ctxContext, knowledge.Query) ([]knowledge.Item, error)
}

// ctxContext is the minimal context surface accepted by KnowledgeSearcher.
type ctxContext interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(key any) any
}

// Server exposes the private Provider-side knowledge API.
type Server struct {
	service *knowledge.Service
	secret  string
	mux     *http.ServeMux
}

// New creates a private approved-knowledge HTTP handler.
func New(service *knowledge.Service, secret string) (*Server, error) {
	if service == nil {
		return nil, errors.New("knowledge service is required")
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, errors.New("knowledge service secret is required")
	}
	server := &Server{service: service, secret: secret, mux: http.NewServeMux()}
	server.mux.HandleFunc("GET /healthz", server.health)
	server.mux.HandleFunc("GET /readyz", server.ready)
	server.mux.HandleFunc("POST /v1/search", server.search)
	return server, nil
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) ready(w http.ResponseWriter, request *http.Request) {
	if !constantTimeEqual(request.Header.Get("X-Nivora-Knowledge-Key"), s.secret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) search(w http.ResponseWriter, request *http.Request) {
	if !constantTimeEqual(request.Header.Get("X-Nivora-Knowledge-Key"), s.secret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, maxBodyBytes)
	defer request.Body.Close()
	var input struct {
		TenantID string  `json:"tenant_id"`
		Query    string  `json:"query"`
		Limit    int     `json:"limit,omitempty"`
		MinScore float64 `json:"min_score,omitempty"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	items, err := s.service.Search(request.Context(), knowledge.Query{
		TenantID: input.TenantID,
		Text:     input.Query,
		Limit:    input.Limit,
		MinScore: input.MinScore,
	})
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "knowledge_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
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
