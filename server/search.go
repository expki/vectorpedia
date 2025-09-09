package server

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/compute"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/logger"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type SearchRequest struct {
	Query     string          `json:"query"`
	Locations SearchLocations `json:"locations"`
	Rerank    bool            `json:"rerank"`
	Limit     int             `json:"limit,omitempty"`
}

type SearchLocations struct {
	Title   bool `json:"title"`
	Summary bool `json:"summary"`
	Content bool `json:"content"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Count   int            `json:"count"`
}

type SearchResult struct {
	PageID  uint64  `json:"page_id"`
	Uri     string  `json:"uri"`
	Title   string  `json:"title"`
	Summary string  `json:"summary,omitempty"`
	Score   float64 `json:"score"`
	Source  string  `json:"source"`
}

func (s *Server) SearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Sugar().Errorf("Failed to decode search request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	if !req.Locations.Title && !req.Locations.Summary && !req.Locations.Content {
		req.Locations.Title = true
	}

	if req.Limit == 0 {
		req.Limit = 10
	} else if req.Limit > 20 {
		req.Limit = 20
	}

	backend := s.ai.SelectServer()
	if backend == nil {
		logger.Sugar().Error("No AI backend available")
		http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
		return
	}

	embedReq := &ai.EmbedRequest{
		Model: "embed",
		Input: []string{
			fmt.Sprintf("task: search result | query: %s", req.Query),
		},
	}

	embedResp, err := backend.Embed(ctx, embedReq)
	if err != nil {
		logger.Sugar().Errorf("Failed to generate query embedding: %v", err)
		http.Error(w, "Failed to process query", http.StatusInternalServerError)
		return
	}

	if len(embedResp.Data) == 0 || len(embedResp.Data[0].Embedding) == 0 {
		logger.Sugar().Error("No embedding returned from AI backend")
		http.Error(w, "Failed to process query", http.StatusInternalServerError)
		return
	}

	queryVector := embedResp.Data[0].Embedding

	var sourcesToSearch []database.EmbeddingSource
	if req.Locations.Title {
		sourcesToSearch = append(sourcesToSearch, database.EmbeddingSource_Title)
	}
	if req.Locations.Summary {
		sourcesToSearch = append(sourcesToSearch, database.EmbeddingSource_Summary)
	}
	if req.Locations.Content {
		sourcesToSearch = append(sourcesToSearch, database.EmbeddingSource_Content)
	}

	results, err := s.searchEmbeddings(ctx, queryVector, sourcesToSearch, req.Limit*2)
	if err != nil {
		logger.Sugar().Errorf("Failed to search embeddings: %v", err)
		http.Error(w, "Search failed", http.StatusInternalServerError)
		return
	}

	if req.Rerank && len(results) > 0 {
		results, err = s.rerankResults(ctx, req.Query, results, req.Limit)
		if err != nil {
			logger.Sugar().Warnf("Reranking failed, returning original results: %v", err)
		}
		// Final sort of all results (descending by score)
		slices.SortFunc(results, func(a, b SearchResult) int {
			return cmp.Compare(b.Score, a.Score)
		})
	} else if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	response := SearchResponse{
		Results: results,
		Count:   len(results),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.Sugar().Errorf("Failed to encode search response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *Server) searchEmbeddings(ctx context.Context, queryVector []float64, sources []database.EmbeddingSource, limit int) ([]SearchResult, error) {
	// First, find all centroids in the database
	var centroids []database.Centroid
	err := s.db.WithContext(ctx).Clauses(dbresolver.Read).Find(&centroids).Error
	if err != nil {
		return nil, err
	}

	if len(centroids) == 0 {
		// No centroids available, cannot perform search
		return []SearchResult{}, nil
	}

	// Find the closest centroids to the query vector
	type centroidSimilarity struct {
		centroid   *database.Centroid
		similarity float64
	}

	queryVec := compute.NewVector(queryVector)
	closestCentroids := make([]centroidSimilarity, 0, len(centroids))

	// Convert centroids to matrix for batch similarity calculation
	centroidMatrix := make([][]float64, len(centroids))
	for i, centroid := range centroids {
		centroidMatrix[i] = compute.DequantizeVector(centroid.Vector)
	}

	// Calculate similarity to all centroids
	similarities := queryVec.Clone().MatrixCosineSimilarity(compute.NewMatrix(centroidMatrix))
	for i, similarity := range similarities {
		closestCentroids = append(closestCentroids, centroidSimilarity{
			centroid:   &centroids[i],
			similarity: similarity,
		})
	}

	// Sort centroids by similarity (descending)
	slices.SortFunc(closestCentroids, func(a, b centroidSimilarity) int {
		return cmp.Compare(b.similarity, a.similarity)
	})

	// Use top centroids (e.g., top 3 or all if fewer)
	numCentroids := min(3, len(closestCentroids))
	if numCentroids == 0 {
		return []SearchResult{}, nil
	}

	topCentroidIDs := make([]uint64, numCentroids)
	for i := 0; i < numCentroids; i++ {
		topCentroidIDs[i] = closestCentroids[i].centroid.ID
	}

	// Search within the top centroids
	type scoredEmbedding struct {
		embedding *database.Embedding
		score     float64
	}

	batchSize := 1000
	scoredResults := make([]scoredEmbedding, 0, limit+batchSize)

	// Query embeddings in batches from the selected centroids
	var embeddings []database.Embedding
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).
		Where("source IN ? AND centroid_id IN ?", sources, topCentroidIDs).
		FindInBatches(&embeddings, batchSize, func(tx *gorm.DB, batch int) error {
			// Convert batch to matrix for efficient similarity calculation
			embeddingMatrix := make([][]float64, len(embeddings))
			for i, embedding := range embeddings {
				embeddingMatrix[i] = compute.DequantizeVector(embedding.Vector)
			}

			// Calculate similarities for entire batch at once
			similarities := queryVec.Clone().MatrixCosineSimilarity(compute.NewMatrix(embeddingMatrix))

			// Process results
			for i, similarity := range similarities {
				scoredResults = append(scoredResults, scoredEmbedding{
					embedding: &embeddings[i],
					score:     similarity,
				})
			}

			// Sort current results (descending by score)
			slices.SortFunc(scoredResults, func(a, b scoredEmbedding) int {
				return cmp.Compare(b.score, a.score)
			})
			// Keep top results
			if len(scoredResults) > limit {
				scoredResults = scoredResults[:limit]
			}

			return nil
		}).Error

	if err != nil {
		return nil, err
	}

	// Final sort of all results (descending by score)
	slices.SortFunc(scoredResults, func(a, b scoredEmbedding) int {
		return cmp.Compare(b.score, a.score)
	})

	if len(scoredResults) > limit {
		scoredResults = scoredResults[:limit]
	}

	// Build a map to track highest scoring embedding per page
	type pageScore struct {
		pageID    uint64
		embedding *database.Embedding
		score     float64
		source    database.EmbeddingSource
	}

	pageScoreMap := make(map[uint64]*pageScore)

	// Collect embedding IDs by source type for batch queries
	titleEmbeddingIDs := make([]uint64, 0)
	summaryEmbeddingIDs := make([]uint64, 0)
	contentEmbeddingIDs := make([]uint64, 0)
	embeddingScoreMap := make(map[uint64]float64)

	for _, scored := range scoredResults {
		embeddingScoreMap[scored.embedding.ID] = scored.score
		switch scored.embedding.Source {
		case database.EmbeddingSource_Title:
			titleEmbeddingIDs = append(titleEmbeddingIDs, scored.embedding.ID)
		case database.EmbeddingSource_Summary:
			summaryEmbeddingIDs = append(summaryEmbeddingIDs, scored.embedding.ID)
		case database.EmbeddingSource_Content:
			contentEmbeddingIDs = append(contentEmbeddingIDs, scored.embedding.ID)
		}
	}

	// Batch query for title embeddings with joined pages
	if len(titleEmbeddingIDs) > 0 {
		type TitlePageResult struct {
			EmbeddingID uint64
			PageID      uint64
		}
		var titleResults []TitlePageResult
		err = s.db.WithContext(ctx).Clauses(dbresolver.Read).
			Table("titles").
			Select("titles.embedding_id, pages.id as page_id").
			Joins("INNER JOIN pages ON pages.title_id = titles.id").
			Where("titles.embedding_id IN ?", titleEmbeddingIDs).
			Scan(&titleResults).Error
		if err != nil {
			logger.Sugar().Errorf("Failed to fetch title pages: %v", err)
		} else {
			for _, result := range titleResults {
				if score, exists := embeddingScoreMap[result.EmbeddingID]; exists {
					if existing, pageExists := pageScoreMap[result.PageID]; !pageExists || score > existing.score {
						pageScoreMap[result.PageID] = &pageScore{
							pageID: result.PageID,
							score:  score,
							source: database.EmbeddingSource_Title,
						}
					}
				}
			}
		}
	}

	// Batch query for summary embeddings with joined pages
	if len(summaryEmbeddingIDs) > 0 {
		type SummaryPageResult struct {
			EmbeddingID uint64
			PageID      uint64
		}
		var summaryResults []SummaryPageResult
		err = s.db.WithContext(ctx).Clauses(dbresolver.Read).
			Table("summaries").
			Select("summaries.embedding_id, pages.id as page_id").
			Joins("INNER JOIN pages ON pages.summary_id = summaries.id").
			Where("summaries.embedding_id IN ?", summaryEmbeddingIDs).
			Scan(&summaryResults).Error
		if err != nil {
			logger.Sugar().Errorf("Failed to fetch summary pages: %v", err)
		} else {
			for _, result := range summaryResults {
				if score, exists := embeddingScoreMap[result.EmbeddingID]; exists {
					if existing, pageExists := pageScoreMap[result.PageID]; !pageExists || score > existing.score {
						pageScoreMap[result.PageID] = &pageScore{
							pageID: result.PageID,
							score:  score,
							source: database.EmbeddingSource_Summary,
						}
					}
				}
			}
		}
	}

	// Batch query for content embeddings with joined pages
	if len(contentEmbeddingIDs) > 0 {
		type ContentPageResult struct {
			EmbeddingID uint64
			PageID      uint64
		}
		var contentResults []ContentPageResult
		err = s.db.WithContext(ctx).Clauses(dbresolver.Read).
			Table("content_embeddings").
			Select("content_embeddings.embedding_id, pages.id as page_id").
			Joins("INNER JOIN pages ON pages.content_id = content_embeddings.content_id").
			Where("content_embeddings.embedding_id IN ?", contentEmbeddingIDs).
			Scan(&contentResults).Error
		if err != nil {
			logger.Sugar().Errorf("Failed to fetch content pages: %v", err)
		} else {
			for _, result := range contentResults {
				if score, exists := embeddingScoreMap[result.EmbeddingID]; exists {
					if existing, pageExists := pageScoreMap[result.PageID]; !pageExists || score > existing.score {
						pageScoreMap[result.PageID] = &pageScore{
							pageID: result.PageID,
							score:  score,
							source: database.EmbeddingSource_Content,
						}
					}
				}
			}
		}
	}

	// Convert map to slice for sorting
	uniquePages := make([]*pageScore, 0, len(pageScoreMap))
	for _, ps := range pageScoreMap {
		uniquePages = append(uniquePages, ps)
	}

	// Sort by score descending
	slices.SortFunc(uniquePages, func(a, b *pageScore) int {
		return cmp.Compare(b.score, a.score)
	})

	// Limit results if needed
	if len(uniquePages) > limit {
		uniquePages = uniquePages[:limit]
	}

	// Collect page IDs for batch fetch
	pageIDs := make([]uint64, len(uniquePages))
	for i, ps := range uniquePages {
		pageIDs[i] = ps.pageID
	}

	// Batch fetch all pages with their titles and summaries
	var pages []database.Page
	err = s.db.WithContext(ctx).Clauses(dbresolver.Read).
		Preload("Title").
		Preload("Summary").
		Where("id IN ?", pageIDs).
		Find(&pages).Error
	if err != nil {
		return nil, err
	}

	// Create a map for quick page lookup
	pageMap := make(map[uint64]*database.Page)
	for i := range pages {
		pageMap[pages[i].ID] = &pages[i]
	}

	// Build final results maintaining score order
	results := make([]SearchResult, 0, len(uniquePages))
	for _, ps := range uniquePages {
		page, exists := pageMap[ps.pageID]
		if !exists {
			continue
		}

		result := SearchResult{
			PageID: page.ID,
			Uri:    page.Uri,
			Score:  ps.score,
			Source: getSourceName(ps.source),
		}

		if page.Title != nil {
			result.Title = page.Title.Text
		}
		if page.Summary != nil {
			result.Summary = page.Summary.Text
		}

		results = append(results, result)
	}

	return results, nil
}

func (s *Server) rerankResults(ctx context.Context, query string, results []SearchResult, limit int) ([]SearchResult, error) {
	if len(results) == 0 {
		return results, nil
	}

	backend := s.ai.SelectServer()
	if backend == nil {
		return results, nil
	}

	documents := make([]string, len(results))
	for i, result := range results {
		documents[i] = result.Title
	}

	rerankReq := &ai.RerankRequest{
		Model:     "rerank",
		Query:     query,
		Documents: documents,
		TopN:      limit,
	}

	rerankResp, err := backend.Rerank(ctx, rerankReq)
	if err != nil {
		return results, err
	}

	rerankedResults := make([]SearchResult, 0, len(rerankResp.Results))
	for _, rerankResult := range rerankResp.Results {
		if rerankResult.Index >= 0 && rerankResult.Index < len(results) {
			result := results[rerankResult.Index]
			result.Score = float64(rerankResult.Score)
			rerankedResults = append(rerankedResults, result)
		}
	}

	return rerankedResults, nil
}

func getSourceName(source database.EmbeddingSource) string {
	switch source {
	case database.EmbeddingSource_Title:
		return "title"
	case database.EmbeddingSource_Summary:
		return "summary"
	case database.EmbeddingSource_Content:
		return "content"
	default:
		return "unknown"
	}
}
