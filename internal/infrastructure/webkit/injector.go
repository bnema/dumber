package webkit

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	// ScriptWorldName is the isolated world used for the injected UI.
	ScriptWorldName = "dumber"
	// MessageHandlerName is the name of the script message handler registered with WebKit.
	MessageHandlerName = "dumber"
)

// ContentInjector encapsulates script and style injection into WebViews.
// It wraps WebKit's UserContentManager to load our frontend assets.
type ContentInjector struct {
	allowList   []string
	blockList   []string
	prefersDark bool
}

// NewContentInjector creates a new injector instance with config-based dark mode detection.
func NewContentInjector(ctx context.Context, cfg *config.Config) *ContentInjector {
	ci := &ContentInjector{}
	ci.resolveColorScheme(ctx, cfg)
	return ci
}

// resolveColorScheme determines dark mode preference from config or system.
func (ci *ContentInjector) resolveColorScheme(ctx context.Context, cfg *config.Config) {
	log := logging.FromContext(ctx)

	if cfg != nil {
		switch cfg.Appearance.ColorScheme {
		case "prefer-dark", "dark":
			ci.prefersDark = true
			log.Debug().Bool("prefers_dark", true).Msg("color scheme from config: dark")
			return
		case "prefer-light", "light":
			ci.prefersDark = false
			log.Debug().Bool("prefers_dark", false).Msg("color scheme from config: light")
			return
		}
	}

	// Fallback: detect from GTK_THEME environment variable
	gtkTheme := os.Getenv("GTK_THEME")
	ci.prefersDark = strings.Contains(strings.ToLower(gtkTheme), "dark")
	log.Debug().Bool("prefers_dark", ci.prefersDark).Str("gtk_theme", gtkTheme).Msg("color scheme from environment")
}

// PrefersDark returns the current dark mode preference.
func (ci *ContentInjector) PrefersDark() bool {
	return ci.prefersDark
}

// SetURLFilters configures optional allow/block lists for script/style injection.
func (ci *ContentInjector) SetURLFilters(allowList, blockList []string) {
	ci.allowList = allowList
	ci.blockList = blockList
}

// InjectScripts adds the bootstrap and GUI scripts to the given content manager.
func (ci *ContentInjector) InjectScripts(ctx context.Context, ucm *webkit.UserContentManager, webviewID WebViewID) {
	log := logging.FromContext(ctx).With().Str("component", "content-injector").Logger()

	if ucm == nil {
		log.Warn().Msg("cannot inject scripts: user content manager is nil")
		return
	}

	addScript := func(script *webkit.UserScript, label string) {
		if script == nil {
			log.Warn().Str("script", label).Msg("failed to create user script")
			return
		}
		ucm.AddScript(script)
		log.Debug().Str("script", label).Msg("injected user script")
	}

	// Inject GTK dark mode preference for color-scheme.js (must be before ColorSchemeScript)
	darkModeScript := fmt.Sprintf("window.__dumber_gtk_prefers_dark=%t;", ci.prefersDark)
	addScript(
		webkit.NewUserScript(
			darkModeScript,
			webkit.UserContentInjectTopFrameValue,
			webkit.UserScriptInjectAtDocumentStartValue,
			nil,
			nil,
		),
		"gtk-dark-mode",
	)

	// Expose the WebView ID early so JS can tag outgoing messages.
	// Inject in both main world and isolated world since they have separate window objects.
	if webviewID != 0 {
		idScript := fmt.Sprintf("window.__dumber_webview_id=%d;", uint64(webviewID))
		// Main world injection
		addScript(
			webkit.NewUserScript(
				idScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				nil,
				nil,
			),
			"webview-id",
		)
		// Isolated world injection
		addScript(
			webkit.NewUserScriptForWorld(
				idScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				ScriptWorldName,
				nil,
				nil,
			),
			"webview-id-isolated",
		)
	}

	// Inject color-scheme handler (uses __dumber_gtk_prefers_dark)
	if assets.ColorSchemeScript != "" {
		addScript(
			webkit.NewUserScript(
				assets.ColorSchemeScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				nil,
				nil,
			),
			"color-scheme",
		)
	}

	if assets.MainWorldScript != "" {
		addScript(
			webkit.NewUserScript(
				assets.MainWorldScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentStartValue,
				ci.allowList,
				ci.blockList,
			),
			"main-world",
		)
	} else {
		log.Warn().Msg("MainWorldScript asset is empty; skipping injection")
	}

	if assets.GUIScript != "" {
		addScript(
			webkit.NewUserScriptForWorld(
				assets.GUIScript,
				webkit.UserContentInjectTopFrameValue,
				webkit.UserScriptInjectAtDocumentEndValue,
				ScriptWorldName,
				ci.allowList,
				ci.blockList,
			),
			"gui-world",
		)
	} else {
		log.Warn().Msg("GUIScript asset is empty; skipping injection")
	}
}

// InjectStyles adds the component CSS to the isolated world.
func (ci *ContentInjector) InjectStyles(ctx context.Context, ucm *webkit.UserContentManager) {
	log := logging.FromContext(ctx).With().Str("component", "content-injector").Logger()

	if ucm == nil {
		log.Warn().Msg("cannot inject styles: user content manager is nil")
		return
	}

	if assets.ComponentStyles == "" {
		log.Warn().Msg("ComponentStyles asset is empty; skipping injection")
		return
	}

	style := webkit.NewUserStyleSheetForWorld(
		assets.ComponentStyles,
		webkit.UserContentInjectTopFrameValue,
		webkit.UserStyleLevelUserValue,
		ScriptWorldName,
		ci.allowList,
		ci.blockList,
	)
	if style == nil {
		log.Warn().Msg("failed to create user style sheet")
		return
	}

	ucm.AddStyleSheet(style)
	log.Debug().Msg("injected component styles")
}
