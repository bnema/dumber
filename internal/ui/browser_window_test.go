package ui

import (
	"context"
	"testing"
)

func TestBrowserWindow_RegisterAndRemoveTrackCollection(t *testing.T) {
	app := &App{browserWindows: make(map[string]*browserWindow)}
	first := &browserWindow{id: "window-1"}
	second := &browserWindow{id: "window-2"}

	app.registerBrowserWindow(first)
	app.registerBrowserWindow(second)

	if got := len(app.browserWindows); got != 2 {
		t.Fatalf("browserWindows length = %d, want 2", got)
	}
	if _, ok := app.browserWindows[first.id]; !ok {
		t.Fatalf("first browser window was not registered")
	}
	if _, ok := app.browserWindows[second.id]; !ok {
		t.Fatalf("second browser window was not registered")
	}

	app.removeBrowserWindow(first.id)

	if _, ok := app.browserWindows[first.id]; ok {
		t.Fatalf("first browser window was not removed")
	}
	if _, ok := app.browserWindows[second.id]; !ok {
		t.Fatalf("second browser window should remain registered")
	}
}

func TestOpenFreshWindow_DispatchesAndTracksBrowserWindow(t *testing.T) {
	app := &App{browserWindows: make(map[string]*browserWindow)}

	dispatched := false
	createdURL := ""
	app.dispatchOnMainThread = func(fn func()) {
		dispatched = true
		fn()
	}
	app.browserWindowFactory = func(ctx context.Context, url string) (*browserWindow, error) {
		createdURL = url
		return &browserWindow{id: "window-1", initialURL: url}, nil
	}

	window, err := app.OpenFreshWindow(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}
	if !dispatched {
		t.Fatalf("OpenFreshWindow did not use the main-thread dispatcher")
	}
	if createdURL != "https://example.com" {
		t.Fatalf("browser window factory URL = %q, want %q", createdURL, "https://example.com")
	}
	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want 1", got)
	}
	if got := app.browserWindows[window.id]; got != window {
		t.Fatalf("browser window was not tracked")
	}
	if window.initialURL != "https://example.com" {
		t.Fatalf("browser window initialURL = %q, want %q", window.initialURL, "https://example.com")
	}
}
