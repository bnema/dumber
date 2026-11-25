package content

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
	"bnema.dev/dumber/internal/logging"
)

const systemViewScheme = "dumb://"

// BroadcastToSystemViews stores script as the current state for key and runs
// it on every loaded dumb:// pane. Panes still loading get it when the load
// commits. Pages that navigate to dumb:// later receive all stored state in
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
		c.systemViewOrder = append(c.systemViewOrder, key)
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
			log.Debug().Str("pane_id", string(paneID)).Str("key", key).Msg("deferred system view broadcast until load")
			continue
		}
		wv.RunJavaScript(ctx, script)
	}
}

// NotifySystemViews runs script once on every loaded dumb:// pane. The script
// is not stored and loading panes are skipped, because they fetch fresh data
// when their load completes.
func (c *Coordinator) NotifySystemViews(ctx context.Context, script string) {
	if script == "" {
		return
	}
	for _, wv := range c.snapshotWebViews() {
		if !isLiveSystemView(wv) || c.shouldDeferAppearance(wv) {
			continue
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
	if !isLiveSystemView(wv) {
		return
	}
	for _, script := range c.systemViewScripts() {
		wv.RunJavaScript(ctx, script)
	}
}

// applySystemViewStateOnFinish re-applies stored state only when a broadcast
// arrived after the load committed.
func (c *Coordinator) applySystemViewStateOnFinish(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	if !c.takeSystemViewPending(paneID) || !isLiveSystemView(wv) {
		return
	}
	for _, script := range c.systemViewScripts() {
		wv.RunJavaScript(ctx, script)
	}
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
