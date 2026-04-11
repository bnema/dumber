package systemviews

import (
	"fmt"
	"html"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

func configHTML(cfg port.SystemviewConfigPayload, keybindings any) string {
	groups, bindings, loaded := summarizeKeybindings(keybindings)
	status := "not loaded"
	if loaded {
		status = "loaded"
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>systemviews config</title>
</head>
<body>
  <main id="app" data-route="config">
    <h1>Config</h1>
    <p>Engine: %s</p>
    <p>Default search engine: %s</p>
    <p>Keybindings: %s (%d %s, %d %s)</p>
  </main>
</body>
</html>`,
		html.EscapeString(cfg.EngineType),
		html.EscapeString(cfg.DefaultSearchEngine),
		status,
		groups, plural(groups, "group"),
		bindings, plural(bindings, "binding"),
	)
}

func summarizeKeybindings(keybindings any) (groups int, bindings int, loaded bool) {
	switch v := keybindings.(type) {
	case nil:
		return 0, 0, false
	case port.KeybindingsConfig:
		return countKeybindingGroups(v.Groups), countKeybindingBindings(v.Groups), true
	case *port.KeybindingsConfig:
		if v == nil {
			return 0, 0, false
		}
		return countKeybindingGroups(v.Groups), countKeybindingBindings(v.Groups), true
	case map[string]any:
		return countBridgeKeybindings(v)
	default:
		return 1, 0, true
	}
}

func countBridgeKeybindings(data map[string]any) (groups int, bindings int, loaded bool) {
	groupValue, ok := data["groups"]
	if !ok {
		return 1, 0, true
	}

	groupList, ok := groupValue.([]any)
	if !ok {
		return 1, 0, true
	}

	for _, group := range groupList {
		groupMap, ok := group.(map[string]any)
		if !ok {
			continue
		}
		groups++
		if bindingValue, ok := groupMap["bindings"]; ok {
			if bindingList, ok := bindingValue.([]any); ok {
				bindings += len(bindingList)
			}
		}
	}

	return groups, bindings, true
}

func countKeybindingGroups(groups []port.KeybindingGroup) int {
	count := 0
	for _, group := range groups {
		if group.Mode == "" && group.DisplayName == "" && len(group.Bindings) == 0 {
			continue
		}
		count++
	}
	return count
}

func countKeybindingBindings(groups []port.KeybindingGroup) int {
	count := 0
	for _, group := range groups {
		count += len(group.Bindings)
	}
	return count
}

func plural(count int, singular string) string {
	if count == 1 {
		return singular
	}
	return strings.TrimSpace(singular + "s")
}
