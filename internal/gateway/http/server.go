package gatewayhttp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/honghu-ai/llm-infra-gateway/internal/config"
	"github.com/honghu-ai/llm-infra-gateway/internal/observability"
	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
	"github.com/honghu-ai/llm-infra-gateway/internal/router"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	cfg     config.Config
	router  router.Router
	metrics *observability.Metrics
	logger  *slog.Logger
	mux     *http.ServeMux
}

// NewServer 创建整个 HTTP 层。
//
// 从请求视角看，这里是网关的“入口总开关”：
// client -> net/http -> NewServer 返回的 Handler -> routes() 里注册的具体接口。
func NewServer(cfg config.Config, route router.Router, metrics *observability.Metrics, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		cfg:     cfg,
		router:  route,
		metrics: metrics,
		logger:  logger,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	// 浏览器打开根路径时展示接口说明；真实业务调用主要走 /v1/chat/completions。
	s.mux.HandleFunc("GET /", s.handleIndex)
	// 运维入口：存活检查。只说明进程还活着。
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	// vLLM 兼容别名：真实 vLLM 暴露 /health，这里同样提供，方便 fake-vllm
	// 作为可替换的本地后端，并让 Provider 健康探测落在明确的接口上。
	s.mux.HandleFunc("GET /health", s.handleHealthz)
	// 运维入口：就绪检查。会检查下游 Provider 是否至少有一个可用。
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)
	// 指标出口：Prometheus 从这里抓 Gateway 指标。
	s.mux.Handle("GET /metrics", promhttp.HandlerFor(s.metrics.Registry, promhttp.HandlerOpts{}))
	// OpenAI-compatible 入口：模型列表。
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	// OpenAI-compatible 入口：聊天补全，支持 stream=false 和 stream=true。
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	// 兼容旧 completions API，内部会转换成 ChatRequest。
	s.mux.HandleFunc("POST /v1/completions", s.handleCompletions)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	// 中间件包住所有接口，统一做鉴权、request_id/trace_id、耗时和请求数指标。
	return requestIDMiddleware(authMiddleware(s.cfg.AllowedAPIKeys, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		route := routeName(r)
		providerName := ""
		status := http.StatusOK

		s.metrics.InFlightRequests.Inc()
		defer s.metrics.InFlightRequests.Dec()

		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		status = rw.status
		if value := r.Context().Value(providerContextKey{}); value != nil {
			providerName, _ = value.(string)
		}
		s.metrics.RequestsTotal.WithLabelValues(route, r.Method, fmt.Sprint(status), providerName).Inc()
		s.metrics.RequestDuration.WithLabelValues(route, providerName).Observe(time.Since(start).Seconds())
	})))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name": "Honghu LLM Infra Gateway",
		"entrypoints": []string{
			"GET /healthz",
			"GET /readyz",
			"GET /metrics",
			"GET /v1/models",
			"POST /v1/chat/completions",
		},
		"note": "This is an API gateway. Use /v1/chat/completions for OpenAI-compatible requests.",
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	providers := s.router.Providers()
	ready := false
	checks := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		health := p.Health(ctx)
		if health.Healthy {
			ready = true
		}
		checks = append(checks, map[string]any{
			"name":    p.Name(),
			"healthy": health.Healthy,
			"reason":  health.Reason,
		})
	}
	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{"ready": ready, "providers": checks})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models := []map[string]any{
		{"id": s.defaultModel(), "object": "model", "owned_by": "honghu"},
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": models})
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	var req provider.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.Prompt != nil && len(req.Messages) == 0 {
		req.Messages = []provider.ChatMessage{{Role: "user", Content: provider.ContentText(req.Prompt)}}
	}
	s.serveChat(w, r, req)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req provider.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	s.serveChat(w, r, req)
}

func (s *Server) serveChat(w http.ResponseWriter, r *http.Request, req provider.ChatRequest) {
	// 请求主流程：
	// 1. 补默认模型并校验 messages。
	// 2. 调 Router.Pick 选择 Provider。
	// 3. 根据 stream 字段进入普通响应或 SSE 流式响应。
	if req.Model == "" {
		req.Model = s.defaultModel()
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages_required", "messages must not be empty")
		return
	}

	decision, err := s.router.Pick(r.Context(), router.RouteRequest{Request: req})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "route_failed", err.Error())
		return
	}

	setProviderName(r, decision.ProviderName)
	s.metrics.RouteDecisions.WithLabelValues(decision.ProviderName, decision.Task, decision.Reason).Inc()
	s.logger.Info("route decision",
		"request_id", requestIDFrom(r.Context()),
		"trace_id", traceIDFrom(r.Context()),
		"provider", decision.ProviderName,
		"fallback", providerName(decision.Fallback),
		"task", decision.Task,
		"prompt_tokens", decision.PromptTokens,
		"reason", decision.Reason,
	)

	if req.Stream {
		s.streamChat(w, r, req, decision)
		return
	}
	s.chat(w, r, req, decision)
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request, req provider.ChatRequest, decision router.RouteDecision) {
	// 非流式出口：这里真正调用 Provider.Chat，把请求转发给下游模型服务。
	resp, err := decision.Provider.Chat(r.Context(), req)
	if err != nil && decision.Fallback != nil {
		s.metrics.ProviderErrors.WithLabelValues(decision.ProviderName).Inc()
		s.logger.Warn("primary provider failed, trying fallback", "provider", decision.ProviderName, "fallback", decision.Fallback.Name(), "error", err)
		resp, err = decision.Fallback.Chat(r.Context(), req)
		setProviderName(r, decision.Fallback.Name())
	}
	if err != nil {
		s.metrics.ProviderErrors.WithLabelValues(decision.ProviderName).Inc()
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}

	providerName := providerNameFrom(r, decision.ProviderName)
	s.metrics.TokensTotal.WithLabelValues(providerName, "prompt").Add(float64(resp.Usage.PromptTokens))
	s.metrics.TokensTotal.WithLabelValues(providerName, "completion").Add(float64(resp.Usage.CompletionTokens))
	s.metrics.TokensTotal.WithLabelValues(providerName, "total").Add(float64(resp.Usage.TotalTokens))

	w.Header().Set("X-Request-ID", requestIDFrom(r.Context()))
	w.Header().Set("X-Trace-ID", traceIDFrom(r.Context()))
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) streamChat(w http.ResponseWriter, r *http.Request, req provider.ChatRequest, decision router.RouteDecision) {
	// 流式出口：Provider.StreamChat 返回一个 channel，每个事件会被写成 SSE data 行。
	// 注意：只有第一个 token 之前允许 fallback，已经开始输出后就不能切换 Provider。
	start := time.Now()
	events, err := decision.Provider.StreamChat(r.Context(), req)
	if err != nil && decision.Fallback != nil {
		s.metrics.ProviderErrors.WithLabelValues(decision.ProviderName).Inc()
		s.logger.Warn("primary stream provider failed before first chunk, trying fallback", "provider", decision.ProviderName, "fallback", decision.Fallback.Name(), "error", err)
		events, err = decision.Fallback.StreamChat(r.Context(), req)
		setProviderName(r, decision.Fallback.Name())
	}
	if err != nil {
		s.metrics.ProviderErrors.WithLabelValues(decision.ProviderName).Inc()
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}

	firstEvent, ok := <-events
	if !ok {
		writeError(w, http.StatusBadGateway, "provider_error", "provider closed stream without chunks")
		return
	}
	if firstEvent.Err != nil {
		if decision.Fallback != nil {
			s.metrics.ProviderErrors.WithLabelValues(decision.ProviderName).Inc()
			fallback := decision.Fallback
			s.streamChat(w, r, req, router.RouteDecision{
				Provider:     fallback,
				ProviderName: fallback.Name(),
				Reason:       "stream_error_before_first_chunk_fallback",
				Task:         decision.Task,
				PromptTokens: decision.PromptTokens,
			})
			return
		}
		s.metrics.ProviderErrors.WithLabelValues(decision.ProviderName).Inc()
		writeError(w, http.StatusBadGateway, "provider_error", firstEvent.Err.Error())
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "response writer does not support streaming")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Request-ID", requestIDFrom(r.Context()))
	w.Header().Set("X-Trace-ID", traceIDFrom(r.Context()))
	w.WriteHeader(http.StatusOK)

	var completionText strings.Builder
	s.metrics.FirstToken.WithLabelValues(providerNameFrom(r, decision.ProviderName)).Observe(time.Since(start).Seconds())
	if !s.writeStreamEvent(w, flusher, firstEvent, &completionText) {
		return
	}

	for event := range events {
		if event.Err != nil {
			s.logger.Warn("stream provider error", "provider", providerNameFrom(r, decision.ProviderName), "error", event.Err)
			return
		}
		if !s.writeStreamEvent(w, flusher, event, &completionText) {
			return
		}
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()

	providerName := providerNameFrom(r, decision.ProviderName)
	s.metrics.TokensTotal.WithLabelValues(providerName, "prompt").Add(float64(decision.PromptTokens))
	s.metrics.TokensTotal.WithLabelValues(providerName, "completion").Add(float64(provider.EstimateTokens(completionText.String())))
}

func (s *Server) writeStreamEvent(w http.ResponseWriter, flusher http.Flusher, event provider.StreamEvent, completionText *strings.Builder) bool {
	for _, choice := range event.Chunk.Choices {
		completionText.WriteString(provider.ContentText(choice.Delta.Content))
	}
	payload, err := json.Marshal(event.Chunk)
	if err != nil {
		s.logger.Warn("failed to encode stream chunk", "error", err)
		return false
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
	return true
}

func (s *Server) defaultModel() string {
	if s.cfg.DefaultModel == "" {
		return "honghu-fake-llm"
	}
	return s.cfg.DefaultModel
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

func providerName(p provider.Provider) string {
	if p == nil {
		return ""
	}
	return p.Name()
}

func randomID(prefix string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func routeName(r *http.Request) string {
	switch {
	case strings.HasPrefix(r.URL.Path, "/v1/chat/completions"):
		return "/v1/chat/completions"
	case strings.HasPrefix(r.URL.Path, "/v1/completions"):
		return "/v1/completions"
	default:
		return r.URL.Path
	}
}

func providerNameFrom(r *http.Request, fallback string) string {
	value := r.Context().Value(providerContextKey{})
	if name, ok := value.(string); ok && name != "" {
		return name
	}
	return fallback
}

func setProviderName(r *http.Request, name string) {
	ctx := context.WithValue(r.Context(), providerContextKey{}, name)
	*r = *r.WithContext(ctx)
}

func requestIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}

func traceIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(traceIDContextKey{}).(string)
	return value
}

type requestIDContextKey struct{}
type traceIDContextKey struct{}
type providerContextKey struct{}

var errUnauthorized = errors.New("missing or invalid API key")
