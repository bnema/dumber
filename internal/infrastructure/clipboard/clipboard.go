// Package clipboard provides a clipboard adapter with Wayland/X11 preference and toolkit fallback.
package clipboard

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/rs/zerolog"
)

type toolkitClipboard interface {
	ReadText(ctx context.Context) (string, error)
	WriteText(ctx context.Context, text string) error
	WriteImage(ctx context.Context, image entity.ImageData) error
}

// Adapter implements port.Clipboard using wl-clipboard on Wayland, then xclip/xsel on X11.
// If no system clipboard tool is available, it falls back to the toolkit clipboard.
type Adapter struct {
	toolkit  toolkitClipboard
	copyCmd  string
	pasteCmd string
}

func (a *Adapter) ensureToolkitClipboard() toolkitClipboard {
	if a == nil {
		return nil
	}
	if a.toolkit == nil {
		a.toolkit = newToolkitClipboard()
	}
	return a.toolkit
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

func (c *gdkToolkitClipboard) WriteText(ctx context.Context, text string) error {
	return withToolkitClipboard(ctx, func() error {
		if c == nil || c.clipboard == nil {
			return fmt.Errorf("toolkit clipboard not available")
		}
		c.clipboard.SetText(text)
		return nil
	})
}

func (c *gdkToolkitClipboard) ReadText(ctx context.Context) (string, error) {
	var text string
	err := withToolkitClipboard(ctx, func() error {
		if c == nil || c.clipboard == nil {
			return fmt.Errorf("toolkit clipboard not available")
		}

		resultCh := make(chan struct {
			text string
			err  error
		}, 1)
		cb := gio.AsyncReadyCallback(func(_ uintptr, result uintptr, _ uintptr) {
			asyncResult := &gio.AsyncResultBase{}
			asyncResult.SetGoPointer(result)
			readText, readErr := c.clipboard.ReadTextFinish(asyncResult)
			resultCh <- struct {
				text string
				err  error
			}{text: readText, err: readErr}
		})
		c.clipboard.ReadTextAsync(nil, &cb, 0)

		mainContext := glib.MainContextDefault()
		if mainContext == nil {
			return fmt.Errorf("toolkit main context not available")
		}

		for {
			select {
			case result := <-resultCh:
				text = result.text
				return result.err
			default:
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			mainContext.Iteration(true)
		}
	})
	return text, err
}

func (c *gdkToolkitClipboard) WriteImage(ctx context.Context, image entity.ImageData) error {
	return withToolkitClipboard(ctx, func() error {
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
	})
}

func withToolkitClipboard(ctx context.Context, fn func() error) error {
	mainContext := glib.MainContextDefault()
	if mainContext == nil || mainContext.IsOwner() {
		return fn()
	}

	resultCh := make(chan error, 1)
	cb := glib.SourceFunc(func(_ uintptr) bool {
		resultCh <- fn()
		return false
	})
	glib.IdleAdd(&cb, 0)
	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
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
	var commandErr error
	var toolkitErr error
	if a.copyCmd != "" {
		err := a.writeTextWithCommand(ctx, text, log)
		if err == nil {
			return nil
		}
		commandErr = err
	}
	if toolkit := a.ensureToolkitClipboard(); toolkit != nil {
		err := toolkit.WriteText(ctx, text)
		if err == nil {
			log.Debug().Str("backend", "toolkit").Int("len", len(text)).Msg("clipboard write success")
			return nil
		}
		toolkitErr = err
		log.Debug().Err(err).Msg("toolkit clipboard write failed; falling back")
	}
	if commandErr != nil {
		return commandErr
	}
	if toolkitErr != nil {
		log.Error().Err(toolkitErr).Msg("clipboard write failed")
		return toolkitErr
	}
	err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
	log.Error().Err(err).Msg("clipboard write failed")
	return err
}

// WriteImage copies image bytes to the clipboard.
// xsel is not supported because the adapter has no reliable binary image mode
// for it in this codebase.
func (a *Adapter) WriteImage(ctx context.Context, image entity.ImageData) error {
	log := logging.FromContext(ctx)
	var commandErr error
	var toolkitErr error

	if len(image.Bytes) == 0 {
		err := fmt.Errorf("empty image data")
		log.Error().Err(err).Msg("clipboard image write failed")
		return err
	}
	if toolkit := a.ensureToolkitClipboard(); toolkit != nil {
		err := toolkit.WriteImage(ctx, image)
		if err == nil {
			log.Debug().
				Str("backend", "toolkit").
				Str("mime_type", image.MimeType).
				Int("bytes", len(image.Bytes)).
				Msg("clipboard image write success")
			return nil
		}
		toolkitErr = err
		log.Debug().Err(err).Msg("toolkit clipboard image write failed; falling back")
	}
	if a.copyCmd != "" {
		err := a.writeImageWithCommand(ctx, image, log)
		if err == nil {
			return nil
		}
		commandErr = err
	}
	if commandErr != nil {
		return commandErr
	}
	if toolkitErr != nil {
		log.Error().Err(toolkitErr).Msg("clipboard image write failed")
		return toolkitErr
	}
	err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
	log.Error().Err(err).Msg("clipboard image write failed")
	return err
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

func (a *Adapter) writeImageWithCommand(ctx context.Context, image entity.ImageData, log *zerolog.Logger) error {
	if a.copyCmd == "" {
		err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
		log.Error().Err(err).Msg("clipboard image write failed")
		return err
	}
	mimeType := image.MimeType
	if mimeType == "" {
		mimeType = "image/png"
	}

	var cmd *exec.Cmd
	if strings.Contains(a.copyCmd, "wl-copy") {
		cmd = commandContext(ctx, a.copyCmd, "--type", mimeType)
	} else if strings.Contains(a.copyCmd, "xclip") {
		cmd = commandContext(ctx, a.copyCmd, "-selection", "clipboard", "-t", mimeType, "-i")
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

func (a *Adapter) readTextWithCommand(ctx context.Context, log *zerolog.Logger) (string, error) {
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
		err = fmt.Errorf("run clipboard tool %s: %w", a.pasteCmd, err)
		log.Debug().Err(err).Str("tool", a.pasteCmd).Msg("clipboard read failed")
		return "", err
	}

	log.Debug().Str("tool", a.pasteCmd).Int("len", len(out)).Msg("clipboard read success")
	return string(out), nil
}

// ReadText reads text from the clipboard.
func (a *Adapter) ReadText(ctx context.Context) (string, error) {
	log := logging.FromContext(ctx)
	var commandErr error

	if a.pasteCmd != "" {
		text, err := a.readTextWithCommand(ctx, log)
		if err == nil {
			return text, nil
		}
		commandErr = err
		log.Debug().Err(err).Msg("system clipboard read failed; falling back")
	}

	if toolkit := a.ensureToolkitClipboard(); toolkit != nil {
		text, err := toolkit.ReadText(ctx)
		if err == nil {
			log.Debug().Str("backend", "toolkit").Int("len", len(text)).Msg("clipboard read success")
			return text, nil
		}
		log.Debug().Err(err).Msg("toolkit clipboard read failed")
		if commandErr != nil {
			return "", commandErr
		}
		return "", err
	}

	if commandErr != nil {
		return "", commandErr
	}
	err := fmt.Errorf("no clipboard tool available (install wl-clipboard or xclip)")
	log.Error().Err(err).Msg("clipboard read failed")
	return "", err
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
