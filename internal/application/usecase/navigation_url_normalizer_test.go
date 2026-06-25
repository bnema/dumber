package usecase

import (
	"context"
	"errors"
	stdurl "net/url"
	"path/filepath"
	"testing"
)

func TestNavigationURLNormalizerResolvesExistingLocalPaths(t *testing.T) {
	ctx := context.Background()
	absFile := filepath.Join(string(filepath.Separator), "tmp", "page.html")
	normalizer := NewNavigationURLNormalizer(fakeLocalPathResolver{paths: map[string]string{
		absFile:       absFile,
		"page.html":   absFile,
		"~/page.html": absFile,
	}})

	for _, input := range []string{absFile, "page.html", "~/page.html"} {
		t.Run(input, func(t *testing.T) {
			got := normalizer.Normalize(ctx, input)
			want := (&stdurl.URL{Scheme: "file", Path: absFile}).String()
			if got != want {
				t.Fatalf("Normalize(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

func TestNavigationURLNormalizerFallsBackToDomainNormalize(t *testing.T) {
	ctx := context.Background()
	normalizer := NewNavigationURLNormalizer(fakeLocalPathResolver{err: errors.New("boom")})

	if got := normalizer.Normalize(ctx, "example.com"); got != "https://example.com" {
		t.Fatalf("Normalize(example.com) = %q", got)
	}
	if got := normalizer.Normalize(ctx, "./missing.html"); got != "./missing.html" {
		t.Fatalf("Normalize(./missing.html) = %q", got)
	}
}
