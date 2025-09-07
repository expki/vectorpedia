package wikipedia

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cespare/xxhash"
	"github.com/expki/vectorpedia/database"
)

// Skip checks if a title has already been imported
func (w *Wikipedia) Skip(title string) bool {
	w.importedLock.RLock()
	defer w.importedLock.RUnlock()
	key := xxhash.Sum64String(title)
	_, ok := w.imported[key]
	return ok
}

// Record marks a title as imported
func (w *Wikipedia) Record(title string) {
	w.importedLock.Lock()
	defer w.importedLock.Unlock()
	w.imported[xxhash.Sum64String(title)] = struct{}{}
}

// ProcessPage processes a single Wikipedia page
func (w *Wikipedia) ProcessPage(ctx context.Context, page *Page) error {
	title := strings.TrimSpace(page.Title)
	if title == "" {
		return nil
	}
	if w.Skip(title) {
		return nil
	}

	// Filter garbage pages
	if shouldSkipPage(title) {
		return nil
	}

	text := page.Revision.Text

	// Remove pages that simply list other pages
	if isDisambiguationPage(text) {
		return nil
	}

	// Clean the page text to remove non-text elements
	cleanText := cleanWikiMarkup(text)
	if cleanText == "" {
		return nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Generate summary using AI
	summaryStart := time.Now()
	summary, err := w.GenerateSummary(reqCtx, title, cleanText)
	summaryDuration := time.Since(summaryStart).Nanoseconds()
	w.updateMetrics(summaryDuration, MetricType_Summary)
	if err != nil {
		return fmt.Errorf("failed to generate summary for '%s': %w", title, err)
	}

	// Chunk content for embeddings
	chunks := chunkContent(cleanText, w.contextSizeEmbed)

	// Prepare all texts for batch embedding
	embeddingTexts := make([]string, 0, 2+len(chunks))
	embeddingTexts = append(embeddingTexts, title)     // Index 0: title
	embeddingTexts = append(embeddingTexts, summary)   // Index 1: summary
	embeddingTexts = append(embeddingTexts, chunks...) // Index 2+: content chunks

	// Generate all embeddings in a single batch request
	embedStart := time.Now()
	embeddings, err := w.GenerateEmbedding(reqCtx, embeddingTexts)
	embedDuration := time.Since(embedStart).Nanoseconds()
	w.updateMetrics(embedDuration, MetricType_Embedding)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings for '%s': %w", title, err)
	}

	// Validate we got the expected number of embeddings
	if len(embeddings) != len(embeddingTexts) {
		return fmt.Errorf("embedding count mismatch for '%s': expected %d, got %d", title, len(embeddingTexts), len(embeddings))
	}

	// Extract embeddings by type
	titleEmbedding := embeddings[0]
	summaryEmbedding := embeddings[1]

	// Build content embeddings from the remaining embeddings
	contentEmbeddings := make([]*database.Embedding, 0, len(chunks))
	for i := 2; i < len(embeddings); i++ {
		contentEmbeddings = append(contentEmbeddings, &database.Embedding{
			Vector:    embeddings[i],
			IsContent: true,
		})
	}

	// Create database models
	dbPage := &database.Page{
		Title: &database.Title{
			Text: title,
			Embedding: &database.Embedding{
				Vector:  titleEmbedding,
				IsTitle: true,
			},
		},
		Content: &database.Content{
			Text:       cleanText,
			Embeddings: contentEmbeddings,
		},
		Summary: &database.Summary{
			Text: summary,
			Embedding: &database.Embedding{
				Vector:    summaryEmbedding,
				IsSummary: true,
			},
		},
	}

	// Insert page directly into database
	insertStart := time.Now()
	if err := w.InsertPage(reqCtx, dbPage); err != nil {
		return fmt.Errorf("failed to insert page '%s': %w", title, err)
	}
	insertDuration := time.Since(insertStart).Nanoseconds()
	w.updateMetrics(insertDuration, MetricType_Insert)

	w.Record(title)
	return nil
}

// shouldSkipPage checks if a page should be skipped based on its title
func shouldSkipPage(title string) bool {
	titleLower := strings.ToLower(strings.TrimSpace(title))
	prefixesToSkip := []string{
		"file:", "file talk:", "category:", "category talk:",
		"user:", "talk:", "user talk:", "wikipedia:", "wikipedia talk:",
		"help:", "help talk:", "module:", "module talk:",
		"mediawiki:", "mediawiki talk:", "draft:", "draft talk:",
		"book:", "book talk:", "timedtext:", "timedtext talk:",
		"template:", "template talk:", "portal:", "portal talk:",
		"special:", "media:",
	}

	for _, prefix := range prefixesToSkip {
		if strings.HasPrefix(titleLower, prefix) {
			return true
		}
	}
	return false
}

// chunkContent splits content into overlapping chunks
func chunkContent(content string, ctxTokensEmbed uint) []string {
	var chunks []string

	// Calculate chunk size in characters based on token limit
	chunkSize := estimateContentLength(int(ctxTokensEmbed))
	contentLen := len(content)

	// If content fits in one chunk, return as is
	if contentLen <= chunkSize {
		return []string{content}
	}

	// Calculate overlap (5% of chunk size, capped at 256 characters)
	overlap := chunkSize / 20
	if overlap > 256 {
		overlap = 256
	}

	// Split content into chunks with simple cutting
	for start := 0; start < contentLen; {
		end := start + chunkSize
		if end > contentLen {
			end = contentLen
		}

		// Extract chunk and trim whitespace
		chunk := strings.TrimSpace(content[start:end])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		// If we've reached the end, break
		if end >= contentLen {
			break
		}

		// Move to next chunk with overlap
		start = end - overlap
	}

	return chunks
}

// updateMetrics updates processing metrics in a thread-safe manner
func (w *Wikipedia) updateMetrics(duration int64, metricType MetricType) {
	w.metricsLock.Lock()
	defer w.metricsLock.Unlock()

	switch metricType {
	case MetricType_Embedding:
		atomic.AddInt64(&w.metrics.EmbeddingTimeTotal, duration)
		atomic.AddInt64(&w.metrics.EmbeddingCount, 1)
	case MetricType_Summary:
		atomic.AddInt64(&w.metrics.SummaryTimeTotal, duration)
		atomic.AddInt64(&w.metrics.SummaryCount, 1)
	case MetricType_Insert:
		atomic.AddInt64(&w.metrics.InsertTimeTotal, duration)
		atomic.AddInt64(&w.metrics.InsertCount, 1)
	}
}

// GetMetrics returns processing metrics
func (w *Wikipedia) GetMetrics() ProcessingMetrics {
	w.metricsLock.RLock()
	defer w.metricsLock.RUnlock()

	return ProcessingMetrics{
		EmbeddingTimeTotal: atomic.LoadInt64(&w.metrics.EmbeddingTimeTotal),
		EmbeddingCount:     atomic.LoadInt64(&w.metrics.EmbeddingCount),
		SummaryTimeTotal:   atomic.LoadInt64(&w.metrics.SummaryTimeTotal),
		SummaryCount:       atomic.LoadInt64(&w.metrics.SummaryCount),
		InsertTimeTotal:    atomic.LoadInt64(&w.metrics.InsertTimeTotal),
		InsertCount:        atomic.LoadInt64(&w.metrics.InsertCount),
	}
}

type MetricType uint8

const (
	MetricType_Embedding MetricType = iota
	MetricType_Summary
	MetricType_Insert
)
