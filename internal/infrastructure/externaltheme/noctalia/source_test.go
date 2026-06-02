package noctalia

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestParseColorsJSONValidActivePalette(t *testing.T) {
	theme, err := ParseColorsJSON(noctaliaColorsJSON())
	if err != nil {
		t.Fatalf("ParseColorsJSON() error = %v", err)
	}
	if theme.Provider != "noctalia" {
		t.Fatalf("Provider = %q, want noctalia", theme.Provider)
	}
	if theme.Name == "" {
		t.Fatal("Name is empty, want source metadata")
	}
	want := entity.ColorPalette{
		Background:     "#000000",
		Surface:        "#0c0c0c",
		SurfaceVariant: "#282828",
		Text:           "#ffffff",
		Muted:          "#a0a0a0",
		Accent:         "#ffc799",
		Border:         "#505050",
	}
	if theme.LightPalette == nil || *theme.LightPalette != want {
		t.Fatalf("LightPalette = %+v, want %+v", theme.LightPalette, want)
	}
	if theme.DarkPalette == nil || *theme.DarkPalette != want {
		t.Fatalf("DarkPalette = %+v, want active Noctalia palette %+v", theme.DarkPalette, want)
	}
}

func TestParseColorsJSONDocsExample(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "docs", "examples", "noctalia-colors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	theme, err := ParseColorsJSON(data)
	if err != nil {
		t.Fatalf("ParseColorsJSON(docs example) error = %v", err)
	}
	if theme.Provider != "noctalia" || theme.DarkPalette.Accent == "" {
		t.Fatalf("parsed docs example = %+v", theme)
	}
}

func TestParseColorsJSONFallsBackWhenNuanceFieldsAreAbsent(t *testing.T) {
	theme, err := ParseColorsJSON([]byte(`{
		"mPrimary":"#ffc799",
		"mSurface":"#0c0c0c",
		"mOnSurface":"#ffffff",
		"mSurfaceVariant":"#1c1c1c",
		"mOnSurfaceVariant":"#a0a0a0",
		"mOutline":"#505050"
	}`))
	if err != nil {
		t.Fatalf("ParseColorsJSON() error = %v", err)
	}
	if theme.DarkPalette.Background != "#0c0c0c" {
		t.Fatalf("Background = %q, want mSurface fallback", theme.DarkPalette.Background)
	}
	if theme.DarkPalette.SurfaceVariant != "#1c1c1c" {
		t.Fatalf("SurfaceVariant = %q, want mSurfaceVariant fallback", theme.DarkPalette.SurfaceVariant)
	}
}

func TestParseColorsJSONRequiresMappedFields(t *testing.T) {
	cases := map[string]string{
		"missing surface":         `{"mPrimary":"#111111","mOnSurface":"#222222","mSurfaceVariant":"#333333","mOnSurfaceVariant":"#444444","mOutline":"#555555"}`,
		"missing surface variant": `{"mPrimary":"#111111","mSurface":"#222222","mOnSurface":"#333333","mOnSurfaceVariant":"#444444","mOutline":"#555555"}`,
		"missing text":            `{"mPrimary":"#111111","mSurface":"#222222","mSurfaceVariant":"#333333","mOnSurfaceVariant":"#444444","mOutline":"#555555"}`,
		"missing muted":           `{"mPrimary":"#111111","mSurface":"#222222","mOnSurface":"#333333","mSurfaceVariant":"#444444","mOutline":"#555555"}`,
		"missing accent":          `{"mSurface":"#111111","mOnSurface":"#222222","mSurfaceVariant":"#333333","mOnSurfaceVariant":"#444444","mOutline":"#555555"}`,
		"missing border":          `{"mPrimary":"#111111","mSurface":"#222222","mOnSurface":"#333333","mSurfaceVariant":"#444444","mOnSurfaceVariant":"#555555"}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseColorsJSON([]byte(input)); err == nil {
				t.Fatal("ParseColorsJSON() error = nil, want missing field error")
			}
		})
	}
}

func TestParseColorsJSONInvalidHexRejection(t *testing.T) {
	if _, err := ParseColorsJSON([]byte(`{"mPrimary":"ffc799","mSurface":"#0c0c0c","mOnSurface":"#ffffff","mSurfaceVariant":"#1c1c1c","mOnSurfaceVariant":"#a0a0a0","mOutline":"#505050"}`)); err == nil {
		t.Fatal("ParseColorsJSON() error = nil, want invalid hex error")
	}
}

func TestParseColorsJSONInvalidOptionalNuanceHexRejection(t *testing.T) {
	cases := map[string]string{
		"invalid shadow": `{"mPrimary":"#ffc799","mSurface":"#0c0c0c","mOnSurface":"#ffffff","mSurfaceVariant":"#1c1c1c","mOnSurfaceVariant":"#a0a0a0","mOutline":"#505050","mShadow":"000000"}`,
		"invalid hover":  `{"mPrimary":"#ffc799","mSurface":"#0c0c0c","mOnSurface":"#ffffff","mSurfaceVariant":"#1c1c1c","mOnSurfaceVariant":"#a0a0a0","mOutline":"#505050","mHover":"282828"}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseColorsJSON([]byte(input)); err == nil {
				t.Fatal("ParseColorsJSON() error = nil, want optional hex validation error")
			}
		})
	}
}

func TestParseColorsJSONRejectsTrailingTokens(t *testing.T) {
	if _, err := ParseColorsJSON(append(noctaliaColorsJSON(), []byte(` {}`)...)); err == nil {
		t.Fatal("ParseColorsJSON() error = nil, want trailing data error")
	}
}

func TestParseDumberJSONValidFullPalette(t *testing.T) {
	json := []byte(`{
		"name":"Night Theme",
		"source":"noctalia-template",
		"mode":"dark",
		"light":{"background":"#FFFFFF","surface":"#F0F0F0","surface_variant":"#E0E0E0","text":"#111111","muted":"#777777","accent":"#3366CC","border":"#CCCCCC"},
		"dark":{"background":"#000000","surface":"#101010","surface_variant":"#202020","text":"#EEEEEE","muted":"#999999","accent":"#AABBCC","border":"#333333"}
	}`)

	theme, err := ParseDumberJSON(json)
	if err != nil {
		t.Fatalf("ParseDumberJSON() error = %v", err)
	}
	if theme.Provider != "noctalia" {
		t.Fatalf("Provider = %q, want noctalia", theme.Provider)
	}
	if theme.Name != "Night Theme" {
		t.Fatalf("Name = %q, want metadata name", theme.Name)
	}
	if theme.LightPalette.Background != "#FFFFFF" || theme.DarkPalette.Accent != "#AABBCC" {
		t.Fatalf("parsed palettes = %+v / %+v", theme.LightPalette, theme.DarkPalette)
	}
}

func TestParseDumberJSONDocsExample(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "docs", "examples", "noctalia-dumber-theme.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	theme, err := ParseDumberJSON(data)
	if err != nil {
		t.Fatalf("ParseDumberJSON(docs example) error = %v", err)
	}
	if theme.Provider != "noctalia" || theme.LightPalette.Background == "" || theme.DarkPalette.Background == "" {
		t.Fatalf("parsed docs example = %+v", theme)
	}
}

func TestParseDumberJSONPartialPalette(t *testing.T) {
	theme, err := ParseDumberJSON([]byte(`{
		"light":{"background":"#FFFFFF","accent":"#112233"},
		"dark":{"text":"#EEEEEE"}
	}`))
	if err != nil {
		t.Fatalf("ParseDumberJSON() error = %v", err)
	}
	if theme.LightPalette.Surface != "" || theme.DarkPalette.Background != "" {
		t.Fatalf("missing fields should remain empty: %+v / %+v", theme.LightPalette, theme.DarkPalette)
	}
}

func TestParseDumberJSONRequiresLightAndDarkObjects(t *testing.T) {
	cases := map[string]string{
		"missing light": `{"dark":{}}`,
		"missing dark":  `{"light":{}}`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseDumberJSON([]byte(input)); err == nil {
				t.Fatal("ParseDumberJSON() error = nil, want error")
			}
		})
	}
}

func TestParseDumberJSONMetadataNameFallsBackToSource(t *testing.T) {
	theme, err := ParseDumberJSON([]byte(`{"source":"noctalia-template","light":{},"dark":{}}`))
	if err != nil {
		t.Fatalf("ParseDumberJSON() error = %v", err)
	}
	if theme.Name != "noctalia-template" {
		t.Fatalf("Name = %q, want source fallback", theme.Name)
	}
}

func TestParseDumberJSONValidHexAcceptance(t *testing.T) {
	_, err := ParseDumberJSON([]byte(`{"light":{"accent":"#aBc123"},"dark":{"accent":"#ABC123"}}`))
	if err != nil {
		t.Fatalf("ParseDumberJSON() error = %v", err)
	}
}

func TestParseDumberJSONInvalidHexRejection(t *testing.T) {
	if _, err := ParseDumberJSON([]byte(`{"light":{"accent":"ABC123"},"dark":{}}`)); err == nil {
		t.Fatal("ParseDumberJSON() error = nil, want invalid hex error")
	}
}

func TestParseDumberJSONMalformedRejection(t *testing.T) {
	if _, err := ParseDumberJSON([]byte(`{"light":`)); err == nil {
		t.Fatal("ParseDumberJSON() error = nil, want malformed JSON error")
	}
}

func TestParseDumberJSONRejectsTrailingTokens(t *testing.T) {
	if _, err := ParseDumberJSON([]byte(`{"light":{},"dark":{}} {}`)); err == nil {
		t.Fatal("ParseDumberJSON() error = nil, want trailing data error")
	}
}

func TestExpandPath(t *testing.T) {
	t.Setenv("NOCTALIA_THEME_TEST", "theme.json")
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ExpandPath("~/$NOCTALIA_THEME_TEST")
	if err != nil {
		t.Fatalf("ExpandPath() error = %v", err)
	}
	want := filepath.Join(home, "theme.json")
	if got != want {
		t.Fatalf("ExpandPath() = %q, want %q", got, want)
	}
}

func TestFileSourceDisabledBehavior(t *testing.T) {
	source := NewFileSource(false, "/does/not/exist")
	if source.IsEnabled() {
		t.Fatal("IsEnabled() = true, want false")
	}
	theme, err := source.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if theme != nil {
		t.Fatalf("Get() theme = %+v, want nil", theme)
	}
}

func TestFileSourceReadsColorsJSONByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "colors.json")
	if err := os.WriteFile(path, noctaliaColorsJSON(), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	theme, err := NewFileSource(true, path).Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if theme.LightPalette.Accent != "#ffc799" || theme.DarkPalette.Background != "#000000" || theme.DarkPalette.SurfaceVariant != "#282828" {
		t.Fatalf("Get() palettes = %+v / %+v", theme.LightPalette, theme.DarkPalette)
	}
}

func TestFileSourceReadsDumberJSONWhenConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(path, []byte(`{"light":{"background":"#FFFFFF"},"dark":{"background":"#000000"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source := NewFileSourceFromConfig(entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "dumber-json",
		Path:     path,
	})
	theme, err := source.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if theme.LightPalette.Background != "#FFFFFF" || theme.DarkPalette.Background != "#000000" {
		t.Fatalf("Get() palettes = %+v / %+v", theme.LightPalette, theme.DarkPalette)
	}
}

func TestFileSourceConfigureUpdatesSettings(t *testing.T) {
	colorsPath := filepath.Join(t.TempDir(), "colors.json")
	if err := os.WriteFile(colorsPath, noctaliaColorsJSON(), 0o600); err != nil {
		t.Fatalf("WriteFile(colors) error = %v", err)
	}
	dumberPath := filepath.Join(t.TempDir(), "theme.json")
	if err := os.WriteFile(dumberPath, []byte(`{"light":{"background":"#FFFFFF"},"dark":{"background":"#000000"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(dumber) error = %v", err)
	}

	source := NewFileSourceFromConfig(entity.ExternalThemeConfig{})
	if source.IsEnabled() {
		t.Fatal("IsEnabled() = true, want false")
	}

	source.Configure(entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: " NOCTALIA ",
		Format:   " COLORS-JSON ",
		Path:     colorsPath,
	})
	if !source.IsEnabled() {
		t.Fatal("IsEnabled() = false for colors-json, want true")
	}
	if _, err := source.Get(context.Background()); err != nil {
		t.Fatalf("Get() colors-json after Configure error = %v", err)
	}

	source.Configure(entity.ExternalThemeConfig{
		Enabled:  true,
		Provider: "noctalia",
		Format:   "dumber-json",
		Path:     dumberPath,
	})
	if !source.IsEnabled() {
		t.Fatal("IsEnabled() = false for dumber-json, want true")
	}
	if _, err := source.Get(context.Background()); err != nil {
		t.Fatalf("Get() dumber-json after Configure error = %v", err)
	}

	source.Configure(entity.ExternalThemeConfig{Enabled: true, Provider: "other", Format: "colors-json", Path: colorsPath})
	if source.IsEnabled() {
		t.Fatal("IsEnabled() = true for unsupported provider, want false")
	}
	source.Configure(entity.ExternalThemeConfig{Enabled: true, Provider: "noctalia", Format: "toml", Path: colorsPath})
	if source.IsEnabled() {
		t.Fatal("IsEnabled() = true for unsupported format, want false")
	}
}

func noctaliaColorsJSON() []byte {
	return []byte(`{
		"mError": "#ff8080",
		"mHover": "#282828",
		"mOnError": "#000000",
		"mOnHover": "#ffffff",
		"mOnPrimary": "#000000",
		"mOnSecondary": "#000000",
		"mOnSurface": "#ffffff",
		"mOnSurfaceVariant": "#a0a0a0",
		"mOnTertiary": "#000000",
		"mOutline": "#505050",
		"mPrimary": "#ffc799",
		"mSecondary": "#99ffe4",
		"mShadow": "#000000",
		"mSurface": "#0c0c0c",
		"mSurfaceVariant": "#1c1c1c",
		"mTertiary": "#fbadff"
	}`)
}
