package cef

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	purecef "bnema.dev/purego-cef/cef"
	cefmocks "bnema.dev/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testHomeTop      = "https://dumber.invalid/home"
	testEmbedLAN     = "http://192.168.1.10:3000"
	testPublicEmbed  = "https://public.example"
	testLoopbackDest = "http://127.0.0.1:8081/secret"
)

func testLocalNetworkScope() *localNetworkScope {
	s := newLocalNetworkScope()
	s.setOrigins([]string{testEmbedLAN + "/d/x", "https://Grafana.LAN:443/"})
	return s
}

func TestLocalNetworkScopeDecide(t *testing.T) {
	t.Parallel()
	const (
		sub   = purecef.ResourceTypeRtSubResource
		frame = purecef.ResourceTypeRtSubFrame
		main  = purecef.ResourceTypeRtMainFrame
		xhr   = purecef.ResourceTypeRtXhr
	)
	cases := []struct {
		name      string
		top       string
		url       string
		initiator string
		rt        purecef.ResourceType
		want      localNetworkVerdict
	}{
		{"regular page is out of scope", "https://site.example/", testLoopbackDest, "https://site.example", sub, localNetworkAllow},
		{"top navigation is out of scope", testHomeTop, testLoopbackDest, "", main, localNetworkAllow},
		{"system view document itself", testHomeTop, "http://10.0.0.9/poster.png", "https://dumber.invalid", sub, localNetworkAllow},
		{"internal host", testHomeTop, "https://dumber.invalid/api/x", "https://public.example", xhr, localNetworkAllow},
		{"configured embed loads", testHomeTop, testEmbedLAN + "/d/x", "https://dumber.invalid", frame, localNetworkAllow},
		{"configured embed subresource", testHomeTop, testEmbedLAN + "/api/ds", testEmbedLAN, xhr, localNetworkAllow},
		{"configured embed origin canonicalized", testHomeTop, "https://grafana.lan/api", "https://grafana.lan", xhr, localNetworkAllow},
		{"configured embed to other LAN host", testHomeTop, "http://10.0.0.5/x", testEmbedLAN, xhr, localNetworkAllow},
		{"public embed to loopback", testHomeTop, testLoopbackDest, testPublicEmbed, xhr, localNetworkBlock},
		{"public embed to LAN", testHomeTop, "http://192.168.1.10:3000/api", testPublicEmbed, sub, localNetworkBlock},
		{"public embed to IPv6 loopback", testHomeTop, "http://[::1]:8081/", testPublicEmbed, xhr, localNetworkBlock},
		{"public embed to localhost", testHomeTop, "http://localhost:8081/", testPublicEmbed, xhr, localNetworkBlock},
		{"public embed to sub.localhost", testHomeTop, "http://a.localhost/", testPublicEmbed, xhr, localNetworkBlock},
		{"public embed websocket to LAN", testHomeTop, "ws://10.0.0.2/socket", testPublicEmbed, xhr, localNetworkBlock},
		{"public embed to public IP", testHomeTop, "https://8.8.8.8/", testPublicEmbed, xhr, localNetworkAllow},
		{"public embed to hostname", testHomeTop, "https://cdn.example/x.js", testPublicEmbed, sub, localNetworkResolve},
		{"embed navigates its frame to loopback", testHomeTop, testLoopbackDest, testEmbedLAN, frame, localNetworkBlock},
		{"embed navigates its frame to unlisted LAN port", testHomeTop, "http://192.168.1.10:3001/", testEmbedLAN, frame, localNetworkBlock},
		{"opaque initiator", testHomeTop, testLoopbackDest, "null", xhr, localNetworkBlock},
		{"missing initiator", testHomeTop, testLoopbackDest, "", xhr, localNetworkBlock},
		{"data URL", testHomeTop, "data:text/plain,x", testPublicEmbed, sub, localNetworkAllow},
		{"unparsable URL", testHomeTop, "http://[::1", testPublicEmbed, xhr, localNetworkBlock},
	}
	scope := testLocalNetworkScope()
	for _, tc := range cases {
		assert.Equal(t, tc.want, scope.decide(tc.top, tc.url, tc.initiator, tc.rt), tc.name)
	}
}

func TestLocalNetworkScopeWithoutOriginsBlocksLAN(t *testing.T) {
	t.Parallel()
	scope := newLocalNetworkScope()
	assert.Equal(t, localNetworkBlock,
		scope.decide(testHomeTop, testEmbedLAN+"/x", testEmbedLAN, purecef.ResourceTypeRtXhr))
	scope.setOrigins([]string{testEmbedLAN})
	assert.Equal(t, localNetworkAllow,
		scope.decide(testHomeTop, testEmbedLAN+"/x", testEmbedLAN, purecef.ResourceTypeRtXhr))
	scope.setOrigins(nil)
	assert.Equal(t, localNetworkBlock,
		scope.decide(testHomeTop, testEmbedLAN+"/x", testEmbedLAN, purecef.ResourceTypeRtXhr))
}

func TestLocalNetworkScopeHostIsPrivate(t *testing.T) {
	t.Parallel()
	var lookups atomic.Int32
	scope := newLocalNetworkScope()
	scope.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		lookups.Add(1)
		switch host {
		case "public.example":
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		case "rebind.example":
			return []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")}, nil
		case "empty.example":
			return nil, nil
		}
		return nil, errors.New("no such host")
	}
	ctx := context.Background()
	assert.False(t, scope.hostIsPrivate(ctx, "public.example"))
	assert.True(t, scope.hostIsPrivate(ctx, "rebind.example"))
	assert.True(t, scope.hostIsPrivate(ctx, "empty.example"))
	assert.True(t, scope.hostIsPrivate(ctx, "missing.example"))

	// Successful lookups are cached, failures are retried.
	before := lookups.Load()
	assert.False(t, scope.hostIsPrivate(ctx, "public.example"))
	assert.True(t, scope.hostIsPrivate(ctx, "missing.example"))
	assert.Equal(t, before+1, lookups.Load())

	// Entries expire.
	now := time.Now()
	scope.now = func() time.Time { return now.Add(2 * localNetworkHostCacheTTL) }
	assert.False(t, scope.hostIsPrivate(ctx, "public.example"))
	assert.Equal(t, before+2, lookups.Load())
}

type recordingCallback struct{ done chan bool }

func (c *recordingCallback) Cont()   { c.done <- true }
func (c *recordingCallback) Cancel() { c.done <- false }

func TestLocalNetworkGuard(t *testing.T) {
	t.Parallel()
	scope := newLocalNetworkScope()
	scope.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "lan.example" {
			return []netip.Addr{netip.MustParseAddr("10.0.0.2")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	request := func(url string) purecef.Request {
		r := cefmocks.NewMockRequest(t)
		r.EXPECT().GetURL().Return(url).Maybe()
		return r
	}

	block := &localNetworkGuard{scope: scope, block: true, ctx: context.Background()}
	assert.Equal(t, purecef.ReturnValueRvCancel,
		block.OnBeforeResourceLoad(nil, nil, request(testLoopbackDest), &recordingCallback{}))

	resolve := &localNetworkGuard{scope: scope, ctx: context.Background()}
	for host, allowed := range map[string]bool{"lan.example": false, "cdn.example": true} {
		cb := &recordingCallback{done: make(chan bool, 1)}
		ret := resolve.OnBeforeResourceLoad(nil, nil, request("https://"+host+"/x"), cb)
		require.Equal(t, purecef.ReturnValueRvContinueAsync, ret, host)
		select {
		case got := <-cb.done:
			assert.Equal(t, allowed, got, host)
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: callback never resolved", host)
		}
		// The second request for the host is answered from the cache.
		assert.Equal(t, localNetworkReturn(!allowed),
			resolve.OnBeforeResourceLoad(nil, nil, request("https://"+host+"/y"), cb), host)
	}
}

func TestGetResourceRequestHandlerGatesSystemViewSubframes(t *testing.T) {
	t.Parallel()
	engine := &Engine{ctx: context.Background(), localNetwork: testLocalNetworkScope()}
	h := &handlerSet{wv: &WebView{ctx: context.Background(), engine: engine}}
	call := func(top, url, initiator string, rt purecef.ResourceType) purecef.ResourceRequestHandler {
		frame := cefmocks.NewMockFrame(t)
		frame.EXPECT().GetURL().Return(top)
		browser := cefmocks.NewMockBrowser(t)
		browser.EXPECT().GetMainFrame().Return(frame)
		request := cefmocks.NewMockRequest(t)
		request.EXPECT().GetURL().Return(url)
		request.EXPECT().GetResourceType().Return(rt)
		return h.GetResourceRequestHandler(browser, nil, request, 0, 0, initiator, nil)
	}

	assert.Nil(t, call("https://site.example/", testLoopbackDest, "https://site.example", purecef.ResourceTypeRtXhr))
	assert.Nil(t, call(testHomeTop, testEmbedLAN+"/api", testEmbedLAN, purecef.ResourceTypeRtXhr))

	// Plain Go handlers: wrapping them for CEF needs libcef.
	block := &localNetworkGuard{scope: engine.localNetwork, block: true}
	resolve := &localNetworkGuard{scope: engine.localNetwork}
	engine.localNetworkHandlers.once.Do(func() {
		engine.localNetworkHandlers.block, engine.localNetworkHandlers.resolve = block, resolve
	})
	assert.Same(t, purecef.ResourceRequestHandler(block), call(testHomeTop, testLoopbackDest, testPublicEmbed, purecef.ResourceTypeRtXhr))
	assert.Same(t, purecef.ResourceRequestHandler(resolve), call(testHomeTop, "https://cdn.example/x.js", testPublicEmbed, purecef.ResourceTypeRtXhr))
}
