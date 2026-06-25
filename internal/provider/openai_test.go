package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestHealthIsCached 验证健康探测结果会被缓存：在 TTL 内多次调用 Health
// 只会触发一次真实的 HTTP 探测，从而不会拖慢请求热路径上的路由决策。
func TestHealthIsCached(t *testing.T) {
	var probes atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Name:      "local_vllm",
		BaseURL:   srv.URL,
		ProbePath: "/health",
		HealthTTL: time.Minute,
	})

	for i := 0; i < 5; i++ {
		if !p.Health(context.Background()).Healthy {
			t.Fatalf("expected healthy provider on call %d", i)
		}
	}

	if got := probes.Load(); got != 1 {
		t.Fatalf("expected exactly 1 probe within TTL, got %d", got)
	}
}

// TestHealthRefreshesAfterTTL 验证缓存过期后会重新探测。
func TestHealthRefreshesAfterTTL(t *testing.T) {
	var probes atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewOpenAICompatibleProvider(OpenAICompatibleConfig{
		Name:      "local_vllm",
		BaseURL:   srv.URL,
		ProbePath: "/health",
		HealthTTL: time.Millisecond,
	})

	p.Health(context.Background())
	time.Sleep(2 * time.Millisecond)
	p.Health(context.Background())

	if got := probes.Load(); got < 2 {
		t.Fatalf("expected re-probe after TTL expiry, got %d probes", got)
	}
}
