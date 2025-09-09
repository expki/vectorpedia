package server

import (
	"context"
	"sync"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/logger"
	"github.com/expki/vectorpedia/wikipedia"
	"gorm.io/plugin/dbresolver"
)

type DatabaseStats struct {
	Pages      int64 `json:"pages"`
	Embeddings int64 `json:"embeddings"`
	Centroids  int64 `json:"centroids"`
}

type Server struct {
	db        *database.Database
	ai        ai.Client
	wikipedia *wikipedia.Wikipedia
	
	// Cached statistics
	statsMu sync.RWMutex
	cachedStats *DatabaseStats
}

func NewServer(db *database.Database, ai ai.Client, wikipedia *wikipedia.Wikipedia) *Server {
	s := &Server{
		db:        db,
		ai:        ai,
		wikipedia: wikipedia,
	}
	
	// Initialize statistics cache at startup
	s.initializeStatisticsCache()
	
	return s
}

func (s *Server) initializeStatisticsCache() {
	logger.Sugar().Info("Initializing statistics cache...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	var pageCount, embeddingCount, centroidCount int64
	
	// Fetch counts with longer timeout for startup
	s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Page{}).Count(&pageCount)
	s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Embedding{}).Count(&embeddingCount)
	s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Centroid{}).Count(&centroidCount)
	
	s.statsMu.Lock()
	s.cachedStats = &DatabaseStats{
		Pages:      pageCount,
		Embeddings: embeddingCount,
		Centroids:  centroidCount,
	}
	s.statsMu.Unlock()
	
	logger.Sugar().Infof("Statistics cache initialized: %d pages, %d embeddings, %d centroids",
		pageCount, embeddingCount, centroidCount)
	
	// Start background refresh goroutine (updates every 5 minutes)
	go s.refreshStatisticsCache()
}

func (s *Server) refreshStatisticsCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		
		var pageCount, embeddingCount, centroidCount int64
		s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Page{}).Count(&pageCount)
		s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Embedding{}).Count(&embeddingCount)
		s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Centroid{}).Count(&centroidCount)
		
		s.statsMu.Lock()
		s.cachedStats = &DatabaseStats{
			Pages:      pageCount,
			Embeddings: embeddingCount,
			Centroids:  centroidCount,
		}
		s.statsMu.Unlock()
		
		cancel()
		logger.Sugar().Debugf("Statistics cache refreshed: %d pages, %d embeddings, %d centroids",
			pageCount, embeddingCount, centroidCount)
	}
}

func (s *Server) GetCachedStats() DatabaseStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	
	if s.cachedStats == nil {
		return DatabaseStats{
			Pages:      6694021, // Default fallback values
			Embeddings: 0,
			Centroids:  0,
		}
	}
	
	return *s.cachedStats
}
