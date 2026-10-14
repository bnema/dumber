package config

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/logging"
)

// actionMapSpec describes one configurable action map. It is the single source
// of truth for the list of action maps shared by load-time completion and the
// config migrator.
type actionMapSpec struct {
	// key is the dotted config path of the action map.
	key string
	// actions returns a pointer to the action map inside a Config.
	actions func(*Config) *map[string]ActionBinding
	// canonKey canonicalizes a key sequence the way the validator does for
	// duplicate detection. nil means keys are compared verbatim.
	canonKey func(string) (string, error)
}

func actionMapSpecs() []actionMapSpec {
	return []actionMapSpec{
		{key: "workspace.shortcuts.actions", actions: func(c *Config) *map[string]ActionBinding { return &c.Workspace.Shortcuts.Actions }},
		{key: "workspace.prefix_mode.actions", actions: func(c *Config) *map[string]ActionBinding { return &c.Workspace.PrefixMode.Actions }},
		{key: "workspace.pane_mode.actions", actions: func(c *Config) *map[string]ActionBinding { return &c.Workspace.PaneMode.Actions }},
		{key: "workspace.tab_mode.actions", actions: func(c *Config) *map[string]ActionBinding { return &c.Workspace.TabMode.Actions }},
		{
			key:      "workspace.vim_mode.actions",
			actions:  func(c *Config) *map[string]ActionBinding { return &c.Workspace.VimMode.Actions },
			canonKey: canonicalizeVimModeSequenceKey,
		},
		{key: "workspace.resize_mode.actions", actions: func(c *Config) *map[string]ActionBinding { return &c.Workspace.ResizeMode.Actions }},
		{key: "session.session_mode.actions", actions: func(c *Config) *map[string]ActionBinding { return &c.Session.SessionMode.Actions }},
	}
}

// legacyActionName returns the underscore spelling used by older configs.
func legacyActionName(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// skippedDefaultAction is a default action not added because a user binding wins.
type skippedDefaultAction struct {
	name     string
	sequence string
	by       string
}

// missingDefaultActions returns the sorted names of default actions that the
// user did not configure (by canonical or legacy underscore name) and whose key
// sequences are not already used by a user action. userKeys maps each user
// action name to its keys; an action with no keys (disabled) is still present,
// so it is never re-added.
func (s actionMapSpec) missingDefaultActions(
	defaults map[string]ActionBinding,
	userKeys map[string][]string,
) (missing []string, skipped []skippedDefaultAction) {
	canon := s.canonKey
	if canon == nil {
		canon = func(key string) (string, error) { return key, nil }
	}
	canonOrRaw := func(key string) string {
		if c, err := canon(key); err == nil {
			return c
		}
		return key
	}

	used := make(map[string]string)
	userNames := make([]string, 0, len(userKeys))
	for name := range userKeys {
		userNames = append(userNames, name)
	}
	sort.Strings(userNames)
	for _, name := range userNames {
		for _, key := range userKeys[name] {
			used[canonOrRaw(key)] = name
		}
	}

	defaultNames := make([]string, 0, len(defaults))
	for name := range defaults {
		defaultNames = append(defaultNames, name)
	}
	sort.Strings(defaultNames)

	for _, name := range defaultNames {
		if _, ok := userKeys[name]; ok {
			continue
		}
		if _, ok := userKeys[legacyActionName(name)]; ok {
			continue
		}
		conflict := false
		for _, key := range defaults[name].Keys {
			sequence := canonOrRaw(key)
			if by, ok := used[sequence]; ok {
				skipped = append(skipped, skippedDefaultAction{name: name, sequence: sequence, by: by})
				conflict = true
				break
			}
		}
		if !conflict {
			missing = append(missing, name)
		}
	}
	return missing, skipped
}

// completeDefaultActions fills every action map of cfg with the default
// actions the user did not configure. User actions always win: a default is
// skipped when one of its key sequences is already used by a user action in the
// same map. Maps are replaced, never mutated, so copies of cfg sharing them are
// unaffected. The config file is not touched.
func completeDefaultActions(cfg *Config) {
	if cfg == nil {
		return
	}
	defaults := DefaultConfig()
	var skippedAll []string
	for _, spec := range actionMapSpecs() {
		userActions := *spec.actions(cfg)
		userKeys := make(map[string][]string, len(userActions))
		for name, binding := range userActions {
			userKeys[name] = binding.Keys
		}

		defaultActions := *spec.actions(defaults)
		missing, skipped := spec.missingDefaultActions(defaultActions, userKeys)

		merged := make(map[string]ActionBinding, len(userActions)+len(missing))
		for name, binding := range userActions {
			merged[name] = binding
		}
		for _, name := range missing {
			binding := defaultActions[name]
			binding.Keys = slices.Clone(binding.Keys)
			merged[name] = binding
		}
		*spec.actions(cfg) = merged

		for _, s := range skipped {
			skippedAll = append(skippedAll, fmt.Sprintf("%s.%s (key '%s' used by %s)", spec.key, s.name, s.sequence, s.by))
		}
	}
	if len(skippedAll) > 0 {
		log := logging.NewFromEnv()
		log.Debug().Strs("skipped", skippedAll).Msg("config: default actions not added because user bindings take precedence")
	}
}
