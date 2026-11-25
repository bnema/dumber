package systemviews

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func actionService(actions ...entity.HomeAction) entity.HomeService {
	return entity.HomeService{ID: "box", Name: "Box", Actions: actions}
}

func newActionApp(t *testing.T, home *homeStore) (*App, *treeDOM) {
	t.Helper()
	dom := newTreeDOM()
	dom.toastOK = true
	app := NewApp(Dependencies{DOM: dom, Home: home.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
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
	home := newHomeStore(t, &homeStore{
		load: homeLayoutWith(actionService(
			entity.HomeAction{ID: "restart", Label: "Restart"},
		)),
	})
	app, _ := newActionApp(t, home)
	assert.Contains(t, app.renderedHTML, `data-sv-region="home-svc-actions:box"`)
	assert.Contains(t, app.renderedHTML, `data-sv-action="home.svc.open" data-sv-service="box" data-sv-home-action="restart"`)
	assert.Contains(t, app.renderedHTML, `>Restart</button>`)
}

func TestHomeServiceActionRunsDirectlyWithBusyState(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		home := newHomeStore(t, &homeStore{
			load:       homeLayoutWith(actionService(entity.HomeAction{ID: "ping", Label: "Ping"})),
			actionResp: dto.HomeActionResponse{Status: dto.HomeStatusOK, Message: "pong"},
			gate:       make(chan struct{}),
		})
		app, dom := newActionApp(t, home)
		ctx := context.Background()

		require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "ping", nil)))
		busy, _ := dom.lastRegion("home-svc-actions:box")
		assert.Contains(t, busy, `aria-busy="true"`)
		assert.Contains(t, busy, `disabled`)
		assert.Contains(t, busy, `Running…`)

		// A second click while busy is ignored.
		require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcOpen, "ping", nil)))
		synctest.Wait()
		require.Len(t, home.actionRequests(), 1)

		close(home.gate)
		synctest.Wait()
		assert.Len(t, home.actionRequests(), 1)
		assert.Equal(t, dto.HomeActionRequest{ServiceID: "box", ActionID: "ping"}, home.actionRequests()[0])
		assert.Equal(t, []string{"Ping: pong"}, dom.toastMessages())
		idle, _ := dom.lastRegion("home-svc-actions:box")
		assert.NotContains(t, idle, `aria-busy`)
		assert.Contains(t, idle, `>Ping</button>`)
	})
}

func TestHomeServiceActionInputsThenConfirm(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{
		load: homeLayoutWith(actionService(entity.HomeAction{
			ID: "scale", Label: "Scale", Confirm: "Scale the service?",
			Inputs: []entity.HomeActionInput{{Name: "replicas", Label: "Replicas", Placeholder: "3"}},
		})),
		actionResp: dto.HomeActionResponse{Status: dto.HomeStatusOK},
	})
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
	assert.Empty(t, home.actionRequests())

	require.NoError(t, app.HandleDOMAction(ctx, svcEvent(homeActionSvcConfirm, "", nil)))
	app.homeFillWG.Wait()
	require.Len(t, home.actionRequests(), 1)
	assert.Equal(t, map[string]string{"replicas": "5"}, home.actionRequests()[0].Inputs)
	assert.Equal(t, []string{"Scale: done"}, dom.toastMessages())
}

func TestHomeServiceActionConfirmOnlyAndCancel(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{
		load: homeLayoutWith(actionService(
			entity.HomeAction{ID: "wipe", Label: "Wipe", Confirm: "Really?"},
		)),
	})
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
	assert.Empty(t, home.actionRequests())
}

func TestHomeServiceActionErrorToast(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{
		load:      homeLayoutWith(actionService(entity.HomeAction{ID: "ping", Label: "Ping"})),
		actionErr: errors.New("bridge down"),
	})
	app, dom := newActionApp(t, home)
	require.NoError(t, app.HandleDOMAction(context.Background(), svcEvent(homeActionSvcOpen, "ping", nil)))
	app.homeFillWG.Wait()
	assert.Equal(t, []string{"Ping failed: bridge down"}, dom.toastMessages())
}

func TestHomeServiceActionStepsMoveFocus(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{
		load: homeLayoutWith(actionService(entity.HomeAction{
			ID: "say", Label: "Say", Confirm: "Sure?",
			Inputs: []entity.HomeActionInput{{Name: "msg", Label: "Message"}},
		})),
		actionResp: dto.HomeActionResponse{Status: dto.HomeStatusOK},
	})
	dom := newTreeDOM()
	dom.toastOK = true
	app := NewApp(Dependencies{DOM: dom, Home: home.service, LocationURI: "dumb://home"})
	require.NoError(t, app.LoadInitial(context.Background()))
	require.NoError(t, dom.Mount(app.renderedHTML))
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
