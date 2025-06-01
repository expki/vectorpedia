package server

import (
	"sync/atomic"

	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	"github.com/expki/go-vectorsearch/server"
	"github.com/expki/vectorpedia/config"
	"golang.org/x/sync/singleflight"
)

var index atomic.Uint64

type Server struct {
	centroidCountCache  atomic.Bool
	documentCountCache  atomic.Bool
	embeddingCountCache atomic.Bool
	lastCentroidCount   atomic.Int64
	lastDocumentCount   atomic.Int64
	lastEmbeddingCount  atomic.Int64
	singleCount         singleflight.Group
	config              config.Config
	vs                  *server.Server
	db                  *database.Database
}

func New(cfg config.Config, vs *server.Server, db *database.Database) *Server {
	s := &Server{
		vs:     vs,
		config: cfg,
		db:     db,
	}
	// Load initial count
	s.SetCentroidCountCache(true)
	s.SetDocumentCountCache(true)
	s.SetEmbeddingCountCache(true)
	_, _, _, err := s.Count()
	if err != nil {
		logger.Sugar().Errorf("Failed to do first count: %v", err)
	} else {
		s.SetCentroidCountCache(false)
		s.SetDocumentCountCache(false)
		s.SetEmbeddingCountCache(false)
	}
	// return
	return s
}

func (s *Server) SetCentroidCountCache(enabled bool) {
	if enabled {
		logger.Sugar().Debug("updating cache count centroids: enabled")
	} else {
		logger.Sugar().Debug("updating cache count centroids: disabled")
	}
	s.centroidCountCache.Store(enabled)
}

func (s *Server) SetDocumentCountCache(enabled bool) {
	if enabled {
		logger.Sugar().Debug("updating cache count paragraphs: enabled")
	} else {
		logger.Sugar().Debug("updating cache count paragraphs: disabled")
	}
	s.documentCountCache.Store(enabled)
}

func (s *Server) SetEmbeddingCountCache(enabled bool) {
	if enabled {
		logger.Sugar().Debug("updating cache count paragraphs: enabled")
	} else {
		logger.Sugar().Debug("updating cache count paragraphs: disabled")
	}
	s.embeddingCountCache.Store(enabled)
}

func (s *Server) VectorSearch() *server.Server {
	return s.vs
}
