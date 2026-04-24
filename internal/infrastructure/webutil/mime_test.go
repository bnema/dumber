package webutil

import "testing"

func TestGetMimeType_BrotliSuffixAndWASM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{name: "path input", filename: "assets/systemviews/systemviews.wasm", want: "application/wasm"},
		{name: "uppercase extension", filename: "systemviews.WASM", want: "application/wasm"},
		{name: "brotli compressed wasm", filename: "systemviews.wasm.br", want: "application/wasm"},
		{name: "uppercase brotli compressed wasm", filename: "SYSTEMVIEWS.WASM.BR", want: "application/wasm"},
		{name: "plain brotli", filename: "data.br", want: "text/plain"},
		{name: "js brotli", filename: "app.js.br", want: "text/javascript; charset=utf-8"},
		{name: "tar brotli", filename: "archive.tar.br", want: "application/x-tar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := GetMimeType(tt.filename); got != tt.want {
				t.Fatalf("GetMimeType(%s) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}
