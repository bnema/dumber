package systemviews

import (
	"fmt"
	"html"
	"reflect"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

func configHTML(cfg port.SystemviewConfigPayload, keybindings any) string {
	groups, bindings, loaded := summarizeKeybindings(keybindings)
	status := "not loaded"
	if loaded {
		status = "loaded"
	}
	keybindingSummary := fmt.Sprintf(
		"Keybindings %s (%d %s, %d %s)",
		status,
		groups,
		plural(groups, "group"),
		bindings,
		plural(bindings, "binding"),
	)
	content := []string{
		metaHTML(keybindingSummary),
		"<h3>Config values</h3>" + kvHTML(flattenValues(cfg)),
		"<h3>Keybindings</h3>" + renderKeybindingsSection(keybindings),
	}

	return sectionHTML("", "Config", strings.Join(content, ""))
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

func renderKeybindingsSection(keybindings any) string {
	groups, ok := collectKeybindingGroups(keybindings)
	if !ok {
		return emptyStateHTML("Keybindings unavailable")
	}
	if len(groups) == 0 {
		return emptyStateHTML("No keybindings configured")
	}

	return renderKeybindingGroups(groups)
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

func renderKeybindingGroups(groups []renderedKeybindingGroup) string {
	var rendered strings.Builder
	for _, group := range groups {
		rendered.WriteString(renderKeybindingGroup(group))
	}
	return rendered.String()
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

func renderKeybindingGroup(group renderedKeybindingGroup) string {
	title := group.DisplayName
	if strings.TrimSpace(title) == "" {
		title = group.Mode
	}
	if strings.TrimSpace(title) == "" {
		title = "Keybinding group"
	}

	var body strings.Builder
	body.WriteString(`<div class="sv-group">`)
	body.WriteString(`<div class="sv-group-header">`)
	body.WriteString(`<div>`)
	body.WriteString(`<h4>` + html.EscapeString(title) + `</h4>`)

	meta := make([]string, 0, 2)
	if strings.TrimSpace(group.Mode) != "" {
		meta = append(meta, "mode: "+group.Mode)
	}
	if strings.TrimSpace(group.Activation) != "" {
		meta = append(meta, "activation: "+group.Activation)
	}
	if len(meta) > 0 {
		body.WriteString(`<p class="sv-meta">` + html.EscapeString(strings.Join(meta, " • ")) + `</p>`)
	}
	body.WriteString(`</div></div>`)
	body.WriteString(`<div class="sv-group-body">`)
	if len(group.Bindings) == 0 {
		body.WriteString(emptyStateHTML("No bindings in this group"))
	} else {
		for _, binding := range group.Bindings {
			body.WriteString(renderKeybinding(binding))
		}
	}
	body.WriteString(`</div></div>`)

	return body.String()
}

func renderKeybinding(binding renderedKeybinding) string {
	description := strings.TrimSpace(binding.Description)
	if description == "" {
		description = strings.TrimSpace(binding.Action)
	}
	if description == "" {
		description = "Unnamed binding"
	}

	action := strings.TrimSpace(binding.Action)
	if action == "" {
		action = "binding"
	}

	custom := binding.IsCustom || !sameStringSlice(binding.Keys, binding.DefaultKeys)
	status := "default"
	if custom {
		status = "custom"
	}

	var body strings.Builder
	body.WriteString(`<div class="sv-binding">`)
	body.WriteString(`<div class="sv-binding-header">`)
	body.WriteString(`<div class="sv-binding-description">` + html.EscapeString(description) + `</div>`)
	body.WriteString(`<div class="sv-binding-keys">` + keyListHTML(binding.Keys) + `</div>`)
	body.WriteString(`</div>`)
	body.WriteString(`<div class="sv-binding-meta">`)
	body.WriteString(`<span>` + html.EscapeString("action: "+action) + `</span>`)
	body.WriteString(`<span>` + html.EscapeString(status) + `</span>`)
	if custom && len(binding.DefaultKeys) > 0 {
		body.WriteString(`<span>default: ` + keyListHTML(binding.DefaultKeys) + `</span>`)
	}
	body.WriteString(`</div></div>`)

	return body.String()
}

func keyListHTML(keys []string) string {
	if len(keys) == 0 {
		return `<span class="sv-key sv-key-empty">Unassigned</span>`
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, `<span class="sv-key">`+html.EscapeString(key)+`</span>`)
	}
	return strings.Join(parts, " ")
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
