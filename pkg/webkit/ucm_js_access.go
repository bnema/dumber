package webkit

// getOmniboxScript exposes the injected omnibox/find component script as a function
// to avoid any build-tag edge cases when referenced from cgo files.
func getOmniboxScript() string {
    return ucmOmniboxScript
}

