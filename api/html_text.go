package api

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

const maxKnowledgeExtractLen = 12_000

var (
	htmlCommentPattern = regexp.MustCompile(`(?is)<!--.*?-->`)
	htmlTagPattern     = regexp.MustCompile(`<[^>]+>`)
	blockTagPatterns   = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<template[^>]*>.*?</template>`),
		regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`),
		regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>`),
	}
)

var nonContentSelectors = []string{
	"script", "style", "noscript", "template", "svg", "iframe", "head",
	"nav", "footer", "header", "aside", "form", "button", "input", "select", "textarea",
}

var contentSelectors = []string{
	"main", "article", "[role='main']", ".content", "#content", ".post", ".entry-content", "body",
}

// extractReadableTextFromHTML returns visible page text without scripts, styles, or markup noise.
func extractReadableTextFromHTML(html string) string {
	html = strings.TrimSpace(html)
	if html == "" {
		return ""
	}

	if text := extractWithGoquery(html); strings.TrimSpace(text) != "" {
		return truncateKnowledgeText(normalizeWhitespace(text))
	}

	return truncateKnowledgeText(normalizeWhitespace(stripHTMLWithRegex(html)))
}

func extractWithGoquery(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}

	doc.Find(strings.Join(nonContentSelectors, ", ")).Remove()

	for _, selector := range contentSelectors {
		selection := doc.Find(selector).First()
		if selection.Length() == 0 {
			continue
		}
		text := strings.TrimSpace(selection.Text())
		if len([]rune(text)) >= 80 {
			return text
		}
	}

	return strings.TrimSpace(doc.Text())
}

func stripHTMLWithRegex(html string) string {
	html = htmlCommentPattern.ReplaceAllString(html, " ")
	for _, pattern := range blockTagPatterns {
		html = pattern.ReplaceAllString(html, " ")
	}
	html = htmlTagPattern.ReplaceAllString(html, " ")
	return html
}

func normalizeWhitespace(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(text))
	lastSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

func truncateKnowledgeText(text string) string {
	runes := []rune(text)
	if len(runes) <= maxKnowledgeExtractLen {
		return text
	}
	return string(runes[:maxKnowledgeExtractLen]) + "…"
}
