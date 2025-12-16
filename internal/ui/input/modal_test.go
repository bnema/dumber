package input

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestModalState_InitialState(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	if ms.Mode() != ModeNormal {
		t.Errorf("initial mode = %v, want ModeNormal", ms.Mode())
	}
}

func TestModalState_EnterTabMode(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	ms.EnterTabMode(ctx, 0)

	if ms.Mode() != ModeTab {
		t.Errorf("mode after EnterTabMode = %v, want ModeTab", ms.Mode())
	}
}

func TestModalState_EnterPaneMode(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	ms.EnterPaneMode(ctx, 0)

	if ms.Mode() != ModePane {
		t.Errorf("mode after EnterPaneMode = %v, want ModePane", ms.Mode())
	}
}

func TestModalState_ExitMode(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	// Enter tab mode
	ms.EnterTabMode(ctx, 0)
	if ms.Mode() != ModeTab {
		t.Fatalf("mode should be ModeTab")
	}

	// Exit mode
	ms.ExitMode(ctx)
	if ms.Mode() != ModeNormal {
		t.Errorf("mode after ExitMode = %v, want ModeNormal", ms.Mode())
	}
}

func TestModalState_ExitMode_FromNormal(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	// ExitMode from normal should be no-op
	ms.ExitMode(ctx)

	if ms.Mode() != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal", ms.Mode())
	}
}

func TestModalState_SwitchModes(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	// Enter tab mode
	ms.EnterTabMode(ctx, 0)
	if ms.Mode() != ModeTab {
		t.Fatalf("expected ModeTab, got %v", ms.Mode())
	}

	// Switch to pane mode directly
	ms.EnterPaneMode(ctx, 0)
	if ms.Mode() != ModePane {
		t.Errorf("mode after switch to pane = %v, want ModePane", ms.Mode())
	}

	// Switch back to tab mode
	ms.EnterTabMode(ctx, 0)
	if ms.Mode() != ModeTab {
		t.Errorf("mode after switch to tab = %v, want ModeTab", ms.Mode())
	}
}

func TestModalState_ModeChangeCallback(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	var callCount int
	var lastFrom, lastTo Mode
	var mu sync.Mutex

	ms.SetOnModeChange(func(from, to Mode) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		lastFrom = from
		lastTo = to
	})

	// Enter tab mode
	ms.EnterTabMode(ctx, 0)

	mu.Lock()
	if callCount != 1 {
		t.Errorf("callback count = %d, want 1", callCount)
	}
	if lastFrom != ModeNormal || lastTo != ModeTab {
		t.Errorf("callback args = (%v, %v), want (ModeNormal, ModeTab)", lastFrom, lastTo)
	}
	mu.Unlock()

	// Exit mode
	ms.ExitMode(ctx)

	mu.Lock()
	if callCount != 2 {
		t.Errorf("callback count = %d, want 2", callCount)
	}
	if lastFrom != ModeTab || lastTo != ModeNormal {
		t.Errorf("callback args = (%v, %v), want (ModeTab, ModeNormal)", lastFrom, lastTo)
	}
	mu.Unlock()
}

func TestModalState_NoCallbackWhenAlreadyInMode(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	var callCount int
	ms.SetOnModeChange(func(from, to Mode) {
		callCount++
	})

	// Enter tab mode
	ms.EnterTabMode(ctx, 0)
	if callCount != 1 {
		t.Fatalf("expected 1 callback, got %d", callCount)
	}

	// Enter tab mode again (should not trigger callback)
	ms.EnterTabMode(ctx, 0)
	if callCount != 1 {
		t.Errorf("callback count = %d, want 1 (no new call)", callCount)
	}
}

func TestModalState_Timeout(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	// Enter with short timeout
	ms.EnterTabMode(ctx, 50*time.Millisecond)

	if ms.Mode() != ModeTab {
		t.Fatalf("mode should be ModeTab")
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	if ms.Mode() != ModeNormal {
		t.Errorf("mode after timeout = %v, want ModeNormal", ms.Mode())
	}
}

func TestModalState_TimeoutReset(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	// Enter with timeout
	ms.EnterTabMode(ctx, 100*time.Millisecond)

	// Wait a bit, then reset
	time.Sleep(50 * time.Millisecond)
	ms.ResetTimeout(ctx)

	// Wait a bit more (original timeout would have expired)
	time.Sleep(80 * time.Millisecond)

	// Should still be in tab mode because timeout was reset
	if ms.Mode() != ModeTab {
		t.Errorf("mode = %v, want ModeTab (timeout should have been reset)", ms.Mode())
	}

	// Wait for the reset timeout to expire
	time.Sleep(50 * time.Millisecond)

	if ms.Mode() != ModeNormal {
		t.Errorf("mode after reset timeout = %v, want ModeNormal", ms.Mode())
	}
}

func TestModalState_ExitCancelsTimeout(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	var callCount int
	ms.SetOnModeChange(func(from, to Mode) {
		callCount++
	})

	// Enter with timeout
	ms.EnterTabMode(ctx, 100*time.Millisecond)
	if callCount != 1 {
		t.Fatalf("expected 1 callback")
	}

	// Exit immediately
	ms.ExitMode(ctx)
	if callCount != 2 {
		t.Fatalf("expected 2 callbacks")
	}

	// Wait past the timeout
	time.Sleep(150 * time.Millisecond)

	// Callback should not have been called again
	if callCount != 2 {
		t.Errorf("callback count = %d, want 2 (timeout should have been cancelled)", callCount)
	}
}

func TestModalState_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	ms := NewModalState(ctx)

	var wg sync.WaitGroup
	iterations := 100

	// Multiple goroutines entering/exiting modes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				ms.EnterTabMode(ctx, 0)
				_ = ms.Mode()
				ms.ExitMode(ctx)
				ms.EnterPaneMode(ctx, 0)
				_ = ms.Mode()
				ms.ExitMode(ctx)
			}
		}()
	}

	wg.Wait()

	// Should end up in normal mode
	if ms.Mode() != ModeNormal {
		t.Errorf("final mode = %v, want ModeNormal", ms.Mode())
	}
}

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeNormal, "normal"},
		{ModeTab, "tab"},
		{ModePane, "pane"},
		{Mode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("Mode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
