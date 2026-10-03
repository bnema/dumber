package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type stubMediaAccessCallback struct {
	contCalls   []uint32
	cancelCalls int
}

func (s *stubMediaAccessCallback) Cont(allowed uint32) { s.contCalls = append(s.contCalls, allowed) }
func (s *stubMediaAccessCallback) Cancel()             { s.cancelCalls++ }

type stubPermissionPromptCallback struct {
	results []purecef.PermissionRequestResult
}

func (s *stubPermissionPromptCallback) Cont(result purecef.PermissionRequestResult) {
	s.results = append(s.results, result)
}

type permissionCapture struct {
	origin string
	types  []string
	allow  func()
	deny   func()
	meta   map[string]string
	calls  int
}

const testTopURL = "https://meet.example.com/room"

// newPermissionTestHandler builds a handlerSet whose OnPermissionRequest
// captures the request, and a browser whose main frame is at testTopURL.
func newPermissionTestHandler(t *testing.T, handled bool) (*handlerSet, *permissionCapture, purecef.Browser) {
	t.Helper()
	capture := &permissionCapture{}
	wv := &WebView{
		ctx: context.Background(),
		callbacks: &port.WebViewCallbacks{
			OnPermissionRequest: func(origin string, types []string, _ map[string]string, allow, deny func()) bool {
				capture.calls++
				capture.origin, capture.types, capture.allow, capture.deny = origin, types, allow, deny
				return handled
			},
		},
	}
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	browser.EXPECT().GetMainFrame().Return(frame).Maybe()
	frame.EXPECT().GetURL().Return(testTopURL).Maybe()
	return &handlerSet{wv: wv}, capture, browser
}

const (
	mediaMic    = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDeviceAudioCapture)
	mediaCamera = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDeviceVideoCapture)
	mediaDeskA  = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDesktopAudioCapture)
	mediaDeskV  = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDesktopVideoCapture)

	promptGeo   = uint32(purecef.PermissionRequestTypesPermissionTypeGeolocation)
	promptNotif = uint32(purecef.PermissionRequestTypesPermissionTypeNotifications)
	promptMic   = uint32(purecef.PermissionRequestTypesPermissionTypeMicStream)
	promptCam   = uint32(purecef.PermissionRequestTypesPermissionTypeCameraStream)
	promptPTZ   = uint32(purecef.PermissionRequestTypesPermissionTypeCameraPanTiltZoom)
	promptClip  = uint32(purecef.PermissionRequestTypesPermissionTypeClipboard)
)

const (
	promptAccept = purecef.PermissionRequestResultPermissionResultAccept
	promptDeny   = purecef.PermissionRequestResultPermissionResultDeny
)

func TestMediaPermissionTypes(t *testing.T) {
	tests := []struct {
		name      string
		requested uint32
		types     []string
	}{
		{"none", 0, nil},
		{"microphone", mediaMic, []string{"microphone"}},
		{"camera", mediaCamera, []string{"camera"}},
		{"microphone and camera", mediaMic | mediaCamera, []string{"microphone", "camera"}},
		{"desktop video", mediaDeskV, []string{"display"}},
		{"desktop audio and video", mediaDeskA | mediaDeskV, []string{"display"}},
		{"camera and desktop", mediaCamera | mediaDeskV, []string{"camera", "display"}},
		{"desktop audio only is denied", mediaDeskA, nil},
		{"mic and desktop audio only is denied", mediaMic | mediaDeskA, nil},
		{"unknown bit denies everything", mediaMic | 1<<10, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.types, mediaPermissionTypes(tt.requested))
		})
	}
}

func TestPromptPermissionTypes(t *testing.T) {
	tests := []struct {
		name      string
		requested uint32
		types     []string
	}{
		{"none", 0, nil},
		{"geolocation", promptGeo, []string{"geolocation"}},
		{"notifications", promptNotif, []string{"notification"}},
		{"mic and camera", promptMic | promptCam, []string{"microphone", "camera"}},
		{"camera pan tilt zoom unsupported", promptPTZ, nil},
		{"unsupported", promptClip, nil},
		{"supported plus unsupported denies all", promptGeo | promptClip, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.types, promptPermissionTypes(tt.requested))
		})
	}
}

func TestOnRequestMediaAccessPermissionAllowGrantsRequestedMask(t *testing.T) {
	h, capture, browser := newPermissionTestHandler(t, true)
	cb := &stubMediaAccessCallback{}

	handled := h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic|mediaCamera, cb)

	require.Equal(t, int32(1), handled)
	require.Equal(t, "https://meet.example.com", capture.origin)
	require.Equal(t, []string{"microphone", "camera"}, capture.types)
	require.Empty(t, cb.contCalls)

	capture.allow()
	assert.Equal(t, []uint32{mediaMic | mediaCamera}, cb.contCalls)
	assert.Zero(t, cb.cancelCalls)
	assert.Empty(t, h.permissions.pending)
}

func TestPermissionRequestResolvesOnlyOnce(t *testing.T) {
	steps := map[string]func(c *permissionCapture){
		"allow first": func(c *permissionCapture) { c.allow(); c.deny(); c.allow() },
		"deny first":  func(c *permissionCapture) { c.deny(); c.allow(); c.deny() },
	}
	for name, step := range steps {
		wantAllow := name == "allow first"
		t.Run("media "+name, func(t *testing.T) {
			h, capture, browser := newPermissionTestHandler(t, true)
			cb := &stubMediaAccessCallback{}
			require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic, cb))
			step(capture)
			if wantAllow {
				assert.Equal(t, []uint32{mediaMic}, cb.contCalls)
				assert.Zero(t, cb.cancelCalls)
			} else {
				assert.Empty(t, cb.contCalls)
				assert.Equal(t, 1, cb.cancelCalls)
			}
		})
		t.Run("prompt "+name, func(t *testing.T) {
			h, capture, browser := newPermissionTestHandler(t, true)
			cb := &stubPermissionPromptCallback{}
			require.Equal(t, int32(1), h.OnShowPermissionPrompt(browser, 7, "https://meet.example.com", promptGeo, cb))
			require.Equal(t, []string{"geolocation"}, capture.types)
			step(capture)
			want := promptDeny
			if wantAllow {
				want = promptAccept
			}
			assert.Equal(t, []purecef.PermissionRequestResult{want}, cb.results)
		})
	}
}

func TestOnRequestMediaAccessPermissionDenies(t *testing.T) {
	tests := []struct {
		name      string
		handled   bool
		origin    string
		requested uint32
		wantCalls int
	}{
		{"handler returns false", false, "https://meet.example.com", mediaMic, 1},
		{"unknown bits", true, "https://meet.example.com", mediaMic | 1<<10, 0},
		{"desktop audio only", true, "https://meet.example.com", mediaDeskA, 0},
		{"invalid origin", true, "", mediaMic, 0},
		{"cross-origin requester", true, "https://evil.example.org", mediaMic, 0},
		{"different port", true, "https://meet.example.com:8443", mediaMic, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, capture, browser := newPermissionTestHandler(t, tt.handled)
			cb := &stubMediaAccessCallback{}
			require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, tt.origin, tt.requested, cb))
			assert.Equal(t, 1, cb.cancelCalls)
			assert.Empty(t, cb.contCalls)
			assert.Equal(t, tt.wantCalls, capture.calls)
		})
	}
}

func TestOnRequestMediaAccessPermissionDeniesWithoutTopLevelOrigin(t *testing.T) {
	t.Run("nil browser", func(t *testing.T) {
		h, capture, _ := newPermissionTestHandler(t, true)
		cb := &stubMediaAccessCallback{}
		require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://meet.example.com", mediaMic, cb))
		assert.Equal(t, 1, cb.cancelCalls)
		assert.Zero(t, capture.calls)
	})
	t.Run("nil main frame", func(t *testing.T) {
		h, capture, _ := newPermissionTestHandler(t, true)
		browser := cefmocks.NewMockBrowser(t)
		browser.EXPECT().GetMainFrame().Return(nil)
		cb := &stubMediaAccessCallback{}
		require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic, cb))
		assert.Equal(t, 1, cb.cancelCalls)
		assert.Zero(t, capture.calls)
	})
	t.Run("invalid main frame url", func(t *testing.T) {
		h, capture, _ := newPermissionTestHandler(t, true)
		browser := cefmocks.NewMockBrowser(t)
		frame := cefmocks.NewMockFrame(t)
		browser.EXPECT().GetMainFrame().Return(frame)
		frame.EXPECT().GetURL().Return("about:blank")
		cb := &stubMediaAccessCallback{}
		require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic, cb))
		assert.Equal(t, 1, cb.cancelCalls)
		assert.Zero(t, capture.calls)
	})
}

func TestOnRequestMediaAccessPermissionDeniesWithoutCallback(t *testing.T) {
	for name, cbs := range map[string]*port.WebViewCallbacks{
		"nil callbacks":              nil,
		"nil OnPermissionRequest fn": {},
	} {
		t.Run(name, func(t *testing.T) {
			h, _, browser := newPermissionTestHandler(t, true)
			h.wv.callbacks = cbs
			cb := &stubMediaAccessCallback{}
			require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic, cb))
			assert.Equal(t, 1, cb.cancelCalls)
			assert.Empty(t, cb.contCalls)
		})
	}
}

func TestOnRequestMediaAccessPermissionDeniesWhenDestroyed(t *testing.T) {
	h, capture, browser := newPermissionTestHandler(t, true)
	h.wv.destroyed.Store(true)
	cb := &stubMediaAccessCallback{}
	require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic, cb))
	assert.Equal(t, 1, cb.cancelCalls)
	assert.Zero(t, capture.calls)
}

func TestDeliverPermissionRequest(t *testing.T) {
	newReq := func(allowed *[]bool) *cefPermissionRequest {
		return &cefPermissionRequest{finish: func(allow bool) { *allowed = append(*allowed, allow) }}
	}
	handlerCalls := 0
	cb := &port.WebViewCallbacks{
		OnPermissionRequest: func(string, []string, map[string]string, func(), func()) bool {
			handlerCalls++
			return true
		},
	}

	t.Run("already resolved is skipped", func(t *testing.T) {
		h := &handlerSet{wv: &WebView{ctx: context.Background()}}
		var results []bool
		req := newReq(&results)
		req.resolve(false)
		handlerCalls = 0
		h.deliverPermissionRequest(req, cb, "https://a.example", []string{"camera"}, nil)
		assert.Zero(t, handlerCalls)
		assert.Equal(t, []bool{false}, results)
	})
	t.Run("destroyed before delivery denies", func(t *testing.T) {
		h := &handlerSet{wv: &WebView{ctx: context.Background()}}
		h.wv.destroyed.Store(true)
		var results []bool
		req := newReq(&results)
		handlerCalls = 0
		h.deliverPermissionRequest(req, cb, "https://a.example", []string{"camera"}, nil)
		assert.Zero(t, handlerCalls)
		assert.Equal(t, []bool{false}, results)
	})
}

func TestPermissionTrackerAddDuplicateKeyReturnsPrevious(t *testing.T) {
	var tracker permissionTracker
	var results []string
	mk := func(name string) *cefPermissionRequest {
		return &cefPermissionRequest{finish: func(bool) { results = append(results, name) }}
	}
	key := permissionKey{prompt: true, id: 1}
	first, second := mk("first"), mk("second")

	assert.Nil(t, tracker.add(key, first))
	prev := tracker.add(key, second)
	require.Same(t, first, prev)
	prev.resolve(false)

	assert.Equal(t, []string{"first"}, results)
	assert.Same(t, second, tracker.get(key), "resolving the replaced request must not untrack the new one")
}

func TestOnShowPermissionPromptDuplicateIDDeniesPrevious(t *testing.T) {
	h, _, browser := newPermissionTestHandler(t, true)
	first, second := &stubPermissionPromptCallback{}, &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(browser, 4, "https://meet.example.com", promptGeo, first))
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(browser, 4, "https://meet.example.com", promptGeo, second))
	assert.Equal(t, []purecef.PermissionRequestResult{promptDeny}, first.results)
	assert.Empty(t, second.results)
}

func TestOnShowPermissionPromptUnsupportedReturnsZero(t *testing.T) {
	for name, mask := range map[string]uint32{
		"empty":       0,
		"clipboard":   promptClip,
		"ptz":         promptPTZ,
		"mixed":       promptGeo | promptClip,
		"unknown bit": 1 << 30,
	} {
		t.Run(name, func(t *testing.T) {
			h, capture, browser := newPermissionTestHandler(t, true)
			cb := &stubPermissionPromptCallback{}
			assert.Equal(t, int32(0), h.OnShowPermissionPrompt(browser, 1, "https://meet.example.com", mask, cb))
			assert.Empty(t, cb.results)
			assert.Zero(t, capture.calls)
			assert.Empty(t, h.permissions.pending)
		})
	}
}

func TestOnShowPermissionPromptDeniesCrossOrigin(t *testing.T) {
	h, capture, browser := newPermissionTestHandler(t, true)
	cb := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(browser, 1, "https://evil.example.org", promptNotif, cb))
	assert.Equal(t, []purecef.PermissionRequestResult{promptDeny}, cb.results)
	assert.Zero(t, capture.calls)
}

func TestOnDismissPermissionPromptDropsPendingWithoutCallingCEF(t *testing.T) {
	h, capture, browser := newPermissionTestHandler(t, true)
	cb := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(browser, 3, "https://meet.example.com", promptNotif, cb))

	h.OnDismissPermissionPrompt(nil, 3, purecef.PermissionRequestResultPermissionResultDismiss)
	capture.allow()
	capture.deny()

	assert.Empty(t, cb.results, "a dismissed prompt callback must not be continued")
	assert.Empty(t, h.permissions.pending)

	// Dismissing an unknown or already-resolved prompt is a no-op.
	h.OnDismissPermissionPrompt(nil, 3, purecef.PermissionRequestResultPermissionResultDismiss)
	h.OnDismissPermissionPrompt(nil, 99, purecef.PermissionRequestResultPermissionResultDismiss)
}

func TestDenyPendingPermissionsResolvesAllOnce(t *testing.T) {
	h, capture, browser := newPermissionTestHandler(t, true)
	media := &stubMediaAccessCallback{}
	prompt := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", mediaMic, media))
	mediaAllow := capture.allow
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(browser, 5, "https://meet.example.com", promptGeo, prompt))
	promptAllow := capture.allow

	h.denyPendingPermissions()
	h.denyPendingPermissions()
	mediaAllow()
	promptAllow()

	assert.Equal(t, 1, media.cancelCalls)
	assert.Empty(t, media.contCalls)
	assert.Equal(t, []purecef.PermissionRequestResult{promptDeny}, prompt.results)
	assert.Empty(t, h.permissions.pending)
}

func TestDispatchPermissionResultRunsInlineWhenPostFails(t *testing.T) {
	prevNew, prevPost := cefNewTask, cefPostTask
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	cefPostTask = func(purecef.ThreadID, purecef.Task) int32 { return 0 }
	defer func() { cefNewTask, cefPostTask = prevNew, prevPost }()

	ran := 0
	dispatchPermissionResult(&WebView{ctx: context.Background(), engine: &Engine{}}, func() { ran++ })
	assert.Equal(t, 1, ran)
}

func TestDispatchPermissionResultPostsToUIThread(t *testing.T) {
	prevNew, prevPost := cefNewTask, cefPostTask
	var posted purecef.Task
	var gotThread purecef.ThreadID
	cefNewTask = func(task purecef.Task) purecef.Task { return task }
	cefPostTask = func(id purecef.ThreadID, task purecef.Task) int32 {
		gotThread, posted = id, task
		return 1
	}
	defer func() { cefNewTask, cefPostTask = prevNew, prevPost }()

	ran := 0
	dispatchPermissionResult(&WebView{ctx: context.Background(), engine: &Engine{}}, func() { ran++ })
	require.NotNil(t, posted)
	assert.Zero(t, ran)
	assert.Equal(t, purecef.ThreadIDTidUi, gotThread)
	posted.Execute()
	assert.Equal(t, 1, ran)
}

func TestOnRequestMediaAccessPermissionFlagsDesktopVideoAsUnmediated(t *testing.T) {
	tests := []struct {
		name      string
		requested uint32
		flagged   bool
	}{
		{"desktop video", mediaDeskV, true},
		{"desktop audio and video", mediaDeskA | mediaDeskV, true},
		{"camera and desktop video", mediaCamera | mediaDeskV, true},
		{"camera only", mediaCamera, false},
		{"microphone and camera", mediaMic | mediaCamera, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capture := &permissionCapture{}
			h, _, browser := newPermissionTestHandler(t, true)
			h.wv.callbacks.OnPermissionRequest = func(_ string, types []string, meta map[string]string, _, _ func()) bool {
				capture.types, capture.meta = types, meta
				return true
			}
			cb := &stubMediaAccessCallback{}
			require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(browser, nil, "https://meet.example.com", tt.requested, cb))
			require.NotNil(t, capture.types)
			assert.Equal(t, tt.flagged, entity.PermissionMetadata(capture.meta).IsUnmediatedCapture())
		})
	}
}
