//go:build !js || !wasm

package systemviews

var currentPrefersDarkImpl = func() bool { return false }
