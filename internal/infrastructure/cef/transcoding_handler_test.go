package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyMediaResponse_UsesInjectedClassifier(t *testing.T) {
	classifier := MediaClassifier{
		IsProprietaryVideoMIME:  func(string) bool { return true },
		IsOpenVideoMIME:         func(string) bool { return false },
		IsStreamingManifestMIME: func(string) bool { return false },
		IsStreamingManifestURL:  func(string) bool { return false },
	}.normalize()

	proprietary, alreadyOpen, manifest := classifyMediaResponse(classifier, "https://cdn.example/video.mp4", "video/mp4")
	require.True(t, proprietary)
	require.False(t, alreadyOpen)
	require.False(t, manifest)
}

func TestResolveTranscodeSource_UsesInjectedClassifier(t *testing.T) {
	classifier := MediaClassifier{
		ParseSyntheticTranscodeURL: func(string) (string, string, string, bool) {
			return "https://cdn.example/video.mp4", "https://example.com", "https://example.com", true
		},
		IsEagerTranscodeURL: func(string) bool { return false },
	}.normalize()

	src, referer, origin, eager := resolveTranscodeSource(classifier, "https://dumber.invalid/__dumber__/transcode.webm?src=x")
	require.True(t, eager)
	require.Equal(t, "https://cdn.example/video.mp4", src)
	require.Equal(t, "https://example.com", referer)
	require.Equal(t, "https://example.com", origin)
}
