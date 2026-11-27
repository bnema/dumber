package content

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/ui/input"
	"bnema.dev/puregotk/v4/gtk"
)

// fakeGestureHandle is hand-written rather than a mockery mock: gestureHandle
// is a package-private seam, and the fake must mirror input.GestureHandler's
// stateful behavior (AttachTo clears the navigator and action callbacks,
// AttachedTo reflects the last attached native widget).
type fakeGestureHandle struct {
	attaches  int
	detaches  int
	attached  uintptr
	navigator input.DirectNavigator
	onAction  input.ActionHandler
}

func (f *fakeGestureHandle) AttachTo(w *gtk.Widget) {
	f.attaches++
	f.attached = w.Ptr
	f.navigator = nil
	f.onAction = nil
}

func (f *fakeGestureHandle) AttachedTo(w *gtk.Widget) bool {
	return w != nil && w.Ptr != 0 && f.attached == w.Ptr
}

func (f *fakeGestureHandle) Detach() {
	f.detaches++
	f.attached = 0
}
func (f *fakeGestureHandle) SetNavigator(nav input.DirectNavigator) { f.navigator = nav }
func (f *fakeGestureHandle) SetOnAction(fn input.ActionHandler)     { f.onAction = fn }

type fakeNavigator struct{ name string }

func (fakeNavigator) GoBackDirect()    {}
func (fakeNavigator) GoForwardDirect() {}

func newFakeHandleFactory(created *[]*fakeGestureHandle) func() gestureHandle {
	return func() gestureHandle {
		h := &fakeGestureHandle{}
		*created = append(*created, h)
		return h
	}
}

// fakeDestroySignals replaces the GTK destroy signal on a registry.
type fakeDestroySignals struct {
	nextID       uint
	connected    map[uint]*func(gtk.Widget)
	disconnected []uint
}

func stubDestroySignals(r *gestureRegistry) *fakeDestroySignals {
	f := &fakeDestroySignals{connected: make(map[uint]*func(gtk.Widget))}
	r.connectDestroy = func(_ *gtk.Widget, cb *func(gtk.Widget)) uint {
		f.nextID++
		f.connected[f.nextID] = cb
		return f.nextID
	}
	r.disconnectDestroy = func(_ *gtk.Widget, id uint) {
		delete(f.connected, id)
		f.disconnected = append(f.disconnected, id)
	}
	return f
}

// destroy emits the destroy signal for a connected handler ID.
func (f *fakeDestroySignals) destroy(t *testing.T, id uint) {
	t.Helper()
	cb, ok := f.connected[id]
	require.True(t, ok, "destroy handler %d is not connected", id)
	(*cb)(gtk.Widget{})
}

func widgetAt(ptr uintptr) *gtk.Widget { return gtk.WidgetNewFromInternalPtr(ptr) }

func TestGestureRegistry_RepeatedBindSameWidgetAttachesOnce(t *testing.T) {
	t.Parallel()

	var r gestureRegistry
	stubDestroySignals(&r)
	var created []*fakeGestureHandle
	navA, navB := fakeNavigator{"a"}, fakeNavigator{"b"}

	// Each wrap of the same native widget yields a new *gtk.Widget.
	assert.True(t, r.bind(1, widgetAt(0x100), navA, nil, newFakeHandleFactory(&created)))
	assert.False(t, r.bind(1, widgetAt(0x100), navB, nil, newFakeHandleFactory(&created)))
	assert.False(t, r.bind(1, widgetAt(0x100), navB, nil, newFakeHandleFactory(&created)))

	require.Len(t, created, 1)
	assert.Equal(t, 1, created[0].attaches)
	assert.Equal(t, navB, created[0].navigator)
}

func TestGestureRegistry_NewWidgetReattachesSameHandler(t *testing.T) {
	t.Parallel()

	var r gestureRegistry
	signals := stubDestroySignals(&r)
	var created []*fakeGestureHandle
	var actions int
	onAction := func(context.Context, input.Action) error { actions++; return nil }

	assert.True(t, r.bind(1, widgetAt(0x100), nil, onAction, newFakeHandleFactory(&created)))
	assert.True(t, r.bind(1, widgetAt(0x200), nil, onAction, newFakeHandleFactory(&created)))

	require.Len(t, created, 1)
	assert.Equal(t, 2, created[0].attaches)
	require.NotNil(t, created[0].onAction, "callbacks must survive the re-attach")
	require.NoError(t, created[0].onAction(context.Background(), input.ActionGoBack))
	assert.Equal(t, 1, actions)
	// The first widget's destroy handler is disconnected on re-attach.
	assert.Equal(t, []uint{1}, signals.disconnected)
	assert.Len(t, signals.connected, 1)
}

func TestGestureRegistry_SeparateHandlerPerWebView(t *testing.T) {
	t.Parallel()

	var r gestureRegistry
	stubDestroySignals(&r)
	var created []*fakeGestureHandle

	r.bind(1, widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))
	r.bind(2, widgetAt(0x200), nil, nil, newFakeHandleFactory(&created))

	assert.Len(t, created, 2)
}

func TestGestureRegistry_RemoveReturnsHandlerOnce(t *testing.T) {
	t.Parallel()

	var r gestureRegistry
	signals := stubDestroySignals(&r)
	var created []*fakeGestureHandle
	r.bind(1, widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))

	assert.Same(t, created[0], r.remove(1))
	assert.Nil(t, r.remove(1))
	assert.Empty(t, signals.connected, "remove disconnects the destroy handler")

	// A later bind for the same ID creates a fresh handler.
	r.bind(1, widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))
	assert.Len(t, created, 2)
}

func TestGestureRegistry_WidgetDestroyDropsEntry(t *testing.T) {
	t.Parallel()

	var r gestureRegistry
	signals := stubDestroySignals(&r)
	var created []*fakeGestureHandle
	r.bind(1, widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))

	signals.destroy(t, 1)

	assert.Nil(t, r.remove(1), "destroy must drop the entry")
	assert.Zero(t, created[0].detaches, "the handler detaches itself on destroy")
}

func TestGestureRegistry_StaleDestroyKeepsNewerEntry(t *testing.T) {
	t.Parallel()

	var r gestureRegistry
	stubDestroySignals(&r)
	var created []*fakeGestureHandle
	r.bind(1, widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))
	stale := r.bindings[1]
	r.remove(1)
	r.bind(1, widgetAt(0x200), nil, nil, newFakeHandleFactory(&created))

	// A late destroy for the first binding must not drop the new one.
	r.dropDestroyed(1, stale)

	assert.Same(t, created[1], r.remove(1))
}

func TestCoordinator_DeleteWebViewDetachesGestureHandler(t *testing.T) {
	t.Parallel()

	c := newMinimalCoordinator()
	stubDestroySignals(&c.gestures)
	wv := &idOnlyWebView{id: 7}
	c.webViews["pane-1"] = wv
	var created []*fakeGestureHandle
	c.gestures.bind(wv.ID(), widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))

	c.deleteWebViewLocked("pane-1")

	require.Len(t, created, 1)
	assert.Equal(t, 1, created[0].detaches)
	assert.Nil(t, c.gestures.remove(wv.ID()))
}

func TestCoordinator_SetWebViewReplacementDetachesOldGestureHandler(t *testing.T) {
	t.Parallel()

	c := newMinimalCoordinator()
	stubDestroySignals(&c.gestures)
	oldWV := &idOnlyWebView{id: 7}
	newWV := &idOnlyWebView{id: 8}
	c.webViews["pane-1"] = oldWV
	var created []*fakeGestureHandle
	c.gestures.bind(oldWV.ID(), widgetAt(0x100), nil, nil, newFakeHandleFactory(&created))

	// Re-registering the same web view keeps its handler.
	c.setWebViewLocked("pane-1", oldWV)
	assert.Zero(t, created[0].detaches)

	c.setWebViewLocked("pane-1", newWV)
	assert.Equal(t, 1, created[0].detaches)
	assert.Nil(t, c.gestures.remove(oldWV.ID()))
}

// idOnlyWebView is a WebView whose only implemented method is ID.
type idOnlyWebView struct {
	port.WebView
	id port.WebViewID
}

func (w *idOnlyWebView) ID() port.WebViewID { return w.id }
