package cef

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

const (
	rendererBridgeMessageName                = "dumber.renderer_bridge"
	rendererBridgeActionExplicitTextCopy     = "explicit_text_copy"
	rendererBridgeActionFocusSync            = "focus_sync"
	rendererBridgeActionEditableFocusChanged = "editable_focus_changed"
	rendererBridgeActionPopupOpen            = "popup_open"
	rendererBridgeActionPopupNavigate        = "popup_navigate"
	rendererBridgeActionReady                = "bridge_ready"
	rendererBridgeExtensionName              = "dumber.renderer_bridge"
)

var newRendererBridgeProcessMessage = purecef.ProcessMessageCreate

const rendererBridgeExtensionJS = `
(function() {
  native function Dispatch(action, payload);

  function send(action, payload) {
    return Dispatch(action, payload == null ? '' : String(payload));
  }

  function isEditable(node) {
    if (!node || node.nodeType !== 1) return false;
    if (node.isContentEditable) return true;
    var tag = node.tagName;
    if (tag !== 'INPUT' && tag !== 'TEXTAREA') return false;
    if (node.disabled || node.readOnly) return false;
    return true;
  }

  function getActiveElementSelection() {
    var el = document.activeElement;
    if (!el) return '';
    var tag = el.tagName;
    if ((tag === 'INPUT' || tag === 'TEXTAREA') && typeof el.value === 'string') {
      var start = typeof el.selectionStart === 'number' ? el.selectionStart : 0;
      var end = typeof el.selectionEnd === 'number' ? el.selectionEnd : 0;
      if (end > start) return el.value.slice(start, end);
    }
    return '';
  }

  function getSelectedText() {
    var activeSelection = getActiveElementSelection();
    if (activeSelection) return activeSelection;
    var sel = window.getSelection();
    return sel ? sel.toString() : '';
  }

  function notifyExplicitTextCopy(action, text) {
    var normalizedText = text == null ? '' : String(text);
    var normalizedAction = action == null || action === '' ? 'copy' : String(action);
    return send('explicit_text_copy', JSON.stringify({ text: normalizedText, action: normalizedAction }));
  }

  function resolvePopupURL(rawURL) {
    if (rawURL == null || rawURL === '') return '';
    try {
      return new URL(String(rawURL), (document && document.baseURI) || location.href).href;
    } catch (_) {
      return String(rawURL);
    }
  }

  function hasUserGesture() {
    try {
      return !!(navigator && navigator.userActivation && navigator.userActivation.isActive);
    } catch (_) {
      return false;
    }
  }

  function popupHasNoOpener(features) {
    if (features == null || features === '') return false;
    var normalized = String(features).toLowerCase();
    return normalized.indexOf('noopener') !== -1 || normalized.indexOf('noreferrer') !== -1;
  }

  function shouldDelegateWindowOpen(target) {
    return target === '_self' || target === '_top' || target === '_parent';
  }

  function createSyntheticPopupProxy(proxyID, initialURL, features) {
    var href = initialURL || 'about:blank';
    var closed = false;

    function navigate(nextURL) {
      href = resolvePopupURL(nextURL);
      send('popup_navigate', JSON.stringify({ proxy_id: proxyID, url: href }));
      return href;
    }

    var locationProxy = {
      assign: function(nextURL) { return navigate(nextURL); },
      replace: function(nextURL) { return navigate(nextURL); },
      toString: function() { return href; }
    };
    try {
      Object.defineProperty(locationProxy, 'href', {
        configurable: true,
        enumerable: true,
        get: function() { return href; },
        set: function(nextURL) { navigate(nextURL); }
      });
    } catch (_) {
      locationProxy.href = href;
    }

    var proxy = {
      blur: function() { return undefined; },
      close: function() { closed = true; },
      focus: function() { return undefined; },
      postMessage: function() { return undefined; }
    };
    proxy.opener = popupHasNoOpener(features) ? null : window;
    proxy.self = proxy;
    proxy.window = proxy;

    try {
      Object.defineProperty(proxy, 'closed', {
        configurable: true,
        enumerable: true,
        get: function() { return closed; },
        set: function(next) { closed = !!next; }
      });
    } catch (_) {
      proxy.closed = false;
    }

    try {
      Object.defineProperty(proxy, 'location', {
        configurable: true,
        enumerable: true,
        get: function() { return locationProxy; },
        set: function(nextURL) { navigate(nextURL); }
      });
    } catch (_) {
      proxy.location = locationProxy;
    }

    try {
      Object.defineProperty(proxy, Symbol.toStringTag, {
        configurable: true,
        value: 'Window'
      });
    } catch (_) {}

    return proxy;
  }

  if (typeof window !== 'undefined' && typeof window.open === 'function' && !window.__dumberPopupOpenPatched) {
    window.__dumberPopupOpenPatched = true;
    var originalWindowOpen = window.open;
    window.open = function(url, target, features) {
      var normalizedTarget = target == null ? '' : String(target);
      if (shouldDelegateWindowOpen(normalizedTarget)) {
        return originalWindowOpen.apply(this, arguments);
      }

      var proxyID = 'popup-' + Date.now() + '-' + Math.random().toString(36).slice(2);
      var resolvedURL = resolvePopupURL(url);
      var popupProxy = createSyntheticPopupProxy(proxyID, resolvedURL, features);
      send('popup_open', JSON.stringify({
        proxy_id: proxyID,
        url: resolvedURL,
        frame_name: normalizedTarget,
        user_gesture: hasUserGesture()
      }));
      return popupProxy;
    };
  }

  if (typeof document !== 'undefined' && document.addEventListener) {
    document.addEventListener('focusin', function(event) {
      if (isEditable(event.target)) {
        send('focus_sync', '');
      }
    }, true);

    function mirrorClipboardEvent(action, e) {
      if (!e.isTrusted) return;
      var capturedText = '';
      if (e.clipboardData && typeof e.clipboardData.getData === 'function') {
        try { capturedText = e.clipboardData.getData('text/plain'); } catch(_) {}
      }
      setTimeout(function() {
        var text = capturedText;
        if (!text && !e.defaultPrevented) text = getSelectedText();
        if (text) notifyExplicitTextCopy(action, text);
      }, 0);
    }

    document.addEventListener('copy', function(e) {
      mirrorClipboardEvent('copy', e);
    }, true);
    document.addEventListener('cut', function(e) {
      mirrorClipboardEvent('cut', e);
    }, true);
  }
  try {
    var clipboardObj = navigator && navigator.clipboard ? navigator.clipboard : null;
    var clipboardProto = (typeof Clipboard !== 'undefined' && Clipboard.prototype) ||
      (clipboardObj && typeof Object.getPrototypeOf === 'function' ? Object.getPrototypeOf(clipboardObj) : null);

    function mirrorClipboardItems(items) {
      try {
        if (!items || typeof items.length !== 'number') return;
        Array.prototype.forEach.call(items, function(item) {
          if (!item || !item.types || item.types.indexOf('text/plain') === -1 || typeof item.getType !== 'function') return;
          Promise.resolve(item.getType('text/plain'))
            .then(function(blob) {
              if (!blob || typeof blob.text !== 'function') return '';
              return blob.text();
            })
            .then(function(text) {
              if (text) notifyExplicitTextCopy('copy', text);
            })
            .catch(function() {});
        });
      } catch (_) {}
    }

    if (clipboardProto && typeof clipboardProto.writeText === 'function' && !clipboardProto.__dumberWriteTextPatched) {
      clipboardProto.__dumberWriteTextPatched = true;
      var originalProtoWriteText = clipboardProto.writeText;
      var wrappedProtoWriteText = function(text) {
        var normalized = text == null ? '' : String(text);
        var result = originalProtoWriteText.call(this, normalized);
        if (result && typeof result.then === 'function') {
          return result.then(function(value) {
            if (normalized) notifyExplicitTextCopy('copy', normalized);
            return value;
          });
        }
        if (normalized) notifyExplicitTextCopy('copy', normalized);
        return result;
      };
      try {
        Object.defineProperty(clipboardProto, 'writeText', {
          configurable: true,
          writable: true,
          value: wrappedProtoWriteText
        });
      } catch (_) {
        clipboardProto.writeText = wrappedProtoWriteText;
      }
    } else if (clipboardObj && typeof clipboardObj.writeText === 'function' && !window.__dumberClipboardWriteTextPatched) {
      window.__dumberClipboardWriteTextPatched = true;
      var originalWriteText = clipboardObj.writeText.bind(clipboardObj);
      var wrappedWriteText = function(text) {
        var normalized = text == null ? '' : String(text);
        var result = originalWriteText(normalized);
        if (result && typeof result.then === 'function') {
          return result.then(function(value) {
            if (normalized) notifyExplicitTextCopy('copy', normalized);
            return value;
          });
        }
        if (normalized) notifyExplicitTextCopy('copy', normalized);
        return result;
      };
      try {
        Object.defineProperty(clipboardObj, 'writeText', {
          configurable: true,
          writable: true,
          value: wrappedWriteText
        });
      } catch (_) {
        clipboardObj.writeText = wrappedWriteText;
      }
    }

    if (clipboardProto && typeof clipboardProto.write === 'function' && !clipboardProto.__dumberWritePatched) {
      clipboardProto.__dumberWritePatched = true;
      var originalProtoWrite = clipboardProto.write;
      var wrappedProtoWrite = function(items) {
        var result = originalProtoWrite.apply(this, arguments);
        if (result && typeof result.then === 'function') {
          return result.then(function(value) {
            mirrorClipboardItems(items);
            return value;
          });
        }
        mirrorClipboardItems(items);
        return result;
      };
      try {
        Object.defineProperty(clipboardProto, 'write', {
          configurable: true,
          writable: true,
          value: wrappedProtoWrite
        });
      } catch (_) {
        clipboardProto.write = wrappedProtoWrite;
      }
    } else if (clipboardObj && typeof clipboardObj.write === 'function' && !window.__dumberClipboardWritePatched) {
      window.__dumberClipboardWritePatched = true;
      var originalWrite = clipboardObj.write.bind(clipboardObj);
      var wrappedWrite = function(items) {
        var result = originalWrite(items);
        if (result && typeof result.then === 'function') {
          return result.then(function(value) {
            mirrorClipboardItems(items);
            return value;
          });
        }
        mirrorClipboardItems(items);
        return result;
      };
      try {
        Object.defineProperty(clipboardObj, 'write', {
          configurable: true,
          writable: true,
          value: wrappedWrite
        });
      } catch (_) {
        clipboardObj.write = wrappedWrite;
      }
    }
  } catch (_) {}

  send('bridge_ready', (typeof location !== 'undefined' && location && location.href) ? location.href : '');
})();
`

type rendererBridgeProcessHandler struct {
	registerOnce  sync.Once
	bridgeHandler purecef.V8Handler
}

func (h *rendererBridgeProcessHandler) ensureExtensionRegistered() {
	if h == nil {
		return
	}
	h.registerOnce.Do(func() {
		if h.bridgeHandler == nil {
			h.bridgeHandler = &rendererBridgeV8Handler{}
		}
		if result := purecef.RegisterExtension(rendererBridgeExtensionName, rendererBridgeExtensionJS, h.bridgeHandler); result != 1 {
			logging.FromContext(context.Background()).Error().Int32("result", result).Msg("cef: failed to register renderer bridge extension")
		}
	})
}

func (h *rendererBridgeProcessHandler) OnWebKitInitialized() {
	h.ensureExtensionRegistered()
}

func (*rendererBridgeProcessHandler) OnBrowserCreated(_ purecef.Browser, _ purecef.DictionaryValue) {}
func (*rendererBridgeProcessHandler) OnBrowserDestroyed(_ purecef.Browser)                          {}
func (*rendererBridgeProcessHandler) GetLoadHandler() purecef.LoadHandler                           { return nil }
func (*rendererBridgeProcessHandler) OnContextCreated(_ purecef.Browser, _ purecef.Frame, _ purecef.V8Context) {
}
func (*rendererBridgeProcessHandler) OnContextReleased(_ purecef.Browser, _ purecef.Frame, _ purecef.V8Context) {
}
func (*rendererBridgeProcessHandler) OnUncaughtException(
	_ purecef.Browser, _ purecef.Frame, _ purecef.V8Context, _ purecef.V8Exception, _ purecef.V8StackTrace,
) {
}
func (*rendererBridgeProcessHandler) OnFocusedNodeChanged(_ purecef.Browser, frame purecef.Frame, node purecef.Domnode) {
	if frame == nil {
		return
	}
	message := newRendererBridgeProcessMessage(rendererBridgeMessageName)
	if message == nil {
		return
	}
	args := message.GetArgumentList()
	if args == nil {
		return
	}
	args.SetString(0, rendererBridgeActionEditableFocusChanged)
	if node != nil && node.IsEditable() {
		args.SetString(1, "1")
	} else {
		args.SetString(1, "0")
	}
	frame.SendProcessMessage(purecef.ProcessIDPidBrowser, message)
}
func (*rendererBridgeProcessHandler) OnProcessMessageReceived(
	_ purecef.Browser, _ purecef.Frame, _ purecef.ProcessID, _ purecef.ProcessMessage,
) int32 {
	return 0
}

type rendererBridgeV8Handler struct{}

func (*rendererBridgeV8Handler) Execute(
	name string, _ purecef.V8Value, arguments []purecef.V8Value, _ unsafe.Pointer, _ uintptr,
) int32 {
	if name != "Dispatch" || len(arguments) < 2 {
		return 0
	}
	if arguments[0] == nil || arguments[1] == nil || !arguments[0].IsString() || !arguments[1].IsString() {
		return 0
	}
	action := arguments[0].GetStringValue()
	payload := arguments[1].GetStringValue()
	if action == "" {
		return 0
	}

	ctx := purecef.V8ContextGetCurrentContext()
	if ctx == nil || !ctx.IsValid() {
		return 0
	}
	frame := ctx.GetFrame()
	if frame == nil {
		return 0
	}
	message := purecef.ProcessMessageCreate(rendererBridgeMessageName)
	if message == nil {
		return 0
	}
	args := message.GetArgumentList()
	if args == nil {
		return 0
	}
	args.SetString(0, action)
	args.SetString(1, payload)
	frame.SendProcessMessage(purecef.ProcessIDPidBrowser, message)
	return 1
}

func decodeRendererBridgeProcessMessage(message purecef.ProcessMessage) (string, string, bool) {
	if message == nil || message.GetName() != rendererBridgeMessageName {
		return "", "", false
	}
	args := message.GetArgumentList()
	if args == nil || args.GetSize() < 2 {
		return "", "", false
	}
	action := args.GetString(0)
	payload := args.GetString(1)
	if action == "" {
		return "", "", false
	}
	return action, payload, true
}

type rendererBridgeExplicitTextCopyPayload struct {
	Text   string `json:"text"`
	Action string `json:"action"`
}

type rendererBridgePopupOpenPayload struct {
	ProxyID     string `json:"proxy_id"`
	URL         string `json:"url"`
	FrameName   string `json:"frame_name"`
	UserGesture bool   `json:"user_gesture"`
}

type rendererBridgePopupNavigatePayload struct {
	ProxyID string `json:"proxy_id"`
	URL     string `json:"url"`
}

func decodeRendererBridgeExplicitTextCopyPayload(payload []byte) (rendererBridgeExplicitTextCopyPayload, error) {
	var req rendererBridgeExplicitTextCopyPayload
	if len(payload) == 0 {
		return req, fmt.Errorf("empty payload")
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, err
	}
	if req.Action == "" {
		req.Action = "copy"
	}
	if req.Text == "" {
		return req, fmt.Errorf("missing text")
	}
	return req, nil
}

func decodeRendererBridgePopupOpenPayload(payload []byte) (rendererBridgePopupOpenPayload, error) {
	var req rendererBridgePopupOpenPayload
	if len(payload) == 0 {
		return req, fmt.Errorf("empty payload")
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, err
	}
	if req.ProxyID == "" {
		return req, fmt.Errorf("missing proxy_id")
	}
	return req, nil
}

func decodeRendererBridgePopupNavigatePayload(payload []byte) (rendererBridgePopupNavigatePayload, error) {
	var req rendererBridgePopupNavigatePayload
	if len(payload) == 0 {
		return req, fmt.Errorf("empty payload")
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, err
	}
	if req.ProxyID == "" {
		return req, fmt.Errorf("missing proxy_id")
	}
	return req, nil
}

func (e *Engine) handleEditableFocusBridge(browser purecef.Browser) {
	logger := logging.FromContext(e.currentContext())
	task := purecef.NewTask(cefTaskFunc(func() {
		if browser == nil {
			logger.Debug().Msg("cef: editable focus sync skipped — browser nil")
			return
		}
		host := browser.GetHost()
		if host == nil {
			logger.Debug().Msg("cef: editable focus sync skipped — host nil")
			return
		}
		syncWindowlessBrowserFocus(host)
		logger.Debug().Msg("cef: editable focus synchronized")
	}))
	if result := purecef.PostTask(purecef.ThreadIDTidUi, task); result != 1 {
		logger.Warn().Int32("result", result).Msg("cef: failed to post editable focus sync to UI thread")
	}
}
