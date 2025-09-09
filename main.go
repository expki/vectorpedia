package main

import (
	//_ "net/http/pprof"

	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/config"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/dnc"
	"github.com/expki/vectorpedia/logger"
	"github.com/expki/vectorpedia/server"
	"github.com/expki/vectorpedia/static"
	"github.com/expki/vectorpedia/wikipedia"

	"github.com/klauspost/compress/zstd"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/http2"
)

func main() {
	// Parse command-line flags
	var (
		configPath    = flag.String("config", "", "Path to configuration file (required)")
		wikipediaPath = flag.String("wikipedia", "", "Path to Wikipedia compressed XML file for import (optional)")
		reindex       = flag.Bool("reindex", false, "Trigger K-means clustering to reindex embeddings (optional)")
		reembed       = flag.Bool("reembed", false, "Regenerate embeddings for all titles and summaries (optional)")
		showHelp      = flag.Bool("help", false, "Show help information")
	)

	// Custom usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Vectorpedia - Vector search engine for Wikipedia data\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s --config <path> [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Start server with config file\n")
		fmt.Fprintf(os.Stderr, "  %s --config config.json\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Import Wikipedia data\n")
		fmt.Fprintf(os.Stderr, "  %s --config config.json --wikipedia wikipedia-dump.xml.br\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Reindex embeddings with K-means clustering\n")
		fmt.Fprintf(os.Stderr, "  %s --config config.json --reindex\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Regenerate embeddings for titles and summaries\n")
		fmt.Fprintf(os.Stderr, "  %s --config config.json --reembed\n\n", os.Args[0])
	}

	flag.Parse()

	// Show help if requested
	if *showHelp {
		flag.Usage()
		os.Exit(0)
	}

	// Validate required flags
	if *configPath == "" {
		fmt.Fprintf(os.Stderr, "Error: --config flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	appCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()

	// Load config
	log.Default().Printf("Config path: %s\n", *configPath)
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		log.Default().Printf("Creating sample config: %s\n", *configPath)
		err = config.CreateSample(*configPath)
		if err != nil {
			log.Fatalf("CreateSample: %v", err)
		}
	}
	log.Default().Println("Reading config...")
	configRaw, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("ReadFile %q: %v", *configPath, err)
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

	var gpuCount int
	for _, backend := range aiClient.AllServers() {
		gpuCount += backend.GpuCount()
	}

	// Create Wikipedia instance
	wikipediaInstance := wikipedia.New(db, aiClient, cfg.CtxSizeChat, cfg.CtxSizeEmbed, cfg.CtxSizeRerank, gpuCount)

	// Server
	srv := server.NewServer(db, aiClient, wikipediaInstance)

	// Import Wikipedia if path provided
	original := cfg.LogLevel.Zap().Level()
	if *wikipediaPath != "" {
		if logger.Sugar().Level() != zapcore.DebugLevel {
			cfg.LogLevel.Zap().SetLevel(zap.ErrorLevel)
		}
		go func() {
			logger.Sugar().Infof("Loading Wikipedia from: %s", *wikipediaPath)
			err = wikipediaInstance.ImportFromFile(appCtx, *wikipediaPath)
			cfg.LogLevel.Zap().SetLevel(original)
			if err != nil {
				logger.Sugar().Fatalf("Wikipedia import: %v", err)
			} else {
				logger.Sugar().Info("Wikipedia import completed successfully")
			}
		}()
	}

	// Reindex embeddings if requested
	if *reindex {
		if logger.Sugar().Level() != zapcore.DebugLevel {
			cfg.LogLevel.Zap().SetLevel(zap.ErrorLevel)
		}
		go func() {
			logger.Sugar().Info("Starting K-means clustering to reindex embeddings...")
			err = dnc.KMeansDivideAndConquer(appCtx, db)
			cfg.LogLevel.Zap().SetLevel(original)
			if err != nil {
				logger.Sugar().Errorf("K-means reindexing failed: %v", err)
			} else {
				logger.Sugar().Info("K-means reindexing completed successfully")
			}
		}()
	}

	// Reembed titles and summaries if requested
	if *reembed {
		if logger.Sugar().Level() != zapcore.DebugLevel {
			cfg.LogLevel.Zap().SetLevel(zap.ErrorLevel)
		}
		go func() {
			logger.Sugar().Info("Starting to regenerate embeddings for titles and summaries...")
			err = wikipediaInstance.ReembedEmbeddings(appCtx)
			cfg.LogLevel.Zap().SetLevel(original)
			if err != nil {
				logger.Sugar().Errorf("Reembedding failed: %v", err)
			} else {
				logger.Sugar().Info("Reembedding completed successfully")
			}
		}()
	}

	// Create mux
	mux := http.NewServeMux()

	// HTTP
	httpServer := http.Server{
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
	mux.Handle("/api/statistics", middlewareHeaders(middlewareDecompression(middlewareCompression(http.HandlerFunc(srv.StatisticsHandler)))))
	// Summary stats endpoint (cached for quick response)
	mux.Handle("/api/stats/summary", middlewareHeaders(middlewareDecompression(middlewareCompression(http.HandlerFunc(srv.SummaryStatsHandler)))))
	// Search endpoint
	mux.Handle("/api/search", middlewareHeaders(middlewareDecompression(middlewareCompression(http.HandlerFunc(srv.SearchHandler)))))

	// Routes: Files - serve static files with SPA fallback
	fileServer := http.FileServerFS(static.Files)
	mux.Handle("/{path...}", middlewareHeaders(middlewareDecompression(middlewareCompression(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the file exists in the static files
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}

		// Try to open the file
		file, err := static.Files.Open(strings.TrimPrefix(path, "/"))
		if err == nil {
			file.Close()
			// File exists, serve it normally
			fileServer.ServeHTTP(w, r)
		} else {
			// File doesn't exist, serve index.html for React routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		}
	})))))

	// Start servers
	serverDone := make(chan struct{})
	go func() {
		logger.Sugar().Infof("HTTP server starting on %s", cfg.Server.HttpAddress)
		err := httpServer.ListenAndServe()
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
	httpServer.Shutdown(shutdownCtx)
	server2.Shutdown(shutdownCtx)
	httpServer.Close()
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
