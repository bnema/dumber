package systemviews

import (
	"context"
	"testing"

	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type paletteDOM struct {
	regionDOM
	palettes []PaletteRequest
}

func (d *paletteDOM) ShowPalette(request PaletteRequest) {
	d.palettes = append(d.palettes, request)
}

func paletteIDs(request PaletteRequest) []string {
	ids := make([]string, 0, len(request.Items))
	for i := range request.Items {
		ids = append(ids, request.Items[i].ID)
	}
	return ids
}

func findPaletteItem(t *testing.T, request PaletteRequest, id string) PaletteItem {
	t.Helper()
	for i := range request.Items {
		if request.Items[i].ID == id {
			return request.Items[i]
		}
	}
	t.Fatalf("palette item %q missing", id)
	return PaletteItem{}
}

func TestPaletteOnHistoryOffersHistoryCommandsAndOtherViews(t *testing.T) {
	t.Parallel()

	dom := &paletteDOM{}
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}})
	app := loadedHistoryApp(t, dom, history)

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: paletteActionOpen}))

	require.Len(t, dom.palettes, 1)
	ids := paletteIDs(dom.palettes[0])
	assert.Contains(t, ids, "history.search")
	assert.Contains(t, ids, "history.deleteRange.all")
	assert.Contains(t, ids, "goto.favorites")
	assert.Contains(t, ids, "goto.config")
	assert.NotContains(t, ids, "goto.history", "the current view is not offered")
	assert.NotContains(t, ids, "history.clear", "no filters, nothing to clear")

	item := findPaletteItem(t, dom.palettes[0], "history.deleteRange.all")
	assert.True(t, item.Danger)
	assert.Equal(t, "Delete all browsing history?", item.Confirm)
	assert.Equal(t, "Delete history: everything", item.Label)
	assert.Equal(t, historyActionDeleteRange, item.Action.Action)
	assert.Equal(t, "all", item.Action.Data["range"])
}

func TestPaletteOffersClearFiltersWhenFiltered(t *testing.T) {
	t.Parallel()

	dom := &paletteDOM{}
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}})
	app := loadedHistoryApp(t, dom, history)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: historyActionFilterDomain, Data: map[string]string{"domain": "example.com"}}))
	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: paletteActionOpen}))

	assert.Contains(t, paletteIDs(dom.palettes[0]), "history.clear")
}

func TestPaletteOutsideHistoryOnlyNavigates(t *testing.T) {
	t.Parallel()

	dom := &paletteDOM{}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://favorites"})
	app.currentRoute = RouteFavorites

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: paletteActionOpen}))

	assert.Equal(t, []string{"goto.history", "goto.config"}, paletteIDs(dom.palettes[0]))
}

func TestPaletteWithoutSupportIsNoop(t *testing.T) {
	t.Parallel()

	dom := &recordingDOM{}
	app := NewApp(Dependencies{DOM: dom, LocationURI: "dumb://history"})

	require.NoError(t, app.HandleDOMAction(context.Background(), DOMAction{Action: paletteActionOpen}))
	assert.False(t, dom.Mounted())
}

func TestPageHeaderOffersPaletteTrigger(t *testing.T) {
	t.Parallel()

	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 1, URL: "https://example.com"}}})
	app := loadedHistoryApp(t, &recordingDOM{}, history)

	assert.Contains(t, app.renderedHTML, `data-sv-action="palette.open"`)
	assert.Contains(t, app.renderedHTML, `aria-keyshortcuts="Control+K"`)
}
