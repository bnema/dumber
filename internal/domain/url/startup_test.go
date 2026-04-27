package url

import "testing"

func TestResolveBrowserStartupURL_PreservesExplicitURL(t *testing.T) {
	got := ResolveBrowserStartupURL("https://example.com")
	if got != "https://example.com" {
		t.Fatalf("expected explicit URL to be preserved, got %q", got)
	}
}

func TestResolveBrowserStartupURL_DefaultsToHistory(t *testing.T) {
	got := ResolveBrowserStartupURL("")
	if got != "dumb://history" {
		t.Fatalf("expected default browser startup URL, got %q", got)
	}
}

func TestDefaultBrowserStartupURL(t *testing.T) {
	got := DefaultBrowserStartupURL()
	if got != "dumb://history" {
		t.Fatalf("expected history default, got %q", got)
	}
}
