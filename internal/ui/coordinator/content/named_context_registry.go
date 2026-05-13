package content

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type namedBrowsingContextKey struct {
	WindowID string
	Name     string
}

type namedBrowsingContextState struct {
	PaneID    entity.PaneID
	WebViewID port.WebViewID
}

type namedBrowsingContextRegistry struct {
	mu       sync.RWMutex
	contexts map[namedBrowsingContextKey]namedBrowsingContextState
}

func newNamedBrowsingContextRegistry() *namedBrowsingContextRegistry {
	return &namedBrowsingContextRegistry{
		contexts: make(map[namedBrowsingContextKey]namedBrowsingContextState),
	}
}

func (r *namedBrowsingContextRegistry) Register(windowID, name string, paneID entity.PaneID, webViewID port.WebViewID) {
	if r == nil || windowID == "" || name == "" || paneID == "" || webViewID == 0 {
		return
	}
	r.mu.Lock()
	r.contexts[namedBrowsingContextKey{WindowID: windowID, Name: name}] = namedBrowsingContextState{
		PaneID:    paneID,
		WebViewID: webViewID,
	}
	r.mu.Unlock()
}

func (r *namedBrowsingContextRegistry) Lookup(
	windowID, name string,
	lookupWebView func(entity.PaneID) port.WebView,
	resolveWindowID func(entity.PaneID) (string, bool),
) (namedBrowsingContextState, port.WebView, bool) {
	if r == nil || windowID == "" || name == "" || lookupWebView == nil || resolveWindowID == nil {
		return namedBrowsingContextState{}, nil, false
	}

	key := namedBrowsingContextKey{WindowID: windowID, Name: name}
	r.mu.RLock()
	state, ok := r.contexts[key]
	r.mu.RUnlock()
	if !ok {
		return namedBrowsingContextState{}, nil, false
	}

	currentWindowID, ok := resolveWindowID(state.PaneID)
	if !ok || currentWindowID != windowID {
		r.mu.Lock()
		delete(r.contexts, key)
		r.mu.Unlock()
		return namedBrowsingContextState{}, nil, false
	}

	wv := lookupWebView(state.PaneID)
	if wv == nil || wv.IsDestroyed() || wv.ID() != state.WebViewID {
		r.mu.Lock()
		delete(r.contexts, key)
		r.mu.Unlock()
		return namedBrowsingContextState{}, nil, false
	}

	return state, wv, true
}

func (r *namedBrowsingContextRegistry) UnregisterByPaneID(paneID entity.PaneID) {
	if r == nil || paneID == "" {
		return
	}
	r.mu.Lock()
	for key, state := range r.contexts {
		if state.PaneID == paneID {
			delete(r.contexts, key)
		}
	}
	r.mu.Unlock()
}

func (r *namedBrowsingContextRegistry) UnregisterByWebViewID(webViewID port.WebViewID) {
	if r == nil || webViewID == 0 {
		return
	}
	r.mu.Lock()
	for key, state := range r.contexts {
		if state.WebViewID == webViewID {
			delete(r.contexts, key)
		}
	}
	r.mu.Unlock()
}

func (r *namedBrowsingContextRegistry) UnregisterWindow(windowID string) {
	if r == nil || windowID == "" {
		return
	}
	r.mu.Lock()
	for key := range r.contexts {
		if key.WindowID == windowID {
			delete(r.contexts, key)
		}
	}
	r.mu.Unlock()
}
