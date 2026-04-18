package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

type blockingClipboard struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	writes  []string
}

type countingClipboard struct {
	mu     sync.Mutex
	writes int
}

type staticAutoCopyConfig struct {
	enabled bool
}

func (s *staticAutoCopyConfig) IsAutoCopyEnabled() bool { return s.enabled }

func newBlockingClipboard() *blockingClipboard {
	return &blockingClipboard{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
}

func (b *blockingClipboard) WriteText(_ context.Context, text string) error {
	b.mu.Lock()
	b.writes = append(b.writes, text)
	b.mu.Unlock()
	b.started <- struct{}{}
	<-b.release
	return nil
}

func (*blockingClipboard) WriteImage(context.Context, entity.ImageData) error { return nil }
func (*blockingClipboard) ReadText(context.Context) (string, error)           { return "", nil }
func (*blockingClipboard) Clear(context.Context) error                        { return nil }
func (*blockingClipboard) HasText(context.Context) (bool, error)              { return false, nil }

func (b *blockingClipboard) writeCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.writes)
}

func (c *countingClipboard) WriteText(_ context.Context, _ string) error {
	c.mu.Lock()
	c.writes++
	c.mu.Unlock()
	return nil
}

func (*countingClipboard) WriteImage(context.Context, entity.ImageData) error { return nil }
func (*countingClipboard) ReadText(context.Context) (string, error)           { return "", nil }
func (*countingClipboard) Clear(context.Context) error                        { return nil }
func (*countingClipboard) HasText(context.Context) (bool, error)              { return false, nil }

func (c *countingClipboard) writeCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writes
}

func TestClipboardTextOrchestrator_HandleSelectionUpdate_AutoCopyDisabledDoesNothing(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: false}
	var toastCalls []int

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "hello", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate() error = %v, want nil", err)
	}

	if len(toastCalls) != 0 {
		t.Fatalf("toast calls = %v, want none", toastCalls)
	}
}

func TestClipboardTextOrchestrator_HandleSelectionUpdate_WritesOncePerUniqueSelectionAndResetsAfterEmptySelection(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}
	var toastCalls []int

	clipboard.EXPECT().WriteText(ctx, "éé").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "é", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate(short) error = %v, want nil", err)
	}
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate(first) error = %v, want nil", err)
	}
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate(duplicate) error = %v, want nil", err)
	}
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate(reset) error = %v, want nil", err)
	}
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate(after reset) error = %v, want nil", err)
	}

	if len(toastCalls) != 2 || toastCalls[0] != 2 || toastCalls[1] != 2 {
		t.Fatalf("toast calls = %v, want [2 2]", toastCalls)
	}
}

func TestClipboardTextOrchestrator_HandleSelectionUpdate_DoesNotDedupAcrossEngines(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	clipboard.EXPECT().WriteText(ctx, "shared").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, nil)

	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceWebKit}); err != nil {
		t.Fatalf("HandleSelectionUpdate(webkit) error = %v, want nil", err)
	}
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF}); err != nil {
		t.Fatalf("HandleSelectionUpdate(cef) error = %v, want nil", err)
	}
}

func TestClipboardTextOrchestrator_HandleSelectionUpdate_DoesNotDedupAcrossViews(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	clipboard.EXPECT().WriteText(ctx, "shared").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, nil)

	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, ViewID: 1}); err != nil {
		t.Fatalf("HandleSelectionUpdate(view 1) error = %v, want nil", err)
	}
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, ViewID: 2}); err != nil {
		t.Fatalf("HandleSelectionUpdate(view 2) error = %v, want nil", err)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_DeduplicatesIdenticalPayloadsWithinWindow(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}
	var toastCalls []int

	clipboard.EXPECT().WriteText(ctx, "éé").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	now := time.Unix(0, 0)
	uc.now = func() time.Time { return now }

	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceWebKit, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(first) error = %v, want nil", err)
	}
	now = now.Add(100 * time.Millisecond)
	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceWebKit, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(duplicate) error = %v, want nil", err)
	}
	now = now.Add(151 * time.Millisecond)
	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceWebKit, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(after window) error = %v, want nil", err)
	}

	if len(toastCalls) != 2 || toastCalls[0] != 2 || toastCalls[1] != 2 {
		t.Fatalf("toast calls = %v, want [2 2]", toastCalls)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_NativeHandledSkipsBackendWrite(t *testing.T) {
	ctx := context.Background()
	clipboard := &countingClipboard{}
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}
	var toastCalls []int

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceWebKit, Action: "copy", NativeHandled: true}); err != nil {
		t.Fatalf("HandleExplicitCopy() error = %v, want nil", err)
	}

	if got := clipboard.writeCount(); got != 0 {
		t.Fatalf("clipboard writes = %d, want 0", got)
	}
	if len(toastCalls) != 1 || toastCalls[0] != 6 {
		t.Fatalf("toast calls = %v, want [6]", toastCalls)
	}
	if got := len(uc.lastExplicit); got != 1 {
		t.Fatalf("lastExplicit entries = %d, want 1", got)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_DeduplicatesRecentAutoCopySelection(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}
	var toastCalls []int

	clipboard.EXPECT().WriteText(ctx, "shared").Return(nil).Once()

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	now := time.Unix(0, 0)
	uc.now = func() time.Time { return now }

	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, ViewID: 1}); err != nil {
		t.Fatalf("HandleSelectionUpdate() error = %v, want nil", err)
	}
	now = now.Add(100 * time.Millisecond)
	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, ViewID: 1, Action: "cut"}); err != nil {
		t.Fatalf("HandleExplicitCopy() error = %v, want nil", err)
	}

	if len(toastCalls) != 1 || toastCalls[0] != 6 {
		t.Fatalf("toast calls = %v, want [6]", toastCalls)
	}
	if got := len(uc.lastExplicit); got != 1 {
		t.Fatalf("lastExplicit entries = %d, want 1", got)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_DoesNotDedupAcrossEngines(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	clipboard.EXPECT().WriteText(ctx, "shared").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, nil)
	uc.now = func() time.Time { return time.Unix(0, 0) }

	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceWebKit, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(webkit) error = %v, want nil", err)
	}
	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(cef) error = %v, want nil", err)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_DoesNotDedupAcrossViews(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	clipboard.EXPECT().WriteText(ctx, "shared").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, nil)
	uc.now = func() time.Time { return time.Unix(0, 0) }

	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, ViewID: 1, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(view 1) error = %v, want nil", err)
	}
	if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "shared", SourceEngine: port.ClipboardSourceCEF, ViewID: 2, Action: "copy"}); err != nil {
		t.Fatalf("HandleExplicitCopy(view 2) error = %v, want nil", err)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_KeepsBoundedStatePerScope(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	clipboard.EXPECT().WriteText(ctx, "one").Return(nil).Once()
	clipboard.EXPECT().WriteText(ctx, "two").Return(nil).Once()
	clipboard.EXPECT().WriteText(ctx, "three").Return(nil).Once()

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, nil)

	for i, text := range []string{"one", "two", "three"} {
		advancedTime := time.Unix(0, 0).Add(time.Duration(i) * time.Second)
		uc.now = func() time.Time { return advancedTime }
		if err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: text, SourceEngine: port.ClipboardSourceCEF, ViewID: 7, Action: "copy"}); err != nil {
			t.Fatalf("HandleExplicitCopy(%q) error = %v, want nil", text, err)
		}
	}

	if got := len(uc.lastExplicit); got != 1 {
		t.Fatalf("lastExplicit entries = %d, want 1", got)
	}
}

func TestClipboardTextOrchestrator_HandleSelectionUpdate_ReenableClearsDedupState(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	clipboard.EXPECT().WriteText(ctx, "toggle").Return(nil).Times(2)

	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, nil)

	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "toggle", SourceEngine: port.ClipboardSourceCEF, ViewID: 1}); err != nil {
		t.Fatalf("HandleSelectionUpdate(enabled) error = %v, want nil", err)
	}
	autoCopyConfig.enabled = false
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "toggle", SourceEngine: port.ClipboardSourceCEF, ViewID: 1}); err != nil {
		t.Fatalf("HandleSelectionUpdate(disabled) error = %v, want nil", err)
	}
	autoCopyConfig.enabled = true
	if err := uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "toggle", SourceEngine: port.ClipboardSourceCEF, ViewID: 1}); err != nil {
		t.Fatalf("HandleSelectionUpdate(reenabled) error = %v, want nil", err)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_ClipboardFailureDoesNotToast(t *testing.T) {
	ctx := context.Background()
	clipboard := portmocks.NewMockClipboard(t)
	clipboard.EXPECT().WriteText(ctx, "éé").Return(errors.New("clipboard failed")).Once()
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}

	var toastCalls []int
	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	err := uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceCEF, Action: "copy"})
	if err == nil {
		t.Fatal("HandleExplicitCopy() error = nil, want non-nil")
	}
	if len(toastCalls) != 0 {
		t.Fatalf("toast calls = %v, want none", toastCalls)
	}
}

func TestClipboardTextOrchestrator_HandleSelectionUpdate_DedupIsAtomic(t *testing.T) {
	ctx := context.Background()
	clipboard := newBlockingClipboard()
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}
	var toastCalls []int
	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})

	done1 := make(chan error, 1)
	go func() {
		done1 <- uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceCEF})
	}()
	<-clipboard.started

	done2 := make(chan error, 1)
	go func() {
		done2 <- uc.HandleSelectionUpdate(ctx, port.SelectionClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceCEF})
	}()

	select {
	case <-clipboard.started:
		t.Fatal("second selection write started before first completed")
	default:
	}

	close(clipboard.release)

	if err := <-done1; err != nil {
		t.Fatalf("first HandleSelectionUpdate() error = %v, want nil", err)
	}
	if err := <-done2; err != nil {
		t.Fatalf("second HandleSelectionUpdate() error = %v, want nil", err)
	}

	if got := clipboard.writeCount(); got != 1 {
		t.Fatalf("clipboard writes = %d, want 1", got)
	}
	if len(toastCalls) != 1 || toastCalls[0] != 2 {
		t.Fatalf("toast calls = %v, want [2]", toastCalls)
	}
}

func TestClipboardTextOrchestrator_HandleExplicitCopy_DedupIsAtomic(t *testing.T) {
	ctx := context.Background()
	clipboard := newBlockingClipboard()
	autoCopyConfig := &staticAutoCopyConfig{enabled: true}
	var toastCalls []int
	uc := NewClipboardTextOrchestrator(clipboard, autoCopyConfig, func(textLen int) {
		toastCalls = append(toastCalls, textLen)
	})
	uc.now = func() time.Time { return time.Unix(0, 0) }

	done1 := make(chan error, 1)
	go func() {
		done1 <- uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceWebKit, Action: "copy"})
	}()
	<-clipboard.started

	done2 := make(chan error, 1)
	go func() {
		done2 <- uc.HandleExplicitCopy(ctx, port.ExplicitClipboardInput{Text: "éé", SourceEngine: port.ClipboardSourceWebKit, Action: "copy"})
	}()

	select {
	case <-clipboard.started:
		t.Fatal("second explicit write started before first completed")
	default:
	}

	close(clipboard.release)

	if err := <-done1; err != nil {
		t.Fatalf("first HandleExplicitCopy() error = %v, want nil", err)
	}
	if err := <-done2; err != nil {
		t.Fatalf("second HandleExplicitCopy() error = %v, want nil", err)
	}

	if got := clipboard.writeCount(); got != 1 {
		t.Fatalf("clipboard writes = %d, want 1", got)
	}
	if len(toastCalls) != 1 || toastCalls[0] != 2 {
		t.Fatalf("toast calls = %v, want [2]", toastCalls)
	}
}
