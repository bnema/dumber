package systemviews

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// homeStore backs a SystemviewHomeService mock with canned data and records
// the calls.
type homeStore struct {
	service *mocks.MockSystemviewHomeService
	mu      sync.Mutex

	load      dto.HomeLoadResponse
	loadCalls int
	sites     map[dto.HomeSitesKind][]dto.HomeSite
	siteReqs  []dto.HomeSitesRequest
	opened    bool

	data      map[string]dto.HomeWidgetDataResponse
	dataErr   error
	dataCalls map[string]int

	actionResp dto.HomeActionResponse
	actionErr  error
	actionReqs []dto.HomeActionRequest
	// gate, when set, holds each HomeAction call until closed.
	gate chan struct{}
}

func newHomeStore(t *testing.T, s *homeStore) *homeStore {
	t.Helper()
	svc := mocks.NewMockSystemviewHomeService(t)
	s.service = svc
	svc.EXPECT().HomeLoad(mock.Anything).RunAndReturn(func(context.Context) (dto.HomeLoadResponse, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.loadCalls++
		return s.load, nil
	}).Maybe()
	svc.EXPECT().HomeSites(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, req dto.HomeSitesRequest) (dto.HomeSitesResponse, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.siteReqs = append(s.siteReqs, req)
		return dto.HomeSitesResponse{Sites: s.sites[req.Kind]}, nil
	}).Maybe()
	svc.EXPECT().HomeOpenConfig(mock.Anything).RunAndReturn(func(context.Context) (dto.HomeOpenConfigResponse, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.opened = true
		return dto.HomeOpenConfigResponse{Path: "/cfg/home.toml"}, nil
	}).Maybe()
	svc.EXPECT().HomeWidgetData(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, id string) (dto.HomeWidgetDataResponse, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.dataCalls == nil {
			s.dataCalls = map[string]int{}
		}
		s.dataCalls[id]++
		if s.dataErr != nil {
			return dto.HomeWidgetDataResponse{}, s.dataErr
		}
		if resp, ok := s.data[id]; ok {
			return resp, nil
		}
		return dto.HomeWidgetDataResponse{ServiceID: id, Status: dto.HomeStatusOK}, nil
	}).Maybe()
	svc.EXPECT().HomeAction(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, req dto.HomeActionRequest) (dto.HomeActionResponse, error) {
		s.mu.Lock()
		s.actionReqs = append(s.actionReqs, req)
		gate, resp, err := s.gate, s.actionResp, s.actionErr
		s.mu.Unlock()
		if gate != nil {
			<-gate
		}
		return resp, err
	}).Maybe()
	return s
}

// set makes HomeWidgetData return resp for service id.
func (s *homeStore) set(id string, resp dto.HomeWidgetDataResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]dto.HomeWidgetDataResponse{}
	}
	s.data[id] = resp
}

func (s *homeStore) callCount(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dataCalls[id]
}

func (s *homeStore) actionRequests() []dto.HomeActionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]dto.HomeActionRequest(nil), s.actionReqs...)
}

var homeTestNow = time.Date(2026, 11, 25, 14, 5, 0, 0, time.UTC)

func newHomeApp(t *testing.T, dom DOM, home *homeStore) *App {
	t.Helper()
	app := NewApp(Dependencies{DOM: dom, Home: home.service, LocationURI: "dumb://home"})
	app.now = func() time.Time { return homeTestNow }
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
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
	home := newHomeStore(t, &homeStore{
		load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()},
		sites: map[dto.HomeSitesKind][]dto.HomeSite{
			dto.HomeSitesTop:    {{URL: "https://top.test/", Title: "Top Site"}},
			dto.HomeSitesRecent: {{URL: "https://recent.test/page"}},
		},
	})
	favorites := newFavoritesStore(t, &favoritesStore{favorites: []*entity.Favorite{
		{URL: "https://fav.test", Title: "Fav"},
	}})
	dom := newTreeDOM()
	mountHomeApp(t, dom, Dependencies{Home: home.service, Favorites: favorites.service})
	html := dom.html()

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
	home := newHomeStore(t, &homeStore{
		load: dto.HomeLoadResponse{
			Layout:     layout,
			ParseError: &dto.HomeParseError{File: "home.toml", Line: 4, Column: 9, Message: "expected value"},
			ValidationErrors: []entity.HomeValidationError{
				{Path: "groups[0].services[1].url", Message: "must be http or https"},
			},
		},
	})
	app := newHomeApp(t, newTreeDOM(), home)

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
	app := newHomeApp(t, newTreeDOM(), newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: layout}}))
	html := app.renderedHTML

	assert.Contains(t, html, `--sv-home-columns: 4;`)
	assert.Contains(t, html, `href="https://jelly.lan"`)
	assert.Contains(t, html, `/api/home-icon?service=jelly`)
	assert.Contains(t, html, `Movies`)
	assert.Contains(t, html, `sv-home-service-widget`)
	assert.Contains(t, html, `data-sv-region="home-svc-status:jelly"`)
	assert.Contains(t, html, `data-sv-region="home-group-count:g1"`)
	assert.Contains(t, html, `data-sv-region="home-status-summary"`)
	assert.Contains(t, html, `2 services`)
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
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout(), SearchEngine: "https://s.test/?q=%s"}})
	dom := &navTreeDOM{treeDOM: newTreeDOM()}
	app := newHomeApp(t, dom, home)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionSearch, Data: map[string]string{"query": "cats"}}))
	assert.Equal(t, []string{"https://s.test/?q=cats"}, dom.navigated)
}

func TestHomeReloadSwapsOnlyChangedRegions(t *testing.T) {
	t.Parallel()
	layout := entity.DefaultHomeLayout()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: layout}})
	dom := newTreeDOM()
	app := newHomeApp(t, dom, home)
	dom.reset()
	mounts := dom.mountCount()

	// Unchanged data: nothing is swapped or remounted.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
	assert.Empty(t, dom.regions())
	assert.Equal(t, mounts, dom.mountCount())

	// Other topics are ignored.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "config"}}))
	assert.Equal(t, 2, home.loadCalls)

	// A new title only swaps the header.
	home.load.Layout.Settings.Title = "Lab"
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
	require.Len(t, dom.regions(), 1)
	require.Len(t, dom.regions()[0], 1)
	assert.Equal(t, homeRegionHeader, dom.regions()[0][0].Name)
	assert.Contains(t, dom.regions()[0][0].HTML, "Lab")

	// A widget added swaps the widgets container only.
	home.load.Layout.Widgets = append(home.load.Layout.Widgets, entity.HomeBuiltinWidget{Type: entity.HomeWidgetClock})
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": "home"}}))
	app.homeFillWG.Wait()
	require.Len(t, dom.regions(), 2)
	assert.Equal(t, homeRegionWidgets, dom.regions()[1][0].Name)
	assert.Equal(t, mounts, dom.mountCount())
}

func TestHomeClockTickSwapsOnlyClock(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}})
	dom := newTreeDOM()
	app := newHomeApp(t, dom, home)
	dom.reset()

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionClockTick}))
	assert.Empty(t, dom.regions(), "same minute, nothing to swap")

	app.now = func() time.Time { return homeTestNow.Add(time.Minute) }
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionClockTick}))
	require.Len(t, dom.regions(), 1)
	require.Len(t, dom.regions()[0], 1)
	assert.Equal(t, homeWidgetRegion(1), dom.regions()[0][0].Name)
	assert.Contains(t, dom.regions()[0][0].HTML, "14:06")
	assert.Equal(t, 1, home.loadCalls)
}

func TestHomeClock12h(t *testing.T) {
	t.Parallel()
	clock, _ := homeClockText(entity.HomeClock12h, homeTestNow)
	assert.Equal(t, "2:05 PM", clock)
}

func TestHomeOpenConfig(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}})
	app := newHomeApp(t, newTreeDOM(), home)
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
	dom := newTreeDOM()
	mountHomeApp(t, dom, Dependencies{Home: newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: layout}}).service, Favorites: favorites.service})
	assert.Contains(t, dom.html(), "Work One")
	assert.NotContains(t, dom.html(), "Work Two")
	assert.NotContains(t, dom.html(), "Home One")
}

func TestHomeSuggestionsUpdateRegion(t *testing.T) {
	t.Parallel()
	history := newHistoryStore(t, &historyStore{searchEntries: []*entity.HistoryEntry{{URL: "https://go.dev", Title: "Go"}}})
	dom := newTreeDOM()
	app := NewApp(Dependencies{DOM: dom, Home: newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}}).service, History: history.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
	fillHome(app)
	dom.reset()

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionSuggest, Data: map[string]string{"query": "go"}}))
	require.Len(t, dom.regions(), 1)
	assert.Equal(t, homeRegionSuggestions, dom.regions()[0][0].Name)
	assert.Contains(t, dom.regions()[0][0].HTML, "data-sv-home-suggestion")
	assert.Equal(t, "go", history.query)
	assert.Equal(t, homeSuggestionLimit, history.searchLimit)
}

func TestHomeMountsBeforeWidgetsLoad(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{
		load:  dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()},
		sites: map[dto.HomeSitesKind][]dto.HomeSite{dto.HomeSitesTop: {{URL: "https://top.test/", Title: "Top Site"}}},
	})
	dom := newTreeDOM()
	app := NewApp(Dependencies{DOM: dom, Home: home.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Contains(t, app.renderedHTML, "sv-home-skeleton")
	assert.NotContains(t, app.renderedHTML, "Top Site")
	assert.Empty(t, home.siteReqs, "sites are not fetched before the page shows")

	require.NoError(t, dom.Mount(app.renderedHTML))

	// The page shows before the widgets load; each widget then swaps its
	// own region.
	fillHome(app)
	assert.Contains(t, dom.html(), "Top Site")
	assert.NotContains(t, dom.html(), "sv-home-skeleton")
	assert.ElementsMatch(t, []string{homeWidgetRegion(2), homeWidgetRegion(3), homeWidgetRegion(4)}, dom.swapped)
}

func TestHomeSuggestionDropsStaleResponse(t *testing.T) {
	t.Parallel()
	history := newHistoryStore(t, &historyStore{searchEntries: []*entity.HistoryEntry{{URL: "https://go.dev", Title: "Go"}}})
	dom := newTreeDOM()
	app := NewApp(Dependencies{DOM: dom, Home: newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}}).service, History: history.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
	fillHome(app)
	dom.reset()

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
	assert.Empty(t, dom.regions())
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

// newMountedHomeApp loads, mounts and fills a home page on dom.
func newMountedHomeApp(t *testing.T, dom *treeDOM, home *homeStore) *App {
	t.Helper()
	return mountHomeApp(t, dom, Dependencies{Home: home.service})
}

// mountHomeApp loads, mounts and fills a home page with deps on dom.
func mountHomeApp(t *testing.T, dom *treeDOM, deps Dependencies) *App {
	t.Helper()
	deps.DOM = dom
	deps.LocationURI = "dumb://home"
	app := NewApp(deps)
	app.now = func() time.Time { return homeTestNow }
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
	fillHome(app)
	return app
}

func homeServiceLayoutWithSearch() entity.HomeLayout {
	layout := homeServiceLayout()
	layout.Settings.Columns = 3
	layout.Widgets = []entity.HomeBuiltinWidget{{Type: entity.HomeWidgetSearch}}
	layout.Groups[0].Services = append(layout.Groups[0].Services, entity.HomeService{
		ID: "nas", Name: "NAS", Actions: []entity.HomeAction{{ID: "wake", Label: "Wake"}},
	})
	layout.Groups = append(layout.Groups, entity.HomeGroup{Name: "Infra", Services: []entity.HomeService{
		{ID: "pve", Name: "Proxmox", Widget: &entity.HomeServiceWidget{Type: entity.HomeServiceWidgetEmbed, URL: "https://pve.lan"}},
	}})
	return layout
}

// homeNodes captures the nodes that must survive a reload.
type homeNodes struct {
	search, camEmbed, pveEmbed interface{}
}

func captureHomeNodes(dom *treeDOM) homeNodes {
	return homeNodes{
		search:   dom.node("data-sv-home-search", ""),
		camEmbed: dom.node("data-sv-region", "home-svc-embed:cam"),
		pveEmbed: dom.node("data-sv-region", "home-svc-embed:pve"),
	}
}

func TestHomeReloadKeepsEmbedsAndSearch(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mutate func(*entity.HomeLayout)
		check  func(t *testing.T, dom *treeDOM)
	}{
		"description": {
			mutate: func(l *entity.HomeLayout) { l.Groups[0].Services[0].Description = "Movies and shows" },
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, []string{"home-svc-tile:jelly"}, dom.swapped)
			},
		},
		"group renamed": {
			mutate: func(l *entity.HomeLayout) { l.Groups[0].Name = "Video" },
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, []string{"home-group-title:g1"}, dom.swapped)
				assert.Equal(t, "Video", dom.text("home-group-title:g1"))
			},
		},
		"columns": {
			mutate: func(l *entity.HomeLayout) { l.Settings.Columns = 5 },
			check: func(t *testing.T, dom *treeDOM) {
				assert.Empty(t, dom.swapped)
				assert.Equal(t, "5", dom.styles["home-groups --sv-home-columns"])
			},
		},
		"action label": {
			mutate: func(l *entity.HomeLayout) { l.Groups[0].Services[2].Actions[0].Label = "Wake up" },
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, []string{homeServiceActionsRegion("nas")}, dom.swapped)
				assert.Contains(t, dom.text(homeServiceActionsRegion("nas")), "Wake up")
			},
		},
		"service added": {
			mutate: func(l *entity.HomeLayout) {
				l.Groups[0].Services = append(l.Groups[0].Services[:1], append([]entity.HomeService{{ID: "new", Name: "New"}}, l.Groups[0].Services[1:]...)...)
			},
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, 1, dom.inserts)
				// Only the service count of the header line swaps.
				assert.Equal(t, []string{homeRegionSummary}, dom.swapped)
				assert.Contains(t, dom.text(homeRegionSummary), "5 services")
				shell := dom.node("data-sv-anchor", "home-svc-shell:new")
				require.NotNil(t, shell)
				assert.Equal(t, dom.node("data-sv-anchor", "home-svc-shell:cam"), shell.NextSibling, "inserted in place")
				assert.Contains(t, dom.text("home-svc-tile:new"), "New")
			},
		},
		"service removed": {
			mutate: func(l *entity.HomeLayout) { l.Groups[0].Services = l.Groups[0].Services[1:] },
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, 1, dom.removes)
				assert.Nil(t, dom.node("data-sv-anchor", "home-svc-shell:jelly"))
				assert.Equal(t, []string{homeRegionSummary}, dom.swapped)
			},
		},
		"group added": {
			mutate: func(l *entity.HomeLayout) {
				l.Groups = append(l.Groups[:1], append([]entity.HomeGroup{{Name: "Net", Services: []entity.HomeService{{ID: "router", Name: "Router"}}}}, l.Groups[1:]...)...)
			},
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, 1, dom.inserts)
				group := dom.node("data-sv-anchor", "home-group:g3")
				require.NotNil(t, group)
				assert.Equal(t, dom.node("data-sv-anchor", "home-group:g2"), group.NextSibling)
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
			dom := newTreeDOM()
			app := newMountedHomeApp(t, dom, home)
			before := captureHomeNodes(dom)
			require.NotNil(t, before.search)
			require.NotNil(t, before.camEmbed)
			dom.swapped = nil

			tc.mutate(&home.load.Layout)
			reloadHome(t, app)

			assert.Equal(t, 1, dom.mounts, "no remount")
			after := captureHomeNodes(dom)
			assert.Same(t, before.search, after.search, "search input kept")
			assert.Same(t, before.camEmbed, after.camEmbed, "embed node kept")
			assert.Same(t, before.pveEmbed, after.pveEmbed, "other group's embed kept")
			tc.check(t, dom)
		})
	}
}

func TestHomeServiceGroupRemoved(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	cam := dom.node("data-sv-region", "home-svc-embed:cam")

	home.load.Layout.Groups = home.load.Layout.Groups[:1]
	reloadHome(t, app)
	assert.Equal(t, 1, dom.mounts)
	assert.Nil(t, dom.node("data-sv-anchor", "home-group:g2"))
	assert.Same(t, cam, dom.node("data-sv-region", "home-svc-embed:cam"))
}

func TestHomeServiceReorderRemounts(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*entity.HomeLayout){
		"services reordered": func(l *entity.HomeLayout) {
			s := l.Groups[0].Services
			s[0], s[1] = s[1], s[0]
		},
		"groups reordered": func(l *entity.HomeLayout) {
			l.Groups[0], l.Groups[1] = l.Groups[1], l.Groups[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
			dom := newTreeDOM()
			app := newMountedHomeApp(t, dom, home)

			mutate(&home.load.Layout)
			reloadHome(t, app)
			assert.Equal(t, 2, dom.mounts)
		})
	}
}

func TestHomeServiceIconURLVersion(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/api/home-icon?service=jelly&v=0123456789ab",
		homeServiceIconURL(entity.HomeService{ID: "jelly", IconVersion: "0123456789ab"}))
	assert.Equal(t, "/api/home-icon?service=jelly", homeServiceIconURL(entity.HomeService{ID: "jelly"}))
}

func TestSystemviewChangedIgnoredOutsideHome(t *testing.T) {
	t.Parallel()
	newApps := map[string]func(t *testing.T, dom *treeDOM) *App{
		"history": func(t *testing.T, dom *treeDOM) *App {
			return loadedHistoryApp(t, dom, newHistoryStore(t, &historyStore{}))
		},
		"favorites": func(t *testing.T, dom *treeDOM) *App {
			favorites := newFavoritesStore(t, &favoritesStore{})
			app := NewApp(Dependencies{DOM: dom, Favorites: favorites.service, LocationURI: "dumb://favorites"})
			require.NoError(t, app.LoadInitial(context.Background()))
			return app
		},
		"config": func(t *testing.T, dom *treeDOM) *App {
			app, _, _ := newConfigApp(t, dom)
			return app
		},
	}
	for name, newApp := range newApps {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dom := newTreeDOM()
			app := newApp(t, dom)
			before := app.renderedHTML
			mounts := dom.mountCount()
			for _, topic := range []string{"home", "config"} {
				require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: actionSystemviewChanged, Data: map[string]string{"topic": topic}}))
			}
			assert.Equal(t, mounts, dom.mountCount(), "no remount")
			assert.Empty(t, dom.regions())
			assert.Equal(t, before, app.renderedHTML, "no error banner")
		})
	}
}

func TestHomeActionLabelChangeKeepsOpenForm(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	app.lockState()
	app.homeSvc.actions = map[string]homeActionState{"nas": {ActionID: "wake", Step: homeActionForm}}
	app.unlockState()
	dom.swapped = nil

	home.load.Layout.Groups[0].Services[2].Actions[0].Label = "Wake up"
	reloadHome(t, app)
	assert.Empty(t, dom.swapped, "the open form keeps its inputs")
	assert.Equal(t, 1, dom.mounts)
}

func TestHomeActionErrorShowsToastAndKeepsPage(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
	dom := newTreeDOM()
	dom.toastOK = true
	app := newMountedHomeApp(t, dom, home)
	embed := dom.node("data-sv-region", "home-svc-embed:cam")

	app.surfaceActionError(context.Background(), errors.New("open home.toml: no opener"))
	assert.Equal(t, []string{"open home.toml: no opener"}, dom.toasts)
	assert.Equal(t, 1, dom.mounts, "the page is kept")
	assert.Same(t, embed, dom.node("data-sv-region", "home-svc-embed:cam"))
}

func TestHomeErrorPageRecoversOnReload(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)

	// Without a toast UI the error page replaces the home page.
	app.surfaceActionError(context.Background(), errors.New("boom"))
	require.Equal(t, 2, dom.mounts)
	assert.Nil(t, dom.node("data-sv-region", "home-svc-embed:cam"))
	// A clock tick finds nothing to swap.
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionClockTick}))

	reloadHome(t, app)
	assert.Equal(t, 3, dom.mounts, "the home.toml change remounts the page")
	assert.NotNil(t, dom.node("data-sv-region", "home-svc-embed:cam"))
}

func TestHomeFailedEditRemounts(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	dom.insertErr = errors.New("insert failed")

	home.load.Layout.Groups[0].Services = append(home.load.Layout.Groups[0].Services, entity.HomeService{ID: "new", Name: "New"})
	reloadHome(t, app)
	assert.Equal(t, 2, dom.mounts, "a failed edit falls back to a remount")
	assert.NotNil(t, dom.node("data-sv-anchor", "home-svc-shell:new"))
}

func TestHomeFreshShellResetsActionsButKeepsBusy(t *testing.T) {
	t.Parallel()
	layout := homeServiceLayoutWithSearch()
	layout.Groups[1].Services = append(layout.Groups[1].Services, entity.HomeService{
		ID: "nas2", Name: "NAS 2", Actions: []entity.HomeAction{{ID: "wake", Label: "Wake"}},
	})
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: layout}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	app.lockState()
	app.homeSvc.actions = map[string]homeActionState{
		"nas":  {ActionID: "wake", Step: homeActionBusy},
		"nas2": {ActionID: "wake", Step: homeActionForm},
	}
	app.unlockState()

	// Moving nas to the next group and nas2 to a new group gives both a
	// new shell.
	groups := home.load.Layout.Groups
	nas, nas2 := groups[0].Services[2], groups[1].Services[1]
	groups[0].Services = groups[0].Services[:2:2]
	groups[1].Services = []entity.HomeService{nas, groups[1].Services[0]}
	groups = append(groups, entity.HomeGroup{Name: "Spare", Services: []entity.HomeService{nas2}})
	home.load.Layout.Groups = groups
	reloadHome(t, app)

	assert.Equal(t, 1, dom.mounts)
	app.lockState()
	_, formKept := app.homeSvc.actions["nas2"]
	busy := app.homeSvc.actions["nas"]
	app.unlockState()
	assert.False(t, formKept, "an open form is dropped with its shell")
	assert.Equal(t, homeActionBusy, busy.Step, "a running action is kept")
	assert.Contains(t, dom.text(homeServiceActionsRegion("nas")), "Running")
}

func TestHomeSlotAndWidgetListChangesKeepEmbeds(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		mutate func(*entity.HomeLayout)
		check  func(t *testing.T, dom *treeDOM)
	}{
		"action added to an embed service": {
			mutate: func(l *entity.HomeLayout) {
				l.Groups[0].Services[1].Actions = []entity.HomeAction{{ID: "snap", Label: "Snapshot"}}
			},
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, []string{homeServiceActionsRegion("cam")}, dom.swapped)
				assert.Contains(t, dom.text(homeServiceActionsRegion("cam")), "Snapshot")
			},
		},
		"api widget added to a service": {
			mutate: func(l *entity.HomeLayout) {
				l.Groups[0].Services[0].Widget = &entity.HomeServiceWidget{Type: entity.HomeServiceWidgetAPI, URL: "https://jelly.lan/api"}
			},
			check: func(t *testing.T, dom *treeDOM) {
				assert.Contains(t, dom.swapped, homeServiceRegion("jelly"))
			},
		},
		"clock widget added": {
			mutate: func(l *entity.HomeLayout) {
				l.Widgets = append(l.Widgets, entity.HomeBuiltinWidget{Type: entity.HomeWidgetClock})
			},
			check: func(t *testing.T, dom *treeDOM) {
				assert.Equal(t, []string{homeRegionWidgets}, dom.swapped)
				assert.NotNil(t, dom.node("data-sv-region", homeWidgetRegion(1)))
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
			dom := newTreeDOM()
			app := newMountedHomeApp(t, dom, home)
			cam := dom.node("data-sv-region", "home-svc-embed:cam")
			pve := dom.node("data-sv-region", "home-svc-embed:pve")
			dom.swapped = nil

			tc.mutate(&home.load.Layout)
			reloadHome(t, app)

			assert.Equal(t, 1, dom.mounts, "no remount")
			assert.Same(t, cam, dom.node("data-sv-region", "home-svc-embed:cam"), "embed node kept")
			assert.Same(t, pve, dom.node("data-sv-region", "home-svc-embed:pve"))
			tc.check(t, dom)
		})
	}
}

func TestHomeRemountRestoresSlotsFormsAndEmbeds(t *testing.T) {
	t.Parallel()
	box := actionService(entity.HomeAction{ID: "scale", Label: "Scale", Inputs: []entity.HomeActionInput{{Name: "n"}}})
	nas := apiService("nas", entity.HomeDisplayStats, "")
	cam := embedService("https://cam.lan/live", 300)
	home := newHomeStore(t, &homeStore{load: homeLayoutWith(box, nas, cam)})
	home.set("nas", dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "Disk", Value: "80%"}}})
	app, dom := newActionApp(t, home)
	require.NoError(t, app.HandleDOMAction(context.Background(), svcEvent(homeActionSvcOpen, "scale", nil)))
	mounts := dom.mountCount()

	checks := map[string]func(t *testing.T){
		"form":  func(t *testing.T) { assert.Contains(t, dom.regionHTML(homeServiceActionsRegion("box")), `name="input.n"`) },
		"slot":  func(t *testing.T) { assert.Contains(t, dom.regionHTML(homeServiceRegion("nas")), `80%`) },
		"embed": func(t *testing.T) { assert.Contains(t, dom.regionHTML(homeServiceEmbedRegion("cam")), `src="https://cam.lan/live"`) },
	}
	for name, check := range checks {
		t.Run("before/"+name, check)
	}

	// A reorder is the structural change that still remounts the page.
	home.load = homeLayoutWith(cam, nas, box)
	reloadHome(t, app)
	require.Equal(t, mounts+1, dom.mountCount())

	for name, check := range checks {
		t.Run("after/"+name, check)
	}
}
