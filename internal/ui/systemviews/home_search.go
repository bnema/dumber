package systemviews

import (
	"encoding/json"

	"github.com/a-h/templ"
	"sort"
	"strings"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	browserurl "bnema.dev/dumber/internal/domain/url"
)

// homeSearchConfig is the engine data the search widget needs. It is
// rendered on the search form, so the page resolves queries without a
// round trip.
type homeSearchConfig struct {
	EngineURL  string
	EngineName string
	Shortcuts  map[string]dto.HomeSearchShortcut
}

func homeSearchConfigFrom(load dto.HomeLoadResponse) homeSearchConfig {
	return homeSearchConfig{EngineURL: load.SearchEngine, EngineName: load.SearchEngineName, Shortcuts: load.SearchShortcuts}
}

// homeShortcutsJSON encodes shortcuts for the search form attribute.
func homeShortcutsJSON(shortcuts map[string]dto.HomeSearchShortcut) string {
	if len(shortcuts) == 0 {
		return "{}"
	}
	raw, err := json.Marshal(shortcuts)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// parseHomeShortcutsJSON decodes the search form attribute; bad input
// means no shortcuts.
func parseHomeShortcutsJSON(raw string) map[string]dto.HomeSearchShortcut {
	var shortcuts map[string]dto.HomeSearchShortcut
	if json.Unmarshal([]byte(raw), &shortcuts) != nil {
		return nil
	}
	return shortcuts
}

// Engine chip states.
const (
	homeChipIdle    = "idle"
	homeChipWeb     = "web"
	homeChipBang    = "bang"
	homeChipAddress = "address"
)

// homeHintKind selects a hint line.
type homeHintKind string

const (
	homeHintIdle        homeHintKind = "idle"
	homeHintOpen        homeHintKind = "open"
	homeHintWeb         homeHintKind = "web"
	homeHintBangSearch  homeHintKind = "bang_search"
	homeHintBangType    homeHintKind = "bang_type"
	homeHintCandidates  homeHintKind = "candidates"
	homeHintUnknown     homeHintKind = "unknown"
	homeHintOpenAddress homeHintKind = "open_address"
	// homeHintSearchFallback: the selected row has no url, so Enter
	// searches the default engine.
	homeHintSearchFallback homeHintKind = "search_fallback"
)

// homeHintCandidate is one shortcut offered for a partial bang.
type homeHintCandidate struct {
	Key  string
	Name string
}

// homeIdleShortcutLimit caps the shortcut keys named by the idle hint.
const homeIdleShortcutLimit = 3

// homeSearchView is what the search shows for a query: which items stay
// visible, which one is selected, the engine chip and the hint line.
type homeSearchView struct {
	Result homeQueryResult
	// Filtering hides the items whose key is not in Result.Matches.
	Filtering bool
	// Selected is the key of the selected item, or "". SelectedIndex is
	// its index among the selectable matches and suggestions, or -1;
	// Selectable holds their identities (see homeItemIdentity).
	Selected      string
	SelectedIndex int
	Selectable    []string
	SelectedName  string
	SelectedURL   string

	ChipState string
	ChipName  string
	ChipIcon  string

	Hint       homeHintKind
	HintQuery  string
	Candidates []homeHintCandidate
	Shortcuts  []string
	// HideSuggestions hides the history suggestions (bang modes).
	HideSuggestions bool
}

// homeItemIdentity identifies a selectable row across re-renders, where
// keys (DOM indexes) change: its url and name.
func homeItemIdentity(item homeQueryItem) string {
	return item.URL + "\n" + item.Name
}

// buildHomeSearchView resolves query against items. Matching items, then
// history suggestions, are selectable; selected is the identity of the
// selection, or "" for the default (the first match, if any). A selection
// that is gone falls back to the default.
func buildHomeSearchView(config homeSearchConfig, query string, items, suggestions []homeQueryItem, selected string) homeSearchView {
	res := resolveHomeQuery(homeQueryInput{
		Query: query, EngineURL: config.EngineURL, EngineName: config.EngineName,
		Shortcuts: config.Shortcuts, Items: items,
	})
	view := homeSearchView{Result: res, ChipName: res.EngineName, ChipIcon: homeFaviconURL(res.EngineURL), SelectedIndex: -1}
	switch res.Mode {
	case homeQueryIdle:
		view.ChipState = homeChipIdle
		view.Hint = homeHintIdle
		view.Shortcuts = homeIdleShortcuts(config.Shortcuts)
	case homeQueryFilter:
		view.Filtering = true
		view.ChipState = homeChipIdle
		view.Hint = homeHintOpen
		view.HintQuery = strings.TrimSpace(query)
	case homeQueryWeb:
		view.Filtering = true
		view.ChipState = homeChipWeb
		view.Hint = homeHintWeb
		view.HintQuery = strings.TrimSpace(query)
	case homeQueryAddress:
		view.Filtering = true
		view.ChipState = homeChipAddress
		view.ChipName = "Address"
		view.ChipIcon = ""
		view.Hint = homeHintOpenAddress
	case homeQueryBang:
		view.ChipState = homeChipBang
		view.HideSuggestions = true
		_, rest, _ := browserurl.ParseBangPrefix(strings.TrimLeft(query, " \t"))
		view.HintQuery = strings.TrimSpace(rest)
		view.Hint = homeHintBangType
		if view.HintQuery != "" {
			view.Hint = homeHintBangSearch
		}
	case homeQueryBangPartial:
		view.ChipState = homeChipIdle
		view.HideSuggestions = true
		view.Hint = homeHintCandidates
		for _, key := range res.Candidates {
			view.Candidates = append(view.Candidates, homeHintCandidate{Key: key, Name: config.Shortcuts[key].Name})
		}
	case homeQueryBangUnknown:
		view.ChipState = homeChipIdle
		view.HideSuggestions = true
		view.Hint = homeHintUnknown
		view.HintQuery = res.BangQuery
	}
	if view.Filtering {
		view.selectAt(items, suggestions, selected)
	}
	return view
}

// selectAt selects the row whose identity is selected among the matches
// then the suggestions; otherwise the first match, or nothing without
// matches.
func (v *homeSearchView) selectAt(items, suggestions []homeQueryItem, selected string) {
	byKey := make(map[string]homeQueryItem, len(items))
	for _, item := range items {
		byKey[item.Key] = item
	}
	selectable := make([]homeQueryItem, 0, len(v.Result.Matches)+len(suggestions))
	for _, key := range v.Result.Matches {
		selectable = append(selectable, byKey[key])
	}
	selectable = append(selectable, suggestions...)
	i := -1
	for j, item := range selectable {
		id := homeItemIdentity(item)
		v.Selectable = append(v.Selectable, id)
		if selected != "" && id == selected && i < 0 {
			i = j
		}
	}
	if i < 0 {
		if len(v.Result.Matches) == 0 {
			return
		}
		i = 0
	}
	item := selectable[i]
	v.SelectedIndex = i
	v.Selected = item.Key
	v.SelectedName = item.Name
	v.SelectedURL = homeNavigable(item.URL)
	switch {
	case v.SelectedURL == "" && v.Hint != homeHintOpenAddress:
		// Nothing to open (a service without url): Enter searches.
		v.Hint = homeHintSearchFallback
	case v.Hint == homeHintWeb:
		// A suggestion picked with the arrows: Enter opens it.
		v.Hint = homeHintOpen
	}
}

// MoveSelection returns the identity of the selection after an arrow key,
// clamped to the selectable rows; "" when there is nothing to select.
func (v homeSearchView) MoveSelection(down bool) string {
	if len(v.Selectable) == 0 {
		return ""
	}
	next := v.SelectedIndex - 1
	if down {
		next = v.SelectedIndex + 1
	}
	return v.Selectable[max(0, min(next, len(v.Selectable)-1))]
}

// Enter returns what Enter opens: the selected item, else the resolved
// target, else the web search. A selected row without url searches the
// web. Shift+Enter opens the web search or the typed address, except in
// bang modes, where it does what Enter does.
func (v homeSearchView) Enter(shift bool) string {
	if v.bangMode() {
		// The bang engine, or nothing while its query is empty.
		return v.Result.Target
	}
	if shift {
		return v.Result.WebTarget
	}
	if v.SelectedURL != "" {
		return v.SelectedURL
	}
	if v.Selected == "" && v.Result.Target != "" {
		return v.Result.Target
	}
	return v.Result.WebTarget
}

func (v homeSearchView) bangMode() bool {
	switch v.Result.Mode {
	case homeQueryBang, homeQueryBangPartial, homeQueryBangUnknown:
		return true
	}
	return false
}

// homeIdleShortcuts returns a few shortcut keys for the idle hint, the
// shortest first.
func homeIdleShortcuts(shortcuts map[string]dto.HomeSearchShortcut) []string {
	keys := make([]string, 0, len(shortcuts))
	for key := range shortcuts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) < len(keys[j])
		}
		return keys[i] < keys[j]
	})
	if len(keys) > homeIdleShortcutLimit {
		keys = keys[:homeIdleShortcutLimit]
	}
	return keys
}

// homeItemText is the lowercased haystack of a searchable item.
func homeItemText(parts ...string) string {
	return strings.ToLower(strings.Join(parts, " "))
}

func homeRowItemText(row homeRow) string {
	return homeItemText(homeRowLabel(row), stripURLScheme(row.URL))
}

// homeServiceItemAttrs are the search data attributes of a service tile.
func homeServiceItemAttrs(service entity.HomeService) templ.Attributes {
	return templ.Attributes{
		"data-sv-home-item": "service",
		"data-sv-home-name": service.Name,
		"data-sv-home-text": homeItemText(service.Name, service.Description, stripURLScheme(service.URL)),
		"data-sv-home-url":  service.URL,
	}
}

// homeKeyAction is what a key does on the Home page.
type homeKeyAction int

const (
	homeKeyNone homeKeyAction = iota
	homeKeyOpen
	homeKeyOpenWeb
	homeKeySelectNext
	homeKeySelectPrev
	homeKeyClear
	homeKeyBlur
	homeKeyFocusSearch
)

// homeSearchKey maps a key typed in the search field. Focus stays in the
// field: arrows move the selection, Escape clears then blurs.
func homeSearchKey(key string, shift bool, query string) homeKeyAction {
	switch key {
	case "enter":
		if shift {
			return homeKeyOpenWeb
		}
		return homeKeyOpen
	case "arrowdown":
		return homeKeySelectNext
	case "arrowup":
		return homeKeySelectPrev
	case "escape":
		if query != "" {
			return homeKeyClear
		}
		return homeKeyBlur
	}
	return homeKeyNone
}

// homePageKey maps a key typed outside the search field: "/" focuses the
// search unless the user is typing in another field.
func homePageKey(key string, inTextEntry bool) homeKeyAction {
	if key == "/" && !inTextEntry {
		return homeKeyFocusSearch
	}
	return homeKeyNone
}
