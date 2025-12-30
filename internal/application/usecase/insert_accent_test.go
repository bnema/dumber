package usecase

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestInsertAccentUseCase_OnKeyPressed_NoAccents(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)

	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) { fn() })

	// Key 'x' has no accents - should return false and not start timer
	handled := uc.OnKeyPressed(ctx, 'x', false)
	assert.False(t, handled, "OnKeyPressed should return false for key without accents")
	assert.False(t, uc.IsPickerVisible(), "picker should not be visible")
}

func TestInsertAccentUseCase_OnKeyPressed_HasAccents(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)
	textInput := mocks.NewMockTextInputTarget(t)

	// GetFocusedInput is called when starting long-press to capture target
	focusProvider.EXPECT().GetFocusedInput().Return(textInput).Maybe()

	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) { fn() })

	// Key 'e' has accents - should return false initially (timer started)
	handled := uc.OnKeyPressed(ctx, 'e', false)
	assert.False(t, handled, "OnKeyPressed should return false while timer is pending")

	// Cancel to clean up timer
	uc.Cancel(ctx)
}

func TestInsertAccentUseCase_OnKeyReleased_CancelsTimer(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)
	textInput := mocks.NewMockTextInputTarget(t)

	// GetFocusedInput is called when starting long-press to capture target
	focusProvider.EXPECT().GetFocusedInput().Return(textInput).Maybe()

	timerFired := false
	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) {
		timerFired = true
		fn()
	})

	// Start long-press detection
	uc.OnKeyPressed(ctx, 'e', false)

	// Release before timer fires
	uc.OnKeyReleased(ctx, 'e')

	// Wait a bit longer than LongPressDelay
	time.Sleep(LongPressDelay + 50*time.Millisecond)

	assert.False(t, timerFired, "timer should have been canceled by key release")
}

func TestInsertAccentUseCase_LongPress_ShowsPicker(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)
	textInput := mocks.NewMockTextInputTarget(t)

	var wg sync.WaitGroup
	wg.Add(1)

	// GetFocusedInput is called when showing picker to delete repeated characters
	focusProvider.EXPECT().GetFocusedInput().Return(textInput).Maybe()
	// DeleteBeforeCursor will be called to delete the initial character + any repeats
	textInput.EXPECT().DeleteBeforeCursor(mock.Anything, mock.AnythingOfType("int")).Return(nil).Maybe()

	// Expect Show to be called with accents for 'e'
	accentPicker.EXPECT().
		Show(mock.MatchedBy(func(accents []rune) bool {
			return len(accents) > 0 && accents[0] == 'è'
		}), mock.AnythingOfType("func(int32)"), mock.AnythingOfType("func()")).
		Run(func(_ []rune, _ func(rune), _ func()) {
			wg.Done()
		}).
		Once()

	// Hide will be called by Cancel
	accentPicker.EXPECT().Hide().Maybe()

	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) { fn() })

	// Start long-press detection
	uc.OnKeyPressed(ctx, 'e', false)

	// Wait for timer to fire
	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Give time for state update

	assert.True(t, uc.IsPickerVisible(), "picker should be visible after long-press")

	// Clean up
	uc.Cancel(ctx)
}

func TestInsertAccentUseCase_Cancel(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)
	textInput := mocks.NewMockTextInputTarget(t)

	// GetFocusedInput is called when starting long-press to capture target
	focusProvider.EXPECT().GetFocusedInput().Return(textInput).Maybe()

	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) { fn() })

	// Start timer
	uc.OnKeyPressed(ctx, 'e', false)

	// Cancel should stop timer
	uc.Cancel(ctx)

	// Wait past the timer duration
	time.Sleep(LongPressDelay + 50*time.Millisecond)

	// Picker should never have been shown
	assert.False(t, uc.IsPickerVisible(), "picker should not be visible after cancel")
}

func TestInsertAccentUseCase_UppercaseAccents(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)
	textInput := mocks.NewMockTextInputTarget(t)

	var wg sync.WaitGroup
	wg.Add(1)

	// GetFocusedInput is called when showing picker to delete repeated characters
	focusProvider.EXPECT().GetFocusedInput().Return(textInput).Maybe()
	// DeleteBeforeCursor will be called to delete the initial character + any repeats
	textInput.EXPECT().DeleteBeforeCursor(mock.Anything, mock.AnythingOfType("int")).Return(nil).Maybe()

	// Expect Show to be called with uppercase accents when shift is held
	accentPicker.EXPECT().
		Show(mock.MatchedBy(func(accents []rune) bool {
			return len(accents) > 0 && accents[0] == 'È' // Uppercase È
		}), mock.AnythingOfType("func(int32)"), mock.AnythingOfType("func()")).
		Run(func(_ []rune, _ func(rune), _ func()) {
			wg.Done()
		}).
		Once()

	// Hide will be called by Cancel
	accentPicker.EXPECT().Hide().Maybe()

	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) { fn() })

	// Start long-press with shift held
	uc.OnKeyPressed(ctx, 'e', true)

	// Wait for timer
	wg.Wait()

	// Clean up
	uc.Cancel(ctx)
}

func TestInsertAccentUseCase_PickerVisibleBlocksNewLongPress(t *testing.T) {
	ctx := context.Background()
	focusProvider := mocks.NewMockFocusedInputProvider(t)
	accentPicker := mocks.NewMockAccentPickerUI(t)
	textInput := mocks.NewMockTextInputTarget(t)

	var showCalled int
	var mu sync.Mutex

	// GetFocusedInput is called when showing picker to delete repeated characters
	focusProvider.EXPECT().GetFocusedInput().Return(textInput).Maybe()
	// DeleteBeforeCursor will be called to delete the initial character + any repeats
	textInput.EXPECT().DeleteBeforeCursor(mock.Anything, mock.AnythingOfType("int")).Return(nil).Maybe()

	accentPicker.EXPECT().
		Show(mock.Anything, mock.Anything, mock.Anything).
		Run(func(_ []rune, _ func(rune), _ func()) {
			mu.Lock()
			showCalled++
			mu.Unlock()
		}).
		Maybe()

	accentPicker.EXPECT().Hide().Maybe()

	uc := NewInsertAccentUseCase(focusProvider, accentPicker, func(fn func()) { fn() })

	// Trigger first long-press
	uc.OnKeyPressed(ctx, 'e', false)
	time.Sleep(LongPressDelay + 50*time.Millisecond)

	// Try to start another long-press while picker is visible
	handled := uc.OnKeyPressed(ctx, 'a', false)
	assert.True(t, handled, "should return true when picker is visible")

	mu.Lock()
	count := showCalled
	mu.Unlock()
	assert.LessOrEqual(t, count, 1, "Show should only be called once")

	uc.Cancel(ctx)
}
