package component

import (
	"context"
	"errors"
	"testing"
	"time"

	appusecase "github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestOmniboxInitialBehaviorBadgeState(t *testing.T) {
	tests := []struct {
		name        string
		behavior    entity.OmniboxInitialBehavior
		wantVisible bool
		wantLabel   string
		wantTooltip string
		wantNext    entity.OmniboxInitialBehavior
	}{
		{
			name:        "recent",
			behavior:    entity.OmniboxInitialBehaviorRecent,
			wantVisible: true,
			wantLabel:   "Recent",
			wantTooltip: "Toggle default history order (Ctrl+R)",
			wantNext:    entity.OmniboxInitialBehaviorMostVisited,
		},
		{
			name:        "most visited",
			behavior:    entity.OmniboxInitialBehaviorMostVisited,
			wantVisible: true,
			wantLabel:   "Most used",
			wantTooltip: "Toggle default history order (Ctrl+R)",
			wantNext:    entity.OmniboxInitialBehaviorRecent,
		},
		{
			name:     "none",
			behavior: entity.OmniboxInitialBehaviorNone,
		},
		{
			name:     "invalid",
			behavior: entity.OmniboxInitialBehavior("unsupported"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := initialBehaviorBadgeState(tt.behavior)
			assert.Equal(t, tt.wantVisible, got.visible)
			assert.Equal(t, tt.wantLabel, got.label)
			assert.Equal(t, tt.wantTooltip, got.tooltip)
			assert.Equal(t, tt.wantNext, got.nextBehavior)
		})
	}
}

func TestShouldRefreshInitialHistoryAfterToggle(t *testing.T) {
	tests := []struct {
		name        string
		viewMode    ViewMode
		query       string
		behavior    entity.OmniboxInitialBehavior
		wantRefresh bool
	}{
		{
			name:        "empty history mode",
			viewMode:    ViewModeHistory,
			query:       "",
			behavior:    entity.OmniboxInitialBehaviorRecent,
			wantRefresh: true,
		},
		{
			name:        "typed query",
			viewMode:    ViewModeHistory,
			query:       "go",
			behavior:    entity.OmniboxInitialBehaviorRecent,
			wantRefresh: false,
		},
		{
			name:        "favorites mode",
			viewMode:    ViewModeFavorites,
			query:       "",
			behavior:    entity.OmniboxInitialBehaviorRecent,
			wantRefresh: false,
		},
		{
			name:        "unsupported behavior",
			viewMode:    ViewModeHistory,
			query:       "",
			behavior:    entity.OmniboxInitialBehaviorNone,
			wantRefresh: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRefresh, shouldRefreshInitialHistoryAfterToggle(tt.viewMode, tt.query, tt.behavior))
		})
	}
}

func TestOmniboxToggleInitialBehaviorPreference_SaveFailureKeepsBehaviorAndShowsToast(t *testing.T) {
	o := &Omnibox{initialBehavior: entity.OmniboxInitialBehaviorRecent, ctx: context.Background()}
	o.saveInitialBehaviorFn = func(context.Context, entity.OmniboxInitialBehavior) error {
		return errors.New("boom")
	}

	var gotMessage string
	var gotLevel ToastLevel
	o.onToast = func(_ context.Context, message string, level ToastLevel) {
		gotMessage = message
		gotLevel = level
	}

	refreshCalls := 0
	origLoadInitialHistoryFn := loadInitialHistoryFn
	loadInitialHistoryFn = func(*Omnibox, uint64) {
		refreshCalls++
	}
	t.Cleanup(func() {
		loadInitialHistoryFn = origLoadInitialHistoryFn
	})

	assert.False(t, o.toggleInitialBehaviorPreference())
	assert.Equal(t, entity.OmniboxInitialBehaviorRecent, o.initialBehavior)
	assert.Equal(t, "Failed to save default history order", gotMessage)
	assert.Equal(t, ToastError, gotLevel)
	assert.Equal(t, 0, refreshCalls)
}

func TestOmniboxToggleInitialBehaviorPreference_RefreshesEmptyHistory(t *testing.T) {
	o := &Omnibox{initialBehavior: entity.OmniboxInitialBehaviorRecent, viewMode: ViewModeHistory, ctx: context.Background()}
	o.saveInitialBehaviorFn = func(context.Context, entity.OmniboxInitialBehavior) error {
		return nil
	}

	refreshCalls := 0
	origLoadInitialHistoryFn := loadInitialHistoryFn
	loadInitialHistoryFn = func(*Omnibox, uint64) {
		refreshCalls++
	}
	t.Cleanup(func() {
		loadInitialHistoryFn = origLoadInitialHistoryFn
	})

	assert.True(t, o.toggleInitialBehaviorPreference())
	assert.Equal(t, entity.OmniboxInitialBehaviorMostVisited, o.initialBehavior)
	assert.Equal(t, 1, refreshCalls)
}

func TestOmniboxLoadInitialHistory_UsesCapturedInitialBehavior(t *testing.T) {
	repo := repomocks.NewMockHistoryRepository(t)
	done := make(chan struct{})
	repo.EXPECT().GetRecent(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(context.Context, int, int) ([]*entity.HistoryEntry, error) {
			close(done)
			return []*entity.HistoryEntry{{URL: "https://example.com"}}, nil
		},
	)

	o := &Omnibox{
		historyUC:       appusecase.NewSearchHistoryUseCase(repo),
		initialBehavior: entity.OmniboxInitialBehaviorRecent,
		ctx:             context.Background(),
	}

	o.loadInitialHistory(1)
	o.initialBehavior = entity.OmniboxInitialBehaviorMostVisited

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recent-history load")
	}
}
