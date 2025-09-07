package ai

import "context"

type EmbedRequest struct {
	Model string   `json:"model,omitempty"`
	Input []string `json:"input"`
}

type EmbedResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  Usage       `json:"usage"`
}

type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

func (c *client) Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error) {
	resp, err := c.doRequest(ctx, "/v1/embeddings", req, &EmbedResponse{})
	if err != nil {
		return nil, err
	}
	return resp.(*EmbedResponse), nil
}
