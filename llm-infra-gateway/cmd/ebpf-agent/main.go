package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/honghu-ai/llm-infra-gateway/internal/observability"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	addr := os.Getenv("EBPF_AGENT_ADDR")
	if addr == "" {
		addr = ":18081"
	}

	metrics := observability.NewMetrics()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","mode":"stub"}` + "\n"))
	})
	mux.Handle("GET /metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))

	slog.Info("ebpf agent stub listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("ebpf agent stopped", "error", err)
		os.Exit(1)
	}
}
