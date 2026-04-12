package cef

import (
	"net/url"
	"testing"
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

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", raw, err)
	}
	return u
}
