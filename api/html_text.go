package api

import "git.f4mily.net/goloom/internal/htmltext"

func extractReadableTextFromHTML(html string) string {
	return htmltext.ExtractReadableText(html)
}
