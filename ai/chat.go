package ai

import (
	"context"
)

// ChatRequest represents a chat completion request
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float32       `json:"temperature,omitempty"`
}

// ChatMessage represents a chat message
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Message is an alias for ChatMessage for backward compatibility
type Message = ChatMessage

// ChatResponse represents a chat completion response
type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage,omitempty"`
}

// Choice represents a response choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Chat sends a chat completion request
func (bc *backendClient) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	var resp ChatResponse
	if err := bc.doRequest(ctx, "/chat", "/v1/chat/completions", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
