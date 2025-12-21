package port

import "context"

// WebUIConfig represents the subset of configuration editable in dumb://config.
type WebUIConfig struct {
	Appearance          WebUIAppearanceConfig     `json:"appearance"`
	DefaultUIScale      float64                   `json:"default_ui_scale"`
	DefaultSearchEngine string                    `json:"default_search_engine"`
	SearchShortcuts     map[string]SearchShortcut `json:"search_shortcuts"`
}

type WebUIAppearanceConfig struct {
	SansFont        string       `json:"sans_font"`
	SerifFont       string       `json:"serif_font"`
	MonospaceFont   string       `json:"monospace_font"`
	DefaultFontSize int          `json:"default_font_size"`
	ColorScheme     string       `json:"color_scheme"`
	LightPalette    ColorPalette `json:"light_palette"`
	DarkPalette     ColorPalette `json:"dark_palette"`
}

type ColorPalette struct {
	Background     string `json:"background"`
	Surface        string `json:"surface"`
	SurfaceVariant string `json:"surface_variant"`
	Text           string `json:"text"`
	Muted          string `json:"muted"`
	Accent         string `json:"accent"`
	Border         string `json:"border"`
}

type SearchShortcut struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// WebUIConfigSaver persists WebUI configuration changes.
type WebUIConfigSaver interface {
	SaveWebUIConfig(ctx context.Context, cfg WebUIConfig) error
}
