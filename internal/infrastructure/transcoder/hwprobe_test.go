package transcoder

import "testing"

func TestIsUsableEncoder(t *testing.T) {
	tests := []struct {
		name    string
		encoder string
		want    bool
	}{
		// Usable: open codec output that CEF can decode
		{name: "av1_vaapi", encoder: "av1_vaapi", want: true},
		{name: "vp9_vaapi", encoder: "vp9_vaapi", want: true},
		{name: "av1_nvenc", encoder: "av1_nvenc", want: true},

		// Not usable: proprietary codec output CEF cannot decode
		{name: "h264_vaapi", encoder: "h264_vaapi", want: false},
		{name: "hevc_nvenc", encoder: "hevc_nvenc", want: false},
		{name: "h264_nvenc", encoder: "h264_nvenc", want: false},

		// Software encoders are not usable either
		{name: "libx264", encoder: "libx264", want: false},
		{name: "libsvtav1", encoder: "libsvtav1", want: false},
		{name: "libvpx-vp9", encoder: "libvpx-vp9", want: false},

		// Edge cases
		{name: "empty string", encoder: "", want: false},
		{name: "random string", encoder: "not_a_codec", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUsableEncoder(tt.encoder)
			if got != tt.want {
				t.Errorf("isUsableEncoder(%q) = %v, want %v", tt.encoder, got, tt.want)
			}
		})
	}
}
