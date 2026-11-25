package cef

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	purecef "bnema.dev/purego-cef/cef"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/infrastructure/netaddr"
	"bnema.dev/dumber/internal/logging"
)

// System views get local network access without a prompt
// (systemviewLocalNetworkPrompt), and Chromium grants it per top-level
// origin: any frame under dumb://home would inherit it. localNetworkScope
// keeps that access to the home embed origins from home.toml: in a browser
// whose top-level page is a system view, a request from any other origin to
// a private or loopback address is canceled.
//
// Known limits: hostnames are resolved with Go's resolver, which can answer
// differently from Chromium's (DNS rebinding). WebSockets and service or
// shared worker fetches are not seen by the CEF resource request handler.

const (
	localNetworkResolveTimeout = 3 * time.Second
	localNetworkHostCacheTTL   = time.Minute
	localNetworkHostCacheMax   = 512
)

type localNetworkVerdict int

const (
	localNetworkAllow localNetworkVerdict = iota
	localNetworkBlock
	// localNetworkResolve means the destination is a hostname whose
	// addresses decide.
	localNetworkResolve
)

// localNetworkScope holds the configured embed origins and a short cache of
// hostname classifications.
type localNetworkScope struct {
	origins atomic.Pointer[map[string]struct{}]

	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	now    func() time.Time

	cacheMu sync.Mutex
	cache   map[string]localNetworkHostEntry
}

type localNetworkHostEntry struct {
	private bool
	expires time.Time
}

func newLocalNetworkScope() *localNetworkScope {
	return &localNetworkScope{
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		now:   time.Now,
		cache: map[string]localNetworkHostEntry{},
	}
}

var _ port.HomeEmbedOriginSetter = (*Engine)(nil)

// SetHomeEmbedOrigins sets the origins allowed to reach the local network
// from a system view page.
func (e *Engine) SetHomeEmbedOrigins(origins []string) {
	if e == nil || e.localNetwork == nil {
		return
	}
	e.localNetwork.setOrigins(origins)
}

func (s *localNetworkScope) setOrigins(origins []string) {
	set := make(map[string]struct{}, len(origins))
	for _, raw := range origins {
		if origin, ok := entity.HomeEmbedOrigin(raw); ok {
			set[origin] = struct{}{}
		}
	}
	s.origins.Store(&set)
}

func (s *localNetworkScope) allowsOrigin(raw string) bool {
	if s == nil {
		return false
	}
	set := s.origins.Load()
	if set == nil {
		return false
	}
	origin, ok := entity.HomeEmbedOrigin(raw)
	if !ok {
		return false
	}
	_, found := (*set)[origin]
	return found
}

// decide classifies one request. topURL is the browser's main frame URL,
// initiator the origin of the document or worker that made the request.
// A subframe navigation is judged by its destination origin, any other
// request by its initiator.
func (s *localNetworkScope) decide(
	topURL, rawURL, initiator string, resourceType purecef.ResourceType,
) localNetworkVerdict {
	if !isActualInternalURL(topURL) || resourceType == purecef.ResourceTypeRtMainFrame {
		return localNetworkAllow
	}
	dest, err := url.Parse(rawURL)
	if err != nil {
		return localNetworkBlock
	}
	switch strings.ToLower(dest.Scheme) {
	case "http", "https", "ws", "wss":
	default:
		// data:, blob:, about: and the like never reach the network.
		return localNetworkAllow
	}
	host := strings.TrimSuffix(strings.ToLower(dest.Hostname()), ".")
	if host == actualInternalHost {
		return localNetworkAllow
	}
	requester := initiator
	if resourceType == purecef.ResourceTypeRtSubFrame {
		requester = rawURL
	} else if isActualInternalURL(initiator) {
		// The system view document itself.
		return localNetworkAllow
	}
	if s.allowsOrigin(requester) {
		return localNetworkAllow
	}
	if host == "" {
		return localNetworkBlock
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if netaddr.IsPrivate(addr) {
			return localNetworkBlock
		}
		return localNetworkAllow
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return localNetworkBlock
	}
	return localNetworkResolve
}

// hostIsPrivate reports whether host resolves to any non-public address.
// Lookup failures count as private.
func (s *localNetworkScope) hostIsPrivate(ctx context.Context, host string) bool {
	if private, ok := s.cachedHost(host); ok {
		return private
	}
	ctx, cancel := context.WithTimeout(ctx, localNetworkResolveTimeout)
	defer cancel()
	addrs, err := s.lookup(ctx, host)
	private := err != nil || len(addrs) == 0
	for _, addr := range addrs {
		if netaddr.IsPrivate(addr) {
			private = true
			break
		}
	}
	if err == nil {
		s.storeHost(host, private)
	}
	return private
}

func (s *localNetworkScope) cachedHost(host string) (private, ok bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	entry, found := s.cache[host]
	if !found || !s.now().Before(entry.expires) {
		return false, false
	}
	return entry.private, true
}

func (s *localNetworkScope) storeHost(host string, private bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if len(s.cache) >= localNetworkHostCacheMax {
		clear(s.cache)
	}
	s.cache[host] = localNetworkHostEntry{private: private, expires: s.now().Add(localNetworkHostCacheTTL)}
}

// localNetworkGuard cancels the requests handed to it by
// GetResourceRequestHandler: all of them when block is set, otherwise those
// whose host resolves to a non-public address. It runs on the CEF IO thread
// and never blocks it: lookups run on a goroutine, and the CEF callback may
// be continued from any thread.
type localNetworkGuard struct {
	scope *localNetworkScope
	block bool
	ctx   context.Context
}

var _ purecef.ResourceRequestHandler = (*localNetworkGuard)(nil)

func (g *localNetworkGuard) OnBeforeResourceLoad(
	_ purecef.Browser, _ purecef.Frame, request purecef.Request, callback purecef.Callback,
) purecef.ReturnValue {
	if g.block || request == nil {
		return purecef.ReturnValueRvCancel
	}
	dest, err := url.Parse(request.GetURL())
	if err != nil {
		return purecef.ReturnValueRvCancel
	}
	host := strings.TrimSuffix(strings.ToLower(dest.Hostname()), ".")
	if private, ok := g.scope.cachedHost(host); ok {
		return localNetworkReturn(private)
	}
	if callback == nil {
		return purecef.ReturnValueRvCancel
	}
	go func() {
		if g.scope.hostIsPrivate(g.ctx, host) {
			callback.Cancel()
			return
		}
		callback.Cont()
	}()
	return purecef.ReturnValueRvContinueAsync
}

func localNetworkReturn(private bool) purecef.ReturnValue {
	if private {
		return purecef.ReturnValueRvCancel
	}
	return purecef.ReturnValueRvContinue
}

func (*localNetworkGuard) GetCookieAccessFilter(
	purecef.Browser, purecef.Frame, purecef.Request,
) purecef.CookieAccessFilter {
	return nil
}

func (*localNetworkGuard) GetResourceHandler(
	purecef.Browser, purecef.Frame, purecef.Request,
) purecef.ResourceHandler {
	return nil
}

func (*localNetworkGuard) OnResourceRedirect(
	purecef.Browser, purecef.Frame, purecef.Request, purecef.Response, uintptr,
) {
}

func (*localNetworkGuard) OnResourceResponse(
	purecef.Browser, purecef.Frame, purecef.Request, purecef.Response,
) int32 {
	return 0
}

func (*localNetworkGuard) GetResourceResponseFilter(
	purecef.Browser, purecef.Frame, purecef.Request, purecef.Response,
) purecef.ResponseFilter {
	return nil
}

func (*localNetworkGuard) OnResourceLoadComplete(
	purecef.Browser, purecef.Frame, purecef.Request, purecef.Response, purecef.UrlrequestStatus, int64,
) {
}

func (*localNetworkGuard) OnProtocolExecution(
	purecef.Browser, purecef.Frame, purecef.Request, *int32,
) {
}

// localNetworkHandlers are the two CEF guard handlers of an engine, built
// once and shared by every browser.
type localNetworkHandlers struct {
	once    sync.Once
	block   purecef.ResourceRequestHandler
	resolve purecef.ResourceRequestHandler
}

func (h *localNetworkHandlers) get(ctx context.Context, scope *localNetworkScope) (block, resolve purecef.ResourceRequestHandler) {
	if ctx == nil {
		ctx = context.Background()
	}
	h.once.Do(func() {
		h.block = purecef.NewResourceRequestHandler(&localNetworkGuard{scope: scope, block: true, ctx: ctx})
		h.resolve = purecef.NewResourceRequestHandler(&localNetworkGuard{scope: scope, ctx: ctx})
	})
	return h.block, h.resolve
}

// requestOrigin returns scheme://host[:port] of raw, for logs that must not
// carry paths or query strings.
func requestOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// systemViewLocalNetworkSettings are the content settings Chromium stores
// when a local network prompt is accepted.
var systemViewLocalNetworkSettings = []purecef.ContentSettingTypes{
	purecef.ContentSettingTypesContentSettingTypeLocalNetworkAccess,
	purecef.ContentSettingTypesContentSettingTypeLocalNetwork,
	purecef.ContentSettingTypesContentSettingTypeLoopbackNetwork,
}

// resetSystemViewLocalNetworkGrants drops the local network grants Chromium
// persisted for the system view origin when systemviewLocalNetworkPrompt
// accepted a prompt, so that silent grant lasts one session. Runs on the CEF
// UI thread once the global request context exists.
func resetSystemViewLocalNetworkGrants(ctx context.Context) {
	requestContext := purecef.RequestContextGetGlobalContext()
	if requestContext == nil {
		return
	}
	origin := actualInternalOrigin + "/"
	for _, setting := range systemViewLocalNetworkSettings {
		if requestContext.GetContentSetting(origin, origin, setting) != purecef.ContentSettingValuesContentSettingValueAllow {
			continue
		}
		requestContext.SetContentSetting(origin, origin, setting, purecef.ContentSettingValuesContentSettingValueDefault)
		logging.FromContext(ctx).Debug().Int("setting", int(setting)).
			Msg("cef: cleared a persisted local network grant of the system view origin")
	}
}
