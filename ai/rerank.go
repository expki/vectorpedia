package ai

import "context"

type RerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	Model     string   `json:"model,omitempty"`
}

type RerankResponse struct {
	Model   string         `json:"model"`
	Object  string         `json:"object"`
	Usage   Usage          `json:"usage"`
	Results []RerankResult `json:"results"`
}

type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

func (c *client) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	resp, err := c.doRequest(ctx, "/v1/rerank", req, &RerankResponse{})
	if err != nil {
		return nil, err
	}
	return resp.(*RerankResponse), nil
}
