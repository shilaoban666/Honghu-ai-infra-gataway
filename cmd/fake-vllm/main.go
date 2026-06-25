package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/honghu-ai/llm-infra-gateway/internal/config"
	gatewayhttp "github.com/honghu-ai/llm-infra-gateway/internal/gateway/http"
	"github.com/honghu-ai/llm-infra-gateway/internal/observability"
	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
	"github.com/honghu-ai/llm-infra-gateway/internal/router"
)

func main() {
	cfg := config.Config{
		Addr:         ":8000",
		DefaultModel: "fake-vllm-model",
	}
	if value := os.Getenv("FAKE_VLLM_ADDR"); value != "" {
		cfg.Addr = value
	}
	p := provider.NewFakeProvider("fake_vllm")
	r := router.NewRuleRouter([]provider.Provider{p})
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	log.Printf("fake vLLM listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, gatewayhttp.NewServer(cfg, r, observability.NewMetrics(), logger)); err != nil {
		log.Fatal(err)
	}
}
