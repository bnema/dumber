package adapter

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/favicon"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/mock"
)

func TestFaviconAdapterInvalidateRemovesStoredKey(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})
	texture := &gdk.Texture{}
	adapter.setTexture("example.com", texture)

	if adapter.GetTexture("example.com") == nil {
		t.Fatal("expected key to be cached before invalidation")
	}
	if err := adapter.Invalidate(context.Background(), "example.com"); err != nil {
		t.Fatal(err)
	}
	if adapter.GetTexture("example.com") != nil {
		t.Fatal("expected invalidation to remove the stored key")
	}
}

func TestFaviconAdapterGetTextureByURLPrefersExactThenParentThenDomainFallback(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})
	domainTexture := &gdk.Texture{}
	repoTexture := &gdk.Texture{}
	prTexture := &gdk.Texture{}
	adapter.setTexture("github.com", domainTexture)
	adapter.setTexture("github.com/bnema/gordon", repoTexture)
	adapter.setTexture("github.com/bnema/gordon/pull/123", prTexture)

	if got := adapter.GetTextureByURL("https://github.com/bnema/gordon/pull/123"); got != prTexture {
		t.Fatal("expected exact PR favicon")
	}
	if got := adapter.GetTextureByURL("https://github.com/bnema/gordon/issues/5"); got != repoTexture {
		t.Fatal("expected issue URL to fall back to repository favicon")
	}
	if got := adapter.GetTextureByURL("https://github.com/notifications"); got != domainTexture {
		t.Fatal("expected unrelated URL to fall back to domain favicon")
	}
}

func TestFaviconAdapterSetResolvedTextureCachesOnlyResolvedKey(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})
	fallbackTexture := &gdk.Texture{}
	pageURL := "https://github.com/bnema/gordon/pull/123"
	pageKey := "github.com/bnema/gordon/pull/123"

	adapter.setResolvedTexture(pageURL, favicon.Key("github.com"), fallbackTexture)

	if got := adapter.GetTexture("github.com"); got != fallbackTexture {
		t.Fatal("expected resolved fallback key to be cached")
	}
	if got := adapter.GetTexture(pageKey); got != nil {
		t.Fatal("fallback result should not be cached under the exact page key")
	}
	if got := adapter.GetTextureByURL(pageURL); got != fallbackTexture {
		t.Fatal("expected exact page lookup to fall back through candidate ordering")
	}
}

func TestFaviconAdapterResolvedPathSpecificTextureDoesNotPolluteDomain(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})
	prTexture := &gdk.Texture{}
	adapter.setResolvedTexture("https://github.com/bnema/gordon/pull/123", favicon.Key("github.com/bnema/gordon/pull/123"), prTexture)

	if got := adapter.GetTextureByURL("https://github.com/bnema/gordon/pull/123"); got != prTexture {
		t.Fatal("expected exact PR favicon")
	}
	if got := adapter.GetTextureByURL("https://github.com/bnema/gordon"); got != nil {
		t.Fatal("path-specific favicon leaked to sibling repository URL")
	}
	if got := adapter.GetTexture("github.com"); got != nil {
		t.Fatal("path-specific favicon was cached under bare domain")
	}
	if got := adapter.GetTextureByURL("https://gitlab.com/group/project"); got != nil {
		t.Fatal("path-specific favicon leaked across domains")
	}
}

func TestFaviconAdapterGetOrFetchSkipsResolverForExactMemoryCacheHit(t *testing.T) {
	resolver := portmocks.NewMockFaviconResolver(t)
	db := portmocks.NewMockFaviconDatabase(t)
	adapter := NewFaviconAdapterWithResolver(nil, resolver, db, FaviconAdapterConfig{})
	texture := &gdk.Texture{}
	pageURL := "https://github.com/bnema/gordon/pull/123"
	adapter.setTexture("github.com/bnema/gordon/pull/123", texture)

	var got *gdk.Texture
	adapter.GetOrFetch(context.Background(), pageURL, func(received *gdk.Texture) {
		got = received
	})

	if got != texture {
		t.Fatal("expected exact cached texture")
	}
	db.AssertNotCalled(t, "GetFaviconAsync", mock.Anything, mock.Anything)
}

func TestFaviconAdapterGetOrFetchConsultsResolverBeforeFallbackMemoryCache(t *testing.T) {
	resolver := portmocks.NewMockFaviconResolver(t)
	adapter := NewFaviconAdapterWithResolver(nil, resolver, nil, FaviconAdapterConfig{})
	fallbackTexture := &gdk.Texture{}
	pageURL := "https://github.com/bnema/gordon/pull/123"
	adapter.setTexture("github.com", fallbackTexture)
	resolver.EXPECT().Resolve(
		mock.Anything,
		pageURL,
		0,
		port.FaviconResolveOptions{Purpose: port.FaviconResolvePurposeUI, ScheduleBackgroundRefresh: true},
	).Return((*port.ResolvedFavicon)(nil), nil).Once()

	var got *gdk.Texture
	adapter.GetOrFetch(context.Background(), pageURL, func(received *gdk.Texture) {
		got = received
	})

	if got != fallbackTexture {
		t.Fatal("expected domain fallback texture after resolver miss")
	}
}

func TestFaviconAdapterPreloadFromCacheConsultsResolverBeforeFallbackMemoryCache(t *testing.T) {
	resolver := portmocks.NewMockFaviconResolver(t)
	adapter := NewFaviconAdapterWithResolver(nil, resolver, nil, FaviconAdapterConfig{})
	fallbackTexture := &gdk.Texture{}
	pageURL := "https://github.com/bnema/gordon/pull/123"
	adapter.setTexture("github.com", fallbackTexture)
	resolver.EXPECT().Resolve(
		mock.Anything,
		pageURL,
		0,
		port.FaviconResolveOptions{Purpose: port.FaviconResolvePurposeUI},
	).Return((*port.ResolvedFavicon)(nil), nil).Once()

	if got := adapter.PreloadFromCache(context.Background(), pageURL); got != fallbackTexture {
		t.Fatal("expected domain fallback texture after resolver preload miss")
	}
}

func TestFaviconAdapterGetOrFetchProbesEngineAfterFallbackMemoryCacheHit(t *testing.T) {
	resolver := portmocks.NewMockFaviconResolver(t)
	db := portmocks.NewMockFaviconDatabase(t)
	adapter := NewFaviconAdapterWithResolver(nil, resolver, db, FaviconAdapterConfig{})
	fallbackTexture := &gdk.Texture{}
	pageURL := "https://github.com/bnema/gordon/pull/123"
	adapter.setTexture("github.com", fallbackTexture)
	resolver.EXPECT().Resolve(
		mock.Anything,
		pageURL,
		0,
		port.FaviconResolveOptions{Purpose: port.FaviconResolvePurposeUI, ScheduleBackgroundRefresh: true},
	).Return((*port.ResolvedFavicon)(nil), nil).Once()
	db.EXPECT().GetFaviconAsync(pageURL, mock.Anything).Run(func(_ string, callback func(port.Texture)) {
		callback(nil)
	}).Return().Once()

	var (
		got   *gdk.Texture
		calls int
	)
	adapter.GetOrFetch(context.Background(), pageURL, func(received *gdk.Texture) {
		calls++
		got = received
	})

	if got != fallbackTexture {
		t.Fatal("expected fallback texture while probing exact engine favicon")
	}
	if calls != 1 {
		t.Fatalf("expected single callback on engine miss, got %d", calls)
	}
}

func TestFaviconAdapterStoreFromWebKitWithOriginDoesNotAliasSameDomainSpecificPaths(t *testing.T) {
	resolver := portmocks.NewMockFaviconResolver(t)
	resolver.EXPECT().Observe(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((*favicon.Metadata)(nil), nil).
		Maybe()
	adapter := NewFaviconAdapterWithResolver(nil, resolver, nil, FaviconAdapterConfig{})
	texture := &gdk.Texture{}

	adapter.StoreFromWebKitWithOrigin(
		context.Background(),
		"https://github.com/login",
		"https://github.com/bnema/gordon/pull/123",
		texture,
	)

	if got := adapter.GetTextureByURL("https://github.com/login"); got != texture {
		t.Fatal("expected current URL favicon to be cached")
	}
	if got := adapter.GetTextureByURL("https://github.com/bnema/gordon/pull/123"); got != nil {
		t.Fatal("same-domain redirect aliased favicon onto unrelated exact path")
	}
	if got := adapter.GetTexture("github.com"); got != nil {
		t.Fatal("same-domain redirect promoted favicon to bare domain")
	}
}

func TestFaviconAdapterStoreFromWebKitWithOriginDoesNotAliasCrossDomainRedirectsInResolverMode(t *testing.T) {
	resolver := portmocks.NewMockFaviconResolver(t)
	resolver.EXPECT().Observe(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((*favicon.Metadata)(nil), nil).
		Maybe()
	adapter := NewFaviconAdapterWithResolver(nil, resolver, nil, FaviconAdapterConfig{})
	texture := &gdk.Texture{}
	currentURL := "https://auth.example.net/challenge"
	originURL := "https://github.com/bnema/gordon/pull/123"

	adapter.StoreFromWebKitWithOrigin(context.Background(), currentURL, originURL, texture)

	if got := adapter.GetTextureByURL(currentURL); got != texture {
		t.Fatal("expected current redirected URL favicon to be cached")
	}
	if got := adapter.GetTextureByURL(originURL); got != nil {
		t.Fatal("cross-domain redirect aliased favicon onto original exact path")
	}
	if got := adapter.GetTexture("github.com"); got != nil {
		t.Fatal("cross-domain redirect promoted favicon to original domain")
	}
}

func TestFaviconWarningDedup_FirstAndRepeated(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})

	first, suppressed := adapter.shouldLogWarningDedup("save-png:example.com")
	if !first {
		t.Fatalf("expected first warning to be logged")
	}
	if suppressed != 0 {
		t.Fatalf("expected no suppressed count on first warning, got %d", suppressed)
	}

	first, suppressed = adapter.shouldLogWarningDedup("save-png:example.com")
	if first {
		t.Fatalf("expected repeated warning to be suppressed")
	}
	if suppressed != 1 {
		t.Fatalf("expected suppressed count to be 1, got %d", suppressed)
	}

	first, suppressed = adapter.shouldLogWarningDedup("save-png:example.com")
	if first {
		t.Fatalf("expected repeated warning to remain suppressed")
	}
	if suppressed != 2 {
		t.Fatalf("expected suppressed count to be 2, got %d", suppressed)
	}
}

func TestFaviconWarningDedup_ClearResetsState(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})
	key := "sized-png:example.com"

	first, _ := adapter.shouldLogWarningDedup(key)
	if !first {
		t.Fatalf("expected first warning to be logged")
	}

	adapter.clearWarningDedup(key)

	first, suppressed := adapter.shouldLogWarningDedup(key)
	if !first {
		t.Fatalf("expected warning to log again after clear")
	}
	if suppressed != 0 {
		t.Fatalf("expected suppressed count reset after clear, got %d", suppressed)
	}
}

func TestFaviconWarningDedup_LogWarningDedupInvokesCallbackOnce(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil, FaviconAdapterConfig{})
	key := "save-png:example.com"
	var calls atomic.Int32

	adapter.logWarningDedup(t.Context(), key, nil, func(_ *zerolog.Logger, _ error) {
		calls.Add(1)
	})
	adapter.logWarningDedup(t.Context(), key, nil, func(_ *zerolog.Logger, _ error) {
		calls.Add(1)
	})

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected warning callback to run once, got %d", got)
	}
}
