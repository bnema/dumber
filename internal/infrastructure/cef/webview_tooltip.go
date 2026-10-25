package cef

import (
	"unicode/utf16"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
)

// cefTooltipString mirrors cef_string_t. The storage belongs to CEF and is
// borrowed only for the duration of OnTooltip; never call its destructor.
type cefTooltipString struct {
	Str    *uint16
	Length uintptr
	Dtor   uintptr
}

func decodeCEFTooltipString(text uintptr) string {
	if text == 0 {
		return ""
	}
	value := (*cefTooltipString)(nativePointer(text))
	if value.Str == nil || value.Length == 0 {
		return ""
	}
	return string(utf16.Decode(unsafe.Slice(value.Str, value.Length)))
}

// nativePointer converts a native address that CEF passes as uintptr. CEF owns
// the memory and keeps it valid for the callback, so checkptr does not apply.
//
//go:nocheckptr
func nativePointer(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func (h *handlerSet) OnTooltip(browser purecef.Browser, text uintptr) int32 {
	if h == nil || h.wv == nil || browser == nil {
		return 0
	}
	wv := h.wv
	wv.mu.RLock()
	current, seq := wv.browser, wv.documentSeq
	wv.mu.RUnlock()
	if wv.destroyed.Load() || current == nil || browser.GetIdentifier() != current.GetIdentifier() {
		return 0
	}
	decoded := decodeCEFTooltipString(text)
	wv.runOnGTK(func() {
		wv.mu.RLock()
		current, currentSeq := wv.browser, wv.documentSeq
		wv.mu.RUnlock()
		if wv.destroyed.Load() || current == nil || currentSeq != seq || browser.GetIdentifier() != current.GetIdentifier() {
			return
		}
		wv.setTooltipOnGTK(decoded)
	})
	return 0
}

// setTooltipOnGTK uses the same widget as AttachInputToWidget. Its state and
// optional test seam are confined to the GTK dispatcher.
func (wv *WebView) setTooltipOnGTK(text string) {
	if wv == nil || wv.nativeWidget == nil || wv.tooltipText == text {
		return
	}
	var value *string
	if text != "" {
		value = &text
	}
	if wv.tooltipSetFunc != nil {
		wv.tooltipSetFunc(value)
	} else {
		wv.nativeWidget.SetTooltipText(value)
	}
	wv.tooltipText = text
}
