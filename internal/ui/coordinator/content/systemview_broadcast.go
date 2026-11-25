package content

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/logging"
)

const systemViewScheme = "dumb://"

// BroadcastToSystemViews stores script as the current state for key and runs
// it on every committed dumb:// pane, again when a still loading one
// finishes. Panes that have not committed yet get it when the load commits.
// Pages that navigate to dumb:// later receive all stored state in
// insertion order. The last script for a key wins.
func (c *Coordinator) BroadcastToSystemViews(ctx context.Context, key, script string) {
	if key == "" || script == "" {
		return
	}

	c.appearanceMu.Lock()
	if c.systemViewState == nil {
		c.systemViewState = make(map[string]string)
	}
	if _, exists := c.systemViewState[key]; !exists {
		c.systemViewOrder = insertSystemViewKey(c.systemViewOrder, key)
	}
	c.systemViewState[key] = script
	c.appearanceMu.Unlock()

	log := logging.FromContext(ctx)
	for paneID, wv := range c.snapshotWebViews() {
		if !isLiveSystemView(wv) {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			c.markSystemViewPending(paneID)
			if !c.isSystemViewCommitted(paneID) {
				log.Debug().Str("pane_id", string(paneID)).Str("key", key).Msg("deferred system view broadcast until load")
				continue
			}
			// A committed page can stay loading for long (a slow embed):
			// apply now, and again when it finishes.
		}
		log.Debug().Str("pane_id", string(paneID)).Str("key", key).Msg("applied system view broadcast")
		wv.RunJavaScript(ctx, script)
	}
}

// NotifySystemViews runs script once on every dumb:// pane whose document has
// committed, even when it is still loading (a slow embed iframe keeps the
// page loading). A still loading page gets script again when it finishes,
// in case it was not listening yet. The script is not stored: a page that
// has not committed yet fetches fresh data when it starts.
func (c *Coordinator) NotifySystemViews(ctx context.Context, script string) {
	if script == "" {
		return
	}
	for paneID, wv := range c.snapshotWebViews() {
		if !isLiveSystemView(wv) {
			continue
		}
		if c.shouldDeferAppearance(wv) {
			if !c.isSystemViewCommitted(paneID) {
				continue
			}
			c.addSystemViewNotify(paneID, script)
		}
		wv.RunJavaScript(ctx, script)
	}
}

// BuildSystemViewEventScript returns JS that dispatches a window CustomEvent
// named name with detail JSON-encoded.
func BuildSystemViewEventScript(name string, detail any) (string, error) {
	nameJSON, err := json.Marshal(name)
	if err != nil {
		return "", err
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"(function(){try{window.dispatchEvent(new CustomEvent(%s,{detail:%s}));}catch(e){}})();",
		nameJSON, detailJSON,
	), nil
}

// applySystemViewStateOnCommit runs all stored state scripts on a dumb:// pane
// whose load just committed, and clears its pending mark.
func (c *Coordinator) applySystemViewStateOnCommit(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	c.takeSystemViewPending(paneID)
	live := isLiveSystemView(wv)
	c.setSystemViewCommitted(paneID, live)
	if !live {
		return
	}
	logging.FromContext(ctx).Debug().Str("pane_id", string(paneID)).Msg("applying system view state on commit")
	for _, script := range c.systemViewScripts() {
		wv.RunJavaScript(ctx, script)
	}
}

// applySystemViewStateOnFinish re-applies stored state only when a broadcast
// arrived after the load committed.
// It also re-runs the notifications sent while the page was loading.
func (c *Coordinator) applySystemViewStateOnFinish(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	pending := c.takeSystemViewPending(paneID)
	notify := c.takeSystemViewNotify(paneID)
	if !isLiveSystemView(wv) {
		return
	}
	if pending {
		for _, script := range c.systemViewScripts() {
			wv.RunJavaScript(ctx, script)
		}
	}
	for _, script := range notify {
		wv.RunJavaScript(ctx, script)
	}
}

// systemViewKeyRank orders state keys: the theme first, user CSS last so it
// overrides everything, other topics in insertion order between them.
func systemViewKeyRank(key string) int {
	switch key {
	case systemViewThemeKey:
		return 0
	case systemViewUserCSSKey:
		return 2
	default:
		return 1
	}
}

func insertSystemViewKey(order []string, key string) []string {
	rank := systemViewKeyRank(key)
	i := len(order)
	for i > 0 && systemViewKeyRank(order[i-1]) > rank {
		i--
	}
	return slices.Insert(order, i, key)
}

func (c *Coordinator) systemViewScripts() []string {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	scripts := make([]string, 0, len(c.systemViewOrder))
	for _, key := range c.systemViewOrder {
		scripts = append(scripts, c.systemViewState[key])
	}
	return scripts
}

func (c *Coordinator) markSystemViewPending(paneID entity.PaneID) {
	c.appearanceMu.Lock()
	if c.pendingSystemViewPanes == nil {
		c.pendingSystemViewPanes = make(map[entity.PaneID]bool)
	}
	c.pendingSystemViewPanes[paneID] = true
	c.appearanceMu.Unlock()
}

func (c *Coordinator) takeSystemViewPending(paneID entity.PaneID) bool {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	if !c.pendingSystemViewPanes[paneID] {
		return false
	}
	delete(c.pendingSystemViewPanes, paneID)
	return true
}

func (c *Coordinator) snapshotWebViews() map[entity.PaneID]port.WebView {
	// Snapshot under lock to avoid racing concurrent popup create/close.
	c.webViewsMu.RLock()
	defer c.webViewsMu.RUnlock()
	snapshot := make(map[entity.PaneID]port.WebView, len(c.webViews))
	maps.Copy(snapshot, c.webViews)
	return snapshot
}

func isLiveSystemView(wv port.WebView) bool {
	return wv != nil && !wv.IsDestroyed() && strings.HasPrefix(wv.URI(), systemViewScheme)
}

func (c *Coordinator) setSystemViewCommitted(paneID entity.PaneID, committed bool) {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	// A new document drops the notifications queued for the previous one.
	delete(c.pendingSystemViewNotify, paneID)
	if !committed {
		delete(c.committedSystemViewPanes, paneID)
		return
	}
	if c.committedSystemViewPanes == nil {
		c.committedSystemViewPanes = make(map[entity.PaneID]bool)
	}
	c.committedSystemViewPanes[paneID] = true
}

func (c *Coordinator) isSystemViewCommitted(paneID entity.PaneID) bool {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	return c.committedSystemViewPanes[paneID]
}

func (c *Coordinator) addSystemViewNotify(paneID entity.PaneID, script string) {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	if slices.Contains(c.pendingSystemViewNotify[paneID], script) {
		return
	}
	if c.pendingSystemViewNotify == nil {
		c.pendingSystemViewNotify = make(map[entity.PaneID][]string)
	}
	c.pendingSystemViewNotify[paneID] = append(c.pendingSystemViewNotify[paneID], script)
}

func (c *Coordinator) takeSystemViewNotify(paneID entity.PaneID) []string {
	c.appearanceMu.Lock()
	defer c.appearanceMu.Unlock()
	scripts := c.pendingSystemViewNotify[paneID]
	delete(c.pendingSystemViewNotify, paneID)
	return scripts
}
