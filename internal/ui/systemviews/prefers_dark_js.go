//go:build js && wasm

package systemviews

import "syscall/js"

var currentPrefersDarkImpl = func() bool {
	window := js.Global()
	if !window.Truthy() {
		return false
	}

	matchMedia := window.Get("matchMedia")
	if !matchMedia.Truthy() {
		return false
	}

	media := matchMedia.Invoke("(prefers-color-scheme: dark)")
	if !media.Truthy() {
		return false
	}
	matches := media.Get("matches")
	return matches.Truthy() && matches.Bool()
}
