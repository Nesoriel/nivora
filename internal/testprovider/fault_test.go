package testprovider

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFaultInjectorFailsConfiguredCountThenRecovers(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := WithFaults(next, FaultConfig{StatusCode: http.StatusTooManyRequests, Count: 2})
	for index, expected := range []int{http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusOK} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
		if response.Code != expected {
			t.Fatalf("request %d: expected %d, got %d", index+1, expected, response.Code)
		}
	}
}
