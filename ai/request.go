package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/expki/calculator/lib/encoding"
)

func (c *client) doRequestWithQueryType(ctx context.Context, endpoint string, reqBody any, respBody any, queryType string) (any, error) {
	srv, done := c.selectServer()
	defer done()
	if srv == nil {
		return nil, fmt.Errorf("no available servers")
	}

	binaryData := encoding.Encode(reqBody)
	compressed := c.encoder.EncodeAll(binaryData, nil)

	req, err := http.NewRequestWithContext(ctx, "POST", srv.url+endpoint, bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "zstd")
	req.Header.Set("Accept-Encoding", "zstd")
	req.Header.Set("Encode-Binary", "true")
	req.Header.Set("Accept-Binary", "true")
	if queryType != "" {
		req.Header.Set("Query-Type", queryType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	httpClient, err := c.getHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTP client: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var decompressed []byte
	if resp.Header.Get("Content-Encoding") == "zstd" {
		decompressed, err = c.decoder.DecodeAll(body, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress response: %w", err)
		}
	} else {
		decompressed = body
	}

	var data []byte
	if resp.Header.Get("Encode-Binary") == "true" {
		raw, _ := encoding.Decode(decompressed)
		data, err = json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to json encode the decoded response: %w", err)
		}
	} else {
		data = decompressed
	}

	return unmarshalResponse(data, respBody)
}

func (c *client) doRequest(ctx context.Context, endpoint string, reqBody any, respBody any) (any, error) {
	// Determine Query-Type based on response type
	var queryType string
	switch respBody.(type) {
	case *ChatResponse:
		queryType = "chat"
	case *EmbedResponse:
		queryType = "embed"
	case *RerankResponse:
		queryType = "rerank"
	default:
		// For types that don't have a specific query type
		queryType = ""
	}

	return c.doRequestWithQueryType(ctx, endpoint, reqBody, respBody, queryType)
}

func unmarshalResponse(data []byte, respBody any) (any, error) {
	switch v := respBody.(type) {
	case *ChatResponse:
		var chatResp ChatResponse
		if err := json.Unmarshal(data, &chatResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return &chatResp, nil
	case *EmbedResponse:
		var embedResp EmbedResponse
		if err := json.Unmarshal(data, &embedResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return &embedResp, nil
	case *TokenizeResponse:
		var tokenResp TokenizeResponse
		if err := json.Unmarshal(data, &tokenResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return &tokenResp, nil
	case *DetokenizeResponse:
		var detokenResp DetokenizeResponse
		if err := json.Unmarshal(data, &detokenResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return &detokenResp, nil
	case *RerankResponse:
		var rerankResp RerankResponse
		if err := json.Unmarshal(data, &rerankResp); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return &rerankResp, nil
	default:
		return nil, fmt.Errorf("unknown response type: %T", v)
	}
}
