//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/bnema/dumber/internal/infrastructure/systemviewsbridge"
	"github.com/bnema/dumber/internal/ui/systemviews"
)

func main() {
	bridge := systemviewsbridge.NewBrowserClient()
	app := newBridgeApp(systemviews.NewDOM(), js.Global().Get("location").Get("href").String(), bridge)

	if err := app.Run(); err != nil {
		js.Global().Get("console").Call("error", err.Error())
		panic(err)
	}

	select {}
}
