package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/honghu-ai/llm-infra-gateway/internal/config"
	gatewayhttp "github.com/honghu-ai/llm-infra-gateway/internal/gateway/http"
	"github.com/honghu-ai/llm-infra-gateway/internal/observability"
	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
	"github.com/honghu-ai/llm-infra-gateway/internal/router"
)

func main() {
	// 程序入口：`go run ./cmd/gateway` 会从这里开始执行。
	// 这一层只负责组装配置、日志、Provider、Router、HTTP Server。
	// 真正处理 HTTP 请求的代码在 internal/gateway/http。
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	// Provider 是“出口”：网关最终会把请求发给本地 vLLM、DeepSeek、
	// OpenAI-compatible 服务，或者开发环境用的 fake provider。
	providers := buildProviders(cfg)
	metrics := observability.NewMetrics()
	// Router 是“中间决策层”：它决定这次请求走哪个 Provider。
	ruleRouter := router.NewRuleRouter(providers)

	srv := &http.Server{
		Addr: cfg.Addr,
		// HTTP 入口：客户端请求先进 gatewayhttp.NewServer 返回的 Handler。
		Handler:           gatewayhttp.NewServer(cfg, ruleRouter, metrics, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("gateway listening", "addr", cfg.Addr, "providers", providerNames(providers))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("gateway stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("gateway shutdown complete")
}

func buildProviders(cfg config.Config) []provider.Provider {
	providers := make([]provider.Provider, 0, 4)

	// LOCAL_VLLM_URL 配置后，请求可以从 Gateway 出口转发到本地 vLLM。
	if cfg.LocalVLLMURL != "" {
		providers = append(providers, provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{
			Name:      "local_vllm",
			BaseURL:   cfg.LocalVLLMURL,
			ProbePath: "/health",
			Timeout:   cfg.ProviderTimeout,
		}))
	}
	// OPENAI_COMPATIBLE_URL 用于接任意兼容 OpenAI API 的云端服务。
	if cfg.OpenAICompatibleURL != "" {
		providers = append(providers, provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{
			Name:    "openai_compatible",
			BaseURL: cfg.OpenAICompatibleURL,
			APIKey:  cfg.OpenAICompatibleAPIKey,
			Timeout: cfg.ProviderTimeout,
		}))
	}
	// DEEPSEEK_API_KEY 存在时启用 DeepSeek 作为云端 Provider。
	if cfg.DeepSeekAPIKey != "" {
		providers = append(providers, provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{
			Name:    "deepseek",
			BaseURL: cfg.DeepSeekURL,
			APIKey:  cfg.DeepSeekAPIKey,
			Timeout: cfg.ProviderTimeout,
		}))
	}
	// 本地开发没有真实模型时，用 fake provider 保证项目能启动、能测试。
	if len(providers) == 0 || cfg.EnableFakeProvider {
		providers = append(providers, provider.NewFakeProvider("fake_local"))
	}

	return providers
}

func providerNames(providers []provider.Provider) []string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name())
	}
	return names
}
