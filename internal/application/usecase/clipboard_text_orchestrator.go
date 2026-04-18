package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bnema/dumber/internal/application/dto"
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
	source dto.ClipboardSource
	viewID uint64
}

type explicitCopyState struct {
	text   string
	action string
	at     time.Time
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
	input dto.SelectionClipboardInput,
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
	selectionState := input.Text
	explicitInput := dto.ExplicitClipboardInput{SourceEngine: input.SourceEngine, ViewID: input.ViewID, Action: "copy"}
	explicitState := explicitCopyState{text: input.Text, action: explicitInput.Action, at: currentTime}
	uc.lastSelection[scope] = selectionState
	uc.lastExplicit[explicitScope(explicitInput)] = explicitState
	uc.mu.Unlock()

	if err := uc.clipboard.WriteText(ctx, input.Text); err != nil {
		uc.mu.Lock()
		if uc.lastSelection[scope] == selectionState {
			delete(uc.lastSelection, scope)
		}
		if state, ok := uc.lastExplicit[explicitScope(explicitInput)]; ok && state == explicitState {
			delete(uc.lastExplicit, explicitScope(explicitInput))
		}
		uc.mu.Unlock()
		return fmt.Errorf("clipboard write failed: %w", err)
	}

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
	input dto.ExplicitClipboardInput,
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

	state := explicitCopyState{text: input.Text, action: input.Action, at: currentTime}

	uc.mu.Lock()
	if previous, ok := uc.lastExplicit[scope]; ok &&
		input.Text == previous.text &&
		input.Action == previous.action &&
		currentTime.Sub(previous.at) < explicitClipboardDedupWindow {
		uc.mu.Unlock()
		return nil
	}
	if !input.NativeHandled && uc.clipboard == nil {
		uc.mu.Unlock()
		return fmt.Errorf("clipboard not available")
	}
	uc.lastExplicit[scope] = state
	uc.mu.Unlock()

	if !input.NativeHandled {
		if err := uc.clipboard.WriteText(ctx, input.Text); err != nil {
			uc.mu.Lock()
			if current, ok := uc.lastExplicit[scope]; ok && current == state {
				delete(uc.lastExplicit, scope)
			}
			uc.mu.Unlock()
			return fmt.Errorf("clipboard write failed: %w", err)
		}
	}

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

func selectionScope(input dto.SelectionClipboardInput) clipboardScope {
	return clipboardScope{source: input.SourceEngine, viewID: input.ViewID}
}

func explicitScope(input dto.ExplicitClipboardInput) clipboardScope {
	return clipboardScope{source: input.SourceEngine, viewID: input.ViewID}
}
