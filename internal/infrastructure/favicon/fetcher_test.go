package favicon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	appport "github.com/bnema/dumber/internal/application/port"
	domainfavicon "github.com/bnema/dumber/internal/domain/favicon"
)

func TestFetcherExplicitIconURLAndDuckDuckGoFallback(t *testing.T) {
	png := testPNG(t, 8, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer srv.Close()
	fetcher := NewFetcherWithClient(srv.Client(), srv.URL+"/%s.ico")

	got, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: srv.URL + "/page", IconURL: srv.URL + "/icon.png"})
	if err != nil || got.ContentType != "image/png" || len(got.Bytes) == 0 {
		t.Fatalf("explicit got=%+v err=%v", got, err)
	}
	got, err = fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: "https://example.com/page"})
	if err != nil || got.Source == "" || len(got.Bytes) == 0 {
		t.Fatalf("fallback got=%+v err=%v", got, err)
	}
}

func TestFetcherResolveURLUsesHostKeyForDuckDuckGoFallback(t *testing.T) {
	fetcher := NewFetcherWithClient(http.DefaultClient, "https://icons.example/%s.ico")

	got, resolvedKey, source, err := fetcher.resolveURL(appport.FaviconFetchRequest{PageURL: "https://github.com/bnema/gordon/pull/123"})
	if err != nil {
		t.Fatal(err)
	}
	if source != domainfavicon.SourceDuckDuckGo {
		t.Fatalf("source = %q, want %q", source, domainfavicon.SourceDuckDuckGo)
	}
	if resolvedKey != "github.com" {
		t.Fatalf("resolvedKey = %q, want github.com", resolvedKey)
	}
	if got != "https://icons.example/github.com.ico" {
		t.Fatalf("duckduckgo URL = %q, want host-based fallback URL", got)
	}
}

func TestFetcherMissesAndGuards(t *testing.T) {
	fetcher := NewFetcher()
	if _, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: "https://example.com", IconURL: "file:///x"}); !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("scheme err=%v", err)
	}
	if _, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: "https://example.com", IconURL: "http://127.0.0.1/icon.png"}); !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("ssrf err=%v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusNotFound) }))
	defer srv.Close()
	fetcher = NewFetcherWithClient(srv.Client(), "")
	if _, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: srv.URL, IconURL: srv.URL + "/missing"}); !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("404 err=%v", err)
	}
}

func TestFetcherRejectsMismatchedDeclaredContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("<html>not a png</html>"))
	}))
	defer srv.Close()
	fetcher := NewFetcherWithClient(srv.Client(), "")
	if _, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: srv.URL, IconURL: srv.URL + "/icon.png"}); !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("mismatched content err=%v", err)
	}
}

func TestFetcherLocalhostExceptionRequiresSameOrigin(t *testing.T) {
	page := mustParseURL(t, "http://localhost:3000/page")
	same := mustParseURL(t, "http://localhost:3000/favicon.ico")
	otherPort := mustParseURL(t, "http://localhost:8080/favicon.ico")
	otherScheme := mustParseURL(t, "https://localhost:3000/favicon.ico")
	defaultPortPage := mustParseURL(t, "http://localhost/page")
	defaultPortIcon := mustParseURL(t, "http://localhost:80/favicon.ico")
	if !allowedURLForPage(page.String(), same) {
		t.Fatal("same-origin localhost should be allowed")
	}
	if allowedURLForPage(page.String(), otherPort) {
		t.Fatal("different localhost port should be rejected")
	}
	if allowedURLForPage(page.String(), otherScheme) {
		t.Fatal("different localhost scheme should be rejected")
	}
	if !allowedURLForPage(defaultPortPage.String(), defaultPortIcon) {
		t.Fatal("default localhost port should normalize as same-origin")
	}
	if got := allowedLocalOrigin(defaultPortPage.String()); got != "localhost:80" {
		t.Fatalf("allowedLocalOrigin default port = %q, want localhost:80", got)
	}
	if !allowedURLForOrigin(context.WithValue(context.Background(), allowedLocalOriginKey, allowedLocalOrigin(defaultPortPage.String())), mustParseURL(t, "http://localhost/favicon.ico")) {
		t.Fatal("same-origin localhost redirect with implicit default port should be allowed")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestFetcherMaxSizeAndRedirectLimit(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(make([]byte, 20))
	}))
	defer big.Close()
	fetcher := NewFetcherWithClient(big.Client(), "")
	fetcher.maxBytes = 4
	if _, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: big.URL, IconURL: big.URL}); err == nil {
		t.Fatal("expected max size error")
	}

	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("/r%s", r.URL.Path), http.StatusFound)
	}))
	defer redir.Close()
	fetcher = NewFetcherWithClient(redir.Client(), "")
	if _, err := fetcher.Fetch(context.Background(), appport.FaviconFetchRequest{PageURL: redir.URL, IconURL: redir.URL + "/r"}); !errors.Is(err, appport.ErrFaviconMiss) {
		t.Fatalf("redirect err=%v", err)
	}
}
