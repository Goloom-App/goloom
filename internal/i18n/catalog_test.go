package i18n

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestDiscoverLanguagesFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"en.json":   {Data: []byte(`{"api":{}}`)},
		"de.json":   {Data: []byte(`{"api":{}}`)},
		"fr.json":   {Data: []byte(`{"api":{}}`)},
		"README.md": {Data: []byte("ignore me")},
	}
	got := discoverLanguages(fsys)
	want := []string{"de", "en", "fr"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverLanguages = %v, want %v (sorted, .json only)", got, want)
	}
}

func TestDiscoverLanguagesFallsBackToDefault(t *testing.T) {
	got := discoverLanguages(fstest.MapFS{})
	if len(got) != 1 || got[0] != DefaultLanguage {
		t.Fatalf("empty FS should fall back to [%q], got %v", DefaultLanguage, got)
	}
}

func TestSupportedLanguagesDerivedFromBundle(t *testing.T) {
	has := func(code string) bool {
		for _, l := range SupportedLanguages {
			if l == code {
				return true
			}
		}
		return false
	}
	if !has("en") || !has("de") {
		t.Fatalf("SupportedLanguages should be derived from the bundle and include en+de, got %v", SupportedLanguages)
	}
}

func TestMatchLanguage(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"", DefaultLanguage},
		{"de", "de"},
		{"de-DE,de;q=0.9,en;q=0.8", "de"},
		{"fr,en", DefaultLanguage},
		{"en-US", "en"},
	}
	for _, tc := range tests {
		if got := MatchLanguage(tc.header); got != tc.want {
			t.Errorf("MatchLanguage(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestLoadCatalog(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Message("de", "team_not_found") == "team_not_found" {
		t.Fatal("expected German translation for team_not_found")
	}
	if c.Message("en", "team_not_found") != "team not found" {
		t.Fatalf("en team_not_found = %q", c.Message("en", "team_not_found"))
	}
}
