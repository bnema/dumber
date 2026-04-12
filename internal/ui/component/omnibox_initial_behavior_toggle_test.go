package component

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/gdk"
)

func TestHandleKeyPress_CtrlRTriggersInitialBehaviorToggle(t *testing.T) {
	o := &Omnibox{initialBehavior: entity.OmniboxInitialBehaviorRecent, viewMode: ViewModeHistory}
	saveCalls := 0
	o.saveInitialBehaviorFn = func(context.Context, entity.OmniboxInitialBehavior) error {
		saveCalls++
		return nil
	}

	var refreshCalls int
	origLoadInitialHistoryFn := loadInitialHistoryFn
	loadInitialHistoryFn = func(*Omnibox, uint64) {
		refreshCalls++
	}
	defer func() {
		loadInitialHistoryFn = origLoadInitialHistoryFn
	}()

	if got := o.handleKeyPress(uint(gdk.KEY_r), 0, gdk.ControlMaskValue); !got {
		t.Fatalf("handleKeyPress Ctrl+R = false, want true")
	}
	if got := o.initialBehavior; got != entity.OmniboxInitialBehaviorMostVisited {
		t.Fatalf("initialBehavior = %q, want %q", got, entity.OmniboxInitialBehaviorMostVisited)
	}
	if saveCalls != 1 {
		t.Fatalf("saveInitialBehavior calls = %d, want 1", saveCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("loadInitialHistoryFn calls = %d, want 1", refreshCalls)
	}
}

func TestHandleKeyPress_CtrlRIgnoredWhenBehaviorUnsupported(t *testing.T) {
	o := &Omnibox{initialBehavior: entity.OmniboxInitialBehavior("unsupported")}
	saveCalls := 0
	o.saveInitialBehaviorFn = func(context.Context, entity.OmniboxInitialBehavior) error {
		saveCalls++
		return nil
	}

	if got := o.handleKeyPress(uint(gdk.KEY_r), 0, gdk.ControlMaskValue); got {
		t.Fatalf("handleKeyPress Ctrl+R = true, want false")
	}
	if got := o.initialBehavior; got != entity.OmniboxInitialBehavior("unsupported") {
		t.Fatalf("initialBehavior = %q, want %q", got, entity.OmniboxInitialBehavior("unsupported"))
	}
	if saveCalls != 0 {
		t.Fatalf("saveInitialBehavior calls = %d, want 0", saveCalls)
	}
}
