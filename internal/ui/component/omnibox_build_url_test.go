package component

import (
	"context"
	"testing"
)

func TestOmniboxBuildURLUsesNavigationNormalizerBeforeURLFallback(t *testing.T) {
	o := &Omnibox{
		ctx:           context.Background(),
		defaultSearch: "https://search.example/?q=%s",
		normalizeNavigationURL: func(_ context.Context, input string) string {
			if input == "page.html" {
				return "file:///tmp/page.html"
			}
			return input
		},
	}

	if got := o.buildURL("page.html"); got != "file:///tmp/page.html" {
		t.Fatalf("buildURL(page.html) = %q, want local file URL", got)
	}
}

func TestOmniboxBuildURLLeavesBangShortcutsToSearchHandling(t *testing.T) {
	o := &Omnibox{
		ctx:           context.Background(),
		defaultSearch: "https://search.example/?q=%s",
		normalizeNavigationURL: func(_ context.Context, _ string) string {
			return "file:///tmp/page.html"
		},
	}

	if got := o.buildURL("!unknown page.html"); got != "https://search.example/?q=!unknown page.html" {
		t.Fatalf("buildURL(unknown bang) = %q, want search fallback", got)
	}
}
