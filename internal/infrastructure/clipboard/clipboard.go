// Package clipboard provides a clipboard adapter with toolkit preference and wl-clipboard/X11 fallback.
package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/rs/zerolog"
)

type toolkitClipboard interface {
	WriteText(ctx context.Context, text string) error
	WriteImage(ctx context.Context, image port.ImageData) error
}

// Adapter implements port.Clipboard using the toolkit clipboard when available.
// It falls back to wl-clipboard for Wayland, then xclip/xsel for X11.
type Adapter struct {
	toolkit  toolkitClipboard
	copyCmd  string
	pasteCmd string
}

var (
	lookPath            = exec.LookPath
	commandContext      = exec.CommandContext
	newToolkitClipboard = defaultToolkitClipboard
)

// New creates a new clipboard adapter.
// Detects Wayland vs X11 and selects appropriate clipboard tool.
func New() port.Clipboard {
	a := &Adapter{toolkit: newToolkitClipboard()}
	a.copyCmd, a.pasteCmd = selectSystemClipboard()
	return a
}

func defaultToolkitClipboard() toolkitClipboard {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return nil
	}

	clipboard := display.GetClipboard()
	if clipboard == nil {
		return nil
	}

	return &gdkToolkitClipboard{clipboard: clipboard}
}

type gdkToolkitClipboard struct {
	clipboard *gdk.Clipboard
}

func (c *gdkToolkitClipboard) WriteText(_ context.Context, text string) error {
	if c == nil || c.clipboard == nil {
		return fmt.Errorf("toolkit clipboard not available")
	}
	c.clipboard.SetText(text)
	return nil
}

func (c *gdkToolkitClipboard) WriteImage(_ context.Context, image port.ImageData) error {
	if c == nil || c.clipboard == nil {
		return fmt.Errorf("toolkit clipboard not available")
	}

	texture, err := gdk.NewTextureFromBytes(glib.NewBytes(image.Bytes, uint(len(image.Bytes))))
	if err != nil {
		return err
	}
	if texture == nil {
		return fmt.Errorf("toolkit clipboard returned nil texture")
	}

	c.clipboard.SetTexture(texture)
	return nil
}

func selectSystemClipboard() (copyCmd, pasteCmd string) {
	// Check for Wayland first.
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if path, err := lookPath("wl-copy"); err == nil {
			copyCmd = path
			if pastePath, err := lookPath("wl-paste"); err == nil {
				pasteCmd = pastePath
			}
		}
	}

	// Fall back to X11 if Wayland tools are not available.
	if copyCmd == "" && os.Getenv("DISPLAY") != "" {
		if path, err := lookPath("xclip"); err == nil {
			copyCmd = path
			pasteCmd = path
		} else if path, err := lookPath("xsel"); err == nil {
			copyCmd = path
			pasteCmd = path
		}
	}

	return copyCmd, pasteCmd
}

// WriteText copies text to the clipboard.
func (a *Adapter) WriteText(ctx context.Context, text string) error {
	log := logging.FromContext(ctx)
	if a.toolkit != nil {
		err := a.toolkit.WriteText(ctx, text)
		if err == nil {
			log.Debug().Str("backend", "toolkit").Int("len", len(text)).Msg("clipboard write success")
			return nil
		}
		log.Debug().Err(err).Msg("toolkit clipboard write failed; falling back")
	}

	return a.writeTextWithCommand(ctx, text, log)
}

// WriteImage copies image bytes to the clipboard.
// This path is PNG-only; xsel is not supported because the adapter has no
// reliable binary image mode for it in this codebase.
func (a *Adapter) WriteImage(ctx context.Context, image port.ImageData) error {
	log := logging.FromContext(ctx)

	if len(image.Bytes) == 0 {
		err := fmt.Errorf("empty image data")
		log.Error().Err(err).Msg("clipboard image write failed")
		return err
	}
	if a.toolkit != nil {
		err := a.toolkit.WriteImage(ctx, image)
		if err == nil {
			log.Debug().Str("backend", "toolkit").Int("bytes", len(image.Bytes)).Msg("clipboard image write success")
			return nil
		}
		log.Debug().Err(err).Msg("toolkit clipboard image write failed; falling back")
	}

	return a.writeImageWithCommand(ctx, image, log)
}

func (a *Adapter) writeTextWithCommand(ctx context.Context, text string, log *zerolog.Logger) error {
	if a.copyCmd == "" {
		err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
		log.Error().Err(err).Msg("clipboard write failed")
		return err
	}

	var cmd *exec.Cmd
	if strings.Contains(a.copyCmd, "wl-copy") {
		cmd = commandContext(ctx, a.copyCmd)
	} else if strings.Contains(a.copyCmd, "xclip") {
		cmd = commandContext(ctx, a.copyCmd, "-selection", "clipboard")
	} else if strings.Contains(a.copyCmd, "xsel") {
		cmd = commandContext(ctx, a.copyCmd, "--clipboard", "--input")
	} else {
		err := fmt.Errorf("unknown clipboard tool: %s", a.copyCmd)
		log.Error().Err(err).Msg("clipboard write failed")
		return err
	}

	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("run clipboard tool %s: %w", a.copyCmd, err)
		log.Error().Err(err).Str("tool", a.copyCmd).Msg("clipboard write failed")
		return err
	}

	log.Debug().Str("tool", a.copyCmd).Int("len", len(text)).Msg("clipboard write success")
	return nil
}

func (a *Adapter) writeImageWithCommand(ctx context.Context, image port.ImageData, log *zerolog.Logger) error {
	if a.copyCmd == "" {
		err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
		log.Error().Err(err).Msg("clipboard image write failed")
		return err
	}

	var cmd *exec.Cmd
	if strings.Contains(a.copyCmd, "wl-copy") {
		cmd = commandContext(ctx, a.copyCmd, "--type", "image/png")
	} else if strings.Contains(a.copyCmd, "xclip") {
		cmd = commandContext(ctx, a.copyCmd, "-selection", "clipboard", "-t", "image/png", "-i")
	} else {
		err := fmt.Errorf("clipboard tool does not support image writes: %s", a.copyCmd)
		log.Error().Err(err).Msg("clipboard image write failed")
		return err
	}

	cmd.Stdin = bytes.NewReader(image.Bytes)
	if err := cmd.Run(); err != nil {
		err = fmt.Errorf("run clipboard tool %s: %w", a.copyCmd, err)
		log.Error().Err(err).Str("tool", a.copyCmd).Msg("clipboard image write failed")
		return err
	}

	log.Debug().Str("tool", a.copyCmd).Int("bytes", len(image.Bytes)).Msg("clipboard image write success")
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
		cmd = commandContext(ctx, a.pasteCmd, "--no-newline")
	} else if strings.Contains(a.pasteCmd, "xclip") {
		cmd = commandContext(ctx, a.pasteCmd, "-selection", "clipboard", "-o")
	} else if strings.Contains(a.pasteCmd, "xsel") {
		cmd = commandContext(ctx, a.pasteCmd, "--clipboard", "--output")
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
