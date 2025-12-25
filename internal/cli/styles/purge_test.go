package styles_test

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTheme() *styles.Theme {
	return styles.NewTheme(config.DefaultConfig())
}

func TestPurgeModel_SessionsDisabledWhenDataSelected(t *testing.T) {
	theme := testTheme()

	targets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetConfig, Path: "/config", Exists: true, Size: 100},
		{Type: entity.PurgeTargetData, Path: "/data", Exists: true, Size: 1000},
	}

	endedAt := time.Now().Add(-time.Hour)
	sessions := []entity.SessionPurgeItem{
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session1",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-2 * time.Hour),
					EndedAt:   &endedAt,
				},
				TabCount:  2,
				PaneCount: 3,
			},
			Selected: true,
		},
	}

	m := styles.NewPurgeWithSessions(theme, targets, sessions, 500)

	// Initially Data is selected (default for existing items)
	assert.False(t, m.SessionsEnabled(), "sessions should be disabled when Data is selected")

	// Deselect Data
	m.Items[1].Selected = false
	assert.True(t, m.SessionsEnabled(), "sessions should be enabled when Data is not selected")
}

func TestPurgeModel_SelectedSessionIDs_ReturnsNilWhenDataSelected(t *testing.T) {
	theme := testTheme()

	targets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetData, Path: "/data", Exists: true, Size: 1000},
	}

	endedAt := time.Now().Add(-time.Hour)
	sessions := []entity.SessionPurgeItem{
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session1",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-2 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
	}

	m := styles.NewPurgeWithSessions(theme, targets, sessions, 500)

	// Data is selected, so sessions should return nil
	ids := m.SelectedSessionIDs()
	assert.Nil(t, ids, "should return nil when Data is selected")

	// Deselect Data and select all sessions
	m.Items[0].Selected = false
	m.ToggleAllSessions() // Now allSessionsSelected = true
	ids = m.SelectedSessionIDs()
	require.Len(t, ids, 1)
	assert.Equal(t, entity.SessionID("session1"), ids[0])
}

func TestPurgeModel_SelectedCount_IncludesSessions(t *testing.T) {
	theme := testTheme()

	targets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetConfig, Path: "/config", Exists: true, Size: 100},
		{Type: entity.PurgeTargetCache, Path: "/cache", Exists: true, Size: 200},
	}

	endedAt := time.Now().Add(-time.Hour)
	sessions := []entity.SessionPurgeItem{
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session1",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-2 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session2",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-3 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
	}

	m := styles.NewPurgeWithSessions(theme, targets, sessions, 500)

	// 2 targets selected, sessions not selected = 2
	assert.Equal(t, 2, m.SelectedCount())

	// Select all sessions (as a single item)
	m.ToggleAllSessions()
	// 2 targets + 1 sessions item = 3
	assert.Equal(t, 3, m.SelectedCount())
}

func TestPurgeModel_ToggleAllSessions(t *testing.T) {
	theme := testTheme()

	targets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetConfig, Path: "/config", Exists: true, Size: 100},
	}

	endedAt := time.Now().Add(-time.Hour)
	sessions := []entity.SessionPurgeItem{
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session1",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-2 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session2",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-3 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
	}

	m := styles.NewPurgeWithSessions(theme, targets, sessions, 500)

	// Initially sessions are not selected
	ids := m.SelectedSessionIDs()
	assert.Nil(t, ids, "sessions should not be selected initially")

	// Toggle to select all
	m.ToggleAllSessions()
	ids = m.SelectedSessionIDs()
	require.Len(t, ids, 2, "all sessions should be selected after toggle")

	// Toggle again to deselect all
	m.ToggleAllSessions()
	ids = m.SelectedSessionIDs()
	assert.Nil(t, ids, "sessions should be deselected after second toggle")
}

func TestPurgeModel_SelectedSize_IncludesAllSessionsWhenSelected(t *testing.T) {
	theme := testTheme()

	targets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetConfig, Path: "/config", Exists: true, Size: 100},
	}

	endedAt := time.Now().Add(-time.Hour)
	sessions := []entity.SessionPurgeItem{
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session1",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-2 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session2",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-3 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
	}

	// Total sessions size is 1000
	m := styles.NewPurgeWithSessions(theme, targets, sessions, 1000)

	// Only config selected = 100
	assert.Equal(t, int64(100), m.SelectedSize())

	// Select all sessions
	m.ToggleAllSessions()
	// 100 (config) + 1000 (all sessions) = 1100
	assert.Equal(t, int64(1100), m.SelectedSize())
}

func TestPurgeModel_SelectedSize_IncludesFullSessionsSizeWhenDataSelected(t *testing.T) {
	theme := testTheme()

	targets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetData, Path: "/data", Exists: true, Size: 100},
	}

	endedAt := time.Now().Add(-time.Hour)
	sessions := []entity.SessionPurgeItem{
		{
			Info: entity.SessionInfo{
				Session: &entity.Session{
					ID:        "session1",
					Type:      entity.SessionTypeBrowser,
					StartedAt: time.Now().Add(-2 * time.Hour),
					EndedAt:   &endedAt,
				},
			},
		},
	}

	m := styles.NewPurgeWithSessions(theme, targets, sessions, 1000)

	// 100 (data) + 1000 (all sessions because DB is being deleted) = 1100
	assert.Equal(t, int64(1100), m.SelectedSize())
}
