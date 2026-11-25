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
// tile. The slot sits inside the home-groups region, which renders it empty;
// its data is pushed separately so a refresh never swaps the whole grid.
const homeServiceRegionPrefix = "home-svc-"

// homeServiceMinRefresh is the shortest allowed widget refresh interval.
const homeServiceMinRefresh = 5 * time.Second

// homeServiceWidgets is the page state of the service widgets. It is
// guarded by App.mu.
type homeServiceWidgets struct {
	// cancel stops the refresh loops of the current generation.
	cancel context.CancelFunc
	// slots is the last markup pushed into each service slot.
	slots map[string]string
	// newTicker returns a tick channel and its stop function. Tests replace it.
	newTicker func(time.Duration) (<-chan time.Time, func())
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
	return service.ID != "" && service.Widget != nil && service.Widget.Type == entity.HomeServiceWidgetAPI
}

// homeAPIServices returns the services of load with an api widget.
func homeAPIServices(load dto.HomeLoadResponse) []entity.HomeService {
	var services []entity.HomeService
	for _, group := range load.Layout.Groups {
		for _, service := range group.Services {
			if homeServiceHasAPIWidget(service) {
				services = append(services, service)
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
	if a.closed || ctx == nil || a.deps.Home == nil {
		return
	}
	services := homeAPIServices(a.homeData.Load)
	if len(services) == 0 {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	a.homeSvc.cancel = cancel
	generation := a.homeGeneration
	for _, service := range services {
		a.homeFillWG.Add(1)
		go func(service entity.HomeService) {
			defer a.homeFillWG.Done()
			if !a.fetchHomeServiceWidget(loopCtx, generation, service) {
				return
			}
			if interval := homeServiceRefresh(service.Widget); interval > 0 {
				go a.refreshHomeServiceWidget(loopCtx, generation, service, interval)
			}
		}(service)
	}
}

func (a *App) refreshHomeServiceWidget(ctx context.Context, generation uint64, service entity.HomeService, interval time.Duration) {
	a.lockState()
	newTicker := a.homeSvc.newTicker
	a.unlockState()
	if newTicker == nil {
		newTicker = func(d time.Duration) (<-chan time.Time, func()) {
			ticker := time.NewTicker(d)
			return ticker.C, ticker.Stop
		}
	}
	ticks, stop := newTicker(interval)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
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
}

// restoreHomeServiceSlotsLocked pushes the cached widget markup back after
// the home-groups region (which renders the slots empty) was swapped or the
// page remounted. Caller holds App.mu.
func (a *App) restoreHomeServiceSlotsLocked(changed []DOMRegion, remounted bool) error {
	if len(a.homeSvc.slots) == 0 {
		return nil
	}
	if !remounted && !homeRegionsInclude(changed, homeRegionGroups) {
		return nil
	}
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return nil
	}
	var regions []DOMRegion
	for _, service := range homeAPIServices(a.homeData.Load) {
		if markup := a.homeSvc.slots[service.ID]; markup != "" {
			regions = append(regions, DOMRegion{Name: homeServiceRegion(service.ID), HTML: markup})
		}
	}
	if len(regions) == 0 {
		return nil
	}
	return updater.UpdateRegions(RegionUpdate{Regions: regions})
}

func homeRegionsInclude(regions []DOMRegion, name string) bool {
	for _, region := range regions {
		if region.Name == name {
			return true
		}
	}
	return false
}
