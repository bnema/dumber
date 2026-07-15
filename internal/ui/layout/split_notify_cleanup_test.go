package layout

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bnema/puregotk/v4/glib"
)

// notifyingPaned only implements the SplitView interactions exercised below.
// Embedding the interface supplies the unused methods.
type notifyingPaned struct {
	PanedWidget
	position int
	notify   func()
}

func (p *notifyingPaned) SetResizeStartChild(bool)             {}
func (p *notifyingPaned) SetResizeEndChild(bool)               {}
func (p *notifyingPaned) SetVisible(bool)                      {}
func (p *notifyingPaned) ConnectNotifyPosition(fn func()) uint { p.notify = fn; return 0 }
func (p *notifyingPaned) GetAllocatedWidth() int               { return 100 }
func (p *notifyingPaned) GetPosition() int                     { return p.position }
func (p *notifyingPaned) SetPosition(position int)             { p.position = position }

type notifyingFactory struct {
	WidgetFactory
	paned PanedWidget
}

func (f notifyingFactory) NewPaned(Orientation) PanedWidget { return f.paned }

func newNotifyingSplitView(t *testing.T) (*SplitView, func(int)) {
	t.Helper()

	paned := &notifyingPaned{}
	sv := NewSplitView(context.Background(), notifyingFactory{paned: paned}, OrientationHorizontal, nil, nil, 0)
	sv.mu.Lock()
	sv.suppressNotifyUntil = time.Time{}
	sv.mu.Unlock()
	require.NotNil(t, paned.notify)
	return sv, func(nextPosition int) {
		paned.position = nextPosition
		paned.notify()
	}
}

func receiveQueuedIdle(t *testing.T, queued <-chan *glib.SourceFunc) *glib.SourceFunc {
	t.Helper()
	select {
	case source := <-queued:
		return source
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounce to queue idle callback")
		return nil
	}
}

func TestSplitViewQueuedRatioCallbackDoesNotRunAfterCleanup(t *testing.T) {
	queued := make(chan *glib.SourceFunc, 1)
	previousIdleAdd := idleAdd
	idleAdd = func(source *glib.SourceFunc, _ uintptr) uint {
		queued <- source
		return 1
	}
	t.Cleanup(func() { idleAdd = previousIdleAdd })

	sv, notify := newNotifyingSplitView(t)
	var calls atomic.Int32
	sv.SetOnRatioChanged(func(float64) { calls.Add(1) })

	notify(25)
	source := receiveQueuedIdle(t, queued) // Debounce has fired and queued this work.
	sv.Cleanup()
	(*source)(0) // Run the queued GLib closure after cleanup.

	require.Zero(t, calls.Load(), "cleanup must invalidate queued ratio callbacks")
}

func TestSplitViewSupersededRatioCallbackDeliversOnlyLatestValueOnce(t *testing.T) {
	queued := make(chan *glib.SourceFunc, 2)
	previousIdleAdd := idleAdd
	idleAdd = func(source *glib.SourceFunc, _ uintptr) uint {
		queued <- source
		return 1
	}
	t.Cleanup(func() { idleAdd = previousIdleAdd })

	sv, notify := newNotifyingSplitView(t)
	var ratios []float64
	sv.SetOnRatioChanged(func(ratio float64) { ratios = append(ratios, ratio) })

	notify(20)
	first := receiveQueuedIdle(t, queued)
	notify(80)
	second := receiveQueuedIdle(t, queued)
	(*first)(0)
	(*second)(0)

	require.Equal(t, []float64{0.8}, ratios)
}
