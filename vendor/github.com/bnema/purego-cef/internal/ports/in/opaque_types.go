package in

// StringList is an opaque CEF string-list handle (cef_string_list_t).
// Callers should treat it as an owned native handle managed by the CEF API.
type StringList uintptr

// StringMap is an opaque CEF string-map handle (cef_string_map_t).
// Callers should treat it as an owned native handle managed by the CEF API.
type StringMap uintptr

// StringMultimap is an opaque CEF string-multimap handle (cef_string_multimap_t).
// Callers should treat it as an owned native handle managed by the CEF API.
type StringMultimap uintptr
