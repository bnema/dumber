package usecase

import (
	"context"
	"testing"
)

func TestFilterBangs(t *testing.T) {
	shortcuts := map[string]SearchShortcut{
		"ddg": {URL: "https://duckduckgo.com/?q=%s", Description: "DuckDuckGo search"},
		"g":   {URL: "https://google.com/search?q=%s", Description: "Google search"},
		"gh":  {URL: "https://github.com/search?q=%s", Description: "GitHub search"},
		"n":   {URL: "https://news.ycombinator.com/", Description: ""},
	}
	uc := NewSearchShortcutsUseCase(shortcuts)
	ctx := context.Background()

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
			output := uc.FilterBangs(ctx, FilterBangsInput{Query: tc.query})
			got := output.Suggestions
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
	shortcuts := map[string]SearchShortcut{
		"gh":  {URL: "https://github.com/search?q=%s", Description: "GitHub search"},
		"ddg": {URL: "https://duckduckgo.com/?q=%s", Description: ""},
	}
	uc := NewSearchShortcutsUseCase(shortcuts)
	ctx := context.Background()

	cases := []struct {
		name      string
		query     string
		wantKey   string
		wantDescr string
	}{
		{name: "no bang prefix", query: "gh test", wantKey: "", wantDescr: ""},
		{name: "bang only has no space", query: "!gh", wantKey: "", wantDescr: ""},
		{name: "space at position 1", query: "! test", wantKey: "", wantDescr: ""},
		{name: "unknown bang", query: "!nope test", wantKey: "", wantDescr: ""},
		{name: "case-insensitive match", query: "!GH test", wantKey: "gh", wantDescr: "GitHub search"},
		{name: "valid bang key", query: "!ddg test", wantKey: "ddg", wantDescr: "https://duckduckgo.com/?q=%s"},
		{name: "description fallback to URL", query: "!DDG query", wantKey: "ddg", wantDescr: "https://duckduckgo.com/?q=%s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := uc.DetectBangKey(ctx, DetectBangKeyInput{Query: tc.query})
			if output.Key != tc.wantKey {
				t.Fatalf("DetectBangKey(%q).Key=%q want=%q", tc.query, output.Key, tc.wantKey)
			}
			if output.Description != tc.wantDescr {
				t.Fatalf("DetectBangKey(%q).Description=%q want=%q", tc.query, output.Description, tc.wantDescr)
			}
		})
	}
}

func TestBuildNavigationText(t *testing.T) {
	shortcuts := map[string]SearchShortcut{
		"gh":  {URL: "https://github.com/search?q=%s"},
		"ddg": {URL: "https://duckduckgo.com/?q=%s"},
	}
	uc := NewSearchShortcutsUseCase(shortcuts)
	ctx := context.Background()

	cases := []struct {
		name      string
		entryText string
		want      string
		wantValid bool
	}{
		{name: "not a bang shortcut", entryText: "example.com", want: "", wantValid: false},
		{name: "bang key without query", entryText: "!gh ", want: "", wantValid: false},
		{name: "unknown bang key", entryText: "!nope test", want: "", wantValid: false},
		{name: "normalizes key case", entryText: "!GH dumber", want: "!gh dumber", wantValid: true},
		{name: "keeps query unchanged", entryText: "!ddg some query", want: "!ddg some query", wantValid: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := uc.BuildNavigationText(ctx, BuildNavigationTextInput{EntryText: tc.entryText})
			if output.Text != tc.want {
				t.Fatalf("BuildNavigationText(%q).Text=%q want=%q", tc.entryText, output.Text, tc.want)
			}
			if output.Valid != tc.wantValid {
				t.Fatalf("BuildNavigationText(%q).Valid=%v want=%v", tc.entryText, output.Valid, tc.wantValid)
			}
		})
	}
}

func TestGetShortcut(t *testing.T) {
	shortcuts := map[string]SearchShortcut{
		"gh": {URL: "https://github.com/search?q=%s", Description: "GitHub"},
	}
	uc := NewSearchShortcutsUseCase(shortcuts)

	cases := []struct {
		name    string
		key     string
		wantOK  bool
		wantURL string
	}{
		{name: "exact match", key: "gh", wantOK: true, wantURL: "https://github.com/search?q=%s"},
		{name: "case insensitive", key: "GH", wantOK: true, wantURL: "https://github.com/search?q=%s"},
		{name: "not found", key: "unknown", wantOK: false, wantURL: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shortcut, ok := uc.GetShortcut(tc.key)
			if ok != tc.wantOK {
				t.Fatalf("GetShortcut(%q) ok=%v want=%v", tc.key, ok, tc.wantOK)
			}
			if shortcut.URL != tc.wantURL {
				t.Fatalf("GetShortcut(%q).URL=%q want=%q", tc.key, shortcut.URL, tc.wantURL)
			}
		})
	}
}
