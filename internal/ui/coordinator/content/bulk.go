package content

import (
	"context"

	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// ApplyFiltersToAll applies content filters to all active webviews.
// Called when filters become available after webviews were already created.
func (c *Coordinator) ApplyFiltersToAll(ctx context.Context, applier webkit.FilterApplier) {
	log := logging.FromContext(ctx)

	for paneID, wv := range c.snapshotWebViews() {
		if wv != nil && !wv.IsDestroyed() {
			applier.ApplyTo(ctx, wv.UserContentManager())
			log.Debug().Str("pane_id", string(paneID)).Msg("applied filters to existing webview")
		}
	}
}

// ApplySettingsToAll reapplies WebKit settings to all active WebViews.
func (c *Coordinator) ApplySettingsToAll(ctx context.Context, sm *webkit.SettingsManager) {
	log := logging.FromContext(ctx)
	if sm == nil {
		return
	}

	for paneID, wv := range c.snapshotWebViews() {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		sm.ApplyToWebView(ctx, wv.Widget())
		log.Debug().Str("pane_id", string(paneID)).Msg("reapplied settings to webview")
	}
}

// RefreshInjectedScriptsToAll clears and re-injects user scripts into all active WebViews.
//
// WebKit user scripts are snapshotted when added to a WebKitUserContentManager, so when
// appearance settings change at runtime (dark mode, palettes, UI scale), we must refresh
// the scripts so future navigations pick up the latest values.
// Script refresh is deferred for any WebView that is currently loading to avoid
// removing scripts mid-navigation.
func (c *Coordinator) RefreshInjectedScriptsToAll(ctx context.Context, injector *webkit.ContentInjector) {
	log := logging.FromContext(ctx)
	if injector == nil {
		return
	}

	c.injector = injector
	for paneID, wv := range c.webViews {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			c.queueScriptRefresh(paneID)
			log.Debug().Str("pane_id", string(paneID)).Msg("deferred script refresh until load finished")
			continue
		}

		c.refreshInjectedScripts(ctx, injector, paneID, wv)
	}
}
