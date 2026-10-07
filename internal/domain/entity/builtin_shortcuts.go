package entity

// BuiltinGlobalShortcuts returns the standard browser shortcuts shared by every
// keymap preset. Config defaults include them so users can rebind or disable
// them; the input layer also falls back to them for actions missing from an
// older config file. An action present with empty keys stays disabled.
func BuiltinGlobalShortcuts() map[string]ActionBinding {
	return map[string]ActionBinding{
		"open-omnibox":      {Keys: []string{"ctrl+l"}, Desc: "Open omnibox"},
		"open-find":         {Keys: []string{"ctrl+f"}, Desc: "Find in page"},
		"find-next":         {Keys: []string{"f3", "ctrl+g"}, Desc: "Find next match"},
		"find-prev":         {Keys: []string{"shift+f3", "ctrl+shift+g"}, Desc: "Find previous match"},
		"reload":            {Keys: []string{"ctrl+r", "f5"}, Desc: "Reload page"},
		"hard-reload":       {Keys: []string{"ctrl+shift+r", "ctrl+f5"}, Desc: "Reload page bypassing cache"},
		"open-devtools":     {Keys: []string{"f12"}, Desc: "Open developer tools"},
		"go-back":           {Keys: []string{"ctrl+arrowleft"}, Desc: "Go back"},
		"go-forward":        {Keys: []string{"ctrl+arrowright"}, Desc: "Go forward"},
		"zoom-in":           {Keys: []string{"ctrl+plus", "ctrl+equal"}, Desc: "Zoom in"},
		"zoom-out":          {Keys: []string{"ctrl+minus"}, Desc: "Zoom out"},
		"zoom-reset":        {Keys: []string{"ctrl+0"}, Desc: "Reset zoom"},
		"quit":              {Keys: []string{"ctrl+q"}, Desc: "Quit"},
		"toggle-fullscreen": {Keys: []string{"f11"}, Desc: "Toggle fullscreen"},
		"copy-url":          {Keys: []string{"ctrl+shift+c"}, Desc: "Copy current URL"},
		"print-page":        {Keys: []string{"ctrl+shift+p"}, Desc: "Print page"},
		"session-manager":   {Keys: []string{"ctrl+shift+s"}, Desc: "Open session manager"},
		"focus-left":        {Keys: []string{"alt+h", "alt+arrowleft"}, Desc: "Focus pane to the left"},
		"focus-right":       {Keys: []string{"alt+l", "alt+arrowright"}, Desc: "Focus pane to the right"},
		"focus-up":          {Keys: []string{"alt+k", "alt+arrowup"}, Desc: "Focus pane above"},
		"focus-down":        {Keys: []string{"alt+j", "alt+arrowdown"}, Desc: "Focus pane below"},
	}
}
