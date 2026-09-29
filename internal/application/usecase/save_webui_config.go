package usecase

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/validation"
)

type SaveWebUIConfigUseCase struct {
	saver port.WebUIConfigSaver
}

func NewSaveWebUIConfigUseCase(saver port.WebUIConfigSaver) *SaveWebUIConfigUseCase {
	return &SaveWebUIConfigUseCase{saver: saver}
}

func (uc *SaveWebUIConfigUseCase) Execute(ctx context.Context, cfg dto.WebUIConfig) error {
	if uc == nil || uc.saver == nil {
		return fmt.Errorf("config saver is nil")
	}

	if err := validateSearchShortcutKeyCollisions(cfg.SearchShortcuts); err != nil {
		return err
	}

	normalized := normalizeWebUIConfig(cfg)
	if err := validateWebUIConfig(normalized); err != nil {
		return err
	}

	return uc.saver.SaveWebUIConfig(ctx, normalized)
}

func validateSearchShortcutKeyCollisions(shortcuts map[string]dto.SearchShortcut) error {
	seen := make(map[string]string, len(shortcuts))
	for key := range shortcuts {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if previous, ok := seen[trimmedKey]; ok && previous != key {
			return fmt.Errorf("search shortcut key %q duplicates %q after trimming", key, previous)
		}
		seen[trimmedKey] = key
	}
	return nil
}

func normalizeWebUIConfig(cfg dto.WebUIConfig) dto.WebUIConfig {
	cfg.Appearance.SansFont = strings.TrimSpace(cfg.Appearance.SansFont)
	cfg.Appearance.SerifFont = strings.TrimSpace(cfg.Appearance.SerifFont)
	cfg.Appearance.MonospaceFont = strings.TrimSpace(cfg.Appearance.MonospaceFont)
	cfg.Appearance.GtkFont = strings.TrimSpace(cfg.Appearance.GtkFont)
	cfg.Appearance.ColorScheme = strings.TrimSpace(cfg.Appearance.ColorScheme)
	cfg.DefaultSearchEngine = strings.TrimSpace(cfg.DefaultSearchEngine)
	cfg.Performance.Profile = strings.TrimSpace(cfg.Performance.Profile)
	if cfg.Performance.Profile == performanceProfileCustom {
		cfg.Performance.Custom = clampCustomPerformance(cfg.Performance.Custom)
	}
	if len(cfg.SearchShortcuts) > 0 {
		normalized := make(map[string]dto.SearchShortcut, len(cfg.SearchShortcuts))
		for key, shortcut := range cfg.SearchShortcuts {
			trimmedKey := strings.TrimSpace(key)
			if trimmedKey == "" {
				continue
			}
			shortcut.URL = strings.TrimSpace(shortcut.URL)
			shortcut.Description = strings.TrimSpace(shortcut.Description)
			normalized[trimmedKey] = shortcut
		}
		cfg.SearchShortcuts = normalized
	}
	return cfg
}

const performanceProfileCustom = "custom"

// Bounds for user-editable custom performance fields.
const (
	maxSkiaCPUThreads         = 8
	maxSkiaGPUThreads         = 8
	maxWebProcessMemoryMB     = 16384
	maxNetworkProcessMemoryMB = 4096
	maxWebViewPoolPrewarm     = 20
)

// clampCustomPerformance keeps custom profile values inside supported bounds.
// A GPU thread count of -1 means "engine default".
func clampCustomPerformance(c dto.WebUICustomPerformanceConfig) dto.WebUICustomPerformanceConfig {
	c.SkiaCPUThreads = min(max(c.SkiaCPUThreads, 0), maxSkiaCPUThreads)
	c.SkiaGPUThreads = min(max(c.SkiaGPUThreads, -1), maxSkiaGPUThreads)
	c.WebProcessMemoryMB = min(max(c.WebProcessMemoryMB, 0), maxWebProcessMemoryMB)
	c.NetworkProcessMemoryMB = min(max(c.NetworkProcessMemoryMB, 0), maxNetworkProcessMemoryMB)
	c.WebViewPoolPrewarm = min(max(c.WebViewPoolPrewarm, 0), maxWebViewPoolPrewarm)
	return c
}

func validateWebUIConfig(cfg dto.WebUIConfig) error {
	var errs []string

	if cfg.Appearance.DefaultFontSize < 1 || cfg.Appearance.DefaultFontSize > 72 {
		errs = append(errs, "appearance.default_font_size must be between 1 and 72")
	}
	if math.IsNaN(cfg.DefaultUIScale) || math.IsInf(cfg.DefaultUIScale, 0) || cfg.DefaultUIScale < 0.5 || cfg.DefaultUIScale > 3.0 {
		errs = append(errs, "default_ui_scale must be a finite value between 0.5 and 3.0")
	}

	errs = append(errs, validation.ValidateFontFamily("appearance.sans_font", cfg.Appearance.SansFont)...)
	errs = append(errs, validation.ValidateFontFamily("appearance.serif_font", cfg.Appearance.SerifFont)...)
	errs = append(errs, validation.ValidateFontFamily("appearance.monospace_font", cfg.Appearance.MonospaceFont)...)
	errs = append(errs, validation.ValidateFontFamily("appearance.gtk_font", cfg.Appearance.GtkFont)...)

	for _, err := range validation.ValidateShortcutURL(cfg.DefaultSearchEngine) {
		errs = append(errs, fmt.Sprintf("default_search_engine: %s", err))
	}

	for key, shortcut := range cfg.SearchShortcuts {
		for _, err := range validation.ValidateShortcutKey(key) {
			errs = append(errs, fmt.Sprintf("search_shortcuts.%s: %s", key, err))
		}
		for _, err := range validation.ValidateShortcutURL(shortcut.URL) {
			errs = append(errs, fmt.Sprintf("search_shortcuts.%s.url: %s", key, err))
		}
		for _, err := range validation.ValidateShortcutDescription(shortcut.Description) {
			errs = append(errs, fmt.Sprintf("search_shortcuts.%s.description: %s", key, err))
		}
	}

	switch cfg.Performance.Profile {
	case "", "default", "lite", "balanced", "max", "custom":
		// ok
	default:
		errs = append(errs, fmt.Sprintf(
			"performance.profile must be one of: default, lite, balanced, max, custom (got: %s)",
			cfg.Performance.Profile,
		))
	}

	switch cfg.Appearance.ColorScheme {
	case "default", "prefer-dark", "prefer-light":
		// ok
	default:
		errs = append(errs, fmt.Sprintf(
			"appearance.color_scheme must be one of: prefer-dark, prefer-light, default (got: %s)",
			cfg.Appearance.ColorScheme,
		))
	}

	errs = append(errs, validation.ValidatePaletteHex(
		"appearance.light_palette",
		cfg.Appearance.LightPalette.Background,
		cfg.Appearance.LightPalette.Surface,
		cfg.Appearance.LightPalette.SurfaceVariant,
		cfg.Appearance.LightPalette.Text,
		cfg.Appearance.LightPalette.Muted,
		cfg.Appearance.LightPalette.Accent,
		cfg.Appearance.LightPalette.Border,
	)...)
	errs = append(errs, validation.ValidatePaletteHex(
		"appearance.dark_palette",
		cfg.Appearance.DarkPalette.Background,
		cfg.Appearance.DarkPalette.Surface,
		cfg.Appearance.DarkPalette.SurfaceVariant,
		cfg.Appearance.DarkPalette.Text,
		cfg.Appearance.DarkPalette.Muted,
		cfg.Appearance.DarkPalette.Accent,
		cfg.Appearance.DarkPalette.Border,
	)...)

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}
