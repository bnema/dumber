package main

import (
	"flag"
	"fmt"
	"os"
)

const defaultIterations = 5000

func main() {
	iterations := flag.Int("iterations", defaultIterations, "number of synthetic omnibox updates")
	flag.Parse()

	fmt.Printf("stress harness placeholder: iterations=%d\n", *iterations)
	fmt.Println("TODO: attach to a live dumber session and drive omnibox updates")
	os.Exit(0)
}
