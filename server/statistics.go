package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/logger"
	"github.com/expki/vectorpedia/wikipedia"
)

func (s *Server) StatisticsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get cached database statistics
	cachedStats := s.GetCachedStats()

	stats := struct {
		Servers    ai.ClientStatistics          `json:"servers"`
		Processing wikipedia.ProcessingAverages `json:"processing,omitempty"`
		Database   DatabaseStats                `json:"database"`
	}{
		Servers:    s.ai.GetStatistics(ctx),
		Processing: s.wikipedia.GetAverages(),
		Database:   cachedStats,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		logger.Sugar().Errorf("Failed to encode statistics: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
