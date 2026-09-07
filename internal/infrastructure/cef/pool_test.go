package cef

import (
	"context"
	"testing"

	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

func TestPoolZeroCountCreatesNothing(t *testing.T) {
	pool := &WebViewPool{}
	pool.Prewarm(0)
	pool.Prewarm(-3)
	pool.PrewarmAsync(context.Background(), 0)
	require.Equal(t, 0, pool.Size())
}

func TestPoolReserveReadinessRequiresBrowser(t *testing.T) {
	// A wrapper-only reserve (no browser yet) is never browser-ready.
	require.False(t, pooledViewBrowserReady(nil))
	require.False(t, pooledViewBrowserReady(&WebView{}))

	browser := cefmocks.NewMockBrowser(t)
	wv := &WebView{ctx: context.Background(), browser: browser}
	require.True(t, pooledViewBrowserReady(wv))

	wv.Destroy()
	require.False(t, pooledViewBrowserReady(wv), "destroyed views leave the ready set")
}

func TestPoolReleaseDestroysAndIgnoresNil(t *testing.T) {
	pool := &WebViewPool{}
	require.NotPanics(t, func() { pool.Release(nil) })

	wv := &WebView{ctx: context.Background()}
	pool.Release(wv)
	require.True(t, wv.destroyed.Load(), "Release destroys user-loaded views")
}
