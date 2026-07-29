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

	if wv.lastRequest.Command != port.PageScrollCommandDown {
		t.Fatalf("ScrollPage request.Command = %d, want %d", wv.lastRequest.Command, port.PageScrollCommandDown)
	}
	if wv.lastRequest.FallbackDX != 0 || wv.lastRequest.FallbackDY != 80 {
		t.Fatalf("ScrollPage request fallback delta = (%d, %d), want (0, 80)", wv.lastRequest.FallbackDX, wv.lastRequest.FallbackDY)
	}
}

func TestPageScroll_FastVerticalUp(t *testing.T) {
	wv := newScrollableStub(nil)
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), wv, PageScrollUpFast)
	if err != nil {
		t.Fatalf("Scroll() unexpected error: %v", err)
	}

	if wv.lastRequest.Command != port.PageScrollCommandUpFast {
		t.Fatalf("ScrollPage request.Command = %d, want %d", wv.lastRequest.Command, port.PageScrollCommandUpFast)
	}
	if wv.lastRequest.FallbackDX != 0 || wv.lastRequest.FallbackDY != -320 {
		t.Fatalf("ScrollPage request fallback delta = (%d, %d), want (0, -320)", wv.lastRequest.FallbackDX, wv.lastRequest.FallbackDY)
	}
}

func TestPageScroll_HorizontalDirectionMapping(t *testing.T) {
	t.Run("left", func(t *testing.T) {
		wv := newScrollableStub(nil)
		uc := NewPageScrollUseCase()

		if err := uc.Scroll(context.Background(), wv, PageScrollLeft); err != nil {
			t.Fatalf("Scroll() unexpected error: %v", err)
		}

		if wv.lastRequest.Command != port.PageScrollCommandLeft {
			t.Fatalf("ScrollPage request.Command = %d, want %d", wv.lastRequest.Command, port.PageScrollCommandLeft)
		}
		if wv.lastRequest.FallbackDX != -80 || wv.lastRequest.FallbackDY != 0 {
			t.Fatalf("ScrollPage request fallback delta = (%d, %d), want (-80, 0)", wv.lastRequest.FallbackDX, wv.lastRequest.FallbackDY)
		}
	})

	t.Run("right", func(t *testing.T) {
		wv := newScrollableStub(nil)
		uc := NewPageScrollUseCase()
		if err := uc.Scroll(context.Background(), wv, PageScrollRight); err != nil {
			t.Fatalf("Scroll() unexpected error: %v", err)
		}

		if wv.lastRequest.Command != port.PageScrollCommandRight {
			t.Fatalf("ScrollPage request.Command = %d, want %d", wv.lastRequest.Command, port.PageScrollCommandRight)
		}
		if wv.lastRequest.FallbackDX != 80 || wv.lastRequest.FallbackDY != 0 {
			t.Fatalf("ScrollPage request fallback delta = (%d, %d), want (80, 0)", wv.lastRequest.FallbackDX, wv.lastRequest.FallbackDY)
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
	if err.Error() != "page scroll: webview does not support page scrolling" {
		t.Fatalf("Scroll() error = %q, want %q", err.Error(), "page scroll: webview does not support page scrolling")
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
		if err.Error() != "page scroll: webview does not support page scrolling" {
			t.Fatalf("Scroll(command=%s) error = %q, want %q", cmd, err.Error(), "page scroll: webview does not support page scrolling")
		}
	}
}

func TestPageScroll_NilWebView_StableFailure(t *testing.T) {
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), nil, PageScrollDown)
	if err == nil {
		t.Fatal("Scroll() expected error for nil webview, got nil")
	}
	if err.Error() != "page scroll: nil webview" {
		t.Fatalf("Scroll() error = %q, want %q", err.Error(), "page scroll: nil webview")
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

func TestPageScroll_TapDistancesRemainFixed(t *testing.T) {
	tests := []struct {
		name string
		cmd  PageScrollCommand
		dx   int
		dy   int
	}{
		{"left 80 px", PageScrollLeft, -80, 0},
		{"right 80 px", PageScrollRight, 80, 0},
		{"up 80 px", PageScrollUp, 0, -80},
		{"down 80 px", PageScrollDown, 0, 80},
		{"up fast 320 px", PageScrollUpFast, 0, -320},
		{"down fast 320 px", PageScrollDownFast, 0, 320},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wv := newScrollableStub(nil)
			if err := NewPageScrollUseCase().Scroll(context.Background(), wv, tt.cmd); err != nil {
				t.Fatal(err)
			}
			if wv.lastRequest.FallbackDX != tt.dx || wv.lastRequest.FallbackDY != tt.dy {
				t.Fatalf("tap delta=(%d,%d), want (%d,%d)", wv.lastRequest.FallbackDX, wv.lastRequest.FallbackDY, tt.dx, tt.dy)
			}
			if wv.lastRequest.Continuous {
				t.Fatal("tap request must not be marked continuous")
			}
		})
	}
}

func TestPageScroll_ScrollContinuousMarksRequest(t *testing.T) {
	wv := newScrollableStub(nil)
	if err := NewPageScrollUseCase().ScrollContinuous(context.Background(), wv, PageScrollDown); err != nil {
		t.Fatal(err)
	}
	if !wv.lastRequest.Continuous || wv.lastRequest.FallbackDY != 80 {
		t.Fatalf("continuous request = %+v, want continuous 80 px", wv.lastRequest)
	}
}

func TestPageScroll_StopCancelsOptionalCapability(t *testing.T) {
	wv := &cancelableScrollableStub{scrollableStub: newScrollableStub(nil)}
	if err := NewPageScrollUseCase().Stop(context.Background(), wv); err != nil {
		t.Fatal(err)
	}
	if wv.cancelCalls != 1 {
		t.Fatalf("cancel calls=%d, want 1", wv.cancelCalls)
	}
	if err := NewPageScrollUseCase().Stop(context.Background(), newScrollableStub(nil)); err != nil {
		t.Fatalf("non-cancelable WebKit-style adapter should be a no-op: %v", err)
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

func TestPageScroll_CommandIdentityPropagation(t *testing.T) {
	// Prove that PageScrollDownFast forwards command identity separately
	// from fallback delta (dy=320).
	wv := newScrollableStub(nil)
	uc := NewPageScrollUseCase()

	err := uc.Scroll(context.Background(), wv, PageScrollDownFast)
	if err != nil {
		t.Fatalf("Scroll() unexpected error: %v", err)
	}

	if wv.lastRequest.Command != port.PageScrollCommandDownFast {
		t.Fatalf("ScrollPage request.Command = %d, want %d (PageScrollDownFast)",
			wv.lastRequest.Command, port.PageScrollCommandDownFast)
	}
	if int(wv.lastRequest.Command) == wv.lastRequest.FallbackDY {
		t.Fatal("command identity must NOT be confused with fallback delta: Command and FallbackDY should differ")
	}
	if wv.lastRequest.FallbackDX != 0 || wv.lastRequest.FallbackDY != 320 {
		t.Fatalf("ScrollPage request fallback delta = (%d, %d), want (0, 320)",
			wv.lastRequest.FallbackDX, wv.lastRequest.FallbackDY)
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
// and port.PageScrollable (via ScrollPage).
type scrollableStub struct {
	*nonScrollableStub
	lastRequest port.PageScrollRequest
	scrollErr   error
}

type cancelableScrollableStub struct {
	*scrollableStub
	cancelCalls int
}

func (s *cancelableScrollableStub) CancelPageScroll(context.Context) {
	s.cancelCalls++
}

func newScrollableStub(scrollErr error) *scrollableStub {
	return &scrollableStub{
		nonScrollableStub: &nonScrollableStub{},
		scrollErr:         scrollErr,
	}
}

// ScrollPage implements port.PageScrollable.
func (s *scrollableStub) ScrollPage(_ context.Context, req port.PageScrollRequest) error {
	s.lastRequest = req
	return s.scrollErr
}

// nonScrollableStub satisfies port.WebView but NOT port.PageScrollable.
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
