package content

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// ApplyFiltersToAll applies content filters to all active webviews.
// Called when filters become available after webviews were already created.
func (c *Coordinator) ApplyFiltersToAll(ctx context.Context) {
	log := logging.FromContext(ctx)
	if c.filterApplier == nil {
		log.Debug().Msg("ApplyFiltersToAll: no filter applier configured")
		return
	}

	c.webViewsMu.RLock()
	snapshot := make([]port.WebView, 0, len(c.webViews))
	for _, wv := range c.webViews {
		snapshot = append(snapshot, wv)
	}
	c.webViewsMu.RUnlock()

	c.filterApplier.ApplyToAll(ctx, snapshot)
	log.Debug().Int("count", len(snapshot)).Msg("applied filters to all webviews")
}

// ApplySettingsToAll reapplies engine settings to all active WebViews.
func (c *Coordinator) ApplySettingsToAll(ctx context.Context) {
	log := logging.FromContext(ctx)
	if c.settingsApplier == nil {
		log.Debug().Msg("ApplySettingsToAll: no settings applier configured")
		return
	}

	c.webViewsMu.RLock()
	snapshot := make([]port.WebView, 0, len(c.webViews))
	for _, wv := range c.webViews {
		snapshot = append(snapshot, wv)
	}
	c.webViewsMu.RUnlock()

	c.settingsApplier.ApplyToAll(ctx, snapshot)
	log.Debug().Int("count", len(snapshot)).Msg("applied settings to all webviews")
}

// RefreshInjectedScriptsToAll clears and re-injects user scripts into all active WebViews.
//
// WebKit user scripts are snapshotted when added to a WebKitUserContentManager, so when
// appearance settings change at runtime (dark mode, palettes, UI scale), we must refresh
// the scripts so future navigations pick up the latest values.
// Script refresh is deferred for any WebView that is currently loading to avoid
// removing scripts mid-navigation.
func (c *Coordinator) RefreshInjectedScriptsToAll(ctx context.Context) {
	log := logging.FromContext(ctx)
	if c.injector == nil {
		return
	}

	// Snapshot webViews under lock to avoid data race with concurrent popup create/close.
	c.webViewsMu.RLock()
	snapshot := make(map[entity.PaneID]port.WebView, len(c.webViews))
	for k, v := range c.webViews {
		snapshot[k] = v
	}
	c.webViewsMu.RUnlock()

	for paneID, wv := range snapshot {
		if wv == nil || wv.IsDestroyed() {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			c.queueScriptRefresh(paneID)
			log.Debug().Str("pane_id", string(paneID)).Msg("deferred script refresh until load finished")
			continue
		}

		c.refreshInjectedScripts(ctx, c.injector, paneID, wv)
	}
}
