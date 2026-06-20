package content

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestHandleURIChanged_BlocksExternalSchemeWithoutLaunching(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	wv.EXPECT().CanGoBack().Return(false).Once()

	launchCount := 0
	c := &Coordinator{
		onLaunchExternalURL: func(uri string) {
			launchCount++
		},
	}

	c.handleURIChanged(ctx, paneID, wv, "vscode://file/etc/passwd")

	assert.Equal(t, 0, launchCount)
}

func TestHandleURIChanged_BlocksExternalSchemeRedirectAndRestoresPreviousPage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	wv.EXPECT().CanGoBack().Return(true).Once()
	wv.EXPECT().GoBack(ctx).Return(nil).Once()

	launchCount := 0
	c := &Coordinator{
		onLaunchExternalURL: func(uri string) {
			launchCount++
		},
	}

	c.handleURIChanged(ctx, paneID, wv, "spotify://track/123")

	assert.Equal(t, 0, launchCount)
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
	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(errors.New("stop failed")).Once()
	wv.EXPECT().CanGoBack().Return(false).Once()

	launchCount := 0
	c := &Coordinator{
		onLaunchExternalURL: func(uri string) {
			launchCount++
		},
	}

	c.handleURIChanged(ctx, paneID, wv, "vscode://file/etc/passwd")

	assert.Equal(t, 0, launchCount)
}

func TestHandleURIChanged_GoBackErrorDoesNotPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paneID := entity.PaneID("pane-1")
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	wv.EXPECT().CanGoBack().Return(true).Once()
	wv.EXPECT().GoBack(ctx).Return(errors.New("goback failed")).Once()

	launchCount := 0
	c := &Coordinator{
		onLaunchExternalURL: func(uri string) {
			launchCount++
		},
	}

	c.handleURIChanged(ctx, paneID, wv, "spotify://track/123")

	assert.Equal(t, 0, launchCount)
}
