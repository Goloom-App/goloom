package api

import (
	"net/url"
	"strings"
)

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
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil
	}
	if host := strings.ToLower(u.Hostname()); host != "github.com" && host != "www.github.com" {
		return nil
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	rawBase := "https://raw.githubusercontent.com/" + owner + "/" + repo + "/"

	// readmes lists the likely README files under ref (and optional dir), in
	// descending order of how common the filename is.
	readmes := func(ref, dir string) []string {
		prefix := ref + "/"
		if dir != "" {
			prefix += dir + "/"
		}
		return []string{
			rawBase + prefix + "README.md",
			rawBase + prefix + "readme.md",
			rawBase + prefix + "README.rst",
		}
	}

	switch {
	case len(parts) == 2:
		// Repo root: HEAD resolves the default branch without an API call.
		return readmes("HEAD", "")
	case parts[2] == "blob" && len(parts) >= 5:
		return []string{rawBase + parts[3] + "/" + strings.Join(parts[4:], "/")}
	case parts[2] == "raw" && len(parts) >= 5:
		return []string{rawBase + parts[3] + "/" + strings.Join(parts[4:], "/")}
	case parts[2] == "tree" && len(parts) >= 4:
		return readmes(parts[3], strings.Join(parts[4:], "/"))
	default:
		return nil
	}
}
