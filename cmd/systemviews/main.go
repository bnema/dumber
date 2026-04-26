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
	if location.Truthy() {
		hrefValue := location.Get("href")
		if hrefValue.Truthy() && hrefValue.Type() == js.TypeString {
			href = hrefValue.String()
		}
	}
	app := newBridgeApp(systemviews.NewDOM(), href, bridge)

	if err := app.Run(); err != nil {
		js.Global().Get("console").Call("error", err.Error())
		panic(err)
	}

	select {}
}
