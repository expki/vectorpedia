package wikipedia

import (
	"context"
	"fmt"

	"github.com/expki/vectorpedia/database"
	"gorm.io/plugin/dbresolver"
)

// InsertPage inserts a single page into the database
func (w *Wikipedia) InsertPage(ctx context.Context, page *database.Page) error {
	return w.insertSinglePage(ctx, page)
}

// insertSinglePage inserts a single page into the database
func (w *Wikipedia) insertSinglePage(ctx context.Context, page *database.Page) error {

	// Insert embeddings
	embeddings := make([]*database.Embedding, 0, 2+len(page.Content.Embeddings))
	embeddings = append(embeddings, page.Title.Embedding)
	embeddings = append(embeddings, page.Summary.Embedding)
	embeddings = append(embeddings, page.Content.Embeddings...)
	err := w.db.WithContext(ctx).Clauses(dbresolver.Write).Create(&embeddings).Error
	if err != nil {
		return fmt.Errorf("failed to insert embeddings: %w", err)
	}

	// Insert title
	err = w.db.WithContext(ctx).Clauses(dbresolver.Write).Create(page.Title).Error
	if err != nil {
		return fmt.Errorf("failed to insert title: %w", err)
	}

	// Insert content
	err = w.db.WithContext(ctx).Clauses(dbresolver.Write).Create(page.Content).Error
	if err != nil {
		return fmt.Errorf("failed to insert content: %w", err)
	}

	// Insert summary
	err = w.db.WithContext(ctx).Clauses(dbresolver.Write).Create(page.Summary).Error
	if err != nil {
		return fmt.Errorf("failed to insert summary: %w", err)
	}

	// Insert page
	err = w.db.WithContext(ctx).Clauses(dbresolver.Write).Create(page).Error
	if err != nil {
		return fmt.Errorf("failed to insert page: %w", err)
	}

	return nil
}
