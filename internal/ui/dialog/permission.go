// Package dialog provides UI dialog implementations for the application layer.
package dialog

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

// PermissionDialog implements the port.PermissionDialogPresenter interface.
// It uses a custom PermissionPopup overlay to sidestep the purego ConnectResponse bug
// and match the app's custom UI style.
type PermissionDialog struct {
	popup *component.PermissionPopup
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
	log := logging.FromContext(ctx)

	if d.popup == nil {
		log.Error().Msg("permission popup not available")
		callback(port.PermissionDialogResult{Allowed: false, Persistent: false})
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
	})

	log.Debug().
		Str("origin", origin).
		Strs("types", entity.PermissionTypesToStrings(permTypes)).
		Msg("showing permission popup")
}

// buildHeading creates the dialog heading based on permission types.
func (d *PermissionDialog) buildHeading(permTypes []entity.PermissionType) string {
	hasMic := false
	hasCam := false

	for _, pt := range permTypes {
		switch pt {
		case entity.PermissionTypeMicrophone:
			hasMic = true
		case entity.PermissionTypeCamera:
			hasCam = true
		}
	}

	switch {
	case hasMic && hasCam:
		return "Allow Microphone and Camera?"
	case hasMic:
		return "Allow Microphone Access?"
	case hasCam:
		return "Allow Camera Access?"
	default:
		return "Allow Permission?"
	}
}

// buildBody creates the dialog body text.
func (d *PermissionDialog) buildBody(origin string, permTypes []entity.PermissionType) string {
	hasMic := false
	hasCam := false

	for _, pt := range permTypes {
		switch pt {
		case entity.PermissionTypeMicrophone:
			hasMic = true
		case entity.PermissionTypeCamera:
			hasCam = true
		}
	}

	var action string
	switch {
	case hasMic && hasCam:
		action = "access your microphone and camera"
	case hasMic:
		action = "access your microphone"
	case hasCam:
		action = "access your camera"
	default:
		action = "access your device"
	}

	return fmt.Sprintf("%s wants to %s.", origin, action)
}

// Ensure PermissionDialog implements the interface.
var _ port.PermissionDialogPresenter = (*PermissionDialog)(nil)
