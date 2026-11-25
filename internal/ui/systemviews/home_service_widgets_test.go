package systemviews

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func apiService(id string, display entity.HomeDisplay, refresh string, fields ...entity.HomeField) entity.HomeService {
	return entity.HomeService{ID: id, Name: id, Widget: &entity.HomeServiceWidget{
		Type: entity.HomeServiceWidgetAPI, Display: display, Refresh: refresh, Fields: fields,
	}}
}

func homeLayoutWith(services ...entity.HomeService) dto.HomeLoadResponse {
	layout := entity.HomeLayout{Groups: []entity.HomeGroup{{Name: "Lab", Services: services}}}
	return dto.HomeLoadResponse{Layout: layout}
}

func newServiceApp(t *testing.T, home *homeStore) (*App, *treeDOM) {
	t.Helper()
	dom := newTreeDOM()
	app := NewApp(Dependencies{DOM: dom, Home: home.service, LocationURI: "dumb://home"})
	app.now = func() time.Time { return homeTestNow }
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
	fillHome(app)
	t.Cleanup(app.Close)
	return app, dom
}

func TestHomeServiceWidgetSlotIsAnEmptyRegion(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: homeLayoutWith(apiService("nas", entity.HomeDisplayStats, ""), entity.HomeService{ID: "plain", Name: "Plain"})})
	app, _ := newServiceApp(t, home)
	assert.Contains(t, app.renderedHTML, `data-sv-region="home-svc:nas"`)
	// The slot of a service without an api widget exists, empty, so adding
	// a widget later swaps its contents.
	assert.Contains(t, app.renderedHTML, `data-sv-region="home-svc:plain"></div>`)
	assert.Equal(t, 0, home.callCount("plain"))
}

func TestHomeServiceWidgetDisplays(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		service entity.HomeService
		data    dto.HomeWidgetDataResponse
		want    []string
		notWant []string
	}{
		{
			name:    "stats",
			service: apiService("s", entity.HomeDisplayStats, ""),
			data:    dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "Queries", Value: "1,204"}}},
			want:    []string{`sv-home-svc-stats`, `<dt class="sv-home-svc-label">Queries</dt>`, `<dd class="sv-home-svc-value">1,204</dd>`},
		},
		{
			name:    "list items",
			service: apiService("s", entity.HomeDisplayList, ""),
			data: dto.HomeWidgetDataResponse{Items: []dto.HomeWidgetItem{
				{Fields: []dto.HomeWidgetValue{{Label: "Name", Value: "web"}, {Label: "State", Value: "up"}}},
				{Fields: []dto.HomeWidgetValue{{Label: "Name", Value: "db"}}},
			}},
			want: []string{`sv-home-svc-list`, `title="Name">web</span>`, `title="State">up</span>`, `title="Name">db</span>`},
		},
		{
			name:    "list fields",
			service: apiService("s", entity.HomeDisplayList, ""),
			data:    dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "Up", Value: "3"}}},
			want:    []string{`<li class="sv-home-svc-row"><span class="sv-home-svc-label">Up</span>`},
		},
		{
			name: "bar clamps and scales ratios",
			service: apiService("s", entity.HomeDisplayBar, "",
				entity.HomeField{Label: "CPU", Format: entity.HomeFormatPercent},
				entity.HomeField{Label: "Disk"}),
			data: dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{
				{Label: "CPU", Value: "42%", Raw: 0.42},
				{Label: "Disk", Value: "250", Raw: 250.0},
				{Label: "Bad", Value: "n/a", Raw: "n/a"},
				{Label: "Neg", Value: "-5", Raw: "-5"},
			}},
			want: []string{
				`style="--sv-home-bar: 42.0%;"`, `style="--sv-home-bar: 100.0%;"`,
				`<span class="sv-home-svc-value">n/a</span>`, `style="--sv-home-bar: 0.0%;"`,
			},
		},
		{
			name:    "html is inserted as markup",
			service: apiService("s", entity.HomeDisplayStats, ""),
			data:    dto.HomeWidgetDataResponse{HTML: `<b class="x">sanitized</b>`, Fields: []dto.HomeWidgetValue{{Label: "Hidden", Value: "1"}}},
			want:    []string{`<div class="sv-home-svc-html"><b class="x">sanitized</b></div>`},
			notWant: []string{`Hidden`},
		},
		{
			name:    "stale and error",
			service: apiService("s", entity.HomeDisplayStats, ""),
			data:    dto.HomeWidgetDataResponse{Stale: true, Error: "timeout <after 5s>", Fields: []dto.HomeWidgetValue{{Label: "Old", Value: "7"}}},
			want:    []string{`sv-home-svc-stale`, `>stale</span>`, `<p class="sv-home-svc-error" role="status">timeout &lt;after 5s&gt;</p>`, `Old`},
		},
		{
			name:    "text is escaped",
			service: apiService("s", entity.HomeDisplayStats, ""),
			data:    dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "<img src=x onerror=1>", Value: "<script>"}}},
			want:    []string{`&lt;img src=x onerror=1&gt;`, `&lt;script&gt;`},
			notWant: []string{`<script>`, `<img`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := newHomeStore(t, &homeStore{load: homeLayoutWith(tc.service)})
			home.set("s", tc.data)
			_, dom := newServiceApp(t, home)
			got, count := dom.lastRegion("home-svc:s")
			require.Equal(t, 1, count)
			for _, want := range tc.want {
				assert.Contains(t, got, want)
			}
			for _, notWant := range tc.notWant {
				assert.NotContains(t, got, notWant)
			}
		})
	}
}

func TestHomeServiceWidgetBridgeErrorShowsInline(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: homeLayoutWith(apiService("s", entity.HomeDisplayStats, "")), dataErr: errors.New("bridge down")})
	_, dom := newServiceApp(t, home)
	got, _ := dom.lastRegion("home-svc:s")
	assert.Contains(t, got, `bridge down`)
}

// homeTestRefresh is the refresh interval of the refresh loop tests. They
// run in a synctest bubble: time.Sleep advances the fake clock and
// synctest.Wait lets the loop finish its tick.
const homeTestRefresh = 10 * time.Second

func refreshService(id string) entity.HomeService {
	return apiService(id, entity.HomeDisplayStats, homeTestRefresh.String())
}

// tickHome advances the fake clock by one refresh interval and waits for
// the loops to settle.
func tickHome() {
	time.Sleep(homeTestRefresh)
	synctest.Wait()
}

func TestHomeServiceWidgetRefreshSwapsOnlyOnChange(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		home := newHomeStore(t, &homeStore{load: homeLayoutWith(refreshService("s"))})
		first := dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "A", Value: "1"}}, FetchedAt: time.Unix(1, 0)}
		home.set("s", first)
		_, dom := newServiceApp(t, home)

		// Same data, new fetched_at: no swap.
		first.FetchedAt = time.Unix(2, 0)
		home.set("s", first)
		tickHome()
		assert.Equal(t, 2, home.callCount("s"))
		_, count := dom.lastRegion("home-svc:s")
		assert.Equal(t, 1, count)

		home.set("s", dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "A", Value: "2"}}})
		tickHome()
		got, count := dom.lastRegion("home-svc:s")
		assert.Equal(t, 2, count)
		assert.Contains(t, got, `>2</dd>`)
	})
}

func TestHomeServiceWidgetRefreshLoopLifetime(t *testing.T) {
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
				home := newHomeStore(t, &homeStore{load: homeLayoutWith(refreshService("s"))})
				app, _ := newServiceApp(t, home)
				reload := name == "reload replaces the loop"
				stop(app)
				synctest.Wait()
				before := home.callCount("s")

				tickHome()
				// One loop left after a reload, none otherwise. A leaked
				// loop also fails the bubble when the test ends.
				want := before
				if reload {
					want++
				}
				assert.Equal(t, want, home.callCount("s"))
			})
		})
	}
}
