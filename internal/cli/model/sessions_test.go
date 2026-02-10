package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestRenderSessionsList_KeepsSelectedVisibleWhenExpandedRowAbove(t *testing.T) {
	theme := styles.NewTheme(config.DefaultConfig())
	m := NewSessionsModel(context.Background(), theme, SessionsModelConfig{})

	m.sessions = make([]entity.SessionInfo, 0, 6)
	for i := 0; i < 6; i++ {
		m.sessions = append(m.sessions, testSessionInfo(fmt.Sprintf("session-%02d", i), 1))
	}

	// Expand a row above the selected row with many detail lines.
	m.sessions[2] = testSessionInfo("session-02", 6)
	m.selectedIdx = 4
	m.expandedIdx = 2

	view := m.renderSessionsList(5)
	require.Contains(t, view, "session-04", "selected row should remain visible")
}

func testSessionInfo(id string, tabCount int) entity.SessionInfo {
	tabs := make([]entity.TabSnapshot, 0, tabCount)
	for i := 0; i < tabCount; i++ {
		tabs = append(tabs, entity.TabSnapshot{Name: fmt.Sprintf("Tab %d", i+1)})
	}

	return entity.SessionInfo{
		Session: &entity.Session{
			ID:        entity.SessionID(id),
			Type:      entity.SessionTypeBrowser,
			StartedAt: time.Now().Add(-time.Hour),
		},
		State:     &entity.SessionState{Tabs: tabs},
		TabCount:  tabCount,
		PaneCount: 0,
		UpdatedAt: time.Now(),
	}
}
