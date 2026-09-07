package cef

import (
	"sync"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/application/port"
)

// listMatcher is the P4.2 fake matcher: synthetic block-list decisions over
// loopback fixtures, recording every offered request.
type listMatcher struct {
	mu      sync.Mutex
	blocked map[string]bool
	seen    []port.FilterRequest
}

func (m *listMatcher) Decide(req port.FilterRequest) port.FilterAction {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen = append(m.seen, req)
	if m.blocked[req.URL] {
		return port.FilterBlock
	}
	return port.FilterAllow
}

func (m *listMatcher) seenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.seen)
}

func filterRequest(t *testing.T, url string, resourceType purecef.ResourceType, isMain bool) (purecef.Frame, purecef.Request) {
	t.Helper()
	frame := cefmocks.NewMockFrame(t)
	frame.EXPECT().IsMain().Return(isMain).Maybe()
	request := cefmocks.NewMockRequest(t)
	request.EXPECT().GetURL().Return(url).Maybe()
	request.EXPECT().GetResourceType().Return(resourceType).Maybe()
	return frame, request
}

// TestFilterSeam_AllowsMainDocument verifies the baseline: an unlisted main
// document proceeds.
func TestFilterSeam_AllowsMainDocument(t *testing.T) {
	matcher := &listMatcher{blocked: map[string]bool{}}
	handler := newFilterRequestHandler(matcher)

	frame, request := filterRequest(t, "http://127.0.0.1:8471/page", purecef.ResourceTypeRtMainFrame, true)
	rv := handler.OnBeforeResourceLoad(nil, frame, request, nil)
	require.Equal(t, purecef.ReturnValueRvContinue, rv)
	require.Equal(t, 1, matcher.seenCount())
}

// TestFilterSeam_BlocksListedSubresource verifies subresource blocking while
// the document itself stays allowed.
func TestFilterSeam_BlocksListedSubresource(t *testing.T) {
	matcher := &listMatcher{blocked: map[string]bool{"http://127.0.0.1:8471/tracker.js": true}}
	handler := newFilterRequestHandler(matcher)

	frame, _ := filterRequest(t, "http://127.0.0.1:8471/page", purecef.ResourceTypeRtMainFrame, true)
	require.Equal(t, purecef.ReturnValueRvContinue,
		handler.OnBeforeResourceLoad(nil, frame, mustFilterRequest(t, "http://127.0.0.1:8471/app.js", purecef.ResourceTypeRtScript), nil))

	_, blocked := filterRequest(t, "http://127.0.0.1:8471/tracker.js", purecef.ResourceTypeRtScript, false)
	require.Equal(t, purecef.ReturnValueRvCancel,
		handler.OnBeforeResourceLoad(nil, frame, blocked, nil))
}

func mustFilterRequest(t *testing.T, url string, resourceType purecef.ResourceType) purecef.Request {
	t.Helper()
	_, request := filterRequest(t, url, resourceType, false)
	return request
}

// TestFilterSeam_ClassifiesNavigationVsSubresource proves main/sub-frame
// document loads arrive as navigations and scripts as subresources.
func TestFilterSeam_ClassifiesNavigationVsSubresource(t *testing.T) {
	var got []port.FilterRequest
	matcher := &listMatcher{blocked: map[string]bool{}}
	handler := newFilterRequestHandler(recordingMatcher(matcher, &got))

	frame, mainReq := filterRequest(t, "http://127.0.0.1:8471/page", purecef.ResourceTypeRtMainFrame, true)
	handler.OnBeforeResourceLoad(nil, frame, mainReq, nil)
	// Page scripts belong to the main frame yet are subresources: frame
	// identity and navigation classification are independent axes.
	handler.OnBeforeResourceLoad(nil, frame, mustFilterRequest(t, "http://127.0.0.1:8471/app.js", purecef.ResourceTypeRtScript), nil)
	subFrame, _ := filterRequest(t, "http://127.0.0.1:8471/frame", purecef.ResourceTypeRtSubFrame, false)
	_, frameReq := filterRequest(t, "http://127.0.0.1:8471/frame", purecef.ResourceTypeRtSubFrame, false)
	handler.OnBeforeResourceLoad(nil, subFrame, frameReq, nil)

	require.Len(t, got, 3)
	require.True(t, got[0].IsMainFrame && got[0].IsNavigation, "main document must be a main-frame navigation")
	require.True(t, got[1].IsMainFrame, "page scripts belong to the main frame")
	require.False(t, got[1].IsNavigation, "scripts must be subresources")
	require.True(t, got[2].IsNavigation, "sub-frame documents are navigations")
	require.False(t, got[2].IsMainFrame)
}

func recordingMatcher(inner *listMatcher, out *[]port.FilterRequest) port.ContentFilter {
	return filterFunc(func(req port.FilterRequest) port.FilterAction {
		*out = append(*out, req)
		return inner.Decide(req)
	})
}

type filterFunc func(port.FilterRequest) port.FilterAction

func (f filterFunc) Decide(req port.FilterRequest) port.FilterAction { return f(req) }

// TestFilterSeam_RedirectTargetEvaluatedIndependently verifies each request
// in a redirect chain is offered separately: blocking the redirect target
// cancels it without retroactively affecting the source decision.
func TestFilterSeam_RedirectTargetEvaluatedIndependently(t *testing.T) {
	matcher := &listMatcher{blocked: map[string]bool{"http://127.0.0.1:8471/target": true}}
	handler := newFilterRequestHandler(matcher)

	frame, source := filterRequest(t, "http://127.0.0.1:8471/redirect", purecef.ResourceTypeRtMainFrame, true)
	require.Equal(t, purecef.ReturnValueRvContinue,
		handler.OnBeforeResourceLoad(nil, frame, source, nil))
	_, target := filterRequest(t, "http://127.0.0.1:8471/target", purecef.ResourceTypeRtMainFrame, true)
	require.Equal(t, purecef.ReturnValueRvCancel,
		handler.OnBeforeResourceLoad(nil, frame, target, nil))
	require.Equal(t, 2, matcher.seenCount())
}

// TestFilterSeam_UnclassifiableAllows verifies fail-open guards: nil
// request, empty URL, nil frame, and nil matcher never cancel.
func TestFilterSeam_UnclassifiableAllows(t *testing.T) {
	matcher := &listMatcher{blocked: map[string]bool{"http://127.0.0.1:8471/x": true}}
	handler := newFilterRequestHandler(matcher)

	require.Equal(t, purecef.ReturnValueRvContinue, handler.OnBeforeResourceLoad(nil, nil, nil, nil))

	// A nil frame does not make a listed URL unclassifiable: the URL
	// decision still applies.
	require.Equal(t, purecef.ReturnValueRvCancel, handler.OnBeforeResourceLoad(nil, nil, mustFilterRequest(t, "http://127.0.0.1:8471/x", purecef.ResourceTypeRtScript), nil))

	frame, _ := filterRequest(t, "http://127.0.0.1:8471/x", purecef.ResourceTypeRtScript, false)
	require.Equal(t, purecef.ReturnValueRvContinue, handler.OnBeforeResourceLoad(nil, frame, nil, nil))

	emptyURL := cefmocks.NewMockRequest(t)
	emptyURL.EXPECT().GetURL().Return("").Once()
	require.Equal(t, purecef.ReturnValueRvContinue, handler.OnBeforeResourceLoad(nil, frame, emptyURL, nil))

	allowAll := newFilterRequestHandler(nil)
	_, blocked := filterRequest(t, "http://127.0.0.1:8471/x", purecef.ResourceTypeRtScript, false)
	require.Equal(t, purecef.ReturnValueRvContinue, allowAll.OnBeforeResourceLoad(nil, frame, blocked, nil))
	require.Equal(t, purecef.ReturnValueRvContinue, (*filterRequestHandler)(nil).OnBeforeResourceLoad(nil, frame, blocked, nil))
}

// TestFilterSeam_CloseRetiresMatcher verifies shutdown semantics: after
// Close the matcher is no longer consulted and everything is allowed.
func TestFilterSeam_CloseRetiresMatcher(t *testing.T) {
	matcher := &listMatcher{blocked: map[string]bool{"http://127.0.0.1:8471/tracker.js": true}}
	handler := newFilterRequestHandler(matcher)

	_, blocked := filterRequest(t, "http://127.0.0.1:8471/tracker.js", purecef.ResourceTypeRtScript, false)
	frame, _ := filterRequest(t, "http://127.0.0.1:8471/page", purecef.ResourceTypeRtMainFrame, true)
	require.Equal(t, purecef.ReturnValueRvCancel, handler.OnBeforeResourceLoad(nil, frame, blocked, nil))
	before := matcher.seenCount()

	handler.Close()
	require.Equal(t, purecef.ReturnValueRvContinue, handler.OnBeforeResourceLoad(nil, frame, blocked, nil))
	require.Equal(t, before, matcher.seenCount(), "retired matcher must not be consulted")
	handler.Close()
}
