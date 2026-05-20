package i18n

import "testing"

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
