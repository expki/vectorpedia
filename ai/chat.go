package ai

import "context"

type ChatRequest struct {
	Model       string        `json:"model,omitempty"`
	Messages    []ChatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	TopP        float32       `json:"top_p,omitempty"`
	TopK        int           `json:"top_k,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	FinishReason string      `json:"finish_reason,omitempty"`
}

func (c *client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	resp, err := c.doRequest(ctx, "/v1/chat/completions", req, &ChatResponse{})
	if err != nil {
		return nil, err
	}
	return resp.(*ChatResponse), nil
}
