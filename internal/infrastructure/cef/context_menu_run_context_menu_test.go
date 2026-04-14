package cef

import (
	"context"
	"testing"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

type stubMenuModel struct{}

func (stubMenuModel) IsSubMenu() bool                                  { return false }
func (stubMenuModel) Clear() int32                                     { panic("unexpected call") }
func (stubMenuModel) GetCount() int                                    { return 1 }
func (stubMenuModel) AddSeparator() int32                              { panic("unexpected call") }
func (stubMenuModel) AddItem(commandID int32, label string) int32      { panic("unexpected call") }
func (stubMenuModel) AddCheckItem(commandID int32, label string) int32 { panic("unexpected call") }
func (stubMenuModel) AddRadioItem(commandID int32, label string, groupID int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) AddSubMenu(commandID int32, label string) purecef.MenuModel {
	panic("unexpected call")
}
func (stubMenuModel) InsertSeparatorAt(index int) int32 { panic("unexpected call") }
func (stubMenuModel) InsertItemAt(index int, commandID int32, label string) int32 {
	panic("unexpected call")
}
func (stubMenuModel) InsertCheckItemAt(index int, commandID int32, label string) int32 {
	panic("unexpected call")
}
func (stubMenuModel) InsertRadioItemAt(index int, commandID int32, label string, groupID int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) InsertSubMenuAt(index int, commandID int32, label string) purecef.MenuModel {
	panic("unexpected call")
}
func (stubMenuModel) Remove(commandID int32) int32                    { panic("unexpected call") }
func (stubMenuModel) RemoveAt(index int) int32                        { panic("unexpected call") }
func (stubMenuModel) GetIndexOf(commandID int32) int32                { panic("unexpected call") }
func (stubMenuModel) GetCommandIDAt(index int) int32                  { return 101 }
func (stubMenuModel) SetCommandIDAt(index int, commandID int32) int32 { panic("unexpected call") }
func (stubMenuModel) GetLabel(commandID int32) string                 { panic("unexpected call") }
func (stubMenuModel) GetLabelAt(index int) string                     { return "Copy" }
func (stubMenuModel) SetLabel(commandID int32, label string) int32    { panic("unexpected call") }
func (stubMenuModel) SetLabelAt(index int, label string) int32        { panic("unexpected call") }
func (stubMenuModel) GetType(commandID int32) purecef.MenuItemType    { panic("unexpected call") }
func (stubMenuModel) GetTypeAt(index int) purecef.MenuItemType {
	return purecef.MenuItemTypeMenuitemtypeCommand
}
func (stubMenuModel) GetGroupID(commandID int32) int32                { panic("unexpected call") }
func (stubMenuModel) GetGroupIDAt(index int) int32                    { panic("unexpected call") }
func (stubMenuModel) SetGroupID(commandID int32, groupID int32) int32 { panic("unexpected call") }
func (stubMenuModel) SetGroupIDAt(index int, groupID int32) int32     { panic("unexpected call") }
func (stubMenuModel) GetSubMenu(commandID int32) purecef.MenuModel    { panic("unexpected call") }
func (stubMenuModel) GetSubMenuAt(index int) purecef.MenuModel        { panic("unexpected call") }
func (stubMenuModel) IsVisible(commandID int32) bool                  { panic("unexpected call") }
func (stubMenuModel) IsVisibleAt(index int) bool                      { panic("unexpected call") }
func (stubMenuModel) SetVisible(commandID int32, visible int32) int32 { panic("unexpected call") }
func (stubMenuModel) SetVisibleAt(index int, visible int32) int32     { panic("unexpected call") }
func (stubMenuModel) IsEnabled(commandID int32) bool                  { panic("unexpected call") }
func (stubMenuModel) IsEnabledAt(index int) bool                      { panic("unexpected call") }
func (stubMenuModel) SetEnabled(commandID int32, enabled int32) int32 { panic("unexpected call") }
func (stubMenuModel) SetEnabledAt(index int, enabled int32) int32     { panic("unexpected call") }
func (stubMenuModel) IsChecked(commandID int32) bool                  { panic("unexpected call") }
func (stubMenuModel) IsCheckedAt(index int) bool                      { panic("unexpected call") }
func (stubMenuModel) SetChecked(commandID int32, checked int32) int32 { panic("unexpected call") }
func (stubMenuModel) SetCheckedAt(index int, checked int32) int32     { panic("unexpected call") }
func (stubMenuModel) HasAccelerator(commandID int32) bool             { panic("unexpected call") }
func (stubMenuModel) HasAcceleratorAt(index int) bool                 { panic("unexpected call") }
func (stubMenuModel) SetAccelerator(commandID int32, keyCode int32, shiftPressed int32, ctrlPressed int32, altPressed int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetAcceleratorAt(index int, keyCode int32, shiftPressed int32, ctrlPressed int32, altPressed int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) RemoveAccelerator(commandID int32) int32 { panic("unexpected call") }
func (stubMenuModel) RemoveAcceleratorAt(index int) int32     { panic("unexpected call") }
func (stubMenuModel) GetAccelerator(commandID int32, keyCode unsafe.Pointer, shiftPressed unsafe.Pointer, ctrlPressed unsafe.Pointer, altPressed unsafe.Pointer) int32 {
	panic("unexpected call")
}
func (stubMenuModel) GetAcceleratorAt(index int, keyCode unsafe.Pointer, shiftPressed unsafe.Pointer, ctrlPressed unsafe.Pointer, altPressed unsafe.Pointer) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetColor(commandID int32, colorType purecef.MenuColorType, color uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetColorAt(index int32, colorType purecef.MenuColorType, color uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) GetColor(commandID int32, colorType purecef.MenuColorType, color uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) GetColorAt(index int32, colorType purecef.MenuColorType, color uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetFontList(commandID int32, fontList string) int32 { panic("unexpected call") }
func (stubMenuModel) SetFontListAt(index int32, fontList string) int32   { panic("unexpected call") }

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
