package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type homeIconFixture struct {
	src   *portmocks.MockHomeIconSource
	conv  *portmocks.MockFaviconImageConverter
	cache *portmocks.MockHomeIconCache
	now   time.Time
	uc    *HomeIconUseCase
}

func newHomeIconFixture(t *testing.T) *homeIconFixture {
	fx := &homeIconFixture{
		src:   portmocks.NewMockHomeIconSource(t),
		conv:  portmocks.NewMockFaviconImageConverter(t),
		cache: portmocks.NewMockHomeIconCache(t),
		now:   time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC),
	}
	fx.uc = NewHomeIconUseCase(HomeIconDeps{
		Source: fx.src, Converter: fx.conv, Cache: fx.cache,
		Now: func() time.Time { return fx.now }, TTL: time.Hour,
	})
	return fx
}

func img(ct, data string) *appport.HomeIconImage {
	return &appport.HomeIconImage{Bytes: []byte(data), ContentType: ct}
}

func converted(png string) *appport.ConvertedFavicon {
	return &appport.ConvertedFavicon{SizedPNG: map[int][]byte{HomeIconSize: []byte(png)}}
}

const svc = "http://jelly.local:8096/"

func TestHomeIconOverrideWins(t *testing.T) {
	fx := newHomeIconFixture(t)
	fx.src.EXPECT().ReadLocalIcon(mock.Anything, "/cfg/icons/j.svg").Return(img("image/svg+xml", "<svg/>"), nil)
	fx.conv.EXPECT().Convert(mock.Anything, []byte("<svg/>"), "image/svg+xml", []int{128}).Return(converted("p128"), nil)
	got, ct, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "/cfg/icons/j.svg")
	if err != nil || string(got) != "p128" || ct != "image/png" {
		t.Fatalf("got %q %q %v", got, ct, err)
	}
}

func TestHomeIconBadOverrideFallsBackToDiscovery(t *testing.T) {
	fx := newHomeIconFixture(t)
	fx.src.EXPECT().ReadLocalIcon(mock.Anything, "/missing.png").Return(nil, errors.New("no such file"))
	fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false)
	fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).Return([]string{"http://jelly.local:8096/favicon.ico"}, nil)
	fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "http://jelly.local:8096/favicon.ico").Return(img("image/x-icon", "ico"), nil)
	fx.conv.EXPECT().Convert(mock.Anything, []byte("ico"), "image/x-icon", []int{128}).Return(converted("ico128"), nil)
	fx.cache.EXPECT().Put(mock.Anything, svc, []byte("ico128")).Return(nil)
	got, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "/missing.png")
	if err != nil || string(got) != "ico128" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestHomeIconFreshCacheHit(t *testing.T) {
	fx := newHomeIconFixture(t)
	fx.cache.EXPECT().Get(mock.Anything, svc).Return([]byte("cached"), fx.now.Add(-time.Minute), true)
	got, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
	if err != nil || string(got) != "cached" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestHomeIconFallsThroughFailedCandidates(t *testing.T) {
	fx := newHomeIconFixture(t)
	// Stale cache entry triggers a refresh.
	fx.cache.EXPECT().Get(mock.Anything, svc).Return([]byte("old"), fx.now.Add(-2*time.Hour), true)
	fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).Return([]string{"u/a.svg", "u/b.png", "u/c.png"}, nil)
	fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/a.svg").Return(img("image/svg+xml", "bad"), nil)
	fx.conv.EXPECT().Convert(mock.Anything, []byte("bad"), "image/svg+xml", []int{128}).Return(nil, ErrFaviconMiss)
	fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/b.png").Return(nil, errors.New("404"))
	fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/c.png").Return(img("image/png", "c"), nil)
	fx.conv.EXPECT().Convert(mock.Anything, []byte("c"), "image/png", []int{128}).Return(converted("c128"), nil)
	fx.cache.EXPECT().Put(mock.Anything, svc, []byte("c128")).Return(nil)
	got, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
	if err != nil || string(got) != "c128" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestHomeIconNotFoundSentinel(t *testing.T) {
	fx := newHomeIconFixture(t)
	fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false)
	fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).Return([]string{"u/favicon.ico"}, errors.New("page down"))
	fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/favicon.ico").Return(nil, errors.New("404"))
	_, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
	if !errors.Is(err, ErrHomeIconNotFound) || !errors.Is(err, appport.ErrHomeIconNotFound) {
		t.Fatalf("want sentinel, got %v", err)
	}
}

func TestHomeIconNegativeCacheSkipsRetryWithinTTL(t *testing.T) {
	fx := newHomeIconFixture(t)
	calls := 0
	fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false)
	fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).
		RunAndReturn(func(context.Context, string) ([]string, error) {
			calls++
			return nil, errors.New("down")
		})
	resolve := func() {
		_, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
		require.ErrorIs(t, err, ErrHomeIconNotFound)
	}
	resolve()
	resolve()
	fx.now = fx.now.Add(HomeIconNegativeTTL - time.Second)
	resolve()
	require.Equal(t, 1, calls, "failures within the negative TTL must not refetch")
	fx.now = fx.now.Add(2 * time.Second)
	resolve()
	require.Equal(t, 2, calls, "expired failure must be retried")
}

func TestHomeIconConcurrentCallsShareOneResolution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newHomeIconFixture(t)
		release := make(chan struct{})
		fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false).Once()
		fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).
			RunAndReturn(func(context.Context, string) ([]string, error) {
				<-release
				return []string{"u/a.png"}, nil
			}).Once()
		fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/a.png").Return(img("image/png", "a"), nil).Once()
		fx.conv.EXPECT().Convert(mock.Anything, []byte("a"), "image/png", []int{128}).Return(converted("a128"), nil).Once()
		fx.cache.EXPECT().Put(mock.Anything, svc, []byte("a128")).Return(nil).Once()

		const callers = 5
		results := make([]string, callers)
		var wg sync.WaitGroup
		for i := range callers {
			wg.Go(func() {
				got, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
				if err != nil {
					results[i] = "err:" + err.Error()
					return
				}
				results[i] = string(got)
			})
		}
		// Every caller is now either the resolver blocked on release or a
		// waiter blocked on the shared call.
		synctest.Wait()
		fx.uc.mu.Lock()
		require.Len(t, fx.uc.inflight, 1)
		fx.uc.mu.Unlock()
		close(release)
		wg.Wait()
		for _, r := range results {
			require.Equal(t, "a128", r)
		}
	})
}

func TestHomeIconPanicIsRecoveredAndRemembered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newHomeIconFixture(t)
		fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false).Once()
		fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).Return([]string{"u/bad.png"}, nil).Once()
		fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/bad.png").Return(img("image/png", "bad"), nil).Once()
		fx.conv.EXPECT().Convert(mock.Anything, []byte("bad"), "image/png", []int{128}).
			RunAndReturn(func(context.Context, []byte, string, []int) (*appport.ConvertedFavicon, error) {
				panic("decoder exploded")
			}).Once()

		_, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
		require.ErrorIs(t, err, ErrHomeIconNotFound)
		_, _, err = fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
		require.ErrorIs(t, err, ErrHomeIconNotFound, "the failure is remembered")
		fx.uc.mu.Lock()
		defer fx.uc.mu.Unlock()
		require.Empty(t, fx.uc.inflight)
	})
}

func TestHomeIconSharedTimeoutIsRemembered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newHomeIconFixture(t)
		fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false).Once()
		fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).Return([]string{"u/slow.png", "u/next.png"}, nil).Once()
		fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/slow.png").
			RunAndReturn(func(ctx context.Context, _, _ string) (*appport.HomeIconImage, error) {
				<-ctx.Done() // the shared 20s budget expires (fake clock)
				return nil, ctx.Err()
			}).Once()

		_, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		fx.uc.mu.Lock()
		_, remembered := fx.uc.failures[homeIconKey(svc, "")]
		fx.uc.mu.Unlock()
		require.True(t, remembered)
		_, _, err = fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
		require.ErrorIs(t, err, ErrHomeIconNotFound, "no second resolution within the TTL")
	})
}

func TestHomeIconExpiredFailuresArePrunedOnInsert(t *testing.T) {
	fx := newHomeIconFixture(t)
	fx.uc.failures["old"] = fx.now.Add(-2 * HomeIconNegativeTTL)
	fx.uc.failures["recent"] = fx.now.Add(-time.Minute)
	fx.uc.mu.Lock()
	fx.uc.rememberFailureLocked("new")
	fx.uc.mu.Unlock()
	require.NotContains(t, fx.uc.failures, "old")
	require.Contains(t, fx.uc.failures, "recent")
	require.Contains(t, fx.uc.failures, "new")
}

type stubHomeConfig struct{ layout entity.HomeLayout }

func (s stubHomeConfig) Load(context.Context) (appport.HomeConfigSnapshot, error) {
	return appport.HomeConfigSnapshot{Layout: s.layout}, nil
}
func (stubHomeConfig) Path() string                { return "" }
func (stubHomeConfig) EnsureExists() (bool, error) { return false, nil }

func newHomeIconByIDFixture(t *testing.T, services ...entity.HomeService) *homeIconFixture {
	fx := newHomeIconFixture(t)
	fx.uc.config = stubHomeConfig{layout: entity.HomeLayout{Groups: []entity.HomeGroup{{Name: "g", Services: services}}}}
	fx.uc.configDir = "/cfg"
	return fx
}

func TestHomeIconByIDUsesOverrideUnderConfigDir(t *testing.T) {
	fx := newHomeIconByIDFixture(t, entity.HomeService{ID: "jellyfin", URL: svc, Icon: "icons/j.svg"})
	fx.src.EXPECT().ReadLocalIcon(mock.Anything, "/cfg/icons/j.svg").Return(img("image/svg+xml", "<svg/>"), nil)
	fx.conv.EXPECT().Convert(mock.Anything, []byte("<svg/>"), "image/svg+xml", []int{128}).Return(converted("j128"), nil)
	got, ct, err := fx.uc.ResolveHomeIconByID(context.Background(), "jellyfin")
	require.NoError(t, err)
	require.Equal(t, "j128", string(got))
	require.Equal(t, "image/png", ct)
}

func TestHomeIconByIDRejectsUnknownBadAndEscapingIDs(t *testing.T) {
	fx := newHomeIconByIDFixture(t,
		entity.HomeService{ID: "evil", URL: svc, Icon: "../secret.png"},
		entity.HomeService{ID: "abs", URL: svc, Icon: "/etc/x.png"},
	)
	for _, id := range []string{"unknown", "", "../x", "UPPER", "evil", "abs"} {
		_, _, err := fx.uc.ResolveHomeIconByID(context.Background(), id)
		require.ErrorIs(t, err, appport.ErrHomeIconNotFound, id)
	}
}

func TestResolveUnderDir(t *testing.T) {
	cases := map[string]bool{"a.png": true, "icons/a.png": true, "../a.png": false, "icons/../../a.png": false, "/abs.png": false, ".": false}
	for rel, want := range cases {
		_, ok := resolveUnderDir("/cfg", rel)
		require.Equal(t, want, ok, rel)
	}
	_, ok := resolveUnderDir("", "a.png")
	require.False(t, ok)
}

func TestHomeIconLeaderCancelDoesNotFailFollowers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		fx := newHomeIconFixture(t)
		release := make(chan struct{})
		fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false).Once()
		fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).Return([]string{"u/favicon.ico"}, nil).Once()
		fx.src.EXPECT().FetchIcon(mock.Anything, mock.Anything, "u/favicon.ico").
			RunAndReturn(func(ctx context.Context, _, _ string) (*appport.HomeIconImage, error) {
				select {
				case <-release:
					return img("image/x-icon", "ico"), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}).Once()
		fx.conv.EXPECT().Convert(mock.Anything, []byte("ico"), "image/x-icon", []int{128}).Return(converted("ico128"), nil).Once()
		fx.cache.EXPECT().Put(mock.Anything, svc, []byte("ico128")).Return(nil).Once()

		leaderCtx, cancelLeader := context.WithCancel(context.Background())
		var leaderErr error
		var follower string
		var wg sync.WaitGroup
		wg.Go(func() { _, _, leaderErr = fx.uc.ResolveHomeServiceIcon(leaderCtx, svc, "") })
		synctest.Wait() // the shared fetch is blocked on release
		wg.Go(func() {
			got, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
			if err != nil {
				follower = "err:" + err.Error()
				return
			}
			follower = string(got)
		})
		synctest.Wait() // the follower waits on the same call
		cancelLeader()
		synctest.Wait()
		close(release)
		wg.Wait()
		require.ErrorIs(t, leaderErr, context.Canceled)
		require.Equal(t, "ico128", follower)
		fx.uc.mu.Lock()
		defer fx.uc.mu.Unlock()
		require.Empty(t, fx.uc.failures)
	})
}

func TestHomeIconByIDConfigEditBypassesFailureMemory(t *testing.T) {
	svcA := entity.HomeService{ID: "jellyfin", Name: "Jellyfin", URL: svc}
	fx := newHomeIconByIDFixture(t, svcA)
	calls := 0
	fx.cache.EXPECT().Get(mock.Anything, svc).Return(nil, time.Time{}, false)
	fx.src.EXPECT().DiscoverIconURLs(mock.Anything, svc).
		RunAndReturn(func(context.Context, string) ([]string, error) {
			calls++
			return nil, errors.New("down")
		})
	resolve := func() {
		_, _, err := fx.uc.ResolveHomeIconByID(context.Background(), "jellyfin")
		require.ErrorIs(t, err, ErrHomeIconNotFound)
	}
	resolve()
	resolve()
	require.Equal(t, 1, calls, "unchanged config must hit the failure memory")
	svcA.Name = "Jellyfin 2"
	fx.uc.config = stubHomeConfig{layout: entity.HomeLayout{Groups: []entity.HomeGroup{{Name: "g", Services: []entity.HomeService{svcA}}}}}
	resolve()
	require.Equal(t, 2, calls, "an edited service must retry immediately")
}

func TestHomeIconVersionTracksURLIconAndOverrideMTime(t *testing.T) {
	fx := newHomeIconFixture(t)
	fx.uc.configDir = "/cfg"
	ctx := context.Background()
	mtime := time.Unix(1000, 0)
	fx.src.EXPECT().LocalIconModTime(mock.Anything, "/cfg/icons/j.svg").
		RunAndReturn(func(context.Context, string) (time.Time, error) { return mtime, nil })

	base := entity.HomeService{ID: "j", URL: svc, Icon: "icons/j.svg"}
	v1 := fx.uc.HomeIconVersion(ctx, base)
	require.Len(t, v1, 12)
	require.Equal(t, v1, fx.uc.HomeIconVersion(ctx, base), "stable when nothing changes")

	renamed := base
	renamed.Name = "Renamed"
	require.Equal(t, v1, fx.uc.HomeIconVersion(ctx, renamed), "unrelated fields do not bust the cache")

	moved := base
	moved.URL = "http://jelly.local:9000/"
	require.NotEqual(t, v1, fx.uc.HomeIconVersion(ctx, moved), "url change")

	mtime = mtime.Add(time.Second)
	require.NotEqual(t, v1, fx.uc.HomeIconVersion(ctx, base), "override file edited")

	noIcon := entity.HomeService{ID: "j", URL: svc}
	require.NotEqual(t, v1, fx.uc.HomeIconVersion(ctx, noIcon), "icon removed")

	escaping := entity.HomeService{ID: "j", URL: svc, Icon: "../x.png"}
	require.NotEmpty(t, fx.uc.HomeIconVersion(ctx, escaping), "escaping paths are never stat'ed but still versioned")
}

type stubIconVersioner struct{}

func (stubIconVersioner) HomeIconVersion(_ context.Context, s entity.HomeService) string {
	return "v-" + s.ID
}

func TestLoadHomeSetsIconVersions(t *testing.T) {
	ctx := context.Background()
	layout := entity.DefaultHomeLayout()
	layout.Groups = []entity.HomeGroup{{Name: "G", Services: []entity.HomeService{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}}}
	source := portmocks.NewMockHomeConfigSource(t)
	source.EXPECT().Load(ctx).Return(appport.HomeConfigSnapshot{Layout: layout}, nil)

	resp, err := NewHomeUseCase(HomeDeps{Source: source, IconVersions: stubIconVersioner{}}).LoadHome(ctx)
	require.NoError(t, err)
	got := resp.Layout.Groups[0].Services
	require.Equal(t, "v-a", got[0].IconVersion)
	require.Equal(t, "v-b", got[1].IconVersion)
	require.Empty(t, layout.Groups[0].Services[0].IconVersion, "source layout is not mutated")
}
