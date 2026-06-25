package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Provider interface {
	Name() string
	// Chat 是非流式出口：Gateway 选中 Provider 后，会调用这里拿完整响应。
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	// StreamChat 是流式出口：返回的 channel 会被 HTTP 层逐块写成 SSE。
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	// Health 给 Router 和 /readyz 使用，用来判断这个出口是否可用。
	Health(ctx context.Context) ProviderHealth
}

type ProviderHealth struct {
	Healthy bool
	Reason  string
	Checked time.Time
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages,omitempty"`
	Prompt      any           `json:"prompt,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage,omitempty"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionChunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []StreamChoice `json:"choices"`
}

type StreamChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type StreamEvent struct {
	Chunk ChatCompletionChunk
	Err   error
}

func MessageText(messages []ChatMessage) string {
	var builder strings.Builder
	for _, message := range messages {
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(ContentText(message.Content))
	}
	return builder.String()
}

func ContentText(content any) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case []byte:
		return string(value)
	case json.RawMessage:
		var decoded any
		if err := json.Unmarshal(value, &decoded); err == nil {
			return ContentText(decoded)
		}
		return string(value)
	case []any:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			parts = append(parts, ContentText(part))
		}
		return strings.Join(parts, " ")
	case map[string]any:
		if text, ok := value["text"]; ok {
			return ContentText(text)
		}
		if text, ok := value["content"]; ok {
			return ContentText(text)
		}
		return fmt.Sprint(value)
	default:
		return fmt.Sprint(value)
	}
}
