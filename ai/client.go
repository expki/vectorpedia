package ai

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/net/http2"
)

type Client interface {
	// Chat sends a chat completion request
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// Embed generates embeddings for the given input
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)

	// Rerank reranks documents based on relevance to a query
	Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error)

	// TokenizeChat tokenizes content for chat queries
	TokenizeChat(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error)

	// TokenizeEmbed tokenizes content for embedding queries
	TokenizeEmbed(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error)

	// TokenizeRerank tokenizes content for rerank queries
	TokenizeRerank(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error)

	// DetokenizeChat detokenizes tokens for chat queries
	DetokenizeChat(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error)

	// DetokenizeEmbed detokenizes tokens for embedding queries
	DetokenizeEmbed(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error)

	// DetokenizeRerank detokenizes tokens for rerank queries
	DetokenizeRerank(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error)

	// Close cleans up resources used by the client
	Close()
}

type client struct {
	token          string
	servers        []*server
	httpClient     *http.Client
	clientRequests atomic.Int64
	encoder        *zstd.Encoder
	decoder        *zstd.Decoder
	mu             sync.RWMutex
	clientMu       sync.RWMutex
}

type server struct {
	url            string
	activeRequests atomic.Int64
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewClient(urls []string, token string) (Client, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("at least one URL must be provided")
	}

	httpClient, err := createHTTPClient()
	if err != nil {
		return nil, err
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	servers := make([]*server, len(urls))
	for i, url := range urls {
		servers[i] = &server{
			url: url,
		}
	}

	return &client{
		token:      token,
		servers:    servers,
		httpClient: httpClient,
		encoder:    encoder,
		decoder:    decoder,
	}, nil
}

func createHTTPClient() (*http.Client, error) {
	transport := &http2.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

func (c *client) Close() {
	c.encoder.Close()
	c.decoder.Close()
}

func (c *client) getHTTPClient() (*http.Client, error) {
	if c.clientRequests.Add(1) > 200 {
		c.clientMu.Lock()
		defer c.clientMu.Unlock()

		newClient, err := createHTTPClient()
		if err != nil {
			return nil, err
		}
		c.httpClient = newClient
		c.clientRequests.Store(1)
		return newClient, nil
	}

	c.clientMu.RLock()
	defer c.clientMu.RUnlock()
	return c.httpClient, nil
}

func (c *client) selectServer() (*server, func()) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	server := c.servers[rand.IntN(len(c.servers)-1)]
	requests := server.activeRequests.Load()

	for _, challengerServer := range c.servers {
		challengerRequests := challengerServer.activeRequests.Load()
		if challengerRequests >= requests {
			continue
		}
		server = challengerServer
		requests = challengerRequests
		break
	}

	server.activeRequests.Add(1)
	return server, func() {
		server.activeRequests.Add(-1)
	}
}
