// Package clipboard provides a clipboard adapter using wl-clipboard (Wayland) with X11 fallback.
package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Adapter implements port.Clipboard using system clipboard tools.
// Uses wl-clipboard for Wayland, falls back to xclip for X11.
type Adapter struct {
	copyCmd  string
	pasteCmd string
}

// New creates a new clipboard adapter.
// Detects Wayland vs X11 and selects appropriate clipboard tool.
func New() port.Clipboard {
	a := &Adapter{}

	// Check for Wayland first
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if path, err := exec.LookPath("wl-copy"); err == nil {
			a.copyCmd = path
			if pastePath, err := exec.LookPath("wl-paste"); err == nil {
				a.pasteCmd = pastePath
			}
		}
	}

	// Fall back to X11 if Wayland tools not available
	if a.copyCmd == "" && os.Getenv("DISPLAY") != "" {
		if path, err := exec.LookPath("xclip"); err == nil {
			a.copyCmd = path
			a.pasteCmd = path
		} else if path, err := exec.LookPath("xsel"); err == nil {
			a.copyCmd = path
			a.pasteCmd = path
		}
	}

	return a
}

// WriteText copies text to the clipboard.
func (a *Adapter) WriteText(ctx context.Context, text string) error {
	log := logging.FromContext(ctx)

	if a.copyCmd == "" {
		err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
		log.Error().Err(err).Msg("clipboard write failed")
		return err
	}

	var cmd *exec.Cmd
	if strings.Contains(a.copyCmd, "wl-copy") {
		cmd = exec.CommandContext(ctx, a.copyCmd)
	} else if strings.Contains(a.copyCmd, "xclip") {
		cmd = exec.CommandContext(ctx, a.copyCmd, "-selection", "clipboard")
	} else if strings.Contains(a.copyCmd, "xsel") {
		cmd = exec.CommandContext(ctx, a.copyCmd, "--clipboard", "--input")
	} else {
		err := fmt.Errorf("unknown clipboard tool: %s", a.copyCmd)
		log.Error().Err(err).Msg("clipboard write failed")
		return err
	}

	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		log.Error().Err(err).Str("tool", a.copyCmd).Msg("clipboard write failed")
		return err
	}

	log.Debug().Str("tool", a.copyCmd).Int("len", len(text)).Msg("clipboard write success")
	return nil
}

// ReadText reads text from the clipboard.
func (a *Adapter) ReadText(ctx context.Context) (string, error) {
	log := logging.FromContext(ctx)

	if a.pasteCmd == "" {
		err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
		log.Error().Err(err).Msg("clipboard read failed")
		return "", err
	}

	var cmd *exec.Cmd
	if strings.Contains(a.pasteCmd, "wl-paste") {
		cmd = exec.CommandContext(ctx, a.pasteCmd, "--no-newline")
	} else if strings.Contains(a.pasteCmd, "xclip") {
		cmd = exec.CommandContext(ctx, a.pasteCmd, "-selection", "clipboard", "-o")
	} else if strings.Contains(a.pasteCmd, "xsel") {
		cmd = exec.CommandContext(ctx, a.pasteCmd, "--clipboard", "--output")
	} else {
		err := fmt.Errorf("unknown clipboard tool: %s", a.pasteCmd)
		log.Error().Err(err).Msg("clipboard read failed")
		return "", err
	}

	out, err := cmd.Output()
	if err != nil {
		log.Debug().Err(err).Str("tool", a.pasteCmd).Msg("clipboard read failed (may be empty)")
		return "", err
	}

	log.Debug().Str("tool", a.pasteCmd).Int("len", len(out)).Msg("clipboard read success")
	return string(out), nil
}

// Clear clears the clipboard contents.
func (a *Adapter) Clear(ctx context.Context) error {
	return a.WriteText(ctx, "")
}

// HasText returns true if the clipboard contains text data.
func (a *Adapter) HasText(ctx context.Context) (bool, error) {
	text, err := a.ReadText(ctx)
	if err != nil {
		// Empty clipboard often returns error, treat as no text
		return false, nil
	}
	return text != "", nil
}
