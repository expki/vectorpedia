package server

import (
	"encoding/json"
	"net/http"

	"github.com/expki/vectorpedia/logger"
)

// SummaryStatsHandler returns only the cached database counts
// This endpoint is optimized for fast response and is used by the UI home page
func (s *Server) SummaryStatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return cached statistics immediately
	stats := s.GetCachedStats()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		logger.Sugar().Errorf("Failed to encode summary stats: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}