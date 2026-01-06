package entity

import "testing"

func TestPaneNode_VisibleAreaCount(t *testing.T) {
	tests := []struct {
		name     string
		node     *PaneNode
		expected int
	}{
		{
			name: "single leaf pane",
			node: &PaneNode{
				ID:   "pane1",
				Pane: NewPane("pane1"),
			},
			expected: 1,
		},
		{
			name: "horizontal split with two leaves",
			node: &PaneNode{
				ID:       "split1",
				SplitDir: SplitHorizontal,
				Children: []*PaneNode{
					{ID: "pane1", Pane: NewPane("pane1")},
					{ID: "pane2", Pane: NewPane("pane2")},
				},
			},
			expected: 2,
		},
		{
			name: "stacked panes count as one",
			node: &PaneNode{
				ID:               "stack1",
				IsStacked:        true,
				ActiveStackIndex: 0,
				Children: []*PaneNode{
					{ID: "pane1", Pane: NewPane("pane1")},
					{ID: "pane2", Pane: NewPane("pane2")},
					{ID: "pane3", Pane: NewPane("pane3")},
				},
			},
			expected: 1,
		},
		{
			name: "split with one stacked side",
			node: &PaneNode{
				ID:       "split1",
				SplitDir: SplitHorizontal,
				Children: []*PaneNode{
					{ID: "pane1", Pane: NewPane("pane1")},
					{
						ID:               "stack1",
						IsStacked:        true,
						ActiveStackIndex: 0,
						Children: []*PaneNode{
							{ID: "pane2", Pane: NewPane("pane2")},
							{ID: "pane3", Pane: NewPane("pane3")},
						},
					},
				},
			},
			expected: 2,
		},
		{
			name: "nested splits",
			node: &PaneNode{
				ID:       "split1",
				SplitDir: SplitHorizontal,
				Children: []*PaneNode{
					{ID: "pane1", Pane: NewPane("pane1")},
					{
						ID:       "split2",
						SplitDir: SplitVertical,
						Children: []*PaneNode{
							{ID: "pane2", Pane: NewPane("pane2")},
							{ID: "pane3", Pane: NewPane("pane3")},
						},
					},
				},
			},
			expected: 3,
		},
		{
			name: "complex tree with stacks and splits",
			node: &PaneNode{
				ID:       "split1",
				SplitDir: SplitHorizontal,
				Children: []*PaneNode{
					{
						ID:               "stack1",
						IsStacked:        true,
						ActiveStackIndex: 0,
						Children: []*PaneNode{
							{ID: "pane1", Pane: NewPane("pane1")},
							{ID: "pane2", Pane: NewPane("pane2")},
						},
					},
					{
						ID:       "split2",
						SplitDir: SplitVertical,
						Children: []*PaneNode{
							{ID: "pane3", Pane: NewPane("pane3")},
							{
								ID:               "stack2",
								IsStacked:        true,
								ActiveStackIndex: 1,
								Children: []*PaneNode{
									{ID: "pane4", Pane: NewPane("pane4")},
									{ID: "pane5", Pane: NewPane("pane5")},
								},
							},
						},
					},
				},
			},
			expected: 3, // stack1(1) + pane3(1) + stack2(1)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.node.VisibleAreaCount()
			if got != tt.expected {
				t.Errorf("VisibleAreaCount() = %d, want %d", got, tt.expected)
			}
		})
	}
}
