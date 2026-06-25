package usecase

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type fakeLocalPathResolver struct {
	paths map[string]string
	err   error
}

func (r fakeLocalPathResolver) ResolveExistingPath(_ context.Context, input string) (string, bool, error) {
	if r.err != nil {
		return "", false, r.err
	}
	abs, ok := r.paths[input]
	return abs, ok, nil
}

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
			want := "file://" + absFile
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
