package component

import "strings"

// ModalAppearance carries the current runtime appearance for a modal.
type ModalAppearance struct {
	UIScale float64
	Style   string
}

// modalStyleClass follows the omnibox's default and class naming convention.
func modalStyleClass(style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		style = "command"
	}
	return "omnibox-style-" + style
}
