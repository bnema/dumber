package transcoder

import "testing"

func TestIsProprietaryVideoMIME(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		// Proprietary formats
		{name: "mp4", contentType: "video/mp4", want: true},
		{name: "mp4 with codecs param", contentType: "video/mp4; codecs=\"avc1.42E01E\"", want: true},
		{name: "mp4 with charset", contentType: "video/mp4; charset=utf-8", want: true},
		{name: "flv", contentType: "video/x-flv", want: true},
		{name: "3gpp", contentType: "video/3gpp", want: true},
		{name: "3gpp2", contentType: "video/3gpp2", want: true},
		{name: "mpeg-ts", contentType: "video/mp2t", want: true},
		{name: "mpeg", contentType: "video/mpeg", want: true},
		{name: "avi", contentType: "video/x-msvideo", want: true},
		{name: "mkv", contentType: "video/x-matroska", want: true},
		{name: "quicktime", contentType: "video/quicktime", want: true},
		{name: "m4v", contentType: "video/x-m4v", want: true},

		// Open formats (should be false)
		{name: "webm", contentType: "video/webm", want: false},
		{name: "ogg", contentType: "video/ogg", want: false},

		// Non-video (should be false)
		{name: "audio mp3", contentType: "audio/mpeg", want: false},
		{name: "text html", contentType: "text/html", want: false},
		{name: "application json", contentType: "application/json", want: false},
		{name: "octet-stream", contentType: "application/octet-stream", want: false},
		{name: "empty", contentType: "", want: false},

		// Case insensitivity
		{name: "mp4 uppercase", contentType: "Video/MP4", want: true},
		{name: "webm uppercase", contentType: "Video/WebM", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProprietaryVideoMIME(tt.contentType)
			if got != tt.want {
				t.Errorf("IsProprietaryVideoMIME(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestIsOpenVideoMIME(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		// Open formats
		{name: "webm", contentType: "video/webm", want: true},
		{name: "webm with codecs", contentType: "video/webm; codecs=\"vp9,opus\"", want: true},
		{name: "ogg", contentType: "video/ogg", want: true},

		// Proprietary formats (should be false)
		{name: "mp4", contentType: "video/mp4", want: false},
		{name: "flv", contentType: "video/x-flv", want: false},
		{name: "mkv", contentType: "video/x-matroska", want: false},

		// Non-video (should be false)
		{name: "audio ogg", contentType: "audio/ogg", want: false},
		{name: "text html", contentType: "text/html", want: false},
		{name: "octet-stream", contentType: "application/octet-stream", want: false},
		{name: "empty", contentType: "", want: false},

		// Case insensitivity
		{name: "webm uppercase", contentType: "Video/WebM", want: true},
		{name: "ogg uppercase", contentType: "VIDEO/OGG", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsOpenVideoMIME(tt.contentType)
			if got != tt.want {
				t.Errorf("IsOpenVideoMIME(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}
