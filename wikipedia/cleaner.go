package wikipedia

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	urlRe          = regexp.MustCompile(`http[s]?://\S+`)
	tableRe        = regexp.MustCompile(`(?s)\{\|.*?\|\}`)
	htmlTableRe    = regexp.MustCompile(`(?s)<table.*?>.*?</table>`)
	entityRe       = regexp.MustCompile(`&[a-zA-Z0-9#]+;`)
	headingRe      = regexp.MustCompile(`(?m)^=+.*?=+\s*$`)
	galleryRe      = regexp.MustCompile(`(?s)<gallery.*?>.*?</gallery>`)
	redirectRe     = regexp.MustCompile(`(?i)#REDIRECT\s*\[\[.*?\]\]`)
	infoboxRe      = regexp.MustCompile(`(?s)\{\{[Ii]nfobox.*?\}\}`)
	templateRe     = regexp.MustCompile(`\{\{[^}]+\}\}`)
	refRe          = regexp.MustCompile(`(?s)<ref[^>]*>.*?</ref>`)
	commentRe      = regexp.MustCompile(`(?s)<!--.*?-->`)
	categoryRe     = regexp.MustCompile(`\[\[Category:.*?\]\]`)
	fileRe         = regexp.MustCompile(`\[\[File:.*?\]\]|\[\[Image:.*?\]\]`)
	portalRe       = regexp.MustCompile(`(?i)\{\{[Pp]ortal.*?\}\}|Portal\|[^}]+`)
	mainRe         = regexp.MustCompile(`(?i)\{\{[Mm]ain\|.*?\}\}|Main\|[^}]+`)
	seeAlsoRe      = regexp.MustCompile(`(?i)\{\{[Ss]ee also.*?\}\}`)
	citationRe     = regexp.MustCompile(`(?i)\{\{[Cc]it[ae].*?\}\}|\{\{[Cc]itation.*?\}\}`)
	authorityRe    = regexp.MustCompile(`(?i)\{\{[Aa]uthority control.*?\}\}`)
	commonsCatRe   = regexp.MustCompile(`(?i)\{\{[Cc]ommons category.*?\}\}|Commons category`)
	wikiRe         = regexp.MustCompile(`\[\[([^|\]]+\|)?([^\]]+)\]\]`)
	reflistRe      = regexp.MustCompile(`(?i)\{\{[Rr]eflist.*?\}\}|\{\{[Rr]efs.*?\}\}`)
	bibliographyRe = regexp.MustCompile(`(?i)\{\{[Bb]ibliography.*?\}\}`)
	isbnRe         = regexp.MustCompile(`ISBN\s+[\d\-X]+`)
	doiRe          = regexp.MustCompile(`doi:\s*[\S]+`)
	pmidRe         = regexp.MustCompile(`PMID\s+\d+`)
	arxivRe        = regexp.MustCompile(`arXiv:\s*[\S]+`)
)

// cleanWikiMarkup removes Wikipedia markup from text and returns only paragraph text
func cleanWikiMarkup(text string) string {
	// Remove redirect pages entirely
	if redirectRe.MatchString(text) {
		return ""
	}

	// First, remove entire sections that are not useful for LLM training
	text = removeBibliographicSections(text)

	// Remove various wiki markup elements in order of precedence
	text = commentRe.ReplaceAllString(text, "")          // HTML comments
	text = refRe.ReplaceAllString(text, "")              // References with content
	text = reflistRe.ReplaceAllString(text, "")          // Reference lists
	text = bibliographyRe.ReplaceAllString(text, "")     // Bibliography templates
	text = galleryRe.ReplaceAllString(text, "")          // Gallery tags
	text = infoboxRe.ReplaceAllString(text, "")          // Infoboxes (must be before general templates)
	text = tableRe.ReplaceAllString(text, "")            // Wiki tables
	text = htmlTableRe.ReplaceAllString(text, "")        // HTML tables
	text = categoryRe.ReplaceAllString(text, "")         // Category links
	text = fileRe.ReplaceAllString(text, "")             // File/Image links
	text = portalRe.ReplaceAllString(text, "")           // Portal links
	text = mainRe.ReplaceAllString(text, "")             // Main article links
	text = seeAlsoRe.ReplaceAllString(text, "")          // See also links
	text = citationRe.ReplaceAllString(text, "")         // Citations
	text = authorityRe.ReplaceAllString(text, "")        // Authority control
	text = commonsCatRe.ReplaceAllString(text, "")       // Commons category
	text = isbnRe.ReplaceAllString(text, "")             // ISBN numbers
	text = doiRe.ReplaceAllString(text, "")              // DOI references
	text = pmidRe.ReplaceAllString(text, "")             // PubMed IDs
	text = arxivRe.ReplaceAllString(text, "")            // arXiv references
	text = headingRe.ReplaceAllString(text, "")          // Section headings
	text = templateRe.ReplaceAllString(text, "")         // Remaining templates
	text = urlRe.ReplaceAllString(text, "")              // External URLs

	// Convert wiki links to plain text (keep link text, remove markup)
	text = wikiRe.ReplaceAllString(text, "$2")

	// Remove any remaining brackets and braces
	text = strings.ReplaceAll(text, "[[", "")
	text = strings.ReplaceAll(text, "]]", "")
	text = strings.ReplaceAll(text, "{{", "")
	text = strings.ReplaceAll(text, "}}", "")
	text = strings.ReplaceAll(text, "<ref>", "")
	text = strings.ReplaceAll(text, "</ref>", "")

	// Remove HTML entities
	text = entityRe.ReplaceAllString(text, " ")

	// Process lines to extract only paragraph text
	text = extractParagraphText(text)

	return strings.TrimSpace(text)
}

// removeBibliographicSections removes entire reference/bibliography sections
func removeBibliographicSections(text string) string {
	// Common section headers for references and sources
	sectionHeaders := []string{
		"== References ==",
		"== Sources ==",
		"== Bibliography ==",
		"== Further reading ==",
		"== External links ==",
		"== Notes ==",
		"== Footnotes ==",
		"== Citations ==",
		"== Works cited ==",
		"== Literature ==",
		"== Publications ==",
	}
	
	lower := strings.ToLower(text)
	earliestIdx := len(text)
	
	// Find the earliest occurrence of any reference section
	for _, header := range sectionHeaders {
		idx := strings.Index(lower, strings.ToLower(header))
		if idx != -1 && idx < earliestIdx {
			earliestIdx = idx
		}
	}
	
	// If we found a reference section, truncate the text there
	if earliestIdx < len(text) {
		text = text[:earliestIdx]
	}
	
	return text
}

// extractParagraphText filters text to keep only paragraph sentences
func extractParagraphText(text string) string {
	lines := strings.Split(text, "\n")
	var paragraphs []string
	var currentPara []string
	
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Skip empty lines
		if trimmed == "" {
			// If we have accumulated paragraph text, save it
			if len(currentPara) > 0 {
				paragraphs = append(paragraphs, strings.Join(currentPara, " "))
				currentPara = nil
			}
			continue
		}
		
		// Skip lines that are just metadata or formatting
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "file:") ||
			strings.HasPrefix(lower, "image:") ||
			strings.HasPrefix(lower, "category:") ||
			strings.HasPrefix(lower, "thumb|") ||
			strings.HasPrefix(lower, "right|") ||
			strings.HasPrefix(lower, "left|") ||
			strings.HasPrefix(lower, "center|") ||
			strings.Contains(trimmed, "|legend|") ||
			strings.Contains(trimmed, "|caption|") ||
			strings.HasPrefix(trimmed, "|") ||
			strings.HasPrefix(trimmed, "!") ||
			strings.HasPrefix(trimmed, "*") ||
			strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, ":") ||
			strings.HasPrefix(trimmed, ";") {
			continue
		}
		
		// Skip lines with excessive pipes (likely table data)
		if strings.Count(trimmed, "|") > 2 {
			continue
		}
		
		// Skip very short lines that are likely not sentences
		if len(trimmed) < 20 && !strings.ContainsAny(trimmed, ".!?") {
			continue
		}
		
		// This looks like actual paragraph text
		currentPara = append(currentPara, trimmed)
	}
	
	// Don't forget the last paragraph
	if len(currentPara) > 0 {
		paragraphs = append(paragraphs, strings.Join(currentPara, " "))
	}
	
	// Join all paragraphs with a space (single continuous text)
	result := strings.Join(paragraphs, " ")
	
	// Clean up excessive whitespace
	result = regexp.MustCompile(`\s+`).ReplaceAllString(result, " ")
	
	return result
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
