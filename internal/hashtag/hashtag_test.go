package hashtag

import (
	"reflect"
	"testing"
)

func TestExtract(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []Tag
	}{
		{
			name: "basic dedup case-insensitive",
			in:   "Neues Release! #OpenSource und nochmal #opensource",
			want: []Tag{{Norm: "opensource", Display: "OpenSource"}},
		},
		{
			name: "umlauts",
			in:   "Heute: #Müllabfuhr und #straßenfest",
			want: []Tag{{Norm: "müllabfuhr", Display: "Müllabfuhr"}, {Norm: "straßenfest", Display: "straßenfest"}},
		},
		{
			name: "url fragment is not a tag",
			in:   "Docs: https://example.com/page#section sowie #echt",
			want: []Tag{{Norm: "echt", Display: "echt"}},
		},
		{
			name: "numeric only is not a tag",
			in:   "Jahresrückblick #2024 vs #Jahr2024",
			want: []Tag{{Norm: "jahr2024", Display: "Jahr2024"}},
		},
		{
			name: "glued to word is not a tag",
			in:   "C#programmieren ist kein Tag, #CSharp schon",
			want: []Tag{{Norm: "csharp", Display: "CSharp"}},
		},
		{
			name: "no tags",
			in:   "Nur Text ohne alles.",
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Extract(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Extract(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestMatchesByteOffsets(t *testing.T) {
	// Umlaut and emoji before the tag shift byte offsets vs rune offsets.
	in := "Grüße 👋 #Tag!"
	got := Matches(in)
	if len(got) != 1 {
		t.Fatalf("Matches(%q) returned %d matches, want 1", in, len(got))
	}
	m := got[0]
	if in[m.Start:m.End] != "#Tag" {
		t.Fatalf("offsets [%d:%d] select %q, want %q", m.Start, m.End, in[m.Start:m.End], "#Tag")
	}
	if m.Display != "Tag" {
		t.Fatalf("Display = %q, want %q", m.Display, "Tag")
	}
}

func TestURLMatchesTrimsTrailingPunctuation(t *testing.T) {
	in := "Siehe https://example.com/a, oder (https://example.org/b)."
	got := URLMatches(in)
	if len(got) != 2 {
		t.Fatalf("URLMatches returned %d, want 2", len(got))
	}
	if got[0].Display != "https://example.com/a" {
		t.Fatalf("first url = %q", got[0].Display)
	}
	if got[1].Display != "https://example.org/b" {
		t.Fatalf("second url = %q", got[1].Display)
	}
}
