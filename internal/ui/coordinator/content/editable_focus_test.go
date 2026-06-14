package content

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestEditableFocusCallback_ForwardsPaneID(t *testing.T) {
	c := newMinimalCoordinator()
	wv := mocks.NewMockWebView(t)

	var callbacks *port.WebViewCallbacks
	wv.EXPECT().Generation().Return(uint64(0)).Once()
	wv.EXPECT().SetCallbacks(mock.Anything).Run(func(cb *port.WebViewCallbacks) {
		callbacks = cb
	}).Once()

	var gotPaneID entity.PaneID
	var gotEditable bool
	c.SetOnEditableFocusChanged(func(paneID entity.PaneID, editable bool) {
		gotPaneID = paneID
		gotEditable = editable
	})

	c.setupWebViewCallbacks(context.Background(), entity.PaneID("pane-1"), wv)
	require.NotNil(t, callbacks)
	require.NotNil(t, callbacks.OnEditableFocusChanged)

	callbacks.OnEditableFocusChanged(true)

	assert.Equal(t, entity.PaneID("pane-1"), gotPaneID)
	assert.True(t, gotEditable)
}

func TestEditableFocusCallback_BackgroundPaneDoesNotConfusePaneOwnership(t *testing.T) {
	c := newMinimalCoordinator()
	c.SetActivePaneOverride(entity.PaneID("pane-a"))

	wvA := mocks.NewMockWebView(t)
	wvB := mocks.NewMockWebView(t)

	var callbacksA *port.WebViewCallbacks
	var callbacksB *port.WebViewCallbacks
	wvA.EXPECT().Generation().Return(uint64(0)).Once()
	wvA.EXPECT().SetCallbacks(mock.Anything).Run(func(cb *port.WebViewCallbacks) {
		callbacksA = cb
	}).Once()
	wvB.EXPECT().Generation().Return(uint64(0)).Once()
	wvB.EXPECT().SetCallbacks(mock.Anything).Run(func(cb *port.WebViewCallbacks) {
		callbacksB = cb
	}).Once()

	type event struct {
		paneID   entity.PaneID
		editable bool
	}
	var events []event
	c.SetOnEditableFocusChanged(func(paneID entity.PaneID, editable bool) {
		events = append(events, event{paneID: paneID, editable: editable})
	})

	c.setupWebViewCallbacks(context.Background(), entity.PaneID("pane-a"), wvA)
	c.setupWebViewCallbacks(context.Background(), entity.PaneID("pane-b"), wvB)
	require.NotNil(t, callbacksA)
	require.NotNil(t, callbacksB)

	callbacksB.OnEditableFocusChanged(true)
	callbacksA.OnEditableFocusChanged(false)

	require.Len(t, events, 2)
	assert.Equal(t, event{paneID: entity.PaneID("pane-b"), editable: true}, events[0])
	assert.Equal(t, event{paneID: entity.PaneID("pane-a"), editable: false}, events[1])
}
