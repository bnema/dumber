// Package dialog provides UI dialog implementations for the application layer.
package dialog

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

type permissionPopup interface {
	Show(ctx context.Context, heading, body string, callback func(allowed, persistent bool))
}

type permissionDialogRequest struct {
	ctx       context.Context
	origin    string
	permTypes []entity.PermissionType
	callback  func(result port.PermissionDialogResult)
}

// PermissionDialog implements the port.PermissionDialogPresenter interface.
// It uses a custom PermissionPopup overlay to sidestep the purego ConnectResponse bug
// and match the app's custom UI style.
type PermissionDialog struct {
	popup permissionPopup

	mu     sync.Mutex
	active bool
	queue  []permissionDialogRequest
}

// NewPermissionDialog creates a new permission dialog presenter.
// The popup is created once and reused for each permission request.
func NewPermissionDialog(popup *component.PermissionPopup) *PermissionDialog {
	return &PermissionDialog{
		popup: popup,
	}
}

// ShowPermissionDialog displays a permission request dialog to the user.
func (d *PermissionDialog) ShowPermissionDialog(
	ctx context.Context,
	origin string,
	permTypes []entity.PermissionType,
	callback func(result port.PermissionDialogResult),
) {
	req := permissionDialogRequest{
		ctx:       ctx,
		origin:    origin,
		permTypes: permTypes,
		callback:  callback,
	}

	if !d.enqueueOrStart(req) {
		return
	}

	d.showRequest(req)
}

func (d *PermissionDialog) enqueueOrStart(req permissionDialogRequest) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.active {
		d.queue = append(d.queue, req)
		return false
	}

	d.active = true
	return true
}

func (d *PermissionDialog) showRequest(req permissionDialogRequest) {
	ctx := req.ctx
	origin := req.origin
	permTypes := req.permTypes
	callback := req.callback

	log := logging.FromContext(ctx)

	if d.popup == nil {
		log.Error().Msg("permission popup not available")
		callback(port.PermissionDialogResult{Allowed: false, Persistent: false})
		d.showNextQueuedRequest()
		return
	}

	// Build dialog text
	heading := d.buildHeading(permTypes)
	body := d.buildBody(origin, permTypes)

	d.popup.Show(ctx, heading, body, func(allowed, persistent bool) {
		log.Debug().
			Bool("allowed", allowed).
			Bool("persistent", persistent).
			Msg("permission popup response")
		callback(port.PermissionDialogResult{Allowed: allowed, Persistent: persistent})
		d.showNextQueuedRequest()
	})

	log.Debug().
		Str("origin", origin).
		Strs("types", entity.PermissionTypesToStrings(permTypes)).
		Msg("showing permission popup")
}

func (d *PermissionDialog) showNextQueuedRequest() {
	d.mu.Lock()
	if len(d.queue) == 0 {
		d.active = false
		d.mu.Unlock()
		return
	}

	next := d.queue[0]
	d.queue = d.queue[1:]
	d.mu.Unlock()

	d.showRequest(next)
}

// buildHeading creates the dialog heading based on permission types.
func (d *PermissionDialog) buildHeading(permTypes []entity.PermissionType) string {
	hasMic := false
	hasCam := false
	hasDisplay := false

	for _, pt := range permTypes {
		switch pt {
		case entity.PermissionTypeMicrophone:
			hasMic = true
		case entity.PermissionTypeCamera:
			hasCam = true
		case entity.PermissionTypeDisplay:
			hasDisplay = true
		}
	}

	switch {
	case hasMic && hasCam && hasDisplay:
		return "Allow Microphone, Camera, and Screen Sharing?"
	case hasMic && hasDisplay:
		return "Allow Microphone and Screen Sharing?"
	case hasCam && hasDisplay:
		return "Allow Camera and Screen Sharing?"
	case hasMic && hasCam:
		return "Allow Microphone and Camera?"
	case hasMic:
		return "Allow Microphone Access?"
	case hasCam:
		return "Allow Camera Access?"
	case hasDisplay:
		return "Allow Screen Sharing?"
	default:
		return "Allow Permission?"
	}
}

// buildBody creates the dialog body text.
func (d *PermissionDialog) buildBody(origin string, permTypes []entity.PermissionType) string {
	hasMic := false
	hasCam := false
	hasDisplay := false

	for _, pt := range permTypes {
		switch pt {
		case entity.PermissionTypeMicrophone:
			hasMic = true
		case entity.PermissionTypeCamera:
			hasCam = true
		case entity.PermissionTypeDisplay:
			hasDisplay = true
		}
	}

	var action string
	switch {
	case hasMic && hasCam && hasDisplay:
		action = "access your microphone and camera, and share your screen"
	case hasMic && hasDisplay:
		action = "access your microphone and share your screen"
	case hasCam && hasDisplay:
		action = "access your camera and share your screen"
	case hasMic && hasCam:
		action = "access your microphone and camera"
	case hasMic:
		action = "access your microphone"
	case hasCam:
		action = "access your camera"
	case hasDisplay:
		action = "share your screen"
	default:
		action = "access your device"
	}

	return fmt.Sprintf("%s wants to %s.", origin, action)
}

// Ensure PermissionDialog implements the interface.
var _ port.PermissionDialogPresenter = (*PermissionDialog)(nil)
