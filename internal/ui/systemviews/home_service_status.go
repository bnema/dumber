package systemviews

import (
	"context"
	"strconv"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// homeStatusInterval is how often an enabled service is probed. The native
// side caches results for 30s, so a reload that restarts the loops is cheap.
const homeStatusInterval = 60 * time.Second

// homeStatusPending is the dot state before the first result.
const homeStatusPending = "pending"

// homeStatusDot is what one status region renders.
type homeStatusDot struct {
	State string
	Label string
}

// homeStatusServices returns the services whose status check is enabled.
func homeStatusServices(load dto.HomeLoadResponse) []entity.HomeService {
	var services []entity.HomeService
	for _, group := range load.Layout.Groups {
		for i := range group.Services {
			if group.Services[i].StatusEnabled(load.Layout.Settings) {
				services = append(services, group.Services[i])
			}
		}
	}
	return services
}

// newHomeStatusDot turns a probe result into a dot; ok is false while the
// first result is pending. Latency is bucketed so a few ms of jitter does
// not change the label, and so does not re-render the dot.
func newHomeStatusDot(status dto.HomeStatusResponse, ok bool) homeStatusDot {
	if !ok {
		return homeStatusDot{State: homeStatusPending, Label: "Checking…"}
	}
	switch status.State {
	case dto.HomeServiceStateUp:
		label := "Up"
		if status.TLSUnverified {
			label += " · certificate not verified"
		}
		return homeStatusDot{State: status.State, Label: label + " · " + homeLatencyText(status.LatencyMS)}
	case dto.HomeServiceStateError:
		label := "Error"
		if status.Code > 0 {
			label += " · HTTP " + strconv.Itoa(status.Code)
		}
		return homeStatusDot{State: status.State, Label: label}
	default:
		return homeStatusDot{State: dto.HomeServiceStateDown, Label: "Down"}
	}
}

// homeLatencyText buckets a latency: under 10 ms, then tens, then fifties,
// then tenths of a second.
func homeLatencyText(ms int) string {
	const (
		fine   = 10
		coarse = 50
		tenth  = 100
		second = 1000
	)
	switch {
	case ms < fine:
		return "<10 ms"
	case ms < tenth:
		return strconv.Itoa((ms+fine/2)/fine*fine) + " ms"
	case ms < second:
		return strconv.Itoa((ms+coarse/2)/coarse*coarse) + " ms"
	default:
		return strconv.FormatFloat(float64((ms+tenth/2)/tenth)/fine, 'f', 1, 64) + " s"
	}
}

// homeOffline reports whether a result counts as offline: down or error.
func homeOffline(status dto.HomeStatusResponse) bool {
	return status.State == dto.HomeServiceStateDown || status.State == dto.HomeServiceStateError
}

// homeSummary is the service count of the header line.
type homeSummary struct {
	Services int
	// Status is set when at least one service has its status check on.
	Status  bool
	Offline int
}

func homeSummaryFor(load dto.HomeLoadResponse, statuses map[string]dto.HomeStatusResponse) homeSummary {
	summary := homeSummary{Services: homeServiceCount(load)}
	services := homeStatusServices(load)
	for i := range services {
		summary.Status = true
		if status, ok := statuses[services[i].ID]; ok && homeOffline(status) {
			summary.Offline++
		}
	}
	return summary
}

// homeGroupCountText is "up/total" over the status-enabled services of
// group, once each has a result; "" otherwise.
func homeGroupCountText(group entity.HomeGroup, settings entity.HomeSettings, statuses map[string]dto.HomeStatusResponse) string {
	up, total := 0, 0
	for i := range group.Services {
		service := &group.Services[i]
		if !service.StatusEnabled(settings) {
			continue
		}
		status, ok := statuses[service.ID]
		if !ok {
			return ""
		}
		total++
		if !homeOffline(status) {
			up++
		}
	}
	if total == 0 {
		return ""
	}
	return strconv.Itoa(up) + "/" + strconv.Itoa(total)
}

// pruneHomeStatusesLocked forgets the results of services that are gone or
// no longer checked. Caller holds App.mu.
func (a *App) pruneHomeStatusesLocked(load dto.HomeLoadResponse) {
	enabled := make(map[string]bool)
	services := homeStatusServices(load)
	for i := range services {
		enabled[services[i].ID] = true
	}
	for id := range a.homeSvc.statuses {
		if !enabled[id] {
			delete(a.homeSvc.statuses, id)
		}
	}
	// A service gone from the layout takes its dot with it; a service
	// still there but unchecked keeps it until syncHomeDotsLocked clears
	// its region.
	present := make(map[string]bool)
	for gi := range load.Layout.Groups {
		for si := range load.Layout.Groups[gi].Services {
			present[load.Layout.Groups[gi].Services[si].ID] = true
		}
	}
	for id := range a.homeSvc.dots {
		if !present[id] {
			delete(a.homeSvc.dots, id)
		}
	}
}

// syncHomeDotsLocked pushes the current dot of every checked service and
// clears the dots of services no longer checked. Caller holds App.mu.
func (a *App) syncHomeDotsLocked() {
	services := homeStatusServices(a.homeData.Load)
	enabled := make(map[string]bool, len(services))
	var regions []DOMRegion
	for i := range services {
		id := services[i].ID
		enabled[id] = true
		status, ok := a.homeSvc.statuses[id]
		regions = append(regions, a.homeDotRegionLocked(id, newHomeStatusDot(status, ok))...)
	}
	for gi := range a.homeData.Load.Layout.Groups {
		group := &a.homeData.Load.Layout.Groups[gi]
		for si := range group.Services {
			id := group.Services[si].ID
			if !enabled[id] && a.homeSvc.dots[id] != "" {
				delete(a.homeSvc.dots, id)
				regions = append(regions, DOMRegion{Name: homeServiceStatusRegion(id)})
			}
		}
	}
	a.pushHomeRegionsLocked(regions)
}

// startHomeStatusLocked syncs the dots, then starts one probe loop per
// checked service under loopCtx: a probe now, then every
// homeStatusInterval. Caller holds App.mu.
func (a *App) startHomeStatusLocked(loopCtx context.Context, generation uint64) {
	a.syncHomeDotsLocked()
	services := homeStatusServices(a.homeData.Load)
	for i := range services {
		id := services[i].ID
		a.startHomeLoopLocked(loopCtx, homeStatusInterval, func(ctx context.Context) bool {
			return a.probeHomeService(ctx, generation, id)
		})
	}
}

// probeHomeService asks for the status of id and swaps what changed: the
// dot, the header summary and the group count. It reports false once the
// loop is no longer current.
func (a *App) probeHomeService(ctx context.Context, generation uint64, id string) bool {
	status, err := a.deps.Home.HomeStatus(ctx, id)
	if ctx.Err() != nil {
		return false
	}
	if err != nil {
		status = dto.HomeStatusResponse{ServiceID: id, State: dto.HomeServiceStateDown}
	}
	a.lockState()
	defer a.unlockState()
	if a.closed || a.currentRoute != RouteHome || a.homeGeneration != generation {
		return false
	}
	if a.homeSvc.statuses == nil {
		a.homeSvc.statuses = make(map[string]dto.HomeStatusResponse)
	}
	a.homeSvc.statuses[id] = status
	regions := a.homeDotRegionLocked(id, newHomeStatusDot(status, true))
	regions = append(regions, a.changedHomeCountersLocked()...)
	a.pushHomeRegionsLocked(regions)
	return true
}

// homeDotRegionLocked returns the status region of id when its dot
// changed, and records it. Caller holds App.mu.
func (a *App) homeDotRegionLocked(id string, dot homeStatusDot) []DOMRegion {
	markup := mustRenderComponent(HomeServiceStatus(dot))
	if a.homeSvc.dots[id] == markup {
		return nil
	}
	if a.homeSvc.dots == nil {
		a.homeSvc.dots = make(map[string]string)
	}
	a.homeSvc.dots[id] = markup
	return []DOMRegion{{Name: homeServiceStatusRegion(id), HTML: markup}}
}

// changedHomeCountersLocked re-renders the summary and group counts from
// the current results and returns the ones that changed. The rendered
// copies are updated, so a reload diffs against what the page shows.
// Caller holds App.mu.
func (a *App) changedHomeCountersLocked() []DOMRegion {
	var regions []DOMRegion
	summary := mustRenderComponent(HomeStatusSummary(homeSummaryFor(a.homeData.Load, a.homeSvc.statuses)))
	for i := range a.homeRendered {
		if a.homeRendered[i].Name == homeRegionSummary && a.homeRendered[i].HTML != summary {
			a.homeRendered[i].HTML = summary
			regions = append(regions, DOMRegion{Name: homeRegionSummary, HTML: summary})
		}
	}
	for gi := range a.homeData.Load.Layout.Groups {
		if gi >= len(a.homeGroups.Groups) {
			break
		}
		name := homeGroupCountRegion(a.homeGroups.Groups[gi].Key)
		text := homeGroupCountText(a.homeData.Load.Layout.Groups[gi], a.homeData.Load.Layout.Settings, a.homeSvc.statuses)
		markup := mustRenderComponent(HomeGroupCount(text))
		if a.homeLeaves[name] != markup {
			a.homeLeaves[name] = markup
			regions = append(regions, DOMRegion{Name: name, HTML: markup})
		}
	}
	return regions
}

// restoreHomeDotsLocked returns every recorded dot, for a remount that
// rendered the status regions empty. Caller holds App.mu.
func (a *App) restoreHomeDotsLocked() []DOMRegion {
	var regions []DOMRegion
	services := homeStatusServices(a.homeData.Load)
	for i := range services {
		id := services[i].ID
		if markup := a.homeSvc.dots[id]; markup != "" {
			regions = append(regions, DOMRegion{Name: homeServiceStatusRegion(id), HTML: markup})
		}
	}
	return regions
}

// pushHomeRegionsLocked swaps regions, logging a failure. Caller holds
// App.mu.
func (a *App) pushHomeRegionsLocked(regions []DOMRegion) {
	if len(regions) == 0 {
		return
	}
	if updater, ok := a.deps.DOM.(DOMRegionUpdater); ok {
		if err := updater.UpdateRegions(RegionUpdate{Regions: regions}); err != nil {
			logActionMountError(context.Background(), err, nil)
		}
	}
}
