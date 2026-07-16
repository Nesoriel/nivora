package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Nesoriel/nivora/internal/config"
	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
)

// Streamer is implemented by the agent runtime.
type Streamer interface {
	Stream(context.Context, domain.ChatRequest, provider.RequestAuth, func(domain.StreamEvent) error) error
}

// Server exposes Nivora over HTTP.
type Server struct {
	cfg      config.Config
	streamer Streamer
	logger   *slog.Logger
	mux      *http.ServeMux
}

// New builds an HTTP server handler.
func New(cfg config.Config, streamer Streamer, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	server := &Server{cfg: cfg, streamer: streamer, logger: logger, mux: http.NewServeMux()}
	server.mux.HandleFunc("GET /healthz", server.health)
	server.mux.HandleFunc("GET /readyz", server.ready)
	server.mux.HandleFunc("GET /version", server.version)
	server.mux.HandleFunc("POST /v1/chat/stream", server.chat)
	return server
}

// Handler returns the configured HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.withRecovery(s.withRequestID(s.mux))
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) ready(w http.ResponseWriter, _ *http.Request) {
	if !s.cfg.Ready() || s.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "runtime_not_configured"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.cfg.Version, "commit": s.cfg.Commit})
}

func (s *Server) chat(w http.ResponseWriter, request *http.Request) {
	if !s.cfg.Ready() || s.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime_not_configured"})
		return
	}
	if !constantTimeEqual(request.Header.Get("X-Nivora-Key"), s.cfg.SharedSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	maxBodyBytes := int64(s.cfg.MaxQuestionBytes * (s.cfg.MaxHistoryTurns + 2))
	if maxBodyBytes > 2<<20 {
		maxBodyBytes = 2 << 20
	}
	request.Body = http.MaxBytesReader(w, request.Body, maxBodyBytes)
	defer request.Body.Close()

	var input domain.ChatRequest
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if err := validateRequest(&input, s.cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if input.ConversationID == "" {
		input.ConversationID = newID("conv")
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming_unsupported"})
		return
	}

	requestID := requestIDFrom(request.Context())
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx, cancel := context.WithTimeout(request.Context(), s.cfg.RequestTimeout)
	defer cancel()

	bearer := bearerToken(request.Header.Get("Authorization"))
	auth := provider.RequestAuth{BearerToken: bearer}

	emit := func(event domain.StreamEvent) error {
		event.RequestID = requestID
		event.ConversationID = input.ConversationID
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("event: " + event.Type + "\ndata: ")); err != nil {
			return err
		}
		if _, err := w.Write(encoded); err != nil {
			return err
		}
		if _, err := io.WriteString(w, "\n\n"); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := s.streamer.Stream(ctx, input, auth, emit); err != nil {
		s.logger.Error("agent run failed", "request_id", requestID, "conversation_id", input.ConversationID, "error", err)
		_ = emit(domain.StreamEvent{Type: "error", Code: "agent_run_failed", Content: "客服暂时无法完成本次核查，请稍后重试或联系人工客服。"})
	}
}

func validateRequest(request *domain.ChatRequest, cfg config.Config) error {
	request.Question = strings.TrimSpace(request.Question)
	if request.Question == "" {
		return errors.New("question_required")
	}
	if len(request.Question) > cfg.MaxQuestionBytes {
		return errors.New("question_too_large")
	}
	request.Tenant.ID = strings.TrimSpace(request.Tenant.ID)
	if request.Tenant.ID != cfg.TenantID {
		return errors.New("tenant_not_allowed")
	}
	if len(request.History) > cfg.MaxHistoryTurns {
		request.History = request.History[len(request.History)-cfg.MaxHistoryTurns:]
	}
	for index := range request.History {
		turn := &request.History[index]
		turn.Role = strings.ToLower(strings.TrimSpace(turn.Role))
		turn.Content = strings.TrimSpace(turn.Content)
		if turn.Role != "user" && turn.Role != "assistant" {
			return errors.New("invalid_history_role")
		}
		if len(turn.Content) > cfg.MaxQuestionBytes {
			return errors.New("history_turn_too_large")
		}
	}
	return nil
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = newID("req")
		}
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, request.WithContext(context.WithValue(request.Context(), requestIDKey{}, requestID)))
	})
}

func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("http panic", "request_id", requestIDFrom(request.Context()), "panic", recovered)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			}
		}()
		next.ServeHTTP(w, request)
	})
}

type requestIDKey struct{}

func requestIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func newID(prefix string) string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return prefix + "_fallback"
	}
	return prefix + "_" + hex.EncodeToString(raw)
}

func bearerToken(value string) string {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
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
