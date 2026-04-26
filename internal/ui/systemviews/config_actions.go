package systemviews

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
)

const (
	configActionSaveAppearance          = "config.appearance.save"
	configActionResetAppearance         = "config.appearance.reset"
	configActionSaveSearch              = "config.search.save"
	configActionCreateSearchShortcut    = "config.searchShortcut.create"
	configActionUpdateSearchShortcut    = "config.searchShortcut.update"
	configActionDeleteSearchShortcut    = "config.searchShortcut.delete"
	configActionSavePerformance         = "config.performance.save"
	configActionSetKeybinding           = "config.keybinding.set"
	configActionResetKeybinding         = "config.keybinding.reset"
	configActionResetAllKeybindings     = "config.keybinding.resetAll"
	configRequestIDPrefix               = "systemviews-config"
	balancedPerformanceProfile          = "balanced"
	customPerformanceProfile            = "custom"
	httpScheme                          = "http"
	maxCustomPerformanceThreads         = 8
	maxCustomPerformanceWebMemoryMB     = 16384
	maxCustomPerformanceNetworkMemoryMB = 4096
	maxCustomPerformancePrewarm         = 20
)

var configRequestCounter atomic.Uint64

func nextConfigRequestID() string {
	return fmt.Sprintf("%s-%d", configRequestIDPrefix, configRequestCounter.Add(1))
}

func (a *App) handleConfigAction(ctx context.Context, event DOMAction) error {
	if a.deps.Config == nil {
		return fmt.Errorf("config service not configured")
	}

	switch event.Action {
	case configActionSaveAppearance:
		return a.saveAppearanceConfig(ctx, event.Data)
	case configActionResetAppearance:
		return a.resetAppearanceConfig(ctx)
	case configActionSaveSearch:
		return a.saveSearchConfig(ctx, event.Data)
	case configActionCreateSearchShortcut:
		return a.createSearchShortcut(ctx, event.Data)
	case configActionUpdateSearchShortcut:
		return a.updateSearchShortcut(ctx, event.Data)
	case configActionDeleteSearchShortcut:
		return a.deleteSearchShortcut(ctx, event.Data)
	case configActionSavePerformance:
		return a.savePerformanceConfig(ctx, event.Data)
	case configActionSetKeybinding:
		return a.setConfigKeybinding(ctx, event.Data)
	case configActionResetKeybinding:
		return a.resetConfigKeybinding(ctx, event.Data)
	case configActionResetAllKeybindings:
		if err := a.deps.Config.ResetAllKeybindings(ctx); err != nil {
			return err
		}
		a.configNotice = "Reset all keybindings to defaults"
		return nil
	default:
		return fmt.Errorf("unknown config action: %q", event.Action)
	}
}

func (a *App) saveAppearanceConfig(ctx context.Context, data map[string]string) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}

	fontSize, err := parseConfigInt(data["default_font_size"], "default font size")
	if err != nil {
		return err
	}
	uiScale, err := parseConfigFloat(data["default_ui_scale"], "UI scale")
	if err != nil {
		return err
	}
	if fontSize <= 0 {
		return fmt.Errorf("default font size must be positive")
	}
	if uiScale <= 0 || math.IsNaN(uiScale) || math.IsInf(uiScale, 0) {
		return fmt.Errorf("UI scale must be a finite positive number")
	}

	cfg.Appearance.SansFont = strings.TrimSpace(data["sans_font"])
	cfg.Appearance.SerifFont = strings.TrimSpace(data["serif_font"])
	cfg.Appearance.MonospaceFont = strings.TrimSpace(data["monospace_font"])
	cfg.Appearance.DefaultFontSize = fontSize
	cfg.Appearance.ColorScheme = strings.TrimSpace(data["color_scheme"])
	cfg.Appearance.LightPalette = paletteFromForm(data, "light")
	cfg.Appearance.DarkPalette = paletteFromForm(data, "dark")
	cfg.DefaultUIScale = uiScale

	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Saved appearance settings"
	return nil
}

func (a *App) resetAppearanceConfig(ctx context.Context) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}
	defaults, err := a.deps.Config.Default(ctx)
	if err != nil {
		return err
	}
	cfg.Appearance = defaults.Appearance
	cfg.DefaultUIScale = defaults.DefaultUIScale
	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Reset appearance settings to defaults"
	return nil
}

func (a *App) saveSearchConfig(ctx context.Context, data map[string]string) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}
	defaultSearchEngine, err := requireSearchURLTemplate(data["default_search_engine"], "default search engine")
	if err != nil {
		return err
	}
	cfg.DefaultSearchEngine = defaultSearchEngine
	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Saved default search engine"
	return nil
}

func (a *App) createSearchShortcut(ctx context.Context, data map[string]string) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}
	key, shortcut := searchShortcutFromForm(data, "key")
	if key == "" {
		return fmt.Errorf("search shortcut key is required")
	}
	if shortcut.URL, err = requireSearchURLTemplate(shortcut.URL, "search shortcut URL"); err != nil {
		return err
	}
	cfg.SearchShortcuts = cloneSearchShortcuts(cfg.SearchShortcuts)
	if _, exists := cfg.SearchShortcuts[key]; exists {
		return fmt.Errorf("search shortcut %q already exists", key)
	}
	cfg.SearchShortcuts[key] = shortcut
	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Created search shortcut " + key
	return nil
}

func (a *App) updateSearchShortcut(ctx context.Context, data map[string]string) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}
	oldKey := strings.TrimSpace(data["key"])
	if oldKey == "" {
		return fmt.Errorf("search shortcut key is required")
	}
	newKey, shortcut := searchShortcutFromForm(data, "new_key")
	if newKey == "" {
		return fmt.Errorf("new search shortcut key is required")
	}
	if shortcut.URL, err = requireSearchURLTemplate(shortcut.URL, "search shortcut URL"); err != nil {
		return err
	}
	cfg.SearchShortcuts = cloneSearchShortcuts(cfg.SearchShortcuts)
	if _, exists := cfg.SearchShortcuts[oldKey]; !exists {
		return fmt.Errorf("search shortcut %q not found", oldKey)
	}
	if newKey != oldKey {
		if _, exists := cfg.SearchShortcuts[newKey]; exists {
			return fmt.Errorf("search shortcut %q already exists", newKey)
		}
		delete(cfg.SearchShortcuts, oldKey)
	}
	cfg.SearchShortcuts[newKey] = shortcut
	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Saved search shortcut " + newKey
	return nil
}

func (a *App) deleteSearchShortcut(ctx context.Context, data map[string]string) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(data["key"])
	if key == "" {
		return fmt.Errorf("search shortcut key is required")
	}
	cfg.SearchShortcuts = cloneSearchShortcuts(cfg.SearchShortcuts)
	if _, exists := cfg.SearchShortcuts[key]; !exists {
		return fmt.Errorf("search shortcut %q not found", key)
	}
	delete(cfg.SearchShortcuts, key)
	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Deleted search shortcut " + key
	return nil
}

func (a *App) savePerformanceConfig(ctx context.Context, data map[string]string) error {
	cfg, err := a.editableConfig(ctx)
	if err != nil {
		return err
	}
	profile := strings.TrimSpace(data["profile"])
	if !isAllowedPerformanceProfile(profile) {
		return fmt.Errorf("unknown performance profile: %q", profile)
	}
	cfg.Performance.Profile = profile
	if profile != customPerformanceProfile {
		if saveErr := a.saveEditableConfig(ctx, cfg); saveErr != nil {
			return saveErr
		}
		a.configNotice = "Saved performance settings. Restart may be required."
		return nil
	}

	skiaCPU, err := parseConfigInt(data["skia_cpu_threads"], "Skia CPU threads")
	if err != nil {
		return err
	}
	skiaGPU, err := parseConfigInt(data["skia_gpu_threads"], "Skia GPU threads")
	if err != nil {
		return err
	}
	webMemory, err := parseConfigInt(data["web_process_memory_mb"], "web process memory")
	if err != nil {
		return err
	}
	networkMemory, err := parseConfigInt(data["network_process_memory_mb"], "network process memory")
	if err != nil {
		return err
	}
	prewarm, err := parseConfigInt(data["webview_pool_prewarm"], "WebView pool prewarm")
	if err != nil {
		return err
	}
	if err := validatePerformanceInt(skiaCPU, "Skia CPU threads", 0, maxCustomPerformanceThreads); err != nil {
		return err
	}
	if err := validatePerformanceInt(skiaGPU, "Skia GPU threads", -1, maxCustomPerformanceThreads); err != nil {
		return err
	}
	if err := validatePerformanceInt(webMemory, "web process memory", 0, maxCustomPerformanceWebMemoryMB); err != nil {
		return err
	}
	if err := validatePerformanceInt(networkMemory, "network process memory", 0, maxCustomPerformanceNetworkMemoryMB); err != nil {
		return err
	}
	if err := validatePerformanceInt(prewarm, "WebView pool prewarm", 0, maxCustomPerformancePrewarm); err != nil {
		return err
	}

	cfg.Performance.Custom = dto.WebUICustomPerformanceConfig{
		SkiaCPUThreads:         skiaCPU,
		SkiaGPUThreads:         skiaGPU,
		WebProcessMemoryMB:     webMemory,
		NetworkProcessMemoryMB: networkMemory,
		WebViewPoolPrewarm:     prewarm,
	}
	if err := a.saveEditableConfig(ctx, cfg); err != nil {
		return err
	}
	a.configNotice = "Saved performance settings. Restart may be required."
	return nil
}

func (a *App) setConfigKeybinding(ctx context.Context, data map[string]string) error {
	mode := strings.TrimSpace(data["mode"])
	action := strings.TrimSpace(data["action"])
	keys := parseKeyList(data["keys"])
	if mode == "" || action == "" {
		return fmt.Errorf("mode and action are required")
	}
	if err := a.requireKeybindingTarget(ctx, mode, action); err != nil {
		return err
	}
	resp, err := a.deps.Config.SetKeybinding(ctx, port.SetKeybindingRequest{
		RequestID: nextConfigRequestID(),
		Mode:      mode,
		Action:    action,
		Keys:      keys,
	})
	if err != nil {
		return err
	}
	if conflicts := len(resp.Conflicts); conflicts > 0 {
		a.configNotice = fmt.Sprintf("Saved keybinding with %d conflict(s)", conflicts)
		return nil
	}
	a.configNotice = "Saved keybinding " + action
	return nil
}

func (a *App) resetConfigKeybinding(ctx context.Context, data map[string]string) error {
	mode := strings.TrimSpace(data["mode"])
	action := strings.TrimSpace(data["action"])
	if mode == "" || action == "" {
		return fmt.Errorf("mode and action are required")
	}
	if err := a.requireKeybindingTarget(ctx, mode, action); err != nil {
		return err
	}
	if err := a.deps.Config.ResetKeybinding(ctx, port.ResetKeybindingRequest{
		RequestID: nextConfigRequestID(),
		Mode:      mode,
		Action:    action,
	}); err != nil {
		return err
	}
	a.configNotice = "Reset keybinding " + action
	return nil
}

func (a *App) editableConfig(ctx context.Context) (dto.WebUIConfig, error) {
	cfg, err := a.deps.Config.Current(ctx)
	if err != nil {
		return dto.WebUIConfig{}, err
	}
	a.config = &cfg
	return webUIConfigFromPayload(cfg), nil
}

func (a *App) requireKeybindingTarget(ctx context.Context, mode, action string) error {
	keybindings, err := a.deps.Config.GetKeybindings(ctx)
	if err != nil {
		return err
	}
	a.keybindings = keybindings
	groups := configKeybindingGroups(keybindings)
	for _, group := range groups {
		if group.Mode != mode {
			continue
		}
		for _, binding := range group.Bindings {
			if binding.Action == action {
				return nil
			}
		}
	}
	return fmt.Errorf("keybinding %s/%s not found", mode, action)
}

func (a *App) saveEditableConfig(ctx context.Context, cfg dto.WebUIConfig) error {
	if err := a.deps.Config.Save(ctx, cfg); err != nil {
		return err
	}
	a.config = nil
	return nil
}

func webUIConfigFromPayload(payload dto.SystemviewConfigPayload) dto.WebUIConfig {
	return dto.WebUIConfig{
		Appearance:          payload.Appearance,
		DefaultUIScale:      payload.DefaultUIScale,
		DefaultSearchEngine: payload.DefaultSearchEngine,
		SearchShortcuts:     cloneSearchShortcuts(payload.SearchShortcuts),
		Performance: dto.WebUIPerformanceConfig{
			Profile: payload.Performance.Profile,
			Custom: dto.WebUICustomPerformanceConfig{
				SkiaCPUThreads:         payload.Performance.Custom.SkiaCPUThreads,
				SkiaGPUThreads:         payload.Performance.Custom.SkiaGPUThreads,
				WebProcessMemoryMB:     payload.Performance.Custom.WebProcessMemoryMB,
				NetworkProcessMemoryMB: payload.Performance.Custom.NetworkProcessMemoryMB,
				WebViewPoolPrewarm:     payload.Performance.Custom.WebViewPoolPrewarm,
			},
		},
	}
}

func cloneSearchShortcuts(shortcuts map[string]dto.SearchShortcut) map[string]dto.SearchShortcut {
	clone := make(map[string]dto.SearchShortcut, len(shortcuts))
	for key, shortcut := range shortcuts {
		clone[key] = shortcut
	}
	return clone
}

func paletteFromForm(data map[string]string, prefix string) dto.ColorPalette {
	color := func(field string) string {
		return strings.TrimSpace(data[prefix+"_"+field])
	}

	return dto.ColorPalette{
		Background:     color("background"),
		Surface:        color("surface"),
		SurfaceVariant: color("surface_variant"),
		Text:           color("text"),
		Muted:          color("muted"),
		Accent:         color("accent"),
		Border:         color("border"),
	}
}

func searchShortcutFromForm(data map[string]string, keyField string) (string, dto.SearchShortcut) {
	return strings.TrimSpace(data[keyField]), dto.SearchShortcut{
		URL:         strings.TrimSpace(data["url"]),
		Description: strings.TrimSpace(data["description"]),
	}
}

func requireSearchURLTemplate(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if !strings.Contains(value, "%s") {
		return "", fmt.Errorf("%s must include %%s placeholder", label)
	}
	parsed, err := url.Parse(strings.Replace(value, "%s", "example", 1))
	if err != nil || parsed.Host == "" || (parsed.Scheme != httpScheme && parsed.Scheme != "https") {
		return "", fmt.Errorf("%s must be an absolute http(s) URL", label)
	}
	return value, nil
}

func parseConfigInt(raw, label string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid %s", label)
	}
	return value, nil
}

func parseConfigFloat(raw, label string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", label)
	}
	return value, nil
}

func isAllowedPerformanceProfile(profile string) bool {
	switch profile {
	case "default", balancedPerformanceProfile, "lite", "max", customPerformanceProfile:
		return true
	default:
		return false
	}
}

func validatePerformanceInt(value int, label string, minValue, maxValue int) error {
	if value < minValue || value > maxValue {
		return fmt.Errorf("%s must be between %d and %d", label, minValue, maxValue)
	}
	return nil
}

func parseKeyList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = strings.ReplaceAll(raw, ",", " ")
	parts := strings.Fields(raw)
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}
