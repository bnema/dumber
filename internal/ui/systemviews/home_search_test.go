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

	idle := buildHomeSearchView(config, "", items, nil, "")
	assert.Equal(t, homeChipIdle, idle.ChipState)
	assert.Equal(t, homeHintIdle, idle.Hint)
	assert.False(t, idle.Filtering)
	assert.Equal(t, []string{"g", "gh", "go"}, idle.Shortcuts)
	assert.Equal(t, "DuckDuckGo", idle.ChipName)

	filter := buildHomeSearchView(config, "jel", items, nil, "")
	assert.True(t, filter.Filtering)
	assert.Equal(t, []string{"0", "1"}, filter.Result.Matches)
	assert.Equal(t, "0", filter.Selected)
	assert.Equal(t, "Jellyfin", filter.SelectedName)
	assert.Equal(t, "https://jelly.lan", filter.Enter(false))
	assert.Equal(t, "https://duckduckgo.com/?q=jel", filter.Enter(true))

	second := buildHomeSearchView(config, "jel", items, nil, homeItemIdentity(items[1]))
	assert.Equal(t, "1", second.Selected, "selection found by identity")
	assert.Equal(t, "https://docs.lan", second.Enter(false))
	gone := buildHomeSearchView(config, "jel", items, nil, "https://gone.lan\nGone")
	assert.Equal(t, "0", gone.Selected, "a selection that is gone falls back to the first match")

	web := buildHomeSearchView(config, "nothing here", items, nil, "")
	assert.Equal(t, homeChipWeb, web.ChipState)
	assert.Equal(t, homeHintWeb, web.Hint)
	assert.Empty(t, web.Selected)
	assert.Equal(t, "https://duckduckgo.com/?q=nothing+here", web.Enter(false))

	bang := buildHomeSearchView(config, "!gh dumber", items, nil, "")
	assert.Equal(t, homeChipBang, bang.ChipState)
	assert.Equal(t, "GitHub", bang.ChipName)
	assert.Equal(t, homeHintBangSearch, bang.Hint)
	assert.Equal(t, "dumber", bang.HintQuery)
	assert.True(t, bang.HideSuggestions)
	assert.False(t, bang.Filtering)
	assert.Equal(t, "https://github.com/search?q=dumber", bang.Enter(false))

	typing := buildHomeSearchView(config, "!gh", items, nil, "")
	assert.Equal(t, homeHintBangType, typing.Hint)

	noG := testHomeSearchConfig()
	delete(noG.Shortcuts, "g")
	partial := buildHomeSearchView(noG, "!g", items, nil, "")
	assert.Equal(t, homeHintCandidates, partial.Hint)
	assert.Equal(t, []homeHintCandidate{{Key: "gh", Name: "GitHub"}, {Key: "go", Name: "pkg.go.dev"}}, partial.Candidates)
	assert.True(t, partial.HideSuggestions)

	unknown := buildHomeSearchView(config, "!zz", items, nil, "")
	assert.Equal(t, homeHintUnknown, unknown.Hint)
	assert.Equal(t, "zz", unknown.HintQuery)

	address := buildHomeSearchView(config, "pve.lan", items, nil, "")
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
		html := mustRenderComponent(HomeSearchHint(buildHomeSearchView(config, query, items, nil, "")))
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

	wiki := homeItemIdentity(suggestions[0])
	view := buildHomeSearchView(config, "jel", items, suggestions, "")
	assert.Equal(t, 0, view.SelectedIndex, "the first match is the default")
	assert.Len(t, view.Selectable, 3)
	assert.Equal(t, homeItemIdentity(items[1]), view.MoveSelection(true))
	assert.Equal(t, homeItemIdentity(items[0]), view.MoveSelection(false), "clamped at the top")

	last := buildHomeSearchView(config, "jel", items, suggestions, wiki)
	assert.Equal(t, "s0", last.Selected)
	assert.Equal(t, "https://wiki.test/jelly", last.Enter(false))
	assert.Equal(t, wiki, last.MoveSelection(true), "clamped at the bottom")

	web := buildHomeSearchView(config, "nothing", items, suggestions, "")
	assert.Empty(t, web.Selected, "no default selection without matches")
	assert.Equal(t, "https://duckduckgo.com/?q=nothing", web.Enter(false))
	assert.Equal(t, wiki, web.MoveSelection(true))
	picked := buildHomeSearchView(config, "nothing", items, suggestions, wiki)
	assert.Equal(t, "https://wiki.test/jelly", picked.Enter(false))
	assert.Equal(t, homeHintOpen, picked.Hint)
	assert.Contains(t, mustRenderComponent(HomeSearchHint(picked)), "Jelly wiki")

	bang := buildHomeSearchView(config, "!gh x", items, suggestions, wiki)
	assert.Empty(t, bang.Selected, "bang mode selects nothing")
	assert.Empty(t, bang.MoveSelection(true))
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

func TestHomeReloadReappliesTypedQuery(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: dto.HomeLoadResponse{Layout: homeServiceLayoutWithSearch()}})
	dom := newSearchTreeDOM("jelly")
	app := mountHomeApp(t, dom.treeDOM, Dependencies{Home: home.service})
	app.deps.DOM = dom
	dom.ReapplyHomeSearch()

	exists, hidden, selected := dom.shellState("jelly")
	require.True(t, exists)
	assert.False(t, hidden)
	assert.True(t, selected)
	_, hidden, _ = dom.shellState("nas")
	assert.True(t, hidden, "non-matching rows hide")

	// A new matching service appears filtered in, without a remount.
	before := dom.reapply
	mounts := dom.mountCount()
	home.load.Layout.Groups[1].Services = append(home.load.Layout.Groups[1].Services, entity.HomeService{ID: "seerr", Name: "Jellyseerr", URL: "https://seerr.lan"})
	reloadHome(t, app)
	assert.Equal(t, mounts, dom.mountCount(), "no remount")
	assert.Greater(t, dom.reapply, before, "the reload re-applies the query")
	exists, hidden, _ = dom.shellState("seerr")
	require.True(t, exists)
	assert.False(t, hidden, "new matching service is filtered in")

	// Select the last match, then remove it: the selection stays valid.
	dom.mu.Lock()
	dom.selected = "https://seerr.lan\nJellyseerr"
	dom.mu.Unlock()
	dom.ReapplyHomeSearch()
	_, _, selected = dom.shellState("seerr")
	require.True(t, selected)
	home.load.Layout.Groups[1].Services = home.load.Layout.Groups[1].Services[:1]
	reloadHome(t, app)
	exists, _, _ = dom.shellState("seerr")
	assert.False(t, exists)
	_, hidden, selected = dom.shellState("jelly")
	assert.False(t, hidden)
	assert.True(t, selected, "the selection is clamped to the rows left")
}

func TestHomeSearchEnterFallbacks(t *testing.T) {
	t.Parallel()
	config := testHomeSearchConfig()
	items := []homeQueryItem{
		{Key: "0", Name: "Router", Text: "router gateway"},
		{Key: "1", Name: "Router docs", Text: "router docs docs.lan", URL: "https://docs.lan"},
	}

	// The first match has no url: Enter searches the web, and says so.
	view := buildHomeSearchView(config, "router", items, nil, "")
	assert.Equal(t, "0", view.Selected)
	assert.Equal(t, "https://duckduckgo.com/?q=router", view.Enter(false))
	assert.Equal(t, homeHintSearchFallback, view.Hint)
	assert.Contains(t, mustRenderComponent(HomeSearchHint(view)), "on DuckDuckGo")

	// Selecting the row with a url opens it.
	docs := buildHomeSearchView(config, "router", items, nil, homeItemIdentity(items[1]))
	assert.Equal(t, "https://docs.lan", docs.Enter(false))
	assert.Equal(t, homeHintOpen, docs.Hint)

	// In bang modes Shift+Enter does what Enter does.
	bang := buildHomeSearchView(config, "!gh dumber", items, nil, "")
	assert.Equal(t, bang.Enter(false), bang.Enter(true))
	assert.Equal(t, "https://github.com/search?q=dumber", bang.Enter(true))
	empty := buildHomeSearchView(config, "!gh", items, nil, "")
	assert.Empty(t, empty.Enter(true), "an empty bang query opens nothing")
}
