package main

import (
	//_ "net/http/pprof"

	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/config"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/logger"
	"github.com/expki/vectorpedia/static"
	"github.com/expki/vectorpedia/wikipedia"

	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
)

// ProcessingStatisticsJSON represents processing metrics in JSON format
type ProcessingStatisticsJSON struct {
	AverageEmbeddingTimeMs float64 `json:"average_embedding_time_ms"`
	AverageSummaryTimeMs   float64 `json:"average_summary_time_ms"`
	AverageInsertTimeMs    float64 `json:"average_insert_time_ms"`
	TotalEmbeddings        int64   `json:"total_embeddings"`
	TotalSummaries         int64   `json:"total_summaries"`
	TotalInserts           int64   `json:"total_inserts"`
}

// calculateAverage calculates average time in milliseconds
func calculateAverage(totalNanos int64, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(totalNanos) / float64(count) / 1_000_000.0 // Convert nanoseconds to milliseconds
}

func main() {
	//go func() {
	//	log.Println("Starting pprof server on :6060")
	//	log.Println("http://localhost:6060/debug/pprof/")
	//	log.Println(http.ListenAndServe("localhost:6060", nil))
	//}()

	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()

	// Load config
	var configPath string = os.Args[1]
	log.Default().Printf("Config path: %s\n", configPath)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Default().Printf("Creating sample config: %s\n", configPath)
		err = config.CreateSample(configPath)
		if err != nil {
			log.Fatalf("CreateSample: %v", err)
		}
	}
	log.Default().Println("Reading config...")
	configRaw, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("ReadFile %q: %v", configPath, err)
	}
	log.Default().Println("Parsing config...")
	cfg, err := config.ParseConfig(configRaw)
	if err != nil {
		log.Fatalf("ParseConfig: %v", err)
	}
	log.Default().Println("Loading TLS...")
	err = cfg.TLS.Configurate()
	if err != nil {
		log.Fatalf("Configurate: %v", err)
	}

	// Logger
	log.Default().Println("Setting log level:", cfg.LogLevel.String())
	logConf := zap.NewDevelopmentConfig()
	logConf.Level = cfg.LogLevel.Zap()
	l, err := logConf.Build()
	if err != nil {
		log.Fatalf("zap.NewDevelopment: %v", err)
	}
	logger.Initialize(l)
	defer l.Sync()

	// AI
	logger.Sugar().Info("Loading AI Client...")
	aiClient, err := ai.NewClient(appCtx, cfg.URL, cfg.Token)
	if err != nil {
		logger.Sugar().Fatalf("ai.New: %v", err)
	}

	// Database
	logger.Sugar().Info("Loading database...")
	db, err := database.New(appCtx, cfg.Database)
	if err != nil {
		logger.Sugar().Fatalf("database.New: %v", err)
	}

	// Create Wikipedia instance (store it for metrics)
	var wikipediaInstance *wikipedia.Wikipedia
	var wikipediaLock sync.RWMutex

	// Import
	if len(os.Args) > 2 {
		logger.Sugar().Info("Loading Wikipedia...")
		wikipediaInstance = wikipedia.New(db, aiClient, cfg.CtxSizeChat, cfg.CtxSizeEmbed, cfg.CtxSizeRerank, len(cfg.URL))
		err = wikipediaInstance.ImportFromFile(appCtx, os.Args[2])
		if err != nil {
			logger.Sugar().Fatalf("wikipedia import: %v", err)
		}
		return
	}

	// Create mux
	mux := http.NewServeMux()

	// HTTP
	server := http.Server{
		Handler: mux,
		Addr:    cfg.Server.HttpAddress,
	}

	// HTTP2
	server2 := http.Server{
		Handler: mux,
		Addr:    cfg.Server.HttpsAddress,
		TLSConfig: &tls.Config{
			GetCertificate: cfg.TLS.GetCertificate,
			ClientAuth:     tls.NoClientCert,
			NextProtos:     []string{"h2", "http/1.1"},
		},
	}
	err = http2.ConfigureServer(&server2, &http2.Server{})
	if err != nil {
		logger.Sugar().Fatalf("http2.Server: %v", err)
	}

	// Headers middleware
	middlewareHeaders := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// WASM headers
			w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
			w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
			h.ServeHTTP(w, r)
		})
	}

	// Decompression middleware
	middlewareDecompression := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Content-Encoding"), "zstd") {
				h.ServeHTTP(w, r)
				return
			}
			reader, err := zstd.NewReader(r.Body, zstd.WithDecoderLowmem(true))
			if err != nil {
				logger.Sugar().Errorf("Failed to create zstd reader: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			defer reader.Close()
			r.Body = &zstdRequestReader{ReadCloser: r.Body, Reader: reader}
			h.ServeHTTP(w, r)
		})
	}

	// Compression middleware
	middlewareCompression := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "zstd") {
				h.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Encoding", "zstd")
			encoder, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedFastest))
			if err != nil {
				logger.Sugar().Errorf("Failed to create zstd encoder: %v", err)
				h.ServeHTTP(w, r)
				return
			}
			defer encoder.Close()
			zstrw := &zstdResponseWriter{ResponseWriter: w, Writer: encoder}
			h.ServeHTTP(zstrw, r)
		})
	}

	// Routes: API
	// Statistics endpoint
	mux.HandleFunc("/api/statistics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Prepare statistics response
		stats := struct {
			Servers    ai.ClientStatistics        `json:"servers"`
			Processing *ProcessingStatisticsJSON  `json:"processing,omitempty"`
		}{
			Servers: aiClient.GetStatistics(),
		}

		// Add Wikipedia processing metrics if available
		wikipediaLock.RLock()
		if wikipediaInstance != nil {
			metrics := wikipediaInstance.GetMetrics()
			stats.Processing = &ProcessingStatisticsJSON{
				AverageEmbeddingTimeMs: calculateAverage(metrics.EmbeddingTimeTotal, metrics.EmbeddingCount),
				AverageSummaryTimeMs:   calculateAverage(metrics.SummaryTimeTotal, metrics.SummaryCount),
				AverageInsertTimeMs:    calculateAverage(metrics.InsertTimeTotal, metrics.InsertCount),
				TotalEmbeddings:        metrics.EmbeddingCount,
				TotalSummaries:         metrics.SummaryCount,
				TotalInserts:           metrics.InsertCount,
			}
		}
		wikipediaLock.RUnlock()

		// Set headers and encode response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			logger.Sugar().Errorf("Failed to encode statistics: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	})

	// Routes: Files
	mux.Handle("/", middlewareHeaders(middlewareDecompression(middlewareCompression(http.FileServerFS(static.Files)))))

	// Start servers
	serverDone := make(chan struct{})
	go func() {
		logger.Sugar().Infof("HTTP server starting on %s", cfg.Server.HttpAddress)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Sugar().Errorf("ListenAndServe http: %v", err)
		}
		close(serverDone)
	}()
	server2Done := make(chan struct{})
	go func() {
		logger.Sugar().Infof("HTTP2 server starting on %s", cfg.Server.HttpsAddress)
		err := server2.ListenAndServeTLS("", "")
		if err != nil && err != http.ErrServerClosed {
			logger.Sugar().Errorf("ListenAndServe https (http2): %v", err)
		}
		close(server2Done)
	}()

	// Interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Wait for servers to finish
	select {
	case <-interrupt:
		logger.Sugar().Info("Interrupt signal received")
	case <-appCtx.Done():
		logger.Sugar().Info("App stopped")
	case <-serverDone:
		logger.Sugar().Info("HTTP server stopped")
	case <-server2Done:
		logger.Sugar().Info("HTTP2 server stopped")
	}
	stopApp()
	logger.Sugar().Info("Server shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(appCtx, 3*time.Second)
	defer cancelShutdown()
	server.Shutdown(shutdownCtx)
	server2.Shutdown(shutdownCtx)
	server.Close()
	server2.Close()
	db.Close()
	logger.Sugar().Info("Server stopped")
}

// zstdResponseWriter wraps the http.ResponseWriter to provide zstd compression
type zstdResponseWriter struct {
	http.ResponseWriter
	Writer *zstd.Encoder
}

func (w *zstdResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// zstdResponseWriter wraps the io.ReadClose to provide zstd decompression
type zstdRequestReader struct {
	io.ReadCloser
	Reader *zstd.Decoder
}

func (r *zstdRequestReader) Read(p []byte) (int, error) {
	return r.Reader.Read(p)
}
