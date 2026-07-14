// internal/core/marshal.go
package core

import (
	"runtime"
	"unicode/utf16"
	"unsafe"

	"github.com/bnema/purego-cef/internal/capi"
)

// CEFStringT is the raw CEF UTF-16 string layout shared with the generated
// capi layer.
type CEFStringT = capi.CEFStringT

// CefString converts a Go string to a CEF UTF-16 string via the CAPI adapter.
func (e *Engine) CefString(s string) CEFStringT {
	encoded := utf16.Encode([]rune(s))
	var src *uint16
	if len(encoded) > 0 {
		src = &encoded[0]
	}
	var out CEFStringT
	e.capi.StringSet(src, uintptr(len(encoded)), unsafe.Pointer(&out), 1)
	runtime.KeepAlive(encoded)
	return out
}

// FreeCefString releases a CEF string's backing memory.
func (e *Engine) FreeCefString(cs *CEFStringT) {
	if cs != nil {
		e.capi.StringClear(unsafe.Pointer(cs))
	}
}

// GoString converts a pointer to a CEF string to a Go string.
func GoString(cs unsafe.Pointer) string {
	if cs == nil {
		return ""
	}
	v := (*CEFStringT)(cs)
	if v.Str == nil || v.Length == 0 {
		return ""
	}
	slice := unsafe.Slice(v.Str, v.Length)
	return string(utf16.Decode(slice))
}

// GoStringUserfree converts a cef_string_userfree_t to a Go string and frees it.
func (e *Engine) GoStringUserfree(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	s := GoString(ptr)
	e.capi.StringUserfreeFree(ptr)
	return s
}

// NewStringList allocates a cef_string_list_t and appends the provided values.
// It returns 0 if allocation fails.
func (e *Engine) NewStringList(values ...string) uintptr {
	list := e.capi.StringListAlloc()
	if list == 0 {
		return 0
	}
	for _, value := range values {
		e.AppendStringList(list, value)
	}
	return list
}

// AppendStringList appends a Go string to a caller-owned cef_string_list_t.
func (e *Engine) AppendStringList(list uintptr, value string) {
	if list == 0 {
		return
	}
	cs := e.CefString(value)
	defer e.FreeCefString(&cs)
	e.capi.StringListAppend(list, unsafe.Pointer(&cs))
}

// FreeStringList releases a caller-owned cef_string_list_t.
func (e *Engine) FreeStringList(list uintptr) {
	if list != 0 {
		e.capi.StringListFree(list)
	}
}

// DecodeStringList converts a cef_string_list_t handle to a Go string slice.
func (e *Engine) DecodeStringList(list uintptr) []string {
	if list == 0 {
		return nil
	}
	n := int(e.capi.StringListSize(list))
	if n == 0 {
		return nil
	}
	out := make([]string, n)
	for i := range out {
		var cs CEFStringT
		if e.capi.StringListValue(list, uintptr(i), unsafe.Pointer(&cs)) != 0 {
			out[i] = GoString(unsafe.Pointer(&cs))
			e.capi.StringClear(unsafe.Pointer(&cs))
		}
	}
	return out
}

// DecodeSlice converts a raw pointer and count into a Go slice of T.
// The returned slice aliases native memory and is only valid during the callback.
func DecodeSlice[T any](ptr uintptr, count int) []T {
	if count == 0 || ptr == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Pointer(ptr)), count)
}

// RawPointerHolder is implemented by wrapper types that hold a raw CEF pointer.
type RawPointerHolder interface {
	RawPointer() unsafe.Pointer
}

// ExtractRawPointer returns the underlying raw CEF pointer from an interface.
func ExtractRawPointer(v any) unsafe.Pointer {
	if v == nil {
		return nil
	}
	if h, ok := v.(RawPointerHolder); ok {
		return h.RawPointer()
	}
	return nil
}

// ExtractOrWrapRawPointer returns the raw pointer for v, calling wrap if needed.
func ExtractOrWrapRawPointer(v any, wrap func() any) unsafe.Pointer {
	if v == nil {
		return nil
	}
	if ptr := ExtractRawPointer(v); ptr != nil {
		return ptr
	}
	if wrap == nil {
		return nil
	}
	return ExtractRawPointer(wrap())
}
