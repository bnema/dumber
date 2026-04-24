package cef

import (
	"net/url"
	"testing"

	"github.com/bnema/dumber/assets"
)

func TestResolveAssetPath_SystemviewsRootsUseSystemviewsShell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		wantDir string
		wantRel string
	}{
		{name: "history host", raw: "https://dumber.invalid/history", wantDir: "systemviews", wantRel: indexHTML},
		{name: "favorites host", raw: "https://dumber.invalid/favorites", wantDir: "systemviews", wantRel: indexHTML},
		{name: "config host", raw: "https://dumber.invalid/config", wantDir: "systemviews", wantRel: indexHTML},
		{name: "root wasm exec", raw: "https://dumber.invalid/wasm_exec.js", wantDir: "systemviews", wantRel: "wasm_exec.js"},
		{name: "root wasm asset", raw: "https://dumber.invalid/systemviews.wasm", wantDir: "systemviews", wantRel: "systemviews.wasm"},
		{name: "root css asset", raw: "https://dumber.invalid/systemviews.css", wantDir: "systemviews", wantRel: "systemviews.css"},
		{name: "history opaque", raw: "dumb:history", wantDir: "systemviews", wantRel: indexHTML},
		{name: "history sub-asset", raw: "https://dumber.invalid/history/wasm_exec.js", wantDir: "systemviews", wantRel: "wasm_exec.js"},
		{name: "history css asset", raw: "https://dumber.invalid/history/systemviews.css", wantDir: "systemviews", wantRel: "systemviews.css"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			u := mustParseURL(t, tt.raw)
			gotDir, gotRel, ok := resolveAssetPath(u)
			if !ok {
				t.Fatalf("resolveAssetPath(%q) = not ok", tt.raw)
			}
			if gotDir != tt.wantDir || gotRel != tt.wantRel {
				t.Fatalf("resolveAssetPath(%q) = (%q, %q), want (%q, %q)", tt.raw, gotDir, gotRel, tt.wantDir, tt.wantRel)
			}
		})
	}
}

func TestReadAssetWithEncoding_DecompressesEmbeddedBrotliWASM(t *testing.T) {
	t.Parallel()

	data, headers, err := readAssetWithEncoding(assets.WebUIAssets, "systemviews/systemviews.wasm", "systemviews.wasm")
	if err != nil {
		t.Fatalf("readAssetWithEncoding() error = %v", err)
	}
	if len(data) < 4 || string(data[:4]) != "\x00asm" {
		t.Fatalf("decompressed wasm magic = %q, want \\x00asm", data[:4])
	}
	if _, ok := headers["Content-Encoding"]; ok {
		t.Fatalf("Content-Encoding header = %q, want absent after server-side decompression", headers["Content-Encoding"])
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return u
}

func TestSafeSystemviewsAssetPathRejectsTraversal(t *testing.T) {
	t.Parallel()

	fullPath, relPath, ok := safeSystemviewsAssetPath(systemviewsAssetDir, "nested/../systemviews.css")
	if !ok {
		t.Fatal("safeSystemviewsAssetPath() rejected safe cleaned path")
	}
	if fullPath != "systemviews/systemviews.css" || relPath != "systemviews.css" {
		t.Fatalf("safeSystemviewsAssetPath() = (%q, %q), want (systemviews/systemviews.css, systemviews.css)", fullPath, relPath)
	}

	invalid := []struct {
		name     string
		assetDir string
		relPath  string
	}{
		{name: "parent escape", assetDir: systemviewsAssetDir, relPath: "../logo.svg"},
		{name: "nested parent escape", assetDir: systemviewsAssetDir, relPath: "nested/../../logo.svg"},
		{name: "absolute parent escape", assetDir: systemviewsAssetDir, relPath: "/../logo.svg"},
		{name: "null byte", assetDir: systemviewsAssetDir, relPath: "systemviews.css\x00"},
		{name: "wrong asset dir", assetDir: "logos", relPath: "logo.svg"},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if fullPath, relPath, ok := safeSystemviewsAssetPath(tt.assetDir, tt.relPath); ok {
				t.Fatalf("safeSystemviewsAssetPath(%q, %q) = (%q, %q, true), want rejected", tt.assetDir, tt.relPath, fullPath, relPath)
			}
		})
	}
}
