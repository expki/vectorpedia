package wikipedia

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/expki/vectorpedia/config"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/logger"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// ReembedEmbeddings regenerates embeddings for all titles and summaries
func (w *Wikipedia) ReembedEmbeddings(ctx context.Context) error {
	var writer = os.Stderr

	logger.Sugar().Info("Starting reembedding process...")

	// Process pages (titles and summaries together)
	if err := w.reembedPages(ctx, writer); err != nil {
		return fmt.Errorf("failed to reembed pages: %w", err)
	}

	logger.Sugar().Info("Reembedding process completed successfully")
	return nil
}

// reembedPages processes all pages and regenerates embeddings for their titles and summaries
func (w *Wikipedia) reembedPages(ctx context.Context, writer *os.File) error {
	// Count total pages
	var totalCount int64
	if err := w.db.WithContext(ctx).Model(&database.Page{}).Count(&totalCount).Error; err != nil {
		return fmt.Errorf("failed to count pages: %w", err)
	}

	logger.Sugar().Infof("Processing %d pages (titles and summaries)...", totalCount)

	// Create progress bar for pages (each page has both title and summary)
	bar := progressbar.NewOptions64(
		totalCount*2, // Each page has both title and summary
		progressbar.OptionSetDescription("Reembedding Titles and Summaries"),
		progressbar.OptionSetWriter(writer),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(writer, "\n")
		}),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// Process pages in batches
	var pages []database.Page
	batchSize := config.BATCH_SIZE_DATABASE

	return w.db.WithContext(ctx).
		Clauses(dbresolver.Read).
		Preload("Title.Embedding", func(db *gorm.DB) *gorm.DB {
			// Only load the ID, not the vector data
			return db.Select("id")
		}).
		Preload("Title").
		Preload("Summary.Embedding", func(db *gorm.DB) *gorm.DB {
			// Only load the ID, not the vector data
			return db.Select("id")
		}).
		Preload("Summary").
		FindInBatches(&pages, batchSize, func(tx *gorm.DB, batch int) error {
			transportPages := make([]*transportPage, 0, len(pages))
			for _, page := range pages {
				if page.Title == nil || page.Summary == nil {
					continue
				}
				var titleEmbeddingID uint64
				var title string = page.Title.Text
				if page.Title.Embedding != nil {
					titleEmbeddingID = page.Title.Embedding.ID
				} else {
					titleEmbeddingID = page.Title.EmbeddingID
				}
				var summaryEmbeddingID uint64
				var summary string = page.Summary.Text
				if page.Summary.Embedding != nil {
					summaryEmbeddingID = page.Summary.Embedding.ID
				} else {
					summaryEmbeddingID = page.Summary.EmbeddingID
				}
				transportPages = append(transportPages, &transportPage{
					title:              title,
					summary:            summary,
					titleEmbeddingID:   titleEmbeddingID,
					summaryEmbeddingID: summaryEmbeddingID,
				})
			}

			// Process batch
			processLockChan <- struct{}{}
			go func(transportPages []*transportPage) {
				if err := w.processPageBatch(ctx, transportPages, bar); err != nil {
					logger.Sugar().Errorf("failed to process page batch %d: %w", batch, err)
				}
				<-processLockChan
			}(transportPages)

			return nil
		}).Error
}

type transportPage struct {
	title              string
	titleEmbeddingID   uint64
	summary            string
	summaryEmbeddingID uint64
}

// processPageBatch processes a batch of pages and regenerates embeddings for titles and summaries
func (w *Wikipedia) processPageBatch(ctx context.Context, pages []*transportPage, bar *progressbar.ProgressBar) error {
	// Prepare texts for embedding (titles and summaries interleaved)
	texts := make([]string, 0, len(pages)*2)
	embeddingIDs := make([]uint64, 0, len(pages)*2)

	for _, page := range pages {
		// Sanitize title for use in summary embedding (remove pipes)
		safeTitle := strings.ReplaceAll(page.title, "|", "")

		// Add title embedding text
		texts = append(texts, fmt.Sprintf("title: none | text: %s", page.title))
		embeddingIDs = append(embeddingIDs, page.titleEmbeddingID)

		// Add summary embedding text (with title context)
		texts = append(texts, fmt.Sprintf("title: %s | text: %s", safeTitle, page.summary))
		embeddingIDs = append(embeddingIDs, page.summaryEmbeddingID)
	}

	if len(texts) == 0 {
		return nil
	}

	// Generate embeddings
	embeddings, err := w.GenerateEmbedding(ctx, texts)
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	type unit struct {
		embeddings   [][]byte
		embeddingIDs []uint64
	}

	saveLockChan <- struct{}{}
	go func(item *unit) {
		// Update embeddings in database using a transaction for better performance
		err = w.db.WithContext(ctx).Clauses(dbresolver.Write).Transaction(func(tx *gorm.DB) error {
			for i, embedding := range embeddings {
				// Update each embedding's vector column
				if err := tx.Model(&database.Embedding{}).
					Where("id = ?", embeddingIDs[i]).
					Update("vector", embedding).Error; err != nil {
					return fmt.Errorf("failed to update embedding %d: %w", embeddingIDs[i], err)
				}
				bar.Add(1)
			}
			return nil
		})
		if err != nil {
			logger.Sugar().Errorf("Failed to update embeddings: %v", err)
		}
		<-saveLockChan
	}(&unit{
		embeddings:   embeddings,
		embeddingIDs: embeddingIDs,
	})

	return nil
}

var (
	processLockChan chan struct{} = make(chan struct{}, 8)
	saveLockChan    chan struct{} = make(chan struct{}, 8)
)
