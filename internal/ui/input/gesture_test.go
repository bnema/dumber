package input

import (
	"context"
	"testing"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/assert"
)

type testGestureNavigator struct{}

func (testGestureNavigator) GoBackDirect()    {}
func (testGestureNavigator) GoForwardDirect() {}

func TestGestureHandlerAttachClearsExistingAttachmentState(t *testing.T) {
	h := NewGestureHandler(context.Background())
	h.pressedCb = func(gtk.GestureClick, int, float64, float64) {}
	h.destroyCb = func(gtk.Widget) {}
	h.pressedHandlerID = 1
	h.destroyHandlerID = 2
	h.onAction = func(context.Context, Action) error { return nil }
	h.navigator = testGestureNavigator{}

	h.detachExistingAttachment()

	assert.Nil(t, h.pressedCb)
	assert.Nil(t, h.destroyCb)
	assert.Zero(t, h.pressedHandlerID)
	assert.Zero(t, h.destroyHandlerID)
	assert.Nil(t, h.onAction)
	assert.Nil(t, h.navigator)
}

func TestGestureHandlerDetachClearsRetainedCallbacksAndState(t *testing.T) {
	h := NewGestureHandler(context.Background())
	h.pressedCb = func(gtk.GestureClick, int, float64, float64) {}
	h.destroyCb = func(gtk.Widget) {}
	h.pressedHandlerID = 1
	h.destroyHandlerID = 2
	h.onAction = func(context.Context, Action) error { return nil }
	h.navigator = testGestureNavigator{}

	h.Detach()

	assert.Nil(t, h.pressedCb)
	assert.Nil(t, h.destroyCb)
	assert.Zero(t, h.pressedHandlerID)
	assert.Zero(t, h.destroyHandlerID)
	assert.Nil(t, h.onAction)
	assert.Nil(t, h.navigator)
}

func TestGestureHandlerDetachForDestroyClearsRetainedCallbacksAndState(t *testing.T) {
	h := NewGestureHandler(context.Background())
	h.pressedCb = func(gtk.GestureClick, int, float64, float64) {}
	h.destroyCb = func(gtk.Widget) {}
	h.pressedHandlerID = 1
	h.destroyHandlerID = 2
	h.onAction = func(context.Context, Action) error { return nil }
	h.navigator = testGestureNavigator{}

	h.DetachForDestroy()

	assert.Nil(t, h.pressedCb)
	assert.Nil(t, h.destroyCb)
	assert.Zero(t, h.pressedHandlerID)
	assert.Zero(t, h.destroyHandlerID)
	assert.Nil(t, h.onAction)
	assert.Nil(t, h.navigator)
}
