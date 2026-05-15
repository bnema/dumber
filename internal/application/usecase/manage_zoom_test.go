package usecase

import "testing"

func TestExtractZoomKey(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		want    string
		wantErr bool
	}{
		{
			name:   "host based url uses host",
			rawURL: "https://example.com/docs?q=1#top",
			want:   "example.com",
		},
		{
			name:   "file url uses canonical file uri without query or fragment",
			rawURL: "file:///tmp/demo.html?print=1#section",
			want:   "file:///tmp/demo.html",
		},
		{
			name:    "about blank has no zoom key",
			rawURL:  "about:blank",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractZoomKey(tt.rawURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExtractZoomKey(%q) error = nil, want error", tt.rawURL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractZoomKey(%q) error = %v", tt.rawURL, err)
			}
			if got != tt.want {
				t.Fatalf("ExtractZoomKey(%q) = %q, want %q", tt.rawURL, got, tt.want)
			}
		})
	}
}
