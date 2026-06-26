package usecase

import (
	"context"
	"errors"
	stdurl "net/url"
	"path/filepath"
	"testing"
)

type spyLocalPathResolver struct {
	calls int
}

func (r *spyLocalPathResolver) ResolveExistingPath(_ context.Context, _ string) (string, bool, error) {
	r.calls++
	return filepath.Join(string(filepath.Separator), "tmp", "should-not-be-used.html"), true, nil
}

func TestNavigationURLNormalizerResolvesExistingLocalPaths(t *testing.T) {
	ctx := context.Background()
	absFile := filepath.Join(string(filepath.Separator), "tmp", "page.html")
	normalizer := NewNavigationURLNormalizer(fakeLocalPathResolver{paths: map[string]string{
		absFile:          absFile,
		"page.html":      absFile,
		"~/page.html":    absFile,
		"notes:2026.txt": absFile,
		"http:notes.txt": absFile,
		"http:notes":     absFile,
	}})

	for _, input := range []string{absFile, "page.html", "~/page.html", "notes:2026.txt", "http:notes.txt", "http:notes"} {
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

func TestNavigationURLNormalizerDoesNotProbeSchemeBearingInputs(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https scheme",
			input: "https://example.com/page",
			want:  "https://example.com/page",
		},
		{
			name:  "file scheme",
			input: "file:///tmp/page.html",
			want:  "file:///tmp/page.html",
		},
		{
			name:  "about scheme",
			input: "about:blank",
			want:  "about:blank",
		},
		{
			name:  "external scheme without slashes",
			input: "mailto:user@example.com",
			want:  "mailto:user@example.com",
		},
		{
			name:  "data scheme",
			input: "data:text/plain,hello",
			want:  "data:text/plain,hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &spyLocalPathResolver{}
			normalizer := NewNavigationURLNormalizer(resolver)

			got := normalizer.Normalize(ctx, tt.input)
			if got != tt.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if resolver.calls != 0 {
				t.Fatalf("ResolveExistingPath called %d times for %q, want 0", resolver.calls, tt.input)
			}
		})
	}
}
