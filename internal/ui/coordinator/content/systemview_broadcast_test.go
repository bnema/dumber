package content

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
)

// fakeView is a mocks.MockWebView whose URI and loading state the test sets,
// and which records the scripts run on it.
type fakeView struct {
	*mocks.MockWebView
	mu      sync.Mutex
	uri     string
	loading bool
	scripts []string
}

func newFakeView(t *testing.T, uri string, loading bool) *fakeView {
	t.Helper()
	v := &fakeView{MockWebView: mocks.NewMockWebView(t), uri: uri, loading: loading}
	v.EXPECT().URI().RunAndReturn(func() string {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.uri
	}).Maybe()
	v.EXPECT().IsDestroyed().Return(false).Maybe()
	v.EXPECT().IsLoading().RunAndReturn(func() bool {
		v.mu.Lock()
		defer v.mu.Unlock()
		return v.loading
	}).Maybe()
	v.EXPECT().EstimatedProgress().RunAndReturn(func() float64 {
		v.mu.Lock()
		defer v.mu.Unlock()
		if v.loading {
			return 0.5
		}
		return 1.0
	}).Maybe()
	v.EXPECT().RunJavaScript(mock.Anything, mock.Anything).Run(func(_ context.Context, script string) {
		v.mu.Lock()
		defer v.mu.Unlock()
		v.scripts = append(v.scripts, script)
	}).Maybe()
	return v
}

func (v *fakeView) setLoading(loading bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.loading = loading
}

func (v *fakeView) ran() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return slices.Clone(v.scripts)
}

func newSystemViewCoordinator(views map[entity.PaneID]port.WebView) *Coordinator {
	return &Coordinator{webViews: views}
}

func TestBroadcastToSystemViews_RunsOnLoadedSystemViewOnly(t *testing.T) {
	t.Parallel()
	home := newFakeView(t, "dumb://home", false)
	web := newFakeView(t, "https://example.com", false)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": home, "b": web})

	c.BroadcastToSystemViews(context.Background(), "theme", "S1")

	assert.Equal(t, []string{"S1"}, home.ran())
	assert.Empty(t, web.ran())
}

func TestBroadcastToSystemViews_LoadingPaneDefersToCommit(t *testing.T) {
	t.Parallel()
	wv := newFakeView(t, "dumb://home", true)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()

	c.BroadcastToSystemViews(ctx, "theme", "S1")
	assert.Empty(t, wv.ran())

	c.applySystemViewStateOnCommit(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.ran())

	// Already applied on commit: finish must not apply again.
	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.ran())
}

// A committed page kept loading by a slow embed gets state now and again
// when it finishes.
func TestBroadcastToSystemViews_BroadcastAfterCommitAppliesNowAndOnFinish(t *testing.T) {
	t.Parallel()
	wv := newFakeView(t, "dumb://home", true)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()

	c.applySystemViewStateOnCommit(ctx, "a", wv)
	c.BroadcastToSystemViews(ctx, "theme", "S1")
	assert.Equal(t, []string{"S1"}, wv.ran())

	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Equal(t, []string{"S1", "S1"}, wv.ran())
}

func TestBroadcastToSystemViews_PageCommittedLaterGetsState(t *testing.T) {
	t.Parallel()
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{})
	ctx := context.Background()
	c.BroadcastToSystemViews(ctx, "theme", "S1")

	wv := newFakeView(t, "dumb://history", false)
	c.applySystemViewStateOnCommit(ctx, "a", wv)
	assert.Equal(t, []string{"S1"}, wv.ran())

	web := newFakeView(t, "https://example.com", false)
	c.applySystemViewStateOnCommit(ctx, "b", web)
	assert.Empty(t, web.ran())
}

func TestBroadcastToSystemViews_LastValueWinsAndKeysKeepOrder(t *testing.T) {
	t.Parallel()
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{})
	ctx := context.Background()
	c.BroadcastToSystemViews(ctx, "theme", "T1")
	c.BroadcastToSystemViews(ctx, "config", "C1")
	c.BroadcastToSystemViews(ctx, "theme", "T2")

	wv := newFakeView(t, "dumb://home", false)
	c.applySystemViewStateOnCommit(ctx, "a", wv)
	assert.Equal(t, []string{"T2", "C1"}, wv.ran())
}

func TestNotifySystemViews_SkipsUncommittedLoadingPanes(t *testing.T) {
	t.Parallel()
	loaded := newFakeView(t, "dumb://home", false)
	loading := newFakeView(t, "dumb://home", true)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": loaded, "b": loading})
	ctx := context.Background()

	c.NotifySystemViews(ctx, "N1")
	assert.Equal(t, []string{"N1"}, loaded.ran())
	assert.Empty(t, loading.ran())

	// Not stored: the new document fetches fresh data on its own.
	c.applySystemViewStateOnCommit(ctx, "b", loading)
	c.applySystemViewStateOnFinish(ctx, "b", loading)
	assert.Empty(t, loading.ran())
}

// A committed page kept loading by a slow embed still gets notified, and
// again once it finishes.
func TestNotifySystemViews_CommittedLoadingPaneNotifiedNowAndOnFinish(t *testing.T) {
	t.Parallel()
	wv := newFakeView(t, "dumb://home", true)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()
	c.applySystemViewStateOnCommit(ctx, "a", wv)

	c.NotifySystemViews(ctx, "N1")
	c.NotifySystemViews(ctx, "N1")
	assert.Equal(t, []string{"N1", "N1"}, wv.ran())

	wv.setLoading(false)
	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Equal(t, []string{"N1", "N1", "N1"}, wv.ran(), "one replay per distinct script")

	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Len(t, wv.ran(), 3, "the replay is one-shot")
}

func TestNotifySystemViews_NewDocumentDropsQueuedReplay(t *testing.T) {
	t.Parallel()
	wv := newFakeView(t, "dumb://home", true)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()
	c.applySystemViewStateOnCommit(ctx, "a", wv)
	c.NotifySystemViews(ctx, "N1")

	c.applySystemViewStateOnCommit(ctx, "a", wv)
	c.applySystemViewStateOnFinish(ctx, "a", wv)
	assert.Equal(t, []string{"N1"}, wv.ran())
}

func TestNotifySystemViews_PaneLeftForWebSiteIsSkipped(t *testing.T) {
	t.Parallel()
	wv := newFakeView(t, "dumb://home", true)
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{"a": wv})
	ctx := context.Background()
	c.applySystemViewStateOnCommit(ctx, "a", wv)
	wv.mu.Lock()
	wv.uri = "https://example.com"
	wv.mu.Unlock()
	c.applySystemViewStateOnCommit(ctx, "a", wv)

	c.NotifySystemViews(ctx, "N1")
	assert.Empty(t, wv.ran())
	assert.False(t, c.isSystemViewCommitted("a"))
}

func TestClearPendingAppearance_DropsSystemViewPending(t *testing.T) {
	t.Parallel()
	wv := newFakeView(t, "dumb://home", true)
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
