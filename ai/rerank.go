package ai

import (
	"context"
)

// RerankRequest represents a reranking request
type RerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// RerankResponse represents a reranking response
type RerankResponse struct {
	Model   string         `json:"model"`
	Results []RerankResult `json:"results"`
}

// RerankResult represents a single rerank result
type RerankResult struct {
	Index    int     `json:"index"`
	Document string  `json:"document"`
	Score    float32 `json:"relevance_score"`
}

// Rerank reranks documents
func (bc *backendClient) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	var resp RerankResponse
	if err := bc.doRequest(ctx, "/rerank", "/v1/rerank", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
