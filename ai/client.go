package ai

import (
	"context"
	"fmt"
	"sync"

	"github.com/expki/vectorpedia/logger"
)

// Client manages multiple backend servers and provides server selection
type Client interface {
	// Server Selection Methods
	SelectServer() BackendClient
	SelectServerGPU() BackendClient
	AllServers() []BackendClient

	// Management
	Close()
	GetStatistics(ctx context.Context) ClientStatistics
}

// client manages multiple backend servers
type client struct {
	servers []BackendClient
	mu      sync.RWMutex
	ctx     context.Context
	cancel  context.CancelFunc
}

// GPUInfo represents information about a single GPU
type GPUInfo struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	MemoryUsed  uint64  `json:"memory_used_bytes"`
	MemoryTotal uint64  `json:"memory_total_bytes"`
	MemoryUsage float64 `json:"memory_usage_percent"`
	CoreUsage   uint32  `json:"core_usage_percent"`
	Temperature uint32  `json:"temperature_celsius"`
	PowerDraw   uint32  `json:"power_draw_watts"`
}

// EndpointStats tracks performance metrics for an endpoint
type EndpointStats struct {
	AverageMs float64 `json:"average_ms"`
	MinMs     float64 `json:"min_ms"`
	MaxMs     float64 `json:"max_ms"`
	LastMs    float64 `json:"last_ms"`
}

// EndpointMetrics contains timing metrics for all endpoints
type EndpointMetrics struct {
	Chat             EndpointStats `json:"chat,omitempty"`
	Embed            EndpointStats `json:"embed,omitempty"`
	Rerank           EndpointStats `json:"rerank,omitempty"`
	TokenizeChat     EndpointStats `json:"tokenize_chat,omitempty"`
	TokenizeEmbed    EndpointStats `json:"tokenize_embed,omitempty"`
	TokenizeRerank   EndpointStats `json:"tokenize_rerank,omitempty"`
	DetokenizeChat   EndpointStats `json:"detokenize_chat,omitempty"`
	DetokenizeEmbed  EndpointStats `json:"detokenize_embed,omitempty"`
	DetokenizeRerank EndpointStats `json:"detokenize_rerank,omitempty"`
}

// ServerStatistics contains stats for a single server
type ServerStatistics struct {
	URL            string           `json:"url"`
	IsHealthy      bool             `json:"is_healthy"`
	TotalRequests  int64            `json:"total_requests"`
	ActiveRequests int64            `json:"active_requests"`
	GPUCount       int32            `json:"gpu_count"`
	GPUs           []GPUInfo        `json:"gpus,omitempty"`
	GPUError       string           `json:"gpu_error,omitempty"`
	Endpoints      *EndpointMetrics `json:"endpoints,omitempty"`
}

// ClientStatistics contains stats for all servers
type ClientStatistics struct {
	Servers []ServerStatistics `json:"servers"`
}

// gpuResponse represents GPU info from backend
type gpuResponse struct {
	GPUs  []GPUInfo `json:"gpus"`
	Count int       `json:"count"`
	Error string    `json:"error,omitempty"`
}

// NewClient creates a new AI client managing multiple backends
func NewClient(appCtx context.Context, urls []string, token string) (Client, error) {
	if len(urls) == 0 {
		return nil, fmt.Errorf("at least one URL must be provided")
	}

	ctx, cancel := context.WithCancel(appCtx)

	// Create backend clients for each URL in parallel
	type result struct {
		client BackendClient
		err    error
		url    string
	}
	
	results := make(chan result, len(urls))
	var wg sync.WaitGroup
	wg.Add(len(urls))
	
	for _, url := range urls {
		go func(url string) {
			defer wg.Done()
			bc, err := NewBackendClient(ctx, url, token)
			results <- result{client: bc, err: err, url: url}
		}(url)
	}
	
	// Wait for all goroutines to complete
	wg.Wait()
	close(results)
	
	// Collect successful clients
	servers := make([]BackendClient, 0, len(urls))
	for res := range results {
		if res.err != nil {
			logger.Sugar().Warnf("failed to create backend client for %s: %v", res.url, res.err)
			continue
		}
		servers = append(servers, res.client)
	}

	if len(servers) == 0 {
		cancel()
		return nil, fmt.Errorf("failed to create any backend clients")
	}

	return &client{
		servers: servers,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
}

// Close shuts down all backend clients
func (c *client) Close() {
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, server := range c.servers {
		server.Close()
	}
}

// SelectServer selects the backend with the least active requests
func (c *client) SelectServer() BackendClient {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var selected BackendClient = c.servers[0]
	var minRequests int64 = selected.ActiveRequests()

	for _, server := range c.servers {
		if !server.IsHealthy() {
			continue
		}

		requests := server.ActiveRequests()
		if requests < minRequests {
			selected = server
			minRequests = requests
		}
	}

	return selected
}

// SelectServerGPU selects the backend with the least active requests per GPU
func (c *client) SelectServerGPU() BackendClient {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var selected BackendClient = c.servers[0]
	var minRequestsPerGPU float64 = selected.ActiveRequestsPerGPU()

	for _, server := range c.servers {
		if !server.IsHealthy() {
			continue
		}

		requestsPerGPU := server.ActiveRequestsPerGPU()
		if requestsPerGPU < minRequestsPerGPU {
			selected = server
			minRequestsPerGPU = requestsPerGPU
		}
	}

	return selected
}

// AllServers returns all healthy backend servers
func (c *client) AllServers() []BackendClient {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var healthy []BackendClient
	for _, server := range c.servers {
		if server.IsHealthy() {
			healthy = append(healthy, server)
		}
	}

	return healthy
}

// GetStatistics returns statistics for all servers
func (c *client) GetStatistics(ctx context.Context) ClientStatistics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := ClientStatistics{
		Servers: make([]ServerStatistics, len(c.servers)),
	}

	var wg sync.WaitGroup
	wg.Add(len(c.servers))
	for i, server := range c.servers {
		wg.Go(func() {
			stats.Servers[i] = server.GetStatistics(ctx)
			wg.Done()
		})
	}
	wg.Wait()

	return stats
}
