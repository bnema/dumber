package cef

import (
	"context"
	"sync"
	"sync/atomic"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/domain/entity"
	urlutil "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
)

// Mask of media access bits Dumber knows how to map to a permission type.
const supportedMediaAccessMask = uint32(
	purecef.MediaAccessPermissionTypesMediaPermissionDeviceAudioCapture |
		purecef.MediaAccessPermissionTypesMediaPermissionDeviceVideoCapture |
		purecef.MediaAccessPermissionTypesMediaPermissionDesktopAudioCapture |
		purecef.MediaAccessPermissionTypesMediaPermissionDesktopVideoCapture)

// mediaPermissionTypes maps a CEF media access bitmask to Dumber permission
// types. The returned allowedMask contains only the requested bits that Dumber
// understands; it is what must be granted when the user allows the request.
// An empty request or one containing unknown bits yields no types (deny).
func mediaPermissionTypes(requested uint32) (types []string, allowedMask uint32) {
	if requested == 0 || requested&^supportedMediaAccessMask != 0 {
		return nil, 0
	}
	allowedMask = requested
	if allowedMask&uint32(purecef.MediaAccessPermissionTypesMediaPermissionDeviceAudioCapture) != 0 {
		types = append(types, string(entity.PermissionTypeMicrophone))
	}
	if allowedMask&uint32(purecef.MediaAccessPermissionTypesMediaPermissionDeviceVideoCapture) != 0 {
		types = append(types, string(entity.PermissionTypeCamera))
	}
	const desktop = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDesktopAudioCapture |
		purecef.MediaAccessPermissionTypesMediaPermissionDesktopVideoCapture)
	if allowedMask&desktop != 0 {
		types = append(types, string(entity.PermissionTypeDisplay))
	}
	return types, allowedMask
}

// promptPermissionTypes maps a CEF permission prompt bitmask to Dumber
// permission types. ok is false when the mask is empty or contains any bit that
// Dumber does not support, in which case the whole request must be denied.
func promptPermissionTypes(requested uint32) (types []string, ok bool) {
	if requested == 0 {
		return nil, false
	}
	const (
		camera = uint32(purecef.PermissionRequestTypesPermissionTypeCameraStream |
			purecef.PermissionRequestTypesPermissionTypeCameraPanTiltZoom)
		mic           = uint32(purecef.PermissionRequestTypesPermissionTypeMicStream)
		geolocation   = uint32(purecef.PermissionRequestTypesPermissionTypeGeolocation)
		notifications = uint32(purecef.PermissionRequestTypesPermissionTypeNotifications)
		supported     = camera | mic | geolocation | notifications
	)
	if requested&^supported != 0 {
		return nil, false
	}
	if requested&mic != 0 {
		types = append(types, string(entity.PermissionTypeMicrophone))
	}
	if requested&camera != 0 {
		types = append(types, string(entity.PermissionTypeCamera))
	}
	if requested&geolocation != 0 {
		types = append(types, string(entity.PermissionTypeGeolocation))
	}
	if requested&notifications != 0 {
		types = append(types, string(entity.PermissionTypeNotification))
	}
	return types, true
}

// cefPermissionRequest guarantees that a CEF permission callback is resolved
// exactly once, whichever of allow, deny, dismissal or browser destruction
// happens first.
type cefPermissionRequest struct {
	done    atomic.Bool
	finish  func(allow bool)
	untrack func()
}

// resolve completes the request with the given decision. Only the first call
// (of resolve and abandon combined) has any effect.
func (r *cefPermissionRequest) resolve(allow bool) {
	if !r.done.CompareAndSwap(false, true) {
		return
	}
	if r.untrack != nil {
		r.untrack()
	}
	r.finish(allow)
}

// abandon marks the request as finished without touching the CEF callback.
// It is used when CEF has already invalidated the callback (prompt dismissed).
func (r *cefPermissionRequest) abandon() bool {
	if !r.done.CompareAndSwap(false, true) {
		return false
	}
	if r.untrack != nil {
		r.untrack()
	}
	return true
}

func (r *cefPermissionRequest) isDone() bool { return r.done.Load() }

type permissionKey struct {
	prompt bool
	id     uint64
}

// permissionTracker holds the unresolved requests of one WebView. The zero
// value is ready to use.
type permissionTracker struct {
	mu      sync.Mutex
	nextID  uint64
	pending map[permissionKey]*cefPermissionRequest
}

func (t *permissionTracker) nextMediaKey() permissionKey {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextID++
	return permissionKey{id: t.nextID}
}

// add registers req under key and returns any request previously registered
// under the same key.
func (t *permissionTracker) add(key permissionKey, req *cefPermissionRequest) *cefPermissionRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending == nil {
		t.pending = make(map[permissionKey]*cefPermissionRequest)
	}
	prev := t.pending[key]
	t.pending[key] = req
	req.untrack = func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.pending[key] == req {
			delete(t.pending, key)
		}
	}
	return prev
}

func (t *permissionTracker) get(key permissionKey) *cefPermissionRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pending[key]
}

func (t *permissionTracker) drain() []*cefPermissionRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	reqs := make([]*cefPermissionRequest, 0, len(t.pending))
	for _, req := range t.pending {
		reqs = append(reqs, req)
	}
	return reqs
}

// OnRequestMediaAccessPermission handles getUserMedia/getDisplayMedia requests.
func (h *handlerSet) OnRequestMediaAccessPermission(
	_ purecef.Browser,
	_ purecef.Frame,
	requestingOrigin string,
	requestedPermissions uint32,
	callback purecef.MediaAccessCallback,
) int32 {
	if h == nil || h.wv == nil || callback == nil {
		return 0
	}
	types, allowedMask := mediaPermissionTypes(requestedPermissions)
	h.beginPermissionRequest(h.permissions.nextMediaKey(), requestingOrigin, types, func(allow bool) {
		if allow && allowedMask != 0 {
			callback.Cont(allowedMask)
			return
		}
		callback.Cancel()
	})
	return 1
}

// OnShowPermissionPrompt handles non-media permission requests
// (geolocation, notifications, ...).
func (h *handlerSet) OnShowPermissionPrompt(
	_ purecef.Browser,
	promptID uint64,
	requestingOrigin string,
	requestedPermissions uint32,
	callback purecef.PermissionPromptCallback,
) int32 {
	if h == nil || h.wv == nil || callback == nil {
		return 0
	}
	types, _ := promptPermissionTypes(requestedPermissions)
	h.beginPermissionRequest(permissionKey{prompt: true, id: promptID}, requestingOrigin, types, func(allow bool) {
		if allow {
			callback.Cont(purecef.PermissionRequestResultPermissionResultAccept)
			return
		}
		callback.Cont(purecef.PermissionRequestResultPermissionResultDeny)
	})
	return 1
}

// OnDismissPermissionPrompt is called when CEF dismisses a prompt we handled.
// The callback is no longer valid, so it must not be invoked anymore.
func (h *handlerSet) OnDismissPermissionPrompt(
	_ purecef.Browser,
	promptID uint64,
	result purecef.PermissionRequestResult,
) {
	if h == nil || h.wv == nil {
		return
	}
	req := h.permissions.get(permissionKey{prompt: true, id: promptID})
	if req == nil || !req.abandon() {
		return
	}
	logCEFPermission(h.wv).Debug().
		Uint64("prompt_id", promptID).
		Int32("result", result).
		Msg("cef: permission prompt dismissed")
}

// denyPendingPermissions resolves every unresolved request as denied. It is
// called when the browser is closing so nothing is left hanging.
func (h *handlerSet) denyPendingPermissions() {
	if h == nil {
		return
	}
	for _, req := range h.permissions.drain() {
		req.resolve(false)
	}
}

// beginPermissionRequest registers a request, forwards it to the WebView's
// OnPermissionRequest callback on the GTK thread, and guarantees finish is
// invoked exactly once on the CEF UI thread. It denies when the request cannot
// be mapped, the origin is invalid, no callback is available, or the WebView
// is destroyed.
func (h *handlerSet) beginPermissionRequest(
	key permissionKey,
	requestingOrigin string,
	types []string,
	finish func(allow bool),
) {
	wv := h.wv
	req := &cefPermissionRequest{
		finish: func(allow bool) {
			dispatchPermissionResult(wv, func() { finish(allow) })
		},
	}
	if prev := h.permissions.add(key, req); prev != nil {
		prev.resolve(false)
	}

	log := logCEFPermission(wv)
	if len(types) == 0 || wv.destroyed.Load() {
		log.Debug().Strs("types", types).Msg("cef: denying unsupported or late permission request")
		req.resolve(false)
		return
	}
	origin, err := urlutil.ExtractOrigin(requestingOrigin)
	if err != nil {
		log.Debug().Err(err).Msg("cef: denying permission request with invalid origin")
		req.resolve(false)
		return
	}

	wv.mu.RLock()
	cb := wv.callbacks
	wv.mu.RUnlock()
	if cb == nil || cb.OnPermissionRequest == nil {
		log.Debug().Str("origin", origin).Msg("cef: denying permission request; no handler")
		req.resolve(false)
		return
	}

	allow := func() { req.resolve(true) }
	deny := func() { req.resolve(false) }
	wv.runOnGTK(func() {
		if req.isDone() {
			return
		}
		if !cb.OnPermissionRequest(origin, types, map[string]string{}, allow, deny) {
			req.resolve(false)
		}
	})
}

// dispatchPermissionResult runs fn on the CEF UI thread, as CEF permission
// callbacks must be continued there. When the UI thread is unreachable fn runs
// inline so the page never hangs.
func dispatchPermissionResult(wv *WebView, fn func()) {
	if wv == nil || wv.engine == nil {
		fn()
		return
	}
	task := cefNewTask(cefTaskFunc(fn))
	if result := cefPostTask(purecef.ThreadIDTidUi, task); result != 1 {
		logCEFPermission(wv).Warn().
			Int32("post_result", result).
			Msg("cef: failed to post permission result to CEF UI thread; continuing inline")
		fn()
	}
}

func logCEFPermission(wv *WebView) *zerolog.Logger {
	if wv == nil || wv.ctx == nil {
		return logging.FromContext(context.Background())
	}
	return logging.FromContext(wv.ctx)
}
