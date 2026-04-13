package webkit

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestBuildButtons(t *testing.T) {
	items := []port.MenuItem{
		{Action: port.MenuActionBack, Label: "Back"},
		{},
		{Action: port.MenuActionReload, Label: "Reload"},
		{Action: port.MenuActionInspectElement, Label: "Inspect Element"},
	}

	renderItems := buildButtons(items)
	require.Len(t, renderItems, 4)

	require.Equal(t, "Back", renderItems[0].item.Label)
	require.False(t, renderItems[0].separator)

	require.True(t, renderItems[1].separator)

	require.Equal(t, "Reload", renderItems[2].item.Label)
	require.False(t, renderItems[2].separator)

	require.Equal(t, "Inspect Element", renderItems[3].item.Label)
	require.False(t, renderItems[3].separator)
}
