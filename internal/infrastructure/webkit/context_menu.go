package webkit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/contextmenu"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// contextMenuResolver implements port.ImageDataResolver by fetching image bytes
// over HTTP. This is the WebKit-specific seam for the shared context menu
// pipeline's copy-image and save-image actions.
type contextMenuResolver struct {
	client *http.Client
}

var defaultImageFetchClient = &http.Client{Timeout: 15 * time.Second}

const maxImageFetchBytes = 50 * 1024 * 1024

// ResolveImageData fetches raw image bytes from the given URI.
func (r *contextMenuResolver) ResolveImageData(ctx context.Context, imageURI string) (port.ImageData, error) {
	if imageURI == "" {
		return port.ImageData{}, fmt.Errorf("empty image URI")
	}

	client := r.client
	if client == nil {
		client = defaultImageFetchClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURI, http.NoBody)
	if err != nil {
		return port.ImageData{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return port.ImageData{}, fmt.Errorf("fetch image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return port.ImageData{}, fmt.Errorf("fetch image: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageFetchBytes))
	if err != nil {
		return port.ImageData{}, fmt.Errorf("read image body: %w", err)
	}
	if len(data) == 0 {
		return port.ImageData{}, fmt.Errorf("read image body: empty image data")
	}

	return port.ImageData{Bytes: data}, nil
}

// Compile-time check that contextMenuResolver implements port.ImageDataResolver.
var _ port.ImageDataResolver = (*contextMenuResolver)(nil)

// buildMenuContextFromHitTest maps a WebKit HitTestResult into a port.MenuContext.
func buildMenuContextFromHitTest(wv *WebView, hit *webkit.HitTestResult, x, y int) port.MenuContext {
	ctx := port.MenuContext{
		X: x,
		Y: y,
	}

	if wv != nil {
		ctx.PageURI = wv.URI()
		ctx.CanGoBack = wv.CanGoBack()
		ctx.CanGoForward = wv.CanGoForward()
	}

	if hit == nil {
		return ctx
	}

	if hit.ContextIsLink() {
		ctx.LinkURI = hit.GetLinkUri()
	}
	if hit.ContextIsImage() {
		ctx.ImageURI = hit.GetImageUri()
	}
	ctx.HasSelection = hit.ContextIsSelection()
	ctx.IsEditable = hit.ContextIsEditable()

	return ctx
}

// contextMenuPipeline holds the dependencies needed to run the shared
// context menu flow from inside a WebKit context-menu signal handler.
type contextMenuPipeline struct {
	buildUC   *usecase.BuildContextMenuUseCase
	executeUC *usecase.ExecuteContextMenuActionUseCase
	renderer  *contextmenu.Renderer
}

// connectContextMenuSignal wires the WebKit "context-menu" signal to the
// shared context menu pipeline. When the user right-clicks, WebKit's native
// menu is suppressed and replaced by the shared GTK popover menu.
func (wv *WebView) connectContextMenuSignal(pipeline *contextMenuPipeline) {
	if pipeline == nil {
		return
	}

	contextMenuCb := func(_ webkit.WebView, _ uintptr, hitTestPtr uintptr) bool { //nolint:unparam // always true: suppress native menu
		hit := webkit.HitTestResultNewFromInternalPtr(hitTestPtr)

		// Determine click coordinates from the hit test result.
		// WebKit does not expose x/y directly on the context-menu signal;
		// we fall back to 0,0 if the hit test lacks positional data.
		// The renderer uses these as popover anchor coordinates.
		var x, y int

		menuContext := buildMenuContextFromHitTest(wv, hit, x, y)
		items := pipeline.buildUC.Build(context.Background(), menuContext)
		if len(items) == 0 {
			return true // suppress native menu, nothing to show
		}

		// Get the widget for anchoring the popover.
		if wv.inner == nil {
			return true
		}
		anchor := &wv.inner.Widget

		pipeline.renderer.Show(
			items,
			anchor,
			int32(x),
			int32(y),
			func(item port.MenuItem) {
				wv.executeContextMenuAction(pipeline.executeUC, item, menuContext)
			},
			nil, // onClose — no action needed
		)

		return true // suppress native WebKit context menu
	}

	sigID := wv.inner.ConnectContextMenu(&contextMenuCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

// executeContextMenuAction runs the selected menu action through the shared
// execute use case. Runs in a goroutine to avoid blocking the GTK main thread.
func (wv *WebView) executeContextMenuAction(
	executeUC *usecase.ExecuteContextMenuActionUseCase,
	item port.MenuItem,
	menuContext port.MenuContext,
) {
	if executeUC == nil {
		return
	}

	go func() {
		ctx := logging.WithContext(context.Background(), wv.logger)
		input := usecase.ExecuteContextMenuActionInput{
			Action:  item.Action,
			Context: menuContext,
		}
		if err := executeUC.Execute(ctx, input); err != nil {
			wv.logger.Warn().Err(err).
				Str("action", string(item.Action)).
				Msg("context menu action failed")
		}
	}()
}

// SetupContextMenu configures the context menu pipeline for this WebView.
// Called during WebView setup when the shared dependencies are available.
func (wv *WebView) SetupContextMenu(
	buildUC *usecase.BuildContextMenuUseCase,
	executeUC *usecase.ExecuteContextMenuActionUseCase,
	renderer *contextmenu.Renderer,
) {
	if wv == nil || wv.inner == nil {
		return
	}
	pipeline := &contextMenuPipeline{
		buildUC:   buildUC,
		executeUC: executeUC,
		renderer:  renderer,
	}
	wv.contextMenu = pipeline
	wv.connectContextMenuSignal(pipeline)
}

// NewContextMenuResolver creates a new ImageDataResolver for WebKit
// that fetches image bytes over HTTP.
func NewContextMenuResolver() port.ImageDataResolver {
	return &contextMenuResolver{}
}
