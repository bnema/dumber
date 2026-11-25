package cef

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"testing/synctest"
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
	req := func(url, initiator string, rt purecef.ResourceType) localNetworkRequest {
		return localNetworkRequest{TopURL: testHomeTop, URL: url, Initiator: initiator, ResourceType: rt}
	}
	// nav is a subframe navigation in a frame showing frameURL whose parent
	// shows parentURL. CEF reports a "null" initiator for navigations.
	nav := func(url, frameURL, parentURL string) localNetworkRequest {
		return localNetworkRequest{
			TopURL: testHomeTop, URL: url, Initiator: "null", ResourceType: frame,
			FrameURL: frameURL, ParentURL: parentURL,
		}
	}
	regular := req(testLoopbackDest, "https://site.example", sub)
	regular.TopURL = "https://site.example/"
	cases := []struct {
		name string
		req  localNetworkRequest
		want localNetworkVerdict
	}{
		{"regular page is out of scope", regular, localNetworkAllow},
		{"top navigation is out of scope", req(testLoopbackDest, "", main), localNetworkAllow},
		{"system view document itself", req("http://10.0.0.9/poster.png", "https://dumber.invalid", sub), localNetworkAllow},
		{"internal host", req("https://dumber.invalid/api/x", testPublicEmbed, xhr), localNetworkAllow},
		{"configured embed subresource", req(testEmbedLAN+"/api/ds", testEmbedLAN, xhr), localNetworkAllow},
		{"configured embed origin canonicalized", req("https://grafana.lan/api", "https://grafana.lan", xhr), localNetworkAllow},
		{"configured embed to other LAN host", req("http://10.0.0.5/x", testEmbedLAN, xhr), localNetworkAllow},
		{"public embed to loopback", req(testLoopbackDest, testPublicEmbed, xhr), localNetworkBlock},
		{"public embed to LAN", req("http://192.168.1.10:3000/api", testPublicEmbed, sub), localNetworkBlock},
		{"public embed to IPv6 loopback", req("http://[::1]:8081/", testPublicEmbed, xhr), localNetworkBlock},
		{"public embed to localhost", req("http://localhost:8081/", testPublicEmbed, xhr), localNetworkBlock},
		{"public embed to sub.localhost", req("http://a.localhost/", testPublicEmbed, xhr), localNetworkBlock},
		{"public embed websocket to LAN", req("ws://10.0.0.2/socket", testPublicEmbed, xhr), localNetworkBlock},
		{"public embed to public IP", req("https://8.8.8.8/", testPublicEmbed, xhr), localNetworkAllow},
		{"public embed to hostname", req("https://cdn.example/x.js", testPublicEmbed, sub), localNetworkResolve},
		{"opaque initiator", req(testLoopbackDest, "null", xhr), localNetworkBlock},
		{"missing initiator", req(testLoopbackDest, "", xhr), localNetworkBlock},
		{"data URL", req("data:text/plain,x", testPublicEmbed, sub), localNetworkAllow},
		{"unparsable URL", req("http://[::1", testPublicEmbed, xhr), localNetworkBlock},

		// Subframe navigations.
		{"home loads its embed", nav(testEmbedLAN+"/d/x", "about:blank", testHomeTop), localNetworkAllow},
		{"home loads its embed in a fresh frame", nav(testEmbedLAN+"/d/x", "", testHomeTop), localNetworkAllow},
		{"embed navigates itself", nav(testEmbedLAN+"/d/y", testEmbedLAN+"/d/x", testHomeTop), localNetworkAllow},
		{"embed loads a child frame", nav(testEmbedLAN+"/child", "about:blank", testEmbedLAN+"/"), localNetworkAllow},
		{"embed frames another allowed origin", nav("https://grafana.lan/d", "about:blank", testEmbedLAN+"/"), localNetworkAllow},
		{"embed navigates itself to loopback", nav(testLoopbackDest, testEmbedLAN+"/", testHomeTop), localNetworkBlock},
		{"embed navigates itself to an unlisted LAN port", nav("http://192.168.1.10:3001/", testEmbedLAN+"/", testHomeTop), localNetworkBlock},
		{"third party frame loads an allowed LAN origin", nav(testEmbedLAN+"/admin", "about:blank", testPublicEmbed+"/"), localNetworkBlock},
		{"third party frame navigates itself to an allowed LAN origin", nav(testEmbedLAN+"/admin", testPublicEmbed+"/", testEmbedLAN+"/"), localNetworkBlock},
		{"sandboxed srcdoc frame posts to an allowed LAN origin", nav(testEmbedLAN+"/api/reboot", "about:srcdoc", testEmbedLAN+"/"), localNetworkBlock},
		{"unidentified frame loads an allowed LAN origin", nav(testEmbedLAN+"/", "", ""), localNetworkBlock},
		{"third party frame loads a public site", nav("https://8.8.8.8/", "about:blank", testPublicEmbed+"/"), localNetworkAllow},
	}
	scope := testLocalNetworkScope()
	for _, tc := range cases {
		assert.Equal(t, tc.want, scope.decide(tc.req), tc.name)
	}
}

func TestLocalNetworkScopeWithoutOriginsBlocksLAN(t *testing.T) {
	t.Parallel()
	scope := newLocalNetworkScope()
	req := localNetworkRequest{
		TopURL: testHomeTop, URL: testEmbedLAN + "/x", Initiator: testEmbedLAN, ResourceType: purecef.ResourceTypeRtXhr,
	}
	assert.Equal(t, localNetworkBlock, scope.decide(req))
	scope.setOrigins([]string{testEmbedLAN})
	assert.Equal(t, localNetworkAllow, scope.decide(req))
	scope.setOrigins(nil)
	assert.Equal(t, localNetworkBlock, scope.decide(req))
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
	synctest.Test(t, testLocalNetworkGuard)
}

func testLocalNetworkGuard(t *testing.T) {
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
		// The lookup goroutine is the only other one in the bubble.
		synctest.Wait()
		select {
		case got := <-cb.done:
			assert.Equal(t, allowed, got, host)
		default:
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
	// Plain Go handlers: wrapping them for CEF needs libcef.
	block := purecef.ResourceRequestHandler(&localNetworkGuard{scope: engine.localNetwork, block: true})
	resolve := purecef.ResourceRequestHandler(&localNetworkGuard{scope: engine.localNetwork})
	engine.localNetworkHandlers.once.Do(func() {
		engine.localNetworkHandlers.block, engine.localNetworkHandlers.resolve = block, resolve
	})

	browserWithTop := func(top string) purecef.Browser {
		mainFrame := cefmocks.NewMockFrame(t)
		mainFrame.EXPECT().GetURL().Return(top)
		browser := cefmocks.NewMockBrowser(t)
		browser.EXPECT().GetMainFrame().Return(mainFrame)
		return browser
	}
	request := func(url string, rt purecef.ResourceType) purecef.Request {
		r := cefmocks.NewMockRequest(t)
		r.EXPECT().GetURL().Return(url)
		r.EXPECT().GetResourceType().Return(rt)
		return r
	}
	subframe := func(frameURL, parentURL string) purecef.Frame {
		parent := cefmocks.NewMockFrame(t)
		parent.EXPECT().GetURL().Return(parentURL)
		f := cefmocks.NewMockFrame(t)
		f.EXPECT().IsMain().Return(false)
		f.EXPECT().GetParent().Return(parent)
		f.EXPECT().GetURL().Return(frameURL)
		return f
	}
	call := func(browser purecef.Browser, frame purecef.Frame, req purecef.Request, initiator string) purecef.ResourceRequestHandler {
		return h.GetResourceRequestHandler(browser, frame, req, 0, 0, initiator, nil)
	}

	// The main frame URL decides whether the guard applies at all.
	assert.Nil(t, call(browserWithTop("https://site.example/"), nil,
		request(testLoopbackDest, purecef.ResourceTypeRtXhr), "https://site.example"))

	// Subresources: the initiator decides.
	assert.Nil(t, call(browserWithTop(testHomeTop), nil,
		request(testEmbedLAN+"/api", purecef.ResourceTypeRtXhr), testEmbedLAN))
	assert.Same(t, block, call(browserWithTop(testHomeTop), nil,
		request(testLoopbackDest, purecef.ResourceTypeRtXhr), testPublicEmbed))
	assert.Same(t, resolve, call(browserWithTop(testHomeTop), nil,
		request("https://cdn.example/x.js", purecef.ResourceTypeRtXhr), testPublicEmbed))

	// Subframe navigations: the navigating frame and its parent decide.
	assert.Nil(t, call(browserWithTop(testHomeTop), subframe("about:blank", testHomeTop),
		request(testEmbedLAN+"/d", purecef.ResourceTypeRtSubFrame), "null"))
	assert.Same(t, block, call(browserWithTop(testHomeTop), subframe("about:blank", testPublicEmbed+"/"),
		request(testEmbedLAN+"/admin", purecef.ResourceTypeRtSubFrame), "null"))

	// CEF passes the main frame when it cannot find the subframe.
	mainAsFrame := cefmocks.NewMockFrame(t)
	mainAsFrame.EXPECT().IsMain().Return(true)
	assert.Same(t, block, call(browserWithTop(testHomeTop), mainAsFrame,
		request(testEmbedLAN+"/d", purecef.ResourceTypeRtSubFrame), "null"))
}
