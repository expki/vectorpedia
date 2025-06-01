package main

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/cosnicolaou/pbzip2"

	"github.com/cespare/xxhash"
	"github.com/schollz/progressbar/v3"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"

	"github.com/expki/go-vectorsearch/database"
	"github.com/expki/go-vectorsearch/logger"
	vectorsearch "github.com/expki/go-vectorsearch/server"
	"github.com/expki/vectorpedia/server"
)

func NewWikipedia(db *database.Database) *wikipedia {
	return &wikipedia{
		db: db,
	}
}

type wikipedia struct {
	imported      map[uint64]struct{}
	db            *database.Database
	parallelQueue chan struct{}
}

func (w *wikipedia) Skip(title string) bool {
	key := xxhash.Sum64String(title)
	_, ok := w.imported[key]
	return ok
}

func (w *wikipedia) Record(title string) {
	w.imported[xxhash.Sum64String(title)] = struct{}{}
}

// Implementation for importing Wikipedia data
func ImportWikipedia(ctx context.Context, db *database.Database, srv *server.Server, filePath string, paralell int, show bool) {
	var writer io.Writer
	if show {
		writer = os.Stderr
	} else {
		writer = io.Discard
	}

	w := &wikipedia{
		db:            db,
		imported:      make(map[uint64]struct{}),
		parallelQueue: make(chan struct{}, paralell),
	}

	// check current
	barSkip := progressbar.NewOptions64(
		-1,
		progressbar.OptionSetDescription("Populating already added hashmap"),
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
	var documents []database.Document
	db.DB.WithContext(ctx).Clauses(dbresolver.Read).Select("id", "name").FindInBatches(&documents, 1000, func(tx *gorm.DB, batch int) error {
		for _, document := range documents {
			w.Record(document.Name)
		}
		barSkip.Add(len(documents))
		return nil
	})
	barSkip.Finish()

	srv.SetCentroidCountCache(false)
	srv.SetDocumentCountCache(false)
	srv.SetEmbeddingCountCache(false)
	defer func() {
		_, _, _, err := srv.Count()
		if err != nil {
			logger.Sugar().Errorf("Failed to do count: %v", err)
		} else {
			srv.SetCentroidCountCache(true)
			srv.SetDocumentCountCache(true)
			srv.SetEmbeddingCountCache(true)
		}
	}()
	fmt.Println("Importing Wikipedia data from:", filePath)

	// Interrupt signal
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	// Open the bz2 file.
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0644)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer file.Close()

	// Read statistics
	stats, err := file.Stat()
	if err != nil {
		log.Fatalf("Error reading file stats: %v", err)
	}

	// Create a progress bar.

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

	// Create a bzip2 reader from the file.
	bz2Reader := pbzip2.NewReader(ctx, &barReader)

	// Create an XML decoder reading from the decompressed stream.
	decoder := xml.NewDecoder(bz2Reader)

	// Process the XML tokens
	for {
		select {
		case <-ctx.Done():
			log.Println("Context done. Stopping import.")
			return
		case <-interrupt:
			log.Println("Interrupt received. Stopping import.")
			return
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
				var page Page
				if err := decoder.DecodeElement(&page, &se); err != nil {
					fmt.Printf("Error decoding page: %v\n", err)
					continue
				}
				w.processPage(ctx, srv, page)
			}
		default:
			// continue
		}
	}
}

func (w *wikipedia) processPage(ctx context.Context, srv *server.Server, page Page) {
	title := strings.TrimSpace(page.Title)
	if title == "" {
		return
	}
	if w.Skip(title) {
		return
	}

	// filter garbage
	titleLower := strings.TrimSpace(strings.ToLower(page.Title))
	if strings.HasPrefix(titleLower, "file:") {
		return
	}
	if strings.HasPrefix(titleLower, "file talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "category:") {
		return
	}
	if strings.HasPrefix(titleLower, "category talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "user:") {
		return
	}
	if strings.HasPrefix(titleLower, "talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "user talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "wikipedia:") {
		return
	}
	if strings.HasPrefix(titleLower, "wikipedia talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "help:") {
		return
	}
	if strings.HasPrefix(titleLower, "help talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "module:") {
		return
	}
	if strings.HasPrefix(titleLower, "module talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "mediawiki:") {
		return
	}
	if strings.HasPrefix(titleLower, "mediawiki talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "draft:") {
		return
	}
	if strings.HasPrefix(titleLower, "draft talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "book:") {
		return
	}
	if strings.HasPrefix(titleLower, "book talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "timedtext:") {
		return
	}
	if strings.HasPrefix(titleLower, "timedtext talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "template:") {
		return
	}
	if strings.HasPrefix(titleLower, "template talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "portal:") {
		return
	}
	if strings.HasPrefix(titleLower, "portal talk:") {
		return
	}
	if strings.HasPrefix(titleLower, "special:") {
		return
	}
	if strings.HasPrefix(titleLower, "media:") {
		return
	}
	text := page.Revision.Text

	// Remove pages that simply list other pages
	if isDisambiguationPage(text) {
		return
	}

	// Clean the page text to remove non-text elements (basic example)
	cleanText := cleanWikiMarkup(text)
	if cleanText == "" {
		return
	}

	w.parallelQueue <- struct{}{}
	go func(title, link, text string) {
		reqCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)

		document := vectorsearch.DocumentUpload{
			Name:       title,
			ExternalID: link,
			Document:   text,
		}
		req := vectorsearch.UploadRequest{
			Owner:     "wikipedia",
			Category:  "wikipedia",
			Documents: []vectorsearch.DocumentUpload{document},
		}

		_, err := srv.VectorSearch().Upload(reqCtx, req)
		if err == nil {
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		} else {
			fmt.Printf("Failed to upload documents for title '%s': %v\n", title, err)
		}
		cancel()
		<-w.parallelQueue
	}(title, generateWikipediaURL(title), cleanText)
	w.Record(title)
}

var (
	urlRe       = regexp.MustCompile(`http[s]?://\S+`)
	tableRe     = regexp.MustCompile(`(?s)\{\|.*?\|\}`)
	htmlTableRe = regexp.MustCompile(`(?s)<table.*?>.*?</table>`)
	entityRe    = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
)

// cleanWikiMarkup removes some basic Wikipedia markup.
// For more robust cleaning, consider using a dedicated parser.
func cleanWikiMarkup(text string) string {
	// Remove internal wiki link brackets and template markers.
	replacements := []struct {
		old string
		new string
	}{
		{"[[", ""},
		{"]]", ""},
		{"{{", ""},
		{"}}", ""},
		{"<ref>", ""},
		{"</ref>", ""},
	}
	for _, r := range replacements {
		text = strings.ReplaceAll(text, r.old, r.new)
	}

	// Remove external URLs (http or https links)
	text = urlRe.ReplaceAllString(text, "")

	// Remove wikicode tables (e.g. starting with {| and ending with |})
	text = tableRe.ReplaceAllString(text, "")

	// Remove HTML tables if any (using non-greedy matching)
	text = htmlTableRe.ReplaceAllString(text, "")

	// Remove HTML entities such as &ndash; using a regular expression
	text = entityRe.ReplaceAllString(text, "")

	// Remove lines that likely contain non-text content (e.g., file references, table legends)
	text = removeNonTextLines(text)

	return strings.TrimSpace(text)
}

// removeNonTextLines filters out lines that are likely to be non-text content.
// It skips lines that start with "File:" (case-insensitive) or contain table formatting.
func removeNonTextLines(text string) string {
	lines := strings.Split(text, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Remove lines starting with "File:" (case-insensitive)
		if strings.HasPrefix(strings.ToLower(trimmed), "file:") {
			continue
		}
		// Remove lines that include "|legend|" (table legends)
		if strings.Contains(trimmed, "|legend|") {
			continue
		}
		// If the line contains more than one pipe and is not a section header, skip it.
		if strings.Count(trimmed, "|") > 1 && !strings.HasPrefix(trimmed, "==") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// Contributor represents the author of a revision.
type Contributor struct {
	Username string `xml:"username"`
	ID       int    `xml:"id"`
	IP       string `xml:"ip"` // For anonymous edits.
}

// Revision represents a single revision of a page.
type Revision struct {
	ID          int         `xml:"id"`
	Timestamp   string      `xml:"timestamp"`
	Comment     string      `xml:"comment"`
	Contributor Contributor `xml:"contributor"`
	Text        string      `xml:"text"`
}

// Page represents a Wikipedia page with full content.
type Page struct {
	Title        string    `xml:"title"`
	ID           int       `xml:"id"`
	Namespace    int       `xml:"ns"`
	Redirect     *Redirect `xml:"redirect"`
	Restrictions string    `xml:"restrictions"`
	Revision     Revision  `xml:"revision"`
	//Revisions    []Revision `xml:"revision"`
}

// Redirect represents a redirect page.
type Redirect struct {
	Title string `xml:"title,attr"`
}

// generateWikipediaURL creates a valid Wikipedia URL from a page title.
func generateWikipediaURL(title string) string {
	escapedTitle := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	return "https://en.wikipedia.org/wiki/" + escapedTitle
}

func isDisambiguationPage(text string) bool {
	lowerText := strings.ToLower(text)
	return strings.Contains(lowerText, "[[category:disambiguation") ||
		strings.Contains(lowerText, "{{disambiguation") ||
		strings.Contains(lowerText, "{{disambig")
}
