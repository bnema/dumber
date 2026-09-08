package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/andybalholm/brotli"

	"github.com/bnema/dumber/assets"
)

func writeFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func fixtureBundle(t *testing.T, wasm, js, css []byte) string {
	t.Helper()
	dir := t.TempDir()
	writeFixture(t, dir, "systemviews.wasm", wasm)
	writeFixture(t, dir, "systemviews.wasm.br", brotliCompressForGeneratorTest(t, wasm))
	writeFixture(t, dir, "wasm_exec.js", js)
	writeFixture(t, dir, "systemviews.css", css)
	return dir
}

func brotliCompressForGeneratorTest(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatalf("brotli write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("brotli close: %v", err)
	}
	return buf.Bytes()
}

func TestCollectManifestPinsAllFiles(t *testing.T) {
	t.Parallel()

	dir := fixtureBundle(t, []byte("\x00asm-m"), []byte("/* js */"), []byte("/* css */"))
	m, err := collectManifest(os.DirFS(dir))
	if err != nil {
		t.Fatalf("collectManifest() error = %v", err)
	}
	if m.Version != assets.ManifestVersion {
		t.Fatalf("version = %d", m.Version)
	}
	if len(m.Files) != len(assets.ManifestFiles) {
		t.Fatalf("pinned %d files", len(m.Files))
	}
	if err := m.Verify(os.DirFS(dir)); err != nil {
		t.Fatalf("collected manifest does not verify: %v", err)
	}
}

func TestRunWritesReproducibleManifest(t *testing.T) {
	t.Parallel()

	dir := fixtureBundle(t, []byte("\x00asm-repro"), []byte("/* js */"), []byte("/* css */"))
	if err := run(dir, false, false); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, assets.ManifestName))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if err := run(dir, false, false); err != nil {
		t.Fatalf("run() again error = %v", err)
	}
	second, readErr := os.ReadFile(filepath.Join(dir, assets.ManifestName))
	if readErr != nil {
		t.Fatalf("read manifest: %v", readErr)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("manifest generation is not reproducible")
	}
	if err := run(dir, true, false); err != nil {
		t.Fatalf("run(check) error = %v", err)
	}
}

func TestRunFailsOnMissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFixture(t, dir, "systemviews.wasm", []byte("\x00asm"))
	writeFixture(t, dir, "systemviews.wasm.br", brotliCompressForGeneratorTest(t, []byte("\x00asm")))
	// wasm_exec.js and systemviews.css missing: loud failure, no manifest.
	if err := run(dir, false, false); err == nil {
		t.Fatal("run() with missing files succeeded, want error")
	}
	if _, err := os.Stat(filepath.Join(dir, assets.ManifestName)); !os.IsNotExist(err) {
		t.Fatal("partial manifest written on failure")
	}
}

func TestRunFailsOnStaleDigest(t *testing.T) {
	t.Parallel()

	dir := fixtureBundle(t, []byte("\x00asm-v1"), []byte("/* js */"), []byte("/* css */"))
	if err := run(dir, false, false); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	// Mutate an artifact after generation: check must fail, never bless it.
	writeFixture(t, dir, "systemviews.css", []byte("/* css v2 */"))
	if err := run(dir, true, false); err == nil {
		t.Fatal("run(check) with mutated artifact succeeded, want error")
	}
}

func TestCollectManifestRejectsRawCompressedMismatch(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"systemviews.wasm":    {Data: []byte("\x00asm-raw")},
		"systemviews.wasm.br": {Data: brotliCompressForGeneratorTest(t, []byte("\x00asm-other"))},
		"wasm_exec.js":        {Data: []byte("/* js */")},
		"systemviews.css":     {Data: []byte("/* css */")},
	}
	if _, err := collectManifest(fsys); err == nil {
		t.Fatal("collectManifest() with raw/compressed mismatch succeeded, want error")
	}
}

func TestRunCheckFailsWithoutManifest(t *testing.T) {
	t.Parallel()

	if err := run(t.TempDir(), true, false); err == nil {
		t.Fatal("run(check) without manifest succeeded, want error")
	}
}

func TestRunCheckRejectsCompressedOnlySwap(t *testing.T) {
	t.Parallel()

	dir := fixtureBundle(t, []byte("\x00asm-v1"), []byte("/* js */"), []byte("/* css */"))
	if err := run(dir, false, false); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	// Swap only the compressed artifact for different valid bytes: raw
	// and manifest still agree, but serving would decode the stale .br.
	writeFixture(t, dir, "systemviews.wasm.br", brotliCompressForGeneratorTest(t, []byte("\x00asm-v2")))
	if err := run(dir, true, false); err == nil {
		t.Fatal("run(check) with swapped compressed WASM succeeded, want error")
	}
}

func TestRunCheckIfPresentSkipsMissingBundle(t *testing.T) {
	t.Parallel()

	if err := run(t.TempDir(), false, true); err != nil {
		t.Fatalf("run(check-if-present) without bundle error = %v", err)
	}
}

func TestRunCheckIfPresentRejectsStaleBundle(t *testing.T) {
	t.Parallel()

	dir := fixtureBundle(t, []byte("\x00asm-v1"), []byte("/* js */"), []byte("/* css */"))
	if err := run(dir, false, false); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	writeFixture(t, dir, "systemviews.css", []byte("/* css v2 */"))
	if err := run(dir, false, true); err == nil {
		t.Fatal("run(check-if-present) with stale artifact succeeded, want error")
	}
}

func TestRunCheckIfPresentRejectsPartialBundle(t *testing.T) {
	t.Parallel()

	// Compressed-only partial bundle: serving would use the stale .br,
	// so check-if-present must validate (and fail) instead of skipping.
	dir := t.TempDir()
	writeFixture(t, dir, "systemviews.wasm.br", brotliCompressForGeneratorTest(t, []byte("\x00asm-stale")))
	if err := run(dir, false, true); err == nil {
		t.Fatal("run(check-if-present) with .br-only bundle succeeded, want error")
	}

	// Raw-only partial bundle: same rule in the other direction.
	dir = t.TempDir()
	writeFixture(t, dir, "systemviews.wasm", []byte("\x00asm-raw-only"))
	if err := run(dir, false, true); err == nil {
		t.Fatal("run(check-if-present) with raw-only bundle succeeded, want error")
	}
}

func TestRequireCompressedAgreementRejectsSentinelSize(t *testing.T) {
	t.Parallel()

	// Both artifacts at max+1 with agreeing bytes: the bound must reject
	// before equality blesses what the runtime loader refuses.
	const bound = 16
	raw := bytes.Repeat([]byte{0x7a}, bound+1)
	fsys := fstest.MapFS{
		"systemviews.wasm":    {Data: raw},
		"systemviews.wasm.br": {Data: brotliCompressForGeneratorTest(t, raw)},
	}
	if err := requireCompressedAgreement(fsys, raw, bound); err == nil {
		t.Fatal("requireCompressedAgreement at max+1 succeeded, want oversize error")
	}
	if _, err := readBoundedManifestFile(fsys, "systemviews.wasm", bound); err == nil {
		t.Fatal("readBoundedManifestFile at max+1 succeeded, want oversize error")
	}
}
