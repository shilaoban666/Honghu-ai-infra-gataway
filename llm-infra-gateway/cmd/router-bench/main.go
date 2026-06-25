package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
	"github.com/honghu-ai/llm-infra-gateway/internal/router"
)

func main() {
	prompts := []string{
		"summarize a short support ticket",
		"write Golang code for a retrying HTTP client",
		"classify this message into billing or support",
	}
	r := router.NewRuleRouter([]provider.Provider{
		provider.NewFakeProvider("local_vllm"),
		provider.NewFakeProvider("deepseek"),
	})

	results := make([]router.RouteDecision, 0, len(prompts))
	for _, prompt := range prompts {
		decision, err := r.Pick(context.Background(), router.RouteRequest{Request: provider.ChatRequest{
			Messages: []provider.ChatMessage{{Role: "user", Content: prompt}},
		}})
		if err != nil {
			fmt.Fprintf(os.Stderr, "route failed: %v\n", err)
			os.Exit(1)
		}
		results = append(results, router.RouteDecision{
			ProviderName: decision.ProviderName,
			Reason:       decision.Reason,
			Task:         decision.Task,
			PromptTokens: decision.PromptTokens,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		fmt.Fprintf(os.Stderr, "encode results: %v\n", err)
		os.Exit(1)
	}
}
