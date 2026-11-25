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

// loadHomeRoute loads the layout and widget data and renders the full page.
// Caller holds App.mu.
func (a *App) loadHomeRoute(ctx context.Context) error {
	data, err := a.buildHomeData(ctx)
	if err != nil {
		return err
	}
	a.resetRouteState()
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
		body:     mustRenderComponent(HomePage(a.homeRendered)),
	}, a.shellTheme)
}

func (a *App) buildHomeData(ctx context.Context) (homeRenderData, error) {
	load := dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout()}
	if a.deps.Home != nil {
		var err error
		if load, err = a.deps.Home.HomeLoad(ctx); err != nil {
			return homeRenderData{}, err
		}
	}
	data := homeRenderData{Load: load, Widgets: make([]homeWidgetData, 0, len(load.Layout.Widgets))}
	for _, widget := range load.Layout.Widgets {
		data.Widgets = append(data.Widgets, a.buildHomeWidget(ctx, widget))
	}
	return data, nil
}

func (a *App) buildHomeWidget(ctx context.Context, widget entity.HomeBuiltinWidget) homeWidgetData {
	data := homeWidgetData{Widget: widget}
	var err error
	switch widget.Type {
	case entity.HomeWidgetClock:
		data.Time, data.Date = homeClockText(widget.Format, a.homeNow())
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
	case homeActionSuggest:
		return a.updateHomeSuggestionsLocked(ctx, event.Data["query"])
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

func (a *App) updateHomeSuggestionsLocked(ctx context.Context, query string) error {
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return nil
	}
	query = strings.TrimSpace(query)
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
	data, err := a.buildHomeData(ctx)
	if err != nil {
		return err
	}
	a.homeData = data
	return a.applyHomeRegionsLocked(homeRegions(data))
}

func (a *App) applyHomeRegionsLocked(next []DOMRegion) error {
	changed, sameShape := changedHomeRegions(a.homeRendered, next)
	a.homeRendered = next
	a.renderHomePageLocked()
	if sameShape && len(changed) == 0 {
		return nil
	}
	if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok && sameShape {
		return updater.UpdateRegions(RegionUpdate{Regions: changed})
	}
	return a.mountRenderedHTML()
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
