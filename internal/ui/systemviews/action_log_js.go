//go:build js && wasm

package systemviews

import (
	"context"
	"syscall/js"
)

func logActionMountError(_ context.Context, mountErr, actionErr error) {
	if mountErr == nil && actionErr == nil {
		return
	}
	console := js.Global().Get("console")
	if !console.Truthy() || !console.Get("error").Truthy() {
		return
	}
	console.Call("error", "failed to mount systemview action error", errorString(mountErr), errorString(actionErr))
}
