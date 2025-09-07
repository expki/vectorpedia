package wikipedia

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	urlRe       = regexp.MustCompile(`http[s]?://\S+`)
	tableRe     = regexp.MustCompile(`(?s)\{\|.*?\|\}`)
	htmlTableRe = regexp.MustCompile(`(?s)<table.*?>.*?</table>`)
	entityRe    = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
)

// cleanWikiMarkup removes Wikipedia markup from text
func cleanWikiMarkup(text string) string {
	// Remove internal wiki link brackets and template markers
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

// removeNonTextLines filters out lines that are likely to be non-text content
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
		// If the line contains more than one pipe and is not a section header, skip it
		if strings.Count(trimmed, "|") > 1 && !strings.HasPrefix(trimmed, "==") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// isDisambiguationPage checks if a page is a disambiguation page
func isDisambiguationPage(text string) bool {
	lowerText := strings.ToLower(text)
	return strings.Contains(lowerText, "[[category:disambiguation") ||
		strings.Contains(lowerText, "{{disambiguation") ||
		strings.Contains(lowerText, "{{disambig")
}

// GenerateWikipediaURL creates a valid Wikipedia URL from a page title
func GenerateWikipediaURL(title string) string {
	escapedTitle := url.QueryEscape(strings.ReplaceAll(title, " ", "_"))
	return "https://en.wikipedia.org/wiki/" + escapedTitle
}
