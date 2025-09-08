package ai

import (
	"context"
)

// EmbedRequest represents an embedding request
type EmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// EmbedResponse represents an embedding response
type EmbedResponse struct {
	Model string      `json:"model"`
	Data  []Embedding `json:"data"`
	Usage Usage       `json:"usage,omitempty"`
}

// Embedding represents a single embedding
type Embedding struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// Embed generates embeddings
func (bc *backendClient) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	var resp EmbedResponse
	if err := bc.doRequest(ctx, "/embed", "/v1/embeddings", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
