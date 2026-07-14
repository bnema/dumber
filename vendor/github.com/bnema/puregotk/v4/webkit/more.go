package webkit

import (
	"fmt"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/puregotk/pkg/core"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gobject"
)

// WebContextOptions contains construct-only properties for creating a WebContext.
type WebContextOptions struct {
	MemoryPressureSettings *MemoryPressureSettings
	TimeZoneOverride       string
}

// NewWebContextWithOptions creates a new WebContext with the specified construct-only properties.
func NewWebContextWithOptions(opts *WebContextOptions) *WebContext {
	if opts == nil {
		return NewWebContext()
	}

	var names []string
	var values []gobject.Value

	if opts.MemoryPressureSettings != nil {
		var v gobject.Value
		v.Init(MemoryPressureSettingsGLibType())
		v.SetBoxed(opts.MemoryPressureSettings.GoPointer())
		names = append(names, "memory-pressure-settings")
		values = append(values, v)
	}

	if opts.TimeZoneOverride != "" {
		var v gobject.Value
		v.Init(gobject.TypeStringVal)
		v.SetString(&opts.TimeZoneOverride)
		names = append(names, "time-zone-override")
		values = append(values, v)
	}

	if len(names) == 0 {
		return NewWebContext()
	}

	obj := gobject.NewObjectWithProperties(
		WebContextGLibType(),
		uint(len(names)),
		names,
		values,
	)
	if obj == nil {
		return nil
	}

	ctx := &WebContext{}
	ctx.Ptr = obj.Ptr
	return ctx
}

// NewWebContextWithMemoryPressureSettings creates a new WebContext with memory pressure settings.
func NewWebContextWithMemoryPressureSettings(settings *MemoryPressureSettings) *WebContext {
	if settings == nil {
		return NewWebContext()
	}
	return NewWebContextWithOptions(&WebContextOptions{MemoryPressureSettings: settings})
}

// WebViewOptions contains construct-only properties for creating a WebView.
type WebViewOptions struct {
	WebContext                   *WebContext
	NetworkSession               *NetworkSession
	UserContentManager           *UserContentManager
	RelatedView                  *WebView
	WebsitePolicies              *WebsitePolicies
	DefaultContentSecurityPolicy string
	IsControlledByAutomation     bool
	AutomationPresentationType   AutomationBrowsingContextPresentation
}

// NewWebViewWithOptions creates a new WebView with the specified construct-only properties.
func NewWebViewWithOptions(opts *WebViewOptions) *WebView {
	if opts == nil {
		return NewWebView()
	}

	var names []string
	var values []gobject.Value

	if opts.WebContext != nil {
		var v gobject.Value
		v.Init(WebContextGLibType())
		obj := gobject.Object{Ptr: opts.WebContext.GoPointer()}
		v.SetObject(&obj)
		names = append(names, "web-context")
		values = append(values, v)
	}

	if opts.NetworkSession != nil {
		var v gobject.Value
		v.Init(NetworkSessionGLibType())
		obj := gobject.Object{Ptr: opts.NetworkSession.GoPointer()}
		v.SetObject(&obj)
		names = append(names, "network-session")
		values = append(values, v)
	}

	if opts.UserContentManager != nil {
		var v gobject.Value
		v.Init(UserContentManagerGLibType())
		obj := gobject.Object{Ptr: opts.UserContentManager.GoPointer()}
		v.SetObject(&obj)
		names = append(names, "user-content-manager")
		values = append(values, v)
	}

	if opts.RelatedView != nil {
		var v gobject.Value
		v.Init(WebViewGLibType())
		obj := gobject.Object{Ptr: opts.RelatedView.GoPointer()}
		v.SetObject(&obj)
		names = append(names, "related-view")
		values = append(values, v)
	}

	if opts.WebsitePolicies != nil {
		var v gobject.Value
		v.Init(WebsitePoliciesGLibType())
		obj := gobject.Object{Ptr: opts.WebsitePolicies.GoPointer()}
		v.SetObject(&obj)
		names = append(names, "website-policies")
		values = append(values, v)
	}

	if opts.DefaultContentSecurityPolicy != "" {
		var v gobject.Value
		v.Init(gobject.TypeStringVal)
		v.SetString(&opts.DefaultContentSecurityPolicy)
		names = append(names, "default-content-security-policy")
		values = append(values, v)
	}

	if opts.IsControlledByAutomation {
		var v gobject.Value
		v.Init(gobject.TypeBooleanVal)
		v.SetBoolean(true)
		names = append(names, "is-controlled-by-automation")
		values = append(values, v)

		var vType gobject.Value
		vType.Init(AutomationBrowsingContextPresentationGLibType())
		vType.SetEnum(int(opts.AutomationPresentationType))
		names = append(names, "automation-presentation-type")
		values = append(values, vType)
	}

	if len(names) == 0 {
		return NewWebView()
	}

	obj := gobject.NewObjectWithProperties(
		WebViewGLibType(),
		uint(len(names)),
		names,
		values,
	)
	if obj == nil {
		return nil
	}

	gobject.IncreaseRef(obj.GoPointer())

	view := &WebView{}
	view.Ptr = obj.Ptr
	return view
}

// NewWebViewWithRelatedView creates a new WebView that shares the same context/session as the related view.
func NewWebViewWithRelatedView(relatedView *WebView) *WebView {
	if relatedView == nil {
		return NewWebView()
	}
	return NewWebViewWithOptions(&WebViewOptions{RelatedView: relatedView})
}

// NewWebViewWithNetworkSession creates a new WebView using the specified network session.
func NewWebViewWithNetworkSession(session *NetworkSession) *WebView {
	if session == nil {
		return NewWebView()
	}
	return NewWebViewWithOptions(&WebViewOptions{NetworkSession: session})
}

var lazyRegisterNavigationActionCopy = func() {
	core.LazyRegister(&xNavigationActionCopy, "WEBKIT", "webkit_navigation_action_copy", false)
}

// NavigationActionFromPointer wraps a raw pointer from the "create" signal into a copied NavigationAction.
func NavigationActionFromPointer(ptr uintptr) *NavigationAction {
	if ptr == 0 {
		return nil
	}
	lazyRegisterNavigationActionCopy()
	cret := xNavigationActionCopy(ptr)
	if cret == 0 {
		return nil
	}
	return (*NavigationAction)(unsafe.Pointer(cret))
}

// ConnectScriptMessageReceivedWithDetail connects to the script-message-received signal with a detail string.
func (x *UserContentManager) ConnectScriptMessageReceivedWithDetail(detail string, cb *func(UserContentManager, uintptr)) uint {
	cbPtr := uintptr(unsafe.Pointer(cb))
	signalName := fmt.Sprintf("script-message-received::%s", detail)
	if cbRefPtr, ok := glib.GetCallback(cbPtr); ok {
		handlerID := gobject.SignalConnect(x.GoPointer(), signalName, cbRefPtr)
		glib.SaveHandlerMapping(handlerID, cbPtr)
		return handlerID
	}

	fcb := func(clsPtr uintptr, valuePtr uintptr) {
		fa := UserContentManager{}
		fa.Ptr = clsPtr
		cbFn := *cb
		cbFn(fa, valuePtr)
	}
	cbRefPtr := purego.NewCallback(fcb)
	glib.SaveCallbackWithClosure(cbPtr, cbRefPtr, cb)
	handlerID := gobject.SignalConnect(x.GoPointer(), signalName, cbRefPtr)
	glib.SaveHandlerMapping(handlerID, cbPtr)
	return handlerID
}
