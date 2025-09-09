package wikipedia

import (
	_ "encoding/xml"
	"sync"
	"time"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/database"
)

// ProcessingMetrics holds timing statistics for page processing
type ProcessingMetrics struct {
	EmbeddingTimeTotal       int64 // Total time in nanoseconds
	EmbeddingCount           int64
	SummaryTimeTotal         int64 // Total time in nanoseconds
	SummaryCount             int64
	InsertTimeTotal          int64 // Total time in nanoseconds
	InsertCount              int64
	ProcessPageTimeTotal     int64 // Total time for ProcessPage in nanoseconds
	ProcessPageCount         int64
	TokenizeChatTimeTotal    int64 // Total time for chat tokenization in nanoseconds
	TokenizeChatCount        int64
	TokenizeEmbedTimeTotal   int64 // Total time for embed tokenization in nanoseconds
	TokenizeEmbedCount       int64
	DetokenizeChatTimeTotal  int64 // Total time for chat detokenization in nanoseconds
	DetokenizeChatCount      int64
	DetokenizeEmbedTimeTotal int64 // Total time for embed detokenization in nanoseconds
	DetokenizeEmbedCount     int64
	ImportStartTime          time.Time // Start time of import
	PagesPerMinute           float64   // Average pages processed per minute
}

// ProcessingAverages holds average timing statistics
type ProcessingAverages struct {
	AverageEmbeddingTimeMs       float64 `json:"average_embedding_time_ms"`
	AverageSummaryTimeMs         float64 `json:"average_summary_time_ms"`
	AverageInsertTimeMs          float64 `json:"average_insert_time_ms"`
	AverageProcessPageTimeMs     float64 `json:"average_process_page_time_ms"`
	AverageTokenizeChatTimeMs    float64 `json:"average_tokenize_chat_time_ms"`
	AverageTokenizeEmbedTimeMs   float64 `json:"average_tokenize_embed_time_ms"`
	AverageDetokenizeChatTimeMs  float64 `json:"average_detokenize_chat_time_ms"`
	AverageDetokenizeEmbedTimeMs float64 `json:"average_detokenize_embed_time_ms"`
	TotalEmbeddings              int64   `json:"total_embeddings"`
	TotalSummaries               int64   `json:"total_summaries"`
	TotalInserts                 int64   `json:"total_inserts"`
	TotalPagesProcessed          int64   `json:"total_pages_processed"`
	TotalTokenizeChat            int64   `json:"total_tokenize_chat"`
	TotalTokenizeEmbed           int64   `json:"total_tokenize_embed"`
	TotalDetokenizeChat          int64   `json:"total_detokenize_chat"`
	TotalDetokenizeEmbed         int64   `json:"total_detokenize_embed"`
	PagesPerMinute               float64 `json:"pages_per_minute"`
}

// Wikipedia manages the import process
type Wikipedia struct {
	importedLock      sync.RWMutex
	imported          map[uint64]struct{}
	db                *database.Database
	client            ai.Client
	contextSizeChat   uint
	contextSizeEmbed  uint
	contextSizeRerank uint
	concurrent        chan struct{}
	metrics           ProcessingMetrics
	metricsLock       sync.RWMutex

	// Duration slices (store up to 100 samples)
	embeddingDurations       []float64
	summaryDurations         []float64
	insertDurations          []float64
	processPageDurations     []float64
	tokenizeChatDurations    []float64
	tokenizeEmbedDurations   []float64
	detokenizeChatDurations  []float64
	detokenizeEmbedDurations []float64

	embedLockChan chan struct{}
}

// Page represents a Wikipedia page with full content
type Page struct {
	Title        string    `xml:"title"`
	ID           int       `xml:"id"`
	Namespace    int       `xml:"ns"`
	Redirect     *Redirect `xml:"redirect"`
	Restrictions string    `xml:"restrictions"`
	Revision     Revision  `xml:"revision"`
}

// Revision represents a single revision of a page
type Revision struct {
	ID          int         `xml:"id"`
	Timestamp   string      `xml:"timestamp"`
	Comment     string      `xml:"comment"`
	Contributor Contributor `xml:"contributor"`
	Text        string      `xml:"text"`
}

// Contributor represents the author of a revision
type Contributor struct {
	Username string `xml:"username"`
	ID       int    `xml:"id"`
	IP       string `xml:"ip"` // For anonymous edits
}

// Redirect represents a redirect page
type Redirect struct {
	Title string `xml:"title,attr"`
}
