package ui

import (
	"github.com/bnema/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/ui/input"
)

// fakeModeToasterView records what the mode toaster asks its view to paint.
type fakeModeToasterView struct {
	header      string
	headers     []string // every distinct header, in order
	mode        input.Mode
	chevron     chevronState
	pillShown   bool
	interactive bool
	bodyShown   bool
	lingering   bool
	animations  bool
	style       string
	groups      []legendGroup
	dimmed      map[string]bool
	flashed     map[string]bool
	flashable   map[string]bool
	rect        func() (int, int, int, int, bool)
	destroyed   bool
}

func newFakeModeToasterView() *fakeModeToasterView {
	return &fakeModeToasterView{dimmed: map[string]bool{}, flashed: map[string]bool{}, flashable: map[string]bool{}}
}

func (f *fakeModeToasterView) setHeader(text string) {
	if text != f.header {
		f.headers = append(f.headers, text)
	}
	f.header = text
}
func (f *fakeModeToasterView) setMode(mode input.Mode)          { f.mode = mode }
func (f *fakeModeToasterView) setChevron(c chevronState)        { f.chevron = c }
func (f *fakeModeToasterView) setPillVisible(v bool)            { f.pillShown = v }
func (f *fakeModeToasterView) setPillInteractive(v bool)        { f.interactive = v }
func (f *fakeModeToasterView) setStyle(style string)            { f.style = style }
func (f *fakeModeToasterView) setAnimations(v bool)             { f.animations = v }
func (f *fakeModeToasterView) setBodyVisible(v bool)            { f.bodyShown = v }
func (f *fakeModeToasterView) setBodyLingering(v bool)          { f.lingering = v }
func (f *fakeModeToasterView) setActionDimmed(a string, d bool) { f.dimmed[a] = d }
func (f *fakeModeToasterView) clearFlash()                      { f.flashed = map[string]bool{} }
func (f *fakeModeToasterView) destroy()                         { f.destroyed = true }
func (f *fakeModeToasterView) setLegend(groups []legendGroup) {
	f.groups = groups
	f.flashable = map[string]bool{}
	for _, group := range groups {
		for _, row := range group.Rows {
			f.flashable[row.Action] = true
		}
	}
}

func (f *fakeModeToasterView) flashAction(action string) bool {
	if !f.flashable[action] {
		return false
	}
	f.flashed = map[string]bool{action: true}
	return true
}

func (f *fakeModeToasterView) lingerRect() (int, int, int, int, bool) {
	if f.rect == nil {
		return 0, 0, 0, 0, false
	}
	return f.rect()
}

// newFakeModeToasterWindow returns a window whose mode toaster paints into a
// fake view.
func newFakeModeToasterWindow(id string) (*browserWindow, *fakeModeToasterView) {
	view := newFakeModeToasterView()
	return &browserWindow{id: id, modeToaster: newModeToasterWithView(view, nil)}, view
}

// pumpMainContext runs pending GLib sources (show delays, flash timers).
func pumpMainContext() {
	ctx := glib.MainContextDefault()
	for ctx.Pending() {
		ctx.Iteration(false)
	}
}
