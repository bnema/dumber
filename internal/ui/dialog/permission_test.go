package dialog

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

type fakePermissionPopup struct {
	showCalls []fakePopupShowCall
	callback  func(allowed, persistent bool)
}

type fakePopupShowCall struct {
	heading string
	body    string
}

func (f *fakePermissionPopup) Show(
	_ context.Context,
	heading string,
	body string,
	callback func(allowed, persistent bool),
) {
	f.showCalls = append(f.showCalls, fakePopupShowCall{
		heading: heading,
		body:    body,
	})
	f.callback = callback
}

func (f *fakePermissionPopup) Respond(allowed, persistent bool) {
	if f.callback == nil {
		return
	}
	cb := f.callback
	f.callback = nil
	cb(allowed, persistent)
}

func TestPermissionDialog_QueuesRequestsWhilePopupVisible(t *testing.T) {
	popup := &fakePermissionPopup{}
	d := &PermissionDialog{popup: popup}

	var firstResult port.PermissionDialogResult
	firstCalled := false
	d.ShowPermissionDialog(
		context.Background(),
		"https://example.com",
		[]entity.PermissionType{entity.PermissionTypeMicrophone},
		func(result port.PermissionDialogResult) {
			firstCalled = true
			firstResult = result
		},
	)

	secondCalled := false
	var secondResult port.PermissionDialogResult
	d.ShowPermissionDialog(
		context.Background(),
		"https://example.com",
		[]entity.PermissionType{entity.PermissionTypeCamera},
		func(result port.PermissionDialogResult) {
			secondCalled = true
			secondResult = result
		},
	)

	if assert.Len(t, popup.showCalls, 1) {
		assert.Equal(t, "Allow Microphone Access?", popup.showCalls[0].heading)
	}
	assert.False(t, firstCalled)
	assert.False(t, secondCalled)

	popup.Respond(true, false)
	assert.True(t, firstCalled)
	assert.Equal(t, port.PermissionDialogResult{Allowed: true, Persistent: false}, firstResult)

	if assert.Len(t, popup.showCalls, 2) {
		assert.Equal(t, "Allow Camera Access?", popup.showCalls[1].heading)
	}
	assert.False(t, secondCalled)

	popup.Respond(false, true)
	assert.True(t, secondCalled)
	assert.Equal(t, port.PermissionDialogResult{Allowed: false, Persistent: true}, secondResult)
}

func TestPermissionDialog_NoPopup_DeniesRequest(t *testing.T) {
	d := &PermissionDialog{popup: nil}

	called := false
	result := port.PermissionDialogResult{}
	d.ShowPermissionDialog(
		context.Background(),
		"https://example.com",
		[]entity.PermissionType{entity.PermissionTypeMicrophone},
		func(r port.PermissionDialogResult) {
			called = true
			result = r
		},
	)

	assert.True(t, called)
	assert.Equal(t, port.PermissionDialogResult{Allowed: false, Persistent: false}, result)
}
