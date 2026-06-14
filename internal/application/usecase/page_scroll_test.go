package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestPageScroll_SmallVerticalDown(t *testing.T) {
	wv := newScrollableStub(nil)
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), wv, PageScrollDown)
	if err != nil {
		t.Fatalf("Scroll() unexpected error: %v", err)
	}

	wantDx, wantDy := 0, 80
	if wv.lastDx != wantDx || wv.lastDy != wantDy {
		t.Fatalf("ScrollBy called with (%d, %d), want (%d, %d)", wv.lastDx, wv.lastDy, wantDx, wantDy)
	}
}

func TestPageScroll_FastVerticalUp(t *testing.T) {
	wv := newScrollableStub(nil)
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), wv, PageScrollUpFast)
	if err != nil {
		t.Fatalf("Scroll() unexpected error: %v", err)
	}

	wantDx, wantDy := 0, -320
	if wv.lastDx != wantDx || wv.lastDy != wantDy {
		t.Fatalf("ScrollBy called with (%d, %d), want (%d, %d)", wv.lastDx, wv.lastDy, wantDx, wantDy)
	}
}

func TestPageScroll_HorizontalDirectionMapping(t *testing.T) {
	t.Run("left", func(t *testing.T) {
		wv := newScrollableStub(nil)
		uc := NewPageScrollUseCase()

		if err := uc.Scroll(context.Background(), wv, PageScrollLeft); err != nil {
			t.Fatalf("Scroll() unexpected error: %v", err)
		}

		wantDx, wantDy := -80, 0
		if wv.lastDx != wantDx || wv.lastDy != wantDy {
			t.Fatalf("ScrollBy called with (%d, %d), want (%d, %d)", wv.lastDx, wv.lastDy, wantDx, wantDy)
		}
	})

	t.Run("right", func(t *testing.T) {
		wv := newScrollableStub(nil)
		uc := NewPageScrollUseCase()
		if err := uc.Scroll(context.Background(), wv, PageScrollRight); err != nil {
			t.Fatalf("Scroll() unexpected error: %v", err)
		}

		wantDx, wantDy := 80, 0
		if wv.lastDx != wantDx || wv.lastDy != wantDy {
			t.Fatalf("ScrollBy called with (%d, %d), want (%d, %d)", wv.lastDx, wv.lastDy, wantDx, wantDy)
		}
	})
}

func TestPageScroll_UnsupportedWebView(t *testing.T) {
	wv := &nonScrollableStub{}
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), wv, PageScrollDown)
	if err == nil {
		t.Fatal("Scroll() expected error for unsupported webview, got nil")
	}
}

func TestPageScroll_UnsupportedWebView_AllCommandsStable(t *testing.T) {
	wv := &nonScrollableStub{}
	uc := NewPageScrollUseCase()

	cmds := []PageScrollCommand{
		PageScrollLeft,
		PageScrollRight,
		PageScrollUp,
		PageScrollDown,
		PageScrollUpFast,
		PageScrollDownFast,
	}

	for _, cmd := range cmds {
		err := uc.Scroll(context.Background(), wv, cmd)
		if err == nil {
			t.Fatalf("Scroll(command=%s) expected error for unsupported webview, got nil", cmd)
		}
	}
}

func TestPageScroll_NilWebView_StableFailure(t *testing.T) {
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), nil, PageScrollDown)
	if err == nil {
		t.Fatal("Scroll() expected error for nil webview, got nil")
	}
}

func TestPageScroll_PropagatesScrollError(t *testing.T) {
	wantErr := errors.New("engine scroll failure")
	wv := newScrollableStub(wantErr)
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), wv, PageScrollDown)
	if err == nil {
		t.Fatal("Scroll() expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Scroll() error = %v, want %v", err, wantErr)
	}
}

func TestPageScroll_DeltaMethod(t *testing.T) {
	tests := []struct {
		cmd      PageScrollCommand
		wantDx   int
		wantDy   int
		wantName string
	}{
		{PageScrollLeft, -80, 0, "left"},
		{PageScrollRight, 80, 0, "right"},
		{PageScrollUp, 0, -80, "up"},
		{PageScrollDown, 0, 80, "down"},
		{PageScrollUpFast, 0, -320, "up-fast"},
		{PageScrollDownFast, 0, 320, "down-fast"},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			dx, dy := scrollDelta(tt.cmd)
			if dx != tt.wantDx || dy != tt.wantDy {
				t.Errorf("scrollDelta(%v) = (%d, %d), want (%d, %d)", tt.cmd, dx, dy, tt.wantDx, tt.wantDy)
			}
			if got := tt.cmd.String(); got != tt.wantName {
				t.Errorf("PageScrollCommand.String() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestPageScroll_UnknownCommand_ReturnsNoOp(t *testing.T) {
	dx, dy := scrollDelta(PageScrollCommand(999))
	if dx != 0 || dy != 0 {
		t.Errorf("scrollDelta(unknown) = (%d, %d), want (0, 0)", dx, dy)
	}

	got := PageScrollCommand(999).String()
	if got != "unknown" {
		t.Errorf("PageScrollCommand(999).String() = %q, want %q", got, "unknown")
	}
}

// --- test helpers ---

// scrollableStub satisfies both port.WebView (via embedded nonScrollableStub)
// and port.Scrollable (via ScrollBy).
type scrollableStub struct {
	*nonScrollableStub
	lastDx, lastDy int
	scrollErr      error
}

func newScrollableStub(scrollErr error) *scrollableStub {
	return &scrollableStub{
		nonScrollableStub: &nonScrollableStub{},
		scrollErr:         scrollErr,
	}
}

// ScrollBy implements port.Scrollable.
func (s *scrollableStub) ScrollBy(_ context.Context, dx, dy int) error {
	s.lastDx = dx
	s.lastDy = dy
	return s.scrollErr
}

// nonScrollableStub satisfies port.WebView but NOT port.Scrollable.
type nonScrollableStub struct{}

func (*nonScrollableStub) ID() port.WebViewID                              { return 0 }
func (*nonScrollableStub) LoadURI(_ context.Context, _ string) error       { return nil }
func (*nonScrollableStub) LoadHTML(_ context.Context, _, _ string) error   { return nil }
func (*nonScrollableStub) Reload(_ context.Context) error                  { return nil }
func (*nonScrollableStub) ReloadBypassCache(_ context.Context) error       { return nil }
func (*nonScrollableStub) Stop(_ context.Context) error                    { return nil }
func (*nonScrollableStub) GoBack(_ context.Context) error                  { return nil }
func (*nonScrollableStub) GoForward(_ context.Context) error               { return nil }
func (*nonScrollableStub) State() port.WebViewState                        { return port.WebViewState{} }
func (*nonScrollableStub) URI() string                                     { return "" }
func (*nonScrollableStub) Title() string                                   { return "" }
func (*nonScrollableStub) IsLoading() bool                                 { return false }
func (*nonScrollableStub) EstimatedProgress() float64                      { return 0 }
func (*nonScrollableStub) CanGoBack() bool                                 { return false }
func (*nonScrollableStub) CanGoForward() bool                              { return false }
func (*nonScrollableStub) SetZoomLevel(_ context.Context, _ float64) error { return nil }
func (*nonScrollableStub) GetZoomLevel() float64                           { return 1.0 }
func (*nonScrollableStub) GetFindController() port.FindController          { return nil }
func (*nonScrollableStub) SetCallbacks(_ *port.WebViewCallbacks)           {}
func (*nonScrollableStub) RunJavaScript(_ context.Context, _ string)       {}
func (*nonScrollableStub) SetBackgroundColor(_, _, _, _ float64)           {}
func (*nonScrollableStub) ResetBackgroundToDefault()                       {}
func (*nonScrollableStub) Favicon() port.Texture                           { return nil }
func (*nonScrollableStub) Generation() uint64                              { return 0 }
func (*nonScrollableStub) IsFullscreen() bool                              { return false }
func (*nonScrollableStub) IsPlayingAudio() bool                            { return false }
func (*nonScrollableStub) IsDestroyed() bool                               { return false }
func (*nonScrollableStub) Destroy()                                        {}
