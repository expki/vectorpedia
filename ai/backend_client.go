package ai

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/expki/vectorpedia/logger"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/net/http2"
)

type BackendClient interface {
	// Server Information
	IsHealthy() bool
	ActiveRequests() int64
	ActiveRequestsPerGPU() float64

	// API Methods
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	Embed(ctx context.Context, req *EmbedRequest) (*EmbedResponse, error)
	Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error)
	TokenizeChat(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error)
	TokenizeEmbed(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error)
	TokenizeRerank(ctx context.Context, req *TokenizeRequest) (*TokenizeResponse, error)
	DetokenizeChat(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error)
	DetokenizeEmbed(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error)
	DetokenizeRerank(ctx context.Context, req *DetokenizeRequest) (*DetokenizeResponse, error)

	// Statistics and Cleanup
	GpuCount() int
	GetStatistics(ctx context.Context) ServerStatistics
	Close()
}

// movingAverage tracks the last N request times
type movingAverage struct {
	times []float64
	mu    sync.RWMutex
}

const maxSamples = 100

func newMovingAverage() *movingAverage {
	return &movingAverage{
		times: make([]float64, 0, maxSamples),
	}
}

func (ma *movingAverage) add(value float64) {
	ma.mu.Lock()
	defer ma.mu.Unlock()

	ma.times = append(ma.times, value)
	if len(ma.times) > maxSamples {
		ma.times = ma.times[1:] // Remove oldest
	}
}

func (ma *movingAverage) getStats() EndpointStats {
	ma.mu.RLock()
	defer ma.mu.RUnlock()

	if len(ma.times) == 0 {
		return EndpointStats{}
	}

	// Calculate statistics
	sum := float64(0)
	min := ma.times[0]
	max := ma.times[0]

	for _, t := range ma.times {
		sum += t
		if t < min {
			min = t
		}
		if t > max {
			max = t
		}
	}

	return EndpointStats{
		AverageMs: sum / float64(len(ma.times)),
		MinMs:     min,
		MaxMs:     max,
		LastMs:    ma.times[len(ma.times)-1],
	}
}

// endpointMetricsTracker tracks metrics for all endpoints
type endpointMetricsTracker struct {
	chat             *movingAverage
	embed            *movingAverage
	rerank           *movingAverage
	tokenizeChat     *movingAverage
	tokenizeEmbed    *movingAverage
	tokenizeRerank   *movingAverage
	detokenizeChat   *movingAverage
	detokenizeEmbed  *movingAverage
	detokenizeRerank *movingAverage
}

func newEndpointMetricsTracker() *endpointMetricsTracker {
	return &endpointMetricsTracker{
		chat:             newMovingAverage(),
		embed:            newMovingAverage(),
		rerank:           newMovingAverage(),
		tokenizeChat:     newMovingAverage(),
		tokenizeEmbed:    newMovingAverage(),
		tokenizeRerank:   newMovingAverage(),
		detokenizeChat:   newMovingAverage(),
		detokenizeEmbed:  newMovingAverage(),
		detokenizeRerank: newMovingAverage(),
	}
}

// backendClient represents a connection to a single backend server
type backendClient struct {
	url        string
	token      string
	httpClient *http.Client
	encoder    *zstd.Encoder
	decoder    *zstd.Decoder

	// Metrics
	activeRequests atomic.Int64
	totalRequests  atomic.Int64
	isHealthy      atomic.Bool
	gpuCount       atomic.Int32

	// Endpoint timing metrics
	endpointMetrics *endpointMetricsTracker

	// Context for lifecycle management
	appCtx context.Context
}

// NewBackendClient creates a new backend client for a specific server
func NewBackendClient(appCtx context.Context, url string, token string) (BackendClient, error) {
	httpClient, err := createBackendHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}

	bc := &backendClient{
		url:             url,
		token:           token,
		httpClient:      httpClient,
		encoder:         encoder,
		decoder:         decoder,
		appCtx:          appCtx,
		endpointMetrics: newEndpointMetricsTracker(),
	}

	// Set initial defaults
	bc.isHealthy.Store(true)
	bc.gpuCount.Store(1)

	ctx, cancel := context.WithTimeout(appCtx, 10*time.Second)
	defer cancel()
	res, err := bc.GetGPUInfo(ctx)
	if err == nil && len(res.GPUs) > 1 {
		bc.gpuCount.Store(int32(len(res.GPUs)))
	}

	// Start health monitoring
	go bc.healthMonitor()

	return bc, nil
}

func createBackendHTTPClient() (*http.Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		IdleConnTimeout: 20 * time.Second,
		MaxIdleConns:    2,
	}

	if err := http2.ConfigureTransport(transport); err != nil {
		return nil, fmt.Errorf("http2 transport: %v", err)
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

// Close cleans up resources
func (bc *backendClient) Close() {
	bc.encoder.Close()
	bc.decoder.Close()
	if bc.httpClient != nil {
		bc.httpClient.CloseIdleConnections()
	}
}

func (bc *backendClient) GpuCount() int {
	return int(bc.gpuCount.Load())
}

// URL returns the backend server URL
func (bc *backendClient) URL() string {
	return bc.url
}

// IsHealthy returns the health status
func (bc *backendClient) IsHealthy() bool {
	return bc.isHealthy.Load()
}

// ActiveRequests returns the number of active requests
func (bc *backendClient) ActiveRequests() int64 {
	return bc.activeRequests.Load()
}

// TotalRequests returns the total number of requests
func (bc *backendClient) TotalRequests() int64 {
	return bc.totalRequests.Load()
}

// GPUCount returns the number of GPUs
func (bc *backendClient) GPUCount() int32 {
	count := bc.gpuCount.Load()
	if count <= 0 {
		return 1 // Never return 0 to avoid division issues
	}
	return count
}

// ActiveRequestsPerGPU returns the average requests per GPU
func (bc *backendClient) ActiveRequestsPerGPU() float64 {
	return float64(bc.activeRequests.Load()) / float64(bc.GPUCount())
}

// Ping checks if the server is responsive
func (bc *backendClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", bc.url+"/ping", nil)
	if err != nil {
		return err
	}

	if bc.token != "" {
		req.Header.Set("Authorization", "Bearer "+bc.token)
	}

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetGPUInfo fetches GPU information from the backend
func (bc *backendClient) GetGPUInfo(ctx context.Context) (gpuResponse, error) {
	var gpuResp gpuResponse
	req, err := http.NewRequestWithContext(ctx, "GET", bc.url+"/gpus", nil)
	if err != nil {
		return gpuResp, err
	}

	if bc.token != "" {
		req.Header.Set("Authorization", "Bearer "+bc.token)
	}

	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return gpuResp, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return gpuResp, fmt.Errorf("GPU endpoint returned status %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&gpuResp); err != nil {
		return gpuResp, err
	}

	if gpuResp.Error != "" {
		return gpuResp, errors.New(gpuResp.Error)
	}

	return gpuResp, nil
}

// healthMonitor continuously monitors the server health
func (bc *backendClient) healthMonitor() {
	healthTicker := time.NewTicker(15 * time.Second)
	defer healthTicker.Stop()

	// Initial health check
	bc.checkHealth()

	for {
		select {
		case <-bc.appCtx.Done():
			return
		case <-healthTicker.C:
			bc.checkHealth()
		}
	}
}

// checkHealth performs a health check
func (bc *backendClient) checkHealth() {
	wasHealthy := bc.isHealthy.Load()

	ctx, cancel := context.WithTimeout(bc.appCtx, 10*time.Second)
	defer cancel()

	err := bc.Ping(ctx)
	isHealthy := err == nil

	if isHealthy != wasHealthy {
		bc.isHealthy.Store(isHealthy)
		if isHealthy {
			logger.Sugar().Infof("server %s is now healthy", bc.url)
		} else {
			logger.Sugar().Warnf("server %s is now unhealthy: %v", bc.url, err)
		}
	}
}

// doRequest performs a request with tracking
func (bc *backendClient) doRequest(ctx context.Context, name string, endpoint string, request interface{}, response interface{}) error {
	// Start timing
	startTime := time.Now()

	// Track active requests
	bc.activeRequests.Add(1)
	bc.totalRequests.Add(1)
	defer func() {
		bc.activeRequests.Add(-1)

		// Record timing metrics
		elapsedMs := float64(time.Since(startTime).Microseconds()) / 1000.0
		bc.recordEndpointTiming(name, elapsedMs)
	}()

	// Marshal request
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Compress request body
	compressedBody := bc.encoder.EncodeAll(body, nil)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", bc.url+endpoint, bytes.NewReader(compressedBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "zstd")
	req.Header.Set("Accept-Encoding", "zstd")
	if bc.token != "" {
		req.Header.Set("Authorization", "Bearer "+bc.token)
	}

	// Send request
	resp, err := bc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read and decompress response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Decompress if needed
	if resp.Header.Get("Content-Encoding") == "zstd" {
		respBody, err = bc.decoder.DecodeAll(respBody, nil)
		if err != nil {
			return fmt.Errorf("failed to decompress response: %w", err)
		}
	}

	// Unmarshal response
	if err := json.Unmarshal(respBody, response); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return nil
}

// recordEndpointTiming records timing for an endpoint
func (bc *backendClient) recordEndpointTiming(endpoint string, milliseconds float64) {
	switch endpoint {
	case "/chat":
		bc.endpointMetrics.chat.add(milliseconds)
	case "/embed":
		bc.endpointMetrics.embed.add(milliseconds)
	case "/rerank":
		bc.endpointMetrics.rerank.add(milliseconds)
	case "/tokenize/chat":
		bc.endpointMetrics.tokenizeChat.add(milliseconds)
	case "/tokenize/embed":
		bc.endpointMetrics.tokenizeEmbed.add(milliseconds)
	case "/tokenize/rerank":
		bc.endpointMetrics.tokenizeRerank.add(milliseconds)
	case "/detokenize/chat":
		bc.endpointMetrics.detokenizeChat.add(milliseconds)
	case "/detokenize/embed":
		bc.endpointMetrics.detokenizeEmbed.add(milliseconds)
	case "/detokenize/rerank":
		bc.endpointMetrics.detokenizeRerank.add(milliseconds)
	}
}

// getEndpointMetrics returns endpoint metrics
func (bc *backendClient) getEndpointMetrics() *EndpointMetrics {
	metrics := &EndpointMetrics{}

	// Only include metrics that have been used (count > 0)
	metrics.Chat = bc.endpointMetrics.chat.getStats()
	metrics.Embed = bc.endpointMetrics.embed.getStats()
	metrics.Rerank = bc.endpointMetrics.rerank.getStats()
	metrics.TokenizeChat = bc.endpointMetrics.tokenizeChat.getStats()
	metrics.TokenizeEmbed = bc.endpointMetrics.tokenizeEmbed.getStats()
	metrics.TokenizeRerank = bc.endpointMetrics.tokenizeRerank.getStats()
	metrics.DetokenizeChat = bc.endpointMetrics.detokenizeChat.getStats()
	metrics.DetokenizeEmbed = bc.endpointMetrics.detokenizeEmbed.getStats()
	metrics.DetokenizeRerank = bc.endpointMetrics.detokenizeRerank.getStats()

	return metrics
}

// GetStatistics returns statistics for this backend
func (bc *backendClient) GetStatistics(ctx context.Context) ServerStatistics {
	gpuInfo, err := bc.GetGPUInfo(ctx)
	if err != nil {
		logger.Sugar().Debugf("failed to fetch GPU info from %s: %v", bc.url, err)
	}
	stats := ServerStatistics{
		URL:            bc.url,
		IsHealthy:      bc.isHealthy.Load(),
		TotalRequests:  bc.totalRequests.Load(),
		ActiveRequests: bc.activeRequests.Load(),
		GPUCount:       bc.gpuCount.Load(),
		Endpoints:      bc.getEndpointMetrics(),
		GPUs:           gpuInfo.GPUs,
	}
	if err != nil {
		stats.GPUError = err.Error()
	}
	return stats
}
