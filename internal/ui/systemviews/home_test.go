package systemviews

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeHome struct {
	mu        sync.Mutex
	load      dto.HomeLoadResponse
	sites     map[dto.HomeSitesKind][]dto.HomeSite
	siteReqs  []dto.HomeSitesRequest
	opened    bool
	loadCalls int
}

func (f *fakeHome) HomeLoad(context.Context) (dto.HomeLoadResponse, error) {
	f.loadCalls++
	return f.load, nil
}

func (f *fakeHome) HomeSites(_ context.Context, req dto.HomeSitesRequest) (dto.HomeSitesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.siteReqs = append(f.siteReqs, req)
	return dto.HomeSitesResponse{Sites: f.sites[req.Kind]}, nil
}

func (f *fakeHome) HomeOpenConfig(context.Context) (dto.HomeOpenConfigResponse, error) {
	f.opened = true
	return dto.HomeOpenConfigResponse{Path: "/cfg/home.toml"}, nil
}

func (f *fakeHome) HomeWidgetData(_ context.Context, serviceID string) (dto.HomeWidgetDataResponse, error) {
	return dto.HomeWidgetDataResponse{ServiceID: serviceID, Status: dto.HomeStatusOK}, nil
}

func (f *fakeHome) HomeAction(context.Context, dto.HomeActionRequest) (dto.HomeActionResponse, error) {
	return dto.HomeActionResponse{Status: dto.HomeStatusOK}, nil
}

type navDOM struct {
	regionDOM
	navigated []string
}

func (d *navDOM) NavigateTo(rawURL string) error {
	d.navigated = append(d.navigated, rawURL)
	return nil
}

var homeTestNow = time.Date(2026, 11, 25, 14, 5, 0, 0, time.UTC)

func newHomeApp(t *testing.T, dom DOM, home *fakeHome, favorites *favoritesStore) *App {
	t.Helper()
	deps := Dependencies{DOM: dom, Home: home, LocationURI: "dumb://home"}
	if favorites != nil {
		deps.Favorites = favorites.service
	}
	app := NewApp(deps)
	app.now = func() time.Time { return homeTestNow }
	require.NoError(t, app.LoadInitial(context.Background()))
	fillHome(app)
	return app
}

// fillHome runs the background widget fill and waits for it.
func fillHome(app *App) {
	app.lockState()
	app.startHomeFillLocked(context.Background())
	app.unlockState()
	app.homeFillWG.Wait()
}

func TestHomeRendersDefaultLayout(t *testing.T) {
	t.Parallel()
	home := &fakeHome{
		load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()},
		sites: map[dto.HomeSitesKind][]dto.HomeSite{
			dto.HomeSitesTop:    {{URL: "https://top.test/", Title: "Top Site"}},
			dto.HomeSitesRecent: {{URL: "https://recent.test/page"}},
		},
	}
	favorites := newFavoritesStore(t, &favoritesStore{favorites: []*entity.Favorite{
		{URL: "https://fav.test", Title: "Fav"},
	}})
	app := newHomeApp(t, &regionDOM{}, home, favorites)
	html := app.renderedHTML

	assert.Contains(t, html, `data-route="home"`)
	assert.Contains(t, html, `data-page-title="Home"`)
	for i := range entity.DefaultHomeLayout().Widgets {
		assert.Contains(t, html, `data-sv-region="`+homeWidgetRegion(i)+`"`)
	}
	assert.Contains(t, html, `data-sv-home-search`)
	assert.Contains(t, html, `14:05`)
	assert.Contains(t, html, `Wednesday, November 25`)
	assert.Contains(t, html, `Fav`)
	assert.Contains(t, html, `Top Site`)
	assert.Contains(t, html, `recent.test/page`)
	assert.Contains(t, html, `/api/favicon?domain=top.test&amp;size=32`)
	assert.Contains(t, html, `data-sv-action="home.openConfig"`)
	assert.NotContains(t, html, `sv-home-errors"`)
	assert.ElementsMatch(t, []dto.HomeSitesRequest{
		{Kind: dto.HomeSitesTop, Limit: entity.HomeDefaultLimit},
		{Kind: dto.HomeSitesRecent, Limit: entity.HomeDefaultLimit},
	}, home.siteReqs)
}

func TestHomeRendersErrorBanner(t *testing.T) {
	t.Parallel()
	layout := entity.DefaultHomeLayout()
	layout.Widgets = nil
	home := &fakeHome{load: dto.HomeLoadResponse{
		Layout:     layout,
		ParseError: &dto.HomeParseError{File: "home.toml", Line: 4, Column: 9, Message: "expected value"},
		ValidationErrors: []entity.HomeValidationError{
			{Path: "groups[0].services[1].url", Message: "must be http or https"},
		},
	}}
	app := newHomeApp(t, &regionDOM{}, home, nil)

	assert.Contains(t, app.renderedHTML, `data-sv-region="home-errors"`)
	assert.Contains(t, app.renderedHTML, `home.toml:4:9`)
	assert.Contains(t, app.renderedHTML, `expected value`)
	assert.Contains(t, app.renderedHTML, `groups[0].services[1].url`)
	assert.Contains(t, app.renderedHTML, `must be http or https`)
}

func TestHomeRendersServiceGroups(t *testing.T) {
	t.Parallel()
	layout := entity.HomeLayout{
		Settings: entity.HomeSettings{Title: "Lab", Columns: 4},
		Groups: []entity.HomeGroup{{Name: "Media", Services: []entity.HomeService{
			{ID: "jelly", Name: "Jellyfin", URL: "https://jelly.lan", Description: "Movies"},
			{ID: "nas", Name: "NAS"},
		}}},
	}
	app := newHomeApp(t, &regionDOM{}, &fakeHome{load: dto.HomeLoadResponse{Layout: layout}}, nil)
	html := app.renderedHTML

	assert.Contains(t, html, `--sv-home-columns: 4;`)
	assert.Contains(t, html, `href="https://jelly.lan"`)
	assert.Contains(t, html, `/api/home-icon?service=jelly`)
	assert.Contains(t, html, `Movies`)
	assert.Contains(t, html, `sv-home-service-widget`)
	assert.Contains(t, html, `data-page-title="Lab"`)
}

func TestHomeSearchURL(t *testing.T) {
	t.Parallel()
	engine := "https://search.test/?q=%s"
	for query, want := range map[string]string{
		"":                    "",
		"go generics":         "https://search.test/?q=go+generics",
		"a&b=c":               "https://search.test/?q=a%26b%3Dc",
		"example.com":         "https://example.com",
		"https://x.test/path": "https://x.test/path",
	} {
		assert.Equal(t, want, homeSearchURL(engine, query), query)
	}
	assert.Equal(t, "https://duckduckgo.com/?q=hi", homeSearchURL("", "hi"))
}

func TestHomeSearchURLNeverNavigatesToScriptSchemes(t *testing.T) {
	t.Parallel()
	engine := "https://search.test/?q=%s"
	for _, query := range []string{
		"javascript://x.com/%0aalert(1)",
		"javascript:alert(1)",
		"JAVASCRIPT://x.com/%0aalert(document.domain)",
		"vbscript:x",
		"data:text/html,<script>alert(1)</script>",
		"blob:x",
	} {
		got := homeSearchURL(engine, query)
		assert.True(t, strings.HasPrefix(got, "https://search.test/?q="), "%q -> %q", query, got)
	}
	assert.Empty(t, homeSearchURL("javascript:%s", "hi"), "unsafe search engine template")
	assert.Equal(t, "dumb://history", homeSearchURL(engine, "dumb://history"))
	assert.Equal(t, "file:///tmp/x.html", homeSearchURL(engine, "file:///tmp/x.html"))
}

func TestHomeSearchActionNavigates(t *testing.T) {
	t.Parallel()
	home := &fakeHome{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout(), SearchEngine: "https://s.test/?q=%s"}}
	dom := &navDOM{}
	app := newHomeApp(t, dom, home, nil)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionSearch, Data: map[string]string{"query": "cats"}}))
	assert.Equal(t, []string{"https://s.test/?q=cats"}, dom.navigated)
}

func TestHomeReloadSwapsOnlyChangedRegions(t *testing.T) {
	t.Parallel()
	layout := entity.DefaultHomeLayout()
	home := &fakeHome{load: dto.HomeLoadResponse{Layout: layout}}
	dom := &regionDOM{}
	app := newHomeApp(t, dom, home, nil)
	dom.regions = nil
	mounts := dom.mountCount

	// Unchanged data: nothing is swapped or remounted.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
	assert.Empty(t, dom.regions)
	assert.Equal(t, mounts, dom.mountCount)

	// Other topics are ignored.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "config"}}))
	assert.Equal(t, 2, home.loadCalls)

	// A new title only swaps the header.
	home.load.Layout.Settings.Title = "Lab"
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
	require.Len(t, dom.regions, 1)
	require.Len(t, dom.regions[0], 1)
	assert.Equal(t, homeRegionHeader, dom.regions[0][0].Name)
	assert.Contains(t, dom.regions[0][0].HTML, "Lab")

	// A widget added changes the region list: full remount.
	home.load.Layout.Widgets = append(home.load.Layout.Widgets, entity.HomeBuiltinWidget{Type: entity.HomeWidgetClock})
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
	assert.Len(t, dom.regions, 1)
	assert.Equal(t, mounts+1, dom.mountCount)
}

func TestHomeClockTickSwapsOnlyClock(t *testing.T) {
	t.Parallel()
	home := &fakeHome{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}}
	dom := &regionDOM{}
	app := newHomeApp(t, dom, home, nil)
	dom.regions = nil

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionClockTick}))
	assert.Empty(t, dom.regions, "same minute, nothing to swap")

	app.now = func() time.Time { return homeTestNow.Add(time.Minute) }
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionClockTick}))
	require.Len(t, dom.regions, 1)
	require.Len(t, dom.regions[0], 1)
	assert.Equal(t, homeWidgetRegion(1), dom.regions[0][0].Name)
	assert.Contains(t, dom.regions[0][0].HTML, "14:06")
	assert.Equal(t, 1, home.loadCalls)
}

func TestHomeClock12h(t *testing.T) {
	t.Parallel()
	clock, _ := homeClockText(entity.HomeClock12h, homeTestNow)
	assert.Equal(t, "2:05 PM", clock)
}

func TestHomeOpenConfig(t *testing.T) {
	t.Parallel()
	home := &fakeHome{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}}
	app := newHomeApp(t, &regionDOM{}, home, nil)
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionOpenConfig}))
	assert.True(t, home.opened)
}

func TestHomeFavoritesWidgetFiltersByTagAndLimit(t *testing.T) {
	t.Parallel()
	layout := entity.HomeLayout{Widgets: []entity.HomeBuiltinWidget{{Type: entity.HomeWidgetFavorites, Tag: "work", Limit: 1}}}
	favorites := newFavoritesStore(t, &favoritesStore{favorites: []*entity.Favorite{
		{URL: "https://home.test", Title: "Home One"},
		{URL: "https://w1.test", Title: "Work One", Tags: []entity.Tag{{Name: "Work"}}},
		{URL: "https://w2.test", Title: "Work Two", Tags: []entity.Tag{{Name: "work"}}},
	}})
	app := newHomeApp(t, &regionDOM{}, &fakeHome{load: dto.HomeLoadResponse{Layout: layout}}, favorites)
	assert.Contains(t, app.renderedHTML, "Work One")
	assert.NotContains(t, app.renderedHTML, "Work Two")
	assert.NotContains(t, app.renderedHTML, "Home One")
}

func TestHomeSuggestionsUpdateRegion(t *testing.T) {
	t.Parallel()
	history := newHistoryStore(t, &historyStore{searchEntries: []*entity.HistoryEntry{{URL: "https://go.dev", Title: "Go"}}})
	dom := &regionDOM{}
	app := NewApp(Dependencies{DOM: dom, Home: &fakeHome{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}}, History: history.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	fillHome(app)
	dom.regions = nil

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionSuggest, Data: map[string]string{"query": "go"}}))
	require.Len(t, dom.regions, 1)
	assert.Equal(t, homeRegionSuggestions, dom.regions[0][0].Name)
	assert.Contains(t, dom.regions[0][0].HTML, "data-sv-home-suggestion")
	assert.Equal(t, "go", history.query)
	assert.Equal(t, homeSuggestionLimit, history.searchLimit)
}

func TestHomeMountsBeforeWidgetsLoad(t *testing.T) {
	t.Parallel()
	home := &fakeHome{
		load:  dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()},
		sites: map[dto.HomeSitesKind][]dto.HomeSite{dto.HomeSitesTop: {{URL: "https://top.test/", Title: "Top Site"}}},
	}
	dom := &regionDOM{}
	app := NewApp(Dependencies{DOM: dom, Home: home, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, "sv-home-skeleton")
	assert.NotContains(t, app.renderedHTML, "Top Site")
	assert.Empty(t, home.siteReqs, "sites are not fetched before the page shows")

	fillHome(app)
	assert.Contains(t, app.renderedHTML, "Top Site")
	assert.NotContains(t, app.renderedHTML, "sv-home-skeleton")
	// Each widget swaps its own region.
	for _, update := range dom.regions {
		require.Len(t, update, 1)
	}
	assert.Len(t, dom.regions, 3)
}

func TestHomeSuggestionDropsStaleResponse(t *testing.T) {
	t.Parallel()
	history := newHistoryStore(t, &historyStore{searchEntries: []*entity.HistoryEntry{{URL: "https://go.dev", Title: "Go"}}})
	dom := &regionDOM{}
	app := NewApp(Dependencies{DOM: dom, Home: &fakeHome{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}}, History: history.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	fillHome(app)
	dom.regions = nil

	// A newer query is taken while this one is in flight: bump the sequence
	// the way a second handleHomeSuggest would.
	history.service.ExpectedCalls = nil
	history.service.EXPECT().Search(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, string, int) ([]*entity.HistoryEntry, error) {
			app.lockState()
			app.homeSuggestSeq++
			app.unlockState()
			return history.searchEntries, nil
		})
	require.NoError(t, app.handleHomeSuggest(context.Background(), "go"))
	assert.Empty(t, dom.regions)
}

func homeServiceLayout() entity.HomeLayout {
	return entity.HomeLayout{Groups: []entity.HomeGroup{
		{Name: "Media", Services: []entity.HomeService{
			{ID: "jelly", Name: "Jellyfin", URL: "https://jelly.lan", Description: "Movies"},
			{ID: "cam", Name: "Camera", Widget: &entity.HomeServiceWidget{Type: entity.HomeServiceWidgetEmbed, URL: "https://cam.lan"}},
		}},
	}}
}

func reloadHome(t *testing.T, app *App) {
	t.Helper()
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
}

func TestHomeServiceTilesAreIndependentRegions(t *testing.T) {
	t.Parallel()
	home := &fakeHome{load: dto.HomeLoadResponse{Layout: homeServiceLayout()}}
	dom := &regionDOM{}
	app := newHomeApp(t, dom, home, nil)
	html := app.renderedHTML
	assert.Contains(t, html, `data-sv-region="home-svc-tile:jelly"`)
	assert.Contains(t, html, `data-sv-region="home-svc-embed:cam"`)
	assert.Contains(t, html, "Movies", "tiles are filled in the full page")
	dom.regions = nil
	mounts := dom.mountCount

	// A description change swaps only that tile.
	home.load.Layout.Groups[0].Services[0].Description = "Movies and shows"
	reloadHome(t, app)
	require.Len(t, dom.regions, 1)
	require.Len(t, dom.regions[0], 1)
	assert.Equal(t, "home-svc-tile:jelly", dom.regions[0][0].Name)
	assert.Contains(t, dom.regions[0][0].HTML, "Movies and shows")
	assert.Equal(t, mounts, dom.mountCount, "no remount")
	for _, update := range dom.regions {
		for _, region := range update {
			assert.NotEqual(t, "home-svc-embed:cam", region.Name, "the embed region is never swapped")
		}
	}
}

func TestHomeServiceStructureChangeRemounts(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*entity.HomeLayout){
		"service added": func(l *entity.HomeLayout) {
			l.Groups[0].Services = append(l.Groups[0].Services, entity.HomeService{ID: "nas", Name: "NAS"})
		},
		"service removed": func(l *entity.HomeLayout) { l.Groups[0].Services = l.Groups[0].Services[:1] },
		"services reordered": func(l *entity.HomeLayout) {
			s := l.Groups[0].Services
			s[0], s[1] = s[1], s[0]
		},
		"group renamed": func(l *entity.HomeLayout) { l.Groups[0].Name = "Video" },
		"group added":   func(l *entity.HomeLayout) { l.Groups = append(l.Groups, entity.HomeGroup{Name: "Infra"}) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := &fakeHome{load: dto.HomeLoadResponse{Layout: homeServiceLayout()}}
			dom := &regionDOM{}
			app := newHomeApp(t, dom, home, nil)
			dom.regions = nil
			mounts := dom.mountCount

			mutate(&home.load.Layout)
			reloadHome(t, app)
			assert.Equal(t, mounts+1, dom.mountCount)
			assert.Empty(t, dom.regions)
		})
	}
}
