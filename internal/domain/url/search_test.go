package url

import "testing"

// Test shortcuts for BuildSearchURL tests
var testShortcuts = map[string]string{
	"g":   "https://google.com/search?q=%s",
	"ddg": "https://duckduckgo.com/?q=%s",
	"gh":  "https://github.com/search?q=%s",
	"gi":  "https://google.com/search?tbm=isch&q=%s",
}

const testDefaultSearch = "https://duckduckgo.com/?q=%s"

func TestBuildSearchURL(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		shortcuts     map[string]string
		defaultSearch string
		want          string
	}{
		{
			name:          "bang shortcut google",
			input:         "!g golang",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://google.com/search?q=golang",
		},
		{
			name:          "bang shortcut duckduckgo multi-word",
			input:         "!ddg rust async await",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://duckduckgo.com/?q=rust async await",
		},
		{
			name:          "bang shortcut google images",
			input:         "!gi cats",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://google.com/search?tbm=isch&q=cats",
		},
		{
			name:          "unknown bang falls back to default search",
			input:         "!unknown test query",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://duckduckgo.com/?q=!unknown test query",
		},
		{
			name:          "url-like input gets normalized",
			input:         "example.com",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://example.com",
		},
		{
			name:          "url with path",
			input:         "github.com/user/repo",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://github.com/user/repo",
		},
		{
			name:          "url with scheme unchanged",
			input:         "https://example.com",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://example.com",
		},
		{
			name:          "plain search query uses default",
			input:         "how to parse json",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://duckduckgo.com/?q=how to parse json",
		},
		{
			name:          "empty input returns empty",
			input:         "",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "",
		},
		{
			name:          "no shortcuts uses default for bang",
			input:         "!g test",
			shortcuts:     nil,
			defaultSearch: testDefaultSearch,
			want:          "https://duckduckgo.com/?q=!g test",
		},
		{
			name:          "no default search returns input for plain query",
			input:         "plain query",
			shortcuts:     testShortcuts,
			defaultSearch: "",
			want:          "plain query",
		},
		{
			name:          "bang without query treated as search",
			input:         "!g",
			shortcuts:     testShortcuts,
			defaultSearch: testDefaultSearch,
			want:          "https://duckduckgo.com/?q=!g",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSearchURL(tt.input, tt.shortcuts, tt.defaultSearch)
			if got != tt.want {
				t.Errorf("BuildSearchURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseBangShortcut(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantShortcut string
		wantQuery    string
		wantFound    bool
	}{
		{
			name:         "simple google search",
			input:        "!g golang",
			wantShortcut: "g",
			wantQuery:    "golang",
			wantFound:    true,
		},
		{
			name:         "duckduckgo multi-word query",
			input:        "!ddg rust async await",
			wantShortcut: "ddg",
			wantQuery:    "rust async await",
			wantFound:    true,
		},
		{
			name:         "github search",
			input:        "!gh opencode cli",
			wantShortcut: "gh",
			wantQuery:    "opencode cli",
			wantFound:    true,
		},
		{
			name:         "google images",
			input:        "!gi cats",
			wantShortcut: "gi",
			wantQuery:    "cats",
			wantFound:    true,
		},
		{
			name:         "youtube search",
			input:        "!yt music video",
			wantShortcut: "yt",
			wantQuery:    "music video",
			wantFound:    true,
		},
		{
			name:         "no query after shortcut",
			input:        "!g",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "empty query after space",
			input:        "!g ",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "multiple spaces only",
			input:        "!g   ",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "empty shortcut",
			input:        "! query",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "plain text no bang",
			input:        "plain text",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "bang at end not supported",
			input:        "query !g",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "url-like input",
			input:        "example.com",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "empty input",
			input:        "",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "just exclamation mark",
			input:        "!",
			wantShortcut: "",
			wantQuery:    "",
			wantFound:    false,
		},
		{
			name:         "query with extra spaces trimmed",
			input:        "!g   lots of spaces  ",
			wantShortcut: "g",
			wantQuery:    "lots of spaces",
			wantFound:    true,
		},
		{
			name:         "long shortcut key",
			input:        "!stackoverflow how to parse json",
			wantShortcut: "stackoverflow",
			wantQuery:    "how to parse json",
			wantFound:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotShortcut, gotQuery, gotFound := ParseBangShortcut(tt.input)
			if gotShortcut != tt.wantShortcut {
				t.Errorf("ParseBangShortcut() shortcut = %q, want %q", gotShortcut, tt.wantShortcut)
			}
			if gotQuery != tt.wantQuery {
				t.Errorf("ParseBangShortcut() query = %q, want %q", gotQuery, tt.wantQuery)
			}
			if gotFound != tt.wantFound {
				t.Errorf("ParseBangShortcut() found = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}
