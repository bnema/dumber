package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/ui/window"
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

func TestBrowserWindow_RemovePromotesRemainingMainWindow(t *testing.T) {
	mainWindow := &window.MainWindow{}
	otherWindow := &window.MainWindow{}
	first := &browserWindow{id: "window-1", mainWindow: mainWindow}
	second := &browserWindow{id: "window-2", mainWindow: otherWindow}
	app := &App{
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
	}

	app.removeBrowserWindow(first.id)

	if app.mainWindow != otherWindow {
		t.Fatalf("mainWindow = %p, want promoted window %p", app.mainWindow, otherWindow)
	}
	if len(app.browserWindows) != 1 {
		t.Fatalf("browserWindows length = %d, want 1", len(app.browserWindows))
	}
	if _, ok := app.browserWindows[second.id]; !ok {
		t.Fatalf("remaining browser window was not kept")
	}
}

func TestBrowserWindow_RemoveLastClearsMainWindow(t *testing.T) {
	mainWindow := &window.MainWindow{}
	first := &browserWindow{id: "window-1", mainWindow: mainWindow}
	app := &App{
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{first.id: first},
	}

	app.removeBrowserWindow(first.id)

	if app.mainWindow != nil {
		t.Fatalf("mainWindow = %p, want nil", app.mainWindow)
	}
	if len(app.browserWindows) != 0 {
		t.Fatalf("browserWindows length = %d, want 0", len(app.browserWindows))
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

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
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
	if got := app.browserWindows["window-1"]; got == nil {
		t.Fatalf("browser window was not tracked")
	}
	if app.browserWindows["window-1"].initialURL != "https://example.com" {
		t.Fatalf("browser window initialURL = %q, want %q", app.browserWindows["window-1"].initialURL, "https://example.com")
	}
}
