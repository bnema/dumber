package browser

import (
	"github.com/bnema/dumber/internal/app/messaging"
	"github.com/bnema/dumber/pkg/webkit"
)

//go:generate mockgen -source=interfaces.go -destination=mocks/mock_browser.go -package=mock_browser

// WebViewInterface abstracts WebKit WebView for testing
type WebViewInterface interface {
	LoadURL(url string) error
	Show() error
	Hide() error
	Destroy() error
	Window() *webkit.Window
	GetCurrentURL() string
	Widget() uintptr
	RootWidget() uintptr
	InjectScript(script string) error
	DispatchCustomEvent(event string, detail map[string]any) error
	RegisterKeyboardShortcut(key string, handler func()) error
	RegisterURIChangedHandler(handler func(string))
}

// PaneInterface abstracts BrowserPane for testing
type PaneInterface interface {
	WebView() WebViewInterface
	MessageHandler() MessageHandlerInterface
	NavigationController() NavigationControllerInterface
	ZoomController() ZoomControllerInterface
	ClipboardController() ClipboardControllerInterface
	ShortcutHandler() ShortcutHandlerInterface
	ID() string
	HasGUI() bool
	SetHasGUI(bool)
	SetID(string)
	UpdateLastFocus()
	HasGUIComponent(component string) bool
	SetGUIComponent(component string, loaded bool)
	Cleanup()
}

// MessageHandlerInterface abstracts messaging.Handler
type MessageHandlerInterface interface {
	SetWorkspaceObserver(observer messaging.WorkspaceObserver)
}

// NavigationControllerInterface abstracts navigation operations
type NavigationControllerInterface interface {
	NavigateToURL(url string) error
}

// ZoomControllerInterface abstracts zoom operations
type ZoomControllerInterface interface {
	ApplyInitialZoom()
}

// ClipboardControllerInterface abstracts clipboard operations
type ClipboardControllerInterface interface {
	// Add methods as needed
}

// ShortcutHandlerInterface abstracts shortcut handling
type ShortcutHandlerInterface interface {
	// Add methods as needed
}

// BrowserAppInterface abstracts BrowserApp for testing
type BrowserAppInterface interface {
	GetPanes() []*BrowserPane
	SetActivePane(pane *BrowserPane)
	GetActivePane() *BrowserPane
	AppendPane(pane *BrowserPane)
	RemovePane(pane *BrowserPane)
	BuildWebkitConfig() (*webkit.Config, error)
	CreatePaneForView(view *webkit.WebView) (*BrowserPane, error)
}

// WindowInterface abstracts webkit.Window for testing
type WindowInterface interface {
	SetChild(widget uintptr)
}
