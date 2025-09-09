package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/logger"
	"github.com/expki/vectorpedia/wikipedia"
	"gorm.io/plugin/dbresolver"
)

func (s *Server) StatisticsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get database counts
	var pageCount, embeddingCount, centroidCount int64
	s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Page{}).Count(&pageCount)
	s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Embedding{}).Count(&embeddingCount)
	s.db.WithContext(ctx).Clauses(dbresolver.Read).Model(&database.Centroid{}).Count(&centroidCount)

	stats := struct {
		Servers    ai.ClientStatistics          `json:"servers"`
		Processing wikipedia.ProcessingAverages `json:"processing,omitempty"`
		Database   struct {
			Pages      int64 `json:"pages"`
			Embeddings int64 `json:"embeddings"`
			Centroids  int64 `json:"centroids"`
		} `json:"database"`
	}{
		Servers:    s.ai.GetStatistics(ctx),
		Processing: s.wikipedia.GetAverages(),
	}

	stats.Database.Pages = pageCount
	stats.Database.Embeddings = embeddingCount
	stats.Database.Centroids = centroidCount

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		logger.Sugar().Errorf("Failed to encode statistics: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
