package wikipedia

import (
	"context"
	"fmt"
	"strings"
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
	summary, err := w.GenerateSummary(reqCtx, title, cleanText)
	if err != nil {
		return fmt.Errorf("failed to generate summary for '%s': %w", title, err)
	}

	// Generate embeddings for title
	titleEmbedding, err := w.GenerateEmbedding(reqCtx, title)
	if err != nil {
		return fmt.Errorf("failed to generate title embedding for '%s': %w", title, err)
	}

	// Generate embedding for summary
	summaryEmbedding, err := w.GenerateEmbedding(reqCtx, summary)
	if err != nil {
		return fmt.Errorf("failed to generate summary embedding for '%s': %w", title, err)
	}

	// Chunk content and generate embeddings
	chunks := chunkContent(cleanText, w.contextSizeEmbed)
	contentEmbeddings := make([]*database.Embedding, 0, len(chunks))

	for _, chunk := range chunks {
		embedding, err := w.GenerateEmbedding(reqCtx, chunk)
		if err != nil {
			fmt.Printf("Failed to generate content embedding for '%s': %v\n", title, err)
			continue
		}
		contentEmbeddings = append(contentEmbeddings, &database.Embedding{
			Vector:    embedding,
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
	if err := w.InsertPage(reqCtx, dbPage); err != nil {
		return fmt.Errorf("failed to insert page '%s': %w", title, err)
	}

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
func chunkContent(content string, ctxEmbed uint) []string {
	var chunks []string
	contentLen := len(content)

	if contentLen <= int(ctxEmbed) {
		return []string{content}
	}

	for start := 0; start < contentLen; {
		end := start + int(ctxEmbed)
		if end > contentLen {
			end = contentLen
		}

		// Try to break at a sentence or paragraph boundary
		if end < contentLen {
			// Look for sentence endings
			lastPeriod := strings.LastIndex(content[start:end], ". ")
			lastNewline := strings.LastIndex(content[start:end], "\n")

			breakPoint := -1
			if lastNewline > 0 && lastNewline > end-200 {
				breakPoint = lastNewline
			} else if lastPeriod > 0 && lastPeriod > end-200 {
				breakPoint = lastPeriod + 1
			}

			if breakPoint > 0 {
				end = start + breakPoint
			}
		}

		chunks = append(chunks, strings.TrimSpace(content[start:end]))

		// Move start with overlap
		start = end - int(ctxEmbed/20)
		if start < 0 {
			start = 0
		}
	}

	return chunks
}
