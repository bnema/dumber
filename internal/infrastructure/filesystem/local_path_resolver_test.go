package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExistingPath(t *testing.T) {
	ctx := context.Background()
	adapter := New()
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "page.html")
	if err := os.WriteFile(file, []byte("ok"), 0644); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	t.Setenv("HOME", tmpDir)

	tests := []struct {
		name  string
		input string
	}{
		{name: "absolute", input: file},
		{name: "relative", input: "page.html"},
		{name: "home", input: "~/page.html"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := adapter.ResolveExistingPath(ctx, tt.input)
			if err != nil {
				t.Fatalf("ResolveExistingPath returned error: %v", err)
			}
			if !ok {
				t.Fatalf("ResolveExistingPath ok=false")
			}
			if got != file {
				t.Fatalf("ResolveExistingPath(%q) = %q, want %q", tt.input, got, file)
			}
		})
	}
}

func TestResolveExistingPathMissing(t *testing.T) {
	got, ok, err := New().ResolveExistingPath(context.Background(), filepath.Join(t.TempDir(), "missing.html"))
	if err != nil {
		t.Fatalf("ResolveExistingPath returned error: %v", err)
	}
	if ok || got != "" {
		t.Fatalf("ResolveExistingPath missing = (%q, %v), want empty false", got, ok)
	}
}
