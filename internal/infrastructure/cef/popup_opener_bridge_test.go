package cef

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvePopupOpenerNavigationTarget_ResolvesRelativeAgainstOpener(t *testing.T) {
	require.Equal(
		t,
		"https://example.com/oauth/callback",
		resolvePopupOpenerNavigationTarget("callback", "https://example.com/oauth/start"),
	)
}

func TestResolvePopupOpenerNavigationTarget_PreservesAbsoluteTarget(t *testing.T) {
	require.Equal(
		t,
		"https://accounts.example.com/finish",
		resolvePopupOpenerNavigationTarget("https://accounts.example.com/finish", "https://example.com/oauth/start"),
	)
}

func TestResolvePopupOpenerNavigationTarget_RejectsNonHTTPAbsoluteTarget(t *testing.T) {
	require.Empty(t, resolvePopupOpenerNavigationTarget("javascript:alert(1)", "https://example.com/oauth/start"))
	require.Empty(t, resolvePopupOpenerNavigationTarget("data:text/html,boom", "https://example.com/oauth/start"))
	require.Empty(t, resolvePopupOpenerNavigationTarget("file:///tmp/boom", "https://example.com/oauth/start"))
}

func TestOriginFromURL_PreservesNonDefaultPort(t *testing.T) {
	require.Equal(t, "https://example.com:8443", originFromURL("https://example.com:8443/callback"))
}

func TestOriginFromURL_NormalizesInternalConceptualURLs(t *testing.T) {
	require.Equal(t, actualInternalOrigin, originFromURL("dumb://home"))
}

func TestTargetOriginMatchesPopupOpener_NormalizesDefaultPorts(t *testing.T) {
	require.True(t, targetOriginMatchesPopupOpener("https://example.com:443", "https://example.com/callback"))
}
