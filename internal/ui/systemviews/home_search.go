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
	// Selected is the key of the selected item, or "".
	Selected     string
	SelectedName string
	SelectedURL  string

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

// buildHomeSearchView resolves query against items. selected is the index
// of the selected match; it is clamped to the matches.
func buildHomeSearchView(config homeSearchConfig, query string, items []homeQueryItem, selected int) homeSearchView {
	res := resolveHomeQuery(homeQueryInput{
		Query: query, EngineURL: config.EngineURL, EngineName: config.EngineName,
		Shortcuts: config.Shortcuts, Items: items,
	})
	view := homeSearchView{Result: res, ChipName: res.EngineName, ChipIcon: homeFaviconURL(res.EngineURL)}
	switch res.Mode {
	case homeQueryIdle:
		view.ChipState = homeChipIdle
		view.Hint = homeHintIdle
		view.Shortcuts = homeIdleShortcuts(config.Shortcuts)
	case homeQueryFilter:
		view.Filtering = true
		view.ChipState = homeChipIdle
		view.Hint = homeHintOpen
		view.selectMatch(items, selected)
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
		view.selectMatch(items, selected)
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
	return view
}

// selectMatch selects match index i (clamped).
func (v *homeSearchView) selectMatch(items []homeQueryItem, i int) {
	if len(v.Result.Matches) == 0 {
		return
	}
	i = max(0, min(i, len(v.Result.Matches)-1))
	v.Selected = v.Result.Matches[i]
	for _, item := range items {
		if item.Key == v.Selected {
			v.SelectedName = item.Name
			v.SelectedURL = homeNavigable(item.URL)
			return
		}
	}
}

// Enter returns what Enter opens: the selected item, else the resolved
// target. With shift, the web search or the typed address.
func (v homeSearchView) Enter(shift bool) string {
	if shift {
		return v.Result.WebTarget
	}
	if v.SelectedURL != "" {
		return v.SelectedURL
	}
	return v.Result.Target
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
