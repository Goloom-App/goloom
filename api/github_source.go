package api

import "git.f4mily.net/goloom/internal/pagefetch"

// githubRawCandidates rewrites a github.com web URL to the raw source URLs that
// actually hold its text. GitHub renders READMEs and file blobs client-side, so
// the HTML page is mostly chrome ("Uh oh! There was an error while loading…")
// with little usable prose — useless for drafting a post about a repo. The raw
// host serves the underlying markdown directly, follows repo renames, and is not
// rate-limited like the API.
//
// It returns an ordered list of raw.githubusercontent.com candidates to try, or
// nil when the URL is not a rewritable GitHub page (issues, PRs, wikis, gists,
// non-GitHub hosts) — those keep their normal HTML fetch. README filenames vary,
// so repo-root and directory URLs return several candidates to try in order.
func githubRawCandidates(rawURL string) []string {
	return pagefetch.GitHubRawCandidates(rawURL)
}
