// Package config provides validation utilities for configuration values.
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

	if config.DefaultZoom < 0.1 || config.DefaultZoom > 5.0 {
		validationErrors = append(validationErrors, "default_zoom must be between 0.1 and 5.0")
	}

	// Validate default search engine
	if config.DefaultSearchEngine == "" {
		validationErrors = append(validationErrors, "default_search_engine cannot be empty")
	} else if !strings.Contains(config.DefaultSearchEngine, "%s") {
		validationErrors = append(validationErrors, "default_search_engine must contain %s placeholder for the search query")
	}

	// Validate popup behavior
	switch config.Workspace.Popups.Behavior {
	case PopupBehaviorSplit, PopupBehaviorStacked, PopupBehaviorTabbed, PopupBehaviorWindowed:
		// Valid
	default:
		validationErrors = append(validationErrors, fmt.Sprintf("workspace.popups.behavior must be one of: split, stacked, tabbed, windowed (got: %s)", config.Workspace.Popups.Behavior))
	}

	// Validate popup placement for split behavior
	if config.Workspace.Popups.Behavior == PopupBehaviorSplit {
		switch config.Workspace.Popups.Placement {
		case "right", "left", "top", "bottom":
			// Valid
		default:
			validationErrors = append(validationErrors, fmt.Sprintf("workspace.popups.placement must be one of: right, left, top, bottom (got: %s)", config.Workspace.Popups.Placement))
		}
	}

	// Validate blank target behavior
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
	if config.Workspace.Styling.InactiveBorderWidth < 0 {
		validationErrors = append(validationErrors, "workspace.styling.inactive_border_width must be non-negative")
	}
	if config.Workspace.Styling.TransitionDuration < 0 {
		validationErrors = append(validationErrors, "workspace.styling.transition_duration must be non-negative")
	}
	if config.Workspace.Styling.BorderRadius < 0 {
		validationErrors = append(validationErrors, "workspace.styling.border_radius must be non-negative")
	}
	if config.Workspace.Styling.UIScale < 0.5 || config.Workspace.Styling.UIScale > 3.0 {
		validationErrors = append(validationErrors, "workspace.styling.ui_scale must be between 0.5 and 3.0")
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

	// Validate tab bar position
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

	// Validate codec buffer sizes
	if config.CodecPreferences.VideoBufferSizeMB < 0 {
		validationErrors = append(validationErrors, "codec_preferences.video_buffer_size_mb must be non-negative")
	}
	if config.CodecPreferences.QueueBufferTimeSec < 0 {
		validationErrors = append(validationErrors, "codec_preferences.queue_buffer_time_sec must be non-negative")
	}

	// Validate AV1 max resolution
	if config.CodecPreferences.AV1MaxResolution != "" {
		validResolutions := map[string]bool{
			"720p": true, "1080p": true, "1440p": true, "4k": true, "unlimited": true,
		}
		if !validResolutions[config.CodecPreferences.AV1MaxResolution] {
			validationErrors = append(validationErrors, fmt.Sprintf("codec_preferences.av1_max_resolution must be one of: 720p, 1080p, 1440p, 4k, unlimited (got: %s)", config.CodecPreferences.AV1MaxResolution))
		}
	}

	// Validate logging values
	if config.Logging.MaxSize < 0 {
		validationErrors = append(validationErrors, "logging.max_size must be non-negative")
	}
	if config.Logging.MaxBackups < 0 {
		validationErrors = append(validationErrors, "logging.max_backups must be non-negative")
	}
	if config.Logging.MaxAge < 0 {
		validationErrors = append(validationErrors, "logging.max_age must be non-negative")
	}

	// If there are validation errors, return them
	if len(validationErrors) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(validationErrors, "\n  - "))
	}

	return nil
}
