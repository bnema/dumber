package port

import "context"

// ContentInjector defines the port interface for injecting scripts and styles into web views.
// This abstracts platform-specific content injection implementations (WebKit, etc.).
type ContentInjector interface {
	// InjectThemeCSS injects CSS variables into the page for theming.
	// The css parameter should contain CSS custom property declarations.
	InjectThemeCSS(ctx context.Context, css string) error
}
