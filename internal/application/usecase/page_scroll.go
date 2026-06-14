// Package usecase contains application use cases that orchestrate domain logic.
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

// PageScrollCommand represents a semantic page scroll action.
// Each command maps to a fixed pixel delta defined in this package.
type PageScrollCommand int

const (
	PageScrollLeft     PageScrollCommand = iota // scroll left by small horizontal amount
	PageScrollRight                             // scroll right by small horizontal amount
	PageScrollUp                                // scroll up by small vertical amount
	PageScrollDown                              // scroll down by small vertical amount
	PageScrollUpFast                            // scroll up by fast vertical amount
	PageScrollDownFast                          // scroll down by fast vertical amount
)

// Internal fixed pixel defaults for page scroll commands.
// These are application-layer constants, not user-configurable.
const (
	scrollSmallHorizontal = 80
	scrollSmallVertical   = 80
	scrollFastVertical    = 320
)

// PageScrollUseCase handles semantic page scrolling commands.
// It maps semantic scroll actions (left, down, up-fast, etc.) to pixel deltas
// and delegates to the WebView's scroll capability. No UI logic or
// engine-specific JavaScript exists in this layer.
type PageScrollUseCase struct{}

// NewPageScrollUseCase creates a new PageScrollUseCase.
func NewPageScrollUseCase() *PageScrollUseCase {
	return &PageScrollUseCase{}
}

// Scroll applies a semantic scroll command to a WebView.
// It returns an error if the WebView does not support programmatic scrolling
// (i.e., does not implement port.Scrollable).
func (*PageScrollUseCase) Scroll(ctx context.Context, wv port.WebView, cmd PageScrollCommand) error {
	if wv == nil {
		return errors.New("page scroll: nil webview")
	}

	scroller, ok := wv.(port.Scrollable)
	if !ok {
		return errors.New("page scroll: webview does not support programmatic scrolling")
	}

	dx, dy := scrollDelta(cmd)
	if err := scroller.ScrollBy(ctx, dx, dy); err != nil {
		return fmt.Errorf("page scroll: %w", err)
	}
	return nil
}

// scrollDelta returns the pixel delta for a PageScrollCommand.
func scrollDelta(cmd PageScrollCommand) (dx, dy int) {
	switch cmd {
	case PageScrollLeft:
		return -scrollSmallHorizontal, 0
	case PageScrollRight:
		return scrollSmallHorizontal, 0
	case PageScrollUp:
		return 0, -scrollSmallVertical
	case PageScrollDown:
		return 0, scrollSmallVertical
	case PageScrollUpFast:
		return 0, -scrollFastVertical
	case PageScrollDownFast:
		return 0, scrollFastVertical
	default:
		return 0, 0
	}
}

// String returns a human-readable name for the scroll command.
func (cmd PageScrollCommand) String() string {
	switch cmd {
	case PageScrollLeft:
		return "left"
	case PageScrollRight:
		return "right"
	case PageScrollUp:
		return "up"
	case PageScrollDown:
		return "down"
	case PageScrollUpFast:
		return "up-fast"
	case PageScrollDownFast:
		return "down-fast"
	default:
		return "unknown"
	}
}
