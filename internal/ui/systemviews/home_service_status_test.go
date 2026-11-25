package systemviews

import (
	"context"
	"html"
	"testing"
	"testing/synctest"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func homeStatusLayout() entity.HomeLayout {
	off := false
	return entity.HomeLayout{
		Settings: entity.HomeSettings{Title: "Lab", Columns: 3, Status: true},
		Groups: []entity.HomeGroup{
			{Name: "Media", Services: []entity.HomeService{
				{ID: "jelly", Name: "Jellyfin", URL: "https://jelly.lan"},
				{ID: "cam", Name: "Camera", URL: "https://cam.lan"},
			}},
			{Name: "Infra", Services: []entity.HomeService{
				{ID: "nas", Name: "NAS", URL: "https://nas.lan", Status: &off},
				{ID: "router", Name: "Router"},
			}},
		},
	}
}

func (s *homeStore) setStatus(id string, resp dto.HomeStatusResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = map[string]dto.HomeStatusResponse{}
	}
	s.statuses[id] = resp
}

func (s *homeStore) statusCallCount(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusCalls[id]
}

func TestHomeStatusDotLabels(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		status dto.HomeStatusResponse
		ok     bool
		state  string
		label  string
	}{
		"pending":         {ok: false, state: homeStatusPending, label: "Checking…"},
		"up":              {status: dto.HomeStatusResponse{State: dto.HomeServiceStateUp, LatencyMS: 4}, ok: true, state: "up", label: "Up · <10 ms"},
		"up tens":         {status: dto.HomeStatusResponse{State: dto.HomeServiceStateUp, LatencyMS: 42}, ok: true, state: "up", label: "Up · 40 ms"},
		"up slow":         {status: dto.HomeStatusResponse{State: dto.HomeServiceStateUp, LatencyMS: 1260}, ok: true, state: "up", label: "Up · 1.3 s"},
		"up tls":          {status: dto.HomeStatusResponse{State: dto.HomeServiceStateUp, LatencyMS: 4, TLSUnverified: true}, ok: true, state: "up", label: "Up · certificate not verified · <10 ms"},
		"error":           {status: dto.HomeStatusResponse{State: dto.HomeServiceStateError, Code: 503}, ok: true, state: "error", label: "Error · HTTP 503"},
		"down":            {status: dto.HomeStatusResponse{State: dto.HomeServiceStateDown}, ok: true, state: "down", label: "Down"},
		"unknown is down": {status: dto.HomeStatusResponse{State: "weird"}, ok: true, state: "down", label: "Down"},
	} {
		dot := newHomeStatusDot(tc.status, tc.ok)
		assert.Equal(t, tc.state, dot.State, name)
		assert.Equal(t, tc.label, dot.Label, name)
		markup := mustRenderComponent(HomeServiceStatus(dot))
		assert.Contains(t, markup, `data-state="`+tc.state+`"`, name)
		assert.Contains(t, markup, `aria-label="`+html.EscapeString(tc.label)+`"`, name)
	}
	// Jitter inside a bucket keeps the same markup.
	a := mustRenderComponent(HomeServiceStatus(newHomeStatusDot(dto.HomeStatusResponse{State: "up", LatencyMS: 41}, true)))
	b := mustRenderComponent(HomeServiceStatus(newHomeStatusDot(dto.HomeStatusResponse{State: "up", LatencyMS: 43}, true)))
	assert.Equal(t, a, b)
}

func TestHomeStatusCountersAndDots(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeStatusLayout()}})
	home.setStatus("cam", dto.HomeStatusResponse{ServiceID: "cam", State: dto.HomeServiceStateError, Code: 503})
	dom := newTreeDOM()
	newMountedHomeApp(t, dom, home)

	assert.Equal(t, 1, home.statusCallCount("jelly"))
	assert.Equal(t, 1, home.statusCallCount("cam"))
	assert.Zero(t, home.statusCallCount("nas"), "status = false")
	assert.Zero(t, home.statusCallCount("router"), "no url")

	assert.Contains(t, dom.html(), `data-sv-region="home-svc-status:jelly"><span class="sv-home-status-dot" data-state="up"`)
	assert.Contains(t, dom.html(), `aria-label="Error · HTTP 503"`)
	assert.Empty(t, dom.text(homeServiceStatusRegion("nas")))
	summary := dom.text(homeRegionSummary)
	assert.Contains(t, summary, "4 services")
	assert.Contains(t, summary, "1 offline")
	assert.Contains(t, dom.html(), "sv-home-status-offline-some")
	assert.Equal(t, "1/2", dom.text("home-group-count:g1"))
	assert.Empty(t, dom.text("home-group-count:g2"), "no checked service in the group")
}

func TestHomeStatusSummaryWithoutChecks(t *testing.T) {
	t.Parallel()
	layout := homeStatusLayout()
	layout.Settings.Status = false
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: layout}})
	dom := newTreeDOM()
	newMountedHomeApp(t, dom, home)
	assert.Equal(t, "4 services", dom.text(homeRegionSummary))
	assert.Empty(t, dom.text("home-group-count:g1"))
	assert.NotContains(t, dom.html(), "sv-home-status-dot")
}

func TestHomeStatusNoSwapWhenUnchanged(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeStatusLayout()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	dom.reset()

	// Same state, latency within the bucket: nothing swaps.
	home.setStatus("jelly", dto.HomeStatusResponse{ServiceID: "jelly", State: dto.HomeServiceStateUp, LatencyMS: 6})
	require.True(t, app.probeHomeService(t.Context(), app.homeGeneration, "jelly"))
	assert.Empty(t, dom.regions())

	// Going down swaps the dot, the summary and the group count.
	home.setStatus("jelly", dto.HomeStatusResponse{ServiceID: "jelly", State: dto.HomeServiceStateDown})
	require.True(t, app.probeHomeService(t.Context(), app.homeGeneration, "jelly"))
	require.Len(t, dom.regions(), 1)
	var names []string
	for _, region := range dom.regions()[0] {
		names = append(names, region.Name)
	}
	assert.ElementsMatch(t, []string{homeServiceStatusRegion("jelly"), homeRegionSummary, "home-group-count:g1"}, names)
	assert.Contains(t, dom.text(homeRegionSummary), "1 offline")
	assert.Equal(t, "1/2", dom.text("home-group-count:g1"))
}

func TestHomeStatusReloadStartsAndStopsChecks(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeStatusLayout()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	require.Contains(t, dom.html(), `home-svc-status:cam"><span class="sv-home-status-dot"`)

	// Turning a service on starts its check.
	on := true
	home.mu.Lock()
	home.load.Layout.Groups[1].Services[0].Status = &on
	home.mu.Unlock()
	reloadHome(t, app)
	assert.Equal(t, 1, home.statusCallCount("nas"))
	assert.Contains(t, dom.html(), `home-svc-status:nas"><span class="sv-home-status-dot" data-state="up"`)
	assert.Equal(t, "1/1", dom.text("home-group-count:g2"), "router has no url")

	// Turning status off everywhere clears every dot and counter, without a
	// remount.
	mounts := dom.mountCount()
	home.mu.Lock()
	home.load.Layout.Settings.Status = false
	home.load.Layout.Groups[1].Services[0].Status = nil
	home.mu.Unlock()
	reloadHome(t, app)
	assert.Equal(t, mounts, dom.mountCount())
	assert.NotContains(t, dom.html(), "sv-home-status-dot")
	assert.Equal(t, "4 services", dom.text(homeRegionSummary))
	assert.Empty(t, dom.text("home-group-count:g1"))
	assert.Empty(t, dom.text("home-group-count:g2"))
	app.lockState()
	assert.Empty(t, app.homeSvc.statuses)
	app.unlockState()
}

func TestHomeStatusLoopLifetime(t *testing.T) {
	t.Parallel()
	for name, stop := range map[string]func(*App){
		"reload replaces the loop": func(app *App) {
			app.lockState()
			require.NoError(t, app.reloadHomeLocked(context.Background()))
			app.unlockState()
			app.homeFillWG.Wait()
		},
		"route change stops the loop": func(app *App) {
			app.lockState()
			app.resetRouteState()
			app.unlockState()
		},
		"close stops the loop": func(app *App) { app.Close() },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				load := homeLayoutWith(entity.HomeService{ID: "s", Name: "S", URL: "https://s.lan"})
				load.Layout.Settings.Status = true
				home := newHomeStore(t, &homeStore{load: load})
				app, _ := newServiceApp(t, home)
				require.Equal(t, 1, home.statusCallCount("s"), "probed right away")
				reload := name == "reload replaces the loop"
				stop(app)
				synctest.Wait()
				before := home.statusCallCount("s")

				time.Sleep(homeStatusInterval)
				synctest.Wait()
				// One loop left after a reload, none otherwise. A leaked
				// loop also fails the bubble when the test ends.
				want := before
				if reload {
					want++
				}
				assert.Equal(t, want, home.statusCallCount("s"))
			})
		})
	}
}

func TestHomeStatusRemovedServiceForgetsItsDot(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeStatusLayout()}})
	dom := newTreeDOM()
	app := newMountedHomeApp(t, dom, home)
	app.lockState()
	require.NotEmpty(t, app.homeSvc.dots["cam"])
	app.unlockState()

	home.mu.Lock()
	home.load.Layout.Groups[0].Services = home.load.Layout.Groups[0].Services[:1]
	home.mu.Unlock()
	reloadHome(t, app)

	app.lockState()
	defer app.unlockState()
	assert.NotContains(t, app.homeSvc.dots, "cam")
	assert.NotContains(t, app.homeSvc.statuses, "cam")
	assert.Contains(t, app.homeSvc.dots, "jelly")
}
