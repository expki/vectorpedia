package wikipedia

import (
	_ "encoding/xml"
	"sync"

	"github.com/expki/vectorpedia/ai"
	"github.com/expki/vectorpedia/database"
)

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
