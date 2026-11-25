package systemviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// HomeService is the data source of dumb://home.
type HomeService interface {
	HomeLoad(ctx context.Context) (dto.HomeLoadResponse, error)
	HomeSites(ctx context.Context, req dto.HomeSitesRequest) (dto.HomeSitesResponse, error)
	HomeOpenConfig(ctx context.Context) (dto.HomeOpenConfigResponse, error)
	HomeWidgetData(ctx context.Context, serviceID string) (dto.HomeWidgetDataResponse, error)
	HomeAction(ctx context.Context, req dto.HomeActionRequest) (dto.HomeActionResponse, error)
}

// DOMNavigator is implemented by DOM adapters that can open a URL in the
// current tab.
type DOMNavigator interface {
	NavigateTo(rawURL string) error
}

// Home actions.
const (
	homeActionOpenConfig = "home.openConfig"
	homeActionSearch     = "home.search"
	homeActionSuggest    = "home.suggest"
	homeActionClockTick  = "home.clockTick"
	// actionSystemviewChanged is dispatched by the DOM for the window event
	// dto.HomeChangedEvent, with Data["topic"] from its detail.
	actionSystemviewChanged = "systemview.changed"
)

// homeClockInterval is how often the clock widget is refreshed.
const homeClockInterval = 30 * time.Second

func (a *App) homeNow() time.Time {
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

// loadHomeRoute loads the layout and renders the page with the list widgets
// in their loading state; fillHomeWidgets loads them afterwards. Caller holds
// App.mu.
func (a *App) loadHomeRoute(ctx context.Context) error {
	data, err := a.buildHomeData(ctx, nil)
	if err != nil {
		return err
	}
	a.resetRouteState()
	a.homeGeneration++
	a.homeData = data
	a.homeRendered = homeRegions(data)
	a.renderHomePageLocked()
	return nil
}

func (a *App) renderHomePageLocked() {
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteHome,
		title:    homeTitle(a.homeData.Load),
		subtitle: "",
		body:     mustRenderComponent(HomePage(a.homeRendered, a.homeData.Load)),
	}, a.shellTheme)
}

// buildHomeData loads the layout. List widgets keep the rows of the same
// widget in previous (so a reload does not flash), else start loading.
func (a *App) buildHomeData(ctx context.Context, previous []homeWidgetData) (homeRenderData, error) {
	load := dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}
	if a.deps.Home != nil {
		var err error
		if load, err = a.deps.Home.HomeLoad(ctx); err != nil {
			return homeRenderData{}, err
		}
	}
	data := homeRenderData{Load: load, Widgets: make([]homeWidgetData, 0, len(load.Layout.Widgets))}
	for i, widget := range load.Layout.Widgets {
		item := homeWidgetData{Widget: widget}
		switch {
		case widget.Type == entity.HomeWidgetClock:
			item.Time, item.Date = homeClockText(widget.Format, a.homeNow())
		case !homeListWidget(widget.Type):
		case i < len(previous) && previous[i].Widget == widget && !previous[i].Loading:
			item = previous[i]
		default:
			item.Loading = true
		}
		data.Widgets = append(data.Widgets, item)
	}
	return data, nil
}

func homeListWidget(kind entity.HomeBuiltinWidgetType) bool {
	switch kind {
	case entity.HomeWidgetFavorites, entity.HomeWidgetTopSites, entity.HomeWidgetRecent:
		return true
	default:
		return false
	}
}

// fetchHomeWidget loads the rows of a list widget.
func (a *App) fetchHomeWidget(ctx context.Context, widget entity.HomeBuiltinWidget) homeWidgetData {
	data := homeWidgetData{Widget: widget}
	var err error
	switch widget.Type {
	case entity.HomeWidgetFavorites:
		data.Rows, err = a.homeFavoriteRows(ctx, widget)
	case entity.HomeWidgetTopSites:
		data.Rows, err = a.homeSiteRows(ctx, dto.HomeSitesTop, widget)
	case entity.HomeWidgetRecent:
		data.Rows, err = a.homeSiteRows(ctx, dto.HomeSitesRecent, widget)
	}
	if err != nil {
		data.Error = err.Error()
	}
	return data
}

func (a *App) homeFavoriteRows(ctx context.Context, widget entity.HomeBuiltinWidget) ([]homeRow, error) {
	if a.deps.Favorites == nil {
		return nil, errors.New("favorites are not available")
	}
	favorites, err := a.deps.Favorites.List(ctx)
	if err != nil {
		return nil, err
	}
	tag := strings.TrimSpace(widget.Tag)
	limit := homeWidgetLimit(widget)
	rows := make([]homeRow, 0, limit)
	for _, fav := range favorites {
		if len(rows) == limit {
			break
		}
		if fav == nil || (tag != "" && !favoriteHasTag(fav, tag)) {
			continue
		}
		rows = append(rows, homeRow{URL: fav.URL, Title: fav.Title})
	}
	return rows, nil
}

func favoriteHasTag(fav *entity.Favorite, name string) bool {
	for _, tag := range fav.Tags {
		if strings.EqualFold(tag.Name, name) {
			return true
		}
	}
	return false
}

func (a *App) homeSiteRows(ctx context.Context, kind dto.HomeSitesKind, widget entity.HomeBuiltinWidget) ([]homeRow, error) {
	if a.deps.Home == nil {
		return nil, errors.New("history is not available")
	}
	resp, err := a.deps.Home.HomeSites(ctx, dto.HomeSitesRequest{Kind: kind, Limit: homeWidgetLimit(widget)})
	if err != nil {
		return nil, err
	}
	rows := make([]homeRow, 0, len(resp.Sites))
	for _, site := range resp.Sites {
		rows = append(rows, homeRow{URL: site.URL, Title: site.Title})
	}
	return rows, nil
}

// applyHomeActionLocked runs a home action. Caller holds App.mu.
func (a *App) applyHomeActionLocked(ctx context.Context, event DOMAction) error {
	switch event.Action {
	case homeActionOpenConfig:
		return a.openHomeConfigLocked(ctx)
	case homeActionSearch:
		return a.navigateHomeSearchLocked(event.Data["query"])
	case homeActionClockTick:
		return a.refreshHomeClockLocked()
	case actionSystemviewChanged:
		if event.Data["topic"] != dto.HomeChangedTopic {
			return nil
		}
		return a.reloadHomeLocked(ctx)
	default:
		return nil
	}
}

func (a *App) openHomeConfigLocked(ctx context.Context) error {
	if a.deps.Home == nil {
		return errors.New("home service not configured")
	}
	resp, err := a.deps.Home.HomeOpenConfig(ctx)
	if err != nil {
		return fmt.Errorf("open home.toml: %w", err)
	}
	if toaster, ok := a.deps.DOM.(DOMToaster); ok {
		message := "Opened " + resp.Path
		if resp.Created {
			message = "Created " + resp.Path
		}
		toaster.ShowToast(ToastRequest{Message: message, Timeout: 4 * time.Second})
	}
	return nil
}

func (a *App) navigateHomeSearchLocked(query string) error {
	target := homeSearchURL(a.homeData.Load.SearchEngine, query)
	if target == "" {
		return nil
	}
	navigator, ok := a.deps.DOM.(DOMNavigator)
	if !ok {
		return errors.New("navigation is not available")
	}
	return navigator.NavigateTo(target)
}

// handleHomeSuggest shows history suggestions for query. Typing is
// coalesced in the action queue; a response is dropped when a newer query
// was taken meanwhile.
func (a *App) handleHomeSuggest(ctx context.Context, query string) error {
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return nil
	}
	query = strings.TrimSpace(query)
	a.lockState()
	if a.closed || a.currentRoute != RouteHome {
		a.unlockState()
		return nil
	}
	a.homeSuggestSeq++
	seq := a.homeSuggestSeq
	a.unlockState()

	var rows []homeRow
	if query != "" && a.deps.History != nil {
		entries, err := a.deps.History.Search(ctx, query, homeSuggestionLimit)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry != nil {
				rows = append(rows, homeRow{URL: entry.URL, Title: entry.Title})
			}
		}
	}

	a.lockState()
	defer a.unlockState()
	if a.closed || seq != a.homeSuggestSeq {
		return nil
	}
	return updater.UpdateRegions(RegionUpdate{Regions: []DOMRegion{{
		Name: homeRegionSuggestions,
		HTML: mustRenderComponent(HomeSuggestions(rows)),
	}}})
}

// refreshHomeClockLocked re-renders the clock widgets whose text changed.
func (a *App) refreshHomeClockLocked() error {
	now := a.homeNow()
	next := append([]DOMRegion(nil), a.homeRendered...)
	for i := range a.homeData.Widgets {
		widget := &a.homeData.Widgets[i]
		if widget.Widget.Type != entity.HomeWidgetClock {
			continue
		}
		widget.Time, widget.Date = homeClockText(widget.Widget.Format, now)
		for j := range next {
			if next[j].Name == homeWidgetRegion(i) {
				next[j].HTML = mustRenderComponent(HomeWidget(*widget))
			}
		}
	}
	return a.applyHomeRegionsLocked(next)
}

// reloadHomeLocked reloads home.toml data and swaps only changed regions.
func (a *App) reloadHomeLocked(ctx context.Context) error {
	data, err := a.buildHomeData(ctx, a.homeData.Widgets)
	if err != nil {
		return err
	}
	a.homeGeneration++
	a.homeData = data
	if err := a.applyHomeRegionsLocked(homeRegions(data)); err != nil {
		return err
	}
	a.startHomeFillLocked(ctx)
	return nil
}

// startHomeFillLocked loads every list widget in the background, each one
// swapping its own region when it returns. Caller holds App.mu.
func (a *App) startHomeFillLocked(ctx context.Context) {
	if a.closed || ctx == nil {
		return
	}
	a.startHomeServiceWidgetsLocked(ctx)
	generation := a.homeGeneration
	widgets := append([]homeWidgetData(nil), a.homeData.Widgets...)
	for i := range widgets {
		if !homeListWidget(widgets[i].Widget.Type) {
			continue
		}
		a.homeFillWG.Add(1)
		go func(index int, widget entity.HomeBuiltinWidget) {
			defer a.homeFillWG.Done()
			data := a.fetchHomeWidget(ctx, widget)
			if ctx.Err() != nil {
				return
			}
			a.lockState()
			defer a.unlockState()
			if a.closed || a.homeGeneration != generation || index >= len(a.homeData.Widgets) {
				return
			}
			a.homeData.Widgets[index] = data
			next := append([]DOMRegion(nil), a.homeRendered...)
			for j := range next {
				if next[j].Name == homeWidgetRegion(index) {
					next[j].HTML = mustRenderComponent(HomeWidget(data))
				}
			}
			if err := a.applyHomeRegionsLocked(next); err != nil {
				logActionMountError(ctx, err, nil)
			}
		}(i, widgets[i].Widget)
	}
}

// startHomeFill is startHomeFillLocked after the initial mount, bound to
// the action workers' context when they run.
func (a *App) startHomeFill(ctx context.Context) {
	a.lockAction()
	if a.actionCtx != nil {
		ctx = a.actionCtx
	}
	a.unlockAction()
	a.lockState()
	defer a.unlockState()
	if a.currentRoute == RouteHome {
		a.startHomeFillLocked(ctx)
	}
}

func (a *App) applyHomeRegionsLocked(next []DOMRegion) error {
	changed, sameShape := changedHomeRegions(a.homeRendered, next)
	a.homeRendered = next
	a.renderHomePageLocked()
	if sameShape && len(changed) == 0 {
		return nil
	}
	if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok && sameShape {
		// Widget, action and embed regions are never in changed: they
		// survive swaps untouched and need no restore.
		return updater.UpdateRegions(RegionUpdate{Regions: changed})
	}
	if err := a.mountRenderedHTML(); err != nil {
		return err
	}
	return a.restoreHomeServiceSlotsLocked(nil, true)
}

// startHomeClock queues a clock refresh every homeClockInterval until the
// action workers stop.
func (a *App) startHomeClock() {
	a.lockAction()
	ctx := a.actionCtx
	ready := !a.actionClosed && ctx != nil && ctx.Err() == nil
	if ready {
		a.actionWG.Add(1)
	}
	a.unlockAction()
	if !ready {
		return
	}
	go func() {
		defer a.actionWG.Done()
		ticker := time.NewTicker(homeClockInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.enqueueDOMAction(DOMAction{Action: homeActionClockTick})
			}
		}
	}()
}
