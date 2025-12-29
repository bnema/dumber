package config

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

type WebUIConfigGateway struct {
	mgr *Manager
}

func NewWebUIConfigGateway(mgr *Manager) *WebUIConfigGateway {
	return &WebUIConfigGateway{mgr: mgr}
}

// Validation limits for custom performance fields.
const (
	maxWebProcessMemoryMB     = 16384
	maxNetworkProcessMemoryMB = 4096
	maxWebViewPoolPrewarm     = 20
)

// clampInt returns v clamped to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (g *WebUIConfigGateway) SaveWebUIConfig(ctx context.Context, cfg port.WebUIConfig) error {
	_ = ctx
	if g == nil || g.mgr == nil {
		return fmt.Errorf("config manager not initialized")
	}

	current := Get()
	current.Appearance.SansFont = cfg.Appearance.SansFont
	current.Appearance.SerifFont = cfg.Appearance.SerifFont
	current.Appearance.MonospaceFont = cfg.Appearance.MonospaceFont
	current.Appearance.DefaultFontSize = cfg.Appearance.DefaultFontSize
	current.Appearance.ColorScheme = cfg.Appearance.ColorScheme
	current.Appearance.LightPalette = ColorPalette{
		Background:     cfg.Appearance.LightPalette.Background,
		Surface:        cfg.Appearance.LightPalette.Surface,
		SurfaceVariant: cfg.Appearance.LightPalette.SurfaceVariant,
		Text:           cfg.Appearance.LightPalette.Text,
		Muted:          cfg.Appearance.LightPalette.Muted,
		Accent:         cfg.Appearance.LightPalette.Accent,
		Border:         cfg.Appearance.LightPalette.Border,
	}
	current.Appearance.DarkPalette = ColorPalette{
		Background:     cfg.Appearance.DarkPalette.Background,
		Surface:        cfg.Appearance.DarkPalette.Surface,
		SurfaceVariant: cfg.Appearance.DarkPalette.SurfaceVariant,
		Text:           cfg.Appearance.DarkPalette.Text,
		Muted:          cfg.Appearance.DarkPalette.Muted,
		Accent:         cfg.Appearance.DarkPalette.Accent,
		Border:         cfg.Appearance.DarkPalette.Border,
	}
	current.DefaultUIScale = cfg.DefaultUIScale
	current.DefaultSearchEngine = cfg.DefaultSearchEngine
	if len(cfg.SearchShortcuts) == 0 {
		current.SearchShortcuts = nil
	} else {
		shortcuts := make(map[string]SearchShortcut, len(cfg.SearchShortcuts))
		for key, shortcut := range cfg.SearchShortcuts {
			shortcuts[key] = SearchShortcut{
				URL:         shortcut.URL,
				Description: shortcut.Description,
			}
		}
		current.SearchShortcuts = shortcuts
	}

	// Performance profile (requires restart to take effect)
	if cfg.Performance.Profile != "" {
		current.Performance.Profile = PerformanceProfile(cfg.Performance.Profile)
	}

	// Custom performance fields (only used when profile is "custom")
	if cfg.Performance.Profile == string(ProfileCustom) {
		current.Performance.SkiaCPUPaintingThreads = clampInt(cfg.Performance.Custom.SkiaCPUThreads, 0, maxSkiaCPUThreads)
		current.Performance.SkiaGPUPaintingThreads = clampInt(cfg.Performance.Custom.SkiaGPUThreads, -1, maxSkiaGPUThreads)
		current.Performance.WebProcessMemoryLimitMB = clampInt(cfg.Performance.Custom.WebProcessMemoryMB, 0, maxWebProcessMemoryMB)
		current.Performance.NetworkProcessMemoryLimitMB = clampInt(cfg.Performance.Custom.NetworkProcessMemoryMB, 0, maxNetworkProcessMemoryMB)
		current.Performance.WebViewPoolPrewarmCount = clampInt(cfg.Performance.Custom.WebViewPoolPrewarm, 0, maxWebViewPoolPrewarm)
	}

	return g.mgr.Save(current)
}
