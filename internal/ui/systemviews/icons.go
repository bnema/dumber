package systemviews

type systemIcon string

const (
	iconTrash     systemIcon = "trash-2"
	iconX         systemIcon = "x"
	iconSearch    systemIcon = "search"
	iconSave      systemIcon = "save"
	iconPlus      systemIcon = "plus"
	iconExternal  systemIcon = "external-link"
	iconRotateCCW systemIcon = "rotate-ccw"
	iconCheck     systemIcon = "check"
	iconHistory   systemIcon = "history"
	iconFolder    systemIcon = "folder"
	iconTag       systemIcon = "tag"
	iconSettings  systemIcon = "settings"
	iconAlert     systemIcon = "alert-triangle"
)

func iconClass(name systemIcon) string {
	if name == "" {
		return "sv-icon"
	}
	return "sv-icon sv-icon-" + string(name)
}
