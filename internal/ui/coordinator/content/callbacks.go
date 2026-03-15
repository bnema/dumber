package content

import (
	"context"
	"net/url"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gdk"
)

// internalSchemePath is the path used in dumb:// URIs (replaces webkit.HomePath).
const internalSchemePath = "home"

func shouldRenderCrashPage(reason port.WebProcessTerminationReason) bool {
	switch reason {
	case port.WebProcessTerminationCrashed, port.WebProcessTerminationExceededMemory:
		return true
	case port.WebProcessTerminationByAPI:
		return false
	default:
		return true
	}
}

func extractOriginalURIFromCrashPage(uri string) string {
	if uri == "" {
		return ""
	}

	parsed, err := url.Parse(uri)
	if err != nil {
		return uri
	}

	if parsed.Scheme != "dumb" || parsed.Host != internalSchemePath {
		return uri
	}

	if strings.Trim(parsed.Path, "/") != "crash" {
		return uri
	}

	original := strings.TrimSpace(parsed.Query().Get("url"))
	if original == "" {
		return ""
	}
	return original
}

func buildCrashPageURI(originalURI string) string {
	if strings.TrimSpace(originalURI) == "" {
		return crashPageURI
	}
	query := url.Values{}
	query.Set("url", originalURI)
	return crashPageURI + "?" + query.Encode()
}

// setupWebViewCallbacks configures standard callbacks and popup handling.
func (c *Coordinator) setupWebViewCallbacks(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	log := logging.FromContext(ctx)

	// Build port callbacks for standard events
	callbacks := &port.WebViewCallbacks{
		OnTitleChanged: func(title string) {
			c.onTitleChanged(ctx, paneID, title)
		},
		OnLoadChanged: func(event port.LoadEvent) {
			switch event {
			case port.LoadStarted:
				c.onLoadStarted(paneID)
			case port.LoadCommitted:
				c.onLoadCommitted(ctx, paneID, wv)
			case port.LoadFinished:
				c.onLoadFinished(ctx, paneID, wv)
			}
		},
		OnProgressChanged: func(progress float64) {
			c.onProgressChanged(paneID, progress)
		},
		OnURIChanged: func(uri string) {
			c.handleURIChanged(ctx, paneID, wv, uri)
		},
		OnLinkHover: func(uri string) {
			c.onLinkHover(paneID, uri)
		},
		OnWebProcessTerminated: func(reason port.WebProcessTerminationReason, reasonLabel string, uri string) {
			originalURI := extractOriginalURIFromCrashPage(uri)
			if !shouldRenderCrashPage(reason) {
				log.Info().
					Str("pane_id", string(paneID)).
					Str("reason", reasonLabel).
					Str("uri", uri).
					Msg("web process termination handled without crash page")
				return
			}

			crashURI := buildCrashPageURI(originalURI)
			log.Warn().
				Str("pane_id", string(paneID)).
				Str("reason", reasonLabel).
				Str("uri", uri).
				Str("crash_uri", crashURI).
				Msg("web process terminated, redirecting to crash page")

			if err := wv.LoadURI(ctx, crashURI); err != nil {
				log.Error().
					Err(err).
					Str("pane_id", string(paneID)).
					Str("reason", reasonLabel).
					Str("uri", uri).
					Str("crash_uri", crashURI).
					Msg("failed to load crash page after web process termination")
			}
		},
		OnPermissionRequest: func(origin string, permTypes []string, allow, deny func()) bool {
			return c.handlePermissionRequest(ctx, origin, permTypes, allow, deny)
		},
	}

	// Favicon changes - need webkit-specific Generation() to detect stale callbacks
	if wkWV, ok := wv.(*webkit.WebView); ok {
		faviconGen := wkWV.Generation()
		callbacks.OnFaviconChanged = func(favicon port.Texture) {
			if wkWV.Generation() != faviconGen {
				return // WebView was reused, ignore stale signal
			}
			if gdkTexture, ok := favicon.(*gdk.Texture); ok {
				c.onFaviconChanged(ctx, paneID, wkWV, gdkTexture)
			}
		}
	}

	// Add popup create handler if popup handling is configured
	callbacks.OnCreate = c.buildPopupCreateHandler(ctx, paneID, wv)

	wv.SetCallbacks(callbacks)

	// Webkit-specific callbacks not in port.WebViewCallbacks (middle-click, fullscreen, audio)
	if wkWV, ok := wv.(*webkit.WebView); ok {
		wkWV.OnLinkMiddleClick = func(uri string) bool {
			return c.handleLinkMiddleClick(ctx, paneID, uri)
		}
	}

	// Fullscreen handlers for idle inhibition
	c.setupIdleInhibitionHandlers(ctx, paneID, wv)
}

// handlePermissionRequest processes media permission requests from WebKit.
// It delegates to the permission use case which handles auto-allow, stored permissions, and dialogs.
func (c *Coordinator) handlePermissionRequest(
	ctx context.Context,
	origin string,
	permTypes []string,
	allow, deny func(),
) bool {
	log := logging.FromContext(ctx)

	// Convert string permission types to entity types
	entityTypes := make([]entity.PermissionType, 0, len(permTypes))
	for _, pt := range permTypes {
		switch pt {
		case "microphone":
			entityTypes = append(entityTypes, entity.PermissionTypeMicrophone)
		case "camera":
			entityTypes = append(entityTypes, entity.PermissionTypeCamera)
		case "display":
			entityTypes = append(entityTypes, entity.PermissionTypeDisplay)
		case "device_info":
			entityTypes = append(entityTypes, entity.PermissionTypeDeviceInfo)
		default:
			log.Warn().Str("type", pt).Msg("unknown permission type, skipping")
		}
	}

	if len(entityTypes) == 0 {
		log.Warn().Str("origin", origin).Msg("permission request with no valid types, denying")
		deny()
		return true
	}

	trackedTypes := filterWebRTCPermissionTypes(entityTypes)
	notifyActivity := func(state PermissionActivityState) {
		if c.onPermissionActivity == nil || len(trackedTypes) == 0 {
			return
		}
		c.onPermissionActivity(origin, trackedTypes, state)
	}

	notifyActivity(PermissionActivityRequesting)

	wrappedAllow := func() {
		notifyActivity(PermissionActivityAllowed)
		allow()
	}
	wrappedDeny := func() {
		notifyActivity(PermissionActivityBlocked)
		deny()
	}

	// Check if permission use case is available
	if c.permissionUC == nil {
		log.Warn().Str("origin", origin).Msg("no permission use case available, auto-allowing low-risk permissions")
		// Auto-allow display and device_info, deny others
		allAutoAllow := true
		for _, pt := range entityTypes {
			if !entity.IsAutoAllow(pt) {
				allAutoAllow = false
				break
			}
		}
		if allAutoAllow {
			wrappedAllow()
		} else {
			wrappedDeny()
		}
		return true
	}

	// Delegate to use case
	callback := usecase.PermissionCallback{
		Allow: wrappedAllow,
		Deny:  wrappedDeny,
	}

	c.permissionUC.HandlePermissionRequest(ctx, origin, entityTypes, callback)
	return true
}

func filterWebRTCPermissionTypes(types []entity.PermissionType) []entity.PermissionType {
	filtered := make([]entity.PermissionType, 0, len(types))
	for _, permType := range types {
		switch permType {
		case entity.PermissionTypeMicrophone, entity.PermissionTypeCamera, entity.PermissionTypeDisplay:
			filtered = append(filtered, permType)
		}
	}
	return filtered
}

// setupIdleInhibitionHandlers configures fullscreen and audio callbacks for idle inhibition.
// Idle is inhibited when:
// - The webview enters fullscreen mode (e.g., fullscreen video)
// - The webview is playing audio (e.g., video/music playback)
// The inhibitor uses refcounting, so both can be active simultaneously.
func (c *Coordinator) setupIdleInhibitionHandlers(ctx context.Context, paneID entity.PaneID, wv port.WebView) {
	log := logging.FromContext(ctx)

	if wv == nil {
		return
	}

	// Type-assert to access webkit-specific fullscreen/audio callbacks
	wkWV, ok := wv.(*webkit.WebView)
	if !ok {
		return
	}

	// Fullscreen handling
	wkWV.OnEnterFullscreen = func() bool {
		if c.idleInhibitor != nil {
			if err := c.idleInhibitor.Inhibit(ctx, "Fullscreen video playback"); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to inhibit idle")
			}
		}
		if c.onFullscreenChanged != nil {
			c.onFullscreenChanged(true)
		}
		return false // Allow fullscreen
	}

	wkWV.OnLeaveFullscreen = func() bool {
		if c.idleInhibitor != nil {
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle")
			}
		}
		if c.onFullscreenChanged != nil {
			c.onFullscreenChanged(false)
		}
		return false // Allow leaving fullscreen
	}

	// Audio playback handling
	wkWV.OnAudioStateChanged = func(playing bool) {
		if c.idleInhibitor == nil {
			return
		}
		if playing {
			if err := c.idleInhibitor.Inhibit(ctx, "Media playback"); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to inhibit idle for audio")
			}
		} else {
			if err := c.idleInhibitor.Uninhibit(ctx); err != nil {
				log.Warn().Err(err).Str("pane_id", string(paneID)).Msg("failed to uninhibit idle for audio")
			}
		}
	}
}
