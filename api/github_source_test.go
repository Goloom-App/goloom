package api

import "testing"

func TestGithubRawCandidates(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		first   string
		wantNil bool
	}{
		{"repo root resolves default-branch README", "https://github.com/YaLTeR/niri", "https://raw.githubusercontent.com/YaLTeR/niri/HEAD/README.md", false},
		{"repo root tolerates trailing slash and query", "https://github.com/o/r/?tab=readme-ov-file", "https://raw.githubusercontent.com/o/r/HEAD/README.md", false},
		{"repo .git suffix is stripped", "https://github.com/o/r.git", "https://raw.githubusercontent.com/o/r/HEAD/README.md", false},
		{"blob points at the raw file", "https://github.com/o/r/blob/main/docs/x.md", "https://raw.githubusercontent.com/o/r/main/docs/x.md", false},
		{"blob drops the line fragment", "https://github.com/o/r/blob/main/README.md#install", "https://raw.githubusercontent.com/o/r/main/README.md", false},
		{"tree resolves that directory's README", "https://github.com/o/r/tree/main/sub", "https://raw.githubusercontent.com/o/r/main/sub/README.md", false},
		{"raw path maps to raw host", "https://github.com/o/r/raw/main/x.md", "https://raw.githubusercontent.com/o/r/main/x.md", false},
		{"issues page keeps HTML fetch", "https://github.com/o/r/issues/12", "", true},
		{"pull page keeps HTML fetch", "https://github.com/o/r/pull/3", "", true},
		{"wiki keeps HTML fetch", "https://github.com/o/r/wiki/Home", "", true},
		{"gist host is not rewritten", "https://gist.github.com/o/abc123", "", true},
		{"non-github host is not rewritten", "https://example.com/o/r", "", true},
		{"bare owner is not a repo", "https://github.com/o", "", true},
	}
	for _, c := range cases {
		got := githubRawCandidates(c.in)
		if c.wantNil {
			if got != nil {
				t.Errorf("%s: want nil, got %v", c.name, got)
			}
			continue
		}
		if len(got) == 0 {
			t.Errorf("%s: got no candidates", c.name)
			continue
		}
		if got[0] != c.first {
			t.Errorf("%s: first candidate = %q, want %q (all=%v)", c.name, got[0], c.first, got)
		}
	}
}
