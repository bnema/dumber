package content

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

type externalLaunchRecorder struct {
	prompts  []string
	origins  []string
	respond  func(bool)
	launched []string
}

const testPaneID = entity.PaneID("pane-1")

// uriChanged delivers a URI change to testPaneID as the engine callback
// installed for wv.
func uriChanged(ctx context.Context, c *Coordinator, wv port.WebView, uri string) {
	identity, _ := identityForWebView(wv)
	c.handleURIChanged(ctx, testPaneID, wv, identity, uri)
}

// externalWebView returns a WebView mock with ID 1 and the given generation.
func externalWebView(t *testing.T, generation uint64) *mocks.MockWebView {
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Maybe()
	wv.EXPECT().Generation().Return(generation).Maybe()
	return wv
}

func (r *externalLaunchRecorder) install(c *Coordinator) {
	c.onLaunchExternalURL = func(uri string) {
		r.launched = append(r.launched, uri)
	}
	c.onConfirmExternalLaunch = func(_ entity.PaneID, origin, uri string, respond func(bool)) {
		r.prompts = append(r.prompts, uri)
		r.origins = append(r.origins, origin)
		r.respond = respond
	}
}

func TestHandleURIChanged_ExternalSchemeWithoutConfirmerIsBlocked(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	launchCount := 0
	c := &Coordinator{
		webViews:            map[entity.PaneID]port.WebView{"pane-1": wv},
		onLaunchExternalURL: func(string) { launchCount++ },
	}

	uriChanged(ctx, c, wv, "vscode://file/etc/passwd")

	assert.Equal(t, 0, launchCount)
}

func TestHandleURIChanged_ExternalSchemeLaunchesOnlyAfterConfirmation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uri := "claude://login/google-auth?code=abc"
	uriChanged(ctx, c, wv, uri)

	assert.Equal(t, []string{uri}, rec.prompts)
	assert.Empty(t, rec.launched)

	rec.respond(true)
	assert.Equal(t, []string{uri}, rec.launched)
}

func TestHandleURIChanged_ExternalSchemeDeniedDoesNotLaunchOrGoBack(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	// GoBack must not be called: going back to an OAuth endpoint loops.
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uriChanged(ctx, c, wv, "spotify://track/123")
	rec.respond(false)

	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_OneOutstandingPromptPerPane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(nil).Times(3)

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	// Alternating URIs must not queue more prompts while one is pending.
	uriChanged(ctx, c, wv, "claude://a")
	uriChanged(ctx, c, wv, "claude://b")
	uriChanged(ctx, c, wv, "claude://a")
	assert.Equal(t, []string{"claude://a"}, rec.prompts)

	// Once answered, the pane may prompt again.
	rec.respond(false)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	uriChanged(ctx, c, wv, "claude://b")
	assert.Len(t, rec.prompts, 2)
}

func TestHandleURIChanged_StalePromptAfterReleaseDoesNotLaunch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uriChanged(ctx, c, wv, "claude://login?code=x")
	// Pane closed while the prompt was queued.
	c.clearExternalPrompt("pane-1")
	delete(c.webViews, "pane-1")

	rec.respond(true)
	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_HTTPURIStillRecordsSPANavigation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	paneID := testPaneID
	wv := externalWebView(t, 1)
	wv.EXPECT().IsLoading().Return(false).Once()

	var recordedPane entity.PaneID
	var recordedURI string
	c := &Coordinator{
		webViews: map[entity.PaneID]port.WebView{paneID: wv},
		onPaneURIUpdated: func(paneID entity.PaneID, uri string) {
			recordedPane = paneID
			recordedURI = uri
		},
	}

	uriChanged(ctx, c, wv, "https://example.com/app#state")

	assert.Equal(t, paneID, recordedPane)
	assert.Equal(t, "https://example.com/app#state", recordedURI)
}

func TestHandleURIChanged_StopErrorDoesNotPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(errors.New("stop failed")).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uriChanged(ctx, c, wv, "vscode://file/etc/passwd")

	assert.Len(t, rec.prompts, 1)
	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_OldAnswerCannotLaunchReplacementPrompt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(nil).Twice()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uriChanged(ctx, c, wv, "claude://old")
	oldRespond := rec.respond
	// The pane is invalidated, then the same WebView asks again.
	c.clearExternalPrompt("pane-1")
	uriChanged(ctx, c, wv, "claude://new")
	newRespond := rec.respond

	oldRespond(true)
	assert.Empty(t, rec.launched, "old answer must not launch")

	newRespond(true)
	assert.Equal(t, []string{"claude://new"}, rec.launched)
}

func TestHandleURIChanged_RebindToNewGenerationMakesPromptStale(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().Stop(ctx).Return(nil).Once()
	rebound := externalWebView(t, 2)

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uriChanged(ctx, c, wv, "claude://login?code=x")
	c.webViews["pane-1"] = rebound

	rec.respond(true)
	assert.Empty(t, rec.launched)
}

func TestHandleURIChanged_PromptNamesLastWebOrigin(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)
	wv.EXPECT().IsLoading().Return(true).Once()
	wv.EXPECT().Stop(ctx).Return(nil).Once()

	rec := &externalLaunchRecorder{}
	c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
	rec.install(c)

	uriChanged(ctx, c, wv, "https://claude.ai/login/popup-google-auth?code=x")
	uriChanged(ctx, c, wv, "claude://login")

	assert.Equal(t, []string{"claude.ai"}, rec.origins)
}

func TestHandleURIChanged_IgnoresCallbackFromReleasedWebView(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	wv := externalWebView(t, 1)

	rec := &externalLaunchRecorder{}
	c := &Coordinator{}
	rec.install(c)

	// The callback was queued before the pane released its WebView.
	uriChanged(ctx, c, wv, "https://site.example/")
	uriChanged(ctx, c, wv, "claude://login")

	assert.Empty(t, rec.prompts)
	assert.Empty(t, c.externalPromptOrigins)
}

func TestHandleURIChanged_NonHTTPDocumentClearsOrigin(t *testing.T) {
	t.Parallel()

	for _, doc := range []string{"data:text/html,hi", "file://trusted.example/page.html"} {
		ctx := context.Background()
		wv := externalWebView(t, 1)
		wv.EXPECT().IsLoading().Return(true).Times(2)
		wv.EXPECT().Stop(ctx).Return(nil).Once()

		rec := &externalLaunchRecorder{}
		c := &Coordinator{webViews: map[entity.PaneID]port.WebView{"pane-1": wv}}
		rec.install(c)

		uriChanged(ctx, c, wv, "https://trusted.example/")
		uriChanged(ctx, c, wv, doc)
		uriChanged(ctx, c, wv, "claude://login")

		assert.Equal(t, []string{""}, rec.origins, doc)
	}
}

func TestURIChangedCallback_IgnoredAfterSameWebViewIsReusedFromPool(t *testing.T) {
	ctx := context.Background()
	c := newMinimalCoordinator()
	rec := &externalLaunchRecorder{}
	rec.install(c)
	spaCalls := 0
	c.onPaneURIUpdated = func(entity.PaneID, string) { spaCalls++ }

	generation := uint64(1)
	wv := mocks.NewMockWebView(t)
	wv.EXPECT().ID().Return(port.WebViewID(1)).Maybe()
	wv.EXPECT().Generation().RunAndReturn(func() uint64 { return generation }).Maybe()
	var callbacks *port.WebViewCallbacks
	wv.EXPECT().SetCallbacks(mock.Anything).Run(func(cb *port.WebViewCallbacks) { callbacks = cb }).Once()

	c.setWebViewLocked(testPaneID, wv)
	c.setupWebViewCallbacks(ctx, testPaneID, wv)
	require.NotNil(t, callbacks)

	// The same object is reused from the pool for a new binding.
	generation = 2
	callbacks.OnURIChanged("https://site.example/")
	callbacks.OnURIChanged("claude://login")

	assert.Empty(t, rec.prompts)
	assert.Empty(t, c.externalPromptOrigins)
	assert.Zero(t, spaCalls)
}
