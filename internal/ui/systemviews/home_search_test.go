package systemviews

import (
	"context"
	"testing"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testHomeSearchConfig() homeSearchConfig {
	return homeSearchConfig{
		EngineURL:  "https://duckduckgo.com/?q=%s",
		EngineName: "DuckDuckGo",
		Shortcuts: map[string]dto.HomeSearchShortcut{
			"g":  {URL: "https://www.google.com/search?q=%s", Name: "Google"},
			"gh": {URL: "https://github.com/search?q=%s", Name: "GitHub"},
			"go": {URL: "https://pkg.go.dev/search?q=%s", Name: "pkg.go.dev"},
		},
	}
}

func testHomeSearchItems() []homeQueryItem {
	return []homeQueryItem{
		{Key: "0", Name: "Jellyfin", Text: "jellyfin movies jelly.lan", URL: "https://jelly.lan"},
		{Key: "1", Name: "Jelly docs", Text: "jelly docs docs.lan", URL: "https://docs.lan"},
		{Key: "2", Name: "Proxmox", Text: "proxmox pve.lan", URL: "https://pve.lan"},
	}
}

func TestBuildHomeSearchView(t *testing.T) {
	t.Parallel()
	config, items := testHomeSearchConfig(), testHomeSearchItems()

	idle := buildHomeSearchView(config, "", items, nil, -1)
	assert.Equal(t, homeChipIdle, idle.ChipState)
	assert.Equal(t, homeHintIdle, idle.Hint)
	assert.False(t, idle.Filtering)
	assert.Equal(t, []string{"g", "gh", "go"}, idle.Shortcuts)
	assert.Equal(t, "DuckDuckGo", idle.ChipName)

	filter := buildHomeSearchView(config, "jel", items, nil, -1)
	assert.True(t, filter.Filtering)
	assert.Equal(t, []string{"0", "1"}, filter.Result.Matches)
	assert.Equal(t, "0", filter.Selected)
	assert.Equal(t, "Jellyfin", filter.SelectedName)
	assert.Equal(t, "https://jelly.lan", filter.Enter(false))
	assert.Equal(t, "https://duckduckgo.com/?q=jel", filter.Enter(true))

	second := buildHomeSearchView(config, "jel", items, nil, 7)
	assert.Equal(t, "1", second.Selected, "selection is clamped to the matches")
	assert.Equal(t, "https://docs.lan", second.Enter(false))

	web := buildHomeSearchView(config, "nothing here", items, nil, -1)
	assert.Equal(t, homeChipWeb, web.ChipState)
	assert.Equal(t, homeHintWeb, web.Hint)
	assert.Empty(t, web.Selected)
	assert.Equal(t, "https://duckduckgo.com/?q=nothing+here", web.Enter(false))

	bang := buildHomeSearchView(config, "!gh dumber", items, nil, -1)
	assert.Equal(t, homeChipBang, bang.ChipState)
	assert.Equal(t, "GitHub", bang.ChipName)
	assert.Equal(t, homeHintBangSearch, bang.Hint)
	assert.Equal(t, "dumber", bang.HintQuery)
	assert.True(t, bang.HideSuggestions)
	assert.False(t, bang.Filtering)
	assert.Equal(t, "https://github.com/search?q=dumber", bang.Enter(false))

	typing := buildHomeSearchView(config, "!gh", items, nil, -1)
	assert.Equal(t, homeHintBangType, typing.Hint)

	noG := testHomeSearchConfig()
	delete(noG.Shortcuts, "g")
	partial := buildHomeSearchView(noG, "!g", items, nil, -1)
	assert.Equal(t, homeHintCandidates, partial.Hint)
	assert.Equal(t, []homeHintCandidate{{Key: "gh", Name: "GitHub"}, {Key: "go", Name: "pkg.go.dev"}}, partial.Candidates)
	assert.True(t, partial.HideSuggestions)

	unknown := buildHomeSearchView(config, "!zz", items, nil, -1)
	assert.Equal(t, homeHintUnknown, unknown.Hint)
	assert.Equal(t, "zz", unknown.HintQuery)

	address := buildHomeSearchView(config, "pve.lan", items, nil, -1)
	assert.Equal(t, homeChipAddress, address.ChipState)
	assert.Equal(t, "Address", address.ChipName)
	assert.Equal(t, "Proxmox", address.SelectedName)
	assert.Equal(t, "https://pve.lan", address.Enter(false))
	assert.Contains(t, address.Enter(true), "pve.lan")
}

func TestHomeSearchHintText(t *testing.T) {
	t.Parallel()
	config, items := testHomeSearchConfig(), testHomeSearchItems()
	for query, want := range map[string]string{
		"":           "Filter services as you type",
		"jel":        "Jellyfin",
		"!gh dumber": "on GitHub",
		"!gh":        "Type your GitHub search",
		"!zz":        "Unknown shortcut",
		"pve.lan":    "Proxmox",
		"nothing":    "No service matches",
	} {
		html := mustRenderComponent(HomeSearchHint(buildHomeSearchView(config, query, items, nil, -1)))
		assert.Contains(t, html, want, query)
	}
}

func TestHomeShortcutsJSONRoundTrip(t *testing.T) {
	t.Parallel()
	shortcuts := testHomeSearchConfig().Shortcuts
	assert.Equal(t, shortcuts, parseHomeShortcutsJSON(homeShortcutsJSON(shortcuts)))
	assert.Nil(t, parseHomeShortcutsJSON("not json"))
	assert.Equal(t, "{}", homeShortcutsJSON(nil))
}

func TestHomeSearchWidgetRendersEngineData(t *testing.T) {
	t.Parallel()
	load := dto.HomeLoadResponse{
		Layout: entity.HomeLayout{
			Widgets: []entity.HomeBuiltinWidget{{Type: entity.HomeWidgetSearch}},
			Groups: []entity.HomeGroup{{Name: "Media", Services: []entity.HomeService{
				{ID: "jelly", Name: "Jellyfin", URL: "https://jelly.lan", Description: "Movies"},
			}}},
		},
		SearchEngine: "https://s.test/?q=%s", SearchEngineName: "S",
		SearchShortcuts: map[string]dto.HomeSearchShortcut{"gh": {URL: "https://github.com/search?q=%s", Name: "GitHub"}},
	}
	app := newHomeApp(t, newTreeDOM(), newHomeStore(t, &homeStore{load: load}))
	html := app.renderedHTML
	assert.Contains(t, html, `data-sv-home-engine-url="https://s.test/?q=%s"`)
	assert.Contains(t, html, `data-sv-home-engine-name="S"`)
	assert.Contains(t, html, `data-sv-home-shortcuts="{&#34;gh&#34;`)
	assert.Contains(t, html, `data-sv-home-item="service"`)
	assert.Contains(t, html, `data-sv-home-text="jellyfin movies jelly.lan"`)
	assert.Contains(t, html, `aria-live="polite"`)
}

func TestHomeSearchActionOpensResolvedURL(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: entity.DefaultHomeLayout(), SearchEngine: "https://s.test/?q=%s"}})
	dom := &navTreeDOM{treeDOM: newTreeDOM()}
	app := newHomeApp(t, dom, home)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionSearch, Data: map[string]string{"url": "https://jelly.lan"}}))
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: homeActionSearch, Data: map[string]string{"url": "javascript:alert(1)", "query": "x"}}))
	assert.Equal(t, []string{"https://jelly.lan"}, dom.navigated, "unsafe urls are dropped")
}

func TestHomeSearchSelectionMovesOverMatchesThenSuggestions(t *testing.T) {
	t.Parallel()
	config, items := testHomeSearchConfig(), testHomeSearchItems()
	suggestions := []homeQueryItem{{Key: "s0", Name: "Jelly wiki", URL: "https://wiki.test/jelly"}}

	view := buildHomeSearchView(config, "jel", items, suggestions, -1)
	assert.Equal(t, 0, view.SelectedIndex, "the first match is the default")
	assert.Equal(t, 3, view.SelectableCount)
	assert.Equal(t, 1, view.MoveSelection(true))
	assert.Equal(t, 0, view.MoveSelection(false), "clamped at the top")

	last := buildHomeSearchView(config, "jel", items, suggestions, 2)
	assert.Equal(t, "s0", last.Selected)
	assert.Equal(t, "https://wiki.test/jelly", last.Enter(false))
	assert.Equal(t, 2, last.MoveSelection(true), "clamped at the bottom")

	web := buildHomeSearchView(config, "nothing", items, suggestions, -1)
	assert.Empty(t, web.Selected, "no default selection without matches")
	assert.Equal(t, "https://duckduckgo.com/?q=nothing", web.Enter(false))
	assert.Equal(t, 0, web.MoveSelection(true))
	picked := buildHomeSearchView(config, "nothing", items, suggestions, 0)
	assert.Equal(t, "https://wiki.test/jelly", picked.Enter(false))
	assert.Equal(t, homeHintOpen, picked.Hint)
	assert.Contains(t, mustRenderComponent(HomeSearchHint(picked)), "Jelly wiki")

	bang := buildHomeSearchView(config, "!gh x", items, suggestions, 0)
	assert.Empty(t, bang.Selected, "bang mode selects nothing")
	assert.Equal(t, -1, bang.MoveSelection(true))
}

func TestHomeSearchKey(t *testing.T) {
	t.Parallel()
	assert.Equal(t, homeKeyOpen, homeSearchKey("enter", false, "x"))
	assert.Equal(t, homeKeyOpenWeb, homeSearchKey("enter", true, "x"))
	assert.Equal(t, homeKeySelectNext, homeSearchKey("arrowdown", false, "x"))
	assert.Equal(t, homeKeySelectPrev, homeSearchKey("arrowup", false, "x"))
	assert.Equal(t, homeKeyClear, homeSearchKey("escape", false, "x"), "first Escape clears")
	assert.Equal(t, homeKeyBlur, homeSearchKey("escape", false, ""), "second Escape blurs")
	assert.Equal(t, homeKeyNone, homeSearchKey("a", false, "x"))
}

func TestHomePageKey(t *testing.T) {
	t.Parallel()
	assert.Equal(t, homeKeyFocusSearch, homePageKey("/", false))
	assert.Equal(t, homeKeyNone, homePageKey("/", true), "typing a slash in a field")
	assert.Equal(t, homeKeyNone, homePageKey("a", false))
}
