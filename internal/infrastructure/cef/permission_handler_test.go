package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
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
	calls  int
}

func newPermissionTestHandler(handled bool) (*handlerSet, *permissionCapture) {
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
	return &handlerSet{wv: wv}, capture
}

const (
	mediaMic    = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDeviceAudioCapture)
	mediaCamera = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDeviceVideoCapture)
	mediaDeskA  = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDesktopAudioCapture)
	mediaDeskV  = uint32(purecef.MediaAccessPermissionTypesMediaPermissionDesktopVideoCapture)
)

func TestMediaPermissionTypes(t *testing.T) {
	tests := []struct {
		name      string
		requested uint32
		types     []string
		allowed   uint32
	}{
		{"none", 0, nil, 0},
		{"microphone", mediaMic, []string{"microphone"}, mediaMic},
		{"camera", mediaCamera, []string{"camera"}, mediaCamera},
		{"microphone and camera", mediaMic | mediaCamera, []string{"microphone", "camera"}, mediaMic | mediaCamera},
		{"desktop video", mediaDeskV, []string{"display"}, mediaDeskV},
		{"desktop audio and video", mediaDeskA | mediaDeskV, []string{"display"}, mediaDeskA | mediaDeskV},
		{"camera and desktop", mediaCamera | mediaDeskV, []string{"camera", "display"}, mediaCamera | mediaDeskV},
		{"unknown bit denies everything", mediaMic | 1<<10, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			types, allowed := mediaPermissionTypes(tt.requested)
			assert.Equal(t, tt.types, types)
			assert.Equal(t, tt.allowed, allowed)
		})
	}
}

func TestPromptPermissionTypes(t *testing.T) {
	const (
		geo   = uint32(purecef.PermissionRequestTypesPermissionTypeGeolocation)
		notif = uint32(purecef.PermissionRequestTypesPermissionTypeNotifications)
		mic   = uint32(purecef.PermissionRequestTypesPermissionTypeMicStream)
		cam   = uint32(purecef.PermissionRequestTypesPermissionTypeCameraStream)
		ptz   = uint32(purecef.PermissionRequestTypesPermissionTypeCameraPanTiltZoom)
		clip  = uint32(purecef.PermissionRequestTypesPermissionTypeClipboard)
	)
	tests := []struct {
		name      string
		requested uint32
		types     []string
		ok        bool
	}{
		{"none", 0, nil, false},
		{"geolocation", geo, []string{"geolocation"}, true},
		{"notifications", notif, []string{"notification"}, true},
		{"mic and camera", mic | cam, []string{"microphone", "camera"}, true},
		{"camera pan tilt zoom", ptz, []string{"camera"}, true},
		{"unsupported", clip, nil, false},
		{"supported plus unsupported denies all", geo | clip, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			types, ok := promptPermissionTypes(tt.requested)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.types, types)
		})
	}
}

func TestOnRequestMediaAccessPermissionAllowGrantsRequestedMask(t *testing.T) {
	h, capture := newPermissionTestHandler(true)
	cb := &stubMediaAccessCallback{}

	handled := h.OnRequestMediaAccessPermission(nil, nil, "https://meet.example.com/room", mediaMic|mediaCamera, cb)

	require.Equal(t, int32(1), handled)
	require.Equal(t, "https://meet.example.com", capture.origin)
	require.Equal(t, []string{"microphone", "camera"}, capture.types)
	require.Empty(t, cb.contCalls)

	capture.allow()
	capture.allow()
	capture.deny()

	assert.Equal(t, []uint32{mediaMic | mediaCamera}, cb.contCalls)
	assert.Zero(t, cb.cancelCalls)
	assert.Empty(t, h.permissions.pending)
}

func TestOnRequestMediaAccessPermissionDenyOnlyOnce(t *testing.T) {
	h, capture := newPermissionTestHandler(true)
	cb := &stubMediaAccessCallback{}

	require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://a.example", mediaMic, cb))
	capture.deny()
	capture.allow()
	capture.deny()

	assert.Empty(t, cb.contCalls)
	assert.Equal(t, 1, cb.cancelCalls)
}

func TestOnRequestMediaAccessPermissionDeniesWhenNotHandledOrUnsupported(t *testing.T) {
	t.Run("handler returns false", func(t *testing.T) {
		h, _ := newPermissionTestHandler(false)
		cb := &stubMediaAccessCallback{}
		require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://a.example", mediaMic, cb))
		assert.Equal(t, 1, cb.cancelCalls)
		assert.Empty(t, cb.contCalls)
	})
	t.Run("unknown bits", func(t *testing.T) {
		h, capture := newPermissionTestHandler(true)
		cb := &stubMediaAccessCallback{}
		require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://a.example", mediaMic|1<<10, cb))
		assert.Equal(t, 1, cb.cancelCalls)
		assert.Zero(t, capture.calls)
	})
	t.Run("invalid origin", func(t *testing.T) {
		h, capture := newPermissionTestHandler(true)
		cb := &stubMediaAccessCallback{}
		require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "", mediaMic, cb))
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
			h := &handlerSet{wv: &WebView{ctx: context.Background(), callbacks: cbs}}
			cb := &stubMediaAccessCallback{}
			require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://a.example", mediaMic, cb))
			assert.Equal(t, 1, cb.cancelCalls)
			assert.Empty(t, cb.contCalls)
		})
	}
}

func TestOnRequestMediaAccessPermissionDeniesWhenDestroyed(t *testing.T) {
	h, capture := newPermissionTestHandler(true)
	h.wv.destroyed.Store(true)
	cb := &stubMediaAccessCallback{}
	require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://a.example", mediaMic, cb))
	assert.Equal(t, 1, cb.cancelCalls)
	assert.Zero(t, capture.calls)
}

func TestOnShowPermissionPromptAllowAndDeny(t *testing.T) {
	const geo = uint32(purecef.PermissionRequestTypesPermissionTypeGeolocation)

	h, capture := newPermissionTestHandler(true)
	allowCb := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(nil, 7, "https://maps.example", geo, allowCb))
	require.Equal(t, []string{"geolocation"}, capture.types)
	capture.allow()
	capture.deny()
	assert.Equal(t, []purecef.PermissionRequestResult{purecef.PermissionRequestResultPermissionResultAccept}, allowCb.results)

	denyCb := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(nil, 8, "https://maps.example", geo, denyCb))
	capture.deny()
	capture.allow()
	assert.Equal(t, []purecef.PermissionRequestResult{purecef.PermissionRequestResultPermissionResultDeny}, denyCb.results)
}

func TestOnShowPermissionPromptDeniesUnsupportedType(t *testing.T) {
	const clip = uint32(purecef.PermissionRequestTypesPermissionTypeClipboard)
	h, capture := newPermissionTestHandler(true)
	cb := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(nil, 1, "https://a.example", clip, cb))
	assert.Equal(t, []purecef.PermissionRequestResult{purecef.PermissionRequestResultPermissionResultDeny}, cb.results)
	assert.Zero(t, capture.calls)
}

func TestOnDismissPermissionPromptDropsPendingWithoutCallingCEF(t *testing.T) {
	const notif = uint32(purecef.PermissionRequestTypesPermissionTypeNotifications)
	h, capture := newPermissionTestHandler(true)
	cb := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(nil, 3, "https://a.example", notif, cb))

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
	const geo = uint32(purecef.PermissionRequestTypesPermissionTypeGeolocation)
	h, capture := newPermissionTestHandler(true)
	media := &stubMediaAccessCallback{}
	prompt := &stubPermissionPromptCallback{}
	require.Equal(t, int32(1), h.OnRequestMediaAccessPermission(nil, nil, "https://a.example", mediaMic, media))
	mediaAllow := capture.allow
	require.Equal(t, int32(1), h.OnShowPermissionPrompt(nil, 5, "https://a.example", geo, prompt))
	promptAllow := capture.allow

	h.denyPendingPermissions()
	h.denyPendingPermissions()
	mediaAllow()
	promptAllow()

	assert.Equal(t, 1, media.cancelCalls)
	assert.Empty(t, media.contCalls)
	assert.Equal(t, []purecef.PermissionRequestResult{purecef.PermissionRequestResultPermissionResultDeny}, prompt.results)
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

func TestGetPermissionHandlerEnabled(t *testing.T) {
	h := &handlerSet{wv: &WebView{ctx: context.Background()}}
	assert.NotNil(t, h.GetPermissionHandler())
}
