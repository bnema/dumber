package systemviews

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/keychord"
)

type configRenderData struct {
	Config      dto.SystemviewConfigPayload
	Keybindings port.KeybindingsConfig
	Notice      string
	Error       string
}

// maxFlattenDepth caps recursive config flattening to avoid runaway nesting.
const maxFlattenDepth = 16

func configHTML(data configRenderData) string {
	return mustRenderComponent(ConfigView(data))
}

func configKeybindingSummary(keybindings port.KeybindingsConfig) string {
	if keybindings.Groups == nil {
		return "Keybindings unavailable"
	}
	groups, bindings := summarizeKeybindings(keybindings)
	return fmt.Sprintf(
		"Keybindings loaded (%d %s, %d %s)",
		groups,
		plural(groups, "group"),
		bindings,
		plural(bindings, "binding"),
	)
}

func configKeybindingGroups(keybindings port.KeybindingsConfig) []renderedKeybindingGroup {
	if keybindings.Groups == nil {
		return nil
	}
	return convertPortKeybindingGroups(keybindings.Groups)
}

func keybindingGroupTitle(group renderedKeybindingGroup) string {
	title := strings.TrimSpace(group.DisplayName)
	if title == "" {
		title = strings.TrimSpace(group.Mode)
	}
	if title == "" {
		return "Keybinding group"
	}
	return title
}

func keybindingDescription(binding renderedKeybinding) string {
	description := strings.TrimSpace(binding.Description)
	if description != "" {
		return description
	}
	if strings.TrimSpace(binding.Action) != "" {
		return strings.TrimSpace(binding.Action)
	}
	return "Unnamed binding"
}

func keybindingAction(binding renderedKeybinding) string {
	action := strings.TrimSpace(binding.Action)
	if action == "" {
		return "binding"
	}
	return action
}

func keybindingIsCustom(binding renderedKeybinding) bool {
	return binding.IsCustom || !sameStringSlice(binding.Keys, binding.DefaultKeys)
}

type searchShortcutRow struct {
	Key         string
	URL         string
	Description string
}

func searchShortcutRows(shortcuts map[string]dto.SearchShortcut) []searchShortcutRow {
	if len(shortcuts) == 0 {
		return nil
	}
	keys := make([]string, 0, len(shortcuts))
	for key := range shortcuts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([]searchShortcutRow, 0, len(keys))
	for _, key := range keys {
		shortcut := shortcuts[key]
		rows = append(rows, searchShortcutRow{
			Key:         key,
			URL:         shortcut.URL,
			Description: shortcut.Description,
		})
	}
	return rows
}

// configSection is one entry of the settings sidebar.
type configSection struct {
	ID    string
	Label string
}

var configSections = []configSection{
	{"sv-config-appearance", "Appearance"},
	{"sv-config-search", "Search"},
	{"sv-config-performance", "Performance"},
	{"sv-config-keybindings", "Keybindings"},
	{"sv-config-advanced", "Advanced"},
}

// segmentOption is one choice of a segmented control.
type segmentOption struct {
	Value string
	Label string
}

var colorSchemeOptions = []segmentOption{
	{"default", "System"},
	{"prefer-light", "Light"},
	{"prefer-dark", "Dark"},
}

var performanceProfileOptions = []segmentOption{
	{"default", "Default"},
	{balancedPerformanceProfile, "Balanced"},
	{"lite", "Lite"},
	{"max", "Max"},
	{customPerformanceProfile, "Custom"},
}

// normalizedOption matches a stored option value case-insensitively.
func normalizedOption(value, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	return value
}

type configFontField struct {
	Name  string
	Label string
	Help  string
	Value string
}

func configFontFields(appearance dto.WebUIAppearanceConfig) []configFontField {
	return []configFontField{
		{"sans_font", "Sans-serif", "Default font for page text.", appearance.SansFont},
		{"serif_font", "Serif", "Used when a page asks for a serif font.", appearance.SerifFont},
		{"monospace_font", "Monospace", "Used for code.", appearance.MonospaceFont},
		{"gtk_font", "Interface", "Font of Dumber's own interface.", appearance.GtkFont},
	}
}

// palettePreviewVars feeds the palette preview card. Invalid colors are left
// out so the card falls back to the page theme.
func palettePreviewVars(palette dto.ColorPalette) string {
	parts := make([]string, 0, 7)
	for _, field := range paletteFieldsOf(&palette) {
		if value, ok := sanitizeCSSColor(*field.Value); ok {
			parts = append(parts, fmt.Sprintf("--pp-%s: %s;", strings.ReplaceAll(field.Key, "_", "-"), value))
		}
	}
	return strings.Join(parts, " ")
}

func swatchStyle(value string) string {
	if color, ok := sanitizeCSSColor(value); ok {
		return "--sv-swatch: " + color + ";"
	}
	return ""
}

// colorPickerValue converts a stored color to the #rrggbb form required by
// <input type="color">.
func colorPickerValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	hex, ok := strings.CutPrefix(value, "#")
	if !ok {
		return "#000000"
	}
	for _, r := range hex {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return "#000000"
		}
	}
	const short, shortAlpha, long, longAlpha = 3, 4, 6, 8
	switch len(hex) {
	case short, shortAlpha:
		return "#" + string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case long, longAlpha:
		return "#" + hex[:long]
	default:
		return "#000000"
	}
}

// externalThemeFormats lists the known formats. The stored value is always
// an option, even empty or unknown, so an untouched form saves it unchanged.
func externalThemeFormats(current string) []string {
	formats := []string{"colors-json", "dumber-json"}
	if !slices.Contains(formats, current) {
		formats = append([]string{current}, formats...)
	}
	return formats
}

func externalThemeFormatLabel(format string) string {
	if format == "" {
		return "Not set"
	}
	return format
}

func shortcutPlaceholder(isNew bool, field string) string {
	if !isNew {
		return ""
	}
	switch field {
	case "key":
		return "New key"
	case "url":
		return "https://example.com/search?q=%s"
	default:
		return "Description"
	}
}

func performanceDescription(engine string) string {
	if strings.TrimSpace(engine) != "" && !engineIsWebKit(engine) {
		return "Profiles tune the WebKit engine; they have no effect on " + engine + "."
	}
	return "How much CPU, GPU and memory web pages may use. Changes apply after a restart."
}

// hardwareSummary reads "8 cores · 16 threads · 32 GB RAM · Radeon (4 GB)".
func hardwareSummary(config dto.SystemviewConfigPayload) string {
	hw := config.Performance.Hardware
	parts := make([]string, 0, 5)
	if hw.CPUCores > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hw.CPUCores, plural(hw.CPUCores, "core")))
	}
	if hw.CPUThreads > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hw.CPUThreads, plural(hw.CPUThreads, "thread")))
	}
	if hw.TotalRAMMB > 0 {
		parts = append(parts, formatMegabytes(hw.TotalRAMMB)+" RAM")
	}
	if gpu := strings.TrimSpace(hw.GPUVendor + " " + hw.GPUName); gpu != "" {
		if hw.VRAMMB > 0 {
			gpu += " (" + formatMegabytes(hw.VRAMMB) + ")"
		}
		parts = append(parts, gpu)
	}
	if engine := strings.TrimSpace(config.EngineType); engine != "" {
		parts = append(parts, engine+" engine")
	}
	if len(parts) == 0 {
		return "Not detected"
	}
	return strings.Join(parts, " · ")
}

const megabytesPerGigabyte = 1024

// formatMegabytes reads "512 MB" or "15.6 GB" (one decimal).
func formatMegabytes(mb int) string {
	if mb >= megabytesPerGigabyte {
		return fmt.Sprintf("%g GB", math.Round(float64(mb)*10/megabytesPerGigabyte)/10)
	}
	return fmt.Sprintf("%d MB", mb)
}

type performanceCustomField struct {
	Name     string
	Label    string
	Help     string
	Min, Max int
	Value    int
}

func performanceCustomFields(custom dto.SystemviewCustomPerformancePayload) []performanceCustomField {
	return []performanceCustomField{
		{"skia_cpu_threads", "Skia CPU threads", "Raster threads. 0 lets the engine decide.",
			0, maxCustomPerformanceThreads, custom.SkiaCPUThreads},
		{"skia_gpu_threads", "Skia GPU threads", "-1 lets the engine decide.",
			-1, maxCustomPerformanceThreads, custom.SkiaGPUThreads},
		{"web_process_memory_mb", "Web process memory", "Limit per page process, in MB. 0 means no limit.",
			0, maxCustomPerformanceWebMemoryMB, custom.WebProcessMemoryMB},
		{"network_process_memory_mb", "Network process memory", "Limit in MB. 0 means no limit.",
			0, maxCustomPerformanceNetworkMemoryMB, custom.NetworkProcessMemoryMB},
		{"webview_pool_prewarm", "Prewarmed views", "Views kept ready so new tabs open instantly.",
			0, maxCustomPerformancePrewarm, custom.WebViewPoolPrewarm},
	}
}

func keybindingInputID(mode, action string) string {
	return "sv-kb-" + cssIdentifier(mode) + "-" + cssIdentifier(action)
}

// cssIdentifier keeps letters, digits, '-' and '_' so a value is safe in an id.
func cssIdentifier(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// keybindingGroupRecordable reports whether every key of the group is a
// single chord, which the key recorder can capture. Groups that use key
// sequences (vim: "gi", "<C-d>", "[[") are edited as text only.
func keybindingGroupRecordable(group renderedKeybindingGroup) bool {
	for _, binding := range group.Bindings {
		for _, keys := range [][]string{binding.Keys, binding.DefaultKeys} {
			for _, key := range keys {
				if _, ok := keychord.Parse(key); !ok {
					return false
				}
			}
		}
	}
	return true
}

func keybindingDefaultLabel(binding renderedKeybinding) string {
	if len(binding.DefaultKeys) == 0 {
		return "unassigned"
	}
	return formatKeyList(binding.DefaultKeys)
}

func formatConfigInt(value int) string {
	return fmt.Sprintf("%d", value)
}

func formatConfigFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

func engineIsWebKit(engine string) bool {
	return strings.EqualFold(strings.TrimSpace(engine), "webkit")
}

func flattenValues(value any) []kvPair {
	rows := make([]kvPair, 0)
	flattenValue(&rows, "", reflect.ValueOf(value), 0)
	return rows
}

func flattenValue(rows *[]kvPair, path string, value reflect.Value, depth int) {
	if !value.IsValid() {
		appendFlatValue(rows, path, "null")
		return
	}
	if depth >= maxFlattenDepth {
		appendFlatValue(rows, path, "...")
		return
	}

	for value.Kind() == reflect.Interface {
		if value.IsNil() {
			appendFlatValue(rows, path, "null")
			return
		}
		value = value.Elem()
	}

	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			appendFlatValue(rows, path, "null")
			return
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Struct:
		flattenStructValue(rows, path, value, depth)
	case reflect.Map:
		flattenMapValue(rows, path, value, depth)
	case reflect.Slice, reflect.Array:
		flattenListValue(rows, path, value, depth)
	default:
		appendFlatValue(rows, path, formatFlatValue(value))
	}
}

func flattenStructValue(rows *[]kvPair, path string, value reflect.Value, depth int) {
	typeOfValue := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := typeOfValue.Field(i)
		if !field.IsExported() {
			continue
		}
		name := jsonFieldName(field)
		if name == "-" {
			continue
		}
		flattenValue(rows, childFlatPath(path, name), value.Field(i), depth+1)
	}
}

func flattenMapValue(rows *[]kvPair, path string, value reflect.Value, depth int) {
	if value.Len() == 0 {
		appendFlatValue(rows, path, "{}")
		return
	}

	keys := value.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return mapKeyString(keys[i]) < mapKeyString(keys[j])
	})

	for _, key := range keys {
		name := mapKeyString(key)
		flattenValue(rows, childFlatPath(path, name), value.MapIndex(key), depth+1)
	}
}

func flattenListValue(rows *[]kvPair, path string, value reflect.Value, depth int) {
	if value.Len() == 0 {
		appendFlatValue(rows, path, "[]")
		return
	}

	for i := 0; i < value.Len(); i++ {
		nextPath := fmt.Sprintf("%s[%d]", path, i)
		if path == "" {
			nextPath = fmt.Sprintf("[%d]", i)
		}
		flattenValue(rows, nextPath, value.Index(i), depth+1)
	}
}

func childFlatPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func appendFlatValue(rows *[]kvPair, path, value string) {
	if strings.TrimSpace(path) == "" {
		path = "value"
	}
	*rows = append(*rows, kvPair{Label: path, Value: value})
}

func formatFlatValue(value reflect.Value) string {
	if !value.IsValid() {
		return "null"
	}

	switch value.Kind() {
	case reflect.String:
		return value.String()
	case reflect.Bool:
		return fmt.Sprint(value.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprint(value.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprint(value.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprint(value.Float())
	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprint(value.Complex())
	default:
		if !value.CanInterface() {
			return "<unexported>"
		}
		return fmt.Sprint(value.Interface())
	}
}

func jsonFieldName(field reflect.StructField) string {
	name := field.Tag.Get("json")
	if name == "" {
		return field.Name
	}
	if idx := strings.Index(name, ","); idx >= 0 {
		name = name[:idx]
	}
	if name == "" {
		return field.Name
	}
	return name
}

func mapKeyString(value reflect.Value) string {
	if value.Kind() == reflect.String {
		return value.String()
	}
	if value.CanInterface() {
		return fmt.Sprint(value.Interface())
	}
	return formatFlatValue(value)
}

func summarizeKeybindings(keybindings port.KeybindingsConfig) (groups, bindings int) {
	for _, group := range keybindings.Groups {
		groups++
		bindings += len(group.Bindings)
	}
	return groups, bindings
}

func convertPortKeybindingGroups(groups []port.KeybindingGroup) []renderedKeybindingGroup {
	rendered := make([]renderedKeybindingGroup, 0, len(groups))
	for _, group := range groups {
		rendered = append(rendered, renderedKeybindingGroup{
			DisplayName: strings.TrimSpace(group.DisplayName),
			Mode:        strings.TrimSpace(group.Mode),
			Activation:  strings.TrimSpace(group.Activation),
			Bindings:    convertPortKeybindingEntries(group.Bindings),
		})
	}
	return rendered
}

func convertPortKeybindingEntries(bindings []port.KeybindingEntry) []renderedKeybinding {
	rendered := make([]renderedKeybinding, 0, len(bindings))
	for _, binding := range bindings {
		rendered = append(rendered, renderedKeybinding{
			Description: strings.TrimSpace(binding.Description),
			Action:      strings.TrimSpace(binding.Action),
			Keys:        append([]string(nil), binding.Keys...),
			DefaultKeys: append([]string(nil), binding.DefaultKeys...),
			IsCustom:    binding.IsCustom,
		})
	}
	return rendered
}

type renderedKeybindingGroup struct {
	DisplayName string
	Mode        string
	Activation  string
	Bindings    []renderedKeybinding
}

type renderedKeybinding struct {
	Description string
	Action      string
	Keys        []string
	DefaultKeys []string
	IsCustom    bool
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func plural(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}
