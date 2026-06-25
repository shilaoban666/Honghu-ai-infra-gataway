package router

import (
	"context"
	"errors"
	"strings"

	"github.com/honghu-ai/llm-infra-gateway/internal/provider"
)

type Router interface {
	// Pick 是路由入口：HTTP 层把 ChatRequest 交给它，它返回应该走哪个 Provider。
	Pick(ctx context.Context, req RouteRequest) (RouteDecision, error)
	Providers() []provider.Provider
}

type RouteRequest struct {
	Request provider.ChatRequest
	Tenant  string
}

type RouteDecision struct {
	Provider     provider.Provider
	Fallback     provider.Provider
	ProviderName string
	Reason       string
	Task         string
	PromptTokens int
}

type RuleRouter struct {
	providers []provider.Provider
}

func NewRuleRouter(providers []provider.Provider) *RuleRouter {
	return &RuleRouter{providers: append([]provider.Provider(nil), providers...)}
}

func (r *RuleRouter) Providers() []provider.Provider {
	return append([]provider.Provider(nil), r.providers...)
}

func (r *RuleRouter) Pick(ctx context.Context, req RouteRequest) (RouteDecision, error) {
	if len(r.providers) == 0 {
		return RouteDecision{}, errors.New("no providers configured")
	}

	// 路由判断需要先抽取文本、粗略估算 token、识别任务类型。
	text := provider.MessageText(req.Request.Messages)
	if text == "" {
		text = provider.ContentText(req.Request.Prompt)
	}
	promptTokens := provider.EstimateTokens(text)
	task := classifyTask(text)

	local := r.firstHealthy(ctx, isLocalProvider)
	cloud := r.firstHealthy(ctx, isCloudProvider)
	any := r.firstHealthy(ctx, func(string) bool { return true })

	// 规则 1：本地不可用时，优先走云端 Provider。
	if local == nil && cloud != nil {
		return decision(cloud, nil, "local_unavailable_route_cloud", task, promptTokens), nil
	}
	if local == nil && cloud == nil && any != nil {
		return decision(any, nil, "fallback_only_provider", task, promptTokens), nil
	}

	// 规则 2：短文本、总结、分类、抽取等简单任务优先走本地，降低成本。
	if local != nil && simpleTask(task) && promptTokens <= 512 {
		return decision(local, cloud, "simple_short_task_route_local", task, promptTokens), nil
	}
	// 规则 3：代码、推理等复杂任务优先走云端，质量更稳。
	if cloud != nil && complexTask(task) {
		return decision(cloud, local, "complex_task_route_cloud", task, promptTokens), nil
	}
	// 默认策略：local-first。
	if local != nil {
		return decision(local, cloud, "default_local_first", task, promptTokens), nil
	}
	if cloud != nil {
		return decision(cloud, nil, "default_cloud", task, promptTokens), nil
	}
	return RouteDecision{}, errors.New("no healthy providers")
}

func (r *RuleRouter) firstHealthy(ctx context.Context, match func(string) bool) provider.Provider {
	for _, p := range r.providers {
		if !match(p.Name()) {
			continue
		}
		if p.Health(ctx).Healthy {
			return p
		}
	}
	return nil
}

func decision(p provider.Provider, fallback provider.Provider, reason string, task string, promptTokens int) RouteDecision {
	return RouteDecision{
		Provider:     p,
		Fallback:     fallback,
		ProviderName: p.Name(),
		Reason:       reason,
		Task:         task,
		PromptTokens: promptTokens,
	}
}

func classifyTask(text string) string {
	normalized := strings.ToLower(text)
	switch {
	case strings.Contains(normalized, "summarize") || strings.Contains(normalized, "summary") || strings.Contains(normalized, "总结"):
		return "summary"
	case strings.Contains(normalized, "classify") || strings.Contains(normalized, "分类"):
		return "classify"
	case strings.Contains(normalized, "rewrite") || strings.Contains(normalized, "改写"):
		return "rewrite"
	case strings.Contains(normalized, "extract") || strings.Contains(normalized, "抽取"):
		return "extract"
	case strings.Contains(normalized, "code") || strings.Contains(normalized, "golang") || strings.Contains(normalized, "python") || strings.Contains(normalized, "代码"):
		return "code"
	case strings.Contains(normalized, "reason") || strings.Contains(normalized, "推理") || strings.Contains(normalized, "证明"):
		return "reasoning"
	default:
		return "general"
	}
}

func simpleTask(task string) bool {
	switch task {
	case "summary", "classify", "rewrite", "extract", "general":
		return true
	default:
		return false
	}
}

func complexTask(task string) bool {
	switch task {
	case "code", "reasoning":
		return true
	default:
		return false
	}
}

func isLocalProvider(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "local") || strings.Contains(name, "vllm") || strings.Contains(name, "fake")
}

func isCloudProvider(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "openai") || strings.Contains(name, "deepseek") || strings.Contains(name, "cloud")
}
