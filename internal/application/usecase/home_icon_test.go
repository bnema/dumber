package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	appport "bnema.dev/dumber/internal/application/port"
	portmocks "bnema.dev/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/mock"
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
	fx.src.EXPECT().FetchIcon(mock.Anything, "http://jelly.local:8096/favicon.ico").Return(img("image/x-icon", "ico"), nil)
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
	fx.src.EXPECT().FetchIcon(mock.Anything, "u/a.svg").Return(img("image/svg+xml", "bad"), nil)
	fx.conv.EXPECT().Convert(mock.Anything, []byte("bad"), "image/svg+xml", []int{128}).Return(nil, ErrFaviconMiss)
	fx.src.EXPECT().FetchIcon(mock.Anything, "u/b.png").Return(nil, errors.New("404"))
	fx.src.EXPECT().FetchIcon(mock.Anything, "u/c.png").Return(img("image/png", "c"), nil)
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
	fx.src.EXPECT().FetchIcon(mock.Anything, "u/favicon.ico").Return(nil, errors.New("404"))
	_, _, err := fx.uc.ResolveHomeServiceIcon(context.Background(), svc, "")
	if !errors.Is(err, ErrHomeIconNotFound) || !errors.Is(err, appport.ErrHomeIconNotFound) {
		t.Fatalf("want sentinel, got %v", err)
	}
}
