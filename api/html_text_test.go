package api

import (
	"strings"
	"testing"
)

func TestExtractReadableTextFromHTML_RemovesScriptAndStyle(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
  <title>Über uns - Binärgewitter</title>
  <style>body{background-color:#fff}.navbar{background-color:#f5f6f6}</style>
  <script>head.js({jquery:"//cdn.example/jquery.min.js"});async function checkLive(){console.debug("live")}</script>
</head>
<body>
  <nav>Home Abonnieren Live</nav>
  <main>
    <h1>Über uns</h1>
    <p>Ein Podcast, der sich mit dem Web, Technologie und Open Source Software auseinander setzt.</p>
    <p>@binaergewitter auf Mastodon</p>
  </main>
</body>
</html>`

	text := extractReadableTextFromHTML(html)

	for _, forbidden := range []string{
		"head.js",
		"background-color",
		"checkLive",
		"console.debug",
		"jquery.min.js",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("expected extracted text to exclude %q, got: %s", forbidden, text)
		}
	}

	for _, required := range []string{
		"Über uns",
		"Podcast",
		"Open Source Software",
		"@binaergewitter",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("expected extracted text to include %q, got: %s", required, text)
		}
	}
}

func TestExtractReadableTextFromHTML_FallsBackToRegex(t *testing.T) {
	html := `<html><body><p>Hello <strong>world</strong></p><script>alert(1)</script></body></html>`
	text := extractReadableTextFromHTML(html)
	if text != "Hello world" {
		t.Fatalf("unexpected text: %q", text)
	}
}

