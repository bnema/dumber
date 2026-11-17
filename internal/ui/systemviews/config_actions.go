package systemviews

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/keychord"
)

const (
	// configActionSaveBatch submits the whole settings form. Fields that are
	// absent from the submitted data keep their stored value.
	configActionSaveBatch               = "config.saveBatch"
	configActionResetAppearance         = "config.appearance.reset"
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

// Form field names shared by config.templ and the batch parser.
const (
	configFieldExternalThemeEnabled = "external_theme_enabled"
	configShortcutPrefix            = "shortcut."
	configKeysPrefix                = "keys:"
)

var configRequestCounter atomic.Uint64

func nextConfigRequestID() string {
	return fmt.Sprintf("%s-%d", configRequestIDPrefix, configRequestCounter.Add(1))
}

// configFieldError is a validation error tied to one form field, so the page
// can focus it without discarding the user's other edits.
type configFieldError struct {
	Field   string
	Message string
}

func (e *configFieldError) Error() string { return e.Message }

func fieldError(field string, err error) error {
	if err == nil {
		return nil
	}
	var existing *configFieldError
	if errors.As(err, &existing) {
		return err
	}
	return &configFieldError{Field: field, Message: err.Error()}
}

func (a *App) handleConfigAction(ctx context.Context, event DOMAction) error {
	if a.deps.Config == nil {
		return fmt.Errorf("config service not configured")
	}

	switch event.Action {
	case configActionResetAppearance:
		return a.resetAppearanceConfig(ctx)
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

// keybindingChange is one keybinding write planned by a batch save.
type keybindingChange struct {
	Mode   string
	Action string
	Keys   []string
	Reset  bool
}

// configBatch is a validated settings form, ready to be written.
type configBatch struct {
	config        dto.WebUIConfig
	configChanged bool
	keybindings   []keybindingChange
}

// prepareConfigBatch validates the whole form before anything is written.
func (a *App) prepareConfigBatch(ctx context.Context, data map[string]string) (configBatch, error) {
	current, err := a.deps.Config.Current(ctx)
	if err != nil {
		return configBatch{}, err
	}
	a.config = &current
	stored := webUIConfigFromPayload(current)
	next := webUIConfigFromPayload(current)

	for _, apply := range []func(*dto.WebUIConfig, map[string]string) error{
		applyAppearanceFields,
		applySearchFields,
		applySearchShortcutRows,
		applyPerformanceFields,
	} {
		if applyErr := apply(&next, data); applyErr != nil {
			return configBatch{}, applyErr
		}
	}

	changes, err := a.keybindingChanges(ctx, data)
	if err != nil {
		return configBatch{}, err
	}
	return configBatch{
		config:        next,
		configChanged: !reflect.DeepEqual(stored, next),
		keybindings:   changes,
	}, nil
}

// saveConfigBatch validates and writes a settings form: one config save for
// every config field, then one call per changed keybinding. wrote reports
// whether anything was persisted before an error, so the caller knows the
// page must be re-rendered from the stored state.
func (a *App) saveConfigBatch(ctx context.Context, data map[string]string) (wrote bool, err error) {
	if a.deps.Config == nil {
		return false, fmt.Errorf("config service not configured")
	}
	batch, err := a.prepareConfigBatch(ctx, data)
	if err != nil {
		return false, err
	}
	if !batch.configChanged && len(batch.keybindings) == 0 {
		a.configNotice = "No changes to save"
		return false, nil
	}
	if batch.configChanged {
		if err := a.saveEditableConfig(ctx, batch.config); err != nil {
			return false, err
		}
		wrote = true
	}

	conflicts := 0
	for _, change := range batch.keybindings {
		n, kbErr := a.applyKeybindingChange(ctx, change)
		if kbErr != nil {
			if wrote {
				return true, fmt.Errorf("saved part of the changes; keybinding %s failed: %w", change.Action, kbErr)
			}
			return false, kbErr
		}
		conflicts += n
		wrote = true
	}
	a.configNotice = configSavedNotice(batch.configChanged, len(batch.keybindings), conflicts)
	return true, nil
}

func (a *App) applyKeybindingChange(ctx context.Context, change keybindingChange) (int, error) {
	if change.Reset {
		return 0, a.deps.Config.ResetKeybinding(ctx, port.ResetKeybindingRequest{
			RequestID: nextConfigRequestID(),
			Mode:      change.Mode,
			Action:    change.Action,
		})
	}
	resp, err := a.deps.Config.SetKeybinding(ctx, port.SetKeybindingRequest{
		RequestID: nextConfigRequestID(),
		Mode:      change.Mode,
		Action:    change.Action,
		Keys:      change.Keys,
	})
	return len(resp.Conflicts), err
}

func configSavedNotice(configChanged bool, keybindings, conflicts int) string {
	notice := "Settings saved"
	if !configChanged {
		notice = "Keybindings saved"
	} else if keybindings > 0 {
		notice += fmt.Sprintf(" · %d %s updated", keybindings, plural(keybindings, "keybinding"))
	}
	if conflicts > 0 {
		notice += fmt.Sprintf(" · %d %s", conflicts, plural(conflicts, "conflict"))
	}
	return notice
}

func formString(data map[string]string, key string, dst *string) {
	if value, ok := data[key]; ok {
		*dst = strings.TrimSpace(value)
	}
}

func formRequiredString(data map[string]string, key, label string, dst *string) error {
	value, ok := data[key]
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return &configFieldError{Field: key, Message: label + " is required"}
	}
	*dst = value
	return nil
}

// formCheckbox reads a checkbox rendered after a hidden "off" input with the
// same name: the last submitted value wins.
func formCheckbox(value string) bool {
	parts := strings.Split(value, ",")
	switch strings.ToLower(strings.TrimSpace(parts[len(parts)-1])) {
	case "on", "true", "1", "yes":
		return true
	default:
		return false
	}
}

func applyAppearanceFields(cfg *dto.WebUIConfig, data map[string]string) error {
	appearance := &cfg.Appearance
	// An empty font name means the engine default.
	formString(data, "sans_font", &appearance.SansFont)
	formString(data, "serif_font", &appearance.SerifFont)
	formString(data, "monospace_font", &appearance.MonospaceFont)
	formString(data, "gtk_font", &appearance.GtkFont)

	if raw, ok := data["default_font_size"]; ok {
		size, err := parseConfigInt(raw, "default font size")
		if err == nil && size <= 0 {
			err = fmt.Errorf("default font size must be positive")
		}
		if err != nil {
			return fieldError("default_font_size", err)
		}
		appearance.DefaultFontSize = size
	}
	if raw, ok := data["default_ui_scale"]; ok {
		scale, err := parseConfigFloat(raw, "UI scale")
		if err == nil && (scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0)) {
			err = fmt.Errorf("UI scale must be a finite positive number")
		}
		if err != nil {
			return fieldError("default_ui_scale", err)
		}
		cfg.DefaultUIScale = scale
	}
	if raw, ok := data["color_scheme"]; ok {
		scheme := strings.TrimSpace(raw)
		if !slices.Contains([]string{"default", "prefer-light", "prefer-dark"}, scheme) {
			return &configFieldError{Field: "color_scheme", Message: fmt.Sprintf("unknown color scheme: %q", scheme)}
		}
		appearance.ColorScheme = scheme
	}

	if raw, ok := data[configFieldExternalThemeEnabled]; ok {
		appearance.ExternalTheme.Enabled = formCheckbox(raw)
	}
	formString(data, "external_theme_provider", &appearance.ExternalTheme.Provider)
	formString(data, "external_theme_format", &appearance.ExternalTheme.Format)
	formString(data, "external_theme_path", &appearance.ExternalTheme.Path)

	if err := applyPaletteFields(&appearance.LightPalette, data, "light"); err != nil {
		return err
	}
	return applyPaletteFields(&appearance.DarkPalette, data, "dark")
}

func applyPaletteFields(palette *dto.ColorPalette, data map[string]string, prefix string) error {
	for _, field := range paletteFieldsOf(palette) {
		key := prefix + "_" + field.Key
		if err := formRequiredString(data, key, field.Label+" color", field.Value); err != nil {
			return err
		}
	}
	return nil
}

// paletteField names one palette color for both the form and the parser.
type paletteField struct {
	Key   string
	Label string
	Value *string
}

func paletteFieldsOf(palette *dto.ColorPalette) []paletteField {
	return []paletteField{
		{"background", "Background", &palette.Background},
		{"surface", "Surface", &palette.Surface},
		{"surface_variant", "Surface variant", &palette.SurfaceVariant},
		{"text", "Text", &palette.Text},
		{"muted", "Muted", &palette.Muted},
		{"accent", "Accent", &palette.Accent},
		{"border", "Border", &palette.Border},
	}
}

func applySearchFields(cfg *dto.WebUIConfig, data map[string]string) error {
	raw, ok := data["default_search_engine"]
	if !ok {
		return nil
	}
	value, err := requireSearchURLTemplate(raw, "default search engine")
	if err != nil {
		return fieldError("default_search_engine", err)
	}
	cfg.DefaultSearchEngine = value
	return nil
}

// searchShortcutForm is one submitted row of the search shortcut table.
// Original is empty for a new row.
type searchShortcutForm struct {
	index       string
	Original    string
	Key         string
	URL         string
	Description string
	Delete      bool
}

func (r searchShortcutForm) field(name string) string {
	return configShortcutPrefix + r.index + "." + name
}

func searchShortcutForms(data map[string]string) []searchShortcutForm {
	indexes := map[string]struct{}{}
	for key := range data {
		rest, ok := strings.CutPrefix(key, configShortcutPrefix)
		if !ok {
			continue
		}
		if index, _, found := strings.Cut(rest, "."); found {
			indexes[index] = struct{}{}
		}
	}
	ordered := slices.Collect(maps.Keys(indexes))
	sort.Slice(ordered, func(i, j int) bool {
		a, errA := strconv.Atoi(ordered[i])
		b, errB := strconv.Atoi(ordered[j])
		if errA != nil || errB != nil {
			return ordered[i] < ordered[j]
		}
		return a < b
	})

	rows := make([]searchShortcutForm, 0, len(ordered))
	for _, index := range ordered {
		row := searchShortcutForm{index: index}
		row.Original = strings.TrimSpace(data[row.field("original")])
		row.Key = strings.TrimSpace(data[row.field("key")])
		row.URL = strings.TrimSpace(data[row.field("url")])
		row.Description = strings.TrimSpace(data[row.field("description")])
		row.Delete = formCheckbox(data[row.field("delete")])
		rows = append(rows, row)
	}
	return rows
}

// applySearchShortcutRows applies the shortcut table. Stored shortcuts that
// have no submitted row are left untouched.
func applySearchShortcutRows(cfg *dto.WebUIConfig, data map[string]string) error {
	rows := searchShortcutForms(data)
	if len(rows) == 0 {
		return nil
	}
	shortcuts := cloneSearchShortcuts(cfg.SearchShortcuts)
	for _, row := range rows {
		if row.Original == "" {
			continue
		}
		if _, exists := shortcuts[row.Original]; !exists {
			return &configFieldError{Field: row.field("key"), Message: fmt.Sprintf("search shortcut %q no longer exists", row.Original)}
		}
		delete(shortcuts, row.Original)
	}
	for _, row := range rows {
		if row.Delete || (row.Original == "" && row.Key == "" && row.URL == "" && row.Description == "") {
			continue
		}
		if row.Key == "" {
			return &configFieldError{Field: row.field("key"), Message: "search shortcut key is required"}
		}
		value, err := requireSearchURLTemplate(row.URL, fmt.Sprintf("URL of search shortcut %q", row.Key))
		if err != nil {
			return fieldError(row.field("url"), err)
		}
		if _, exists := shortcuts[row.Key]; exists {
			return &configFieldError{Field: row.field("key"), Message: fmt.Sprintf("search shortcut %q is used twice", row.Key)}
		}
		shortcuts[row.Key] = dto.SearchShortcut{URL: value, Description: row.Description}
	}
	cfg.SearchShortcuts = shortcuts
	return nil
}

func applyPerformanceFields(cfg *dto.WebUIConfig, data map[string]string) error {
	raw, ok := data["profile"]
	if !ok {
		return nil
	}
	profile := strings.TrimSpace(raw)
	if !isAllowedPerformanceProfile(profile) {
		return &configFieldError{Field: "profile", Message: fmt.Sprintf("unknown performance profile: %q", profile)}
	}
	cfg.Performance.Profile = profile
	if profile != customPerformanceProfile {
		return nil
	}

	custom := &cfg.Performance.Custom
	for _, field := range []struct {
		key, label string
		minValue   int
		maxValue   int
		dst        *int
	}{
		{"skia_cpu_threads", "Skia CPU threads", 0, maxCustomPerformanceThreads, &custom.SkiaCPUThreads},
		{"skia_gpu_threads", "Skia GPU threads", -1, maxCustomPerformanceThreads, &custom.SkiaGPUThreads},
		{"web_process_memory_mb", "web process memory", 0, maxCustomPerformanceWebMemoryMB, &custom.WebProcessMemoryMB},
		{"network_process_memory_mb", "network process memory", 0, maxCustomPerformanceNetworkMemoryMB, &custom.NetworkProcessMemoryMB},
		{"webview_pool_prewarm", "WebView pool prewarm", 0, maxCustomPerformancePrewarm, &custom.WebViewPoolPrewarm},
	} {
		raw, ok := data[field.key]
		if !ok {
			continue
		}
		value, err := parseConfigInt(raw, field.label)
		if err == nil {
			err = validatePerformanceInt(value, field.label, field.minValue, field.maxValue)
		}
		if err != nil {
			return fieldError(field.key, err)
		}
		*field.dst = value
	}
	return nil
}

// keybindingChanges compares the submitted "keys:<mode>:<action>" fields with
// the stored bindings and plans one write per binding that changed.
func (a *App) keybindingChanges(ctx context.Context, data map[string]string) ([]keybindingChange, error) {
	fields := make([]string, 0)
	for key := range data {
		if strings.HasPrefix(key, configKeysPrefix) {
			fields = append(fields, key)
		}
	}
	if len(fields) == 0 {
		return nil, nil
	}
	sort.Strings(fields)

	keybindings, err := a.deps.Config.GetKeybindings(ctx)
	if err != nil {
		return nil, err
	}
	a.keybindings = keybindings
	bindings := map[string]renderedKeybinding{}
	for _, group := range configKeybindingGroups(keybindings) {
		for _, binding := range group.Bindings {
			bindings[keybindingFieldName(group.Mode, binding.Action)] = binding
		}
	}

	changes := make([]keybindingChange, 0)
	for _, field := range fields {
		binding, ok := bindings[field]
		if !ok {
			return nil, &configFieldError{Field: field, Message: "keybinding " + strings.TrimPrefix(field, configKeysPrefix) + " not found"}
		}
		mode, _, _ := strings.Cut(strings.TrimPrefix(field, configKeysPrefix), ":")
		keys := parseKeyList(data[field])
		switch {
		case sameKeys(keys, binding.Keys):
			continue
		case sameKeys(keys, binding.DefaultKeys):
			changes = append(changes, keybindingChange{Mode: mode, Action: binding.Action, Reset: true})
		default:
			changes = append(changes, keybindingChange{Mode: mode, Action: binding.Action, Keys: keys})
		}
	}
	return changes, nil
}

func keybindingFieldName(mode, action string) string {
	return configKeysPrefix + mode + ":" + action
}

// sameKeys compares key lists in order, treating spellings of the same chord
// ("Ctrl+H", "ctrl+h") as equal.
func sameKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if canonicalKeyString(a[i]) != canonicalKeyString(b[i]) {
			return false
		}
	}
	return true
}

func canonicalKeyString(key string) string {
	if canonical, ok := keychord.Canonicalize(key); ok {
		return canonical
	}
	return key
}

func (a *App) editableConfig(ctx context.Context) (dto.WebUIConfig, error) {
	cfg, err := a.deps.Config.Current(ctx)
	if err != nil {
		return dto.WebUIConfig{}, err
	}
	a.config = &cfg
	return webUIConfigFromPayload(cfg), nil
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
	maps.Copy(clone, shortcuts)
	return clone
}

func requireSearchURLTemplate(raw, label string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if !strings.Contains(value, "%s") {
		return "", fmt.Errorf("%s must include the %%s placeholder", label)
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

// parseKeyList splits the keys field ("ctrl+h, alt+h") into keys. Keys are
// separated by whitespace; a comma that ends a token is a separator, except
// when the comma is the key itself ("," or "ctrl+,").
func parseKeyList(raw string) []string {
	parts := strings.Fields(raw)
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed, ok := strings.CutSuffix(part, ","); ok && keyPartOf(trimmed) != "" {
			part = trimmed
		}
		keys = append(keys, part)
	}
	if len(keys) == 0 {
		return nil
	}
	return keys
}

// keyPartOf strips leading modifiers: "ctrl+shift+x" -> "x", "ctrl+" -> "".
func keyPartOf(key string) string {
	for {
		head, rest, found := strings.Cut(key, "+")
		if !found || rest == "" && head == "" {
			return key
		}
		switch strings.ToLower(head) {
		case "ctrl", "control", "alt", "shift":
			key = rest
		default:
			return key
		}
	}
}

// formatKeyList is the inverse of parseKeyList.
func formatKeyList(keys []string) string {
	return strings.Join(keys, ", ")
}
