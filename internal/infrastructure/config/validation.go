package config

import (
	"fmt"
	"strings"
)

// validateConfig performs comprehensive validation of configuration values
func validateConfig(config *Config) error {
	var validationErrors []string

	// Validate numeric ranges
	if config.History.MaxEntries < 0 {
		validationErrors = append(validationErrors, "history.max_entries must be non-negative")
	}
	if config.History.RetentionPeriodDays < 0 {
		validationErrors = append(validationErrors, "history.retention_period_days must be non-negative")
	}
	if config.History.CleanupIntervalDays < 0 {
		validationErrors = append(validationErrors, "history.cleanup_interval_days must be non-negative")
	}

	if config.Dmenu.MaxHistoryItems < 0 {
		validationErrors = append(validationErrors, "dmenu.max_history_items must be non-negative")
	}

	if config.Appearance.DefaultFontSize < 1 || config.Appearance.DefaultFontSize > 72 {
		validationErrors = append(validationErrors, "appearance.default_font_size must be between 1 and 72")
	}

	if config.DefaultWebpageZoom < 0.1 || config.DefaultWebpageZoom > 5.0 {
		validationErrors = append(validationErrors, "default_webpage_zoom must be between 0.1 and 5.0")
	}
	if config.DefaultUIScale < 0.5 || config.DefaultUIScale > 3.0 {
		validationErrors = append(validationErrors, "default_ui_scale must be between 0.5 and 3.0")
	}

	// Validate default search engine
	if config.DefaultSearchEngine == "" {
		validationErrors = append(validationErrors, "default_search_engine cannot be empty")
	} else if !strings.Contains(config.DefaultSearchEngine, "%s") {
		validationErrors = append(validationErrors, "default_search_engine must contain %s placeholder for the search query")
	}

	// Validate popup behavior using switch statement (per CLAUDE.md)
	switch config.Workspace.Popups.Behavior {
	case PopupBehaviorSplit, PopupBehaviorStacked, PopupBehaviorTabbed, PopupBehaviorWindowed:
		// Valid
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("workspace.popups.behavior must be one of: split, stacked, tabbed, windowed (got: %s)", config.Workspace.Popups.Behavior))
	}

	// Validate popup placement for split behavior using switch statement
	if config.Workspace.Popups.Behavior == PopupBehaviorSplit {
		switch config.Workspace.Popups.Placement {
		case "right", "left", "top", "bottom":
			// Valid
		default:
			validationErrors = append(validationErrors, fmt.Sprintf("workspace.popups.placement must be one of: right, left, top, bottom (got: %s)", config.Workspace.Popups.Placement))
		}
	}

	// Validate blank target behavior using switch statement
	switch config.Workspace.Popups.BlankTargetBehavior {
	case "split", "stacked", "tabbed":
		// Valid
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("workspace.popups.blank_target_behavior must be one of: split, stacked, tabbed (got: %s)", config.Workspace.Popups.BlankTargetBehavior))
	}

	// Validate workspace styling values
	if config.Workspace.Styling.BorderWidth < 0 {
		validationErrors = append(validationErrors, "workspace.styling.border_width must be non-negative")
	}
	if config.Workspace.Styling.PaneModeBorderWidth < 0 {
		validationErrors = append(validationErrors, "workspace.styling.pane_mode_border_width must be non-negative")
	}
	if config.Workspace.Styling.TabModeBorderWidth < 0 {
		validationErrors = append(validationErrors, "workspace.styling.tab_mode_border_width must be non-negative")
	}
	if config.Workspace.Styling.TransitionDuration < 0 {
		validationErrors = append(validationErrors, "workspace.styling.transition_duration must be non-negative")
	}

	// Validate pane mode timeout
	if config.Workspace.PaneMode.TimeoutMilliseconds < 0 {
		validationErrors = append(validationErrors, "workspace.pane_mode.timeout_ms must be non-negative")
	}

	// Validate pane mode actions
	if len(config.Workspace.PaneMode.Actions) == 0 {
		validationErrors = append(validationErrors, "workspace.pane_mode.actions cannot be empty")
	}

	// Check for duplicate keys and empty action key lists in pane mode
	seenKeys := make(map[string]string)
	for action, keys := range config.Workspace.PaneMode.Actions {
		if len(keys) == 0 {
			validationErrors = append(validationErrors, fmt.Sprintf("workspace.pane_mode.actions.%s must have at least one key binding", action))
		}
		for _, key := range keys {
			if existingAction, exists := seenKeys[key]; exists {
				validationErrors = append(validationErrors, fmt.Sprintf("duplicate key binding '%s' found in pane_mode actions '%s' and '%s'", key, existingAction, action))
			}
			seenKeys[key] = action
		}
	}

	// Validate tab bar position using switch statement
	switch config.Workspace.TabBarPosition {
	case "top", "bottom":
		// Valid
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("workspace.tab_bar_position must be 'top' or 'bottom' (got: %s)", config.Workspace.TabBarPosition))
	}

	// Validate tab mode timeout
	if config.Workspace.TabMode.TimeoutMilliseconds < 0 {
		validationErrors = append(validationErrors, "workspace.tab_mode.timeout_ms must be non-negative")
	}

	// Validate tab mode actions
	if len(config.Workspace.TabMode.Actions) == 0 {
		validationErrors = append(validationErrors, "workspace.tab_mode.actions cannot be empty")
	}

	// Check for duplicate keys and empty action key lists in tab mode
	tabSeenKeys := make(map[string]string)
	for action, keys := range config.Workspace.TabMode.Actions {
		if len(keys) == 0 {
			validationErrors = append(validationErrors, fmt.Sprintf("workspace.tab_mode.actions.%s must have at least one key binding", action))
		}
		for _, key := range keys {
			if existingAction, exists := tabSeenKeys[key]; exists {
				validationErrors = append(validationErrors, fmt.Sprintf("duplicate key binding '%s' found in tab_mode actions '%s' and '%s'", key, existingAction, action))
			}
			tabSeenKeys[key] = action
		}
	}

	// Validate logging values
	if config.Logging.MaxAge < 0 {
		validationErrors = append(validationErrors, "logging.max_age must be non-negative")
	}

	// Validate logging level using switch statement
	switch config.Logging.Level {
	case "trace", "debug", "info", "warn", "error", "fatal", "":
		// Valid (empty uses default)
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("logging.level must be one of: trace, debug, info, warn, error, fatal (got: %s)", config.Logging.Level))
	}

	// Validate logging format using switch statement
	switch config.Logging.Format {
	case "text", "json", "console", "":
		// Valid (empty uses default)
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("logging.format must be one of: text, json, console (got: %s)", config.Logging.Format))
	}

	// Validate omnibox initial behavior using switch statement
	switch config.Omnibox.InitialBehavior {
	case "recent", "most_visited", "none":
		// Valid
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("omnibox.initial_behavior must be one of: recent, most_visited, none (got: %s)", config.Omnibox.InitialBehavior))
	}

	// Validate rendering mode using switch statement
	switch config.RenderingMode {
	case RenderingModeAuto, RenderingModeGPU, RenderingModeCPU, "":
		// Valid (empty uses default)
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("rendering_mode must be one of: auto, gpu, cpu (got: %s)", config.RenderingMode))
	}

	// Validate color scheme using switch statement
	switch config.Appearance.ColorScheme {
	case "prefer-dark", "prefer-light", ThemeDefault, "":
		// Valid (empty uses default)
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("appearance.color_scheme must be one of: prefer-dark, prefer-light, default (got: %s)", config.Appearance.ColorScheme))
	}

	// If there are validation errors, return them
	if len(validationErrors) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(validationErrors, "\n  - "))
	}

	return nil
}
