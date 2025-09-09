//go:build webkit_cgo

package webkit

/*
#include <stdlib.h>
*/
import "C"

import (
    "unsafe"
)

// Resolver provides bytes and mime for a given custom URI.
type SchemeResolver func(uri string) (mime string, data []byte, ok bool)

var globalResolver SchemeResolver

// SetURISchemeResolver sets the resolver used by the custom URI scheme handler.
func SetURISchemeResolver(r SchemeResolver) { globalResolver = r }

//export goResolveURIScheme
func goResolveURIScheme(curi *C.char, outLen *C.size_t, outMime **C.char) unsafe.Pointer {
    if curi == nil || outLen == nil || outMime == nil { return nil }
    uri := C.GoString(curi)
    if globalResolver == nil {
        *outLen = 0
        *outMime = nil
        return nil
    }
    mime, data, ok := globalResolver(uri)
    if !ok || len(data) == 0 {
        *outLen = 0
        *outMime = nil
        return nil
    }
    buf := C.malloc(C.size_t(len(data)))
    if buf == nil {
        *outLen = 0
        *outMime = nil
        return nil
    }
    // Copy data into C buffer
    dst := (*[1 << 30]byte)(buf)[:len(data):len(data)]
    copy(dst, data)
    *outLen = C.size_t(len(data))
    *outMime = C.CString(mime)
    return buf
}

