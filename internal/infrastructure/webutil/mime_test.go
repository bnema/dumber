package webutil

import "testing"

func TestGetMimeType_WASM(t *testing.T) {
	t.Parallel()

	if got := GetMimeType("systemviews.wasm"); got != "application/wasm" {
		t.Fatalf("GetMimeType(systemviews.wasm) = %q, want %q", got, "application/wasm")
	}
}
