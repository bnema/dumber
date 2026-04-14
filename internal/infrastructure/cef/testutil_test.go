package cef

import (
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
)

type stubMenuModel struct{}

func (stubMenuModel) IsSubMenu() bool                 { return false }
func (stubMenuModel) Clear() int32                    { panic("unexpected call") }
func (stubMenuModel) GetCount() int                   { return 1 }
func (stubMenuModel) AddSeparator() int32             { panic("unexpected call") }
func (stubMenuModel) AddItem(_ int32, _ string) int32 { panic("unexpected call") }
func (stubMenuModel) AddCheckItem(_ int32, _ string) int32 {
	panic("unexpected call")
}
func (stubMenuModel) AddRadioItem(_ int32, _ string, _ int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) AddSubMenu(_ int32, _ string) purecef.MenuModel {
	panic("unexpected call")
}
func (stubMenuModel) InsertSeparatorAt(_ int) int32 { panic("unexpected call") }
func (stubMenuModel) InsertItemAt(_ int, _ int32, _ string) int32 {
	panic("unexpected call")
}
func (stubMenuModel) InsertCheckItemAt(_ int, _ int32, _ string) int32 {
	panic("unexpected call")
}
func (stubMenuModel) InsertRadioItemAt(_ int, _ int32, _ string, _ int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) InsertSubMenuAt(_ int, _ int32, _ string) purecef.MenuModel {
	panic("unexpected call")
}
func (stubMenuModel) Remove(_ int32) int32                 { panic("unexpected call") }
func (stubMenuModel) RemoveAt(_ int) int32                 { panic("unexpected call") }
func (stubMenuModel) GetIndexOf(_ int32) int32             { panic("unexpected call") }
func (stubMenuModel) GetCommandIDAt(_ int) int32           { return 101 }
func (stubMenuModel) SetCommandIDAt(_ int, _ int32) int32  { panic("unexpected call") }
func (stubMenuModel) GetLabel(_ int32) string              { panic("unexpected call") }
func (stubMenuModel) GetLabelAt(_ int) string              { return "Copy" }
func (stubMenuModel) SetLabel(_ int32, _ string) int32     { panic("unexpected call") }
func (stubMenuModel) SetLabelAt(_ int, _ string) int32     { panic("unexpected call") }
func (stubMenuModel) GetType(_ int32) purecef.MenuItemType { panic("unexpected call") }
func (stubMenuModel) GetTypeAt(_ int) purecef.MenuItemType {
	return purecef.MenuItemTypeMenuitemtypeCommand
}
func (stubMenuModel) GetGroupID(_ int32) int32             { panic("unexpected call") }
func (stubMenuModel) GetGroupIDAt(_ int) int32             { panic("unexpected call") }
func (stubMenuModel) SetGroupID(_, _ int32) int32          { panic("unexpected call") }
func (stubMenuModel) SetGroupIDAt(_ int, _ int32) int32    { panic("unexpected call") }
func (stubMenuModel) GetSubMenu(_ int32) purecef.MenuModel { panic("unexpected call") }
func (stubMenuModel) GetSubMenuAt(_ int) purecef.MenuModel { panic("unexpected call") }
func (stubMenuModel) IsVisible(_ int32) bool               { panic("unexpected call") }
func (stubMenuModel) IsVisibleAt(_ int) bool               { panic("unexpected call") }
func (stubMenuModel) SetVisible(_, _ int32) int32          { panic("unexpected call") }
func (stubMenuModel) SetVisibleAt(_ int, _ int32) int32    { panic("unexpected call") }
func (stubMenuModel) IsEnabled(_ int32) bool               { panic("unexpected call") }
func (stubMenuModel) IsEnabledAt(_ int) bool               { panic("unexpected call") }
func (stubMenuModel) SetEnabled(_, _ int32) int32          { panic("unexpected call") }
func (stubMenuModel) SetEnabledAt(_ int, _ int32) int32    { panic("unexpected call") }
func (stubMenuModel) IsChecked(_ int32) bool               { panic("unexpected call") }
func (stubMenuModel) IsCheckedAt(_ int) bool               { panic("unexpected call") }
func (stubMenuModel) SetChecked(_, _ int32) int32          { panic("unexpected call") }
func (stubMenuModel) SetCheckedAt(_ int, _ int32) int32    { panic("unexpected call") }
func (stubMenuModel) HasAccelerator(_ int32) bool          { panic("unexpected call") }
func (stubMenuModel) HasAcceleratorAt(_ int) bool          { panic("unexpected call") }
func (stubMenuModel) SetAccelerator(_, _, _, _, _ int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetAcceleratorAt(_ int, _, _, _, _ int32) int32 {
	panic("unexpected call")
}
func (stubMenuModel) RemoveAccelerator(_ int32) int32 { panic("unexpected call") }
func (stubMenuModel) RemoveAcceleratorAt(_ int) int32 { panic("unexpected call") }
func (stubMenuModel) GetAccelerator(_ int32, _, _, _, _ unsafe.Pointer) int32 {
	panic("unexpected call")
}
func (stubMenuModel) GetAcceleratorAt(_ int, _, _, _, _ unsafe.Pointer) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetColor(_ int32, _ purecef.MenuColorType, _ uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetColorAt(_ int32, _ purecef.MenuColorType, _ uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) GetColor(_ int32, _ purecef.MenuColorType, _ uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) GetColorAt(_ int32, _ purecef.MenuColorType, _ uintptr) int32 {
	panic("unexpected call")
}
func (stubMenuModel) SetFontList(_ int32, _ string) int32   { panic("unexpected call") }
func (stubMenuModel) SetFontListAt(_ int32, _ string) int32 { panic("unexpected call") }
