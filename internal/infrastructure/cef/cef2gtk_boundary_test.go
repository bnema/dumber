package cef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCEF2GTKImportStaysInInfrastructureCEF(t *testing.T) {
	root := filepath.Clean("../../..")
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "dist", "tmp":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), "github.com/bnema/purego-cef2gtk") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(rel), "internal/infrastructure/cef/") {
			return nil
		}
		t.Fatalf("purego-cef2gtk imported outside cef infrastructure: %s", rel)
		return nil
	}); err != nil {
		t.Fatalf("scan imports: %v", err)
	}
}
