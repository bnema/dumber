// Package dialog provides UI dialog implementations for the application layer.
package dialog

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// PermissionDialog implements the port.PermissionDialogPresenter interface.
// It shows Adwaita AlertDialogs for media permission requests.
type PermissionDialog struct {
	// parentWindow is the parent window for modal dialogs (can be nil)
	parentWindow *gtk.ApplicationWindow
}

// NewPermissionDialog creates a new permission dialog presenter.
func NewPermissionDialog(parentWindow *gtk.ApplicationWindow) *PermissionDialog {
	return &PermissionDialog{
		parentWindow: parentWindow,
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

	// Build dialog text
	heading := d.buildHeading(permTypes)
	body := d.buildBody(origin, permTypes)

	// Create alert dialog
	dialog := adw.NewAlertDialog(&heading, &body)
	if dialog == nil {
		log.Error().Msg("failed to create permission dialog")
		// Deny by default if dialog creation fails
		callback(port.PermissionDialogResult{Allowed: false, Persistent: false})
		return
	}

	// Add responses: Allow, Always Allow, Deny, Always Deny
	// Using underscore prefix for mnemonic keyboard shortcuts
	dialog.AddResponse("allow", "_Allow")
	dialog.AddResponse("allow_always", "Always _Allow")
	dialog.AddResponse("deny", "_Deny")
	dialog.AddResponse("deny_always", "Always _Deny")

	// Set appearances
	dialog.SetResponseAppearance("allow", adw.ResponseSuggestedValue)
	dialog.SetResponseAppearance("allow_always", adw.ResponseSuggestedValue)
	dialog.SetResponseAppearance("deny", adw.ResponseDefaultValue)
	dialog.SetResponseAppearance("deny_always", adw.ResponseDestructiveValue)

	// Set default response to Deny (conservative default)
	defaultResponse := "deny"
	dialog.SetDefaultResponse(&defaultResponse)
	dialog.SetCloseResponse("deny")

	// Connect response signal - store callback to prevent GC
	responseCb := func(_ adw.AlertDialog, response string) {
		switch response {
		case "allow":
			callback(port.PermissionDialogResult{Allowed: true, Persistent: false})
		case "allow_always":
			callback(port.PermissionDialogResult{Allowed: true, Persistent: true})
		case "deny_always":
			callback(port.PermissionDialogResult{Allowed: false, Persistent: true})
		default:
			callback(port.PermissionDialogResult{Allowed: false, Persistent: false})
		}
	}
	dialog.ConnectResponse(&responseCb)

	// Present the dialog
	if d.parentWindow != nil {
		// Get the widget representation of the window
		parentWidget := gtk.WidgetNewFromInternalPtr(d.parentWindow.GoPointer())
		dialog.Present(parentWidget)
	} else {
		// Present without parent - GTK will use default window
		dialog.Present(nil)
	}

	log.Debug().
		Str("origin", origin).
		Strs("types", entity.PermissionTypesToStrings(permTypes)).
		Msg("showing permission dialog")
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
