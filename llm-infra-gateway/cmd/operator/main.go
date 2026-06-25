package main

import (
	"log/slog"
	"net/http"
	"os"
)

func main() {
	addr := os.Getenv("OPERATOR_ADDR")
	if addr == "" {
		addr = ":18082"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","component":"llmengine-operator","mode":"stub"}` + "\n"))
	})

	slog.Info("operator stub listening", "addr", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("operator stopped", "error", err)
		os.Exit(1)
	}
}
