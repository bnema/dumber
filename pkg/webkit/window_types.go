package webkit

// WindowType indicates how a new WebView should be treated
type WindowType int

const (
	// WindowTypeTab represents an independent WebView (future: tab)
	WindowTypeTab WindowType = iota
	// WindowTypePopup represents a related WebView (shares process/context)
	WindowTypePopup
	// WindowTypeUnknown indicates type not detected yet
	WindowTypeUnknown
)

// WindowFeatures describes the window features detected from WebKitWindowProperties
type WindowFeatures struct {
	Width              int
	Height             int
	ToolbarVisible     bool
	LocationbarVisible bool
	MenubarVisible     bool
	Resizable          bool
}
