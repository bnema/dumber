//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/bnema/dumber/internal/ui/systemviews"
)

func main() {
	app := systemviews.NewApp(systemviews.Dependencies{
		DOM:         systemviews.NewDOM(),
		LocationURI: js.Global().Get("location").Get("href").String(),
	})

	if err := app.Run(); err != nil {
		js.Global().Get("console").Call("error", err.Error())
		panic(err)
	}

	select {}
}
