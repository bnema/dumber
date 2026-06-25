package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/infrastructure/filesystem"
)

func TestParseSelectionResolvesExistingLocalFile(t *testing.T) {
	oldApp := app
	t.Cleanup(func() { app = oldApp })
	localPaths := filesystem.New()
	app = &cli.App{
		Config:                  config.DefaultConfig(),
		LocalPaths:              localPaths,
		NavigationURLNormalizer: usecase.NewNavigationURLNormalizer(localPaths),
	}

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

	if got := parseSelection("page.html"); got != "file://"+file {
		t.Fatalf("parseSelection(page.html) = %q, want file URL", got)
	}
}

func TestParseSelectionLeavesBangShortcutsToSearchHandling(t *testing.T) {
	oldApp := app
	t.Cleanup(func() { app = oldApp })
	cfg := config.DefaultConfig()
	cfg.DefaultSearchEngine = "https://search.example/?q=%s"
	app = &cli.App{Config: cfg}

	if got := parseSelection("!unknown page.html"); got != "https://search.example/?q=!unknown page.html" {
		t.Fatalf("parseSelection(unknown bang) = %q, want search fallback", got)
	}
}
