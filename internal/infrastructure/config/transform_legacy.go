package config

const (
	sectionWorkspace = "workspace"
	sectionSession   = "session"
)

// LegacyConfigTransformer implements port.ConfigTransformer.
// It transforms old-format action bindings (slices) to new ActionBinding format.
type LegacyConfigTransformer struct {
	defaults *Config
}

// NewLegacyConfigTransformer creates a new transformer with defaults for description lookup.
func NewLegacyConfigTransformer() *LegacyConfigTransformer {
	return &LegacyConfigTransformer{defaults: DefaultConfig()}
}

// TransformLegacyActions converts old-format action bindings to new format.
// This handles migration from:
//
//	[workspace.pane_mode.actions]
//	"focus-left" = ["h", "arrowleft"]
//
// To:
//
//	[workspace.pane_mode.actions.focus-left]
//	keys = ["h", "arrowleft"]
//	desc = "Focus pane to the left"
func (t *LegacyConfigTransformer) TransformLegacyActions(rawConfig map[string]any) {
	actionPaths := [][]string{
		{"workspace", "pane_mode", "actions"},
		{"workspace", "tab_mode", "actions"},
		{"workspace", "resize_mode", "actions"},
		{"workspace", "shortcuts", "actions"},
		{"session", "session_mode", "actions"},
	}

	for _, path := range actionPaths {
		t.transformActionSection(rawConfig, path)
	}
}

func (t *LegacyConfigTransformer) transformActionSection(rawConfig map[string]any, path []string) {
	// Navigate to the parent of the actions section
	current := rawConfig
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			return // Path doesn't exist
		}
		current = next
	}

	actionsKey := path[len(path)-1]
	actionsRaw, ok := current[actionsKey]
	if !ok {
		return
	}

	actions, ok := actionsRaw.(map[string]any)
	if !ok {
		return
	}

	// Check each action and transform if needed
	for actionName, actionValue := range actions {
		// If value is a slice (old format), convert to ActionBinding format
		if slice, ok := actionValue.([]any); ok {
			keys := make([]string, 0, len(slice))
			for _, v := range slice {
				if s, ok := v.(string); ok {
					keys = append(keys, s)
				}
			}

			actions[actionName] = map[string]any{
				"keys": keys,
				"desc": t.getDefaultDesc(path, actionName),
			}
		}
	}
}

func (t *LegacyConfigTransformer) getDefaultDesc(path []string, actionName string) string {
	switch {
	case path[0] == sectionWorkspace && path[1] == "pane_mode":
		if b, ok := t.defaults.Workspace.PaneMode.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionWorkspace && path[1] == "tab_mode":
		if b, ok := t.defaults.Workspace.TabMode.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionWorkspace && path[1] == "resize_mode":
		if b, ok := t.defaults.Workspace.ResizeMode.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionWorkspace && path[1] == "shortcuts":
		if b, ok := t.defaults.Workspace.Shortcuts.Actions[actionName]; ok {
			return b.Desc
		}
	case path[0] == sectionSession && path[1] == "session_mode":
		if b, ok := t.defaults.Session.SessionMode.Actions[actionName]; ok {
			return b.Desc
		}
	}
	return ""
}
