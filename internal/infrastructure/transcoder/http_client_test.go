package transcoder

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
)

func TestValidateHTTPTranscodeURLRejectsPrivateTargets(t *testing.T) {
	source, err := url.Parse("http://127.0.0.1/video.mp4")
	if err != nil {
		t.Fatal(err)
	}

	err = validateHTTPTranscodeURL(context.Background(), source)
	if err == nil {
		t.Fatal("validateHTTPTranscodeURL() expected error for loopback target")
	}
}

func TestIsBlockedTranscodeIPRejectsReservedInternalRanges(t *testing.T) {
	tests := []string{
		"100.64.0.1",
		"198.18.0.1",
		"192.0.2.1",
		"64:ff9b::a00:1",
		"2001::1",
		"2001:db8::1",
		"2002:0a00:0001::1",
	}
	for _, rawIP := range tests {
		t.Run(rawIP, func(t *testing.T) {
			if !isBlockedTranscodeIP(net.ParseIP(rawIP)) {
				t.Fatalf("isBlockedTranscodeIP(%s) = false, want true", rawIP)
			}
		})
	}
}

func TestValidateHTTPTranscodeURLRejectsUnsupportedSchemes(t *testing.T) {
	source, err := url.Parse("file:///etc/passwd")
	if err != nil {
		t.Fatal(err)
	}

	err = validateHTTPTranscodeURL(context.Background(), source)
	if err == nil {
		t.Fatal("validateHTTPTranscodeURL() expected error for unsupported scheme")
	}
}

func TestValidateHTTPTranscodeURLRejectsStreamingManifestURL(t *testing.T) {
	source, err := url.Parse("https://93.184.216.34/playlist.m3u8")
	if err != nil {
		t.Fatal(err)
	}

	err = validateHTTPTranscodeURL(context.Background(), source)
	if err == nil {
		t.Fatal("validateHTTPTranscodeURL() expected error for streaming manifest")
	}
}

func TestValidateHTTPTranscodeURLAllowsPublicIPAddress(t *testing.T) {
	source, err := url.Parse("https://93.184.216.34/video.mp4")
	if err != nil {
		t.Fatal(err)
	}

	if err := validateHTTPTranscodeURL(context.Background(), source); err != nil {
		t.Fatalf("validateHTTPTranscodeURL() unexpected error: %v", err)
	}
}

func TestOpenInputFormatContextRejectsStreamingManifests(t *testing.T) {
	p := newPipeline("test", port.HWCapabilities{}, "https://example.com/playlist.m3u8", nil, "", nil, zerolog.Nop())

	_, _, err := p.openInputFormatContext(context.Background())
	if err == nil {
		t.Fatal("openInputFormatContext() expected error for streaming manifest")
	}
}

func TestIsStreamingManifestResponseDetectsContentType(t *testing.T) {
	resp := &http.Response{Header: http.Header{"Content-Type": []string{"application/vnd.apple.mpegurl"}}}
	if !isStreamingManifestResponse(resp) {
		t.Fatal("isStreamingManifestResponse() = false, want true")
	}
}

func TestValidateTranscodeRedirectRejectsPrivateTarget(t *testing.T) {
	requestURL, err := url.Parse("http://10.0.0.1/private.mp4")
	if err != nil {
		t.Fatal(err)
	}

	err = validateTranscodeRedirect(&http.Request{URL: requestURL}, nil)
	if err == nil {
		t.Fatal("validateTranscodeRedirect() expected error for private redirect target")
	}
}

func TestValidateTranscodeRedirectRejectsStreamingManifestTarget(t *testing.T) {
	requestURL, err := url.Parse("https://93.184.216.34/playlist.m3u8")
	if err != nil {
		t.Fatal(err)
	}

	err = validateTranscodeRedirect(&http.Request{URL: requestURL}, nil)
	if err == nil {
		t.Fatal("validateTranscodeRedirect() expected error for manifest redirect target")
	}
}
