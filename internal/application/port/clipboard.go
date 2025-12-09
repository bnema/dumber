package port

import "context"

// Clipboard defines the port interface for clipboard operations.
// This abstracts platform-specific clipboard implementations (GTK, etc.).
type Clipboard interface {
	// WriteText copies text to the clipboard.
	WriteText(ctx context.Context, text string) error

	// ReadText reads text from the clipboard.
	// Returns empty string if clipboard is empty or contains non-text data.
	ReadText(ctx context.Context) (string, error)

	// Clear clears the clipboard contents.
	Clear(ctx context.Context) error

	// HasText returns true if the clipboard contains text data.
	HasText(ctx context.Context) (bool, error)
}
