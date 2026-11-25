package systemviews

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// homeServiceRegionPrefix + service id names the widget slot of a service
// shell. The shell renders it with a loading placeholder; its data is pushed
// into the slot alone, so a refresh never touches the rest of the shell.
const homeServiceRegionPrefix = "home-svc:"

// homeServiceMinRefresh is the shortest allowed widget refresh interval.
const homeServiceMinRefresh = 5 * time.Second

// homeServiceWidgets is the page state of the service widgets. It is
// guarded by App.mu.
type homeServiceWidgets struct {
	// cancel stops the refresh loops of the current generation.
	cancel context.CancelFunc
	// slots is the last markup pushed into each service slot.
	slots map[string]string
	// actions is the action UI state of services whose actions are not idle.
	actions map[string]homeActionState
	// embeds is the key (url and height) of the iframe pushed into each embed
	// region; an unchanged key is never pushed again, so the iframe is kept.
	embeds map[string]string
}

// homeServiceWidgetView is what one api widget slot renders.
type homeServiceWidgetView struct {
	Display entity.HomeDisplay
	Data    dto.HomeWidgetDataResponse
	// Formats maps a field label to its declared format.
	Formats map[string]entity.HomeFieldFormat
}

// homeServiceBar is one row of a bar display.
type homeServiceBar struct {
	Label   string
	Value   string
	Percent float64
}

func homeServiceRegion(id string) string {
	return homeServiceRegionPrefix + id
}

func homeServiceHasAPIWidget(service entity.HomeService) bool {
	return service.Widget != nil && service.Widget.Type == entity.HomeServiceWidgetAPI
}

// homeAPIServices returns the services of load with an api widget.
func homeAPIServices(load dto.HomeLoadResponse) []entity.HomeService {
	var services []entity.HomeService
	for _, group := range load.Layout.Groups {
		for i := range group.Services {
			service := &group.Services[i]
			if homeServiceHasAPIWidget(*service) {
				services = append(services, *service)
			}
		}
	}
	return services
}

// homeServiceRefresh parses the refresh interval. Zero means no refresh.
func homeServiceRefresh(widget *entity.HomeServiceWidget) time.Duration {
	if widget == nil || strings.TrimSpace(widget.Refresh) == "" {
		return 0
	}
	interval, err := time.ParseDuration(strings.TrimSpace(widget.Refresh))
	if err != nil || interval <= 0 {
		return 0
	}
	return max(interval, homeServiceMinRefresh)
}

func newHomeServiceView(service entity.HomeService, data dto.HomeWidgetDataResponse) homeServiceWidgetView {
	view := homeServiceWidgetView{Display: entity.HomeDisplayStats, Data: data}
	if service.Widget != nil {
		if service.Widget.Display != "" {
			view.Display = service.Widget.Display
		}
		view.Formats = make(map[string]entity.HomeFieldFormat, len(service.Widget.Fields))
		for _, field := range service.Widget.Fields {
			view.Formats[field.Label] = field.Format
		}
	}
	return view
}

// homeServiceBars turns fields into bars. The width is the raw number as a
// percent, clamped to 0-100; a 0-1 ratio counts as a percent when the field
// format is percent, matching entity.FormatHomeField.
func homeServiceBars(view homeServiceWidgetView) []homeServiceBar {
	bars := make([]homeServiceBar, 0, len(view.Data.Fields))
	for _, field := range view.Data.Fields {
		pct, ok := homeRawNumber(field.Raw)
		if ok && view.Formats[field.Label] == entity.HomeFormatPercent && math.Abs(pct) <= 1 {
			pct *= 100
		}
		if !ok || math.IsNaN(pct) {
			pct = 0
		}
		bars = append(bars, homeServiceBar{Label: field.Label, Value: field.Value, Percent: math.Min(math.Max(pct, 0), 100)})
	}
	return bars
}

func homeRawNumber(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, !math.IsInf(v, 0)
	case int:
		return float64(v), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "%")), 64)
		return n, err == nil && !math.IsInf(n, 0)
	default:
		return 0, false
	}
}

func homeBarStyle(bar homeServiceBar) string {
	return "--sv-home-bar: " + strconv.FormatFloat(bar.Percent, 'f', 1, 64) + "%;"
}

// startHomeServiceWidgetsLocked fetches every api widget once, then on its
// refresh interval, until the next reload, route change or Close. Caller
// holds App.mu.
func (a *App) startHomeServiceWidgetsLocked(ctx context.Context) {
	a.stopHomeServiceWidgetsLocked()
	if a.closed || ctx == nil {
		return
	}
	a.syncHomeEmbedsLocked(ctx)
	if a.deps.Home == nil {
		return
	}
	services := homeAPIServices(a.homeData.Load)
	if len(services) == 0 {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	a.homeSvc.cancel = cancel
	generation := a.homeGeneration
	for i := range services {
		service := services[i]
		a.homeFillWG.Add(1)
		go func(service entity.HomeService) {
			defer a.homeFillWG.Done()
			if !a.fetchHomeServiceWidget(loopCtx, generation, service) {
				return
			}
			if interval := homeServiceRefresh(service.Widget); interval > 0 {
				// Add before this fill goroutine finishes, so a Close that
				// has waited for the fills also waits for the loop.
				a.homeRefreshWG.Add(1)
				go func() {
					defer a.homeRefreshWG.Done()
					a.refreshHomeServiceWidget(loopCtx, generation, service, interval)
				}()
			}
		}(service)
	}
}

func (a *App) refreshHomeServiceWidget(ctx context.Context, generation uint64, service entity.HomeService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !a.fetchHomeServiceWidget(ctx, generation, service) {
				return
			}
		}
	}
}

// fetchHomeServiceWidget loads one widget and swaps its slot when the
// markup changed. It reports false once the widget is no longer current.
func (a *App) fetchHomeServiceWidget(ctx context.Context, generation uint64, service entity.HomeService) bool {
	data, err := a.deps.Home.HomeWidgetData(ctx, service.ID)
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		data = dto.HomeWidgetDataResponse{ServiceID: service.ID, Status: dto.HomeStatusError, Error: err.Error()}
	}
	markup := mustRenderComponent(HomeServiceWidget(newHomeServiceView(service, data)))

	a.lockState()
	defer a.unlockState()
	if a.closed || a.currentRoute != RouteHome || a.homeGeneration != generation {
		return false
	}
	if a.homeSvc.slots == nil {
		a.homeSvc.slots = make(map[string]string)
	}
	if a.homeSvc.slots[service.ID] == markup {
		return true
	}
	a.homeSvc.slots[service.ID] = markup
	if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok {
		if err := updater.UpdateRegions(RegionUpdate{Regions: []DOMRegion{{Name: homeServiceRegion(service.ID), HTML: markup}}}); err != nil {
			logActionMountError(ctx, err, nil)
		}
	}
	return true
}

// stopHomeServiceWidgetsLocked stops the refresh loops. Caller holds App.mu.
func (a *App) stopHomeServiceWidgetsLocked() {
	if a.homeSvc.cancel != nil {
		a.homeSvc.cancel()
		a.homeSvc.cancel = nil
	}
}

// resetHomeServiceWidgetsLocked stops the loops and forgets the pushed
// markup when leaving the home route. Caller holds App.mu.
func (a *App) resetHomeServiceWidgetsLocked() {
	a.stopHomeServiceWidgetsLocked()
	a.homeSvc.slots = nil
	a.homeSvc.actions = nil
	a.homeSvc.embeds = nil
}

// restoreHomeServiceSlotsLocked pushes the cached widget markup, open action
// forms and embeds back after a full remount, which renders them empty and
// idle. Caller holds App.mu.
func (a *App) restoreHomeServiceSlotsLocked() error {
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return nil
	}
	var regions []DOMRegion
	services := homeAPIServices(a.homeData.Load)
	for i := range services {
		id := services[i].ID
		if markup := a.homeSvc.slots[id]; markup != "" {
			regions = append(regions, DOMRegion{Name: homeServiceRegion(id), HTML: markup})
		}
	}
	regions = append(regions, a.homeActionRegionsLocked()...)
	// The remount emptied every embed region: push them all again.
	a.homeSvc.embeds = nil
	regions = append(regions, a.changedHomeEmbedRegionsLocked()...)
	if len(regions) == 0 {
		return nil
	}
	return updater.UpdateRegions(RegionUpdate{Regions: regions})
}

// homeServiceWidgetInlineMax is the most stats fields shown inline on a
// service row; more fields, lists, bars and template HTML go under the row.
const homeServiceWidgetInlineMax = 2

// homeServiceWidgetInline reports whether view fits on the service row.
func homeServiceWidgetInline(view homeServiceWidgetView) bool {
	switch view.Display {
	case entity.HomeDisplayList, entity.HomeDisplayBar:
		return false
	}
	return view.Data.HTML == "" && view.Data.Error == "" &&
		len(view.Data.Fields) > 0 && len(view.Data.Fields) <= homeServiceWidgetInlineMax
}
