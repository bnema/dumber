package download

import "testing"

func TestShouldForceDownloadForURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{name: "pdf path", uri: "https://example.com/file.pdf", expected: true},
		{name: "iso path", uri: "https://example.com/linux.iso?mirror=1", expected: true},
		{name: "archive path", uri: "https://example.com/release.tar.gz", expected: true},
		{name: "plain html", uri: "https://example.com/index.html", expected: false},
		{name: "empty", uri: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldForceDownloadForURI(tt.uri); got != tt.expected {
				t.Fatalf("ShouldForceDownloadForURI(%q) = %v, want %v", tt.uri, got, tt.expected)
			}
		})
	}
}

func TestShouldForceDownloadForMIMEType(t *testing.T) {
	tests := []struct {
		name     string
		mimeType string
		expected bool
	}{
		{name: "pdf", mimeType: "application/pdf", expected: true},
		{name: "pdf with params", mimeType: "application/pdf; charset=binary", expected: true},
		{name: "iso image", mimeType: "application/x-iso9660-image", expected: true},
		{name: "html", mimeType: "text/html", expected: false},
		{name: "empty", mimeType: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldForceDownloadForMIMEType(tt.mimeType); got != tt.expected {
				t.Fatalf("ShouldForceDownloadForMIMEType(%q) = %v, want %v", tt.mimeType, got, tt.expected)
			}
		})
	}
}

func TestShouldForceDownload(t *testing.T) {
	if !ShouldForceDownload("https://example.com/file.iso", "") {
		t.Fatal("expected URI extension to trigger forced download")
	}
	if !ShouldForceDownload("", "application/pdf") {
		t.Fatal("expected MIME type to trigger forced download")
	}
	if ShouldForceDownload("https://example.com/", "text/html") {
		t.Fatal("did not expect regular HTML to trigger forced download")
	}
}
