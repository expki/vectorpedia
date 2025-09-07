package ai

import "context"

type TokenizeRequest struct {
	Content string `json:"content"`
	Model   string `json:"model,omitempty"`
}

type TokenizeResponse struct {
	Tokens []int `json:"tokens"`
}

type DetokenizeRequest struct {
	Tokens []int  `json:"tokens"`
	Model  string `json:"model,omitempty"`
}

type DetokenizeResponse struct {
	Content string `json:"content"`
}

// TokenizeChat tokenizes content for chat queries
func (c *client) TokenizeChat(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error) {
	resp, err := c.doRequestWithQueryType(ctx, "/tokenize", req, &TokenizeResponse{}, "chat")
	if err != nil {
		return nil, err
	}
	return resp.(*TokenizeResponse), nil
}

// TokenizeEmbed tokenizes content for embedding queries
func (c *client) TokenizeEmbed(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error) {
	resp, err := c.doRequestWithQueryType(ctx, "/tokenize", req, &TokenizeResponse{}, "embed")
	if err != nil {
		return nil, err
	}
	return resp.(*TokenizeResponse), nil
}

// TokenizeRerank tokenizes content for rerank queries
func (c *client) TokenizeRerank(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error) {
	resp, err := c.doRequestWithQueryType(ctx, "/tokenize", req, &TokenizeResponse{}, "rerank")
	if err != nil {
		return nil, err
	}
	return resp.(*TokenizeResponse), nil
}

// DetokenizeChat detokenizes tokens for chat queries
func (c *client) DetokenizeChat(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error) {
	resp, err := c.doRequestWithQueryType(ctx, "/detokenize", req, &DetokenizeResponse{}, "chat")
	if err != nil {
		return nil, err
	}
	return resp.(*DetokenizeResponse), nil
}

// DetokenizeEmbed detokenizes tokens for embedding queries
func (c *client) DetokenizeEmbed(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error) {
	resp, err := c.doRequestWithQueryType(ctx, "/detokenize", req, &DetokenizeResponse{}, "embed")
	if err != nil {
		return nil, err
	}
	return resp.(*DetokenizeResponse), nil
}

// DetokenizeRerank detokenizes tokens for rerank queries
func (c *client) DetokenizeRerank(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error) {
	resp, err := c.doRequestWithQueryType(ctx, "/detokenize", req, &DetokenizeResponse{}, "rerank")
	if err != nil {
		return nil, err
	}
	return resp.(*DetokenizeResponse), nil
}
