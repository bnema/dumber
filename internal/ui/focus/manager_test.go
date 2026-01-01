package focus

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
)

// TestNavigateWithinStack_UsesActiveStackIndex tests that navigation uses
// ActiveStackIndex from the domain model instead of pointer comparison.
// Note: navigateWithinStack is a pure function that returns the target pane ID
// without modifying state. The actual state mutation happens in ApplyFocusChange.
func TestNavigateWithinStack_UsesActiveStackIndex(t *testing.T) {
	// Create a stack with 3 children
	pane0 := &entity.Pane{ID: "pane0"}
	pane1 := &entity.Pane{ID: "pane1"}
	pane2 := &entity.Pane{ID: "pane2"}

	child0 := &entity.PaneNode{ID: "child0", Pane: pane0}
	child1 := &entity.PaneNode{ID: "child1", Pane: pane1}
	child2 := &entity.PaneNode{ID: "child2", Pane: pane2}

	stackNode := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		Children:         []*entity.PaneNode{child0, child1, child2},
		ActiveStackIndex: 1, // Currently at middle pane
	}

	// Set parents
	child0.Parent = stackNode
	child1.Parent = stackNode
	child2.Parent = stackNode

	m := NewManager(nil)

	tests := []struct {
		name        string
		activeIndex int
		direction   usecase.NavigateDirection
		wantCanNav  bool
		wantPaneID  entity.PaneID
	}{
		{
			name:        "navigate down from middle",
			activeIndex: 1,
			direction:   usecase.NavDown,
			wantCanNav:  true,
			wantPaneID:  "pane2",
		},
		{
			name:        "navigate up from middle",
			activeIndex: 1,
			direction:   usecase.NavUp,
			wantCanNav:  true,
			wantPaneID:  "pane0",
		},
		{
			name:        "navigate down from last - boundary",
			activeIndex: 2,
			direction:   usecase.NavDown,
			wantCanNav:  false,
			wantPaneID:  "",
		},
		{
			name:        "navigate up from first - boundary",
			activeIndex: 0,
			direction:   usecase.NavUp,
			wantCanNav:  false,
			wantPaneID:  "",
		},
		{
			name:        "navigate down from first",
			activeIndex: 0,
			direction:   usecase.NavDown,
			wantCanNav:  true,
			wantPaneID:  "pane1",
		},
		{
			name:        "navigate up from last",
			activeIndex: 2,
			direction:   usecase.NavUp,
			wantCanNav:  true,
			wantPaneID:  "pane1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the active index
			stackNode.ActiveStackIndex = tt.activeIndex

			// navigateWithinStack is a pure function - it does not modify state
			canNav, paneID := m.navigateWithinStack(stackNode, stackNode.Children[tt.activeIndex], tt.direction)

			assert.Equal(t, tt.wantCanNav, canNav)
			assert.Equal(t, tt.wantPaneID, paneID)
		})
	}
}

// TestNavigateWithinStack_EmptyStack tests navigation in empty stack returns false.
func TestNavigateWithinStack_EmptyStack(t *testing.T) {
	stackNode := &entity.PaneNode{
		ID:        "stack",
		IsStacked: true,
		Children:  []*entity.PaneNode{},
	}

	m := NewManager(nil)

	canNav, paneID := m.navigateWithinStack(stackNode, nil, usecase.NavDown)

	assert.False(t, canNav)
	assert.Empty(t, paneID)
}

// TestNavigateWithinStack_NotStacked tests navigation on non-stacked node returns false.
func TestNavigateWithinStack_NotStacked(t *testing.T) {
	node := &entity.PaneNode{
		ID:        "leaf",
		IsStacked: false,
	}

	m := NewManager(nil)

	canNav, paneID := m.navigateWithinStack(node, nil, usecase.NavDown)

	assert.False(t, canNav)
	assert.Empty(t, paneID)
}

// TestNavigateWithinStack_InvalidActiveIndex tests navigation with invalid ActiveStackIndex
// falls back to pointer search.
func TestNavigateWithinStack_InvalidActiveIndexFallback(t *testing.T) {
	pane0 := &entity.Pane{ID: "pane0"}
	pane1 := &entity.Pane{ID: "pane1"}

	child0 := &entity.PaneNode{ID: "child0", Pane: pane0}
	child1 := &entity.PaneNode{ID: "child1", Pane: pane1}

	stackNode := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		Children:         []*entity.PaneNode{child0, child1},
		ActiveStackIndex: -1, // Invalid index
	}

	child0.Parent = stackNode
	child1.Parent = stackNode

	m := NewManager(nil)

	// With invalid ActiveStackIndex, should fall back to pointer search
	// Pass child0 as current node
	canNav, paneID := m.navigateWithinStack(stackNode, child0, usecase.NavDown)

	assert.True(t, canNav)
	assert.Equal(t, entity.PaneID("pane1"), paneID)
}

// TestNavigateWithinStack_InvalidDirection tests navigation with invalid direction returns false.
func TestNavigateWithinStack_InvalidDirection(t *testing.T) {
	pane0 := &entity.Pane{ID: "pane0"}
	child0 := &entity.PaneNode{ID: "child0", Pane: pane0}

	stackNode := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		Children:         []*entity.PaneNode{child0},
		ActiveStackIndex: 0,
	}

	m := NewManager(nil)

	// NavLeft and NavRight should not work for stack navigation
	canNav, paneID := m.navigateWithinStack(stackNode, child0, usecase.NavLeft)

	assert.False(t, canNav)
	assert.Empty(t, paneID)
}

// TestNavigateWithinStack_ReturnsCorrectPaneID tests that navigation returns
// the correct target pane ID for sequential navigation.
// Note: navigateWithinStack is now a pure function - state updates happen in ApplyFocusChange.
func TestNavigateWithinStack_ReturnsCorrectPaneID(t *testing.T) {
	pane0 := &entity.Pane{ID: "pane0"}
	pane1 := &entity.Pane{ID: "pane1"}
	pane2 := &entity.Pane{ID: "pane2"}

	child0 := &entity.PaneNode{ID: "child0", Pane: pane0}
	child1 := &entity.PaneNode{ID: "child1", Pane: pane1}
	child2 := &entity.PaneNode{ID: "child2", Pane: pane2}

	stackNode := &entity.PaneNode{
		ID:               "stack",
		IsStacked:        true,
		Children:         []*entity.PaneNode{child0, child1, child2},
		ActiveStackIndex: 0,
	}

	m := NewManager(nil)

	// Navigate down from first - should return pane1
	stackNode.ActiveStackIndex = 0
	canNav, paneID := m.navigateWithinStack(stackNode, child0, usecase.NavDown)
	assert.True(t, canNav)
	assert.Equal(t, entity.PaneID("pane1"), paneID)

	// Navigate down from middle - should return pane2
	stackNode.ActiveStackIndex = 1
	canNav, paneID = m.navigateWithinStack(stackNode, child1, usecase.NavDown)
	assert.True(t, canNav)
	assert.Equal(t, entity.PaneID("pane2"), paneID)

	// Navigate up from last - should return pane1
	stackNode.ActiveStackIndex = 2
	canNav, paneID = m.navigateWithinStack(stackNode, child2, usecase.NavUp)
	assert.True(t, canNav)
	assert.Equal(t, entity.PaneID("pane1"), paneID)

	// Navigate up from middle - should return pane0
	stackNode.ActiveStackIndex = 1
	canNav, paneID = m.navigateWithinStack(stackNode, child1, usecase.NavUp)
	assert.True(t, canNav)
	assert.Equal(t, entity.PaneID("pane0"), paneID)
}
