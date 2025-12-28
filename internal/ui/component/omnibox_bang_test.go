package component

import (
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestBuildBangSuggestions(t *testing.T) {
	shortcuts := map[string]config.SearchShortcut{
		"ddg": {URL: "https://duckduckgo.com/?q=%s", Description: "DuckDuckGo search"},
		"g":   {URL: "https://google.com/search?q=%s", Description: "Google search"},
		"gh":  {URL: "https://github.com/search?q=%s", Description: "GitHub search"},
		"n":   {URL: "https://news.ycombinator.com/", Description: ""},
	}

	cases := []struct {
		name      string
		query     string
		wantKeys  []string
		wantDescr map[string]string
	}{
		{
			name:     "bang only returns all sorted",
			query:    "!",
			wantKeys: []string{"ddg", "g", "gh", "n"},
		},
		{
			name:     "filters by prefix",
			query:    "!g",
			wantKeys: []string{"g", "gh"},
		},
		{
			name:     "filters by prefix case-insensitive",
			query:    "!DD",
			wantKeys: []string{"ddg"},
		},
		{
			name:     "stops prefix at space",
			query:    "!g query",
			wantKeys: []string{"g", "gh"},
		},
		{
			name:     "falls back to url when description empty",
			query:    "!n",
			wantKeys: []string{"n"},
			wantDescr: map[string]string{
				"n": "https://news.ycombinator.com/",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildBangSuggestions(shortcuts, tc.query)
			if len(got) != len(tc.wantKeys) {
				t.Fatalf("len=%d want=%d", len(got), len(tc.wantKeys))
			}
			for i, wantKey := range tc.wantKeys {
				if got[i].Key != wantKey {
					t.Fatalf("idx %d key=%q want=%q", i, got[i].Key, wantKey)
				}
				if tc.wantDescr != nil {
					if wantD, ok := tc.wantDescr[wantKey]; ok {
						if got[i].Description != wantD {
							t.Fatalf("key %q description=%q want=%q", wantKey, got[i].Description, wantD)
						}
					}
				}
			}
		})
	}
}

func TestDetectBangKey(t *testing.T) {
	shortcuts := map[string]config.SearchShortcut{
		"gh":  {URL: "https://github.com/search?q=%s"},
		"ddg": {URL: "https://duckduckgo.com/?q=%s"},
	}

	cases := []struct {
		name  string
		query string
		want  string
	}{
		{name: "no bang prefix", query: "gh test", want: ""},
		{name: "bang only has no space", query: "!gh", want: ""},
		{name: "space at position 1", query: "! test", want: ""},
		{name: "unknown bang", query: "!nope test", want: ""},
		{name: "case-insensitive match", query: "!GH test", want: "gh"},
		{name: "valid bang key", query: "!ddg test", want: "ddg"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectBangKey(shortcuts, tc.query); got != tc.want {
				t.Fatalf("detectBangKey(%q)=%q want=%q", tc.query, got, tc.want)
			}
		})
	}
}

func TestBuildBangNavigationText(t *testing.T) {
	shortcuts := map[string]config.SearchShortcut{
		"gh":  {URL: "https://github.com/search?q=%s"},
		"ddg": {URL: "https://duckduckgo.com/?q=%s"},
	}

	cases := []struct {
		name      string
		entryText string
		want      string
	}{
		{name: "not a bang shortcut", entryText: "example.com", want: ""},
		{name: "bang key without query", entryText: "!gh ", want: ""},
		{name: "unknown bang key", entryText: "!nope test", want: ""},
		{name: "normalizes key case", entryText: "!GH dumber", want: "!gh dumber"},
		{name: "keeps query unchanged", entryText: "!ddg some query", want: "!ddg some query"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildBangNavigationText(shortcuts, tc.entryText); got != tc.want {
				t.Fatalf("buildBangNavigationText(%q)=%q want=%q", tc.entryText, got, tc.want)
			}
		})
	}
}
