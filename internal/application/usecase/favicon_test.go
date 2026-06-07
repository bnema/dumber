package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/favicon"
)

func TestFaviconResolveExactHit(t *testing.T) {
	fx := newFaviconFixture()
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
	fx := newFaviconFixture()
	fx.seed("example.com", []byte("parent"), false)
	got, err := fx.uc.Resolve(context.Background(), "https://docs.example.com/a", 32, ResolveOptions{Purpose: ResolvePurposeUI})
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != "example.com" || string(got.Bytes) != "parent" {
		t.Fatalf("unexpected fallback: %#v", got)
	}
}

func TestFaviconResolveRepairsOrphanCanonicalBlobMetadata(t *testing.T) {
	fx := newFaviconFixture()
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
	fx := newFaviconFixture()
	fx.uc.converter = nil
	fx.blobs.original["example.com"] = []byte("original")
	fx.blobs.contentTypes["example.com"] = "image/png"

	_, err := fx.uc.ResolveSystemviewIcon(context.Background(), "example.com", SystemviewIconSize)
	if !errors.Is(err, ErrFaviconMiss) {
		t.Fatalf("expected favicon miss, got %v", err)
	}
}

func TestFaviconResolveStaleSchedulesDedupedBackgroundRefresh(t *testing.T) {
	fx := newFaviconFixture()
	fx.seed("example.com", []byte("old"), true)
	for i := 0; i < 2; i++ {
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
	fx := newFaviconFixture()
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
	fx := newFaviconFixture()
	fx.seedOriginal([]byte("same"))
	if _, err := fx.uc.Observe(context.Background(), "https://example.com/page", "https://cdn.test/icon.png", []byte("same"), favicon.SourceEngine, "image/png"); err != nil {
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
	fx := newFaviconFixture()
	fx.seedOriginal([]byte("old"))
	if _, err := fx.uc.Observe(context.Background(), "https://example.com/page", "https://cdn.test/icon.png", []byte("new"), favicon.SourceEngine, "image/png"); err != nil {
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
	fx := newFaviconFixture()
	fx.uc.converter = nil
	if _, err := fx.uc.Observe(context.Background(), "https://example.com/page", "https://example.com/icon.png", []byte("raw"), favicon.SourceEngine, "image/png"); err != nil {
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
	fx := newFaviconFixture()
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
	fx := newFaviconFixture()
	fx.seed("example.com", []byte("parent"), false)
	if err := fx.uc.RefreshIfStale(context.Background(), "https://docs.example.com/page"); err != nil {
		t.Fatal(err)
	}
	if fx.fetcher.calls != 0 {
		t.Fatalf("fresh parent fallback should not fetch, calls=%d", fx.fetcher.calls)
	}
}

func TestFaviconUseCasePropagatesRepositoryErrors(t *testing.T) {
	fx := newFaviconFixture()
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
	fx := newFaviconFixture()
	fx.seed("example.com", []byte("cached"), false)
	fx.blobs.err = errors.New("disk unavailable")
	if _, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{}); !errors.Is(err, fx.blobs.err) {
		t.Fatalf("Resolve err=%v, want %v", err, fx.blobs.err)
	}
}

func TestFaviconResolveBlockingRefreshesWhenMetadataFreshButBlobMissing(t *testing.T) {
	fx := newFaviconFixture()
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
	fx := newFaviconFixture()
	fx.fetcher.err = errors.New("network unavailable")
	if _, err := fx.uc.Resolve(context.Background(), "https://example.com", 32, ResolveOptions{AllowBlockingRefresh: true}); !errors.Is(err, fx.fetcher.err) {
		t.Fatalf("Resolve err=%v, want %v", err, fx.fetcher.err)
	}
}

func TestFaviconRefreshFromIconURLsUsesPageURLKey(t *testing.T) {
	fx := newFaviconFixture()
	if err := fx.uc.RefreshFromIconURLs(context.Background(), "https://docs.example.com/page", []string{"https://cdn.example.net/icon.png"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := fx.repo.byKey["docs.example.com"]; !ok {
		t.Fatalf("page URL key was not stored; keys=%v", fx.repo.byKey)
	}
	if _, ok := fx.repo.byKey["cdn.example.net"]; ok {
		t.Fatal("icon URL host should not be used as cache key")
	}
}

func TestFaviconObserveConversionErrorDoesNotMutateExistingAssets(t *testing.T) {
	fx := newFaviconFixture()
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

func TestFaviconEnsureSizedPropagatesReadSizedErrors(t *testing.T) {
	fx := newFaviconFixture()
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
	repo         *fakeFaviconRepo
	blobs        *fakeBlobStore
	converter    *fakeConverter
	scheduler    *fakeScheduler
	invalidators *fakeInvalidators
	fetcher      *fakeFetcher
	uc           *FaviconUseCase
	now          time.Time
}

type faviconTestContextKey string

const faviconFixtureBackgroundKey faviconTestContextKey = "bg"

func newFaviconFixture() *faviconFixture {
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fx := &faviconFixture{
		repo:         newFakeRepo(),
		blobs:        newFakeBlobStore(),
		converter:    &fakeConverter{},
		scheduler:    &fakeScheduler{seen: map[favicon.Key]bool{}},
		invalidators: &fakeInvalidators{},
		fetcher:      &fakeFetcher{},
		now:          now,
	}
	fx.uc = NewFaviconUseCase(FaviconDeps{
		Repository:   fx.repo,
		BlobStore:    fx.blobs,
		Converter:    fx.converter,
		Scheduler:    fx.scheduler,
		Invalidators: fx.invalidators,
		Fetcher:      fx.fetcher,
		Now:          func() time.Time { return fx.now },
		Background:   context.WithValue(context.Background(), faviconFixtureBackgroundKey, true),
	})
	return fx
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

type fakeFaviconRepo struct {
	byKey map[favicon.Key]*favicon.Metadata
	err   error
}

func newFakeRepo() *fakeFaviconRepo {
	return &fakeFaviconRepo{byKey: map[favicon.Key]*favicon.Metadata{}}
}
func (r *fakeFaviconRepo) Get(_ context.Context, key favicon.Key) (*favicon.Metadata, error) {
	if r.err != nil {
		return nil, r.err
	}
	if m := r.byKey[key]; m != nil {
		cp := *m
		return &cp, nil
	}
	return nil, nil
}
func (r *fakeFaviconRepo) FindFirst(ctx context.Context, keys []favicon.Key) (*favicon.Metadata, error) {
	if r.err != nil {
		return nil, r.err
	}
	for _, k := range keys {
		if m, _ := r.Get(ctx, k); m != nil {
			return m, nil
		}
	}
	return nil, nil
}
func (r *fakeFaviconRepo) Upsert(_ context.Context, meta favicon.Metadata) error {
	if r.err != nil {
		return r.err
	}
	cp := meta
	r.byKey[meta.Key] = &cp
	return nil
}
func (r *fakeFaviconRepo) UpdateLastChecked(_ context.Context, key favicon.Key, hash string, checkedAt time.Time) error {
	if r.err != nil {
		return r.err
	}
	r.byKey[key].ContentHash = hash
	r.byKey[key].LastCheckedAt = checkedAt
	return nil
}
func (r *fakeFaviconRepo) Delete(_ context.Context, key favicon.Key) error {
	if r.err != nil {
		return r.err
	}
	delete(r.byKey, key)
	return nil
}

type fakeBlobStore struct {
	original, png                 map[favicon.Key][]byte
	sized                         map[favicon.Key]map[int][]byte
	contentTypes                  map[favicon.Key]string
	writeOriginals, removeDerived int
	err                           error
	readSizedErr                  error
}

func newFakeBlobStore() *fakeBlobStore {
	return &fakeBlobStore{original: map[favicon.Key][]byte{}, png: map[favicon.Key][]byte{}, sized: map[favicon.Key]map[int][]byte{}, contentTypes: map[favicon.Key]string{}}
}
func (b *fakeBlobStore) ReadOriginal(_ context.Context, key favicon.Key) ([]byte, string, error) {
	if b.err != nil {
		return nil, "", b.err
	}
	v := b.original[key]
	if v == nil {
		return nil, "", ErrFaviconMiss
	}
	return v, b.contentTypes[key], nil
}
func (b *fakeBlobStore) WriteOriginal(_ context.Context, key favicon.Key, data []byte, ct string) error {
	if b.err != nil {
		return b.err
	}
	b.writeOriginals++
	b.original[key] = data
	b.contentTypes[key] = ct
	return nil
}
func (b *fakeBlobStore) ReadPNG(_ context.Context, key favicon.Key) ([]byte, string, error) {
	if b.err != nil {
		return nil, "", b.err
	}
	v := b.png[key]
	if v == nil {
		return nil, "", ErrFaviconMiss
	}
	return v, "image/png", nil
}
func (b *fakeBlobStore) WritePNG(_ context.Context, key favicon.Key, data []byte) error {
	if b.err != nil {
		return b.err
	}
	b.png[key] = data
	return nil
}
func (b *fakeBlobStore) ReadSizedPNG(_ context.Context, key favicon.Key, size int) ([]byte, string, error) {
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
func (b *fakeBlobStore) WriteSizedPNG(_ context.Context, key favicon.Key, size int, data []byte) error {
	if b.err != nil {
		return b.err
	}
	if b.sized[key] == nil {
		b.sized[key] = map[int][]byte{}
	}
	b.sized[key][size] = data
	return nil
}
func (b *fakeBlobStore) RemoveDerived(_ context.Context, key favicon.Key) error {
	if b.err != nil {
		return b.err
	}
	b.removeDerived++
	delete(b.png, key)
	delete(b.sized, key)
	return nil
}

type fakeConverter struct {
	calls int
	err   error
}

func (c *fakeConverter) Convert(_ context.Context, original []byte, _ string, sizes []int) (*appport.ConvertedFavicon, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	out := &appport.ConvertedFavicon{PNG: append([]byte("png:"), original...), SizedPNG: map[int][]byte{}}
	for _, s := range sizes {
		out.SizedPNG[s] = append([]byte("png32:"), original...)
	}
	return out, nil
}

type fakeScheduler struct {
	seen              map[favicon.Key]bool
	works             map[favicon.Key]func(context.Context)
	schedules         int
	usedCallerContext bool
}

func (s *fakeScheduler) Schedule(ctx context.Context, key favicon.Key, work func(context.Context)) bool {
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

func (s *fakeScheduler) run(key favicon.Key) {
	if work := s.works[key]; work != nil {
		work(context.Background())
	}
}

type fakeInvalidators struct{ calls int }

func (i *fakeInvalidators) InvalidateAll(_ context.Context, _ favicon.Key) error {
	i.calls++
	return nil
}

type fakeFetcher struct {
	calls int
	err   error
}

func (f *fakeFetcher) Fetch(_ context.Context, req appport.FaviconFetchRequest) (*appport.FaviconFetchedIcon, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	iconURL := req.IconURL
	if iconURL == "" {
		iconURL = "https://example.com/favicon.ico"
	}
	return &appport.FaviconFetchedIcon{PageURL: req.PageURL, IconURL: iconURL, Bytes: []byte("fetched"), Source: favicon.SourceDuckDuckGo, ContentType: "image/png"}, nil
}
