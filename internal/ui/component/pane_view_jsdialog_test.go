package component

import (
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/dumber/internal/ui/layout/mocks"
)

// fakeJSDialogView records calls made by PaneView.
type fakeJSDialogView struct {
	respond       func(bool, string)
	focus         bool
	shown         int
	focusRequests int
	destroyed     bool
	hiddenCleared bool
	dismissedWith []bool
}

func (f *fakeJSDialogView) Widget() *gtk.Widget { return nil }
func (f *fakeJSDialogView) Show(_ port.JSDialogRequest, respond func(bool, string), focus bool, _ float64) {
	f.shown++
	f.respond, f.focus = respond, focus
}
func (f *fakeJSDialogView) Visible() bool { return f.respond != nil }
func (f *fakeJSDialogView) Hide()         {}
func (f *fakeJSDialogView) RequestFocus() { f.focusRequests++ }
func (f *fakeJSDialogView) SetOnHidden(fn func(bool)) {
	if fn == nil {
		f.hiddenCleared = true
	}
}
func (f *fakeJSDialogView) Dismiss(ok bool) {
	f.dismissedWith = append(f.dismissedWith, ok)
	if f.respond != nil {
		r := f.respond
		f.respond = nil
		r(ok, "")
	}
}
func (f *fakeJSDialogView) Destroy() { f.destroyed = true }

func newJSDialogPaneView(t *testing.T, fake *fakeJSDialogView) *PaneView {
	t.Helper()
	overlay := mocks.NewMockOverlayWidget(t)
	factory := mocks.NewMockWidgetFactory(t)
	wrapped := mocks.NewMockWidget(t)
	factory.EXPECT().WrapWidget(mock.Anything).Return(wrapped).Maybe()
	overlay.EXPECT().AddOverlay(wrapped).Maybe()
	overlay.EXPECT().SetClipOverlay(wrapped, true).Maybe()
	overlay.EXPECT().SetMeasureOverlay(wrapped, false).Maybe()
	overlay.EXPECT().RemoveOverlay(wrapped).Maybe()
	pv := &PaneView{
		overlay: overlay,
		factory: factory,
		newJSDialog: func(layout.OverlayWidget) jsDialogView {
			return fake
		},
	}
	return pv
}

func TestPaneViewCleanupAnswersOpenJSDialog(t *testing.T) {
	fake := &fakeJSDialogView{}
	pv := newJSDialogPaneView(t, fake)

	var answers []bool
	require.True(t, pv.ShowJSDialog(port.JSDialogRequest{}, func(ok bool, _ string) { answers = append(answers, ok) }, 1))

	pv.Cleanup()

	assert.Equal(t, []bool{false}, answers, "page must be answered, not left blocked")
	assert.True(t, fake.hiddenCleared, "hidden callback detached before dismissing")
	assert.True(t, fake.destroyed)
	assert.Nil(t, pv.jsDialog)

	// Cleanup again is a no-op.
	pv.Cleanup()
	assert.Equal(t, []bool{false}, answers)
}

func TestPaneViewShowJSDialogFocusFollowsActive(t *testing.T) {
	fake := &fakeJSDialogView{}
	pv := newJSDialogPaneView(t, fake)

	require.True(t, pv.ShowJSDialog(port.JSDialogRequest{}, func(bool, string) {}, 1))
	assert.False(t, fake.focus, "inactive pane's dialog must not steal focus")

	pv.isActive = true
	require.True(t, pv.ShowJSDialog(port.JSDialogRequest{}, func(bool, string) {}, 1))
	assert.True(t, fake.focus)
}

func TestPaneViewSetActiveFocusesPendingJSDialog(t *testing.T) {
	fake := &fakeJSDialogView{}
	pv := newJSDialogPaneView(t, fake)
	border := mocks.NewMockBoxWidget(t)
	border.EXPECT().AddCssClass(activePaneClass).Once()
	pv.borderBox = border
	require.True(t, pv.ShowJSDialog(port.JSDialogRequest{}, func(bool, string) {}, 1))

	pv.SetActive(true)

	assert.Equal(t, 1, fake.focusRequests)
}

func TestPaneViewGrabFocusDefersToVisibleJSDialog(t *testing.T) {
	fake := &fakeJSDialogView{}
	pv := newJSDialogPaneView(t, fake)
	webview := mocks.NewMockWidget(t) // GrabFocus on it would fail the test (no expectation)
	pv.webViewWidget = webview
	border := mocks.NewMockBoxWidget(t)
	border.EXPECT().AddCssClass(activePaneClass).Once()
	pv.borderBox = border
	require.True(t, pv.ShowJSDialog(port.JSDialogRequest{}, func(bool, string) {}, 1))

	pv.SetActive(true)
	require.Equal(t, 1, fake.focusRequests)
	assert.True(t, pv.GrabFocus())

	assert.Equal(t, 2, fake.focusRequests, "dialog focus requested again, WebView not focused")
}

func TestPaneViewGrabFocusFocusesWebViewWithoutDialog(t *testing.T) {
	fake := &fakeJSDialogView{}
	pv := newJSDialogPaneView(t, fake)
	webview := mocks.NewMockWidget(t)
	webview.EXPECT().GrabFocus().Return(true).Once()
	pv.webViewWidget = webview

	assert.True(t, pv.GrabFocus())
}

func TestPaneViewShowJSDialogAfterCleanupFails(t *testing.T) {
	fake := &fakeJSDialogView{}
	pv := newJSDialogPaneView(t, fake)
	pv.Cleanup()

	assert.False(t, pv.ShowJSDialog(port.JSDialogRequest{}, func(bool, string) {}, 1))
	assert.Zero(t, fake.shown)
}
