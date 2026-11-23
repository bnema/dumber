package systemviews

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"github.com/stretchr/testify/mock"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
)

func testConfigKeybindings() port.KeybindingsConfig {
	return port.KeybindingsConfig{Groups: []port.KeybindingGroup{
		{
			Mode:        "global",
			DisplayName: "Global",
			Bindings: []port.KeybindingEntry{
				{Action: "toggle_history_systemview", Description: "Toggle history", Keys: []string{"ctrl+h"}, DefaultKeys: []string{"ctrl+h"}},
				{Action: "zoom-in", Description: "Zoom in", Keys: []string{"ctrl+plus", "ctrl+equal"}, DefaultKeys: []string{"ctrl+plus", "ctrl+equal"}},
				{Action: "new-tab", Description: "New tab", Keys: []string{"ctrl+y"}, DefaultKeys: []string{"ctrl+t"}, IsCustom: true},
			},
		},
		{
			Mode:        "vim",
			DisplayName: "Vim",
			Activation:  "ctrl+y",
			Bindings: []port.KeybindingEntry{
				{Action: "prev-para", Description: "Previous paragraph", Keys: []string{","}, DefaultKeys: []string{","}},
				{Action: "half-page", Description: "Half page", Keys: []string{"<C-d>", "+"}, DefaultKeys: []string{"<C-d>", "+"}},
				{Action: "insert", Description: "Insert", Keys: []string{"gi"}, DefaultKeys: []string{"gi"}},
			},
		},
	}}
}

// configStore is the stored config behind a mocked config service: Current
// returns it and Save updates it, so remounts show what was written.
type configStore struct {
	current dto.SystemviewConfigPayload
	saved   []dto.WebUIConfig
}

// newConfigService mocks a config service whose Current and GetKeybindings
// serve the store and the given keybindings.
func newConfigService(t *testing.T, current dto.SystemviewConfigPayload, keybindings port.KeybindingsConfig) (*mocks.MockSystemviewConfigService, *configStore) {
	t.Helper()
	store := &configStore{current: current}
	service := mocks.NewMockSystemviewConfigService(t)
	service.EXPECT().Current(mock.Anything).RunAndReturn(func(context.Context) (dto.SystemviewConfigPayload, error) {
		return store.current, nil
	})
	service.EXPECT().GetKeybindings(mock.Anything).Return(keybindings, nil)
	return service, store
}

// expectSave lets the service accept Save calls and records them in the store.
func (s *configStore) expectSave(service *mocks.MockSystemviewConfigService) {
	service.EXPECT().Save(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, cfg dto.WebUIConfig) error {
		s.saved = append(s.saved, cfg)
		s.current.Appearance = cfg.Appearance
		s.current.DefaultUIScale = cfg.DefaultUIScale
		s.current.DefaultSearchEngine = cfg.DefaultSearchEngine
		s.current.SearchShortcuts = cfg.SearchShortcuts
		s.current.Performance.Profile = cfg.Performance.Profile
		s.current.Performance.Custom = dto.SystemviewCustomPerformancePayload{
			SkiaCPUThreads:         cfg.Performance.Custom.SkiaCPUThreads,
			SkiaGPUThreads:         cfg.Performance.Custom.SkiaGPUThreads,
			WebProcessMemoryMB:     cfg.Performance.Custom.WebProcessMemoryMB,
			NetworkProcessMemoryMB: cfg.Performance.Custom.NetworkProcessMemoryMB,
			WebViewPoolPrewarm:     cfg.Performance.Custom.WebViewPoolPrewarm,
		}
		return nil
	})
}

// newConfigApp loads the config route. The service has no Save or keybinding
// write expectations, so any unexpected write fails the test.
func newConfigApp(t *testing.T, dom DOM) (*App, *mocks.MockSystemviewConfigService, *configStore) {
	t.Helper()
	service, store := newConfigService(t, testConfigPayload(), testConfigKeybindings())
	app := NewApp(Dependencies{DOM: dom, Config: service, LocationURI: "dumb://config"})
	require.NoError(t, app.LoadInitial(context.Background()))
	return app, service, store
}

// submittedConfigForm returns what the browser would submit for the settings
// form as rendered: named, enabled controls, checked boxes and radios only,
// duplicate names comma-joined like collectFormData.
func submittedConfigForm(t *testing.T, markup string) map[string]string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	require.NoError(t, err)

	data := map[string]string{}
	add := func(name, value string) {
		if existing := data[name]; existing != "" {
			data[name] = existing + "," + value
			return
		}
		data[name] = value
	}
	var form *html.Node
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inForm bool) {
		if n.Type == html.ElementNode && n.Data == "form" && attr(n, "id") == configFormID {
			form = n
			inForm = true
		}
		if inForm && n.Type == html.ElementNode && hasAttr(n, "name") && !hasAttr(n, "disabled") {
			for _, value := range submittedValues(n) {
				add(attr(n, "name"), value)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inForm)
		}
	}
	walk(doc, false)
	require.NotNil(t, form, "settings form rendered")
	return data
}

// submittedValues is what one named control contributes to the form data.
func submittedValues(n *html.Node) []string {
	switch n.Data {
	case "input":
		kind := attr(n, "type")
		if kind != "checkbox" && kind != "radio" {
			return []string{attr(n, "value")}
		}
		if !hasAttr(n, "checked") {
			return nil
		}
		if !hasAttr(n, "value") {
			return []string{"on"}
		}
		return []string{attr(n, "value")}
	case "select":
		// Like a browser: the selected option, or the first one.
		var values []string
		first := ""
		for option := n.FirstChild; option != nil; option = option.NextSibling {
			if option.Type != html.ElementNode || option.Data != "option" {
				continue
			}
			if first == "" {
				first = attr(option, "value")
			}
			if hasAttr(option, "selected") {
				values = append(values, attr(option, "value"))
			}
		}
		if len(values) == 0 {
			return []string{first}
		}
		return values
	default:
		return nil
	}
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func saveBatch(t *testing.T, app *App, data map[string]string) {
	t.Helper()
	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: configActionSaveBatch, Data: data}))
}

func TestConfigViewRendersSettingsForm(t *testing.T) {
	t.Parallel()

	app, _, _ := newConfigApp(t, &recordingDOM{})
	out := app.renderedHTML

	for _, want := range []string{
		`data-sv-action="config.saveBatch"`,
		`data-sv-region="config-status"`,
		`form="sv-config-form"`,
		`href="#sv-config-keybindings"`,
		`name="color_scheme" value="prefer-dark" checked`,
		`name="profile" value="default" checked`,
		`name="external_theme_enabled" value="off"`,
		`role="switch"`,
		`name="shortcut.0.key" type="text" value="ddg"`,
		`name="shortcut.1.key"`,
		`name="keys:global:new-tab"`,
		`data-sv-action="ui.config.resetKeys"`,
		`data-sv-action="config.appearance.reset"`,
		`data-sv-action="config.keybinding.resetAll"`,
		`data-sv-palette-preview="dark"`,
		`value="#66aaff"`,
		`data-sv-keybinding-filter`,
	} {
		assert.Contains(t, out, want)
	}
	assert.Contains(t, out, `disabled data-sv-performance-custom`, "custom values are off unless the profile is custom")
	assert.Positive(t, strings.Count(out, "data-sv-kb-record"), "chord-only groups get record buttons")
	assert.Less(t, strings.Count(out, "data-sv-kb-record"), strings.Count(out, "data-sv-kb-row"), "only chord-only groups get the recorder")
}

func TestConfigSaveBatchUnchangedFormWritesNothing(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	app, _, store := newConfigApp(t, dom)

	saveBatch(t, app, submittedConfigForm(t, app.renderedHTML))

	assert.Empty(t, store.saved, "an untouched form is not a config change")
	// Rendered keys round-trip, including , + and <C-d>: SetKeybinding and
	// ResetKeybinding have no expectation, so a keybinding write fails the test.
	assert.Contains(t, dom.HTML(), "No changes to save")
}

func TestConfigSaveBatchUnchangedFormWithEmptyFormatWritesNothing(t *testing.T) {
	t.Parallel()

	cfg := testConfigPayload()
	cfg.Appearance.ExternalTheme.Format = ""
	service, store := newConfigService(t, cfg, testConfigKeybindings())
	app := NewApp(Dependencies{DOM: &recordingDOM{}, Config: service, LocationURI: "dumb://config"})
	require.NoError(t, app.LoadInitial(context.Background()))

	saveBatch(t, app, submittedConfigForm(t, app.renderedHTML))

	assert.Empty(t, store.saved, "an empty stored format stays empty")
}

// Field errors name the field so the page can focus it; nothing is written.
func TestConfigSaveBatchFieldErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, field, value, message string
	}{
		{"search engine without placeholder", "default_search_engine", "https://example.com/search", "must include the %s placeholder"},
		{"search engine not http", "default_search_engine", "ftp://example.com/?q=%s", "absolute http(s) URL"},
		{"font size", "default_font_size", "0", "must be positive"},
		{"ui scale", "default_ui_scale", "abc", "invalid UI scale"},
		{"palette color", "dark_text", " ", "Text color is required"},
		{"unknown scheme", "color_scheme", "sepia", "unknown color scheme"},
		{"duplicate shortcut", "shortcut.1.key", "ddg", `&#34;ddg&#34; is used twice`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dom := &regionDOM{}
			app, _, store := newConfigApp(t, dom)
			data := submittedConfigForm(t, app.renderedHTML)
			data[tc.field] = tc.value
			if tc.field == "shortcut.1.key" {
				data["shortcut.1.url"] = "https://example.com/?q=%s"
			}

			saveBatch(t, app, data)

			assert.Empty(t, store.saved)
			require.Len(t, dom.regions, 1)
			assert.Contains(t, dom.regions[0][0].HTML, tc.message)
			assert.Contains(t, dom.regions[0][0].HTML, `data-sv-error-field="`+tc.field+`"`)
		})
	}
}

func TestConfigSaveBatchRejectsOutOfRangeCustomPerformance(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	app, _, store := newConfigApp(t, dom)
	data := submittedConfigForm(t, app.renderedHTML)
	data["profile"] = "custom"
	for _, field := range performanceCustomFields(testConfigPayload().Performance.Custom) {
		data[field.Name] = strconv.Itoa(field.Value)
	}
	data["skia_cpu_threads"] = "65"

	saveBatch(t, app, data)

	assert.Empty(t, store.saved)
	require.Len(t, dom.regions, 1)
	assert.Contains(t, dom.regions[0][0].HTML, "Skia CPU threads must be between 0 and 8")
}

func TestConfigSaveBatchMissingFieldsKeepStoredValues(t *testing.T) {
	t.Parallel()

	app, _, store := newConfigApp(t, &recordingDOM{})

	saveBatch(t, app, map[string]string{})

	assert.Empty(t, store.saved)
	// No keybinding expectation: a keybinding write fails the test.
}

func TestConfigSaveBatchWritesConfigOnceAndChangedKeybindings(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	app, service, store := newConfigApp(t, dom)
	store.expectSave(service)
	var setReqs []port.SetKeybindingRequest
	service.EXPECT().SetKeybinding(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req port.SetKeybindingRequest) (port.SetKeybindingResponse, error) {
			setReqs = append(setReqs, req)
			return port.SetKeybindingResponse{}, nil
		})
	var resetReqs []port.ResetKeybindingRequest
	service.EXPECT().ResetKeybinding(mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, req port.ResetKeybindingRequest) error {
			resetReqs = append(resetReqs, req)
			return nil
		})
	data := submittedConfigForm(t, app.renderedHTML)

	data["sans_font"] = "Fira Sans"
	data["color_scheme"] = "prefer-light"
	data["external_theme_enabled"] = "off"
	data["dark_accent"] = "#ff8800"
	data["default_search_engine"] = "https://kagi.com/search?q=%s"
	data["shortcut.0.key"] = "d"
	data["shortcut.1.key"] = "gh"
	data["shortcut.1.url"] = "https://github.com/search?q=%s"
	data["profile"] = "custom"
	data["skia_cpu_threads"] = "4"
	data["keys:global:toggle_history_systemview"] = "ctrl+h, alt+h"
	data["keys:global:new-tab"] = "ctrl+t"

	saveBatch(t, app, data)

	require.Len(t, store.saved, 1, "all config fields are written together")
	saved := store.saved[0]
	assert.Equal(t, "Fira Sans", saved.Appearance.SansFont)
	assert.Equal(t, "prefer-light", saved.Appearance.ColorScheme)
	assert.False(t, saved.Appearance.ExternalTheme.Enabled)
	assert.Equal(t, "#ff8800", saved.Appearance.DarkPalette.Accent)
	assert.Equal(t, "https://kagi.com/search?q=%s", saved.DefaultSearchEngine)
	assert.Contains(t, saved.SearchShortcuts, "d", "renamed shortcut")
	assert.NotContains(t, saved.SearchShortcuts, "ddg")
	assert.Equal(t, "https://github.com/search?q=%s", saved.SearchShortcuts["gh"].URL)
	assert.Equal(t, "custom", saved.Performance.Profile)
	assert.Equal(t, 4, saved.Performance.Custom.SkiaCPUThreads)

	require.Len(t, setReqs, 1)
	assert.Equal(t, []string{"ctrl+h", "alt+h"}, setReqs[0].Keys)
	require.Len(t, resetReqs, 1, "back to the default keys resets the binding")
	assert.Equal(t, "new-tab", resetReqs[0].Action)
	assert.Contains(t, dom.HTML(), "Settings saved · 2 keybindings updated")
}

func TestConfigSaveBatchDeletesMarkedShortcut(t *testing.T) {
	t.Parallel()

	app, service, store := newConfigApp(t, &recordingDOM{})
	store.expectSave(service)
	data := submittedConfigForm(t, app.renderedHTML)
	data["shortcut.0.delete"] = "on"

	saveBatch(t, app, data)

	require.Len(t, store.saved, 1)
	assert.Empty(t, store.saved[0].SearchShortcuts)
}

func TestConfigSaveBatchValidationErrorKeepsForm(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	app, _, store := newConfigApp(t, dom)
	mountedBefore := dom.HTML()
	data := submittedConfigForm(t, app.renderedHTML)
	data["sans_font"] = "Changed"
	data["shortcut.1.key"] = "bad"
	data["shortcut.1.url"] = "https://example.com/search"

	saveBatch(t, app, data)

	assert.Empty(t, store.saved, "nothing is written when any field is invalid")
	assert.Equal(t, mountedBefore, dom.HTML(), "the form is not remounted, so edits survive")
	require.Len(t, dom.regions, 1)
	require.Len(t, dom.regions[0], 1)
	status := dom.regions[0][0]
	assert.Equal(t, configRegionStatus, status.Name)
	assert.Contains(t, status.HTML, "must include the %s placeholder")
	assert.Contains(t, status.HTML, `data-sv-error-field="shortcut.1.url"`)
}

func TestConfigSaveBatchPartialFailureRemounts(t *testing.T) {
	t.Parallel()

	dom := &regionDOM{}
	service, store := newConfigService(t, testConfigPayload(), testConfigKeybindings())
	store.expectSave(service)
	service.EXPECT().SetKeybinding(mock.Anything, mock.Anything).
		Return(port.SetKeybindingResponse{}, errors.New("bridge closed")).Once()
	app := NewApp(Dependencies{DOM: dom, Config: service, LocationURI: "dumb://config"})
	require.NoError(t, app.LoadInitial(context.Background()))
	data := submittedConfigForm(t, app.renderedHTML)
	data["sans_font"] = "Changed"
	data["keys:global:toggle_history_systemview"] = "alt+h"

	saveBatch(t, app, data)

	assert.Len(t, store.saved, 1)
	assert.Empty(t, dom.regions)
	assert.Contains(t, dom.HTML(), "saved part of the changes; keybinding toggle_history_systemview failed: bridge closed")
	assert.Contains(t, dom.HTML(), `value="Changed"`, "the page shows what was stored")
}

func TestConfigSaveBatchShowsToastInsteadOfAlert(t *testing.T) {
	t.Parallel()

	dom := &toastDOM{}
	app, service, store := newConfigApp(t, dom)
	store.expectSave(service)
	data := submittedConfigForm(t, app.renderedHTML)
	data["serif_font"] = "Georgia Pro"

	saveBatch(t, app, data)

	require.Len(t, dom.toasts, 1)
	assert.Equal(t, "Settings saved", dom.toasts[0].Message)
	assert.Empty(t, dom.toasts[0].ActionLabel)
	assert.NotContains(t, dom.HTML(), "Settings saved")
}

func TestConfigImmediateActionsStillRun(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	app, service, store := newConfigApp(t, dom)
	store.expectSave(service)
	service.EXPECT().ResetAllKeybindings(mock.Anything).Return(nil).Once()
	service.EXPECT().Default(mock.Anything).Return(testDefaultConfigPayload(), nil).Once()

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: configActionResetAllKeybindings}))
	service.AssertCalled(t, "ResetAllKeybindings", mock.Anything)
	assert.Contains(t, dom.HTML(), "Reset all keybindings to defaults")

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: configActionResetAppearance}))
	service.AssertCalled(t, "Default", mock.Anything)
	require.Len(t, store.saved, 1, "the default appearance is saved")
	assert.Equal(t, testDefaultConfigPayload().Appearance, store.saved[0].Appearance)
	assert.Contains(t, dom.HTML(), "Reset appearance settings to defaults")
}

func TestParseKeyListRoundTrip(t *testing.T) {
	t.Parallel()

	for _, keys := range [][]string{
		{"ctrl+h"},
		{"ctrl+h", "alt+h"},
		{","},
		{"ctrl+,", "x"},
		{",", "+"},
		{"+", "="},
		{"ctrl++", "ctrl+minus"},
		{"<C-d>", "gi", "[["},
		{"alt+}", "shift+f3"},
	} {
		assert.Equal(t, keys, parseKeyList(formatKeyList(keys)), formatKeyList(keys))
	}
	assert.Nil(t, parseKeyList("   "))
	assert.Equal(t, []string{"ctrl+h", "alt+h"}, parseKeyList("ctrl+h,alt+h"[:6]+", alt+h"))
}

func TestSameKeysIgnoresSpelling(t *testing.T) {
	t.Parallel()

	assert.True(t, sameKeys([]string{"Control+h", "alt+arrowdown"}, []string{"ctrl+h", "alt+down"}))
	assert.False(t, sameKeys([]string{"ctrl+h"}, []string{"ctrl+j"}))
	assert.True(t, sameKeys([]string{"gi"}, []string{"gi"}))
}

func TestConfigPaletteOffersSettingsCommands(t *testing.T) {
	t.Parallel()

	dom := &paletteDOM{}
	app, _, _ := newConfigApp(t, dom)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: paletteActionOpen}))

	require.Len(t, dom.palettes, 1)
	ids := paletteIDs(dom.palettes[0])
	assert.Contains(t, ids, "config.save")
	assert.Contains(t, ids, "config.section.sv-config-keybindings")
	assert.NotContains(t, ids, "goto.config")
}
