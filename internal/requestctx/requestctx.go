package requestctx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

type requestIDKey struct{}

// WithRequestID stores a trusted server-generated request ID in the context.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestID returns the trusted request ID associated with the current run.
func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// Middleware establishes the request ID before the transport's own middleware
// runs. It also writes the normalized value back to the request header so every
// layer and downstream log uses the same identifier.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = newID()
		}
		request.Header.Set("X-Request-ID", requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, request.WithContext(WithRequestID(request.Context(), requestID)))
	})
}

func newID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "req_fallback"
	}
	return "req_" + hex.EncodeToString(raw)
}
