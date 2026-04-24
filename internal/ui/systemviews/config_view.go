package systemviews

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

type configRenderData struct {
	Config      port.SystemviewConfigPayload
	Keybindings any
	Notice      string
	Error       string
}

func configHTML(data configRenderData) string {
	return mustRenderComponent(ConfigView(data))
}

func configKeybindingSummary(keybindings any) string {
	groups, bindings, loaded := summarizeKeybindings(keybindings)
	status := "not loaded"
	if loaded {
		status = "loaded"
	}
	return fmt.Sprintf(
		"Keybindings %s (%d %s, %d %s)",
		status,
		groups,
		plural(groups, "group"),
		bindings,
		plural(bindings, "binding"),
	)
}

func configKeybindingGroups(keybindings any) []renderedKeybindingGroup {
	groups, ok := collectKeybindingGroups(keybindings)
	if !ok {
		return nil
	}
	return groups
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

func searchShortcutRows(shortcuts map[string]port.SearchShortcut) []searchShortcutRow {
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
	flattenValue(&rows, "", reflect.ValueOf(value))
	return rows
}

func flattenValue(rows *[]kvPair, path string, value reflect.Value) {
	if !value.IsValid() {
		appendFlatValue(rows, path, "null")
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
			nextPath := name
			if path != "" {
				nextPath = path + "." + name
			}
			flattenValue(rows, nextPath, value.Field(i))
		}
	case reflect.Map:
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
			nextPath := name
			if path != "" {
				nextPath = path + "." + name
			}
			flattenValue(rows, nextPath, value.MapIndex(key))
		}
	case reflect.Slice, reflect.Array:
		if value.Len() == 0 {
			appendFlatValue(rows, path, "[]")
			return
		}

		for i := 0; i < value.Len(); i++ {
			nextPath := fmt.Sprintf("%s[%d]", path, i)
			if path == "" {
				nextPath = fmt.Sprintf("[%d]", i)
			}
			flattenValue(rows, nextPath, value.Index(i))
		}
	default:
		appendFlatValue(rows, path, formatFlatValue(value))
	}
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
	return fmt.Sprint(value.Interface())
}

func summarizeKeybindings(keybindings any) (groups, bindings int, loaded bool) {
	renderedGroups, ok := collectKeybindingGroups(keybindings)
	if !ok {
		return 0, 0, false
	}

	for _, group := range renderedGroups {
		groups++
		bindings += len(group.Bindings)
	}

	return groups, bindings, true
}

func collectKeybindingGroups(keybindings any) ([]renderedKeybindingGroup, bool) {
	switch v := keybindings.(type) {
	case nil:
		return nil, false
	case port.KeybindingsConfig:
		return convertPortKeybindingGroups(v.Groups), true
	case *port.KeybindingsConfig:
		if v == nil {
			return nil, false
		}
		return convertPortKeybindingGroups(v.Groups), true
	case map[string]any:
		return convertBridgeKeybindingGroups(v)
	default:
		return nil, false
	}
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

func convertBridgeKeybindingGroups(data map[string]any) ([]renderedKeybindingGroup, bool) {
	rawGroups, ok := data["groups"]
	if !ok || rawGroups == nil {
		return nil, false
	}

	groupValues, ok := anySlice(rawGroups)
	if !ok {
		return nil, false
	}

	rendered := make([]renderedKeybindingGroup, 0, len(groupValues))
	for _, rawGroup := range groupValues {
		groupMap, ok := rawGroup.(map[string]any)
		if !ok {
			return nil, false
		}
		group, ok := convertBridgeKeybindingGroup(groupMap)
		if !ok {
			return nil, false
		}
		rendered = append(rendered, group)
	}

	return rendered, true
}

func convertBridgeKeybindingGroup(group map[string]any) (renderedKeybindingGroup, bool) {
	bindings := []renderedKeybinding{}
	if rawBindings, ok := group["bindings"]; ok && rawBindings != nil {
		converted, ok := convertBridgeKeybindingEntries(rawBindings)
		if !ok {
			return renderedKeybindingGroup{}, false
		}
		bindings = converted
	}

	return renderedKeybindingGroup{
		DisplayName: strings.TrimSpace(stringValue(group["display_name"])),
		Mode:        strings.TrimSpace(stringValue(group["mode"])),
		Activation:  strings.TrimSpace(stringValue(group["activation"])),
		Bindings:    bindings,
	}, true
}

func convertBridgeKeybindingEntries(rawBindings any) ([]renderedKeybinding, bool) {
	bindingValues, ok := anySlice(rawBindings)
	if !ok {
		return nil, false
	}

	rendered := make([]renderedKeybinding, 0, len(bindingValues))
	for _, rawBinding := range bindingValues {
		bindingMap, ok := rawBinding.(map[string]any)
		if !ok {
			return nil, false
		}
		rendered = append(rendered, renderedKeybinding{
			Description: strings.TrimSpace(stringValue(bindingMap["description"])),
			Action:      strings.TrimSpace(stringValue(bindingMap["action"])),
			Keys:        stringSlice(bindingMap["keys"]),
			DefaultKeys: stringSlice(bindingMap["default_keys"]),
			IsCustom:    boolValue(bindingMap["is_custom"]),
		})
	}

	return rendered, true
}

func anySlice(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case []map[string]any:
		items := make([]any, len(v))
		for i, item := range v {
			items[i] = item
		}
		return items, true
	default:
		return nil, false
	}
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func stringSlice(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, text)
		}
		return out
	default:
		return nil
	}
}

func boolValue(value any) bool {
	b, _ := value.(bool)
	return b
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
	return strings.TrimSpace(singular + "s")
}
