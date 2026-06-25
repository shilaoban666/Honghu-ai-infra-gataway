package gatewayhttp

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/honghu-ai/llm-infra-gateway/internal/config"
	"github.com/honghu-ai/llm-infra-gateway/internal/observability"
	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
	"github.com/honghu-ai/llm-infra-gateway/internal/router"
)

func TestChatCompletions(t *testing.T) {
	handler := testServer(t, nil)
	body := `{"model":"demo","messages":[{"role":"user","content":"summarize hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Honghu fake provider response") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
}

func TestStreamingChatCompletions(t *testing.T) {
	handler := testServer(t, nil)
	body := `{"model":"demo","stream":true,"messages":[{"role":"user","content":"summarize hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "data:") || !strings.Contains(rec.Body.String(), "[DONE]") {
		t.Fatalf("expected SSE response, got: %s", rec.Body.String())
	}
}

func TestAuthMiddleware(t *testing.T) {
	handler := testServer(t, []string{"secret"})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("expected 200, got %d: %s", rec.Code, body)
	}
}

func testServer(t *testing.T, keys []string) http.Handler {
	t.Helper()
	cfg := config.Config{DefaultModel: "demo", AllowedAPIKeys: keys}
	p := provider.NewFakeProvider("fake_local")
	r := router.NewRuleRouter([]provider.Provider{p})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(cfg, r, observability.NewMetrics(), logger)
}
