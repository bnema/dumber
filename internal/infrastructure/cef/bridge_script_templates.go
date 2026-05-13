package cef

import (
	"strings"

	"github.com/bnema/dumber/internal/infrastructure/webutil"
)

// trustedPageFetchBridgeBaseJS is the invariant fetch-bridge bundle injected on
// external pages. It is assembled once from shared templates and then only the
// bridge nonce placeholder is replaced per navigation.
var trustedPageFetchBridgeBaseJS = buildTrustedPageFetchBridgeBaseJS()

// rendererBridgeExtensionJS is assembled once for renderer-process bridge helpers.
var rendererBridgeExtensionJS = buildRendererBridgeExtensionJS()

func buildTrustedPageFetchBridgeBaseJS() string {
	return clipboardSelectionFetchBridgeJSTemplate
}

const bridgeNoncePlaceholder = "__DUMBER_BRIDGE_NONCE__"

func buildTrustedPageFetchBridgeJS(bridgeNonce string) string {
	if strings.Contains(bridgeNonce, bridgeNoncePlaceholder) {
		panic("bridge nonce contains reserved placeholder")
	}
	return strings.ReplaceAll(trustedPageFetchBridgeBaseJS, bridgeNoncePlaceholder, bridgeNonce)
}

func buildRendererBridgeExtensionJS() string {
	return strings.ReplaceAll(rendererBridgeExtensionTemplateJS, "__DUMBER_POPUP_PROXY_BRIDGE_BODY__", "")
}

func buildPopupOpenerBridgeJS(bridgeNonce, parentURI string) string {
	replacer := strings.NewReplacer(
		"__DUMBER_BRIDGE_NONCE__", webutil.EscapeForJSString(bridgeNonce),
		"__DUMBER_PARENT_URI__", webutil.EscapeForJSString(parentURI),
	)
	return replacer.Replace(popupOpenerBridgeTemplateJS)
}

// clipboardSelectionFetchBridgeJSTemplate keeps the lightweight fetch bridge
// enabled while the native renderer bridge remains disabled due to an OSR
// startup regression. It mirrors explicit clipboard text from DOM selections,
// safe input/textarea selections, and async Clipboard API writes, and also
// reasserts browser focus when an editable element gains DOM focus.
const clipboardSelectionFetchBridgeJSTemplate = `(function(){
  var bridgeNonce = '__DUMBER_BRIDGE_NONCE__';

  function encodeBody(body) {
    return typeof btoa === 'function' ? btoa(unescape(encodeURIComponent(body))) : '';
  }

  function inputType(node) {
    return node && node.type ? String(node.type).toLowerCase() : 'text';
  }

  function isTextEditableInputType(type) {
    switch ((type || 'text').toLowerCase()) {
    case 'button':
    case 'submit':
    case 'reset':
    case 'checkbox':
    case 'radio':
    case 'range':
    case 'color':
    case 'file':
    case 'image':
    case 'hidden':
      return false;
    default:
      return true;
    }
  }

  function isSelectionReadableInputType(type) {
    return isTextEditableInputType(type) && type !== 'password';
  }

  function getActiveElementSelection() {
    var el = document.activeElement;
    if (!el || el.nodeType !== 1 || typeof el.value !== 'string') return '';
    var tag = el.tagName;
    if (el.disabled || el.readOnly) return '';
    if (tag === 'INPUT' && !isSelectionReadableInputType(inputType(el))) return '';
    if (tag !== 'INPUT' && tag !== 'TEXTAREA') return '';
    var start = typeof el.selectionStart === 'number' ? el.selectionStart : 0;
    var end = typeof el.selectionEnd === 'number' ? el.selectionEnd : 0;
    if (end > start) return el.value.slice(start, end);
    return '';
  }

  function getSelectedText() {
    var activeSelection = getActiveElementSelection();
    if (activeSelection) return activeSelection;
    var sel = window.getSelection();
    return sel ? sel.toString() : '';
  }

  function isEditable(node) {
    if (!node || node.nodeType !== 1) return false;
    if (node.isContentEditable) return true;
    var tag = node.tagName;
    if (tag === 'TEXTAREA') return !node.disabled && !node.readOnly;
    if (tag !== 'INPUT') return false;
    if (node.disabled || node.readOnly) return false;
    return isTextEditableInputType(inputType(node));
  }

  function sendToClipboard(text) {
    if (!text) return;
    var body = JSON.stringify({text: text});
    var encoded = encodeBody(body);
    if (!encoded) return;
    fetch('dumb:///api/clipboard-set', {
      method: 'POST',
      headers: {
        'X-Dumber-Body': encoded,
        'X-Dumber-Bridge-Nonce': bridgeNonce
      }
    }).catch(function(){});
  }

  function sendFocusSync() {
    fetch('dumb:///api/focus-sync', {
      method: 'POST',
      headers: {
        'X-Dumber-Bridge-Action': 'focus-sync',
        'X-Dumber-Bridge-Nonce': bridgeNonce
      }
    }).catch(function(){});
  }

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
            if (text) sendToClipboard(text);
          })
          .catch(function(){});
      });
    } catch (_) {}
  }

  document.addEventListener('copy', function(event) {
    if (event && event.isTrusted === false) return;
    var text = getSelectedText();
    if (text) sendToClipboard(text);
  });
  document.addEventListener('cut', function(event) {
    if (event && event.isTrusted === false) return;
    var text = getSelectedText();
    if (text) sendToClipboard(text);
  });
  document.addEventListener('focusin', function(event) {
    if (event && event.isTrusted === false) return;
    if (isEditable(event && event.target)) sendFocusSync();
  }, true);

  try {
    var clipboardObj = navigator && navigator.clipboard ? navigator.clipboard : null;
    var clipboardProto = (typeof Clipboard !== 'undefined' && Clipboard.prototype) ||
      (clipboardObj && typeof Object.getPrototypeOf === 'function' ? Object.getPrototypeOf(clipboardObj) : null);

    if (clipboardProto && typeof clipboardProto.writeText === 'function' && !clipboardProto.__dumberWriteTextPatched) {
      clipboardProto.__dumberWriteTextPatched = true;
      var originalProtoWriteText = clipboardProto.writeText;
      var wrappedProtoWriteText = function(text) {
        var normalized = text == null ? '' : String(text);
        var result = originalProtoWriteText.call(this, normalized);
        if (result && typeof result.then === 'function') {
          return result.then(function(value) {
            if (normalized) sendToClipboard(normalized);
            return value;
          });
        }
        if (normalized) sendToClipboard(normalized);
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
            if (normalized) sendToClipboard(normalized);
            return value;
          });
        }
        if (normalized) sendToClipboard(normalized);
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

  if (isEditable(document.activeElement)) sendFocusSync();
})();`

const rendererBridgeExtensionTemplateJS = `
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

__DUMBER_POPUP_PROXY_BRIDGE_BODY__

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

const popupOpenerBridgeTemplateJS = `(function() {
  if (typeof window === 'undefined') return;
  if (window.__dumberPopupOpenerBridgeInstalled) return;
  window.__dumberPopupOpenerBridgeInstalled = true;
  if (window.opener != null) return;

  var bridgeNonce = '__DUMBER_BRIDGE_NONCE__';
  var openerHref = '__DUMBER_PARENT_URI__';

  function postBridge(path, payload) {
    try {
      fetch('dumb:///api/' + path, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Dumber-Body': btoa(unescape(encodeURIComponent(JSON.stringify(payload)))),
          'X-Dumber-Bridge-Nonce': bridgeNonce
        }
      }).catch(function() {});
    } catch (_) {}
  }

  function normalizeURLValue(rawURL) {
    if (rawURL == null) return '';
    try {
      return String(rawURL);
    } catch (_) {
      return '';
    }
  }

  function navigateOpener(nextURL) {
    var rawURL = normalizeURLValue(nextURL);
    if (rawURL !== '') openerHref = rawURL;
    postBridge('popup-opener-navigate', { url: rawURL });
    return openerHref;
  }

  function serializeMessage(value) {
    try {
      return { kind: 'json', value: JSON.stringify(value) };
    } catch (_) {
      return { kind: 'string', value: String(value) };
    }
  }

  var locationProxy = {
    assign: function(nextURL) { return navigateOpener(nextURL); },
    replace: function(nextURL) { return navigateOpener(nextURL); },
    toString: function() { return openerHref; }
  };
  try {
    Object.defineProperty(locationProxy, 'href', {
      configurable: true,
      enumerable: true,
      get: function() { return openerHref; },
      set: function(nextURL) { navigateOpener(nextURL); }
    });
  } catch (_) {
    locationProxy.href = openerHref;
  }

  var openerProxy = {
    blur: function() { return undefined; },
    close: function() { return undefined; },
    focus: function() { return undefined; },
    postMessage: function(message, targetOrigin) {
      var serialized = serializeMessage(message);
      postBridge('popup-opener-post-message', {
        data: serialized.value,
        data_kind: serialized.kind,
        target_origin: targetOrigin == null ? '*' : String(targetOrigin),
        source_origin: (typeof location !== 'undefined' && location && location.origin) ? location.origin : '',
        source_href: (typeof location !== 'undefined' && location && location.href) ? location.href : ''
      });
      return undefined;
    }
  };
  openerProxy.self = openerProxy;
  openerProxy.window = openerProxy;
  try {
    Object.defineProperty(openerProxy, 'closed', {
      configurable: true,
      enumerable: true,
      get: function() { return false; }
    });
  } catch (_) {
    openerProxy.closed = false;
  }
  try {
    Object.defineProperty(openerProxy, 'location', {
      configurable: true,
      enumerable: true,
      get: function() { return locationProxy; },
      set: function(nextURL) { navigateOpener(nextURL); }
    });
  } catch (_) {
    openerProxy.location = locationProxy;
  }

  try {
    Object.defineProperty(window, 'opener', {
      configurable: true,
      enumerable: true,
      get: function() { return openerProxy; }
    });
  } catch (_) {
    try { window.opener = openerProxy; } catch (_) {}
  }
})();`
