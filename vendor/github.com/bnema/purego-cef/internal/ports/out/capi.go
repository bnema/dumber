package out

import "unsafe"

// CAPI composes all outbound port interfaces.
// It embeds the generated AppFunctions port and adds handwritten
// infrastructure methods that intentionally sit outside cefgen's parsed
// header set: low-level string helpers, string-list helpers, and purego
// callback creation.
type CAPI interface {
	// Generated outbound port — CEF free functions from cef_app_capi.h.
	AppFunctions

	// Infrastructure — not in parsed CEF headers, handwritten.
	NewCallback(fn any) uintptr
	UnrefCallback(cb uintptr) error
	StringSet(src *uint16, srcLen uintptr, output unsafe.Pointer, copy int32) int32
	StringClear(s unsafe.Pointer)
	StringUserfreeFree(s unsafe.Pointer)
	StringListAlloc() uintptr
	StringListAppend(list uintptr, value unsafe.Pointer)
	StringListFree(list uintptr)
	StringListSize(list uintptr) uintptr
	StringListValue(list uintptr, index uintptr, value unsafe.Pointer) int32
}
