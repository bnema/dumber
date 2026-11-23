//go:build stress

// Package stress is an end-to-end stress harness for the Dumber browser.
//
// It builds Dumber, launches it in an isolated NeferWL session with a
// sandboxed XDG environment, and drives it through local multi-site pages
// (redirects, popups, cross-site iframes, memory churn) to catch runtime
// corruption and crashes.
//
// The harness is opt-in: build with -tags stress and set DUMBER_STRESS=1
// to run the browser scenarios, for example:
//
//	DUMBER_STRESS=1 go test -tags stress -count=1 ./test/stress/
package stress
