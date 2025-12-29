package config

import (
	"fmt"
	"net/url"
	"strings"

	domainurl "github.com/bnema/dumber/internal/domain/url"
	domainvalidation "github.com/bnema/dumber/internal/domain/validation"
)

// validateConfig performs comprehensive validation of configuration values
func validateConfig(config *Config) error {
	var validationErrors []string

	validationErrors = append(validationErrors, validateHistory(config)...)
	validationErrors = append(validationErrors, validateDmenu(config)...)
	validationErrors = append(validationErrors, validateAppearance(config)...)
	validationErrors = append(validationErrors, validateSearchEngine(config)...)
	validationErrors = append(validationErrors, validatePopups(config)...)
	validationErrors = append(validationErrors, validateWorkspaceStyling(config)...)
	validationErrors = append(validationErrors, validatePaneMode(config)...)
	validationErrors = append(validationErrors, validateTabBar(config)...)
	validationErrors = append(validationErrors, validateTabMode(config)...)
	validationErrors = append(validationErrors, validateLogging(config)...)
	validationErrors = append(validationErrors, validateWorkspaceNewPaneURL(config)...)
	validationErrors = append(validationErrors, validateOmnibox(config)...)
	validationErrors = append(validationErrors, validateRendering(config)...)
	validationErrors = append(validationErrors, validateColorScheme(config)...)
	validationErrors = append(validationErrors, validateSession(config)...)
	validationErrors = append(validationErrors, validatePerformanceProfile(config)...)

	// If there are validation errors, return them
	if len(validationErrors) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(validationErrors, "\n  - "))
	}

	return nil
}

func validateHistory(config *Config) []string {
	var validationErrors []string
	if config.History.MaxEntries < 0 {
		validationErrors = append(validationErrors, "history.max_entries must be non-negative")
	}
	if config.History.RetentionPeriodDays < 0 {
		validationErrors = append(validationErrors, "history.retention_period_days must be non-negative")
	}
	if config.History.CleanupIntervalDays < 0 {
		validationErrors = append(validationErrors, "history.cleanup_interval_days must be non-negative")
	}
	return validationErrors
}

func validateDmenu(config *Config) []string {
	if config.Dmenu.MaxHistoryDays < 0 {
		return []string{"dmenu.max_history_days must be non-negative"}
	}
	return nil
}

func validateAppearance(config *Config) []string {
	var validationErrors []string
	if config.Appearance.DefaultFontSize < 1 || config.Appearance.DefaultFontSize > 72 {
		validationErrors = append(validationErrors, "appearance.default_font_size must be between 1 and 72")
	}
	validationErrors = append(validationErrors, domainvalidation.ValidateFontFamily("appearance.sans_font", config.Appearance.SansFont)...)
	validationErrors = append(validationErrors, domainvalidation.ValidateFontFamily("appearance.serif_font", config.Appearance.SerifFont)...)
	validationErrors = append(
		validationErrors,
		domainvalidation.ValidateFontFamily("appearance.monospace_font", config.Appearance.MonospaceFont)...,
	)
	validationErrors = append(validationErrors, domainvalidation.ValidatePaletteHex(
		"appearance.light_palette",
		config.Appearance.LightPalette.Background,
		config.Appearance.LightPalette.Surface,
		config.Appearance.LightPalette.SurfaceVariant,
		config.Appearance.LightPalette.Text,
		config.Appearance.LightPalette.Muted,
		config.Appearance.LightPalette.Accent,
		config.Appearance.LightPalette.Border,
	)...)
	validationErrors = append(validationErrors, domainvalidation.ValidatePaletteHex(
		"appearance.dark_palette",
		config.Appearance.DarkPalette.Background,
		config.Appearance.DarkPalette.Surface,
		config.Appearance.DarkPalette.SurfaceVariant,
		config.Appearance.DarkPalette.Text,
		config.Appearance.DarkPalette.Muted,
		config.Appearance.DarkPalette.Accent,
		config.Appearance.DarkPalette.Border,
	)...)
	if config.DefaultWebpageZoom < 0.1 || config.DefaultWebpageZoom > 5.0 {
		validationErrors = append(validationErrors, "default_webpage_zoom must be between 0.1 and 5.0")
	}
	if config.DefaultUIScale < 0.5 || config.DefaultUIScale > 3.0 {
		validationErrors = append(validationErrors, "default_ui_scale must be between 0.5 and 3.0")
	}
	return validationErrors
}

func validateSearchEngine(config *Config) []string {
	if config.DefaultSearchEngine == "" {
		return []string{"default_search_engine cannot be empty"}
	}
	if !strings.Contains(config.DefaultSearchEngine, "%s") {
		return []string{"default_search_engine must contain %s placeholder for the search query"}
	}
	return nil
}

func validatePopups(config *Config) []string {
	var validationErrors []string
	switch config.Workspace.Popups.Behavior {
	case PopupBehaviorSplit, PopupBehaviorStacked, PopupBehaviorTabbed, PopupBehaviorWindowed:
	default:
		validationErrors = append(validationErrors, fmt.Sprintf(
			"workspace.popups.behavior must be one of: split, stacked, tabbed, windowed (got: %s)",
			config.Workspace.Popups.Behavior,
		))
	}

	if config.Workspace.Popups.Behavior == PopupBehaviorSplit {
		switch config.Workspace.Popups.Placement {
		case "right", "left", "top", "bottom":
		default:
			validationErrors = append(validationErrors, fmt.Sprintf(
				"workspace.popups.placement must be one of: right, left, top, bottom (got: %s)",
				config.Workspace.Popups.Placement,
			))
		}
	}

	switch config.Workspace.Popups.BlankTargetBehavior {
	case "split", "stacked", "tabbed":
	default:
		validationErrors = append(validationErrors, fmt.Sprintf(
			"workspace.popups.blank_target_behavior must be one of: split, stacked, tabbed (got: %s)",
			config.Workspace.Popups.BlankTargetBehavior,
		))
	}
	return validationErrors
}

func validateWorkspaceStyling(config *Config) []string {
	var validationErrors []string
	if config.Workspace.Styling.BorderWidth < 0 {
		validationErrors = append(validationErrors, "workspace.styling.border_width must be non-negative")
	}
	if config.Workspace.Styling.ModeBorderWidth < 0 {
		validationErrors = append(validationErrors, "workspace.styling.mode_border_width must be non-negative")
	}
	if config.Workspace.Styling.TransitionDuration < 0 {
		validationErrors = append(validationErrors, "workspace.styling.transition_duration must be non-negative")
	}
	return validationErrors
}

func validatePaneMode(config *Config) []string {
	var validationErrors []string
	if config.Workspace.PaneMode.TimeoutMilliseconds < 0 {
		validationErrors = append(validationErrors, "workspace.pane_mode.timeout_ms must be non-negative")
	}
	if len(config.Workspace.PaneMode.Actions) == 0 {
		validationErrors = append(validationErrors, "workspace.pane_mode.actions cannot be empty")
	}

	seenKeys := make(map[string]string)
	for action, keys := range config.Workspace.PaneMode.Actions {
		if len(keys) == 0 {
			validationErrors = append(validationErrors, fmt.Sprintf("workspace.pane_mode.actions.%s must have at least one key binding", action))
		}
		for _, key := range keys {
			if existingAction, exists := seenKeys[key]; exists {
				validationErrors = append(validationErrors, fmt.Sprintf(
					"duplicate key binding '%s' found in pane_mode actions '%s' and '%s'",
					key,
					existingAction,
					action,
				))
			}
			seenKeys[key] = action
		}
	}
	return validationErrors
}

func validateTabBar(config *Config) []string {
	switch config.Workspace.TabBarPosition {
	case "top", "bottom":
		return nil
	default:
		return []string{fmt.Sprintf("workspace.tab_bar_position must be 'top' or 'bottom' (got: %s)", config.Workspace.TabBarPosition)}
	}
}

func validateTabMode(config *Config) []string {
	var validationErrors []string
	if config.Workspace.TabMode.TimeoutMilliseconds < 0 {
		validationErrors = append(validationErrors, "workspace.tab_mode.timeout_ms must be non-negative")
	}
	if len(config.Workspace.TabMode.Actions) == 0 {
		validationErrors = append(validationErrors, "workspace.tab_mode.actions cannot be empty")
	}

	tabSeenKeys := make(map[string]string)
	for action, keys := range config.Workspace.TabMode.Actions {
		if len(keys) == 0 {
			validationErrors = append(validationErrors, fmt.Sprintf("workspace.tab_mode.actions.%s must have at least one key binding", action))
		}
		for _, key := range keys {
			if existingAction, exists := tabSeenKeys[key]; exists {
				validationErrors = append(validationErrors, fmt.Sprintf(
					"duplicate key binding '%s' found in tab_mode actions '%s' and '%s'",
					key,
					existingAction,
					action,
				))
			}
			tabSeenKeys[key] = action
		}
	}
	return validationErrors
}

func validateLogging(config *Config) []string {
	var validationErrors []string
	if config.Logging.MaxAge < 0 {
		validationErrors = append(validationErrors, "logging.max_age must be non-negative")
	}
	switch config.Logging.Level {
	case "trace", "debug", "info", "warn", "error", "fatal", "":
	default:
		validationErrors = append(validationErrors, fmt.Sprintf(
			"logging.level must be one of: trace, debug, info, warn, error, fatal (got: %s)",
			config.Logging.Level,
		))
	}
	switch config.Logging.Format {
	case "text", "json", "console", "":
	default:
		validationErrors = append(validationErrors, fmt.Sprintf(
			"logging.format must be one of: text, json, console (got: %s)",
			config.Logging.Format,
		))
	}
	return validationErrors
}

func validateWorkspaceNewPaneURL(config *Config) []string {
	var validationErrors []string

	if config.Workspace.NewPaneURL == "" {
		validationErrors = append(validationErrors, "workspace.new_pane_url cannot be empty")
		return validationErrors
	}

	normalized := domainurl.Normalize(config.Workspace.NewPaneURL)
	parsed, err := url.Parse(normalized)
	if err != nil {
		validationErrors = append(validationErrors, fmt.Sprintf(
			"workspace.new_pane_url must be a valid URL (got: %s)",
			config.Workspace.NewPaneURL,
		))
		return validationErrors
	}

	switch parsed.Scheme {
	case "http", "https", "dumb", "file", "about":
		// ok
	default:
		validationErrors = append(validationErrors, fmt.Sprintf(
			"workspace.new_pane_url must use one of: http, https, dumb, file, about (got: %s)",
			parsed.Scheme,
		))
	}

	return validationErrors
}

func validateOmnibox(config *Config) []string {
	switch config.Omnibox.InitialBehavior {
	case "recent", "most_visited", "none":
		return nil
	default:
		return []string{fmt.Sprintf(
			"omnibox.initial_behavior must be one of: recent, most_visited, none (got: %s)",
			config.Omnibox.InitialBehavior,
		)}
	}
}

func validateRendering(config *Config) []string {
	var validationErrors []string

	switch config.Rendering.Mode {
	case RenderingModeAuto, RenderingModeGPU, RenderingModeCPU, "":
	default:
		validationErrors = append(
			validationErrors,
			fmt.Sprintf("rendering.mode must be one of: auto, gpu, cpu (got: %s)", config.Rendering.Mode),
		)
	}

	switch config.Rendering.GSKRenderer {
	case GSKRendererAuto, GSKRendererOpenGL, GSKRendererVulkan, GSKRendererCairo, "":
	default:
		validationErrors = append(
			validationErrors,
			fmt.Sprintf(
				"rendering.gsk_renderer must be one of: auto, opengl, vulkan, cairo (got: %s)",
				config.Rendering.GSKRenderer,
			),
		)
	}

	if config.Rendering.ForceCompositingMode && config.Rendering.DisableCompositingMode {
		validationErrors = append(validationErrors,
			"rendering.force_compositing_mode and rendering.disable_compositing_mode cannot both be true",
		)
	}

	return validationErrors
}

func validateColorScheme(config *Config) []string {
	switch config.Appearance.ColorScheme {
	case ThemePreferDark, ThemePreferLight, ThemeDefault, "":
		return nil
	default:
		return []string{fmt.Sprintf(
			"appearance.color_scheme must be one of: prefer-dark, prefer-light, default (got: %s)",
			config.Appearance.ColorScheme,
		)}
	}
}

func validateSession(config *Config) []string {
	var validationErrors []string
	if config.Session.MaxExitedSessions < 0 {
		validationErrors = append(validationErrors, "session.max_exited_sessions must be non-negative")
	}
	if config.Session.MaxExitedSessionAgeDays < 0 {
		validationErrors = append(validationErrors, "session.max_exited_session_age_days must be non-negative")
	}
	if config.Session.SnapshotIntervalMs < 0 {
		validationErrors = append(validationErrors, "session.snapshot_interval_ms must be non-negative")
	}
	return validationErrors
}

func validatePerformanceProfile(config *Config) []string {
	var validationErrors []string

	// Validate profile name
	if !IsValidPerformanceProfile(config.Performance.Profile) {
		validationErrors = append(validationErrors, fmt.Sprintf(
			"performance.profile must be one of: default, lite, max, custom (got: %s)",
			config.Performance.Profile,
		))
	}

	// When profile is not "custom", warn if individual fields are set
	// (they will be ignored in favor of profile-computed values)
	if config.Performance.Profile != ProfileCustom && config.Performance.Profile != "" {
		if HasCustomPerformanceFields(&config.Performance) {
			validationErrors = append(validationErrors,
				"performance tuning fields (skia_*, *_memory_*) are ignored when profile is not 'custom'; "+
					"set profile = \"custom\" to use individual field values",
			)
		}
	}

	// Validate memory pressure threshold ordering: WebKit requires conservative < strict
	webCons := config.Performance.WebProcessMemoryConservativeThreshold
	webStrict := config.Performance.WebProcessMemoryStrictThreshold
	if webCons > 0 && webStrict > 0 && webCons >= webStrict {
		validationErrors = append(validationErrors,
			"performance.web_process_memory_conservative_threshold must be less than "+
				"web_process_memory_strict_threshold when both are set",
		)
	}

	netCons := config.Performance.NetworkProcessMemoryConservativeThreshold
	netStrict := config.Performance.NetworkProcessMemoryStrictThreshold
	if netCons > 0 && netStrict > 0 && netCons >= netStrict {
		validationErrors = append(validationErrors,
			"performance.network_process_memory_conservative_threshold must be less than "+
				"network_process_memory_strict_threshold when both are set",
		)
	}

	return validationErrors
}
