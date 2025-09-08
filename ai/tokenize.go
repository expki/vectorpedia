package ai

import (
	"context"
)

// TokenizeRequest represents a tokenization request
type TokenizeRequest struct {
	Model   string `json:"model"`
	Content string `json:"content"`
}

// TokenizeResponse represents a tokenization response
type TokenizeResponse struct {
	Tokens []int `json:"tokens"`
	Count  int   `json:"count"`
}

// TokenizeChat tokenizes for chat
func (bc *backendClient) TokenizeChat(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error) {
	var resp TokenizeResponse
	if err := bc.doRequest(ctx, "/tokenize/chat", "/tokenize", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// TokenizeEmbed tokenizes for embedding
func (bc *backendClient) TokenizeEmbed(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error) {
	var resp TokenizeResponse
	if err := bc.doRequest(ctx, "/tokenize/embed", "/tokenize", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// TokenizeRerank tokenizes for reranking
func (bc *backendClient) TokenizeRerank(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error) {
	var resp TokenizeResponse
	if err := bc.doRequest(ctx, "/tokenize/rerank", "/tokenize", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
