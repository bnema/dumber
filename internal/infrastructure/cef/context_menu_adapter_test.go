package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

type stubContextMenuParams struct {
	x, y         int32
	pageURL      string
	linkURL      string
	sourceURL    string
	selection    string
	editable     bool
	imageContent bool
}

func (p stubContextMenuParams) GetXcoord() int32                           { return p.x }
func (p stubContextMenuParams) GetYcoord() int32                           { return p.y }
func (p stubContextMenuParams) GetTypeFlags() purecef.ContextMenuTypeFlags { return 0 }
func (p stubContextMenuParams) GetLinkURL() string                         { return p.linkURL }
func (p stubContextMenuParams) GetUnfilteredLinkURL() string               { return p.linkURL }
func (p stubContextMenuParams) GetSourceURL() string                       { return p.sourceURL }
func (p stubContextMenuParams) HasImageContents() bool                     { return p.imageContent }
func (p stubContextMenuParams) GetTitleText() string                       { return "" }
func (p stubContextMenuParams) GetPageURL() string                         { return p.pageURL }
func (p stubContextMenuParams) GetFrameURL() string                        { return "" }
func (p stubContextMenuParams) GetFrameCharset() string                    { return "" }
func (p stubContextMenuParams) GetMediaType() purecef.ContextMenuMediaType { return 0 }
func (p stubContextMenuParams) GetMediaStateFlags() purecef.ContextMenuMediaStateFlags {
	return 0
}
func (p stubContextMenuParams) GetSelectionText() string               { return p.selection }
func (p stubContextMenuParams) GetMisspelledWord() string              { return "" }
func (p stubContextMenuParams) GetDictionarySuggestions(uintptr) int32 { return 0 }
func (p stubContextMenuParams) IsEditable() bool                       { return p.editable }
func (p stubContextMenuParams) IsSpellCheckEnabled() bool              { return false }
func (p stubContextMenuParams) GetEditStateFlags() purecef.ContextMenuEditStateFlags {
	return 0
}
func (p stubContextMenuParams) IsCustomMenu() bool { return false }

func TestBuildMenuContextFromCEFParams(t *testing.T) {
	wv := &WebView{}
	wv.updateURI("https://example.com/page")
	wv.updateLoadState(false, true, false)

	ctx := buildMenuContext(wv, stubContextMenuParams{
		x:            17,
		y:            42,
		pageURL:      "https://example.com/page",
		linkURL:      "https://example.com/link",
		sourceURL:    "https://example.com/image.png",
		selection:    "selected text",
		editable:     true,
		imageContent: true,
	})

	require.Equal(t, "https://example.com/page", ctx.PageURI)
	require.Equal(t, "https://example.com/link", ctx.LinkURI)
	require.Equal(t, "https://example.com/image.png", ctx.ImageURI)
	require.Equal(t, "selected text", ctx.SelectionText)
	require.True(t, ctx.HasSelection)
	require.True(t, ctx.IsEditable)
	require.True(t, ctx.CanGoBack)
	require.False(t, ctx.CanGoForward)
	require.Equal(t, 17, ctx.X)
	require.Equal(t, 42, ctx.Y)
}

type stubRunContextMenuCallback struct {
	contCalls   int
	cancelCalls int
	commandID   int32
}

type stubContextMenuExecutor struct {
	executeCalls int
	action       port.MenuAction
	menuContext  port.MenuContext
	err          error
}

func (s *stubContextMenuExecutor) ExecuteMenuAction(_ context.Context, action port.MenuAction, menuContext port.MenuContext) error {
	s.executeCalls++
	s.action = action
	s.menuContext = menuContext
	return s.err
}

func (c *stubRunContextMenuCallback) Cont(commandID int32, _ purecef.EventFlags) {
	c.contCalls++
	c.commandID = commandID
}

func (c *stubRunContextMenuCallback) Cancel() {
	c.cancelCalls++
}

func TestContextMenuSelectionCancelsWhenCEFCommandMissing(t *testing.T) {
	callback := &stubRunContextMenuCallback{}

	dispatchContextMenuSelection(context.Background(), nil, callback, nil, map[port.MenuAction]int32{
		port.MenuActionReload: 102,
	}, port.MenuItem{Action: port.MenuActionInspectElement, Label: "Inspect Element"}, port.MenuContext{})

	require.Zero(t, callback.contCalls)
	require.Equal(t, 1, callback.cancelCalls)
}

func TestContextMenuSelectionContinuesWhenCEFCommandPresent(t *testing.T) {
	callback := &stubRunContextMenuCallback{}

	dispatchContextMenuSelection(context.Background(), nil, callback, nil, map[port.MenuAction]int32{
		port.MenuActionInspectElement: 204,
	}, port.MenuItem{Action: port.MenuActionInspectElement, Label: "Inspect Element"}, port.MenuContext{})

	require.Equal(t, 1, callback.contCalls)
	require.Zero(t, callback.cancelCalls)
	require.Equal(t, int32(204), callback.commandID)
}

func TestContextMenuSelectionExecutesDirectActionWhenExecutorAvailable(t *testing.T) {
	callback := &stubRunContextMenuCallback{}
	executor := &stubContextMenuExecutor{}
	menuContext := port.MenuContext{PageURI: "https://example.com"}

	dispatchContextMenuSelection(
		context.Background(),
		executor,
		callback,
		nil,
		map[port.MenuAction]int32{port.MenuActionReload: 102},
		port.MenuItem{Action: port.MenuActionReload, Label: "Reload"},
		menuContext,
	)

	require.Equal(t, 1, executor.executeCalls)
	require.Equal(t, port.MenuActionReload, executor.action)
	require.Equal(t, menuContext, executor.menuContext)
	require.Zero(t, callback.contCalls)
	require.Zero(t, callback.commandID)
	require.Equal(t, 1, callback.cancelCalls)
}

func TestContextMenuSelectionExecutesCopyImageDirectlyWhenExecutorAvailable(t *testing.T) {
	callback := &stubRunContextMenuCallback{}
	executor := &stubContextMenuExecutor{}
	menuContext := port.MenuContext{ImageURI: "https://example.com/image.png"}

	dispatchContextMenuSelection(
		context.Background(),
		executor,
		callback,
		nil,
		map[port.MenuAction]int32{},
		port.MenuItem{Action: port.MenuActionCopyImage, Label: "Copy Image"},
		menuContext,
	)

	require.Equal(t, 1, executor.executeCalls)
	require.Equal(t, port.MenuActionCopyImage, executor.action)
	require.Equal(t, menuContext, executor.menuContext)
	require.Zero(t, callback.contCalls)
	require.Zero(t, callback.commandID)
	require.Equal(t, 1, callback.cancelCalls)
}

func TestContextMenuSelectionUsesNativeCommandForCopySelection(t *testing.T) {
	callback := &stubRunContextMenuCallback{}
	executor := &stubContextMenuExecutor{}
	var copiedLens []int

	dispatchContextMenuSelection(
		context.Background(),
		executor,
		callback,
		func(text string) { copiedLens = append(copiedLens, len(text)) },
		map[port.MenuAction]int32{port.MenuActionCopySelection: 333},
		port.MenuItem{Action: port.MenuActionCopySelection, Label: "Copy"},
		port.MenuContext{SelectionText: "selected text", HasSelection: true},
	)

	require.Equal(t, 1, callback.contCalls)
	require.Zero(t, callback.cancelCalls)
	require.Equal(t, int32(333), callback.commandID)
	require.Zero(t, executor.executeCalls)
	require.Empty(t, copiedLens)
}

func TestContextMenuAnchorPositionScalesCEFCoordinates(t *testing.T) {
	x, y := contextMenuAnchorPosition(stubContextMenuParams{x: 320, y: 180}, 2)

	require.Equal(t, int32(160), x)
	require.Equal(t, int32(90), y)
}

func TestContextMenuRawPositionDefaultsToZeroWhenParamsNil(t *testing.T) {
	x, y := contextMenuRawPosition(nil)

	require.Zero(t, x)
	require.Zero(t, y)
}
