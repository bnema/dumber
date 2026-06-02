package noctalia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

const (
	providerName     = "noctalia"
	formatName       = formatColorsJSON
	formatColorsJSON = "colors-json"
	formatDumberJSON = "dumber-json"
	colorsJSONName   = "Noctalia colors.json"
	dumberJSONName   = "Noctalia Dumber JSON"
)

var _ port.ExternalThemeSource = (*FileSource)(nil)

// FileSource reads a Noctalia external theme file.
//
// The default colors-json format reads Noctalia's native colors.json directly.
// The dumber-json format remains available for explicit Dumber-specific user templates.
type FileSource struct {
	mu       sync.RWMutex
	enabled  bool
	path     string
	format   string
	identity string
}

// NewFileSource creates a Noctalia colors-json file source.
func NewFileSource(enabled bool, path string) *FileSource {
	source := &FileSource{}
	source.Configure(entity.ExternalThemeConfig{
		Enabled:  enabled,
		Provider: providerName,
		Format:   formatName,
		Path:     path,
	})
	return source
}

// NewFileSourceFromConfig creates a file source from external theme config.
func NewFileSourceFromConfig(cfg entity.ExternalThemeConfig) *FileSource {
	source := &FileSource{}
	source.Configure(cfg)
	return source
}

// Configure updates the source from a normalized or raw external theme config snapshot.
func (s *FileSource) Configure(cfg entity.ExternalThemeConfig) {
	provider, format := normalizeProviderAndFormat(cfg.Provider, cfg.Format)
	enabled := cfg.Enabled && provider == providerName && isSupportedFormat(format)
	path := strings.TrimSpace(cfg.Path)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	s.path = path
	s.format = format
	s.identity = externalThemeIdentity(enabled, provider, format, path)
}

// ExternalThemeIdentity returns the current normalized external theme identity.
func (s *FileSource) ExternalThemeIdentity() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.identity
}

// IsEnabled returns whether this source should be used.
func (s *FileSource) IsEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

// Get reads and parses the configured Noctalia theme file.
func (s *FileSource) Get(ctx context.Context) (*entity.ExternalTheme, error) {
	s.mu.RLock()
	enabled := s.enabled
	pathConfig := s.path
	format := s.format
	s.mu.RUnlock()

	if !enabled {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := ExpandPath(pathConfig)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, errors.New("noctalia theme path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read noctalia theme file: %w", err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	switch format {
	case formatColorsJSON:
		return ParseColorsJSON(data)
	case formatDumberJSON:
		return ParseDumberJSON(data)
	default:
		return nil, fmt.Errorf("unsupported noctalia theme format %q", format)
	}
}

// ExpandPath expands environment variables, leading ~, and cleans the path.
func ExpandPath(path string) (string, error) {
	expanded := os.ExpandEnv(strings.TrimSpace(path))
	if expanded == "" {
		return "", nil
	}
	if expanded == "~" || strings.HasPrefix(expanded, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home directory: %w", err)
		}
		if expanded == "~" {
			expanded = home
		} else {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	return filepath.Clean(expanded), nil
}

// ParseColorsJSON parses Noctalia's native colors.json file.
//
// Noctalia colors.json represents the active palette only, so Dumber applies the
// mapped palette to both light and dark resolved palettes. The mapper uses
// optional mShadow/mHover roles when present to preserve background/surface/input
// contrast, while falling back to older required roles for compatibility.
func ParseColorsJSON(data []byte) (*entity.ExternalTheme, error) {
	var raw colorsThemeJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse noctalia colors json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("parse noctalia colors json: trailing data")
	}

	palette, err := raw.toPalette()
	if err != nil {
		return nil, err
	}
	light := palette
	dark := palette
	return &entity.ExternalTheme{
		Name:         colorsJSONName,
		Provider:     providerName,
		LightPalette: &light,
		DarkPalette:  &dark,
	}, nil
}

// ParseDumberJSON parses Noctalia user-template-generated Dumber JSON.
func ParseDumberJSON(data []byte) (*entity.ExternalTheme, error) {
	var raw dumberThemeJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse noctalia dumber json: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("parse noctalia dumber json: trailing data")
	}
	if raw.Light == nil {
		return nil, errors.New("parse noctalia dumber json: missing light palette")
	}
	if raw.Dark == nil {
		return nil, errors.New("parse noctalia dumber json: missing dark palette")
	}

	light := raw.Light.toEntity()
	dark := raw.Dark.toEntity()
	if err := validatePalette(&light, "light"); err != nil {
		return nil, err
	}
	if err := validatePalette(&dark, "dark"); err != nil {
		return nil, err
	}

	return &entity.ExternalTheme{
		Name:         firstNonEmpty(raw.Name, raw.Source, dumberJSONName),
		Provider:     providerName,
		LightPalette: &light,
		DarkPalette:  &dark,
	}, nil
}

type colorsThemeJSON struct {
	MPrimary          *string `json:"mPrimary"`
	MSurface          *string `json:"mSurface"`
	MOnSurface        *string `json:"mOnSurface"`
	MSurfaceVariant   *string `json:"mSurfaceVariant"`
	MOnSurfaceVariant *string `json:"mOnSurfaceVariant"`
	MOutline          *string `json:"mOutline"`
	MShadow           *string `json:"mShadow"`
	MHover            *string `json:"mHover"`
}

func (raw colorsThemeJSON) toPalette() (entity.ColorPalette, error) {
	surface, err := requiredColorsHex("mSurface", raw.MSurface)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	surfaceVariantBase, err := requiredColorsHex("mSurfaceVariant", raw.MSurfaceVariant)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	text, err := requiredColorsHex("mOnSurface", raw.MOnSurface)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	muted, err := requiredColorsHex("mOnSurfaceVariant", raw.MOnSurfaceVariant)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	accent, err := requiredColorsHex("mPrimary", raw.MPrimary)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	border, err := requiredColorsHex("mOutline", raw.MOutline)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	background, err := optionalColorsHex("mShadow", raw.MShadow, surface)
	if err != nil {
		return entity.ColorPalette{}, err
	}
	surfaceVariant, err := optionalColorsHex("mHover", raw.MHover, surfaceVariantBase)
	if err != nil {
		return entity.ColorPalette{}, err
	}

	return entity.ColorPalette{
		Background:     background,
		Surface:        surface,
		SurfaceVariant: surfaceVariant,
		Text:           text,
		Muted:          muted,
		Accent:         accent,
		Border:         border,
	}, nil
}

func requiredColorsHex(field string, value *string) (string, error) {
	if value == nil || *value == "" {
		return "", fmt.Errorf("parse noctalia colors json: missing %s", field)
	}
	return validateColorsHex(field, *value)
}

func optionalColorsHex(field string, value *string, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	return validateColorsHex(field, *value)
}

func validateColorsHex(field, value string) (string, error) {
	if !entity.IsValidHex(value) {
		return "", fmt.Errorf("parse noctalia colors json: %s: not a valid CSS-safe hex color (#RRGGBB)", field)
	}
	return value, nil
}

type dumberThemeJSON struct {
	Source string       `json:"source"`
	Mode   string       `json:"mode"`
	Name   string       `json:"name"`
	Light  *paletteJSON `json:"light"`
	Dark   *paletteJSON `json:"dark"`
}

type paletteJSON struct {
	Background     string `json:"background"`
	Surface        string `json:"surface"`
	SurfaceVariant string `json:"surface_variant"`
	Text           string `json:"text"`
	Muted          string `json:"muted"`
	Accent         string `json:"accent"`
	Border         string `json:"border"`
}

func (p paletteJSON) toEntity() entity.ColorPalette {
	return entity.ColorPalette{
		Background:     p.Background,
		Surface:        p.Surface,
		SurfaceVariant: p.SurfaceVariant,
		Text:           p.Text,
		Muted:          p.Muted,
		Accent:         p.Accent,
		Border:         p.Border,
	}
}

func validatePalette(p *entity.ColorPalette, prefix string) error {
	warnings := entity.ValidatePaletteOverrideHex(p, prefix)
	if len(warnings) == 0 {
		return nil
	}
	return fmt.Errorf("parse noctalia dumber json: %s: %s", warnings[0].Field, warnings[0].Message)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeProviderAndFormat(providerInput, formatInput string) (string, string) {
	provider := strings.ToLower(strings.TrimSpace(providerInput))
	if provider == "" {
		provider = providerName
	}
	format := strings.ToLower(strings.TrimSpace(formatInput))
	if format == "" {
		format = formatName
	}
	return provider, format
}

func isSupportedFormat(format string) bool {
	switch format {
	case formatColorsJSON, formatDumberJSON:
		return true
	default:
		return false
	}
}

func externalThemeIdentity(enabled bool, provider, format, path string) string {
	if !enabled {
		return ""
	}
	return provider + "\x00" + format + "\x00" + strings.TrimSpace(path)
}
