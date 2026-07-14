package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/favicon"
	"github.com/stretchr/testify/mock"
)

func TestFaviconResolveExactHit(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("docs.example.com", []byte("png32"), false)
	got, err := fx.uc.Resolve(context.Background(), "https://docs.example.com/a", 32, ResolveOptions{Purpose: ResolvePurposeUI})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "docs.example.com" || string(got.Bytes) != "png32" {
		t.Fatalf("unexpected resolve: %#v %q", got, got.Bytes)
	}
}

func TestFaviconResolveParentFallback(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("example.com", []byte("parent"), false)
	got, err := fx.uc.Resolve(context.Background(), "https://docs.example.com/a", 32, ResolveOptions{Purpose: ResolvePurposeUI})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "example.com" || string(got.Bytes) != "parent" {
		t.Fatalf("unexpected fallback: %#v", got)
	}
}

func TestFaviconResolvePrefersParentPathFallbackBeforeDomain(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("github.com", []byte("github-default"), false)
	fx.seed("github.com/bnema/gordon", []byte("repo-default"), false)

	got, err := fx.uc.Resolve(context.Background(), "https://github.com/bnema/gordon/issues/5", 32, ResolveOptions{Purpose: ResolvePurposeUI})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "github.com/bnema/gordon" || string(got.Bytes) != "repo-default" {
		t.Fatalf("expected repository fallback before domain: key=%q bytes=%q", got.Key, got.Bytes)
	}
}

func TestFaviconResolvePathSpecificDoesNotLeakToParentOrSiblings(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("github.com", []byte("github-default"), false)

	if _, err := fx.uc.Observe(
		context.Background(),
		"https://github.com/bnema/gordon/pull/123",
		"https://github.com/favicon-status-success.png",
		[]byte("pr-success"),
		favicon.SourceEngine,
		"image/png",
	); err != nil {
		t.Fatal(err)
	}

	pr, err := fx.uc.Resolve(context.Background(), "https://github.com/bnema/gordon/pull/123", 32, ResolveOptions{Purpose: ResolvePurposeUI})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Key != "github.com/bnema/gordon/pull/123" || string(pr.Bytes) != "png32:pr-success" {
		t.Fatalf("PR resolve used wrong icon: key=%q bytes=%q", pr.Key, pr.Bytes)
	}

	for _, rawURL := range []string{
		"https://github.com/bnema/gordon",
		"https://github.com/notifications",
		"https://github.com/bnema?tab=repositories",
	} {
		got, err := fx.uc.Resolve(context.Background(), rawURL, 32, ResolveOptions{Purpose: ResolvePurposeUI})
		if err != nil {
			t.Fatalf("Resolve(%q): %v", rawURL, err)
		}
		if got.Key != "github.com" || string(got.Bytes) != "github-default" {
			t.Fatalf("Resolve(%q) leaked path-specific favicon: key=%q bytes=%q", rawURL, got.Key, got.Bytes)
		}
	}

	if _, err := fx.uc.Resolve(context.Background(), "https://gitlab.com/bnema/gordon", 32, ResolveOptions{Purpose: ResolvePurposeUI}); !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("expected cross-domain miss, got %v", err)
	}
}

func TestFaviconResolveRepairsOrphanCanonicalBlobMetadata(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.blobs.png["example.com"] = []byte("png")
	fx.blobs.sized["example.com"] = map[int][]byte{SystemviewIconSize: []byte("png32")}

	got, err := fx.uc.ResolveSystemviewIcon(context.Background(), "example.com", SystemviewIconSize)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "example.com" || string(got.Bytes) != "png32" {
		t.Fatalf("unexpected resolve: %#v %q", got, got.Bytes)
	}
	meta := fx.repo.byKey["example.com"]
	if meta == nil {
		t.Fatal("expected metadata to be repaired")
	}
	if meta.Source != favicon.SourceImported || meta.ContentType != "image/png" || meta.ContentHash != favicon.Hash([]byte("png32")) {
		t.Fatalf("unexpected repaired metadata: %#v", meta)
	}
}

func TestFaviconResolveRepairOrphanWithoutConverterReturnsMiss(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.uc.converter = nil
	fx.blobs.original["example.com"] = []byte("original")
	fx.blobs.contentTypes["example.com"] = "image/png"

	_, err := fx.uc.ResolveSystemviewIcon(context.Background(), "example.com", SystemviewIconSize)
	if !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("expected favicon miss, got %v", err)
	}
}

func TestFaviconResolveStaleSchedulesDedupedBackgroundRefresh(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("example.com", []byte("old"), true)
	for i := range 2 {
		got, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{ScheduleBackgroundRefresh: true})
		if err != nil {
			t.Fatal(err)
		}
		if !got.BackgroundRefreshScheduled && i == 0 {
			t.Fatal("first stale resolve did not schedule refresh")
		}
	}
	if fx.scheduler.schedules != 1 {
		t.Fatalf("schedules=%d", fx.scheduler.schedules)
	}
	if fx.scheduler.usedCallerContext {
		t.Fatal("scheduler used caller context")
	}
}

func TestFaviconResolveStaleExactRefreshIgnoresFreshParent(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("docs.example.com", []byte("old-exact"), true)
	fx.seed("example.com", []byte("fresh-parent"), false)
	got, err := fx.uc.Resolve(
		context.Background(),
		"https://docs.example.com/page",
		32,
		ResolveOptions{ScheduleBackgroundRefresh: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "docs.example.com" || !got.BackgroundRefreshScheduled {
		t.Fatalf("unexpected resolve: %#v", got)
	}
	fx.scheduler.run("docs.example.com")
	if string(fx.blobs.original["docs.example.com"]) != "fetched" {
		t.Fatalf("exact key was not refreshed, originals=%v", fx.blobs.original)
	}
}

func TestFaviconObserveSameHashUpdatesMetadataOnly(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seedOriginal([]byte("same"))
	if _, err := fx.uc.Observe(context.Background(), "https://example.com", "https://cdn.test/icon.png", []byte("same"), favicon.SourceEngine, "image/png"); err != nil {
		t.Fatal(err)
	}
	if fx.blobs.writeOriginals != 0 || fx.converter.calls != 0 || fx.invalidators.calls != 0 {
		t.Fatalf("rewrote/converted/invalidated same hash")
	}
	if fx.repo.byKey["example.com"].LastCheckedAt != fx.now {
		t.Fatal("last checked not updated")
	}
}

func TestFaviconObserveChangedHashRegeneratesAndInvalidates(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seedOriginal([]byte("old"))
	if _, err := fx.uc.Observe(context.Background(), "https://example.com", "https://cdn.test/icon.png", []byte("new"), favicon.SourceEngine, "image/png"); err != nil {
		t.Fatal(err)
	}
	if fx.blobs.writeOriginals != 1 || fx.converter.calls != 1 || fx.blobs.removeDerived != 1 || fx.invalidators.calls != 1 {
		t.Fatalf("writes=%d converts=%d remove=%d invalidates=%d", fx.blobs.writeOriginals, fx.converter.calls, fx.blobs.removeDerived, fx.invalidators.calls)
	}
	if string(fx.blobs.png["example.com"]) != "png:new" || string(fx.blobs.sized["example.com"][32]) != "png32:new" {
		t.Fatal("derived pngs not regenerated")
	}
	if fx.repo.byKey["example.com"].SourceURL != "https://cdn.test/icon.png" {
		t.Fatal("iconURL not stored as metadata")
	}
}

func TestFaviconObserveWithoutConverterStoresOriginalOnly(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.uc.converter = nil
	if _, err := fx.uc.Observe(context.Background(), "https://example.com", "https://example.com/icon.png", []byte("raw"), favicon.SourceEngine, "image/png"); err != nil {
		t.Fatal(err)
	}
	if string(fx.blobs.original["example.com"]) != "raw" {
		t.Fatalf("original not stored: %v", fx.blobs.original)
	}
	if len(fx.blobs.png) != 0 || len(fx.blobs.sized) != 0 {
		t.Fatalf("unexpected derivatives: png=%v sized=%v", fx.blobs.png, fx.blobs.sized)
	}
}

func TestFaviconResolveMissingFetchesOnlyWhenBlockingAllowed(t *testing.T) {
	fx := newFaviconFixture(t)
	if _, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{}); !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("default resolve err=%v", err)
	}
	if fx.fetcher.calls != 0 {
		t.Fatal("default resolve blocked on fetch")
	}
	got, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{AllowBlockingRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "example.com" || fx.fetcher.calls != 1 {
		t.Fatalf("blocking resolve failed: %#v calls=%d", got, fx.fetcher.calls)
	}
}

func TestFaviconRefreshIfStaleUsesParentFallbackBeforeFetching(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("example.com", []byte("parent"), false)
	if err := fx.uc.RefreshIfStale(context.Background(), "https://docs.example.com/page"); err != nil {
		t.Fatal(err)
	}
	if fx.fetcher.calls != 0 {
		t.Fatalf("fresh parent fallback should not fetch, calls=%d", fx.fetcher.calls)
	}
}

func TestFaviconUseCasePropagatesRepositoryErrors(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.repo.err = errors.New("database unavailable")
	if _, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{}); !errors.Is(err, fx.repo.err) {
		t.Fatalf("Resolve err=%v, want %v", err, fx.repo.err)
	}
	if _, err := fx.uc.Observe(context.Background(), "https://example.com", "https://cdn.example/icon.png", []byte("new"), favicon.SourceEngine, "image/png"); !errors.Is(err, fx.repo.err) {
		t.Fatalf("Observe err=%v, want %v", err, fx.repo.err)
	}
	if err := fx.uc.RefreshIfStale(context.Background(), "https://example.com"); !errors.Is(err, fx.repo.err) {
		t.Fatalf("RefreshIfStale err=%v, want %v", err, fx.repo.err)
	}
}

func TestFaviconResolvePropagatesBlobErrors(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seed("example.com", []byte("cached"), false)
	fx.blobs.err = errors.New("disk unavailable")
	if _, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{}); !errors.Is(err, fx.blobs.err) {
		t.Fatalf("Resolve err=%v, want %v", err, fx.blobs.err)
	}
}

func TestFaviconResolveBlockingRefreshesWhenMetadataFreshButBlobMissing(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.repo.byKey["example.com"] = &favicon.Metadata{Key: "example.com", PageURL: "https://example.com", ContentHash: favicon.Hash([]byte("old")), LastCheckedAt: fx.now}
	got, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{AllowBlockingRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "example.com" || fx.fetcher.calls != 1 || string(got.Bytes) != "png32:fetched" {
		t.Fatalf("unexpected resolve after forced refresh: got=%#v calls=%d bytes=%q", got, fx.fetcher.calls, got.Bytes)
	}
}

func TestFaviconResolveBlockingPropagatesFetchErrors(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.fetcher.err = errors.New("network unavailable")
	if _, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{AllowBlockingRefresh: true}); !errors.Is(err, fx.fetcher.err) {
		t.Fatalf("Resolve err=%v, want %v", err, fx.fetcher.err)
	}
}

func TestFaviconRefreshFromIconURLsUsesPageURLKey(t *testing.T) {
	fx := newFaviconFixture(t)
	if err := fx.uc.RefreshFromIconURLs(context.Background(), "https://docs.example.com/page", []string{"https://cdn.example.net/icon.png"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fx.repo.byKey["docs.example.com/page"]; !ok {
		t.Fatalf("page URL key was not stored; keys=%v", fx.repo.byKey)
	}
	if _, ok := fx.repo.byKey["cdn.example.net/icon.png"]; ok {
		t.Fatal("icon URL should not be used as cache key")
	}
}

func TestFaviconRefreshStoresDuckDuckGoFallbackUnderHostKey(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.fetcher.responses = map[string]faviconFetchResponse{
		"https://example.com/favicon.ico": {
			bytes:       []byte("host-fallback"),
			contentType: "image/png",
			resolvedKey: "github.com",
		},
	}

	if err := fx.uc.RefreshIfStale(context.Background(), "https://github.com/bnema/gordon/pull/123"); err != nil {
		t.Fatal(err)
	}
	if _, ok := fx.repo.byKey["github.com"]; !ok {
		t.Fatalf("host fallback key was not stored; keys=%v", fx.repo.byKey)
	}
	if _, ok := fx.repo.byKey["github.com/bnema/gordon/pull/123"]; ok {
		t.Fatal("duckduckgo host fallback should not be stored under the exact path key")
	}
}

func TestFaviconRefreshRejectsResolvedKeyOutsideCandidateHierarchy(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.fetcher.responses = map[string]faviconFetchResponse{
		"https://example.com/favicon.ico": {
			bytes:       []byte("bad-fallback"),
			contentType: "image/png",
			resolvedKey: "evil.example.com",
		},
	}

	err := fx.uc.RefreshIfStale(context.Background(), "https://github.com/bnema/gordon/pull/123")
	if !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("RefreshIfStale err=%v, want favicon miss", err)
	}
	if len(fx.repo.byKey) != 0 {
		t.Fatalf("invalid resolved key should not persist metadata: %v", fx.repo.byKey)
	}
}

func TestFaviconRefreshRejectsFetcherPageURLOutsideRequestedHierarchy(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.fetcher.responses = map[string]faviconFetchResponse{
		"https://example.com/favicon.ico": {
			bytes:       []byte("bad-page-url"),
			contentType: "image/png",
			pageURL:     "https://evil.example.com/pwned",
			resolvedKey: "evil.example.com/pwned",
		},
	}

	err := fx.uc.RefreshIfStale(context.Background(), "https://github.com/bnema/gordon/pull/123")
	if !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("RefreshIfStale err=%v, want favicon miss", err)
	}
	if len(fx.repo.byKey) != 0 {
		t.Fatalf("fetcher-controlled pageURL should not expand storage scope: %v", fx.repo.byKey)
	}
}

func TestFaviconRefreshFromIconURLsSkipsUnsupportedCandidates(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.fetcher.responses = map[string]faviconFetchResponse{
		"https://cdn.example.net/icon.svg": {bytes: []byte("svg"), contentType: "image/svg+xml"},
		"https://cdn.example.net/icon.png": {bytes: []byte("png"), contentType: "image/png"},
	}
	fx.converter.errByContentType = map[string]error{"image/svg+xml": ErrFaviconMiss}

	err := fx.uc.RefreshFromIconURLs(context.Background(), "https://docs.example.com/page", []string{
		"https://cdn.example.net/icon.svg",
		"https://cdn.example.net/icon.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fx.fetcher.calls != 2 {
		t.Fatalf("fetcher calls=%d, want 2", fx.fetcher.calls)
	}
	meta := fx.repo.byKey["docs.example.com/page"]
	if meta == nil {
		t.Fatalf("expected page URL key to be stored; keys=%v", fx.repo.byKey)
	}
	if meta.SourceURL != "https://cdn.example.net/icon.png" || meta.ContentType != "image/png" {
		t.Fatalf("stored meta = %+v, want png candidate", meta)
	}
	if string(fx.blobs.original["docs.example.com/page"]) != "png" {
		t.Fatalf("stored original = %q, want png", fx.blobs.original["docs.example.com/page"])
	}
}

func TestFaviconObserveConversionErrorDoesNotMutateExistingAssets(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seedOriginal([]byte("old"))
	fx.blobs.png["example.com"] = []byte("old-png")
	fx.blobs.sized["example.com"] = map[int][]byte{32: []byte("old-32")}
	fx.converter.err = errors.New("unsupported image")

	if _, err := fx.uc.Observe(context.Background(), "https://example.com", "https://example.com/favicon.ico", []byte("bad"), favicon.SourceEngine, "image/svg+xml"); !errors.Is(err, fx.converter.err) {
		t.Fatalf("Observe err=%v, want %v", err, fx.converter.err)
	}
	if string(fx.blobs.original["example.com"]) != "old" || string(fx.blobs.png["example.com"]) != "old-png" || string(fx.blobs.sized["example.com"][32]) != "old-32" {
		t.Fatalf("existing assets mutated on conversion error")
	}
	if fx.blobs.writeOriginals != 0 || fx.blobs.removeDerived != 0 || fx.invalidators.calls != 0 {
		t.Fatalf("unexpected mutation counters writes=%d remove=%d invalidates=%d", fx.blobs.writeOriginals, fx.blobs.removeDerived, fx.invalidators.calls)
	}
}

func TestFaviconObserveUnavailableWebPPreservesCachedAssets(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seedOriginal([]byte("old"))
	fx.blobs.png["example.com"] = []byte("old-png")
	fx.blobs.sized["example.com"] = map[int][]byte{32: []byte("old-32")}
	fx.converter.errByContentType = map[string]error{"image/webp": ErrFaviconMiss}

	_, err := fx.uc.Observe(context.Background(), "https://example.com", "https://example.com/favicon.webp", []byte("webp"), favicon.SourceEngine, "image/webp")
	if !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("Observe err=%v, want favicon miss", err)
	}
	if string(fx.blobs.original["example.com"]) != "old" || string(fx.blobs.png["example.com"]) != "old-png" || string(fx.blobs.sized["example.com"][32]) != "old-32" {
		t.Fatalf("cached assets changed after unavailable WebP decoder")
	}
	if fx.blobs.writeOriginals != 0 || fx.blobs.removeDerived != 0 || fx.invalidators.calls != 0 {
		t.Fatalf("unavailable decoder mutated cache: writes=%d removes=%d invalidations=%d", fx.blobs.writeOriginals, fx.blobs.removeDerived, fx.invalidators.calls)
	}
}

func TestFaviconEnsureSizedPropagatesReadSizedErrors(t *testing.T) {
	fx := newFaviconFixture(t)
	fx.seedOriginal([]byte("old"))
	fx.blobs.readSizedErr = errors.New("sized store unavailable")
	if err := fx.uc.EnsureSized(context.Background(), "example.com", 32); !errors.Is(err, fx.blobs.readSizedErr) {
		t.Fatalf("EnsureSized err=%v, want %v", err, fx.blobs.readSizedErr)
	}
	if fx.converter.calls != 0 {
		t.Fatal("converter called after non-miss ReadSizedPNG error")
	}
}

type faviconFixture struct {
	repo         *faviconRepoState
	blobs        *faviconBlobStoreState
	converter    *faviconConverterState
	scheduler    *faviconSchedulerState
	invalidators *faviconInvalidatorsState
	fetcher      *faviconFetcherState
	uc           *FaviconUseCase
	now          time.Time
}

type faviconTestContextKey string

const faviconFixtureBackgroundKey faviconTestContextKey = "bg"

func newFaviconFixture(t *testing.T) *faviconFixture {
	t.Helper()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fx := &faviconFixture{
		repo:         newFaviconRepoState(),
		blobs:        newFaviconBlobStoreState(),
		converter:    &faviconConverterState{},
		scheduler:    &faviconSchedulerState{seen: map[favicon.Key]bool{}},
		invalidators: &faviconInvalidatorsState{},
		fetcher:      &faviconFetcherState{},
		now:          now,
	}

	repo := mockFaviconRepository(t, fx.repo)
	blobs := mockFaviconBlobStore(t, fx.blobs)
	converter := mockFaviconConverter(t, fx.converter)
	scheduler := mockFaviconScheduler(t, fx.scheduler)
	invalidators := mockFaviconInvalidators(t, fx.invalidators)
	fetcher := mockFaviconFetcher(t, fx.fetcher)

	fx.uc = NewFaviconUseCase(FaviconDeps{
		Repository:   repo,
		BlobStore:    blobs,
		Converter:    converter,
		Scheduler:    scheduler,
		Invalidators: invalidators,
		Fetcher:      fetcher,
		Now:          func() time.Time { return fx.now },
		Background:   context.WithValue(context.Background(), faviconFixtureBackgroundKey, true),
	})
	return fx
}

func mockFaviconRepository(t *testing.T, state *faviconRepoState) *portmocks.MockFaviconRepository {
	t.Helper()

	repo := portmocks.NewMockFaviconRepository(t)
	repo.EXPECT().Get(mock.Anything, mock.Anything).RunAndReturn(state.get).Maybe()
	repo.EXPECT().FindFirst(mock.Anything, mock.Anything).RunAndReturn(state.findFirst).Maybe()
	repo.EXPECT().Upsert(mock.Anything, mock.Anything).RunAndReturn(state.upsert).Maybe()
	repo.EXPECT().UpdateLastChecked(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.updateLastChecked).Maybe()
	repo.EXPECT().Delete(mock.Anything, mock.Anything).RunAndReturn(state.delete).Maybe()
	return repo
}

func mockFaviconBlobStore(t *testing.T, state *faviconBlobStoreState) *portmocks.MockFaviconBlobStore {
	t.Helper()

	blobs := portmocks.NewMockFaviconBlobStore(t)
	blobs.EXPECT().ReadOriginal(mock.Anything, mock.Anything).RunAndReturn(state.readOriginal).Maybe()
	blobs.EXPECT().WriteOriginal(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.writeOriginal).Maybe()
	blobs.EXPECT().ReadPNG(mock.Anything, mock.Anything).RunAndReturn(state.readPNG).Maybe()
	blobs.EXPECT().WritePNG(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.writePNG).Maybe()
	blobs.EXPECT().ReadSizedPNG(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.readSizedPNG).Maybe()
	blobs.EXPECT().WriteSizedPNG(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.writeSizedPNG).Maybe()
	blobs.EXPECT().RemoveDerived(mock.Anything, mock.Anything).RunAndReturn(state.removeDerivedForKey).Maybe()
	return blobs
}

func mockFaviconConverter(t *testing.T, state *faviconConverterState) *portmocks.MockFaviconImageConverter {
	t.Helper()

	converter := portmocks.NewMockFaviconImageConverter(t)
	converter.EXPECT().Convert(mock.Anything, mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.convert).Maybe()
	return converter
}

func mockFaviconScheduler(t *testing.T, state *faviconSchedulerState) *portmocks.MockFaviconRefreshScheduler {
	t.Helper()

	scheduler := portmocks.NewMockFaviconRefreshScheduler(t)
	scheduler.EXPECT().Schedule(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(state.schedule).Maybe()
	return scheduler
}

func mockFaviconInvalidators(t *testing.T, state *faviconInvalidatorsState) *portmocks.MockFaviconInvalidators {
	t.Helper()

	invalidators := portmocks.NewMockFaviconInvalidators(t)
	invalidators.EXPECT().InvalidateAll(mock.Anything, mock.Anything).RunAndReturn(state.invalidateAll).Maybe()
	return invalidators
}

func mockFaviconFetcher(t *testing.T, state *faviconFetcherState) *portmocks.MockFaviconFetcher {
	t.Helper()

	fetcher := portmocks.NewMockFaviconFetcher(t)
	fetcher.EXPECT().Fetch(mock.Anything, mock.Anything).RunAndReturn(state.fetch).Maybe()
	return fetcher
}

func (f *faviconFixture) seed(key string, sized []byte, stale bool) {
	checked := f.now
	if stale {
		checked = f.now.Add(-favicon.DefaultTTL - time.Hour)
	}
	f.repo.byKey[favicon.Key(key)] = &favicon.Metadata{Key: favicon.Key(key), PageURL: "https://" + key, ContentHash: favicon.Hash([]byte("orig")), LastCheckedAt: checked}
	f.blobs.sized[favicon.Key(key)] = map[int][]byte{32: sized}
}

func (f *faviconFixture) seedOriginal(b []byte) {
	const contentType = "image/png"
	key := favicon.Key("example.com")
	f.repo.byKey[key] = &favicon.Metadata{
		Key:           key,
		PageURL:       "https://example.com",
		ContentHash:   favicon.Hash(b),
		ContentType:   contentType,
		LastCheckedAt: f.now.Add(-time.Hour),
	}
	f.blobs.original[key] = b
	f.blobs.contentTypes[key] = contentType
}

type faviconRepoState struct {
	byKey map[favicon.Key]*favicon.Metadata
	err   error
}

func newFaviconRepoState() *faviconRepoState {
	return &faviconRepoState{byKey: map[favicon.Key]*favicon.Metadata{}}
}

func (r *faviconRepoState) get(_ context.Context, key favicon.Key) (*favicon.Metadata, error) {
	if r.err != nil {
		return nil, r.err
	}
	if m := r.byKey[key]; m != nil {
		cp := *m
		return &cp, nil
	}
	return nil, nil
}

func (r *faviconRepoState) findFirst(ctx context.Context, keys []favicon.Key) (*favicon.Metadata, error) {
	if r.err != nil {
		return nil, r.err
	}
	for _, k := range keys {
		if m, _ := r.get(ctx, k); m != nil {
			return m, nil
		}
	}
	return nil, nil
}

func (r *faviconRepoState) upsert(_ context.Context, meta favicon.Metadata) error {
	if r.err != nil {
		return r.err
	}
	cp := meta
	r.byKey[meta.Key] = &cp
	return nil
}

func (r *faviconRepoState) updateLastChecked(_ context.Context, key favicon.Key, hash string, checkedAt time.Time) error {
	if r.err != nil {
		return r.err
	}
	r.byKey[key].ContentHash = hash
	r.byKey[key].LastCheckedAt = checkedAt
	return nil
}

func (r *faviconRepoState) delete(_ context.Context, key favicon.Key) error {
	if r.err != nil {
		return r.err
	}
	delete(r.byKey, key)
	return nil
}

type faviconBlobStoreState struct {
	original, png                 map[favicon.Key][]byte
	sized                         map[favicon.Key]map[int][]byte
	contentTypes                  map[favicon.Key]string
	writeOriginals, removeDerived int
	err                           error
	readSizedErr                  error
}

func newFaviconBlobStoreState() *faviconBlobStoreState {
	return &faviconBlobStoreState{
		original:     map[favicon.Key][]byte{},
		png:          map[favicon.Key][]byte{},
		sized:        map[favicon.Key]map[int][]byte{},
		contentTypes: map[favicon.Key]string{},
	}
}

func (b *faviconBlobStoreState) readOriginal(_ context.Context, key favicon.Key) ([]byte, string, error) {
	if b.err != nil {
		return nil, "", b.err
	}
	v := b.original[key]
	if v == nil {
		return nil, "", ErrFaviconMiss
	}
	return v, b.contentTypes[key], nil
}

func (b *faviconBlobStoreState) writeOriginal(_ context.Context, key favicon.Key, data []byte, ct string) error {
	if b.err != nil {
		return b.err
	}
	b.writeOriginals++
	b.original[key] = data
	b.contentTypes[key] = ct
	return nil
}

func (b *faviconBlobStoreState) readPNG(_ context.Context, key favicon.Key) ([]byte, string, error) {
	if b.err != nil {
		return nil, "", b.err
	}
	v := b.png[key]
	if v == nil {
		return nil, "", ErrFaviconMiss
	}
	return v, "image/png", nil
}

func (b *faviconBlobStoreState) writePNG(_ context.Context, key favicon.Key, data []byte) error {
	if b.err != nil {
		return b.err
	}
	b.png[key] = data
	return nil
}

func (b *faviconBlobStoreState) readSizedPNG(_ context.Context, key favicon.Key, size int) ([]byte, string, error) {
	if b.err != nil {
		return nil, "", b.err
	}
	if b.readSizedErr != nil {
		return nil, "", b.readSizedErr
	}
	if b.sized[key] == nil || b.sized[key][size] == nil {
		return nil, "", ErrFaviconMiss
	}
	return b.sized[key][size], "image/png", nil
}

func (b *faviconBlobStoreState) writeSizedPNG(_ context.Context, key favicon.Key, size int, data []byte) error {
	if b.err != nil {
		return b.err
	}
	if b.sized[key] == nil {
		b.sized[key] = map[int][]byte{}
	}
	b.sized[key][size] = data
	return nil
}

func (b *faviconBlobStoreState) removeDerivedForKey(_ context.Context, key favicon.Key) error {
	if b.err != nil {
		return b.err
	}
	b.removeDerived++
	delete(b.png, key)
	delete(b.sized, key)
	return nil
}

type faviconConverterState struct {
	calls            int
	err              error
	errByContentType map[string]error
}

func (c *faviconConverterState) convert(_ context.Context, original []byte, contentType string, sizes []int) (*appport.ConvertedFavicon, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	if err := c.errByContentType[contentType]; err != nil {
		return nil, err
	}
	out := &appport.ConvertedFavicon{PNG: append([]byte("png:"), original...), SizedPNG: map[int][]byte{}}
	for _, s := range sizes {
		out.SizedPNG[s] = append([]byte("png32:"), original...)
	}
	return out, nil
}

type faviconSchedulerState struct {
	seen              map[favicon.Key]bool
	works             map[favicon.Key]func(context.Context)
	schedules         int
	usedCallerContext bool
}

func (s *faviconSchedulerState) schedule(ctx context.Context, key favicon.Key, work func(context.Context)) bool {
	if ctx.Value(faviconFixtureBackgroundKey) != true {
		s.usedCallerContext = true
	}
	if s.seen[key] {
		return false
	}
	if s.works == nil {
		s.works = map[favicon.Key]func(context.Context){}
	}
	s.seen[key] = true
	s.works[key] = work
	s.schedules++
	return true
}

func (s *faviconSchedulerState) run(key favicon.Key) {
	if work := s.works[key]; work != nil {
		work(context.Background())
	}
}

type faviconInvalidatorsState struct{ calls int }

func (i *faviconInvalidatorsState) invalidateAll(_ context.Context, _ favicon.Key) error {
	i.calls++
	return nil
}

type faviconFetchResponse struct {
	bytes       []byte
	contentType string
	pageURL     string
	resolvedKey favicon.Key
}

type faviconFetcherState struct {
	calls     int
	err       error
	responses map[string]faviconFetchResponse
}

func (f *faviconFetcherState) fetch(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	iconURL := req.IconURL
	if iconURL == "" {
		iconURL = "https://example.com/favicon.ico"
	}
	response := f.responses[iconURL]
	if response.bytes == nil {
		response = faviconFetchResponse{bytes: []byte("fetched"), contentType: "image/png"}
	}
	pageURL := req.PageURL
	if response.pageURL != "" {
		pageURL = response.pageURL
	}
	return &appport.FaviconFetchedIcon{
		PageURL:     pageURL,
		IconURL:     iconURL,
		ResolvedKey: response.resolvedKey,
		Bytes:       response.bytes,
		Source:      favicon.SourceDuckDuckGo,
		ContentType: response.contentType,
	}, nil
}
