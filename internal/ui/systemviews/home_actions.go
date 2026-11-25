package systemviews

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	a.setHomeGroupsLocked(buildHomeShape(data.Load.Layout.Groups, homeShape{}, &a.homeGroupSeq))
	a.renderHomePageLocked()
	return nil
}

// setHomeGroupsLocked records shape and the leaves and action keys of the
// current layout. Caller holds App.mu.
func (a *App) setHomeGroupsLocked(shape homeShape) {
	a.homeGroups = shape
	a.homeLeaves = homeGroupLeaves(a.homeData.Load, shape)
	a.homeActionKeys, a.homeWidgetKeys = homeSlotKeys(a.homeData.Load)
}

func (a *App) renderHomePageLocked() {
	a.renderedHTML = renderAppFrame(renderedPage{
		route:    RouteHome,
		title:    homeTitle(a.homeData.Load),
		subtitle: "",
		body:     mustRenderComponent(HomePage(a.homeRendered, a.homeData.Load, a.homeGroups, a.homeLeaves)),
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
	previous := a.homeData.Load
	a.homeData = data
	err = a.applyHomeReloadLocked(previous, homeRegions(data))
	// The widget loops follow the new layout even when the page fell back
	// to a remount.
	a.startHomeFillLocked(ctx)
	return err
}

// applyHomeReloadLocked moves the page to the reloaded layout. Top regions
// swap as usual; the service groups are edited in place (insert, remove,
// leaf swaps, column patch) so untouched embeds, inputs and focus survive.
// Only a reorder, or a DOM without the editing capabilities, remounts.
// Caller holds App.mu.
func (a *App) applyHomeReloadLocked(previous dto.HomeLoadResponse, next []DOMRegion) error {
	shape := buildHomeShape(a.homeData.Load.Layout.Groups, a.homeGroups, &a.homeGroupSeq)
	changed, sameShape := changedHomeRegions(a.homeRendered, next)
	plan, planned := planHomeStructure(a.homeGroups, shape)
	updater, canUpdate := a.deps.DOM.(DOMRegionUpdater)
	editor, canEdit := a.deps.DOM.(DOMStructureEditor)
	oldLeaves, oldActionKeys, oldWidgetKeys := a.homeLeaves, a.homeActionKeys, a.homeWidgetKeys
	a.homeRendered = next
	a.setHomeGroupsLocked(shape)
	a.renderHomePageLocked()
	if !sameShape || !planned || !canUpdate || !canEdit {
		return a.remountHomeLocked()
	}
	base := homeReloadBase{previous: previous, changed: changed, leaves: oldLeaves, actionKeys: oldActionKeys, widgetKeys: oldWidgetKeys}
	if err := a.editHomeLocked(editor, updater, plan, base); err != nil {
		// A failed edit leaves the page half-edited: start over from the
		// full page.
		logActionMountError(context.Background(), err, nil)
		return a.remountHomeLocked()
	}
	return nil
}

// homeReloadBase is what a reload compares against: the previous layout,
// the changed top regions, and the previous leaves and action keys.
type homeReloadBase struct {
	previous   dto.HomeLoadResponse
	changed    []DOMRegion
	leaves     map[string]string
	actionKeys map[string]string
	widgetKeys map[string]string
}

// editHomeLocked applies the in-place edits of a reload. Caller holds App.mu.
func (a *App) editHomeLocked(editor DOMStructureEditor, updater DOMRegionUpdater, plan homeStructurePlan, base homeReloadBase) error {
	if err := a.applyHomeStructureLocked(editor, plan); err != nil {
		return err
	}
	if homeColumns(base.previous) != homeColumns(a.homeData.Load) {
		if err := editor.SetAnchorStyleProperty(homeAnchorGroups, homeColumnsProperty, strconv.Itoa(homeColumns(a.homeData.Load))); err != nil {
			return err
		}
	}
	regions := base.changed
	regions = append(regions, a.changedHomeLeavesLocked(base.leaves, plan)...)
	regions = append(regions, a.changedHomeActionRegionsLocked(base.actionKeys, plan)...)
	regions = append(regions, a.changedHomeWidgetSlotsLocked(base.widgetKeys, plan)...)
	regions = append(regions, a.freshBusyActionRegionsLocked(plan)...)
	if len(regions) == 0 {
		return nil
	}
	return updater.UpdateRegions(RegionUpdate{Regions: regions})
}

// freshBusyActionRegionsLocked renders a running action as busy in its new
// shell, so its completion still lands on a matching button.
func (a *App) freshBusyActionRegionsLocked(plan homeStructurePlan) []DOMRegion {
	var regions []DOMRegion
	for id := range plan.Fresh {
		state, ok := a.homeSvc.actions[id]
		if !ok {
			continue
		}
		service, found := findHomeService(a.homeData.Load, id)
		if !found || !homeServiceHasActions(service) {
			continue
		}
		regions = append(regions, DOMRegion{
			Name: homeServiceActionsRegion(id),
			HTML: mustRenderComponent(HomeServiceActions(homeServiceActionsView{Service: service, State: state})),
		})
	}
	return regions
}

// invalidateHomeRenderLocked forgets the rendered home page after an error
// page replaced it: clock ticks and widget fills find nothing to swap, and
// the next home.toml change remounts the page. Caller holds App.mu.
func (a *App) invalidateHomeRenderLocked() {
	a.homeGeneration++
	a.stopHomeServiceWidgetsLocked()
	a.homeData = homeRenderData{}
	a.homeRendered = nil
	a.homeGroups = homeShape{}
	a.homeLeaves = nil
	a.homeActionKeys = nil
	a.homeWidgetKeys = nil
}

func (a *App) remountHomeLocked() error {
	if err := a.mountRenderedHTML(); err != nil {
		return err
	}
	return a.restoreHomeServiceSlotsLocked()
}

// applyHomeStructureLocked removes, then inserts, the groups and service
// shells of plan. Fresh services forget their pushed widget and embed, so
// the widget loops push them into the new shell. Caller holds App.mu.
func (a *App) applyHomeStructureLocked(editor DOMStructureEditor, plan homeStructurePlan) error {
	for _, anchor := range plan.Removes {
		if err := editor.RemoveAnchor(anchor); err != nil {
			return err
		}
	}
	for _, op := range plan.Inserts {
		markup := ""
		if op.ServiceID == "" {
			group := a.homeData.Load.Layout.Groups[op.GroupIndex]
			markup = mustRenderComponent(HomeGroupBlock(group, a.homeGroups.Groups[op.GroupIndex].Key, a.homeLeaves))
		} else {
			service, ok := findHomeServiceAt(a.homeData.Load, op.GroupIndex, op.ServiceID)
			if !ok {
				continue
			}
			markup = mustRenderComponent(HomeServiceShell(*service, a.homeLeaves))
		}
		if err := editor.InsertAnchorMarkup(op.Parent, op.Before, markup); err != nil {
			return err
		}
	}
	for id := range plan.Fresh {
		delete(a.homeSvc.slots, id)
		delete(a.homeSvc.embeds, id)
		// A form or confirmation does not survive a new shell; a running
		// action does, so its completion toast still fires.
		if a.homeSvc.actions[id].Step != homeActionBusy {
			delete(a.homeSvc.actions, id)
		}
	}
	return nil
}

// changedHomeLeavesLocked returns the group title and tile regions whose
// markup changed, skipping the ones just inserted with their content.
func (a *App) changedHomeLeavesLocked(old map[string]string, plan homeStructurePlan) []DOMRegion {
	fresh := make(map[string]bool)
	for id := range plan.Fresh {
		fresh[homeServiceTileRegion(id)] = true
	}
	var regions []DOMRegion
	for _, group := range a.homeGroups.Groups {
		title := homeGroupTitleRegion(group.Key)
		if markup, ok := old[title]; ok && markup != a.homeLeaves[title] {
			regions = append(regions, DOMRegion{Name: title, HTML: a.homeLeaves[title]})
		}
		for _, service := range group.Services {
			tile := homeServiceTileRegion(service.ID)
			if fresh[tile] {
				continue
			}
			if markup, ok := old[tile]; ok && markup != a.homeLeaves[tile] {
				regions = append(regions, DOMRegion{Name: tile, HTML: a.homeLeaves[tile]})
			}
		}
	}
	return regions
}

// changedHomeActionRegionsLocked re-renders the idle actions of kept
// services whose actions changed, were added or were removed.
func (a *App) changedHomeActionRegionsLocked(old map[string]string, plan homeStructurePlan) []DOMRegion {
	var regions []DOMRegion
	for _, service := range a.keptHomeServicesLocked(plan) {
		if old[service.ID] == a.homeActionKeys[service.ID] {
			continue
		}
		if _, open := a.homeSvc.actions[service.ID]; open {
			// An open form or confirmation keeps what the user typed;
			// closing it renders the new actions.
			continue
		}
		regions = append(regions, DOMRegion{
			Name: homeServiceActionsRegion(service.ID),
			HTML: mustRenderComponent(HomeServiceIdleActions(*service)),
		})
	}
	return regions
}

// changedHomeWidgetSlotsLocked resets the api widget slot of kept services
// whose widget changed, was added or was removed. The widget loops push the
// new data. Caller holds App.mu.
func (a *App) changedHomeWidgetSlotsLocked(old map[string]string, plan homeStructurePlan) []DOMRegion {
	var regions []DOMRegion
	for _, service := range a.keptHomeServicesLocked(plan) {
		if old[service.ID] == a.homeWidgetKeys[service.ID] {
			continue
		}
		delete(a.homeSvc.slots, service.ID)
		regions = append(regions, DOMRegion{
			Name: homeServiceRegion(service.ID),
			HTML: mustRenderComponent(HomeServiceWidgetPlaceholder(*service)),
		})
	}
	return regions
}

// keptHomeServicesLocked returns the services of the layout that keep their
// shell in plan.
func (a *App) keptHomeServicesLocked(plan homeStructurePlan) []*entity.HomeService {
	var kept []*entity.HomeService
	for gi := range a.homeData.Load.Layout.Groups {
		group := &a.homeData.Load.Layout.Groups[gi]
		for si := range group.Services {
			if !plan.Fresh[group.Services[si].ID] {
				kept = append(kept, &group.Services[si])
			}
		}
	}
	return kept
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
	return a.remountHomeLocked()
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

// homeChangeApplies reports whether a systemview change event is a home.toml
// change seen by the home route.
func (a *App) homeChangeApplies(event DOMAction) bool {
	if event.Data["topic"] != dto.HomeChangedTopic {
		return false
	}
	a.lockState()
	defer a.unlockState()
	return a.currentRoute == RouteHome
}

// handleHomeEarlyAction runs the home actions that must not take the render
// lock path of HandleDOMAction. It reports whether event was handled.
func (a *App) handleHomeEarlyAction(ctx context.Context, event DOMAction) (bool, error) {
	switch {
	case event.Action == homeActionSuggest:
		return true, a.handleHomeSuggest(ctx, a.takeLiveSearchQuery(event.Data["query"]))
	case event.Action == actionSystemviewChanged && !a.homeChangeApplies(event):
		// Native notifies every system view pane; only dumb://home reacts.
		return true, nil
	case homeServiceAction(event):
		return true, a.handleHomeServiceAction(ctx, event)
	default:
		return false, nil
	}
}

// toastHomeError shows an action error of the home page as a toast and keeps
// the page: on dumb://home no action error is worth losing the page.
func (a *App) toastHomeError(err error) bool {
	a.lockState()
	home := a.currentRoute == RouteHome && !a.closed && len(a.homeRendered) > 0
	a.unlockState()
	if !home {
		return false
	}
	toaster, ok := a.deps.DOM.(DOMToaster)
	if !ok {
		return false
	}
	return toaster.ShowToast(ToastRequest{Message: err.Error(), Timeout: homeErrorToastTimeout})
}

// homeErrorToastTimeout is how long a home action error stays visible.
const homeErrorToastTimeout = 6 * time.Second
