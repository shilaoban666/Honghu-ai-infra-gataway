package router

import (
	"context"
	"testing"

	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
)

func TestRuleRouterSimpleTaskRoutesLocal(t *testing.T) {
	local := provider.NewFakeProvider("local_vllm")
	cloud := provider.NewFakeProvider("deepseek")
	r := NewRuleRouter([]provider.Provider{local, cloud})

	decision, err := r.Pick(context.Background(), RouteRequest{Request: provider.ChatRequest{
		Messages: []provider.ChatMessage{{Role: "user", Content: "summarize this short text"}},
	}})
	if err != nil {
		t.Fatalf("Pick returned error: %v", err)
	}
	if decision.ProviderName != "local_vllm" {
		t.Fatalf("expected local_vllm, got %s", decision.ProviderName)
	}
	if decision.Task != "summary" {
		t.Fatalf("expected summary task, got %s", decision.Task)
	}
}

func TestRuleRouterComplexTaskRoutesCloud(t *testing.T) {
	local := provider.NewFakeProvider("local_vllm")
	cloud := provider.NewFakeProvider("deepseek")
	r := NewRuleRouter([]provider.Provider{local, cloud})

	decision, err := r.Pick(context.Background(), RouteRequest{Request: provider.ChatRequest{
		Messages: []provider.ChatMessage{{Role: "user", Content: "write Golang code for a retrying HTTP client"}},
	}})
	if err != nil {
		t.Fatalf("Pick returned error: %v", err)
	}
	if decision.ProviderName != "deepseek" {
		t.Fatalf("expected deepseek, got %s", decision.ProviderName)
	}
}

func TestRuleRouterFallsBackWhenLocalUnhealthy(t *testing.T) {
	local := provider.NewFakeProvider("local_vllm")
	local.SetHealthy(false)
	cloud := provider.NewFakeProvider("deepseek")
	r := NewRuleRouter([]provider.Provider{local, cloud})

	decision, err := r.Pick(context.Background(), RouteRequest{Request: provider.ChatRequest{
		Messages: []provider.ChatMessage{{Role: "user", Content: "summarize this short text"}},
	}})
	if err != nil {
		t.Fatalf("Pick returned error: %v", err)
	}
	if decision.ProviderName != "deepseek" {
		t.Fatalf("expected deepseek, got %s", decision.ProviderName)
	}
}
