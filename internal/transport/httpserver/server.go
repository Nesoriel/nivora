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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nesoriel/nivora/internal/config"
	"github.com/Nesoriel/nivora/internal/domain"
	"github.com/Nesoriel/nivora/internal/provider"
	"github.com/Nesoriel/nivora/internal/telemetry"
)

// Streamer is implemented by the agent runtime.
type Streamer interface {
	Stream(context.Context, domain.ChatRequest, provider.RequestAuth, func(domain.StreamEvent) error) error
}

// DependencyChecker verifies external dependencies used by readiness probes.
type DependencyChecker interface {
	Check(context.Context) error
}

// Server exposes Nivora over HTTP.
type Server struct {
	cfg          config.Config
	streamer     Streamer
	checker      DependencyChecker
	metrics      *telemetry.Metrics
	logger       *slog.Logger
	mux          *http.ServeMux
	gate         chan struct{}
	readinessMu  sync.Mutex
	readinessAt  time.Time
	readinessErr error
}

// New builds an HTTP server handler.
func New(cfg config.Config, streamer Streamer, checker DependencyChecker, metrics *telemetry.Metrics, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if metrics == nil {
		metrics = telemetry.New()
	}
	server := &Server{
		cfg:      cfg,
		streamer: streamer,
		checker:  checker,
		metrics:  metrics,
		logger:   logger,
		mux:      http.NewServeMux(),
		gate:     make(chan struct{}, cfg.MaxConcurrentRuns),
	}
	server.mux.HandleFunc("GET /healthz", server.health)
	server.mux.HandleFunc("GET /readyz", server.ready)
	server.mux.HandleFunc("GET /version", server.version)
	server.mux.HandleFunc("GET /metrics", server.prometheusMetrics)
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

func (s *Server) ready(w http.ResponseWriter, request *http.Request) {
	if !s.cfg.Ready() || s.streamer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "runtime_not_configured"})
		return
	}
	if err := s.checkDependencies(request.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "reason": "dependency_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.cfg.Version, "commit": s.cfg.Commit})
}

func (s *Server) prometheusMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	if err := s.metrics.WritePrometheus(w); err != nil {
		s.logger.Error("write metrics", "error", err)
	}
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

	bearer := bearerToken(request.Header.Get("Authorization"))
	if input.Principal.Authenticated && bearer == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "provider_context_required"})
		return
	}
	if !input.Principal.Authenticated && bearer != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "principal_context_mismatch"})
		return
	}
	if !s.acquire(request.Context()) {
		s.metrics.QueueRejected()
		w.Header().Set("Retry-After", "1")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "service_busy"})
		return
	}
	defer s.release()

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
	w.Header().Set("X-Nivora-Conversation-ID", input.ConversationID)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx, cancel := context.WithTimeout(request.Context(), s.cfg.RequestTimeout)
	defer cancel()

	auth := provider.RequestAuth{BearerToken: bearer}
	var writeMu sync.Mutex
	emit := func(event domain.StreamEvent) error {
		event.RequestID = requestID
		event.ConversationID = input.ConversationID
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		writeMu.Lock()
		defer writeMu.Unlock()
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
		if event.Type == "tool.started" {
			s.metrics.ToolStarted(event.ToolName)
		}
		return nil
	}

	stopHeartbeat := make(chan struct{})
	if s.cfg.SSEHeartbeat > 0 {
		go func() {
			ticker := time.NewTicker(s.cfg.SSEHeartbeat)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					writeMu.Lock()
					_, _ = io.WriteString(w, ": ping\n\n")
					flusher.Flush()
					writeMu.Unlock()
				}
			}
		}()
	}
	defer close(stopHeartbeat)

	s.metrics.RunStarted()
	success := false
	defer func() { s.metrics.RunFinished(success) }()

	if err := s.streamer.Stream(ctx, input, auth, emit); err != nil {
		s.logger.Error("agent run failed", "request_id", requestID, "conversation_id", input.ConversationID, "error", err)
		_ = emit(domain.StreamEvent{Type: "error", Code: "agent_run_failed", Content: "客服暂时无法完成本次核查，请稍后重试或联系人工客服。"})
		return
	}
	success = true
}

func (s *Server) acquire(ctx context.Context) bool {
	if s.cfg.QueueTimeout == 0 {
		select {
		case s.gate <- struct{}{}:
			return true
		default:
			return false
		}
	}
	timer := time.NewTimer(s.cfg.QueueTimeout)
	defer timer.Stop()
	select {
	case s.gate <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}

func (s *Server) release() {
	<-s.gate
}

func (s *Server) checkDependencies(parent context.Context) error {
	if s.checker == nil {
		return errors.New("dependency checker is not configured")
	}

	s.readinessMu.Lock()
	defer s.readinessMu.Unlock()
	if !s.readinessAt.IsZero() && time.Since(s.readinessAt) < s.cfg.ReadinessCacheTTL {
		return s.readinessErr
	}

	ctx, cancel := context.WithTimeout(parent, s.cfg.ReadinessTimeout)
	defer cancel()
	err := s.checker.Check(ctx)
	s.readinessAt = time.Now()
	s.readinessErr = err
	if err != nil {
		s.metrics.ReadinessFailed()
		s.logger.Warn("dependency readiness check failed", "error", err)
	}
	return err
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

	allowedScopes := map[string]bool{
		domain.ScopeKnowledgeRead:   true,
		domain.ScopeCustomerRead:    true,
		domain.ScopeResourceRead:    true,
		domain.ScopeTransactionRead: true,
		domain.ScopeCaseCreate:      true,
	}
	seen := make(map[string]struct{})
	var scopes []string
	for _, raw := range request.Principal.Scopes {
		scope := strings.TrimSpace(raw)
		if !allowedScopes[scope] {
			return errors.New("invalid_scope")
		}
		if !request.Principal.Authenticated && (scope == domain.ScopeCustomerRead || scope == domain.ScopeResourceRead || scope == domain.ScopeTransactionRead) {
			return errors.New("anonymous_scope_not_allowed")
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	if len(scopes) == 0 {
		return errors.New("scopes_required")
	}
	sort.Strings(scopes)
	request.Principal.Scopes = scopes
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
