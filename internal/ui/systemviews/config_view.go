package systemviews

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
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

func keybindingGroupMeta(group renderedKeybindingGroup) string {
	meta := make([]string, 0, 2)
	if strings.TrimSpace(group.Mode) != "" {
		meta = append(meta, "mode: "+group.Mode)
	}
	if strings.TrimSpace(group.Activation) != "" {
		meta = append(meta, "activation: "+group.Activation)
	}
	return strings.Join(meta, " • ")
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

func keybindingStatus(binding renderedKeybinding) string {
	if keybindingIsCustom(binding) {
		return "custom"
	}
	return "default"
}

func keybindingKeysValue(keys []string) string {
	return strings.Join(keys, ", ")
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

func formatConfigInt(value int) string {
	return fmt.Sprintf("%d", value)
}

func formatConfigFloat(value float64) string {
	return fmt.Sprintf("%g", value)
}

func colorSchemeSelected(current, option string) bool {
	return strings.EqualFold(strings.TrimSpace(current), option)
}

func performanceProfileSelected(current, option string) bool {
	return strings.EqualFold(strings.TrimSpace(current), option)
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
