package provider

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type FakeProvider struct {
	name    string
	healthy atomic.Bool
}

func NewFakeProvider(name string) *FakeProvider {
	p := &FakeProvider{name: name}
	p.healthy.Store(true)
	return p
}

func (p *FakeProvider) Name() string {
	return p.name
}

func (p *FakeProvider) SetHealthy(healthy bool) {
	p.healthy.Store(healthy)
}

func (p *FakeProvider) Health(context.Context) ProviderHealth {
	healthy := p.healthy.Load()
	reason := "ok"
	if !healthy {
		reason = "forced_unhealthy"
	}
	return ProviderHealth{Healthy: healthy, Reason: reason, Checked: time.Now()}
}

func (p *FakeProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content := fakeAnswer(req)
	promptTokens := EstimateTokens(MessageText(req.Messages))
	completionTokens := EstimateTokens(content)

	return &ChatResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelOrDefault(req.Model),
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

func (p *FakeProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	events := make(chan StreamEvent)
	go func() {
		defer close(events)
		answer := fakeAnswer(req)
		chunks := strings.Fields(answer)
		if len(chunks) == 0 {
			chunks = []string{answer}
		}

		id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
		model := modelOrDefault(req.Model)
		for i, word := range chunks {
			select {
			case <-ctx.Done():
				events <- StreamEvent{Err: ctx.Err()}
				return
			case <-time.After(20 * time.Millisecond):
			}

			delta := word
			if i < len(chunks)-1 {
				delta += " "
			}
			role := ""
			if i == 0 {
				role = "assistant"
			}
			events <- StreamEvent{Chunk: ChatCompletionChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   model,
				Choices: []StreamChoice{
					{
						Index: 0,
						Delta: ChatMessage{
							Role:    role,
							Content: delta,
						},
						FinishReason: nil,
					},
				},
			}}
		}

		finish := "stop"
		events <- StreamEvent{Chunk: ChatCompletionChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []StreamChoice{
				{
					Index:        0,
					Delta:        ChatMessage{},
					FinishReason: &finish,
				},
			},
		}}
	}()
	return events, nil
}

func fakeAnswer(req ChatRequest) string {
	text := strings.TrimSpace(MessageText(req.Messages))
	if text == "" {
		text = strings.TrimSpace(ContentText(req.Prompt))
	}
	if text == "" {
		text = "empty prompt"
	}
	return "Honghu fake provider response: " + text
}

func modelOrDefault(model string) string {
	if strings.TrimSpace(model) == "" {
		return "honghu-fake-llm"
	}
	return model
}

func EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	chars := len([]rune(text))
	estimate := chars / 4
	if estimate < 1 {
		estimate = 1
	}
	return estimate
}
