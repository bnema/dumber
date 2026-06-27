package assets

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/andybalholm/brotli"
)

func TestWebUIAssetsIncludesCompressedSystemviewsWASM(t *testing.T) {
	compressed, err := WebUIAssets.ReadFile("systemviews/systemviews.wasm.br")
	if err != nil {
		if os.Getenv("DUMBER_REQUIRE_SYSTEMVIEWS_WASM") == "" {
			t.Skipf("generated systemviews WASM asset not present; run make build-systemviews: %v", err)
		}
		t.Fatalf("read embedded systemviews WASM asset: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatal("embedded systemviews WASM asset is empty")
	}

	reader := brotli.NewReader(bytes.NewReader(compressed))
	header := make([]byte, 8)
	if _, err := io.ReadFull(reader, header); err != nil {
		t.Fatalf("decompress embedded systemviews WASM header: %v", err)
	}

	wantHeader := []byte{0x00, 'a', 's', 'm', 0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(header, wantHeader) {
		t.Fatalf("embedded systemviews asset is not a WASM module: got header % x", header)
	}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("decompress embedded systemviews WASM body: %v", err)
	}
}
