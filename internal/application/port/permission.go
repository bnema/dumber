package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// PermissionDialogResult represents the user's response from a permission dialog.
type PermissionDialogResult struct {
	// Allowed is true if the user clicked "Allow" or "Always Allow".
	Allowed bool

	// Persistent is true if the user clicked "Always Allow" (save this decision).
	// This should only be true for mic/camera permissions, not display capture.
	Persistent bool
}

// PermissionDialogPresenter defines the interface for showing permission dialogs.
// This is implemented by the UI layer (Adwaita dialogs).
type PermissionDialogPresenter interface {
	// ShowPermissionDialog displays a permission request dialog to the user.
	// The callback is invoked with the user's decision.
	//
	// Parameters:
	//   - ctx: context for cancellation and logging
	//   - origin: the website origin requesting permission (e.g., "https://meet.example.com")
	//   - permTypes: the types of permissions being requested
	//   - callback: invoked when the user makes a decision or dialog is dismissed
	ShowPermissionDialog(
		ctx context.Context,
		origin string,
		permTypes []entity.PermissionType,
		callback func(result PermissionDialogResult),
	)
}
