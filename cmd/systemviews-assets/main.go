// Command systemviews-assets generates and checks the content manifest
// for served systemview assets (Plan 05 P3.1).
//
// Generation (default) reads the uncompressed servables from -dir,
// requires the compressed WASM to decompress to the same bytes under a
// bounded read, and writes asset-manifest.json. It fails loudly on stale
// or missing inputs instead of writing a partial manifest.
//
// Check mode (-check) validates the committed manifest against the
// artifacts on disk and exits nonzero on any mismatch, for CI and
// release verification.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/andybalholm/brotli"

	"github.com/bnema/dumber/assets"
)

// maxManifestWASMBytes bounds manifest-time decompression, mirroring the
// serving bound so a manifest can never bless what serving would reject.
const maxManifestWASMBytes = 64 * 1024 * 1024

// manifestFilePerm matches committed repo asset files.
const manifestFilePerm = 0o644

func main() {
	dir := flag.String("dir", "assets/systemviews", "directory holding the built systemview assets")
	check := flag.Bool("check", false, "verify the committed manifest against disk artifacts instead of regenerating")
	checkIfPresent := flag.Bool("check-if-present", false, "like -check, but pass with a notice when no built WASM exists (for quick builds)")
	flag.Parse()

	if err := run(*dir, *check, *checkIfPresent); err != nil {
		fmt.Fprintf(os.Stderr, "systemviews-assets: %v\n", err)
		os.Exit(1)
	}
}

func run(dir string, check, checkIfPresent bool) error {
	manifestPath := filepath.Join(dir, assets.ManifestName)
	if checkIfPresent {
		if _, err := os.Stat(filepath.Join(dir, "systemviews.wasm")); os.IsNotExist(err) {
			fmt.Printf("no built systemviews WASM in %s; skipping manifest check\n", dir)
			return nil
		}
		return checkTree(dir, manifestPath)
	}
	if check {
		return checkTree(dir, manifestPath)
	}
	m, err := collectManifest(os.DirFS(dir))
	if err != nil {
		return err
	}
	data, err := m.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, data, manifestFilePerm); err != nil {
		return fmt.Errorf("write %s: %w", manifestPath, err)
	}
	fmt.Printf("wrote %s pinning %d assets\n", manifestPath, len(assets.ManifestFiles))
	return nil
}

// checkTree validates the committed manifest against disk artifacts:
// pins, raw bytes, and raw/compressed WASM agreement. Generation checks
// agreement at write time, but only the check path guards a later
// compressed-only swap, which serving would treat as authoritative.
func checkTree(dir, manifestPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read %s: run 'make build-systemviews' first: %w", manifestPath, err)
	}
	m, err := assets.ParseManifest(data)
	if err != nil {
		return err
	}
	fsys := os.DirFS(dir)
	if verr := m.Verify(fsys); verr != nil {
		return verr
	}
	raw, err := fs.ReadFile(fsys, "systemviews.wasm")
	if err != nil {
		return fmt.Errorf("read servable systemviews.wasm: %w", err)
	}
	if err := requireCompressedAgreement(fsys, raw); err != nil {
		return err
	}
	fmt.Printf("manifest %s matches %d pinned assets with agreeing compressed WASM\n", manifestPath, len(assets.ManifestFiles))
	return nil
}

// collectManifest pins every manifest file from fsys. The raw and
// compressed WASM must agree under bounded decompression; any missing,
// corrupt, or oversized input is a hard error.
func collectManifest(fsys fs.FS) (assets.Manifest, error) {
	files := make(map[string]assets.FileEntry, len(assets.ManifestFiles))
	for _, name := range assets.ManifestFiles {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return assets.Manifest{}, fmt.Errorf("read servable asset %s: %w", name, err)
		}
		if name == "systemviews.wasm" {
			if err := requireCompressedAgreement(fsys, data); err != nil {
				return assets.Manifest{}, err
			}
		}
		sum := sha256.Sum256(data)
		files[name] = assets.FileEntry{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	}
	return assets.Manifest{Version: assets.ManifestVersion, Files: files}, nil
}

// requireCompressedAgreement ensures the shipped .br decompresses to
// exactly the raw bytes being pinned, bounding the read before allocation.
func requireCompressedAgreement(fsys fs.FS, raw []byte) error {
	compressed, err := fs.ReadFile(fsys, "systemviews.wasm.br")
	if err != nil {
		return fmt.Errorf("read compressed systemviews.wasm.br: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(brotli.NewReader(bytes.NewReader(compressed)), maxManifestWASMBytes+1))
	if err != nil {
		return fmt.Errorf("decompress systemviews.wasm.br: %w", err)
	}
	if !bytes.Equal(data, raw) {
		return fmt.Errorf("systemviews.wasm.br decompresses to %d bytes, raw file has %d", len(data), len(raw))
	}
	return nil
}
