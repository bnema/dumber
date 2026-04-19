package ui

import (
	"context"
	"testing"

	urlutil "github.com/bnema/dumber/internal/domain/url"
)

func TestHandleStandaloneOmniboxNavigation_UsesBrowserLauncherForHTTP(t *testing.T) {
	browserURL := ""
	externalURL := ""
	handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(uri string) { browserURL = uri },
		LaunchExternalURL: func(uri string) {
			externalURL = uri
		},
	}, context.Background(), "https://example.com")

	if browserURL != "https://example.com" {
		t.Fatalf("expected browser launcher to receive URL, got %q", browserURL)
	}
	if externalURL != "" {
		t.Fatalf("expected external launcher to be unused, got %q", externalURL)
	}
}

func TestHandleStandaloneOmniboxNavigation_UsesExternalLauncherForCustomScheme(t *testing.T) {
	browserURL := ""
	externalURL := ""
	handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(uri string) { browserURL = uri },
		LaunchExternalURL: func(uri string) {
			externalURL = uri
		},
	}, context.Background(), "vscode://file/tmp/demo")

	if externalURL != "vscode://file/tmp/demo" {
		t.Fatalf("expected external launcher to receive URL, got %q", externalURL)
	}
	if browserURL != "" {
		t.Fatalf("expected browser launcher to be unused, got %q", browserURL)
	}
}

func TestHandleStandaloneOmniboxNavigation_UsesExternalLauncherForMailto(t *testing.T) {
	browserURL := ""
	externalURL := ""
	handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(uri string) { browserURL = uri },
		LaunchExternalURL: func(uri string) {
			externalURL = uri
		},
	}, context.Background(), "mailto:foo@example.com")

	if externalURL != "mailto:foo@example.com" {
		t.Fatalf("expected external launcher to receive URL, got %q", externalURL)
	}
	if browserURL != "" {
		t.Fatalf("expected browser launcher to be unused, got %q", browserURL)
	}
}

func TestStandaloneExternalSchemeRules_HTTPIsNotExternal(t *testing.T) {
	if urlutil.IsExternalScheme("https://example.com") {
		t.Fatalf("expected http url to stay in dumber")
	}
}

func TestStandaloneExternalSchemeRules_CustomSchemeIsExternal(t *testing.T) {
	if !urlutil.IsExternalScheme("spotify://track/123") {
		t.Fatalf("expected custom scheme to be external")
	}
}

func TestStandaloneExternalSchemeRules_InternalSchemesStayInternal(t *testing.T) {
	for _, rawURL := range []string{"dumb://home", "file:///tmp/demo", "about:blank", "data:text/plain,hi"} {
		if urlutil.IsExternalScheme(rawURL) {
			t.Fatalf("expected %q to stay internal", rawURL)
		}
	}
}
