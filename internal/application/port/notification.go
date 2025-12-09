package port

import "context"

// NotificationType indicates the visual style of a notification.
type NotificationType int

const (
	// NotificationInfo is for informational messages.
	NotificationInfo NotificationType = iota
	// NotificationSuccess is for success confirmations.
	NotificationSuccess
	// NotificationError is for error messages.
	NotificationError
	// NotificationWarning is for warning messages.
	NotificationWarning
)

// String returns a human-readable representation of the notification type.
func (t NotificationType) String() string {
	switch t {
	case NotificationInfo:
		return "info"
	case NotificationSuccess:
		return "success"
	case NotificationError:
		return "error"
	case NotificationWarning:
		return "warning"
	default:
		return "info"
	}
}

// NotificationID uniquely identifies a displayed notification.
type NotificationID string

// Notification represents the port interface for user notifications/toasts.
// This abstracts the presentation layer (GTK/libadwaita toasts, frontend overlay, etc.).
type Notification interface {
	// Show displays a notification with the given message and type.
	// Duration is in milliseconds; pass 0 for default duration.
	// Returns an ID that can be used to dismiss the notification.
	Show(ctx context.Context, message string, notifType NotificationType, durationMs int) NotificationID

	// ShowZoom displays a special zoom level notification.
	// These typically appear in a fixed position and auto-dismiss.
	ShowZoom(ctx context.Context, zoomPercent int) NotificationID

	// Dismiss removes a specific notification by ID.
	Dismiss(ctx context.Context, id NotificationID)

	// Clear removes all displayed notifications.
	Clear(ctx context.Context)
}
