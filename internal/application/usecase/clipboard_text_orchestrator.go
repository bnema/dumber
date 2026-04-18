package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

const explicitClipboardDedupWindow = 250 * time.Millisecond

// ClipboardTextOrchestratorUseCase applies shared clipboard business rules.
type ClipboardTextOrchestratorUseCase struct {
	clipboard      port.Clipboard
	autoCopyConfig port.AutoCopyConfig
	toast          func(textLen int)
	mu             sync.Mutex
	lastSelection  map[clipboardScope]string
	lastExplicit   map[clipboardScope]explicitCopyState
	now            func() time.Time
}

type clipboardScope struct {
	source port.ClipboardSource
	viewID port.WebViewID
}

type explicitCopyState struct {
	text string
	at   time.Time
}

var _ port.ClipboardTextOrchestrator = (*ClipboardTextOrchestratorUseCase)(nil)

// NewClipboardTextOrchestrator creates a shared clipboard orchestrator.
func NewClipboardTextOrchestrator(
	clipboard port.Clipboard,
	autoCopyConfig port.AutoCopyConfig,
	toast func(textLen int),
) *ClipboardTextOrchestratorUseCase {
	return &ClipboardTextOrchestratorUseCase{
		clipboard:      clipboard,
		autoCopyConfig: autoCopyConfig,
		toast:          toast,
		lastSelection:  make(map[clipboardScope]string),
		lastExplicit:   make(map[clipboardScope]explicitCopyState),
		now:            time.Now,
	}
}

// HandleSelectionUpdate applies auto-copy selection rules.
func (uc *ClipboardTextOrchestratorUseCase) HandleSelectionUpdate(
	ctx context.Context,
	input port.SelectionClipboardInput,
) error {
	if uc == nil || uc.autoCopyConfig == nil || !uc.autoCopyConfig.IsAutoCopyEnabled() {
		if uc != nil {
			uc.mu.Lock()
			delete(uc.lastSelection, selectionScope(input))
			uc.mu.Unlock()
		}
		return nil
	}

	if input.Text == "" {
		uc.mu.Lock()
		delete(uc.lastSelection, selectionScope(input))
		uc.mu.Unlock()
		return nil
	}

	if utf8.RuneCountInString(strings.TrimSpace(input.Text)) < 2 {
		return nil
	}
	textLen := utf8.RuneCountInString(input.Text)
	scope := selectionScope(input)
	now := uc.now
	if now == nil {
		now = time.Now
	}
	currentTime := now()

	uc.mu.Lock()
	if input.Text == uc.lastSelection[scope] {
		uc.mu.Unlock()
		return nil
	}
	if uc.clipboard == nil {
		uc.mu.Unlock()
		return fmt.Errorf("clipboard not available")
	}
	if err := uc.clipboard.WriteText(ctx, input.Text); err != nil {
		uc.mu.Unlock()
		return fmt.Errorf("clipboard write failed: %w", err)
	}

	uc.lastSelection[scope] = input.Text
	explicitInput := port.ExplicitClipboardInput{SourceEngine: input.SourceEngine, ViewID: input.ViewID}
	uc.lastExplicit[explicitScope(explicitInput)] = explicitCopyState{
		text: input.Text,
		at:   currentTime,
	}
	uc.mu.Unlock()

	if uc.toast != nil {
		uc.toast(textLen)
	}
	logging.FromContext(ctx).Debug().
		Int("text_len", textLen).
		Str("source_engine", string(input.SourceEngine)).
		Msg("clipboard selection copied")
	return nil
}

// HandleExplicitCopy applies explicit copy rules.
func (uc *ClipboardTextOrchestratorUseCase) HandleExplicitCopy(
	ctx context.Context,
	input port.ExplicitClipboardInput,
) error {
	if uc == nil || input.Text == "" {
		return nil
	}

	now := uc.now
	if now == nil {
		now = time.Now
	}
	currentTime := now()
	textLen := utf8.RuneCountInString(input.Text)
	scope := explicitScope(input)

	uc.mu.Lock()
	if state, ok := uc.lastExplicit[scope]; ok &&
		input.Text == state.text &&
		currentTime.Sub(state.at) < explicitClipboardDedupWindow {
		uc.mu.Unlock()
		return nil
	}
	if !input.NativeHandled && uc.clipboard == nil {
		uc.mu.Unlock()
		return fmt.Errorf("clipboard not available")
	}
	if !input.NativeHandled {
		if err := uc.clipboard.WriteText(ctx, input.Text); err != nil {
			uc.mu.Unlock()
			return fmt.Errorf("clipboard write failed: %w", err)
		}
	}

	uc.lastExplicit[scope] = explicitCopyState{text: input.Text, at: currentTime}
	uc.mu.Unlock()

	if uc.toast != nil {
		uc.toast(textLen)
	}
	logging.FromContext(ctx).Debug().
		Int("text_len", textLen).
		Str("source_engine", string(input.SourceEngine)).
		Str("action", input.Action).
		Bool("native_handled", input.NativeHandled).
		Msg("clipboard explicit copied")
	return nil
}

func selectionScope(input port.SelectionClipboardInput) clipboardScope {
	return clipboardScope{source: input.SourceEngine, viewID: input.ViewID}
}

func explicitScope(input port.ExplicitClipboardInput) clipboardScope {
	return clipboardScope{source: input.SourceEngine, viewID: input.ViewID}
}
