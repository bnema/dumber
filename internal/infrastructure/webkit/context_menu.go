package webkit

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/contextmenu"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

// contextMenuResolver implements port.ImageDataResolver by fetching image bytes
// over HTTP. This is the WebKit-specific seam for the shared context menu
// pipeline's copy-image and save-image actions.
type contextMenuResolver struct {
	client            *http.Client
	allowPrivateHosts bool
	lookupIPAddr      func(context.Context, string) ([]net.IPAddr, error)
	dialContext       func(context.Context, string, string) (net.Conn, error)
}

var defaultImageFetchClient = &http.Client{Timeout: 15 * time.Second}

const maxImageFetchBytes = 50 * 1024 * 1024

// ResolveImageData fetches raw image bytes from the given URI.
func (r *contextMenuResolver) ResolveImageData(ctx context.Context, imageURI string) (port.ImageData, error) {
	if imageURI == "" {
		return port.ImageData{}, fmt.Errorf("empty image URI")
	}

	parsed, parseErr := url.Parse(imageURI)
	if parseErr != nil {
		return port.ImageData{}, fmt.Errorf("parse image URI: %w", parseErr)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return port.ImageData{}, fmt.Errorf("fetch image: unsupported URI scheme %q", parsed.Scheme)
	}
	if err := r.validateImageURL(ctx, parsed); err != nil {
		return port.ImageData{}, err
	}

	client := r.imageFetchClient(ctx)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), http.NoBody)
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

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageFetchBytes+1))
	if err != nil {
		return port.ImageData{}, fmt.Errorf("read image body: %w", err)
	}
	if len(data) == 0 {
		return port.ImageData{}, fmt.Errorf("read image body: empty image data")
	}
	if len(data) > maxImageFetchBytes {
		return port.ImageData{}, fmt.Errorf("read image body: image too large (limit %d bytes)", maxImageFetchBytes)
	}
	detectedType := validatedImageMimeType(http.DetectContentType(data))
	mimeType := detectedType
	if mimeType == "" {
		mimeType = validatedImageMimeType(resp.Header.Get("Content-Type"))
	}
	if mimeType == "" {
		return port.ImageData{}, fmt.Errorf("read image body: content is not an image (%s)", detectedType)
	}

	return port.ImageData{Bytes: data, MimeType: mimeType}, nil
}

func (r *contextMenuResolver) imageFetchClient(_ context.Context) *http.Client {
	base := r.client
	if base == nil {
		base = defaultImageFetchClient
	}
	client := *base
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	if stdTransport, ok := transport.(*http.Transport); ok {
		cloned := stdTransport.Clone()
		cloned.DialContext = r.dialImageHostContext
		client.Transport = cloned
	} else {
		client.Transport = &validatedImageRoundTripper{
			base:      transport,
			validator: r.validateImageURL,
		}
	}
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		return r.validateImageURL(req.Context(), req.URL)
	}
	return &client
}

func (r *contextMenuResolver) validateImageURL(ctx context.Context, imageURL *url.URL) error {
	if imageURL == nil {
		return fmt.Errorf("fetch image: missing URL")
	}
	switch strings.ToLower(imageURL.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("fetch image: unsupported URI scheme %q", imageURL.Scheme)
	}
	if imageURL.Host == "" {
		return fmt.Errorf("fetch image: missing host")
	}
	hostname := imageURL.Hostname()
	if err := r.validateImageHost(ctx, hostname); err != nil {
		return err
	}
	return nil
}

func (r *contextMenuResolver) validateImageHost(ctx context.Context, host string) error {
	if host == "" {
		return fmt.Errorf("fetch image: missing host")
	}
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		if r.allowPrivateHosts {
			return nil
		}
		return fmt.Errorf("fetch image: private host %q not allowed", host)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		addrs, err := r.allowedImageHostAddrs(ctx, host)
		if err != nil {
			return err
		}
		if len(addrs) == 0 {
			return fmt.Errorf("fetch image: private host %q not allowed", host)
		}
		return nil
	}
	if isPrivateImageIP(ip) && !r.allowPrivateHosts {
		return fmt.Errorf("fetch image: private host %q not allowed", host)
	}
	return nil
}

func (r *contextMenuResolver) allowedImageHostAddrs(ctx context.Context, host string) ([]net.IPAddr, error) {
	lookup := r.lookupIPAddr
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("fetch image: resolve host %q: %w", host, err)
	}
	if r.allowPrivateHosts {
		return addrs, nil
	}
	allowed := make([]net.IPAddr, 0, len(addrs))
	for _, addr := range addrs {
		if !isPrivateImageIP(addr.IP) {
			allowed = append(allowed, addr)
		}
	}
	return allowed, nil
}

func (r *contextMenuResolver) dialImageHostContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, imagePort, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("fetch image: parse dial address %q: %w", addr, err)
	}
	if host == "" {
		return nil, fmt.Errorf("fetch image: missing host")
	}

	dial := r.dialContext
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateImageIP(ip) && !r.allowPrivateHosts {
			return nil, fmt.Errorf("fetch image: private host %q not allowed", host)
		}
		return dial(ctx, network, net.JoinHostPort(ip.String(), imagePort))
	}

	addrs, err := r.allowedImageHostAddrs(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("fetch image: private host %q not allowed", host)
	}

	var lastErr error
	for _, addr := range addrs {
		conn, dialErr := dial(ctx, network, net.JoinHostPort(addr.IP.String(), imagePort))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		return nil, fmt.Errorf("fetch image: dial host %q failed", host)
	}
	return nil, fmt.Errorf("fetch image: dial host %q: %w", host, lastErr)
}

type validatedImageRoundTripper struct {
	base      http.RoundTripper
	validator func(context.Context, *url.URL) error
}

func (t *validatedImageRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.base == nil {
		return nil, fmt.Errorf("fetch image: transport not available")
	}
	if err := t.validator(req.Context(), req.URL); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}

func isPrivateImageIP(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified())
}

func validatedImageMimeType(raw string) string {
	if raw == "" {
		return ""
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return ""
	}
	mediaType = strings.ToLower(mediaType)
	if !strings.HasPrefix(mediaType, "image/") {
		return ""
	}
	return mediaType
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
	builder         port.ContextMenuBuilder
	executorFactory port.ContextMenuActionExecutorFactory
	clipboard       port.Clipboard
	resolver        port.ImageDataResolver
	saver           port.ResolvedImageSaver
	renderer        *contextmenu.Renderer
}

func (p *contextMenuPipeline) newExecutor(wv *WebView) port.ContextMenuActionExecutor {
	if p == nil || p.executorFactory == nil {
		return nil
	}
	return p.executorFactory.NewContextMenuActionExecutor(
		p.clipboard,
		p.resolver,
		p.saver,
		&webkitMenuDelegator{wv: wv},
	)
}

// connectContextMenuSignal wires the WebKit "context-menu" signal to the
// shared context menu pipeline. When the user right-clicks, WebKit's native
// menu is suppressed and replaced by the shared GTK popover menu.
func (wv *WebView) connectContextMenuSignal(pipeline *contextMenuPipeline) {
	if pipeline == nil {
		return
	}
	wv.contextMenu = pipeline

	contextMenuCb := func(_ webkit.WebView, contextMenuPtr uintptr, hitTestPtr uintptr) bool {
		var contextMenu *webkit.ContextMenu
		if contextMenuPtr != 0 {
			contextMenu = webkit.ContextMenuNewFromInternalPtr(contextMenuPtr)
		}
		var hit *webkit.HitTestResult
		if hitTestPtr != 0 {
			hit = webkit.HitTestResultNewFromInternalPtr(hitTestPtr)
		}

		x, y := contextMenuPosition(contextMenu)

		menuContext := buildMenuContextFromHitTest(wv, hit, x, y)
		ctx := logging.WithContext(context.Background(), wv.logger)
		if pipeline.builder == nil || pipeline.renderer == nil {
			return false
		}
		items := pipeline.builder.Build(ctx, menuContext)
		if len(items) == 0 {
			return false
		}

		executor := pipeline.newExecutor(wv)
		if executor == nil {
			return false
		}

		// Get the widget for anchoring the popover.
		if wv.inner == nil {
			return false
		}
		anchor := &wv.inner.Widget

		pipeline.renderer.Show(
			items,
			anchor,
			int32(x),
			int32(y),
			func(item port.MenuItem) {
				wv.executeContextMenuAction(ctx, executor, item, menuContext)
			},
			nil, // onClose — no action needed
		)

		return true // suppress native WebKit context menu
	}

	sigID := wv.inner.ConnectContextMenu(&contextMenuCb)
	wv.signalIDs = append(wv.signalIDs, uintptr(sigID))
}

func contextMenuPosition(menu *webkit.ContextMenu) (int, int) {
	if menu == nil || menu.GoPointer() == 0 {
		return 0, 0
	}
	event := menu.GetEvent()
	if event == nil {
		return 0, 0
	}
	var x, y float64
	if !event.GetPosition(&x, &y) {
		return 0, 0
	}
	return int(x), int(y)
}

// executeContextMenuAction runs the selected menu action through the shared
// executor. GTK-thread-sensitive actions run inline; image fetch/save work
// stays off-thread.
func (wv *WebView) executeContextMenuAction(
	ctx context.Context,
	executor port.ContextMenuActionExecutor,
	item port.MenuItem,
	menuContext port.MenuContext,
) {
	if executor == nil {
		return
	}

	if needsBackgroundContextMenuAction(item.Action) {
		go wv.runContextMenuAction(ctx, executor, item.Action, menuContext)
		return
	}
	wv.runContextMenuAction(ctx, executor, item.Action, menuContext)
}

func (wv *WebView) runContextMenuAction(
	ctx context.Context,
	executor port.ContextMenuActionExecutor,
	action port.MenuAction,
	menuContext port.MenuContext,
) {
	if err := executor.ExecuteMenuAction(ctx, action, menuContext); err != nil {
		wv.logger.Warn().Err(err).
			Str("action", string(action)).
			Msg("context menu action failed")
	}
}

func needsBackgroundContextMenuAction(action port.MenuAction) bool {
	switch action {
	case port.MenuActionCopyImage, port.MenuActionSaveImage:
		return true
	default:
		return false
	}
}

type webkitMenuDelegator struct {
	wv *WebView
}

var _ port.MenuActionDelegator = (*webkitMenuDelegator)(nil)

func (d *webkitMenuDelegator) DelegateMenuAction(ctx context.Context, action port.MenuAction, menuContext port.MenuContext) error {
	if d == nil || d.wv == nil {
		return fmt.Errorf("webkit menu delegator: webview not available")
	}
	switch action {
	case port.MenuActionBack:
		return d.wv.GoBack(ctx)
	case port.MenuActionForward:
		return d.wv.GoForward(ctx)
	case port.MenuActionReload:
		return d.wv.Reload(ctx)
	case port.MenuActionOpenLinkNewTab:
		if menuContext.LinkURI == "" {
			return fmt.Errorf("open link in new tab: link URI not available")
		}
		if d.wv.OnLinkMiddleClick == nil {
			return fmt.Errorf("open link in new tab: middle-click handler not available")
		}
		if !d.wv.OnLinkMiddleClick(menuContext.LinkURI) {
			return fmt.Errorf("open link in new tab: action not handled")
		}
		return nil
	case port.MenuActionInspectElement:
		return d.wv.ShowDevTools()
	case port.MenuActionCopySelection:
		if !menuContext.HasSelection {
			return fmt.Errorf("copy selection: selection not available")
		}
		d.wv.RunJavaScript(ctx, "document.execCommand('copy');")
		return nil
	default:
		return fmt.Errorf("webkit menu delegator: unsupported action %s", action)
	}
}

// NewContextMenuResolver creates a new ImageDataResolver for WebKit
// that fetches image bytes over HTTP.
func NewContextMenuResolver() port.ImageDataResolver {
	return &contextMenuResolver{}
}
