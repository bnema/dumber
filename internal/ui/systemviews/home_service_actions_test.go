package systemviews

import (
	"context"
	"errors"
	"sync"
	"testing"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// actionHome records HomeAction calls; gate, when set, holds each call.
type actionHome struct {
	svcHome
	mu   sync.Mutex
	reqs []dto.HomeActionRequest
	resp dto.HomeActionResponse
	err  error
	gate chan struct{}
}

func (f *actionHome) HomeAction(_ context.Context, req dto.HomeActionRequest) (dto.HomeActionResponse, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	gate := f.gate
	f.mu.Unlock()
	if gate != nil {
		<-gate
	}
	return f.resp, f.err
}

func (f *actionHome) requests() []dto.HomeActionRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]dto.HomeActionRequest(nil), f.reqs...)
}

// toastRegionDOM is a goroutine-safe region DOM that records toasts.
type toastRegionDOM struct {
	syncRegionDOM
	toasts []ToastRequest
}

func (d *toastRegionDOM) ShowToast(request ToastRequest) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.toasts = append(d.toasts, request)
	return true
}

func (d *toastRegionDOM) toastMessages() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	messages := make([]string, 0, len(d.toasts))
	for _, toast := range d.toasts {
		messages = append(messages, toast.Message)
	}
	return messages
}

func actionService(actions ...entity.HomeAction) entity.HomeService {
	return entity.HomeService{ID: "box", Name: "Box", Actions: actions}
}

func newActionApp(t *testing.T, home *actionHome) (*App, *toastRegionDOM) {
	t.Helper()
	dom := &toastRegionDOM{}
	app := NewApp(Dependencies{DOM: dom, Home: home, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	fillHome(app)
	t.Cleanup(app.Close)
	return app, dom
}

func svcEvent(action, actionID string, extra map[string]string) DOMAction {
	data := map[string]string{"svService": "box", "svHomeAction": actionID}
	for k, v := range extra {
		data[k] = v
	}
	return DOMAction{Action: action, Data: data}
}

func TestHomeServiceActionsRenderButtons(t *testing.T) {
	t.Parallel()
	home := &actionHome{svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(actionService(
		entity.HomeAction{ID: "restart", Label: "Restart"},
	))}}}
	app, _ := newActionApp(t, home)
	assert.Contains(t, app.renderedHTML, `data-sv-region="home-svc-actions:box"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="home.svc.open" data-sv-service="box" data-sv-home-action="restart"`)
	assert.Contains(t, app.renderedHTML, `>Restart</button>`)
}

func TestHomeServiceActionRunsDirectlyWithBusyState(t *testing.T) {
	t.Parallel()
	home := &actionHome{
		svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(actionService(entity.HomeAction{ID: "ping", Label: "Ping"}))}},
		resp:    dto.HomeActionResponse{Status: dto.HomeStatusOK, Message: "pong"},
		gate:    make(chan struct{}),
	}
	app, dom := newActionApp(t, home)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "ping", nil)))
	busy, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, busy, `aria-busy="true"`)
	assert.Contains(t, busy, `disabled`)
	assert.Contains(t, busy, `Running…`)

	// A second click while busy is ignored.
	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "ping", nil)))
	waitFor(t, func() bool { return len(home.requests()) == 1 })

	close(home.gate)
	app.homeFillWG.Wait()
	assert.Len(t, home.requests(), 1)
	assert.Equal(t, dto.HomeActionRequest{ServiceID: "box", ActionID: "ping"}, home.requests()[0])
	assert.Equal(t, []string{"Ping: pong"}, dom.toastMessages())
	idle, _ := dom.lastRegion("home-svc-actions:box")
	assert.NotContains(t, idle, `aria-busy`)
	assert.Contains(t, idle, `>Ping</button>`)
}

func TestHomeServiceActionInputsThenConfirm(t *testing.T) {
	t.Parallel()
	home := &actionHome{
		svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(actionService(entity.HomeAction{
			ID: "scale", Label: "Scale", Confirm: "Scale the service?",
			Inputs: []entity.HomeActionInput{{Name: "replicas", Label: "Replicas", Placeholder: "3"}},
		}))}},
		resp: dto.HomeActionResponse{Status: dto.HomeStatusOK},
	}
	app, dom := newActionApp(t, home)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "scale", nil)))
	form, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, form, `<form class="sv-home-svc-form" data-sv-action="home.svc.submit" data-sv-service="box" data-sv-home-action="scale"`)
	assert.Contains(t, form, `name="input.replicas" placeholder="3"`)
	assert.Contains(t, form, `Replicas`)

	// Input values arrive under the input. prefix, apart from the routing keys.
	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcSubmit, "scale", map[string]string{"input.replicas": " 5 "})))
	confirm, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, confirm, `Scale the service?`)
	assert.Contains(t, confirm, `data-sv-action="home.svc.confirm"`)
	assert.Empty(t, home.requests())

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcConfirm, "", nil)))
	app.homeFillWG.Wait()
	require.Len(t, home.requests(), 1)
	assert.Equal(t, map[string]string{"replicas": "5"}, home.requests()[0].Inputs)
	assert.Equal(t, []string{"Scale: done"}, dom.toastMessages())
}

func TestHomeServiceActionConfirmOnlyAndCancel(t *testing.T) {
	t.Parallel()
	home := &actionHome{svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(actionService(
		entity.HomeAction{ID: "wipe", Label: "Wipe", Confirm: "Really?"},
	))}}}
	app, dom := newActionApp(t, home)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "wipe", nil)))
	confirm, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, confirm, `Really?`)

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcCancel, "", nil)))
	idle, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, idle, `>Wipe</button>`)

	// A stray confirm after cancel runs nothing.
	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcConfirm, "", nil)))
	app.homeFillWG.Wait()
	assert.Empty(t, home.requests())
}

func TestHomeServiceActionErrorToast(t *testing.T) {
	t.Parallel()
	home := &actionHome{
		svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(actionService(entity.HomeAction{ID: "ping", Label: "Ping"}))}},
		err:     errors.New("bridge down"),
	}
	app, dom := newActionApp(t, home)
	require.NoError(t, app.HandleDOMAction(context.Background(), svcEvent(homeActionSvcOpen, "ping", nil)))
	app.homeFillWG.Wait()
	assert.Equal(t, []string{"Ping failed: bridge down"}, dom.toastMessages())
}

func TestHomeServiceActionFormSurvivesGroupsSwap(t *testing.T) {
	t.Parallel()
	svc := actionService(entity.HomeAction{ID: "scale", Label: "Scale", Inputs: []entity.HomeActionInput{{Name: "n"}}})
	home := &actionHome{svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(svc)}}}
	app, dom := newActionApp(t, home)
	require.NoError(t, app.HandleDOMAction(context.Background(), svcEvent(homeActionSvcOpen, "scale", nil)))
	_, before := dom.lastRegion("home-svc-actions:box")

	home.load = homeLayoutWith(svc, entity.HomeService{ID: "other", Name: "Other"})
	app.lockState()
	require.NoError(t, app.reloadHomeLocked(context.Background()))
	app.unlockState()
	app.homeFillWG.Wait()

	form, after := dom.lastRegion("home-svc-actions:box")
	assert.Equal(t, before+1, after)
	assert.Contains(t, form, `name="input.n"`)
}

// focusRegionDOM records the focus key of each region update.
type focusRegionDOM struct {
	toastRegionDOM
	focuses []string
}

func (d *focusRegionDOM) UpdateRegions(update RegionUpdate) error {
	d.mu.Lock()
	d.focuses = append(d.focuses, update.Focus)
	d.mu.Unlock()
	return d.toastRegionDOM.UpdateRegions(update)
}

func (d *focusRegionDOM) lastFocus() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.focuses) == 0 {
		return ""
	}
	return d.focuses[len(d.focuses)-1]
}

func TestHomeServiceActionStepsMoveFocus(t *testing.T) {
	t.Parallel()
	home := &actionHome{
		svcHome: svcHome{fakeHome: fakeHome{load: homeLayoutWith(actionService(entity.HomeAction{
			ID: "say", Label: "Say", Confirm: "Sure?",
			Inputs: []entity.HomeActionInput{{Name: "msg", Label: "Message"}},
		}))}},
		resp: dto.HomeActionResponse{Status: dto.HomeStatusOK},
	}
	dom := &focusRegionDOM{}
	app := NewApp(Dependencies{DOM: dom, Home: home, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	fillHome(app)
	t.Cleanup(app.Close)
	ctx := context.Background()
	region := `[data-sv-region="home-svc-actions:box"]`

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "say", nil)))
	assert.Equal(t, "sel:"+region+" form input", dom.lastFocus())
	form, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, form, `<form class="sv-home-svc-form"`)
	assert.Contains(t, form, `<input class="sv-input"`)

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcCancel, "", nil)))
	assert.Equal(t, "sel:"+region+` [data-sv-home-action="say"]`, dom.lastFocus())

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "say", nil)))
	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcSubmit, "say", map[string]string{"input.msg": "hi"})))
	assert.Equal(t, "sel:"+region+` [data-sv-action="home.svc.confirm"]`, dom.lastFocus())
	confirm, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, confirm, `data-sv-action="home.svc.confirm"`)

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcConfirm, "", nil)))
	app.homeFillWG.Wait()
	assert.Equal(t, "sel:"+region+` [data-sv-home-action="say"]`, dom.lastFocus())
	idle, _ := dom.lastRegion("home-svc-actions:box")
	assert.Contains(t, idle, `data-sv-home-action="say"`)
}
