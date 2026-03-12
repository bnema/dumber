package bootstrap

import (
	"fmt"
	"io"
	"os"
)

// ApplyGTKIMModuleFallback sets GTK_IM_MODULE to "gtk-im-context-simple" when:
//   - GTK_IM_MODULE is unset, AND
//   - the session appears to be Wayland (XDG_SESSION_TYPE == "wayland" OR WAYLAND_DISPLAY != "").
//
// This ensures dead-key compose works on Wayland compositors that lack text-input-v3
// (e.g. niri), without overriding an explicit user choice.
//
// The getter and setter parameters allow injection for testing; pass os.Getenv and
// os.Setenv for production use.
func ApplyGTKIMModuleFallback(stderr io.Writer, getter func(string) string, setter func(string, string) error) {
	if getter("GTK_IM_MODULE") != "" {
		return
	}
	isWayland := getter("XDG_SESSION_TYPE") == "wayland" || getter("WAYLAND_DISPLAY") != ""
	if !isWayland {
		return
	}
	_, _ = fmt.Fprintf(stderr, "dumber: GTK_IM_MODULE unset, defaulting to gtk-im-context-simple for dead-key support\n")
	if err := setter("GTK_IM_MODULE", "gtk-im-context-simple"); err != nil {
		_, _ = fmt.Fprintf(stderr, "dumber: failed to set GTK_IM_MODULE: %v\n", err)
	}
}

// ApplyGTKIMModuleFallbackDefault calls ApplyGTKIMModuleFallback with os.Getenv and os.Setenv.
func ApplyGTKIMModuleFallbackDefault(stderr io.Writer) {
	ApplyGTKIMModuleFallback(stderr, os.Getenv, os.Setenv)
}
