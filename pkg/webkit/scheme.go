//go:build !webkit_cgo

package webkit

// SchemeResolver provides bytes and mime for a given custom URI.
type SchemeResolver func(uri string) (mime string, data []byte, ok bool)

// SetURISchemeResolver is a no-op in non-CGO builds.
func SetURISchemeResolver(r SchemeResolver) { /* no-op */ }
