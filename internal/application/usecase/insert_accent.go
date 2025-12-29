// Package usecase contains application use cases that orchestrate domain logic.
package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// LongPressDelay is the duration a key must be held to trigger the accent picker.
const LongPressDelay = 400 * time.Millisecond

// InsertAccentUseCase handles long-press detection and accent selection.
// When a key with accent variants is held for 400ms, it shows an accent picker
// and inserts the selected accent into the focused text input.
type InsertAccentUseCase struct {
	focusProvider port.FocusedInputProvider
	accentPicker  port.AccentPickerUI

	// Current long-press state
	pressedChar   rune
	shiftHeld     bool
	timer         *time.Timer
	pickerVisible bool
	repeatCount   int                  // Number of key repeats while timer is pending
	targetInput   port.TextInputTarget // The input that was focused when long-press started

	// Callback to show picker on GTK main thread
	showPickerOnMainThread func(fn func())

	mu sync.Mutex
}

// NewInsertAccentUseCase creates a new accent insertion use case.
// showOnMainThread is a callback that runs a function on the GTK main thread
// (typically using glib.IdleAdd). This is required because the timer fires
// on a background goroutine.
func NewInsertAccentUseCase(
	focusProvider port.FocusedInputProvider,
	accentPicker port.AccentPickerUI,
	showOnMainThread func(fn func()),
) *InsertAccentUseCase {
	return &InsertAccentUseCase{
		focusProvider:          focusProvider,
		accentPicker:           accentPicker,
		showPickerOnMainThread: showOnMainThread,
	}
}

// OnKeyPressed handles a key press event.
// Returns true if the use case is handling this key and the caller should suppress it.
// Note: Due to WebKit's input handling, key suppression may not be fully effective.
// When the picker shows, we delete any repeated characters that slipped through.
func (uc *InsertAccentUseCase) OnKeyPressed(ctx context.Context, char rune, shiftHeld bool) bool {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	defer uc.mu.Unlock()

	// If picker is visible, block all keys (picker handles them)
	if uc.pickerVisible {
		return true
	}

	// If the same key is still pressed (key repeat), count it
	// We try to block it but also track repeats in case blocking doesn't work
	if uc.pressedChar == char && uc.timer != nil {
		uc.repeatCount++
		log.Debug().
			Str("char", string(char)).
			Int("repeat_count", uc.repeatCount).
			Msg("key repeat detected")
		return true // Try to block repeat keys
	}

	// Cancel any pending timer for a different key
	uc.cancelTimerLocked()

	// Check if this character has accents
	if !entity.HasAccents(char) {
		return false
	}

	// Capture the currently focused input - we'll insert into this target
	// even if focus changes when the picker appears
	uc.targetInput = uc.focusProvider.GetFocusedInput()
	if uc.targetInput == nil {
		log.Debug().Msg("no focused input, skipping accent detection")
		return false
	}

	// Start long-press detection
	uc.pressedChar = char
	uc.shiftHeld = shiftHeld
	uc.repeatCount = 0 // Reset repeat counter

	log.Debug().
		Str("char", string(char)).
		Bool("shift", shiftHeld).
		Msg("starting long-press detection")

	uc.timer = time.AfterFunc(LongPressDelay, func() {
		uc.onLongPressTriggered(ctx)
	})

	// Allow the first key press through (user might just want to type quickly)
	return false
}

// OnKeyReleased handles a key release event.
// This cancels any pending long-press detection.
func (uc *InsertAccentUseCase) OnKeyReleased(ctx context.Context, char rune) {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	defer uc.mu.Unlock()

	// Only cancel if it's the same key that was pressed
	if uc.pressedChar == char && uc.timer != nil {
		log.Debug().
			Str("char", string(char)).
			Msg("key released before long-press threshold")
		uc.cancelTimerLocked()
	}
}

// Cancel cancels any pending long-press detection and hides the picker.
func (uc *InsertAccentUseCase) Cancel(ctx context.Context) {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	defer uc.mu.Unlock()

	if uc.timer != nil || uc.pickerVisible {
		log.Debug().Msg("canceling accent picker")
	}

	uc.cancelTimerLocked()
	if uc.pickerVisible {
		uc.accentPicker.Hide()
		uc.pickerVisible = false
	}
}

// IsPickerVisible returns true if the accent picker is currently visible.
func (uc *InsertAccentUseCase) IsPickerVisible() bool {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	return uc.pickerVisible
}

// onLongPressTriggered is called when the long-press timer fires.
// This runs on a background goroutine, so we use showPickerOnMainThread.
func (uc *InsertAccentUseCase) onLongPressTriggered(ctx context.Context) {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	char := uc.pressedChar
	uppercase := uc.shiftHeld
	repeatCount := uc.repeatCount
	uc.timer = nil
	uc.mu.Unlock()

	accents := entity.GetAccents(char, uppercase)
	if len(accents) == 0 {
		return
	}

	log.Debug().
		Str("char", string(char)).
		Bool("uppercase", uppercase).
		Int("accents", len(accents)).
		Int("repeat_count", repeatCount).
		Msg("long-press triggered, showing accent picker")

	// Show picker on main thread
	uc.showPickerOnMainThread(func() {
		uc.showPicker(ctx, accents, repeatCount)
	})
}

// showPicker displays the accent picker UI.
// Must be called on the GTK main thread.
// repeatCount is the number of repeated characters that may have been typed
// (despite our attempt to block them) and need to be deleted.
func (uc *InsertAccentUseCase) showPicker(ctx context.Context, accents []rune, repeatCount int) {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	target := uc.targetInput // Use the captured target, not current focus
	uc.mu.Unlock()

	// Delete any repeated characters that slipped through
	// +1 for the initial character that was typed
	charsToDelete := repeatCount + 1
	if charsToDelete > 0 && target != nil {
		log.Debug().
			Int("chars_to_delete", charsToDelete).
			Msg("deleting repeated characters before showing picker")
		if err := target.DeleteBeforeCursor(ctx, charsToDelete); err != nil {
			log.Warn().Err(err).Msg("failed to delete repeated characters")
		}
	}

	uc.mu.Lock()
	uc.pickerVisible = true
	uc.mu.Unlock()

	uc.accentPicker.Show(accents, func(accent rune) {
		uc.onAccentSelected(ctx, accent)
	}, func() {
		uc.onPickerCanceled(ctx)
	})

	log.Debug().Msg("accent picker shown")
}

// onAccentSelected is called when the user selects an accent.
func (uc *InsertAccentUseCase) onAccentSelected(ctx context.Context, accent rune) {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	target := uc.targetInput // Use the captured target, not current focus
	uc.pickerVisible = false
	uc.pressedChar = 0
	uc.repeatCount = 0
	uc.targetInput = nil
	uc.mu.Unlock()

	uc.accentPicker.Hide()

	if target == nil {
		log.Warn().Msg("no target input for accent insertion")
		return
	}

	log.Debug().
		Str("accent", string(accent)).
		Msg("inserting accent")

	if err := target.InsertText(ctx, string(accent)); err != nil {
		log.Error().Err(err).Msg("failed to insert accent")
	}
}

// onPickerCanceled is called when the user cancels the picker (Escape).
func (uc *InsertAccentUseCase) onPickerCanceled(ctx context.Context) {
	log := logging.FromContext(ctx)

	uc.mu.Lock()
	uc.pickerVisible = false
	uc.pressedChar = 0
	uc.repeatCount = 0
	uc.targetInput = nil
	uc.mu.Unlock()

	uc.accentPicker.Hide()

	log.Debug().Msg("accent picker canceled")
}

// cancelTimerLocked cancels the long-press timer.
// Must be called with uc.mu held.
func (uc *InsertAccentUseCase) cancelTimerLocked() {
	if uc.timer != nil {
		uc.timer.Stop()
		uc.timer = nil
	}
	uc.pressedChar = 0
	uc.repeatCount = 0
	uc.targetInput = nil
}
