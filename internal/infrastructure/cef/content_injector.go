package cef

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time interface check.
var _ port.ContentInjector = (*contentInjector)(nil)

// scrollbarCSS styles the scrollbar with auto-hide behavior: invisible by
// default, fades in on scroll, widens on hover, fades out after 1s idle.
// Uses --primary theme color for the thumb.
const scrollbarCSS = `
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: transparent;
  border-radius: 3px;
}
.dumber-scrolling ::-webkit-scrollbar-thumb {
  background: var(--primary, rgba(128, 128, 128, 0.4));
}
.dumber-scrolling ::-webkit-scrollbar:hover {
  width: 10px;
  height: 10px;
}
.dumber-scrolling ::-webkit-scrollbar-track:hover {
  background: rgba(128, 128, 128, 0.1);
}
.dumber-scrolling ::-webkit-scrollbar-thumb:hover {
  background: var(--primary, rgba(128, 128, 128, 0.6));
}
::-webkit-scrollbar-corner {
  background: transparent;
}
`

// scrollbarAutoHideJS adds/removes the .dumber-scrolling class on <html>
// on scroll activity, with a 1s fade-out timeout after scrolling stops.
const scrollbarAutoHideJS = `(function(){
  var t, el = document.documentElement;
  function show() {
    el.classList.add('dumber-scrolling');
    clearTimeout(t);
    t = setTimeout(function(){ el.classList.remove('dumber-scrolling'); }, 1000);
  }
  window.addEventListener('scroll', show, {passive:true,capture:true});
  window.addEventListener('wheel', show, {passive:true});
  el.addEventListener('mouseenter', function(){
    if(el.scrollHeight > el.clientHeight) show();
  });
})();`

// videoDiagnosticJS monitors all <video> elements on the page and logs their
// state changes to the console. This helps diagnose playback issues in CEF OSR.
// TODO: remove once video playback is stable.
const videoDiagnosticJS = `(function(){
  var tag = '[VIDEO-DIAG]';
  var events = ['loadstart','loadedmetadata','loadeddata','canplay','canplaythrough',
    'play','playing','pause','waiting','stalled','error','abort','emptied','suspend'];

  function monitor(v, label) {
    if (v._dumberDiag) return;
    v._dumberDiag = true;
    console.warn(tag, label, 'src:', (v.src||'').substring(0,80),
      'currentSrc:', (v.currentSrc||'').substring(0,80),
      'readyState:', v.readyState, 'networkState:', v.networkState,
      'preload:', v.preload, 'autoplay:', v.autoplay);
    events.forEach(function(evt) {
      v.addEventListener(evt, function() {
        var err = v.error ? ('code='+v.error.code+' msg='+v.error.message) : 'none';
        console.warn(tag, evt, label, 'ready:', v.readyState, 'net:', v.networkState,
          'paused:', v.paused, 'error:', err, 'src:', (v.currentSrc||v.src||'').substring(0,80));
      });
    });
  }

  // Recursively scan a root (document or shadowRoot) for video elements.
  function scanRoot(root, label) {
    root.querySelectorAll('video').forEach(function(v, i) { monitor(v, label+'#'+i); });
    // Pierce shadow DOMs.
    root.querySelectorAll('*').forEach(function(el) {
      if (el.shadowRoot) {
        scanRoot(el.shadowRoot, label+'>'+el.tagName.toLowerCase());
        // Observe inside shadow root too.
        observeRoot(el.shadowRoot, label+'>'+el.tagName.toLowerCase());
      }
    });
  }

  function observeRoot(root, label) {
    new MutationObserver(function(muts) {
      muts.forEach(function(m) {
        m.addedNodes.forEach(function(n) {
          if (n.nodeName === 'VIDEO') monitor(n, label+'-dyn');
          if (n.querySelectorAll) {
            n.querySelectorAll('video').forEach(function(v) { monitor(v, label+'-nested'); });
          }
          // New element might have shadow root.
          if (n.shadowRoot) {
            scanRoot(n.shadowRoot, label+'>'+n.tagName.toLowerCase());
            observeRoot(n.shadowRoot, label+'>'+n.tagName.toLowerCase());
          }
        });
      });
    }).observe(root, {childList:true, subtree:true});
  }

  // Patch attachShadow to automatically observe new shadow roots.
  // Use a marker to avoid double-patching if redditDirectVideoJS also patches.
  var origAttach = Element.prototype.__dumberOrigAttachShadow || Element.prototype.attachShadow;
  if (!Element.prototype.__dumberPatched) {
    Element.prototype.__dumberOrigAttachShadow = origAttach;
    Element.prototype.__dumberPatched = true;
  }
  Element.prototype.attachShadow = function(opts) {
    var sr = origAttach.call(this, opts);
    console.warn(tag, 'shadowRoot attached on', this.tagName.toLowerCase());
    observeRoot(sr, 'shadow>'+this.tagName.toLowerCase());
    return sr;
  };

  // Also intercept MediaSource to trace usage.
  if (window.MediaSource) {
    var origAddSB = MediaSource.prototype.addSourceBuffer;
    MediaSource.prototype.addSourceBuffer = function(mime) {
      console.warn(tag, 'MediaSource.addSourceBuffer:', mime,
        'supported:', MediaSource.isTypeSupported(mime));
      try { return origAddSB.call(this, mime); }
      catch(e) { console.error(tag, 'addSourceBuffer FAILED:', e.message, 'mime:', mime); throw e; }
    };
    var origURL = URL.createObjectURL;
    URL.createObjectURL = function(obj) {
      var url = origURL.call(this, obj);
      if (obj instanceof MediaSource) {
        console.warn(tag, 'URL.createObjectURL(MediaSource):', url.substring(0,80));
      }
      return url;
    };
  }

  // Initial scan.
  scanRoot(document, 'doc');

  // Observe document for new elements.
  observeRoot(document.documentElement, 'doc');

  console.warn(tag, 'diagnostic active, MSE:', !!window.MediaSource,
    'h264:', MediaSource ? MediaSource.isTypeSupported('video/mp4; codecs="avc1.4d401e"') : 'n/a',
    'aac:', MediaSource ? MediaSource.isTypeSupported('audio/mp4; codecs="mp4a.40.2"') : 'n/a');
})();`

// redditDirectVideoJS replaces Reddit's blob/MSE playback path with a direct
// HLS source when MP4 MSE buffers are unsupported. This lets the browser issue
// a normal media request that our transcoding handler can answer with WebM.
const redditDirectVideoJS = `(function(){
  var tag = '[REDDIT-VIDEO-PATCH]';
  if (!/(\.|^)reddit\.com$/i.test(location.hostname)) return;
  if (!window.MediaSource) return;
  if (MediaSource.isTypeSupported('video/mp4; codecs="avc1.4d401e"') &&
      MediaSource.isTypeSupported('audio/mp4; codecs="mp4a.40.2"')) {
    return;
  }

  console.warn(tag, 'active for unsupported Reddit MSE playback');

  function ensureStyle() {
    if (document.getElementById('dumber-reddit-direct-video-style')) return;
    var style = document.createElement('style');
    style.id = 'dumber-reddit-direct-video-style';
    style.textContent =
      'shreddit-player[data-dumber-direct-video="1"] { position: relative !important; }' +
      'shreddit-player[data-dumber-direct-video="1"] > .dumber-direct-video {' +
      ' position: absolute !important; inset: 0 !important; width: 100% !important; height: 100% !important;' +
      ' object-fit: contain !important; background: #000 !important; z-index: 2147483647 !important;' +
      ' display: block !important; }';
    document.head.appendChild(style);
  }

  function candidateRoots(video) {
    var roots = [];
    var seen = [];
    function add(root) {
      if (!root) return;
      if (seen.indexOf(root) !== -1) return;
      seen.push(root);
      roots.push(root);
    }

    var current = video;
    while (current) {
      add(current);
      if (current.shadowRoot) add(current.shadowRoot);
      current = current.parentElement;
    }
    add(document);
    return roots;
  }

  function findHlsSource(video) {
    var selectors = [
      'source[type="application/vnd.apple.mpegURL"][src]',
      'source[type="application/x-mpegURL"][src]',
      'source[src*="HLSPlaylist.m3u8"]',
      '[src*="HLSPlaylist.m3u8"]'
    ];

    var roots = candidateRoots(video);
    for (var i = 0; i < roots.length; i++) {
      var root = roots[i];
      if (!root.querySelector) continue;
      for (var j = 0; j < selectors.length; j++) {
        var el = root.querySelector(selectors[j]);
        if (el && el.src) return el.src;
      }
      if (root.querySelectorAll) {
        var all = root.querySelectorAll('*');
        for (var k = 0; k < all.length; k++) {
          if (all[k].shadowRoot) {
            roots.push(all[k].shadowRoot);
          }
        }
      }
    }
    return '';
  }

  function buildPlaybackURL(hlsURL) {
    return location.origin + '/__dumber__/transcode.webm?src=' + encodeURIComponent(hlsURL) +
      '&referer=' + encodeURIComponent(location.href) +
      '&origin=' + encodeURIComponent(location.origin);
  }

  function getPlayerRoot(video) {
    return video.closest('shreddit-player') || video.parentElement;
  }

  function monitorReplacement(video) {
    if (!video || video._dumberReplacementDiag) return;
    video._dumberReplacementDiag = true;
    ['loadstart','loadedmetadata','loadeddata','canplay','playing','waiting','stalled','suspend','abort','emptied','error'].forEach(function(evt) {
      video.addEventListener(evt, function() {
        var err = video.error ? ('code=' + video.error.code + ' msg=' + video.error.message) : 'none';
        console.warn(tag, 'replacement', evt, 'ready:', video.readyState, 'net:', video.networkState,
          'paused:', video.paused, 'error:', err, 'src:', (video.currentSrc || video.src || '').substring(0, 120));
      });
    });
  }

  function ensureReplacement(video, playbackURL) {
    var root = getPlayerRoot(video);
    if (!root) return null;
    ensureStyle();

    var replacement = root.querySelector(':scope > .dumber-direct-video');
    if (!replacement) {
      replacement = document.createElement('video');
      replacement.className = 'dumber-direct-video';
      replacement.controls = true;
      replacement.playsInline = true;
      replacement.preload = 'auto';
      replacement.autoplay = true;
      replacement.muted = true;
      replacement.loop = video.loop;
      root.setAttribute('data-dumber-direct-video', '1');
      root.appendChild(replacement);
    }

    replacement.poster = video.poster || replacement.poster || '';
    monitorReplacement(replacement);
    if (replacement.src !== playbackURL) {
      replacement.src = playbackURL;
      replacement.load();
    }
    return replacement;
  }

  function shouldForce(video) {
    var current = video.currentSrc || video.src || '';
    return current.indexOf('blob:') === 0;
  }

  function forceDirectPlayback(video) {
    if (!video || !shouldForce(video)) return;

    var hlsURL = findHlsSource(video);
    if (!hlsURL) {
      console.warn(tag, 'blob video found without HLS source');
      return;
    }
    var playbackURL = buildPlaybackURL(hlsURL);
    if (video._dumberPlaybackURL === playbackURL) return;
    video._dumberPlaybackURL = playbackURL;

    console.warn(tag, 'forcing replacement video source:', playbackURL.substring(0, 120), 'from:', hlsURL.substring(0, 120));

    var replacement = ensureReplacement(video, playbackURL);
    if (!replacement) return;

    var root = getPlayerRoot(video);
    if (root && root.querySelectorAll) {
      root.querySelectorAll('video').forEach(function(candidate) {
        if (candidate === replacement) return;
        try { candidate.pause(); } catch (e) {}
        try { candidate.removeAttribute('src'); } catch (e) {}
        try { candidate.srcObject = null; } catch (e) {}
        try { candidate.load(); } catch (e) {}
        candidate.style.display = 'none';
      });
    } else {
      try { video.pause(); } catch (e) {}
      try { video.removeAttribute('src'); } catch (e) {}
      try { video.srcObject = null; } catch (e) {}
      try { video.load(); } catch (e) {}
      video.style.display = 'none';
    }

    var playPromise = replacement.play();
    if (playPromise && typeof playPromise.catch === 'function') {
      playPromise.catch(function(err) {
        console.warn(tag, 'replacement video.play() rejected:', err && err.message ? err.message : err);
      });
    }
  }

  function patchTree(root) {
    if (!root || !root.querySelectorAll) return;
    root.querySelectorAll('video').forEach(forceDirectPlayback);
    root.querySelectorAll('*').forEach(function(el) {
      if (el.shadowRoot) patchTree(el.shadowRoot);
    });
  }

  new MutationObserver(function(mutations) {
    mutations.forEach(function(mutation) {
      if (mutation.type === 'attributes' && mutation.target && mutation.target.nodeName === 'VIDEO') {
        forceDirectPlayback(mutation.target);
      }
      mutation.addedNodes.forEach(function(node) {
        if (node.nodeName === 'VIDEO') forceDirectPlayback(node);
        patchTree(node);
        if (node.shadowRoot) patchTree(node.shadowRoot);
      });
    });
  }).observe(document.documentElement, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ['src']
  });

  // Chain to existing attachShadow patch (e.g. videoDiagnosticJS) instead of overwriting.
  var prevAttach = Element.prototype.attachShadow;
  Element.prototype.attachShadow = function(opts) {
    var sr = prevAttach.call(this, opts);
    patchTree(sr);
    return sr;
  };

  patchTree(document);
  var pollCount = 0;
  var maxPolls = 30;
  var pollID = setInterval(function() {
    patchTree(document);
    pollCount++;
    if (pollCount >= maxPolls) clearInterval(pollID);
  }, 1000);
})();`

// clipboardCopyBridgeJS intercepts copy/cut events and sends the selected text
// back to Go via the message bridge so it can be written to the GDK clipboard.
// Without this, Ctrl+C in CEF OSR mode writes to CEF's internal clipboard but
// not the Wayland/X11 system clipboard.
const clipboardCopyBridgeJS = `(function(){
  if (window.__dumberClipboardBridge) return;
  window.__dumberClipboardBridge = true;
  function sendToClipboard(text) {
    if (!text) return;
    var body = JSON.stringify({text: text});
    var encoded = typeof btoa === 'function' ? btoa(unescape(encodeURIComponent(body))) : '';
    if (!encoded) return;
    fetch('dumb:///api/clipboard-set', {
      method: 'POST',
      headers: {'X-Dumber-Body': encoded}
    }).catch(function(){});
  }
  document.addEventListener('copy', function() {
    var sel = window.getSelection();
    if (sel) sendToClipboard(sel.toString());
  });
  document.addEventListener('cut', function() {
    var sel = window.getSelection();
    if (sel) sendToClipboard(sel.toString());
  });
})();`

// contentInjector implements port.ContentInjector for the CEF engine.
// It stores CSS strings and injects them into webviews via ExecuteJavaScript.
// Thread-safe: InjectThemeCSS may be called from the UI thread while OnLoadEnd
// fires on the CEF IO thread.
type contentInjector struct {
	mu                      sync.RWMutex
	themeCSS                string
	findHighlightCSS        string
	engine                  *Engine
	colorResolver           port.ColorSchemeResolver
	videoDiagnosticsEnabled bool
}

// setColorResolver updates the color scheme resolver used for dark mode detection.
func (ci *contentInjector) setColorResolver(resolver port.ColorSchemeResolver) {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.colorResolver = resolver
}

// newContentInjector creates a content injector wired to the given engine.
// Video diagnostics are enabled when DUMBER_VIDEO_DIAG=1 is set.
func newContentInjector(engine *Engine, resolver port.ColorSchemeResolver) *contentInjector {
	diagEnabled := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DUMBER_VIDEO_DIAG"))) {
	case "1", "true", "yes", "on":
		diagEnabled = true
	}
	return &contentInjector{
		engine:                  engine,
		colorResolver:           resolver,
		videoDiagnosticsEnabled: diagEnabled,
	}
}

// InjectThemeCSS stores the theme CSS and broadcasts it to all active webviews.
func (ci *contentInjector) InjectThemeCSS(ctx context.Context, css string) error {
	log := logging.FromContext(ctx).With().Str("component", "cef-content-injector").Logger()

	ci.mu.Lock()
	ci.themeCSS = css
	ci.mu.Unlock()

	log.Debug().Int("css_len", len(css)).Msg("theme CSS set, broadcasting to active webviews")

	// Broadcast to all active webviews.
	ci.engine.activeWebViews.Range(func(_, value any) bool {
		if wv, ok := value.(*WebView); ok {
			ci.injectCSS(wv, "dumber-theme-vars", css)
		}
		return true
	})
	return nil
}

// InjectFindHighlightCSS stores the find highlight CSS and broadcasts it.
func (ci *contentInjector) InjectFindHighlightCSS(ctx context.Context, css string) error {
	log := logging.FromContext(ctx).With().Str("component", "cef-content-injector").Logger()

	ci.mu.Lock()
	ci.findHighlightCSS = css
	ci.mu.Unlock()

	log.Debug().Int("css_len", len(css)).Msg("find highlight CSS set, broadcasting to active webviews")

	ci.engine.activeWebViews.Range(func(_, value any) bool {
		if wv, ok := value.(*WebView); ok {
			ci.injectCSS(wv, "dumber-find-highlight", css)
		}
		return true
	})
	return nil
}

// RefreshScripts re-injects all scripts into a specific webview.
func (ci *contentInjector) RefreshScripts(ctx context.Context, wv port.WebView) error {
	log := logging.FromContext(ctx).With().Str("component", "cef-content-injector").Logger()
	if wv == nil {
		log.Debug().Msg("RefreshScripts: nil webview")
		return nil
	}
	cefWV, ok := wv.(*WebView)
	if !ok {
		log.Debug().Msg("RefreshScripts: webview is not *cef.WebView")
		return nil
	}

	ci.onLoadEnd(cefWV)
	return nil
}

// onLoadEnd is called from the load handler after a page finishes loading.
// It injects the appropriate scripts based on whether the page is internal.
func (ci *contentInjector) onLoadEnd(wv *WebView) {
	uri := wv.URI()
	isInternal := isConceptualInternalURL(uri) || isActualInternalURL(uri)

	ci.mu.RLock()
	themeCSS := ci.themeCSS
	findCSS := ci.findHighlightCSS
	ci.mu.RUnlock()

	// Internal pages get dark mode + message bridge + theme CSS.
	if isInternal {
		prefersDark := false
		if ci.colorResolver != nil {
			prefersDark = ci.colorResolver.Resolve().PrefersDark
		}
		ci.injectDarkModeScript(wv, prefersDark)
		ci.injectMessageBridgeShim(wv)
		if themeCSS != "" {
			ci.injectCSS(wv, "dumber-theme-vars", themeCSS)
		}
	}

	// All pages get find highlight CSS if set.
	if findCSS != "" {
		ci.injectCSS(wv, "dumber-find-highlight", findCSS)
	}

	// All pages get custom scrollbar styling with auto-hide.
	ci.injectCSS(wv, "dumber-scrollbar", scrollbarCSS)
	wv.RunJavaScript(context.Background(), scrollbarAutoHideJS)

	// All pages get clipboard copy bridge (CEF OSR can't write to Wayland clipboard).
	wv.RunJavaScript(context.Background(), clipboardCopyBridgeJS)

	// Video playback diagnostic — logs video element state changes.
	// Gated behind DUMBER_VIDEO_DIAG=1 environment variable.
	if ci.videoDiagnosticsEnabled {
		wv.RunJavaScript(context.Background(), videoDiagnosticJS)
	}

	if strings.Contains(uri, "reddit.com") {
		wv.RunJavaScript(context.Background(), redditDirectVideoJS)
	}
}

// injectCSS injects a CSS string as a <style> element via JavaScript.
func (ci *contentInjector) injectCSS(wv *WebView, id, css string) {
	escapedID := escapeForJSString(id)
	escaped := escapeForJSString(css)
	script := fmt.Sprintf(`(function(){
  var el = document.getElementById('%s');
  if (!el) { el = document.createElement('style'); el.id = '%s'; document.head.appendChild(el); }
  el.textContent = '%s';
})();`, escapedID, escapedID, escaped)

	wv.RunJavaScript(context.Background(), script)
}

// injectDarkModeScript sets dark/light class on <html> and patches matchMedia
// for prefers-color-scheme queries on internal pages.
func (ci *contentInjector) injectDarkModeScript(wv *WebView, prefersDark bool) {
	script := fmt.Sprintf(`(function() {
  var prefersDark = %t;
  window.__dumber_cef_prefers_dark = prefersDark;
  var originalMatchMedia = window.matchMedia.bind(window);

  if (prefersDark) {
    document.documentElement.classList.add('dark');
    document.documentElement.classList.remove('light');
  } else {
    document.documentElement.classList.add('light');
    document.documentElement.classList.remove('dark');
  }

  function isColorSchemeQuery(query, scheme) {
    if (typeof query !== 'string') return false;
    var normalized = query.replace(/\s+/g, '').toLowerCase();
    return normalized.indexOf('prefers-color-scheme:' + scheme) !== -1;
  }

  function createFakeMediaQueryList(query, matches) {
    var listeners = [];
    var onchangeHandler = null;
    return {
      matches: matches,
      media: query,
      get onchange() { return onchangeHandler; },
      set onchange(fn) { onchangeHandler = fn; },
      addListener: function(cb) { if (typeof cb === 'function') listeners.push(cb); },
      removeListener: function(cb) { var idx = listeners.indexOf(cb); if (idx !== -1) listeners.splice(idx, 1); },
      addEventListener: function(type, cb) { if (type === 'change' && typeof cb === 'function') listeners.push(cb); },
      removeEventListener: function(type, cb) {
        if (type === 'change') { var idx = listeners.indexOf(cb); if (idx !== -1) listeners.splice(idx, 1); }
      },
      dispatchEvent: function(event) {
        for (var i = 0; i < listeners.length; i++) { try { listeners[i](event); } catch (e) {} }
        if (onchangeHandler) { try { onchangeHandler(event); } catch (e) {} }
        return true;
      }
    };
  }

  window.matchMedia = function(query) {
    if (isColorSchemeQuery(query, 'dark')) return createFakeMediaQueryList(query, prefersDark);
    if (isColorSchemeQuery(query, 'light')) return createFakeMediaQueryList(query, !prefersDark);
    return originalMatchMedia(query);
  };
})();`, prefersDark)

	wv.RunJavaScript(context.Background(), script)
}

// injectMessageBridgeShim injects the window.dumber.postMessage JS client shim
// so internal pages can communicate with Go handlers via fetch.
// The message body is base64-encoded into the X-Dumber-Body header to work
// around purego-cef's unexported PostData element wrapper.
func (ci *contentInjector) injectMessageBridgeShim(wv *WebView) {
	wv.RunJavaScript(context.Background(), MessageBridgeJS)
}

// escapeForJSString escapes a string for use inside a JS single-quoted string literal.
func escapeForJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\u2028", "\\u2028")
	s = strings.ReplaceAll(s, "\u2029", "\\u2029")
	return s
}
