//go:build js && wasm

package systemviews

import (
	"fmt"
	"syscall/js"
)

type browserDOM struct {
	document js.Value
	target   js.Value
}

func NewDOM() DOM {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		return &browserDOM{}
	}

	target := doc.Call("getElementById", "app")
	if !target.Truthy() {
		target = doc.Get("body")
	}

	return &browserDOM{document: doc, target: target}
}

func (d *browserDOM) Mount(html string) error {
	if d == nil || !d.target.Truthy() {
		return fmt.Errorf("DOM target not available")
	}
	d.target.Set("innerHTML", html)
	return nil
}
