package main

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBridgeApp_WiresHistoryService(t *testing.T) {
	t.Parallel()

	bridge := &fakeBridgeService{historyEntries: []*entity.HistoryEntry{{Title: "Example", URL: "https://example.com"}}}
	app := newBridgeApp(&fakeDOM{}, "dumb://history", bridge)

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.True(t, bridge.calledHistory)
	assert.False(t, bridge.calledFavorites)
	assert.True(t, bridge.calledConfig)
	assert.False(t, bridge.calledKeybindings)
}

func TestNewBridgeApp_UsesCurrentConfigForNonConfigRoutes(t *testing.T) {
	t.Parallel()

	bridge := &fakeBridgeService{
		historyEntries: []*entity.HistoryEntry{{Title: "Example", URL: "https://example.com"}},
		currentConfig: dto.SystemviewConfigPayload{
			Appearance: dto.WebUIAppearanceConfig{
				ColorScheme: "prefer-dark",
				DarkPalette: dto.ColorPalette{
					Background:     "#111111",
					Surface:        "#1a1a1a",
					SurfaceVariant: "#2a2a2a",
					Text:           "#f5f5f5",
					Muted:          "#a0a0a0",
					Accent:         "#66aaff",
					Border:         "#333333",
				},
			},
		},
	}
	dom := &fakeDOM{}
	app := newBridgeApp(dom, "dumb://history", bridge)

	require.NoError(t, app.Run())
	require.Eventually(t, func() bool { return bridge.calledHistory }, time.Second, 10*time.Millisecond)
	assert.True(t, bridge.calledConfig)
	assert.False(t, bridge.calledKeybindings)
	assert.Contains(t, dom.html, `class="sv-app sv-dark"`)
	assert.Contains(t, dom.html, `--sv-background: #111111;`)
}

func TestNewBridgeApp_WiresFavoritesService(t *testing.T) {
	t.Parallel()

	bridge := &fakeBridgeService{
		favorites: []*entity.Favorite{{Title: "Example", URL: "https://example.com"}},
		folders:   []*entity.Folder{{Name: "Read Later"}},
		tags:      []*entity.Tag{{Name: "Go"}},
	}
	app := newBridgeApp(&fakeDOM{}, "dumb://favorites", bridge)

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.False(t, bridge.calledHistory)
	assert.True(t, bridge.calledFavorites)
	assert.True(t, bridge.calledConfig)
	assert.False(t, bridge.calledKeybindings)
}

func TestNewBridgeApp_WiresConfigService(t *testing.T) {
	t.Parallel()

	bridge := &fakeBridgeService{
		currentConfig: dto.SystemviewConfigPayload{EngineType: "webkit"},
		keybindings:   port.KeybindingsConfig{Groups: []port.KeybindingGroup{{DisplayName: "Default"}}},
	}
	app := newBridgeApp(&fakeDOM{}, "dumb://config", bridge)

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.False(t, bridge.calledHistory)
	assert.False(t, bridge.calledFavorites)
	assert.True(t, bridge.calledConfig)
	assert.True(t, bridge.calledKeybindings)
}

type fakeDOM struct {
	html string
}

func (f *fakeDOM) Mount(html string) error {
	f.html = html
	return nil
}

// Handwritten fake intentionally tracks state across history, favorites, config,
// and keybindings boundaries for the composite bootstrap assertions.
type fakeBridgeService struct {
	calledHistory     bool
	calledFavorites   bool
	calledConfig      bool
	calledKeybindings bool

	historyEntries []*entity.HistoryEntry
	favorites      []*entity.Favorite
	folders        []*entity.Folder
	tags           []*entity.Tag
	currentConfig  dto.SystemviewConfigPayload
	keybindings    port.KeybindingsConfig
}

func (f *fakeBridgeService) Timeline(context.Context, int, int) ([]*entity.HistoryEntry, error) {
	f.calledHistory = true
	return f.historyEntries, nil
}

func (f *fakeBridgeService) TimelineByDomain(context.Context, string, int, int) ([]*entity.HistoryEntry, error) {
	f.calledHistory = true
	return f.historyEntries, nil
}

func (f *fakeBridgeService) TimelineWindow(_ context.Context, before time.Time, _ string) (*entity.HistoryWindow, error) {
	f.calledHistory = true
	return &entity.HistoryWindow{Entries: f.historyEntries, Before: before, After: before.Add(-24 * time.Hour)}, nil
}

func (*fakeBridgeService) Search(context.Context, string, int) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (*fakeBridgeService) DeleteEntry(context.Context, int64) error { return nil }

func (*fakeBridgeService) DeleteRange(context.Context, string) error { return nil }

func (*fakeBridgeService) Stats(context.Context) (*entity.HistoryStats, error) {
	return nil, nil
}

func (*fakeBridgeService) Analytics(context.Context) (*entity.HistoryAnalytics, error) {
	return nil, nil
}

func (*fakeBridgeService) DomainStats(context.Context, int) ([]*entity.DomainStat, error) {
	return nil, nil
}

func (*fakeBridgeService) DeleteDomain(context.Context, string) error { return nil }

func (f *fakeBridgeService) List(context.Context) ([]*entity.Favorite, error) {
	f.calledFavorites = true
	return f.favorites, nil
}

func (*fakeBridgeService) CreateFavorite(context.Context, port.FavoriteCreateInput) (*entity.Favorite, error) {
	return nil, nil
}

func (*fakeBridgeService) UpdateFavorite(context.Context, port.FavoriteUpdateInput) (*entity.Favorite, error) {
	return nil, nil
}

func (*fakeBridgeService) DeleteFavorite(context.Context, int64) error { return nil }

func (f *fakeBridgeService) ListFolders(context.Context) ([]*entity.Folder, error) {
	return f.folders, nil
}

func (f *fakeBridgeService) ListTags(context.Context) ([]*entity.Tag, error) {
	return f.tags, nil
}

func (*fakeBridgeService) SetShortcut(context.Context, int64, *int) error { return nil }

func (*fakeBridgeService) SetFolder(context.Context, int64, *int64) error { return nil }

func (*fakeBridgeService) CreateFolder(context.Context, string, string, *int64) (*entity.Folder, error) {
	return nil, nil
}

func (*fakeBridgeService) UpdateFolder(context.Context, int64, string, string) error { return nil }

func (*fakeBridgeService) DeleteFolder(context.Context, int64) error { return nil }

func (*fakeBridgeService) CreateTag(context.Context, string, string) (*entity.Tag, error) {
	return nil, nil
}

func (*fakeBridgeService) UpdateTag(context.Context, int64, string, string) error { return nil }

func (*fakeBridgeService) DeleteTag(context.Context, int64) error { return nil }

func (*fakeBridgeService) AssignTag(context.Context, int64, int64) error { return nil }

func (*fakeBridgeService) RemoveTag(context.Context, int64, int64) error { return nil }

func (f *fakeBridgeService) Current(context.Context) (dto.SystemviewConfigPayload, error) {
	f.calledConfig = true
	return f.currentConfig, nil
}

func (*fakeBridgeService) Default(context.Context) (dto.SystemviewConfigPayload, error) {
	return dto.SystemviewConfigPayload{}, nil
}

func (*fakeBridgeService) Save(context.Context, dto.WebUIConfig) error { return nil }

func (f *fakeBridgeService) GetKeybindings(context.Context) (port.KeybindingsConfig, error) {
	f.calledKeybindings = true
	return f.keybindings, nil
}

func (*fakeBridgeService) SetKeybinding(context.Context, port.SetKeybindingRequest) (port.SetKeybindingResponse, error) {
	return port.SetKeybindingResponse{}, nil
}

func (*fakeBridgeService) ResetKeybinding(context.Context, port.ResetKeybindingRequest) error {
	return nil
}

func (*fakeBridgeService) ResetAllKeybindings(context.Context) error { return nil }
