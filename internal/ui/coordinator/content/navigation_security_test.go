package content

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

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
	c := &Coordinator{}
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
	c := &Coordinator{}
	rec.install(c)

	c.handleURIChanged(ctx, "pane-1", wv, "spotify://track/123")
	rec.respond(false)

	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_DuplicateExternalURIPromptsOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Twice()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{}
	rec.install(c)

	uri := "claude://login/google-auth?code=abc"
	c.handleURIChanged(ctx, "pane-1", wv, uri)
	c.handleURIChanged(ctx, "pane-1", wv, uri)
	assert.Len(t, rec.prompts, 1)

	// Once answered, a later identical request may prompt again.
	rec.respond(false)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	c.handleURIChanged(ctx, "pane-1", wv, uri)
	assert.Len(t, rec.prompts, 2)
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
	c := &Coordinator{}
	rec.install(c)

	c.handleURIChanged(ctx, "pane-1", wv, "vscode://file/etc/passwd")

	assert.Len(t, rec.prompts, 1)
	assert.Empty(t, rec.launched)
}
