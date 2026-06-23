package domain

import "testing"

func TestCreatePostInput_Normalize(t *testing.T) {
	in := CreatePostInput{
		Title:                  "  Hello  ",
		Content:                "  body  ",
		Visibility:             "garbage",
		MediaIDs:               []string{" m1 ", "", "m1"},
		TargetAccounts:         []string{"acc-1", "acc-2"},
		AccountContentOverride: map[string]string{"acc-1": "ok", "acc-unknown": "dropped", "acc-2": "   "},
	}
	in.Normalize()

	if in.Title != "Hello" || in.Content != "body" {
		t.Fatalf("title/content not trimmed: %q / %q", in.Title, in.Content)
	}
	if in.Visibility != "public" {
		t.Errorf("visibility = %q, want normalized to public", in.Visibility)
	}
	// Overrides keep only non-empty entries for targeted accounts.
	if len(in.AccountContentOverride) != 1 || in.AccountContentOverride["acc-1"] != "ok" {
		t.Errorf("override not normalized to targets: %+v", in.AccountContentOverride)
	}
	if !in.UseVersions {
		t.Error("UseVersions must be derived true when an override remains")
	}

	none := CreatePostInput{Title: "t", TargetAccounts: []string{"a"}}
	none.Normalize()
	if none.UseVersions {
		t.Error("UseVersions must be false without overrides")
	}
}

func TestCreatePostInput_Validate(t *testing.T) {
	cases := []struct {
		name    string
		in      CreatePostInput
		wantErr bool
	}{
		{"valid scheduled", CreatePostInput{Title: "t", Content: "c", TargetAccounts: []string{"a"}}, false},
		{"missing title", CreatePostInput{Content: "c", TargetAccounts: []string{"a"}}, true},
		{"blank title", CreatePostInput{Title: "   ", Content: "c", TargetAccounts: []string{"a"}}, true},
		{"scheduled missing content", CreatePostInput{Title: "t", TargetAccounts: []string{"a"}}, true},
		{"scheduled missing targets", CreatePostInput{Title: "t", Content: "c"}, true},
		{"draft needs only title", CreatePostInput{Title: "t", Draft: true}, false},
		{"draft still needs title", CreatePostInput{Draft: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.in.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}
