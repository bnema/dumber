package assets

import (
	"bytes"
	"io/fs"
	"os"
	"testing"
)

func TestParseManifestRejectsBadInputs(t *testing.T) {
	t.Parallel()

	valid := `{"version":1,"files":{"systemviews.css":{"sha256":"a","size":1},"systemviews.wasm":{"sha256":"b","size":2},"wasm_exec.js":{"sha256":"c","size":3}}}`
	if _, err := ParseManifest([]byte(valid)); err != nil {
		t.Fatalf("ParseManifest(valid) error = %v", err)
	}

	cases := map[string]string{
		"bad version":   `{"version":2,"files":{"systemviews.css":{"sha256":"a","size":1},"systemviews.wasm":{"sha256":"b","size":2},"wasm_exec.js":{"sha256":"c","size":3}}}`,
		"missing pin":   `{"version":1,"files":{"systemviews.css":{"sha256":"a","size":1},"systemviews.wasm":{"sha256":"b","size":2}}}`,
		"unknown file":  `{"version":1,"files":{"systemviews.css":{"sha256":"a","size":1},"systemviews.wasm":{"sha256":"b","size":2},"wasm_exec.js":{"sha256":"c","size":3},"evil.js":{"sha256":"d","size":4}}}`,
		"empty digest":  `{"version":1,"files":{"systemviews.css":{"sha256":"","size":1},"systemviews.wasm":{"sha256":"b","size":2},"wasm_exec.js":{"sha256":"c","size":3}}}`,
		"not JSON":      `not json`,
		"trailing data": valid + `{}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseManifest([]byte(input)); err == nil {
				t.Fatalf("ParseManifest(%s) succeeded, want error", name)
			}
		})
	}
}

func TestManifestBytesAreCanonical(t *testing.T) {
	t.Parallel()

	m := Manifest{Version: ManifestVersion, Files: map[string]FileEntry{
		"wasm_exec.js":    {SHA256: "c", Size: 3},
		"systemviews.wasm": {SHA256: "b", Size: 2},
		"systemviews.css": {SHA256: "a", Size: 1},
	}}
	first, err := m.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	second, err := m.Bytes()
	if err != nil {
		t.Fatalf("Bytes() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("manifest encoding is not reproducible")
	}
	if !bytes.HasSuffix(first, []byte("\n")) || bytes.Contains(first, []byte("1970")) {
		t.Fatalf("manifest must end with newline and carry no timestamps:\n%s", first)
	}
	// Sorted keys: css < wasm < wasm_exec.
	css := bytes.Index(first, []byte("systemviews.css"))
	wasm := bytes.Index(first, []byte("systemviews.wasm"))
	js := bytes.Index(first, []byte("wasm_exec.js"))
	if css >= wasm || wasm >= js {
		t.Fatalf("manifest keys not sorted:\n%s", first)
	}
	if _, err := ParseManifest(first); err != nil {
		t.Fatalf("canonical bytes do not round-trip: %v", err)
	}
}

// TestAssetManifestMatchesEmbeddedArtifacts validates every manifest entry
// against the actually embedded artifacts. It skips when the generated
// bundle is absent, mirroring the existing WASM presence test.
func TestAssetManifestMatchesEmbeddedArtifacts(t *testing.T) {
	data, err := WebUIAssets.ReadFile("systemviews/" + ManifestName)
	if err != nil {
		if os.Getenv("DUMBER_REQUIRE_SYSTEMVIEWS_WASM") == "" {
			t.Skipf("generated asset manifest not present; run make build-systemviews: %v", err)
		}
		t.Fatalf("read embedded asset manifest: %v", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest(embedded) error = %v", err)
	}
	sub, err := fs.Sub(WebUIAssets, "systemviews")
	if err != nil {
		t.Fatalf("fs.Sub(embedded systemviews) error = %v", err)
	}
	if err := m.Verify(sub); err != nil {
		t.Fatalf("embedded artifacts do not match manifest: %v", err)
	}
}
