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

func TestIsStreamingManifestMIME(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "hls apple", contentType: "application/vnd.apple.mpegURL", want: true},
		{name: "hls x-mpegurl", contentType: "application/x-mpegURL", want: true},
		{name: "dash", contentType: "application/dash+xml", want: true},
		{name: "dash with charset", contentType: "application/dash+xml; charset=utf-8", want: true},
		{name: "mp4", contentType: "video/mp4", want: false},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStreamingManifestMIME(tt.contentType)
			if got != tt.want {
				t.Errorf("IsStreamingManifestMIME(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestIsStreamingManifestURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{name: "hls manifest", rawURL: "https://v.redd.it/abc/HLSPlaylist.m3u8?f=sd&v=1", want: true},
		{name: "dash manifest", rawURL: "https://example.com/video/manifest.mpd", want: true},
		{name: "uppercase extension", rawURL: "https://example.com/PLAYLIST.M3U8", want: true},
		{name: "segment ts", rawURL: "https://example.com/chunk.ts", want: false},
		{name: "plain mp4", rawURL: "https://example.com/video.mp4", want: false},
		{name: "empty", rawURL: "", want: false},
		{name: "invalid but obvious", rawURL: "not a url but playlist.m3u8", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsStreamingManifestURL(tt.rawURL)
			if got != tt.want {
				t.Errorf("IsStreamingManifestURL(%q) = %v, want %v", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestIsEagerTranscodeURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		want   bool
	}{
		{name: "hls manifest", rawURL: "https://v.redd.it/abc/HLSPlaylist.m3u8?f=sd&v=1", want: true},
		{name: "dash manifest", rawURL: "https://example.com/video/manifest.mpd", want: true},
		{name: "mp4 file", rawURL: "https://example.com/video.mp4", want: true},
		{name: "mov file", rawURL: "https://example.com/video.mov", want: true},
		{name: "mkv uppercase", rawURL: "https://example.com/VIDEO.MKV", want: true},
		{name: "hls segment", rawURL: "https://example.com/chunk.ts", want: false},
		{name: "webm open codec", rawURL: "https://example.com/video.webm", want: false},
		{name: "synthetic transcode", rawURL: "https://www.reddit.com/__dumber__/transcode.webm?src=https%3A%2F%2Fv.redd.it%2Fabc%2FHLSPlaylist.m3u8", want: true},
		{name: "empty", rawURL: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEagerTranscodeURL(tt.rawURL)
			if got != tt.want {
				t.Errorf("IsEagerTranscodeURL(%q) = %v, want %v", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestParseSyntheticTranscodeURL(t *testing.T) {
	rawURL := "https://www.reddit.com/__dumber__/transcode.webm?src=https%3A%2F%2Fv.redd.it%2Fabc%2FHLSPlaylist.m3u8&referer=https%3A%2F%2Fwww.reddit.com%2Fr%2FOpenAI&origin=https%3A%2F%2Fwww.reddit.com"

	sourceURL, referer, origin, ok := ParseSyntheticTranscodeURL(rawURL)
	if !ok {
		t.Fatalf("ParseSyntheticTranscodeURL(%q) = !ok, want ok", rawURL)
	}
	if sourceURL != "https://v.redd.it/abc/HLSPlaylist.m3u8" {
		t.Fatalf("sourceURL = %q", sourceURL)
	}
	if referer != "https://www.reddit.com/r/OpenAI" {
		t.Fatalf("referer = %q", referer)
	}
	if origin != "https://www.reddit.com" {
		t.Fatalf("origin = %q", origin)
	}
}
