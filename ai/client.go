package ai

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/expki/vectorpedia/logger"
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
	appCtx         context.Context
}

type server struct {
	url            string
	activeRequests atomic.Int64
	isHealthy      atomic.Bool
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func NewClient(appCtx context.Context, urls []string, token string) (Client, error) {
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
		servers[i].isHealthy.Store(true) // Assume healthy initially
	}

	c := &client{
		token:      token,
		servers:    servers,
		httpClient: httpClient,
		encoder:    encoder,
		decoder:    decoder,
		appCtx:     appCtx,
	}

	// Start health checks for all servers
	for _, srv := range servers {
		go c.healthCheck(srv)
	}

	return c, nil
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

	// Filter healthy servers
	var healthyServers []*server
	for _, srv := range c.servers {
		if srv.isHealthy.Load() {
			healthyServers = append(healthyServers, srv)
		}
	}

	// If no healthy servers, return nil
	if len(healthyServers) == 0 {
		return nil, func() {}
	}

	// Select server with least active requests from healthy servers
	server := healthyServers[rand.IntN(len(healthyServers))]
	requests := server.activeRequests.Load()

	for _, challengerServer := range healthyServers {
		challengerRequests := challengerServer.activeRequests.Load()
		if challengerRequests >= requests {
			continue
		}
		server = challengerServer
		requests = challengerRequests
	}

	server.activeRequests.Add(1)
	return server, func() {
		server.activeRequests.Add(-1)
	}
}

func (c *client) healthCheck(srv *server) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Do initial health check
	c.checkServerHealth(srv)

	for {
		select {
		case <-c.appCtx.Done():
			return
		case <-ticker.C:
			c.checkServerHealth(srv)
		}
	}
}

func (c *client) checkServerHealth(srv *server) {
	wasHealthy := srv.isHealthy.Load()
	ctx, cancel := context.WithTimeout(c.appCtx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv.url+"/ping", nil)
	if err != nil {
		if wasHealthy {
			srv.isHealthy.Store(false)
			logger.Sugar().Warnf("server is down: %s", srv.url)
		}
		return
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if wasHealthy {
			srv.isHealthy.Store(false)
			logger.Sugar().Warnf("server is down: %s", srv.url)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		if !wasHealthy {
			srv.isHealthy.Store(true)
			logger.Sugar().Infof("server is up: %s", srv.url)
		}
	} else {
		if wasHealthy {
			srv.isHealthy.Store(false)
			logger.Sugar().Warnf("server is down: %s", srv.url)
		}
	}
}
