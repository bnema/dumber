package cutil

import "unsafe"

// CString converts a NUL-terminated C string pointer into a Go string.
// The returned string shares memory with the C string and is only valid while
// the C memory remains allocated and unmodified. Copy the result if it must
// outlive the C memory.
func CString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(ptr, n)) != 0 {
		n++
	}
	return unsafe.String((*byte)(ptr), n)
}
