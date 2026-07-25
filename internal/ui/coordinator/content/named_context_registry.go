package content

import (
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type namedBrowsingContextKey struct {
	OwnerWindowID string
	Name          string
}

type namedBrowsingContextState struct {
	PaneID       entity.PaneID
	WebViewID    port.WebViewID
	HostWindowID string
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

func (r *namedBrowsingContextRegistry) Register(ownerWindowID, hostWindowID, name string, paneID entity.PaneID, webViewID port.WebViewID) {
	if r == nil || ownerWindowID == "" || hostWindowID == "" || name == "" || paneID == "" || webViewID == 0 {
		return
	}
	r.mu.Lock()
	r.contexts[namedBrowsingContextKey{OwnerWindowID: ownerWindowID, Name: name}] = namedBrowsingContextState{
		PaneID: paneID, WebViewID: webViewID, HostWindowID: hostWindowID,
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

	key := namedBrowsingContextKey{OwnerWindowID: windowID, Name: name}
	r.mu.RLock()
	state, ok := r.contexts[key]
	r.mu.RUnlock()
	if !ok {
		return namedBrowsingContextState{}, nil, false
	}

	currentWindowID, ok := resolveWindowID(state.PaneID)
	if !ok || currentWindowID != state.HostWindowID {
		r.deleteIfStateMatches(key, state)
		return namedBrowsingContextState{}, nil, false
	}

	wv := lookupWebView(state.PaneID)
	if wv == nil || wv.IsDestroyed() || wv.ID() != state.WebViewID {
		r.deleteIfStateMatches(key, state)
		return namedBrowsingContextState{}, nil, false
	}

	return state, wv, true
}

func (r *namedBrowsingContextRegistry) deleteIfStateMatches(key namedBrowsingContextKey, state namedBrowsingContextState) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if currentState, ok := r.contexts[key]; ok && currentState == state {
		delete(r.contexts, key)
	}
	r.mu.Unlock()
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
	for key, state := range r.contexts {
		if key.OwnerWindowID == windowID || state.HostWindowID == windowID {
			delete(r.contexts, key)
		}
	}
	r.mu.Unlock()
}
