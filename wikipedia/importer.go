package wikipedia

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cosnicolaou/pbzip2"
	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/database"
	"github.com/expki/vectorpedia/logger"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

// New creates a new Wikipedia importer instance
func New(db *database.Database, client ai.Client, ctxChat, ctxEmbed, ctxRerank uint, providers int) *Wikipedia {
	return &Wikipedia{
		db:                db,
		client:            client,
		imported:          make(map[uint64]struct{}),
		contextSizeChat:   ctxChat,
		contextSizeEmbed:  ctxEmbed,
		contextSizeRerank: ctxRerank,
		concurrent:        make(chan struct{}, providers*10),
	}
}

// ImportFromFile imports Wikipedia data from a compressed XML file
func (w *Wikipedia) ImportFromFile(ctx context.Context, filePath string) error {
	var writer io.Writer = os.Stderr

	// Check existing pages
	if err := w.loadExistingPages(ctx, writer); err != nil {
		return fmt.Errorf("failed to load existing pages: %w", err)
	}

	fmt.Println("Importing Wikipedia data from:", filePath)

	// Record import start time
	w.metricsLock.Lock()
	w.metrics.ImportStartTime = time.Now()
	w.metricsLock.Unlock()

	// Interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Open the bz2 file
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	// Read statistics
	stats, err := file.Stat()
	if err != nil {
		return fmt.Errorf("error reading file stats: %w", err)
	}

	// Create a progress bar
	bar := progressbar.NewOptions64(
		stats.Size(),
		progressbar.OptionSetDescription("Importing Wikipedia data"),
		progressbar.OptionSetWriter(writer),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowTotalBytes(true),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(writer, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)
	barReader := progressbar.NewReader(file, bar)

	// Create a bzip2 reader from the progress bar reader
	bz2Reader := pbzip2.NewReader(ctx, &barReader)

	// Create an XML decoder reading from the decompressed stream
	decoder := xml.NewDecoder(bz2Reader)

	// Process the XML tokens
	for {
		select {
		case <-ctx.Done():
			log.Println("Context done. Stopping import.")
			return ctx.Err()
		case <-interrupt:
			log.Println("Interrupt received. Stopping import.")
			return fmt.Errorf("interrupted")
		default:
			// continue
		}

		tok, err := decoder.Token()
		if err == io.EOF {
			break // reached end of file
		}
		if err != nil {
			fmt.Printf("Token error: %v\n", err)
			continue
		}

		// Look for <page> start elements
		switch se := tok.(type) {
		case xml.StartElement:
			if se.Name.Local == "page" {
				page := &Page{}
				if err := decoder.DecodeElement(page, &se); err != nil {
					logger.Sugar().Errorf("Error decoding page: %v\n", err)
					continue
				}
				w.concurrent <- struct{}{}
				go func() {
					if err := w.ProcessPage(ctx, page); err != nil {
						logger.Sugar().Errorf("Error processing page: %v\n", err)
					}
					<-w.concurrent
				}()

			}
		default:
			// continue
		}
	}

	bar.Finish()
	fmt.Println("Wikipedia import completed")
	return nil
}

// loadExistingPages loads already imported pages into memory
func (w *Wikipedia) loadExistingPages(ctx context.Context, writer io.Writer) error {
	barSkip := progressbar.NewOptions64(
		-1,
		progressbar.OptionSetDescription("Checking existing pages"),
		progressbar.OptionSetWriter(writer),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowTotalBytes(true),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(writer, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)

	var pageTitles []database.Title
	err := w.db.WithContext(ctx).Clauses(dbresolver.Read).FindInBatches(&pageTitles, 1000, func(tx *gorm.DB, batch int) error {
		for _, title := range pageTitles {
			w.Record(title.Text)
		}
		barSkip.Add(len(pageTitles))
		return nil
	}).Error

	barSkip.Finish()
	return err
}
