package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestFloatingSessionRegistry(t *testing.T) {
	tab1, tab2 := entity.TabID("tab-1"), entity.TabID("tab-2")
	gmail := newFloatingPaneSession(tab1, "profile:gmail")
	blank := newFloatingPaneSession(tab1, "profile:blank")
	other := newFloatingPaneSession(tab2, "profile:gmail")
	blank.pane.Show()

	r := floatingSessionRegistry{
		floatingSessionMapKey(tab1, "profile:gmail"): gmail,
		floatingSessionMapKey(tab1, "profile:blank"): blank,
		floatingSessionMapKey(tab2, "profile:gmail"): other,
		floatingSessionMapKey(tab2, "profile:nil"):   nil,
	}

	t.Run("forTab skips other tabs and nil sessions", func(t *testing.T) {
		assert.Len(t, r.forTab(tab1), 2)
		assert.Len(t, r.forTab(tab2), 1)
	})

	t.Run("byPaneID maps the pane to its owning key", func(t *testing.T) {
		key, session, ok := r.byPaneID(other.paneID)
		require.True(t, ok)
		assert.Same(t, other, session)
		assert.Equal(t, tab2, key.tabID)

		_, _, ok = r.byPaneID("")
		assert.False(t, ok)
	})

	t.Run("visibleForTab returns only the shown session", func(t *testing.T) {
		_, session, ok := r.visibleForTab(tab1)
		require.True(t, ok)
		assert.Same(t, blank, session)

		key, _, ok := r.visibleForTab(tab2)
		assert.False(t, ok)
		assert.Equal(t, tab2, key.tabID, "miss still reports the tab")
	})

	t.Run("tabIDs is distinct and sorted", func(t *testing.T) {
		assert.Equal(t, []entity.TabID{tab1, tab2}, r.tabIDs())
	})
}
