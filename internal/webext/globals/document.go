package globals

import (
	"fmt"
	"log"
	"strings"

	"github.com/grafana/sobek"
)

// installDocument creates the document object as instance of HTMLDocument.
func (bg *BrowserGlobals) installDocument() error {
	vm := bg.vm

	// Create document as instance of HTMLDocument using JavaScript
	_, err := vm.RunString(`
(function() {
	// Helper to create element-like objects
	function createElementLike(tagName, proto) {
		var el = Object.create(proto || HTMLElement.prototype);
		var attrs = {};
		var listeners = {};
		var children = [];

		el.tagName = tagName.toUpperCase();
		el.nodeName = tagName.toUpperCase();
		el.nodeType = 1;
		el.childNodes = children;
		el.children = children;
		el.firstChild = null;
		el.lastChild = null;
		el.parentNode = null;
		el.parentElement = null;

		el.getAttribute = function(name) { return attrs[name] || null; };
		el.setAttribute = function(name, value) { attrs[name] = String(value); };
		el.removeAttribute = function(name) { delete attrs[name]; };
		el.hasAttribute = function(name) { return name in attrs; };

		el.appendChild = function(child) { children.push(child); return child; };
		el.removeChild = function(child) { return child; };
		el.insertBefore = function(newNode, refNode) { children.push(newNode); return newNode; };
		el.replaceChild = function(newNode, oldNode) { return oldNode; };
		el.cloneNode = function(deep) { return createElementLike(tagName, proto); };

		el.addEventListener = function(type, fn) {
			if (!listeners[type]) listeners[type] = [];
			listeners[type].push(fn);
		};
		el.removeEventListener = function(type, fn) {
			if (listeners[type]) {
				listeners[type] = listeners[type].filter(function(f) { return f !== fn; });
			}
		};
		el.dispatchEvent = function(evt) {
			var type = evt.type;
			if (listeners[type]) {
				listeners[type].forEach(function(fn) { fn(evt); });
			}
			return true;
		};

		el.querySelector = function() { return null; };
		el.querySelectorAll = function() { return []; };
		el.getElementsByTagName = function() { return []; };
		el.getElementsByClassName = function() { return []; };

		el.classList = {
			_classes: {},
			add: function() { for (var i = 0; i < arguments.length; i++) this._classes[arguments[i]] = true; },
			remove: function() { for (var i = 0; i < arguments.length; i++) delete this._classes[arguments[i]]; },
			contains: function(c) { return !!this._classes[c]; },
			toggle: function(c) { if (this._classes[c]) { delete this._classes[c]; return false; } this._classes[c] = true; return true; }
		};

		el.style = {
			_props: {},
			setProperty: function(name, value) { this._props[name] = value; },
			getPropertyValue: function(name) { return this._props[name] || ''; },
			removeProperty: function(name) { delete this._props[name]; }
		};

		return el;
	}

	var doc = Object.create(HTMLDocument.prototype);

	doc.head = createElementLike('head', HTMLHeadElement.prototype);
	doc.body = createElementLike('body', HTMLBodyElement.prototype);
	doc.documentElement = createElementLike('html', HTMLHtmlElement.prototype);

	doc.readyState = "complete";
	doc.contentType = "text/html";
	doc.title = "uBlock Origin Background Page";
	doc.addEventListener = function() {};
	doc.removeEventListener = function() {};
	doc.dispatchEvent = function() { return true; };
	doc.location = { href: "about:blank", protocol: "about:", host: "", hostname: "", pathname: "blank" };
	doc.querySelector = function() { return null; };
	doc.querySelectorAll = function() { return []; };
	doc.getElementById = function() { return null; };
	doc.getElementsByTagName = function(tag) {
		if (tag.toLowerCase() === 'head') return [doc.head];
		if (tag.toLowerCase() === 'body') return [doc.body];
		return [];
	};
	doc.getElementsByClassName = function() { return []; };
	doc.createTextNode = function(text) {
		var node = Object.create(Node.prototype);
		node.nodeType = 3;
		node.textContent = text;
		node.nodeValue = text;
		return node;
	};
	doc.createDocumentFragment = function() {
		var frag = Object.create(Node.prototype);
		frag.nodeType = 11;
		frag.childNodes = [];
		frag.appendChild = function(child) { this.childNodes.push(child); return child; };
		return frag;
	};
	doc.createComment = function(text) {
		var node = Object.create(Node.prototype);
		node.nodeType = 8;
		node.textContent = text;
		return node;
	};

	globalThis.__document = doc;
	globalThis.__createElementLike = createElementLike;
})();
`)
	if err != nil {
		return fmt.Errorf("create document base: %w", err)
	}

	docVal := vm.Get("__document")
	if docVal == nil {
		return fmt.Errorf("document not created")
	}
	document := docVal.ToObject(vm)

	// Set head.appendChild to our Go function for script loading
	head := document.Get("head").ToObject(vm)
	_ = head.Set("appendChild", bg.documentHeadAppendChild)

	// document.currentScript - dynamically returns current script info
	_ = document.DefineAccessorProperty("currentScript", vm.ToValue(func(sobek.FunctionCall) sobek.Value {
		if bg.currentScriptURL == "" {
			return sobek.Null()
		}
		script := vm.NewObject()
		_ = script.Set("src", bg.currentScriptURL)
		_ = script.Set("tagName", "SCRIPT")
		_ = script.Set("nodeName", "SCRIPT")
		_ = script.Set("nodeType", 1) // Element node
		_ = script.Set("type", "text/javascript")
		return script
	}), nil, sobek.FLAG_FALSE, sobek.FLAG_TRUE)

	// document.createElement - use our Go implementation
	_ = document.Set("createElement", bg.documentCreateElement)

	vm.Set("document", document)
	_, _ = vm.RunString("delete globalThis.__document;")

	return nil
}

// documentCreateElement creates DOM elements as instances of proper classes.
func (bg *BrowserGlobals) documentCreateElement(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return sobek.Undefined()
	}

	tagName := strings.ToLower(call.Arguments[0].String())
	vm := bg.vm

	// Map tag to constructor
	constructorName := "HTMLElement"
	switch tagName {
	case "div":
		constructorName = "HTMLDivElement"
	case "script":
		constructorName = "HTMLScriptElement"
	case "span":
		constructorName = "HTMLSpanElement"
	case "a":
		constructorName = "HTMLAnchorElement"
	case "img":
		constructorName = "HTMLImageElement"
	case "input":
		constructorName = "HTMLInputElement"
	case "form":
		constructorName = "HTMLFormElement"
	case "button":
		constructorName = "HTMLButtonElement"
	case "canvas":
		constructorName = "HTMLCanvasElement"
	case "style":
		constructorName = "HTMLStyleElement"
	case "link":
		constructorName = "HTMLLinkElement"
	case "textarea":
		constructorName = "HTMLTextAreaElement"
	case "select":
		constructorName = "HTMLSelectElement"
	case "option":
		constructorName = "HTMLOptionElement"
	case "table":
		constructorName = "HTMLTableElement"
	case "tr":
		constructorName = "HTMLTableRowElement"
	case "td", "th":
		constructorName = "HTMLTableCellElement"
	case "iframe":
		constructorName = "HTMLIFrameElement"
	case "video":
		constructorName = "HTMLVideoElement"
	case "audio":
		constructorName = "HTMLAudioElement"
	}

	result, err := vm.RunString(fmt.Sprintf("Object.create(%s.prototype)", constructorName))
	if err != nil {
		result = vm.ToValue(vm.NewObject())
	}
	element := result.ToObject(vm)

	_ = element.Set("tagName", strings.ToUpper(tagName))
	_ = element.Set("nodeName", strings.ToUpper(tagName))
	_ = element.Set("nodeType", 1)

	// Element properties
	var srcValue, textContent, innerHTML string
	attributes := make(map[string]string)
	eventListeners := make(map[string][]sobek.Callable)
	var children []sobek.Value

	_ = element.DefineAccessorProperty("src",
		vm.ToValue(func(sobek.FunctionCall) sobek.Value { return vm.ToValue(srcValue) }),
		vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				srcValue = call.Arguments[0].String()
			}
			return sobek.Undefined()
		}),
		sobek.FLAG_FALSE, sobek.FLAG_TRUE)

	_ = element.DefineAccessorProperty("textContent",
		vm.ToValue(func(sobek.FunctionCall) sobek.Value { return vm.ToValue(textContent) }),
		vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				textContent = call.Arguments[0].String()
			}
			return sobek.Undefined()
		}),
		sobek.FLAG_FALSE, sobek.FLAG_TRUE)

	_ = element.DefineAccessorProperty("innerHTML",
		vm.ToValue(func(sobek.FunctionCall) sobek.Value { return vm.ToValue(innerHTML) }),
		vm.ToValue(func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				innerHTML = call.Arguments[0].String()
			}
			return sobek.Undefined()
		}),
		sobek.FLAG_FALSE, sobek.FLAG_TRUE)

	_ = element.Set("getAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			if v, ok := attributes[call.Arguments[0].String()]; ok {
				return vm.ToValue(v)
			}
		}
		return sobek.Null()
	})

	_ = element.Set("setAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 2 {
			attributes[call.Arguments[0].String()] = call.Arguments[1].String()
		}
		return sobek.Undefined()
	})

	_ = element.Set("removeAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			delete(attributes, call.Arguments[0].String())
		}
		return sobek.Undefined()
	})

	_ = element.Set("hasAttribute", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			_, ok := attributes[call.Arguments[0].String()]
			return vm.ToValue(ok)
		}
		return vm.ToValue(false)
	})

	_ = element.Set("addEventListener", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 2 {
			eventType := call.Arguments[0].String()
			if cb, ok := sobek.AssertFunction(call.Arguments[1]); ok {
				eventListeners[eventType] = append(eventListeners[eventType], cb)
			}
		}
		return sobek.Undefined()
	})

	_ = element.Set("removeEventListener", func(sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	_ = element.Set("dispatchEvent", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			evt := call.Arguments[0].ToObject(vm)
			eventType := evt.Get("type").String()
			if cbs, ok := eventListeners[eventType]; ok {
				for _, cb := range cbs {
					_, _ = cb(element, evt)
				}
			}
		}
		return vm.ToValue(true)
	})

	_ = element.Set("appendChild", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			children = append(children, call.Arguments[0])
		}
		return call.Arguments[0]
	})

	_ = element.Set("removeChild", func(call sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	_ = element.Set("cloneNode", func(call sobek.FunctionCall) sobek.Value {
		return bg.documentCreateElement(sobek.FunctionCall{Arguments: []sobek.Value{vm.ToValue(tagName)}})
	})

	_ = element.Set("querySelector", func(sobek.FunctionCall) sobek.Value { return sobek.Null() })
	_ = element.Set("querySelectorAll", func(sobek.FunctionCall) sobek.Value { return vm.ToValue([]interface{}{}) })
	_ = element.Set("parentNode", sobek.Null())
	_ = element.Set("parentElement", sobek.Null())
	_ = element.Set("childNodes", children)
	_ = element.Set("children", children)
	_ = element.Set("firstChild", sobek.Null())
	_ = element.Set("lastChild", sobek.Null())
	_ = element.Set("nextSibling", sobek.Null())
	_ = element.Set("previousSibling", sobek.Null())

	// classList
	classList := vm.NewObject()
	classes := make(map[string]bool)
	_ = classList.Set("add", func(call sobek.FunctionCall) sobek.Value {
		for _, arg := range call.Arguments {
			classes[arg.String()] = true
		}
		return sobek.Undefined()
	})
	_ = classList.Set("remove", func(call sobek.FunctionCall) sobek.Value {
		for _, arg := range call.Arguments {
			delete(classes, arg.String())
		}
		return sobek.Undefined()
	})
	_ = classList.Set("contains", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			return vm.ToValue(classes[call.Arguments[0].String()])
		}
		return vm.ToValue(false)
	})
	_ = classList.Set("toggle", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			name := call.Arguments[0].String()
			if classes[name] {
				delete(classes, name)
				return vm.ToValue(false)
			}
			classes[name] = true
			return vm.ToValue(true)
		}
		return vm.ToValue(false)
	})
	_ = element.Set("classList", classList)

	// style
	style := vm.NewObject()
	styleProps := make(map[string]string)
	_ = style.Set("setProperty", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) >= 2 {
			styleProps[call.Arguments[0].String()] = call.Arguments[1].String()
		}
		return sobek.Undefined()
	})
	_ = style.Set("getPropertyValue", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			return vm.ToValue(styleProps[call.Arguments[0].String()])
		}
		return vm.ToValue("")
	})
	_ = element.Set("style", style)

	// Script-specific properties
	if tagName == "script" {
		_ = element.Set("__isScript", true)
		_ = element.Set("__getSrc", func(sobek.FunctionCall) sobek.Value {
			return vm.ToValue(srcValue)
		})
		_ = element.Set("__triggerEvent", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) > 0 {
				eventType := call.Arguments[0].String()
				if listeners, ok := eventListeners[eventType]; ok {
					for _, cb := range listeners {
						_, _ = cb(element)
					}
				}
			}
			return sobek.Undefined()
		})
	}

	return element
}

// documentHeadAppendChild handles script element loading.
func (bg *BrowserGlobals) documentHeadAppendChild(call sobek.FunctionCall) sobek.Value {
	if len(call.Arguments) < 1 {
		return sobek.Undefined()
	}

	element := call.Arguments[0].ToObject(bg.vm)
	isScript := element.Get("__isScript")
	if isScript == nil || !isScript.ToBoolean() {
		return element
	}

	getSrc, ok := sobek.AssertFunction(element.Get("__getSrc"))
	if !ok {
		return element
	}
	srcVal, _ := getSrc(sobek.Undefined())
	src := srcVal.String()

	triggerEvent, ok := sobek.AssertFunction(element.Get("__triggerEvent"))
	if !ok {
		return element
	}

	// Resolve path
	var fullPath string
	if strings.HasPrefix(src, "dumb-extension://") || strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		if strings.HasPrefix(src, "dumb-extension://") {
			parts := strings.SplitN(src, "/", 4)
			if len(parts) >= 4 {
				fullPath = parts[3]
			}
		}
	} else {
		if bg.currentScriptDir != "" {
			fullPath = bg.currentScriptDir + "/" + src
		} else {
			fullPath = src
		}
	}

	// Load script synchronously to avoid deadlock with Promise-based loaders.
	// In a real browser, script loading is async, but we're running in a Sobek VM
	// where async script loading via tasks queue causes deadlock when JS code
	// awaits a Promise that depends on the load event.
	var loadErr error
	if bg.scriptLoader != nil {
		loadErr = bg.scriptLoader(fullPath)
	} else {
		loadErr = fmt.Errorf("no script loader configured")
	}

	// Fire load/error event synchronously
	if loadErr != nil {
		log.Printf("[browser-globals] Script load error %s: %v", fullPath, loadErr)
		_, _ = triggerEvent(sobek.Undefined(), bg.vm.ToValue("error"))
	} else {
		_, _ = triggerEvent(sobek.Undefined(), bg.vm.ToValue("load"))
	}

	return element
}
