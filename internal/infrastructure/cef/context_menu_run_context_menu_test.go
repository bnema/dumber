package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type stubContextMenuBuilder struct{ items []port.MenuItem }

func (b stubContextMenuBuilder) Build(context.Context, port.MenuContext) []port.MenuItem {
	return b.items
}

func TestRunContextMenuReturnsZeroWhenAnchorMissing(t *testing.T) {
	h := &handlerSet{wv: &WebView{engine: &Engine{ctxMenuBuilder: stubContextMenuBuilder{items: []port.MenuItem{{Action: port.MenuActionCopySelection, Label: "Copy"}}}}}}
	callback := &stubRunContextMenuCallback{}

	result := h.RunContextMenu(nil, nil, stubContextMenuParams{}, stubMenuModel{}, callback)

	require.Equal(t, int32(0), result)
	require.Zero(t, callback.cancelCalls)
}
