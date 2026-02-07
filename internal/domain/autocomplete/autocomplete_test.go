package autocomplete

import "testing"

func TestComputeCompletionSuffix(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		fullText   string
		wantSuffix string
		wantOK     bool
	}{
		{
			name:       "empty input",
			input:      "",
			fullText:   "example.com",
			wantSuffix: "",
			wantOK:     false,
		},
		{
			name:       "empty fullText",
			input:      "exa",
			fullText:   "",
			wantSuffix: "",
			wantOK:     false,
		},
		{
			name:       "both empty",
			input:      "",
			fullText:   "",
			wantSuffix: "",
			wantOK:     false,
		},
		{
			name:       "exact match returns false",
			input:      "example.com",
			fullText:   "example.com",
			wantSuffix: "",
			wantOK:     false,
		},
		{
			name:       "prefix match",
			input:      "exa",
			fullText:   "example.com",
			wantSuffix: "mple.com",
			wantOK:     true,
		},
		{
			name:       "case insensitive match",
			input:      "EXA",
			fullText:   "example.com",
			wantSuffix: "mple.com",
			wantOK:     true,
		},
		{
			name:       "case insensitive preserves original case in suffix",
			input:      "git",
			fullText:   "GitHub.com",
			wantSuffix: "Hub.com",
			wantOK:     true,
		},
		{
			name:       "no prefix match",
			input:      "xyz",
			fullText:   "example.com",
			wantSuffix: "",
			wantOK:     false,
		},
		{
			name:       "input longer than fullText",
			input:      "example.com/path",
			fullText:   "example.com",
			wantSuffix: "",
			wantOK:     false,
		},
		{
			name:       "single character prefix",
			input:      "e",
			fullText:   "example.com",
			wantSuffix: "xample.com",
			wantOK:     true,
		},
		{
			name:       "bang shortcut prefix",
			input:      "!g",
			fullText:   "!github",
			wantSuffix: "ithub",
			wantOK:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suffix, ok := ComputeCompletionSuffix(tt.input, tt.fullText)
			if suffix != tt.wantSuffix {
				t.Errorf("ComputeCompletionSuffix(%q, %q) suffix = %q, want %q",
					tt.input, tt.fullText, suffix, tt.wantSuffix)
			}
			if ok != tt.wantOK {
				t.Errorf("ComputeCompletionSuffix(%q, %q) ok = %v, want %v",
					tt.input, tt.fullText, ok, tt.wantOK)
			}
		})
	}
}

func TestStripProtocol(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "https protocol",
			url:  "https://example.com",
			want: "example.com",
		},
		{
			name: "http protocol",
			url:  "http://example.com",
			want: "example.com",
		},
		{
			name: "no protocol",
			url:  "example.com",
			want: "example.com",
		},
		{
			name: "https with path",
			url:  "https://example.com/path/to/page",
			want: "example.com/path/to/page",
		},
		{
			name: "http with query string",
			url:  "http://example.com?query=value",
			want: "example.com?query=value",
		},
		{
			name: "file protocol unchanged",
			url:  "file:///path/to/file",
			want: "file:///path/to/file",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripProtocol(tt.url)
			if got != tt.want {
				t.Errorf("StripProtocol(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestComputeURLCompletionSuffix(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		fullURL        string
		wantSuffix     string
		wantMatchedURL string
		wantOK         bool
	}{
		{
			name:           "empty input",
			input:          "",
			fullURL:        "https://example.com",
			wantSuffix:     "",
			wantMatchedURL: "",
			wantOK:         false,
		},
		{
			name:           "direct match with protocol",
			input:          "https://exa",
			fullURL:        "https://example.com",
			wantSuffix:     "mple.com",
			wantMatchedURL: "https://example.com",
			wantOK:         true,
		},
		{
			name:           "match without protocol in input",
			input:          "exa",
			fullURL:        "https://example.com",
			wantSuffix:     "mple.com",
			wantMatchedURL: "example.com",
			wantOK:         true,
		},
		{
			name:           "match with www prefix in URL strips www",
			input:          "exa",
			fullURL:        "https://www.example.com",
			wantSuffix:     "mple.com",
			wantMatchedURL: "example.com",
			wantOK:         true,
		},
		{
			name:           "match with www in input but not URL",
			input:          "www.exa",
			fullURL:        "https://example.com",
			wantSuffix:     "mple.com",
			wantMatchedURL: "example.com",
			wantOK:         true,
		},
		{
			name:           "no match",
			input:          "xyz",
			fullURL:        "https://example.com",
			wantSuffix:     "",
			wantMatchedURL: "",
			wantOK:         false,
		},
		{
			name:           "case insensitive URL match",
			input:          "GITHUB",
			fullURL:        "https://github.com/user/repo",
			wantSuffix:     ".com/user/repo",
			wantMatchedURL: "github.com/user/repo",
			wantOK:         true,
		},
		{
			name:           "http protocol stripped",
			input:          "news",
			fullURL:        "http://news.ycombinator.com",
			wantSuffix:     ".ycombinator.com",
			wantMatchedURL: "news.ycombinator.com",
			wantOK:         true,
		},
		{
			name:           "full URL match returns false",
			input:          "example.com",
			fullURL:        "https://example.com",
			wantSuffix:     "",
			wantMatchedURL: "",
			wantOK:         false,
		},
		{
			name:           "URL with path",
			input:          "github.com/user",
			fullURL:        "https://github.com/user/repo",
			wantSuffix:     "/repo",
			wantMatchedURL: "github.com/user/repo",
			wantOK:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suffix, matchedURL, ok := ComputeURLCompletionSuffix(tt.input, tt.fullURL)
			if suffix != tt.wantSuffix {
				t.Errorf("ComputeURLCompletionSuffix(%q, %q) suffix = %q, want %q",
					tt.input, tt.fullURL, suffix, tt.wantSuffix)
			}
			if matchedURL != tt.wantMatchedURL {
				t.Errorf("ComputeURLCompletionSuffix(%q, %q) matchedURL = %q, want %q",
					tt.input, tt.fullURL, matchedURL, tt.wantMatchedURL)
			}
			if ok != tt.wantOK {
				t.Errorf("ComputeURLCompletionSuffix(%q, %q) ok = %v, want %v",
					tt.input, tt.fullURL, ok, tt.wantOK)
			}
		})
	}
}

func TestBestURLCompletion_PrefersHostForHostLikeInput(t *testing.T) {
	urls := []string{
		"https://www.google.com/url?q=https://dashboard.stripe.com/auth",
		"https://google.com",
	}

	suffix, matchedURL, ok := BestURLCompletion("goo", urls)
	if !ok {
		t.Fatalf("expected completion")
	}
	if matchedURL != "google.com" {
		t.Fatalf("expected host completion, got %q", matchedURL)
	}
	if suffix != "gle.com" {
		t.Fatalf("expected host suffix, got %q", suffix)
	}
}

func TestBestURLCompletion_KeepsPathForPathLikeInput(t *testing.T) {
	urls := []string{
		"https://google.com/url?q=https://example.com",
	}

	suffix, matchedURL, ok := BestURLCompletion("google.com/u", urls)
	if !ok {
		t.Fatalf("expected completion")
	}
	if matchedURL != "google.com/url?q=https://example.com" {
		t.Fatalf("unexpected matched URL: %q", matchedURL)
	}
	if suffix != "rl?q=https://example.com" {
		t.Fatalf("unexpected suffix: %q", suffix)
	}
}

func TestBestURLCompletion_SelectsAnyValidPrefixFromVisibleList(t *testing.T) {
	urls := []string{
		"https://example.org",
		"https://google.com/maps",
	}

	suffix, matchedURL, ok := BestURLCompletion("goo", urls)
	if !ok {
		t.Fatalf("expected completion from second URL")
	}
	if matchedURL != "google.com" {
		t.Fatalf("expected host completion from second URL, got %q", matchedURL)
	}
	if suffix != "gle.com" {
		t.Fatalf("unexpected suffix: %q", suffix)
	}
}
