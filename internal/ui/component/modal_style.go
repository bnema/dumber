package component

import "strings"

// modalStyleClass follows the omnibox's default and class naming convention.
func modalStyleClass(style string) string {
	style = strings.TrimSpace(style)
	if style == "" {
		style = "command"
	}
	return "omnibox-style-" + style
}
