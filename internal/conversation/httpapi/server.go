package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Nesoriel/nivora/internal/conversation"
)

// Server exposes support-safe transcripts to trusted internal operators.
type Server struct {
	store    conversation.Store
	tenantID string
	secret   string
}

// New creates a private transcript API.
func New(store conversation.Store, tenantID, secret string) (*Server, error) {
	if store == nil {
		return nil, errors.New("conversation store is required")
	}
	tenantID = strings.TrimSpace(tenantID)
	secret = strings.TrimSpace(secret)
	if tenantID == "" || secret == "" {
		return nil, errors.New("tenant and service secret are required")
	}
	return &Server{store: store, tenantID: tenantID, secret: secret}, nil
}

// Register adds private conversation routes to an existing ServeMux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/conversations/{conversation_id}/transcript", s.transcript)
}

func (s *Server) transcript(w http.ResponseWriter, request *http.Request) {
	if !constantTimeEqual(request.Header.Get("X-Nivora-Key"), s.secret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	conversationID := strings.TrimSpace(request.PathValue("conversation_id"))
	if conversationID == "" || len(conversationID) > 256 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_conversation_id"})
		return
	}
	messages, err := s.store.Transcript(request.Context(), s.tenantID, conversationID)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "conversation_store_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id":       s.tenantID,
		"conversation_id": conversationID,
		"messages":        messages,
	})
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
