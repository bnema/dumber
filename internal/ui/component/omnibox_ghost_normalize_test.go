package component

import "testing"

func TestNormalizeGhostSuggestion_PrefersHostOnlyForRedirectURL(t *testing.T) {
	full, suffix := normalizeGhostSuggestion(
		"google",
		"google.com/url?q=https://dashboard.stripe.com/auth_challenge/email_link/abc",
		".com/url?q=https://dashboard.stripe.com/auth_challenge/email_link/abc",
	)
	if full != "google.com" {
		t.Fatalf("expected host-only completion, got %q", full)
	}
	if suffix != ".com" {
		t.Fatalf("expected .com suffix, got %q", suffix)
	}
}

func TestNormalizeGhostSuggestion_KeepsFallbackWhenInputLooksLikePath(t *testing.T) {
	fallback := "/url?q=https://x"
	full, suffix := normalizeGhostSuggestion("google.com/u", "google.com/url?q=https://x", fallback)
	if full != "google.com/url?q=https://x" {
		t.Fatalf("expected full URL fallback, got %q", full)
	}
	if suffix != fallback {
		t.Fatalf("expected fallback suffix, got %q", suffix)
	}
}

func TestExtractHostForCompletion(t *testing.T) {
	got := extractHostForCompletion("https://WWW.Google.com/url?q=abc")
	if got != "www.google.com" {
		t.Fatalf("expected normalized host, got %q", got)
	}
}

func TestGhostCompletionInput(t *testing.T) {
	tests := []struct {
		name        string
		entryText   string
		wantPrefix  string
		wantTrimmed string
		wantOK      bool
	}{
		{name: "no leading whitespace", entryText: "goo", wantPrefix: "", wantTrimmed: "goo", wantOK: true},
		{name: "leading space", entryText: " goo", wantPrefix: " ", wantTrimmed: "goo", wantOK: true},
		{name: "leading tab", entryText: "\tgoo", wantPrefix: "\t", wantTrimmed: "goo", wantOK: true},
		{name: "only whitespace", entryText: "  \t", wantPrefix: "  \t", wantTrimmed: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrefix, gotTrimmed, gotOK := ghostCompletionInput(tt.entryText)
			if gotPrefix != tt.wantPrefix {
				t.Fatalf("ghostCompletionInput(%q) prefix = %q, want %q", tt.entryText, gotPrefix, tt.wantPrefix)
			}
			if gotTrimmed != tt.wantTrimmed {
				t.Fatalf("ghostCompletionInput(%q) trimmed = %q, want %q", tt.entryText, gotTrimmed, tt.wantTrimmed)
			}
			if gotOK != tt.wantOK {
				t.Fatalf("ghostCompletionInput(%q) ok = %v, want %v", tt.entryText, gotOK, tt.wantOK)
			}
		})
	}
}
