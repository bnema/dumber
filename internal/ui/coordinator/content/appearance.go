package content

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

const (
	aboutBlankURI              = "about:blank"
	crashPageURI               = "dumb://home/crash"
	logURLMaxLen               = 80
	oauthParentRefreshDebounce = 200 * time.Millisecond

	// Dark theme background color (#0a0a0b) as float32 RGBA values
	darkBgR = 0.039
	darkBgG = 0.039
	darkBgB = 0.043
	darkBgA = 1.0
)

// ApplyWebUIThemeToAll updates theme CSS for already-loaded dumb:// pages.
// This is necessary because user scripts only run on navigation.
func (c *Coordinator) ApplyWebUIThemeToAll(ctx context.Context, prefersDark bool, cssText string) {
	log := logging.FromContext(ctx)
	if cssText == "" {
		return
	}

	c.setCurrentTheme(prefersDark, cssText)

	script, err := buildWebUIThemeScript(prefersDark, cssText)
	if err != nil {
		log.Warn().Err(err).Msg("failed to build WebUI theme script")
		return
	}

	for paneID, wv := range c.webViews {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			c.queueThemeApply(paneID, prefersDark, cssText)
			log.Debug().Str("pane_id", string(paneID)).Msg("deferred WebUI theme apply until load committed")
			continue
		}
		c.applyWebUITheme(ctx, paneID, wv, script, prefersDark)
	}
}

func buildWebUIThemeScript(prefersDark bool, cssText string) (string, error) {
	cssJSON, err := json.Marshal(cssText)
	if err != nil {
		return "", err
	}

	script := fmt.Sprintf(`(function(){
  try {
    var cssText = %s;
    var prefersDark = %t;

    // Keep the global flag in sync
    window.__dumber_gtk_prefers_dark = prefersDark;

    // Update dark/light class
    if (prefersDark) {
      document.documentElement.classList.add('dark');
      document.documentElement.classList.remove('light');
    } else {
      document.documentElement.classList.add('light');
      document.documentElement.classList.remove('dark');
    }

    // Update or insert theme style
    var style = document.querySelector('style[data-dumber-theme-vars]');
    if (!style) {
      style = document.createElement('style');
      style.setAttribute('data-dumber-theme-vars', '');
      (document.head || document.documentElement).appendChild(style);
    }
    style.textContent = cssText;

    // Notify any running WebUI that theme changed
    try {
      window.dispatchEvent(new CustomEvent('dumber:theme-changed', {
        detail: { prefersDark: prefersDark }
      }));
    } catch (e) {
      // ignore
    }

    // Keep color-scheme consistent
    var meta = document.querySelector('meta[name="color-scheme"]');
    if (!meta) {
      meta = document.createElement('meta');
      meta.name = 'color-scheme';
      document.documentElement.appendChild(meta);
    }
    meta.content = prefersDark ? 'dark light' : 'light dark';
  } catch (e) {
    console.error('[dumber] failed to apply theme', e);
  }
})();`, string(cssJSON), prefersDark)

	return script, nil
}

func (c *Coordinator) applyWebUITheme(
	ctx context.Context,
	paneID entity.PaneID,
	wv port.WebView,
	script string,
	prefersDark bool,
) {
	if wv == nil || wv.IsDestroyed() {
		return
	}
	uri := wv.URI()
	if !strings.HasPrefix(uri, "dumb://") {
		return
	}
	// Type-assert to access webkit-specific RunJavaScript
	if wkWV, ok := wv.(*webkit.WebView); ok {
		wkWV.RunJavaScript(ctx, script, "")
	}
	logging.FromContext(ctx).
		Debug().
		Str("pane_id", string(paneID)).
		Str("uri", uri).
		Bool("prefers_dark", prefersDark).
		Msg("applied WebUI theme")
}

func (c *Coordinator) queueThemeApply(paneID entity.PaneID, prefersDark bool, cssText string) {
	c.appearanceMu.Lock()
	if c.pendingThemePanes == nil {
		c.pendingThemePanes = make(map[entity.PaneID]bool)
	}
	c.pendingThemePanes[paneID] = true
	c.pendingThemeUpdate = pendingThemeUpdate{
		prefersDark: prefersDark,
		cssText:     cssText,
	}
	c.hasPendingThemeUpdate = true
	c.appearanceMu.Unlock()
}

func (c *Coordinator) setCurrentTheme(prefersDark bool, cssText string) {
	c.appearanceMu.Lock()
	c.currentTheme = pendingThemeUpdate{
		prefersDark: prefersDark,
		cssText:     cssText,
	}
	c.hasCurrentTheme = true
	c.appearanceMu.Unlock()
}

func (c *Coordinator) getCurrentTheme() (pendingThemeUpdate, bool) {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()

	if !c.hasCurrentTheme {
		return pendingThemeUpdate{}, false
	}
	return c.currentTheme, true
}

func (c *Coordinator) takePendingThemeApply(paneID entity.PaneID) (pendingThemeUpdate, bool) {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()

	if !c.hasPendingThemeUpdate || c.pendingThemePanes == nil || !c.pendingThemePanes[paneID] {
		return pendingThemeUpdate{}, false
	}
	delete(c.pendingThemePanes, paneID)
	update := c.pendingThemeUpdate
	if len(c.pendingThemePanes) == 0 {
		c.hasPendingThemeUpdate = false
	}
	return update, true
}

func (c *Coordinator) applyPendingThemeUpdate(ctx context.Context, paneID entity.PaneID, wv port.WebView) bool {
	update, ok := c.takePendingThemeApply(paneID)
	if !ok {
		return false
	}

	script, err := buildWebUIThemeScript(update.prefersDark, update.cssText)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("failed to build deferred WebUI theme script")
		return false
	}
	c.applyWebUITheme(ctx, paneID, wv, script, update.prefersDark)
	return true
}

func (c *Coordinator) applyCurrentTheme(ctx context.Context, paneID entity.PaneID, wv port.WebView) bool {
	update, ok := c.getCurrentTheme()
	if !ok || update.cssText == "" {
		return false
	}

	script, err := buildWebUIThemeScript(update.prefersDark, update.cssText)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("failed to build current WebUI theme script")
		return false
	}
	c.applyWebUITheme(ctx, paneID, wv, script, update.prefersDark)
	return true
}

func (c *Coordinator) queueScriptRefresh(paneID entity.PaneID) {
	c.appearanceMu.Lock()
	if c.pendingScriptRefresh == nil {
		c.pendingScriptRefresh = make(map[entity.PaneID]bool)
	}
	c.pendingScriptRefresh[paneID] = true
	c.appearanceMu.Unlock()
}

func (c *Coordinator) takePendingScriptRefresh(paneID entity.PaneID) bool {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()

	if c.pendingScriptRefresh == nil || !c.pendingScriptRefresh[paneID] {
		return false
	}
	delete(c.pendingScriptRefresh, paneID)
	return true
}

func (c *Coordinator) refreshPendingScripts(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	if wv == nil || wv.IsDestroyed() || c.shouldDeferAppearance(wv) {
		return
	}
	if !c.takePendingScriptRefresh(paneID) {
		return
	}
	if c.injector == nil {
		return
	}
	c.refreshInjectedScripts(ctx, c.injector, paneID, wv)
}

func (c *Coordinator) shouldDeferAppearance(wv port.WebView) bool {
	if wv == nil || wv.IsDestroyed() {
		return false
	}
	if wv.IsLoading() {
		return true
	}
	return wv.EstimatedProgress() < 1.0
}

func (c *Coordinator) refreshInjectedScripts(
	ctx context.Context,
	injector port.ContentInjector,
	paneID entity.PaneID,
	wv port.WebView,
) {
	if injector == nil || wv == nil || wv.IsDestroyed() {
		return
	}
	// Type-assert to access webkit-specific UserContentManager and InjectScripts.
	// refreshInjectedScripts is a webkit-specific operation; other engines will
	// need their own implementation when wired through a port.ScriptRefresher.
	wkInjector, ok := injector.(*webkit.ContentInjector)
	if !ok {
		return
	}
	wkWV, ok := wv.(*webkit.WebView)
	if !ok {
		return
	}
	ucm := wkWV.UserContentManager()
	if ucm == nil {
		return
	}
	ucm.RemoveAllScripts()
	ucm.RemoveAllStyleSheets()
	wkInjector.InjectScripts(ctx, ucm, wv.ID())
	logging.FromContext(ctx).Debug().Str("pane_id", string(paneID)).Msg("refreshed injected scripts for webview")
}

func (c *Coordinator) clearPendingAppearance(paneID entity.PaneID) {
	c.appearanceMu.Lock()
	if c.pendingScriptRefresh != nil {
		delete(c.pendingScriptRefresh, paneID)
	}
	if c.pendingThemePanes != nil {
		delete(c.pendingThemePanes, paneID)
		if len(c.pendingThemePanes) == 0 {
			c.hasPendingThemeUpdate = false
		}
	}
	c.appearanceMu.Unlock()
}
