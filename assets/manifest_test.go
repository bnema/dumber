package assets

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestParseManifestRejectsBadInputs(t *testing.T) {
	t.Parallel()

	good := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	pin := func(digest string, size int) string {
		return fmt.Sprintf(`"sha256":%q,"size":%d`, digest, size)
	}
	entry := func(css, wasm, js string) string {
		return `{"version":1,"files":{"systemviews.css":{` + css + `},"systemviews.wasm":{` + wasm + `},"wasm_exec.js":{` + js + `}}}`
	}
	valid := entry(pin(good, 1), pin(good, 2), pin(good, 3))
	if _, err := ParseManifest([]byte(valid)); err != nil {
		t.Fatalf("ParseManifest(valid) error = %v", err)
	}

	cases := map[string]string{
		"bad version":  strings.Replace(valid, `"version":1`, `"version":2`, 1),
		"missing pin":  `{"version":1,"files":{"systemviews.css":{` + pin(good, 1) + `},"systemviews.wasm":{` + pin(good, 2) + `}}}`,
		"unknown file": strings.TrimSuffix(valid, `}`) + `,"evil.js":{` + pin(good, 4) + `}}`,
		"empty digest": entry(pin("", 1), pin(good, 2), pin(good, 3)),
		"short digest": entry(pin("abc", 1), pin(good, 2), pin(good, 3)),
		"long digest":  entry(pin(good+"00", 1), pin(good, 2), pin(good, 3)),
		"nonhex digest": entry(pin(strings.Repeat("g", 64), 1), pin(good, 2),
			pin(good, 3)),
		"upper digest": entry(pin(strings.Repeat("A", 64), 1), pin(good, 2),
			pin(good, 3)),
		// Pins breaking out of the shell's quoted attributes must fail
		// closed instead of reaching HTML/JS interpolation.
		"quote breakout": entry(pin(good, 1),
			pin("');alert(1);//"+strings.Repeat("0", 52), 2), pin(good, 3)),
		"markup pin": entry(pin(good, 1),
			pin("<script>alert(1)</"+"script>"+strings.Repeat("0", 39), 2), pin(good, 3)),
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

	good := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	m := Manifest{Version: ManifestVersion, Files: map[string]FileEntry{
		"wasm_exec.js":     {SHA256: good, Size: 3},
		"systemviews.wasm": {SHA256: good, Size: 2},
		"systemviews.css":  {SHA256: good, Size: 1},
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
// against the actually embedded artifacts, including raw/compressed WASM
// agreement. It skips when the generated bundle is absent, mirroring the
// existing WASM presence test.
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
	// The manifest pins raw bytes, but serving decodes the compressed
	// artifact as authoritative: the embedded .br must agree too, or a
	// stale compressed bundle would pass verification yet fail serving.
	if err := verifyEmbeddedCompressedAgreement(sub, m); err != nil {
		t.Fatalf("embedded compressed WASM disagrees: %v", err)
	}
}

// verifyEmbeddedCompressedAgreement checks the embedded .br decodes to
// exactly the pinned raw bytes under a bounded read. Test-only: production
// agreement checks live in cmd/systemviews-assets; this keeps the assets
// package dependency-free.
func verifyEmbeddedCompressedAgreement(fsys fs.FS, m Manifest) error {
	want := m.Files["systemviews.wasm"]
	compressed, err := fs.ReadFile(fsys, "systemviews.wasm.br")
	if err != nil {
		return fmt.Errorf("read embedded systemviews.wasm.br: %w", err)
	}
	raw, err := fs.ReadFile(fsys, "systemviews.wasm")
	if err != nil {
		return fmt.Errorf("read embedded systemviews.wasm: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), 64*1024*1024+1))
	if err != nil {
		return fmt.Errorf("decompress embedded systemviews.wasm.br: %w", err)
	}
	if !bytes.Equal(data, raw) || int64(len(raw)) != want.Size {
		return fmt.Errorf("embedded systemviews.wasm.br does not match pinned raw bytes")
	}
	return nil
}
