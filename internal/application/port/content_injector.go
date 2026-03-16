package port

import "context"

// ContentInjector defines the port interface for injecting scripts and styles into web views.
// This abstracts platform-specific content injection implementations (WebKit, etc.).
type ContentInjector interface {
	// InjectThemeCSS injects CSS variables into the page for theming.
	// The css parameter should contain CSS custom property declarations.
	InjectThemeCSS(ctx context.Context, css string) error

	// RefreshScripts clears and re-injects user scripts for a single WebView.
	// Called when appearance settings change so future navigations pick up latest values.
	// Returns an error if the refresh could not be performed.
	RefreshScripts(ctx context.Context, wv WebView) error
}
