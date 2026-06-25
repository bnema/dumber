package main

import (
	"context"
	"strings"
	"sync/atomic"
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

	bridge := &bridgeServiceRecorder{historyEntries: []*entity.HistoryEntry{{Title: "Example", URL: "https://example.com"}}}
	app := newBridgeApp(&recordingDOM{}, "dumb://history", bridge)

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.True(t, bridge.calledHistory.Load())
	assert.False(t, bridge.calledFavorites.Load())
	assert.True(t, bridge.calledConfig.Load())
	assert.False(t, bridge.calledKeybindings.Load())
}

func TestNewBridgeApp_UsesCurrentConfigForNonConfigRoutes(t *testing.T) {
	t.Parallel()

	bridge := &bridgeServiceRecorder{
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
	dom := &recordingDOM{mounts: make(chan string, 4)}
	app := newBridgeApp(dom, "dumb://history", bridge)

	require.NoError(t, app.Run())
	require.Eventually(t, bridge.calledHistory.Load, time.Second, 10*time.Millisecond)
	assert.True(t, bridge.calledConfig.Load())
	assert.False(t, bridge.calledKeybindings.Load())
	html := receiveMountContaining(t, dom.mounts, `class="sv-app sv-dark"`, `--sv-background: #111111;`)
	assert.Contains(t, html, `class="sv-app sv-dark"`)
	assert.Contains(t, html, `--sv-background: #111111;`)
}

func TestNewBridgeApp_WiresFavoritesService(t *testing.T) {
	t.Parallel()

	bridge := &bridgeServiceRecorder{
		favorites: []*entity.Favorite{{Title: "Example", URL: "https://example.com"}},
		folders:   []*entity.Folder{{Name: "Read Later"}},
		tags:      []*entity.Tag{{Name: "Go"}},
	}
	app := newBridgeApp(&recordingDOM{}, "dumb://favorites", bridge)

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.False(t, bridge.calledHistory.Load())
	assert.True(t, bridge.calledFavorites.Load())
	assert.True(t, bridge.calledConfig.Load())
	assert.False(t, bridge.calledKeybindings.Load())
}

func TestNewBridgeApp_WiresConfigService(t *testing.T) {
	t.Parallel()

	bridge := &bridgeServiceRecorder{
		currentConfig: dto.SystemviewConfigPayload{EngineType: "webkit"},
		keybindings:   port.KeybindingsConfig{Groups: []port.KeybindingGroup{{DisplayName: "Default"}}},
	}
	app := newBridgeApp(&recordingDOM{}, "dumb://config", bridge)

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.False(t, bridge.calledHistory.Load())
	assert.False(t, bridge.calledFavorites.Load())
	assert.True(t, bridge.calledConfig.Load())
	assert.True(t, bridge.calledKeybindings.Load())
}

type recordingDOM struct {
	html   string
	mounts chan string
}

func (f *recordingDOM) Mount(html string) error {
	f.html = html
	if f.mounts != nil {
		select {
		case f.mounts <- html:
		default:
		}
	}
	return nil
}

func receiveMountContaining(t *testing.T, mounts <-chan string, values ...string) string {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		select {
		case html := <-mounts:
			matched := true
			for _, value := range values {
				if !strings.Contains(html, value) {
					matched = false
					break
				}
			}
			if matched {
				return html
			}
		case <-deadline:
			t.Fatalf("timed out waiting for DOM mount containing %q", values)
			return ""
		}
	}
}

// Recording fixture intentionally tracks state across history, favorites, config,
// and keybindings boundaries for the composite bootstrap assertions.
type bridgeServiceRecorder struct {
	calledHistory     atomic.Bool
	calledFavorites   atomic.Bool
	calledConfig      atomic.Bool
	calledKeybindings atomic.Bool

	historyEntries []*entity.HistoryEntry
	favorites      []*entity.Favorite
	folders        []*entity.Folder
	tags           []*entity.Tag
	currentConfig  dto.SystemviewConfigPayload
	keybindings    port.KeybindingsConfig
}

func (f *bridgeServiceRecorder) Timeline(context.Context, int, int) ([]*entity.HistoryEntry, error) {
	f.calledHistory.Store(true)
	return f.historyEntries, nil
}

func (f *bridgeServiceRecorder) TimelineByDomain(context.Context, string, int, int) ([]*entity.HistoryEntry, error) {
	f.calledHistory.Store(true)
	return f.historyEntries, nil
}

func (f *bridgeServiceRecorder) TimelineWindow(_ context.Context, before time.Time, _ int64, _ string) (*entity.HistoryWindow, error) {
	f.calledHistory.Store(true)
	return &entity.HistoryWindow{Entries: f.historyEntries, Before: before, After: before.Add(-24 * time.Hour)}, nil
}

func (*bridgeServiceRecorder) Search(context.Context, string, int) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) DeleteEntry(context.Context, int64) error { return nil }

func (*bridgeServiceRecorder) DeleteRange(context.Context, string) error { return nil }

func (*bridgeServiceRecorder) Stats(context.Context) (*entity.HistoryStats, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) Analytics(context.Context) (*entity.HistoryAnalytics, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) DomainStats(context.Context, int) ([]*entity.DomainStat, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) DeleteDomain(context.Context, string) error { return nil }

func (f *bridgeServiceRecorder) List(context.Context) ([]*entity.Favorite, error) {
	f.calledFavorites.Store(true)
	return f.favorites, nil
}

func (*bridgeServiceRecorder) CreateFavorite(context.Context, dto.FavoriteCreateInput) (*entity.Favorite, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) UpdateFavorite(context.Context, dto.FavoriteUpdateInput) (*entity.Favorite, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) DeleteFavorite(context.Context, int64) error { return nil }

func (f *bridgeServiceRecorder) ListFolders(context.Context) ([]*entity.Folder, error) {
	return f.folders, nil
}

func (f *bridgeServiceRecorder) ListTags(context.Context) ([]*entity.Tag, error) {
	return f.tags, nil
}

func (*bridgeServiceRecorder) SetShortcut(context.Context, int64, *int) error { return nil }

func (*bridgeServiceRecorder) SetFolder(context.Context, int64, *int64) error { return nil }

func (*bridgeServiceRecorder) CreateFolder(context.Context, string, string, *int64) (*entity.Folder, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) UpdateFolder(context.Context, int64, string, string) error { return nil }

func (*bridgeServiceRecorder) DeleteFolder(context.Context, int64) error { return nil }

func (*bridgeServiceRecorder) CreateTag(context.Context, string, string) (*entity.Tag, error) {
	return nil, nil
}

func (*bridgeServiceRecorder) UpdateTag(context.Context, int64, string, string) error { return nil }

func (*bridgeServiceRecorder) DeleteTag(context.Context, int64) error { return nil }

func (*bridgeServiceRecorder) AssignTag(context.Context, int64, int64) error { return nil }

func (*bridgeServiceRecorder) RemoveTag(context.Context, int64, int64) error { return nil }

func (f *bridgeServiceRecorder) Current(context.Context) (dto.SystemviewConfigPayload, error) {
	f.calledConfig.Store(true)
	return f.currentConfig, nil
}

func (*bridgeServiceRecorder) Default(context.Context) (dto.SystemviewConfigPayload, error) {
	return dto.SystemviewConfigPayload{}, nil
}

func (*bridgeServiceRecorder) Save(context.Context, dto.WebUIConfig) error { return nil }

func (f *bridgeServiceRecorder) GetKeybindings(context.Context) (port.KeybindingsConfig, error) {
	f.calledKeybindings.Store(true)
	return f.keybindings, nil
}

func (*bridgeServiceRecorder) SetKeybinding(context.Context, port.SetKeybindingRequest) (port.SetKeybindingResponse, error) {
	return port.SetKeybindingResponse{}, nil
}

func (*bridgeServiceRecorder) ResetKeybinding(context.Context, port.ResetKeybindingRequest) error {
	return nil
}

func (*bridgeServiceRecorder) ResetAllKeybindings(context.Context) error { return nil }
