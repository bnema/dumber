package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFaviconDiscoveryResolvesLinksAndFallback(t *testing.T) {
	pageURL := "https://example.com/docs/page.html"
	html := `<html><head>
		<link rel="stylesheet" href="/style.css">
		<link rel="icon" href="/favicon.png">
		<link rel="shortcut icon" href="icons/shortcut.ico">
		<link rel="apple-touch-icon" href="https://cdn.example.com/apple.png">
		<link rel="icon" href="javascript:alert(1)">
	</head></html>`

	got := DiscoverFaviconCandidates(pageURL, html)
	require.Equal(t, []string{
		"https://example.com/favicon.png",
		"https://example.com/docs/icons/shortcut.ico",
		"https://cdn.example.com/apple.png",
		"https://example.com/favicon.ico",
	}, got)
}

func TestFaviconDiscoveryFallbackOnly(t *testing.T) {
	require.Equal(t, []string{"https://example.com/favicon.ico"}, DiscoverFaviconCandidates("https://example.com/path", "<html></html>"))
	require.Nil(t, DiscoverFaviconCandidates("about:blank", "<html></html>"))
}

func TestFaviconDiscoveryKeepsAbsoluteIconsForOpaquePageURL(t *testing.T) {
	got := DiscoverFaviconCandidates("about:blank", `<link rel="icon" href="https://cdn.example.com/icon.png">`)
	require.Equal(t, []string{"https://cdn.example.com/icon.png"}, got)
}

func TestFaviconDiscoveryIgnoresUnsafeIconURLs(t *testing.T) {
	got := DiscoverFaviconCandidates("https://example.com/path", `<link rel="icon" href="data:image/png;base64,AAAA"><link rel="icon" href="//cdn.example.com/icon.png">`)
	require.Equal(t, []string{"https://cdn.example.com/icon.png", "https://example.com/favicon.ico"}, got)
}

func TestCEFOnFaviconUrlchangeForwardsOrderedCandidates(t *testing.T) {
	prevDecode := decodeCEFStringList
	decodeCEFStringList = func(list purecef.StringList) []string {
		require.Equal(t, purecef.StringList(42), list)
		return []string{"/favicon.png", "https://cdn.example.com/icon.ico", "javascript:alert(1)"}
	}
	defer func() { decodeCEFStringList = prevDecode }()

	var page string
	var icons []string
	wv := &WebView{ctx: context.Background(), uri: "https://example.com/docs/page", callbacks: &port.WebViewCallbacks{
		OnFaviconURLChanged: func(p string, urls []string) { page, icons = p, urls },
	}}

	(&handlerSet{wv: wv}).OnFaviconUrlchange(nil, purecef.StringList(42))

	require.Equal(t, "https://example.com/docs/page", page)
	require.Equal(t, []string{"https://example.com/favicon.png", "https://cdn.example.com/icon.ico"}, icons)
}

func TestCEFLoadingFinishedDiscoversMetadataBeforeFallback(t *testing.T) {
	prevNewStringVisitor := cefNewStringVisitor
	cefNewStringVisitor = func(visitor purecef.StringVisitor) purecef.StringVisitor { return visitor }
	defer func() { cefNewStringVisitor = prevNewStringVisitor }()

	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame)
	frame.EXPECT().GetSource(mock.Anything).Run(func(visitor purecef.StringVisitor) {
		visitor.Visit(`<html><head><link rel="icon" href="/meta.png"></head></html>`)
	}).Return()

	var page string
	var icons []string
	wv := &WebView{ctx: context.Background(), uri: "https://example.com/docs/page", callbacks: &port.WebViewCallbacks{
		OnFaviconURLChanged: func(p string, urls []string) { page, icons = p, urls },
	}}

	(&handlerSet{wv: wv}).OnLoadingStateChange(browser, 0, 1, 0)

	require.Equal(t, "https://example.com/docs/page", page)
	require.Equal(t, []string{"https://example.com/meta.png", "https://example.com/favicon.ico"}, icons)
}

func TestCEFLoadingFinishedForwardsFallbackCandidate(t *testing.T) {
	var page string
	var icons []string
	wv := &WebView{ctx: context.Background(), uri: "https://example.com/docs/page", callbacks: &port.WebViewCallbacks{
		OnFaviconURLChanged: func(p string, urls []string) { page, icons = p, urls },
	}}

	(&handlerSet{wv: wv}).OnLoadingStateChange(nil, 0, 1, 0)

	require.Equal(t, "https://example.com/docs/page", page)
	require.Equal(t, []string{"https://example.com/favicon.ico"}, icons)
}
