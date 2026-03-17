package port

import "context"

// TextInputTarget represents a text input field that can receive text insertions.
// Implementations include GTK Entry widgets and WebView text fields.
type TextInputTarget interface {
	// InsertText inserts text at the current cursor position.
	InsertText(ctx context.Context, text string) error

	// DeleteBeforeCursor deletes n characters before the cursor position.
	DeleteBeforeCursor(ctx context.Context, n int) error
}

// Focusable is optionally implemented by text input targets that can restore
// focus after the accent picker closes.
type Focusable interface {
	Focus(ctx context.Context)
}

// GTKEntryInputTarget is a marker interface implemented by text input targets
// that wrap a GTK Entry widget. Used for keyboard routing decisions.
type GTKEntryInputTarget interface {
	TextInputTarget
	IsGTKEntryTarget() bool
}

// FocusedInputProvider tracks which text input target currently has focus.
// This allows the accent picker to insert text into the correct input field.
type FocusedInputProvider interface {
	// GetFocusedInput returns the currently focused text input target.
	// Returns nil if no text input has focus.
	GetFocusedInput() TextInputTarget

	// SetFocusedInput sets the currently focused text input target.
	// Pass nil to clear focus.
	SetFocusedInput(target TextInputTarget)
}
