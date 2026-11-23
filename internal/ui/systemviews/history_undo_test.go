package systemviews

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// toastDOM records toasts on top of region updates.
type toastDOM struct {
	regionDOM
	toasts []ToastRequest
}

func (d *toastDOM) ShowToast(request ToastRequest) bool {
	d.toasts = append(d.toasts, request)
	return true
}

func (d *toastDOM) lastTimeline(t *testing.T) string {
	t.Helper()
	require.NotEmpty(t, d.regions)
	for _, region := range d.regions[len(d.regions)-1] {
		if region.Name == historyRegionTimeline {
			return region.HTML
		}
	}
	t.Fatal("timeline region missing")
	return ""
}

func undoFixture(t *testing.T) (*App, *toastDOM, *historyStore) {
	t.Helper()
	now := time.Now()
	dom := &toastDOM{}
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{
		{ID: 3, URL: "https://c.example", Title: "Third", LastVisited: now},
		{ID: 2, URL: "https://b.example", Title: "Second", LastVisited: now.Add(-time.Minute)},
		{ID: 1, URL: "https://a.example", Title: "First", LastVisited: now.Add(-2 * time.Minute)},
	}})
	return loadedHistoryApp(t, dom, history), dom, history
}

func deferredDelete(id string) DOMAction {
	return DOMAction{Action: historyActionDeleteEntry, Data: map[string]string{"id": id, undoDataKey: "true"}}
}

func TestUndoableDeleteHidesEntryAndWaitsForToast(t *testing.T) {
	t.Parallel()
	app, dom, history := undoFixture(t)

	require.NoError(t, app.HandleDOMAction(context.Background(), deferredDelete("2")))

	assert.Empty(t, history.deletedEntryIDs(), "nothing is deleted before the toast settles")
	assert.NotContains(t, dom.lastTimeline(t), "Second")
	require.Len(t, dom.toasts, 1)
	toast := dom.toasts[0]
	assert.Equal(t, "Deleted “Second”", toast.Message)
	assert.Equal(t, "Undo", toast.ActionLabel)
	assert.Equal(t, historyUndoTimeout, toast.Timeout)
	assert.Equal(t, historyActionUndoDelete, toast.OnAction.Action)
	assert.Equal(t, historyActionCommitDelete, toast.OnDismiss.Action)
}

func TestUndoableDeleteCommitsWhenToastDismissed(t *testing.T) {
	t.Parallel()
	app, dom, history := undoFixture(t)
	history.expectDeleteEntry(2)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))
	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[0].OnDismiss))

	assert.Equal(t, []int64{2}, history.deletedEntryIDs())
	assert.Empty(t, app.pendingHistoryDeletes)
}

func TestUndoRestoresEntryInPlaceAndCancelsDelete(t *testing.T) {
	t.Parallel()
	app, dom, history := undoFixture(t)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))
	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[0].OnAction))
	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[0].OnDismiss), "a late dismiss after undo is a no-op")

	assert.Empty(t, history.deletedEntryIDs())
	timeline := dom.lastTimeline(t)
	third, second, first := strings.Index(timeline, "Third"), strings.Index(timeline, "Second"), strings.Index(timeline, "First")
	assert.True(t, third < second && second < first, "undone entry returns to its time-ordered slot")
}

func TestPendingDeleteStaysHiddenAcrossReloads(t *testing.T) {
	t.Parallel()
	app, dom, _ := undoFixture(t)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))
	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: historyActionSearchLive, Data: map[string]string{"query": "example"}}))
	assert.NotContains(t, dom.lastTimeline(t), "Second", "search results hide pending deletes")

	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: historyActionClear}))
	assert.NotContains(t, dom.lastTimeline(t), "Second", "timeline reloads hide pending deletes")
}

func TestFlushCommitsEveryPendingDelete(t *testing.T) {
	t.Parallel()
	app, _, history := undoFixture(t)
	history.expectDeleteEntry(2, 3)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))
	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("3")))
	require.NoError(t, app.HandleDOMAction(ctx, DOMAction{Action: actionFlushDeletes}))

	assert.ElementsMatch(t, []int64{2, 3}, history.deletedEntryIDs())
	assert.Empty(t, app.pendingHistoryDeletes)
}

func TestUndoableDeleteWithoutToastDeletesImmediately(t *testing.T) {
	t.Parallel()
	dom := &regionDOM{}
	history := newHistoryStore(t, &historyStore{entries: []*entity.HistoryEntry{{ID: 2, URL: "https://b.example", Title: "Second"}}})
	history.expectDeleteEntry(2)
	app := loadedHistoryApp(t, dom, history)

	require.NoError(t, app.HandleDOMAction(context.Background(), deferredDelete("2")))

	assert.Equal(t, []int64{2}, history.deletedEntryIDs())
	assert.Empty(t, app.pendingHistoryDeletes)
}

func TestFailedCommitRestoresEntryAndShowsError(t *testing.T) {
	t.Parallel()
	app, dom, history := undoFixture(t)
	history.deleteErr = errors.New("db locked")
	history.expectDeleteEntry(2)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))
	require.NoError(t, app.HandleDOMAction(ctx, dom.toasts[0].OnDismiss))

	assert.Contains(t, dom.lastTimeline(t), "Second", "a failed delete shows the entry again")
	assert.Contains(t, app.historyError, "db locked")
	assert.Contains(t, app.renderedHTML, "Could not delete history entry")
}

func TestRepeatedDeferredDeleteIsIgnored(t *testing.T) {
	t.Parallel()
	app, dom, _ := undoFixture(t)
	ctx := context.Background()

	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))
	require.NoError(t, app.HandleDOMAction(ctx, deferredDelete("2")))

	assert.Len(t, dom.toasts, 1, "a pending id gets no second toast")
	assert.Equal(t, "Second", app.pendingHistoryDeletes[2].Title)
}

func TestCommitDeleteRejectsInvalidID(t *testing.T) {
	t.Parallel()
	app, _, _ := undoFixture(t)

	err := app.HandleDOMAction(context.Background(), DOMAction{Action: historyActionCommitDelete, Data: map[string]string{"id": "x"}})
	assert.Error(t, err)
}
