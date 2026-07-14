package cef

import "runtime"

// These helpers wrap CEF's opaque cef_string_list_t utility API from
// include/internal/cef_string_list.h. That header intentionally sits outside
// cefgen's parsed header set, so string-list support lives in the handwritten
// foundational layer alongside the raw string helpers.

// NewStringList allocates a caller-owned CEF string list populated with the
// provided values. The returned list must be released with FreeStringList.
func NewStringList(values ...string) StringList {
	return StringList(mustEng().NewStringList(values...))
}

// FreeStringList releases a caller-owned CEF string list.
func FreeStringList(list StringList) {
	mustEng().FreeStringList(uintptr(list))
}

// StringListToSlice decodes an opaque CEF string list into a Go slice.
func StringListToSlice(list StringList) []string {
	return mustEng().DecodeStringList(uintptr(list))
}

// ContinueFileDialog continues a file dialog callback with the provided paths.
// An empty path list is treated the same as canceling the dialog. The opaque
// StringList allocated by this helper remains valid only for the duration of
// the Cont call and must not be retained by the callback after it returns.
func ContinueFileDialog(callback FileDialogCallback, filePaths ...string) {
	if callback == nil {
		return
	}
	if len(filePaths) == 0 {
		callback.Cancel()
		return
	}
	list := NewStringList(filePaths...)
	if list == 0 {
		callback.Cancel()
		return
	}
	defer FreeStringList(list)
	callback.Cont(list)
	runtime.KeepAlive(callback)
}
