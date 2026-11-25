package content

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

// systemViewWebViewStub records scripts run on it. Unused port.WebView
// methods panic through the nil embedded interface.
type systemViewWebViewStub struct {
	port.WebView
	uri     string
	loading bool
	scripts []string
}

func (w *systemViewWebViewStub) URI() string                { return w.uri }
func (w *systemViewWebViewStub) IsDestroyed() bool          { return false }
func (w *systemViewWebViewStub) IsLoading() bool            { return w.loading }
func (w *systemViewWebViewStub) EstimatedProgress() float64 { return progressFor(w.loading) }
func (w *systemViewWebViewStub) RunJavaScript(_ context.Context, script string) {
	w.scripts = append(w.scripts, script)
}

func progressFor(loading bool) float64 {
	if loading {
		return 0.5
	}
	return 1.0
}

func newSystemViewCoordinator(views map[entity.PaneID]port.WebView) *Coordinator {
	return &Coordinator{webViews: views}
}

func TestBroadcastToSystemViews_RunsOnLoadedSystemViewOnly(t *testing.T) {
	t.Parallel()
	home := &systemViewWebViewStub{uri: "dumb://home"}
	web := &systemViewWebViewStub{uri: "https://example.com"}
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": home, "b": web})

	c.BroadcastToSystemViews(context.Background(), "theme", "S1")

	assert.Equal(t, []string{"S1"}, home.scripts)
	assert.Empty(t, web.scripts)
}

func TestBroadcastToSystemViews_LoadingPaneDefersToCommit(t *testing.T) {
	t.Parallel()
	wv := &systemViewWebViewStub{uri: "dumb://home", loading: true}
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()

	c.BroadcastToSystemViews(ctx, "theme", "S1")
	assert.Empty(t, wv.scripts)

	c.applySystemViewStateOnCommit(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.scripts)

	// Already applied on commit: finish must not apply again.
	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.scripts)
}

func TestBroadcastToSystemViews_BroadcastAfterCommitAppliesOnFinish(t *testing.T) {
	t.Parallel()
	wv := &systemViewWebViewStub{uri: "dumb://home", loading: true}
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()

	c.applySystemViewStateOnCommit(ctx, "a", wv)
	c.BroadcastToSystemViews(ctx, "theme", "S1")
	assert.Empty(t, wv.scripts)

	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.scripts)
}

func TestBroadcastToSystemViews_PageCommittedLaterGetsState(t *testing.T) {
	t.Parallel()
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{})
	ctx := context.Background()
	c.BroadcastToSystemViews(ctx, "theme", "S1")

	wv := &systemViewWebViewStub{uri: "dumb://history"}
	c.applySystemViewStateOnCommit(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.scripts)

	web := &systemViewWebViewStub{uri: "https://example.com"}
	c.applySystemViewStateOnCommit(ctx, "b", web)
	assert.Empty(t, web.scripts)
}

func TestBroadcastToSystemViews_LastValueWinsAndKeysKeepOrder(t *testing.T) {
	t.Parallel()
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{})
	ctx := context.Background()
	c.BroadcastToSystemViews(ctx, "theme", "T1")
	c.BroadcastToSystemViews(ctx, "config", "C1")
	c.BroadcastToSystemViews(ctx, "theme", "T2")

	wv := &systemViewWebViewStub{uri: "dumb://home"}
	c.applySystemViewStateOnCommit(ctx, "a", wv)
	assert.Equal(t, []string{"T2", "C1"}, wv.scripts)
}

func TestNotifySystemViews_NotReplayedAndSkipsLoading(t *testing.T) {
	t.Parallel()
	loaded := &systemViewWebViewStub{uri: "dumb://home"}
	loading := &systemViewWebViewStub{uri: "dumb://home", loading: true}
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": loaded, "b": loading})
	ctx := context.Background()

	c.NotifySystemViews(ctx, "N1")
	assert.Equal(t, []string{"N1"}, loaded.scripts)
	assert.Empty(t, loading.scripts)

	c.applySystemViewStateOnCommit(ctx, "b", loading)
	c.applySystemViewStateOnFinish(ctx, "b", loading)
	assert.Empty(t, loading.scripts)
}

func TestClearPendingAppearance_DropsSystemViewPending(t *testing.T) {
	t.Parallel()
	wv := &systemViewWebViewStub{uri: "dumb://home", loading: true}
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	c.BroadcastToSystemViews(context.Background(), "theme", "S1")

	c.clearPendingAppearance("a")
	assert.False(t, c.takeSystemViewPending("a"))
}

func TestBuildSystemViewEventScript_EncodesNameAndDetail(t *testing.T) {
	t.Parallel()
	script, err := BuildSystemViewEventScript("dumber:systemview-changed", map[string]string{"view": `a"b`})
	require.NoError(t, err)

	name, _ := json.Marshal("dumber:systemview-changed")
	detail, _ := json.Marshal(map[string]string{"view": `a"b`})
	assert.Contains(t, script, "new CustomEvent("+string(name)+",{detail:"+string(detail)+"})")
}
