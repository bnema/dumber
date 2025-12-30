package port

// AccentPickerUI defines the interface for displaying an accent selection overlay.
// The picker shows a list of accented characters and allows the user to select one
// via arrow keys, number keys (1-9), or Enter.
type AccentPickerUI interface {
	// Show displays the accent picker with the given accent variants.
	// selectedCallback is called when the user selects an accent.
	// cancelCallback is called when the user cancels (Escape key).
	Show(accents []rune, selectedCallback func(accent rune), cancelCallback func())

	// Hide hides the accent picker.
	Hide()

	// IsVisible returns true if the accent picker is currently visible.
	IsVisible() bool
}
