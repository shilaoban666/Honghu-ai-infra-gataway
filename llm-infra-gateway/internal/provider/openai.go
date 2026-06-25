package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// defaultHealthTTL 决定健康探测结果缓存多久。Router 在每次请求时都会问
	// Provider 是否健康，如果不缓存就会在请求热路径上发起阻塞式 HTTP 探测。
	defaultHealthTTL = 5 * time.Second
	// defaultProbeTimeout 给健康探测一个独立的短超时，避免复用 60s 的请求超时
	// 把一次路由决策拖慢到数秒。
	defaultProbeTimeout = 2 * time.Second
)

type OpenAICompatibleConfig struct {
	Name         string
	BaseURL      string
	APIKey       string
	ProbePath    string
	Timeout      time.Duration
	HealthTTL    time.Duration
	ProbeTimeout time.Duration
}

type OpenAICompatibleProvider struct {
	name         string
	baseURL      string
	apiKey       string
	probePath    string
	client       *http.Client
	healthTTL    time.Duration
	probeTimeout time.Duration

	mu       sync.Mutex
	cached   ProviderHealth
	cachedAt time.Time
}

func NewOpenAICompatibleProvider(cfg OpenAICompatibleConfig) *OpenAICompatibleProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	healthTTL := cfg.HealthTTL
	if healthTTL <= 0 {
		healthTTL = defaultHealthTTL
	}
	probeTimeout := cfg.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = defaultProbeTimeout
	}
	return &OpenAICompatibleProvider{
		name:         cfg.Name,
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:       cfg.APIKey,
		probePath:    cfg.ProbePath,
		client:       &http.Client{Timeout: timeout},
		healthTTL:    healthTTL,
		probeTimeout: probeTimeout,
	}
}

func (p *OpenAICompatibleProvider) Name() string {
	return p.name
}

func (p *OpenAICompatibleProvider) Health(ctx context.Context) ProviderHealth {
	if p.baseURL == "" {
		return ProviderHealth{Healthy: false, Reason: "base_url_missing", Checked: time.Now()}
	}
	if p.probePath == "" {
		return ProviderHealth{Healthy: true, Reason: "probe_disabled", Checked: time.Now()}
	}

	// 命中缓存就直接返回，避免在请求热路径上每次都发起阻塞探测。
	p.mu.Lock()
	if !p.cachedAt.IsZero() && time.Since(p.cachedAt) < p.healthTTL {
		cached := p.cached
		p.mu.Unlock()
		return cached
	}
	p.mu.Unlock()

	health := p.probe(ctx)

	p.mu.Lock()
	p.cached = health
	p.cachedAt = time.Now()
	p.mu.Unlock()
	return health
}

func (p *OpenAICompatibleProvider) probe(ctx context.Context) ProviderHealth {
	ctx, cancel := context.WithTimeout(ctx, p.probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+p.probePath, nil)
	if err != nil {
		return ProviderHealth{Healthy: false, Reason: err.Error(), Checked: time.Now()}
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return ProviderHealth{Healthy: false, Reason: err.Error(), Checked: time.Now()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProviderHealth{Healthy: false, Reason: resp.Status, Checked: time.Now()}
	}
	return ProviderHealth{Healthy: true, Reason: "ok", Checked: time.Now()}
}

func (p *OpenAICompatibleProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	req.Stream = false
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	p.decorate(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, providerHTTPError(resp)
	}
	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *OpenAICompatibleProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	req.Stream = true
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	p.decorate(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, providerHTTPError(resp)
	}

	events := make(chan StreamEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				return
			}
			var chunk ChatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				events <- StreamEvent{Err: err}
				return
			}
			select {
			case <-ctx.Done():
				events <- StreamEvent{Err: ctx.Err()}
				return
			case events <- StreamEvent{Chunk: chunk}:
			}
		}
		if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
			events <- StreamEvent{Err: err}
		}
	}()
	return events, nil
}

func (p *OpenAICompatibleProvider) decorate(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

func providerHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	bodyText := strings.TrimSpace(string(body))
	if bodyText == "" {
		bodyText = resp.Status
	}
	return fmt.Errorf("provider returned %s: %s", resp.Status, bodyText)
}
