package ui

import (
	"context"
	"errors"
	stdurl "net/url"
	"path/filepath"
	"testing"

	urlutil "github.com/bnema/dumber/internal/domain/url"
)

type standaloneTestLocalPathResolver struct {
	paths map[string]string
}

func (r standaloneTestLocalPathResolver) ResolveExistingPath(_ context.Context, input string) (string, bool, error) {
	absPath, ok := r.paths[input]
	return absPath, ok, nil
}

func TestNewStandaloneOmniboxRuntimeUsesExplicitLocalPathResolver(t *testing.T) {
	absFile := filepath.Join(string(filepath.Separator), "tmp", "page.html")
	runtime := NewStandaloneOmniboxRuntime(context.Background(), &Dependencies{
		LocalPathResolver: standaloneTestLocalPathResolver{paths: map[string]string{"page.html": absFile}},
	}, nil)

	got := runtime.OmniboxCfg.NormalizeNavigationURL(context.Background(), "page.html")
	want := (&stdurl.URL{Scheme: "file", Path: absFile}).String()
	if got != want {
		t.Fatalf("NormalizeNavigationURL(page.html) = %q, want %q", got, want)
	}
}

func TestHandleStandaloneOmniboxNavigation_UsesFreshBrowserWindowForHTTP(t *testing.T) {
	browserURL := ""
	externalURL := ""
	err := handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(_ context.Context, uri string) error {
			browserURL = uri
			return nil
		},
		LaunchExternalURL: func(uri string) {
			externalURL = uri
		},
	}, context.Background(), "https://example.com")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
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
	err := handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(_ context.Context, uri string) error {
			browserURL = uri
			return nil
		},
		LaunchExternalURL: func(uri string) {
			externalURL = uri
		},
	}, context.Background(), "vscode://file/tmp/demo")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
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
	err := handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(_ context.Context, uri string) error {
			browserURL = uri
			return nil
		},
		LaunchExternalURL: func(uri string) {
			externalURL = uri
		},
	}, context.Background(), "mailto:foo@example.com")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if externalURL != "mailto:foo@example.com" {
		t.Fatalf("expected external launcher to receive URL, got %q", externalURL)
	}
	if browserURL != "" {
		t.Fatalf("expected browser launcher to be unused, got %q", browserURL)
	}
}

func TestHandleStandaloneOmniboxNavigation_ReturnsBrowserLaunchError(t *testing.T) {
	wantErr := context.DeadlineExceeded

	err := handleStandaloneOmniboxNavigation(&Dependencies{
		LaunchBrowserURL: func(context.Context, string) error {
			return wantErr
		},
	}, context.Background(), "https://example.com")

	if err == nil {
		t.Fatal("expected browser launch error")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
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
	for _, rawURL := range []string{"dumb://history", "dumb://error", "file:///tmp/demo", "about:blank", "data:text/plain,hi"} {
		if urlutil.IsExternalScheme(rawURL) {
			t.Fatalf("expected %q to stay internal", rawURL)
		}
	}
}
