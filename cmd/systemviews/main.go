//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/bnema/dumber/internal/infrastructure/systemviewsbridge"
	"github.com/bnema/dumber/internal/ui/systemviews"
)

func main() {
	bridge := systemviewsbridge.NewBrowserClient()
	href := ""
	location := js.Global().Get("location")
	if location.Type() != js.TypeUndefined && location.Type() != js.TypeNull {
		href = location.Get("href").String()
	}
	app := newBridgeApp(systemviews.NewDOM(), href, bridge)

	if err := app.Run(); err != nil {
		js.Global().Get("console").Call("error", err.Error())
		panic(err)
	}

	select {}
}
