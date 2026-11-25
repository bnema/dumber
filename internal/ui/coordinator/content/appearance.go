package content

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/logging"
)

const (
	aboutBlankURI              = "about:blank"
	crashPageURI               = "dumb://history/crash"
	systemViewThemeKey         = "theme"
	logURLMaxLen               = 80
	oauthParentRefreshDebounce = 200 * time.Millisecond

	// Dark theme background color (#0a0a0b) as float32 RGBA values
	darkBgR = 0.039
	darkBgG = 0.039
	darkBgB = 0.043
	darkBgA = 1.0
)

// ApplyWebUIThemeToAll pushes theme CSS to dumb:// pages, including pages
// loaded later. User scripts only run on navigation, so loaded pages need it.
func (c *Coordinator) ApplyWebUIThemeToAll(ctx context.Context, prefersDark bool, cssText string) {
	if cssText == "" {
		return
	}
	script, err := buildWebUIThemeScript(prefersDark, cssText)
	if err != nil {
		logging.FromContext(ctx).Warn().Err(err).Msg("failed to build WebUI theme script")
		return
	}
	c.appearanceMu.Lock()
	c.prefersDarkWebUI = prefersDark
	c.appearanceMu.Unlock()
	c.BroadcastToSystemViews(ctx, systemViewThemeKey, script)
}

// webUIPrefersDark reports whether the last WebUI theme pushed was dark.
func (c *Coordinator) webUIPrefersDark() bool {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	return c.prefersDarkWebUI
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
    var root = document.documentElement;

    // Keep the global flags in sync for both WebKit and CEF-backed pages.
    window.__dumber_gtk_prefers_dark = prefersDark;
    window.__dumber_cef_prefers_dark = prefersDark;

    // Update dark/light class
    if (prefersDark) {
      root.classList.add('dark');
      root.classList.remove('light');
    } else {
      root.classList.add('light');
      root.classList.remove('dark');
    }
    root.style.colorScheme = prefersDark ? 'dark' : 'light';

    // Update or insert theme style
    var style = document.getElementById('dumber-theme-vars') || document.querySelector('style[data-dumber-theme-vars]');
    if (!style) {
      style = document.createElement('style');
      style.id = 'dumber-theme-vars';
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
      (document.head || document.documentElement).appendChild(meta);
    }
    meta.content = prefersDark ? 'dark light' : 'light dark';
  } catch (e) {
    console.error('[dumber] failed to apply theme', e);
  }
})();`, string(cssJSON), prefersDark)

	return script, nil
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
	if err := injector.RefreshScripts(ctx, wv); err != nil {
		logging.FromContext(ctx).Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to refresh injected scripts")
		return
	}
	logging.FromContext(ctx).Debug().Str("pane_id", string(paneID)).Msg("refreshed injected scripts for webview")
}

func (c *Coordinator) clearPendingAppearance(paneID entity.PaneID) {
	c.appearanceMu.Lock()
	if c.pendingScriptRefresh != nil {
		delete(c.pendingScriptRefresh, paneID)
	}
	delete(c.pendingSystemViewPanes, paneID)
	c.appearanceMu.Unlock()
}
