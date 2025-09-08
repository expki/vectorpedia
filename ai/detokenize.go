package ai

import (
	"context"
)

// DetokenizeRequest represents a detokenization request
type DetokenizeRequest struct {
	Model  string `json:"model"`
	Tokens []int  `json:"tokens"`
}

// DetokenizeResponse represents a detokenization response
type DetokenizeResponse struct {
	Content string `json:"content"`
}

// DetokenizeChat detokenizes chat tokens
func (bc *backendClient) DetokenizeChat(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error) {
	var resp DetokenizeResponse
	if err := bc.doRequest(ctx, "/detokenize/chat", "/detokenize", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DetokenizeEmbed detokenizes embedding tokens
func (bc *backendClient) DetokenizeEmbed(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error) {
	var resp DetokenizeResponse
	if err := bc.doRequest(ctx, "/detokenize/embed", "/detokenize", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DetokenizeRerank detokenizes rerank tokens
func (bc *backendClient) DetokenizeRerank(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error) {
	var resp DetokenizeResponse
	if err := bc.doRequest(ctx, "/detokenize/rerank", "/detokenize", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
