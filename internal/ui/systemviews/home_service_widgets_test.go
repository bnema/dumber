package systemviews

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// svcHome serves widget data per service id.
type svcHome struct {
	fakeHome
	dataMu sync.Mutex
	data   map[string]dto.HomeWidgetDataResponse
	err    error
	calls  map[string]int
}

func (f *svcHome) HomeWidgetData(_ context.Context, id string) (dto.HomeWidgetDataResponse, error) {
	f.dataMu.Lock()
	defer f.dataMu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[id]++
	if f.err != nil {
		return dto.HomeWidgetDataResponse{}, f.err
	}
	return f.data[id], nil
}

func (f *svcHome) set(id string, resp dto.HomeWidgetDataResponse) {
	f.dataMu.Lock()
	defer f.dataMu.Unlock()
	if f.data == nil {
		f.data = map[string]dto.HomeWidgetDataResponse{}
	}
	f.data[id] = resp
}

func (f *svcHome) callCount(id string) int {
	f.dataMu.Lock()
	defer f.dataMu.Unlock()
	return f.calls[id]
}

// syncRegionDOM is a regionDOM safe for the widget goroutines.
type syncRegionDOM struct {
	mu sync.Mutex
	regionDOM
}

func (d *syncRegionDOM) UpdateRegions(update RegionUpdate) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.regionDOM.UpdateRegions(update)
}

func (d *syncRegionDOM) Mount(markup string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.regionDOM.Mount(markup)
}

// lastRegion returns the last markup pushed into name.
func (d *syncRegionDOM) lastRegion(name string) (string, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	html, count := "", 0
	for _, update := range d.regions {
		for _, region := range update {
			if region.Name == name {
				html = region.HTML
				count++
			}
		}
	}
	return html, count
}

// manualTicker hands out tick channels the test drives.
type manualTicker struct {
	mu      sync.Mutex
	ticks   []chan time.Time
	stopped int
}

func (m *manualTicker) new(time.Duration) (<-chan time.Time, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan time.Time)
	m.ticks = append(m.ticks, ch)
	return ch, func() {
		m.mu.Lock()
		m.stopped++
		m.mu.Unlock()
	}
}

func (m *manualTicker) count() (started, stopped int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ticks), m.stopped
}

func (m *manualTicker) tick(t *testing.T, i int) {
	t.Helper()
	m.mu.Lock()
	ch := m.ticks[i]
	m.mu.Unlock()
	select {
	case ch <- time.Now():
	case <-time.After(2 * time.Second):
		t.Fatal("refresh loop did not take the tick")
	}
}

func apiService(id string, display entity.HomeDisplay, refresh string, fields ...entity.HomeField) entity.HomeService {
	return entity.HomeService{ID: id, Name: id, Widget: &entity.HomeServiceWidget{
		Type: entity.HomeServiceWidgetAPI, Display: display, Refresh: refresh, Fields: fields,
	}}
}

func homeLayoutWith(services ...entity.HomeService) dto.HomeLoadResponse {
	layout := entity.HomeLayout{Groups: []entity.HomeGroup{{Name: "Lab", Services: services}}}
	return dto.HomeLoadResponse{Layout: layout}
}

func newServiceApp(t *testing.T, home *svcHome, ticker *manualTicker) (*App, *syncRegionDOM) {
	t.Helper()
	dom := &syncRegionDOM{}
	app := NewApp(Dependencies{DOM: dom, Home: home, LocationURI: "dumb://home"})
	app.now = func() time.Time { return homeTestNow }
	if ticker != nil {
		app.homeSvc.newTicker = ticker.new
	}
	require.NoError(t, app.LoadInitial(context.Background()))
	fillHome(app)
	t.Cleanup(app.Close)
	return app, dom
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 2*time.Second, 5*time.Millisecond)
}

func TestHomeServiceWidgetSlotIsAnEmptyRegion(t *testing.T) {
	t.Parallel()
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(apiService("nas", entity.HomeDisplayStats, ""), entity.HomeService{ID: "plain", Name: "Plain"})}}
	app, _ := newServiceApp(t, home, nil)
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
			home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(tc.service)}}
			home.set("s", tc.data)
			_, dom := newServiceApp(t, home, nil)
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
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(apiService("s", entity.HomeDisplayStats, ""))}, err: errors.New("bridge down")}
	_, dom := newServiceApp(t, home, nil)
	got, _ := dom.lastRegion("home-svc:s")
	assert.Contains(t, got, `bridge down`)
}

func TestHomeServiceWidgetRefreshSwapsOnlyOnChange(t *testing.T) {
	t.Parallel()
	ticker := &manualTicker{}
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(apiService("s", entity.HomeDisplayStats, "1s"))}}
	first := dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "A", Value: "1"}}, FetchedAt: time.Unix(1, 0)}
	home.set("s", first)
	_, dom := newServiceApp(t, home, ticker)
	waitFor(t, func() bool { started, _ := ticker.count(); return started == 1 })

	// Same data, new fetched_at: no swap.
	first.FetchedAt = time.Unix(2, 0)
	home.set("s", first)
	ticker.tick(t, 0)
	waitFor(t, func() bool { return home.callCount("s") == 2 })
	_, count := dom.lastRegion("home-svc:s")
	assert.Equal(t, 1, count)

	home.set("s", dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "A", Value: "2"}}})
	ticker.tick(t, 0)
	waitFor(t, func() bool { _, count := dom.lastRegion("home-svc:s"); return count == 2 })
	got, _ := dom.lastRegion("home-svc:s")
	assert.Contains(t, got, `>2</dd>`)
}

func TestHomeServiceWidgetTimersClearedOnReloadAndClose(t *testing.T) {
	t.Parallel()
	ticker := &manualTicker{}
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(apiService("s", entity.HomeDisplayStats, "10s"))}}
	app, _ := newServiceApp(t, home, ticker)
	waitFor(t, func() bool { started, _ := ticker.count(); return started == 1 })

	app.lockState()
	require.NoError(t, app.reloadHomeLocked(context.Background()))
	app.unlockState()
	app.homeFillWG.Wait()
	waitFor(t, func() bool { started, stopped := ticker.count(); return started == 2 && stopped == 1 })

	app.Close()
	waitFor(t, func() bool { _, stopped := ticker.count(); return stopped == 2 })
}

func TestHomeServiceWidgetTimersStopOnRouteChange(t *testing.T) {
	t.Parallel()
	ticker := &manualTicker{}
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(apiService("s", entity.HomeDisplayStats, "10s"))}}
	app, _ := newServiceApp(t, home, ticker)
	waitFor(t, func() bool { started, _ := ticker.count(); return started == 1 })

	app.lockState()
	app.resetRouteState()
	app.unlockState()
	waitFor(t, func() bool { _, stopped := ticker.count(); return stopped == 1 })
}

func TestHomeServiceWidgetRestoredAfterRemount(t *testing.T) {
	t.Parallel()
	nas := apiService("nas", entity.HomeDisplayStats, "")
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(nas)}}
	home.set("nas", dto.HomeWidgetDataResponse{Fields: []dto.HomeWidgetValue{{Label: "Disk", Value: "80%"}}})
	app, dom := newServiceApp(t, home, nil)
	_, before := dom.lastRegion("home-svc:nas")
	require.Equal(t, 1, before)
	mounts := dom.mountCount

	// Adding a service is a structural change: the page remounts empty slots.
	home.load = homeLayoutWith(nas, entity.HomeService{ID: "new", Name: "New"})
	app.lockState()
	require.NoError(t, app.reloadHomeLocked(context.Background()))
	app.unlockState()
	app.homeFillWG.Wait()

	assert.Equal(t, mounts+1, dom.mountCount)
	got, after := dom.lastRegion("home-svc:nas")
	assert.Equal(t, 2, after, "slot pushed back after the remount")
	assert.Contains(t, got, `80%`)
}

func TestHomeServiceWidgetCloseWaitsForRefreshLoop(t *testing.T) {
	t.Parallel()
	ticker := &manualTicker{}
	home := &svcHome{fakeHome: fakeHome{load: homeLayoutWith(apiService("s", entity.HomeDisplayStats, "10s"))}}
	app, _ := newServiceApp(t, home, ticker)
	waitFor(t, func() bool { started, _ := ticker.count(); return started == 1 })

	app.Close()
	// No waiting: the loop must have stopped its ticker before Close returned.
	_, stopped := ticker.count()
	assert.Equal(t, 1, stopped)
}
