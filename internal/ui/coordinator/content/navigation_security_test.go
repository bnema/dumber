package content

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

type externalLaunchRecorder struct {
	prompts  []string
	respond  func(bool)
	launched []string
}

func (r *externalLaunchRecorder) install(c *Coordinator) {
	c.onLaunchExternalURL = func(uri string) {
		r.launched = append(r.launched, uri)
	}
	c.onConfirmExternalLaunch = func(_ entity.PaneID, uri string, respond func(bool)) {
		r.prompts = append(r.prompts, uri)
		r.respond = respond
	}
}

func TestHandleURIChanged_ExternalSchemeWithoutConfirmerIsBlocked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	launchCount := 0
	c := &Coordinator{
		onLaunchExternalURL: func(string) { launchCount++ },
	}

	c.handleURIChanged(ctx, "pane-1", wv, "vscode://file/etc/passwd")

	assert.Equal(t, 0, launchCount)
}

func TestHandleURIChanged_ExternalSchemeLaunchesOnlyAfterConfirmation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uri := "claude://login/google-auth?code=abc"
	c.handleURIChanged(ctx, "pane-1", wv, uri)

	assert.Equal(t, []string{uri}, rec.prompts)
	assert.Empty(t, rec.launched)

	rec.respond(true)
	assert.Equal(t, []string{uri}, rec.launched)
}

func TestHandleURIChanged_ExternalSchemeDeniedDoesNotLaunchOrGoBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	// GoBack must not be called: going back to an OAuth endpoint loops.
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	c.handleURIChanged(ctx, "pane-1", wv, "spotify://track/123")
	rec.respond(false)

	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_OneOutstandingPromptPerPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Times(3)

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	// Alternating URIs must not queue more prompts while one is pending.
	c.handleURIChanged(ctx, "pane-1", wv, "claude://a")
	c.handleURIChanged(ctx, "pane-1", wv, "claude://b")
	c.handleURIChanged(ctx, "pane-1", wv, "claude://a")
	assert.Equal(t, []string{"claude://a"}, rec.prompts)

	// Once answered, the pane may prompt again.
	rec.respond(false)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	c.handleURIChanged(ctx, "pane-1", wv, "claude://b")
	assert.Len(t, rec.prompts, 2)
}

func TestHandleURIChanged_StalePromptAfterReleaseDoesNotLaunch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	c.handleURIChanged(ctx, "pane-1", wv, "claude://login?code=x")
	// Pane closed while the prompt was queued.
	c.clearExternalPrompt("pane-1")
	delete(c.webViews, "pane-1")

	rec.respond(true)
	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_HTTPURIStillRecordsSPANavigation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().IsLoading().Return(false).Once()

	var recordedPane entity.PaneID
	var recordedURI string
	c := &Coordinator{
		onPaneURIUpdated: func(paneID entity.PaneID, uri string) {
			recordedPane = paneID
			recordedURI = uri
		},
	}

	c.handleURIChanged(ctx, paneID, wv, "https://example.com/app#state")

	assert.Equal(t, paneID, recordedPane)
	assert.Equal(t, "https://example.com/app#state", recordedURI)
}

func TestHandleURIChanged_StopErrorDoesNotPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(errors.New("stop failed")).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	c.handleURIChanged(ctx, "pane-1", wv, "vscode://file/etc/passwd")

	assert.Len(t, rec.prompts, 1)
	assert.Empty(t, rec.launched)
}
