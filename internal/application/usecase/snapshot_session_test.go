package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSnapshotSessionUseCase_Execute_SavesSnapshot(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)

	// Create a tab list with some tabs
	tabList := entity.NewTabList()
	pane := entity.NewPane("pane-1")
	pane.URI = "https://example.com"
	pane.Title = "Example"
	tab := entity.NewTab("tab-1", "ws-1", pane)
	tab.Name = "Test Tab"
	tabList.Add(tab)

	sessionID := entity.SessionID("20251224_120000_test")

	stateRepo.EXPECT().SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Run(func(_ context.Context, state *entity.SessionState) {
			require.Equal(t, sessionID, state.SessionID)
			require.Len(t, state.Tabs, 1)
			require.Equal(t, "Test Tab", state.Tabs[0].Name)
			require.Equal(t, entity.SessionStateVersion, state.Version)
		}).
		Return(nil)

	uc := usecase.NewSnapshotSessionUseCase(stateRepo)

	err := uc.Execute(ctx, usecase.SnapshotInput{
		SessionID: sessionID,
		TabList:   tabList,
	})
	require.NoError(t, err)
}

func TestSnapshotSessionUseCase_Execute_EmptyTabList(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)

	sessionID := entity.SessionID("20251224_120000_empty")

	stateRepo.EXPECT().SaveSnapshot(mock.Anything, mock.AnythingOfType("*entity.SessionState")).
		Run(func(_ context.Context, state *entity.SessionState) {
			require.Equal(t, sessionID, state.SessionID)
			require.Empty(t, state.Tabs)
		}).
		Return(nil)

	uc := usecase.NewSnapshotSessionUseCase(stateRepo)

	err := uc.Execute(ctx, usecase.SnapshotInput{
		SessionID: sessionID,
		TabList:   nil,
	})
	require.NoError(t, err)
}

func TestSnapshotSessionUseCase_Execute_RequiresSessionID(t *testing.T) {
	ctx := testContext()

	stateRepo := repomocks.NewMockSessionStateRepository(t)

	uc := usecase.NewSnapshotSessionUseCase(stateRepo)

	err := uc.Execute(ctx, usecase.SnapshotInput{
		SessionID: "",
		TabList:   entity.NewTabList(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session id required")
}

func TestSnapshotFromTabList_CreatesCorrectSnapshot(t *testing.T) {
	sessionID := entity.SessionID("20251224_120000_snap")

	// Create a workspace with multiple panes
	pane1 := entity.NewPane("pane-1")
	pane1.URI = "https://github.com"
	pane1.Title = "GitHub"
	pane1.ZoomFactor = 1.2

	pane2 := entity.NewPane("pane-2")
	pane2.URI = "https://google.com"
	pane2.Title = "Google"

	tab := entity.NewTab("tab-1", "ws-1", pane1)
	tab.Name = "Dev Tab"
	tab.IsPinned = true

	tabList := entity.NewTabList()
	tabList.Add(tab)

	state := entity.SnapshotFromTabList(sessionID, tabList)

	require.NotNil(t, state)
	assert.Equal(t, entity.SessionStateVersion, state.Version)
	assert.Equal(t, sessionID, state.SessionID)
	assert.Len(t, state.Tabs, 1)
	assert.Equal(t, "Dev Tab", state.Tabs[0].Name)
	assert.True(t, state.Tabs[0].IsPinned)
	assert.NotNil(t, state.Tabs[0].Workspace.Root)
	assert.Equal(t, "https://github.com", state.Tabs[0].Workspace.Root.Pane.URI)
	assert.InDelta(t, 1.2, state.Tabs[0].Workspace.Root.Pane.ZoomFactor, 0.001)
	assert.False(t, state.SavedAt.IsZero())
}

func TestSessionState_CountPanes(t *testing.T) {
	state := &entity.SessionState{
		Tabs: []entity.TabSnapshot{
			{
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Pane: &entity.PaneSnapshot{ID: "pane-1"},
					},
				},
			},
			{
				Workspace: entity.WorkspaceSnapshot{
					Root: &entity.PaneNodeSnapshot{
						Children: []*entity.PaneNodeSnapshot{
							{Pane: &entity.PaneSnapshot{ID: "pane-2"}},
							{Pane: &entity.PaneSnapshot{ID: "pane-3"}},
						},
					},
				},
			},
		},
	}

	assert.Equal(t, 3, state.CountPanes())
}

func TestGetRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		t        time.Time
		expected string
	}{
		{"just now", now.Add(-30 * time.Second), "just now"},
		{"1 minute", now.Add(-1 * time.Minute), "1m ago"},
		{"5 minutes", now.Add(-5 * time.Minute), "05m ago"},
		{"1 hour", now.Add(-1 * time.Hour), "1h ago"},
		{"3 hours", now.Add(-3 * time.Hour), "03h ago"},
		{"1 day", now.Add(-24 * time.Hour), "1d ago"},
		{"2 days", now.Add(-48 * time.Hour), "02d ago"},
		{"1 week", now.Add(-7 * 24 * time.Hour), "1w ago"},
		{"2 weeks", now.Add(-14 * 24 * time.Hour), "02w ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := usecase.GetRelativeTime(tt.t)
			assert.Equal(t, tt.expected, result)
		})
	}
}
