//go:build js && wasm

package main

import (
	"os"
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

	// A Run failure is terminal and exits nonzero without a Go panic
	// trap: the shell's promise handler renders the fatal state and the
	// fixture observes the exit code. The runtime executes exactly once;
	// select{} below keeps it alive so go.run never resolves.
	if err := app.Run(); err != nil {
		js.Global().Get("console").Call("error", err.Error())
		os.Exit(1)
	}

	select {}
}
