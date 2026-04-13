package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestBuildContextMenuUseCase_Build(t *testing.T) {
	uc := NewBuildContextMenuUseCase()

	tests := []struct {
		name     string
		context  port.MenuContext
		expected []port.MenuAction
	}{
		{
			name:     "editable context uses native menu",
			context:  port.MenuContext{IsEditable: true},
			expected: []port.MenuAction{},
		},
		{
			name:     "empty context baseline",
			context:  port.MenuContext{},
			expected: []port.MenuAction{port.MenuActionReload, port.MenuActionInspectElement},
		},
		{
			name:    "image only",
			context: port.MenuContext{ImageURI: "https://example.com/image.png"},
			expected: []port.MenuAction{
				port.MenuActionReload,
				port.MenuActionCopyImage,
				port.MenuActionSaveImage,
				port.MenuActionInspectElement,
			},
		},
		{
			name:    "link only",
			context: port.MenuContext{LinkURI: "https://example.com/link"},
			expected: []port.MenuAction{
				port.MenuActionReload,
				port.MenuActionOpenLinkNewTab,
				port.MenuActionCopyLink,
				port.MenuActionInspectElement,
			},
		},
		{
			name:    "navigation enabled states",
			context: port.MenuContext{CanGoBack: true, CanGoForward: true},
			expected: []port.MenuAction{
				port.MenuActionBack,
				port.MenuActionForward,
				port.MenuActionReload,
				port.MenuActionInspectElement,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := uc.Build(context.Background(), tt.context)
			require.Equal(t, tt.expected, menuActions(items))
		})
	}
}

func menuActions(items []port.MenuItem) []port.MenuAction {
	actions := make([]port.MenuAction, 0, len(items))
	for _, item := range items {
		actions = append(actions, item.Action)
	}
	return actions
}
