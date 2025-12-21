// Package component provides UI components for the browser.
package component

import (
	"context"
	"fmt"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

const (
	findBarMarginPx   = 8
	findBarRowSpacing = 6
	findBarCountWidth = 6
)

// FindBar is a compact find-in-page UI overlay.
type FindBar struct {
	outerBox     *gtk.Box
	containerBox *gtk.Box
	inputRow     *gtk.Box
	optionsRow   *gtk.Box
	entry        *gtk.SearchEntry
	prevBtn      *gtk.Button
	nextBtn      *gtk.Button
	countLabel   *gtk.Label
	closeBtn     *gtk.Button
	caseToggle   *gtk.ToggleButton
	wordToggle   *gtk.ToggleButton
	hlToggle     *gtk.ToggleButton

	uc *usecase.FindInPageUseCase

	visible bool
	mu      sync.RWMutex
	ctx     context.Context
	onClose func()
}

// FindBarConfig holds configuration for creating a FindBar.
type FindBarConfig struct {
	OnClose           func()
	GetFindController func(paneID entity.PaneID) port.FindController
}

// NewFindBar creates a new FindBar component.
func NewFindBar(ctx context.Context, cfg FindBarConfig) *FindBar {
	log := logging.FromContext(ctx)

	fb := &FindBar{
		ctx:     ctx,
		onClose: cfg.OnClose,
		uc:      usecase.NewFindInPageUseCase(ctx),
	}

	if err := fb.createWidgets(); err != nil {
		log.Error().Err(err).Msg("failed to create find bar widgets")
		return nil
	}

	fb.setupHandlers()
	fb.bindUseCase()

	log.Debug().Msg("find bar created")
	return fb
}

// SetFindController attaches the FindController to the use case.
func (fb *FindBar) SetFindController(controller port.FindController) {
	if fb.uc != nil {
		fb.uc.Bind(controller)
	}
}

// WidgetAsLayout returns the find bar's outer widget as a layout.Widget.
func (fb *FindBar) WidgetAsLayout(factory layout.WidgetFactory) layout.Widget {
	if fb.outerBox == nil {
		return nil
	}
	return factory.WrapWidget(&fb.outerBox.Widget)
}

// Show displays the find bar and focuses the entry.
func (fb *FindBar) Show() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.outerBox == nil {
		return
	}

	fb.outerBox.SetVisible(true)
	fb.visible = true

	if fb.entry != nil {
		fb.entry.GrabFocus()
		text := fb.entry.GetText()
		if text != "" {
			fb.entry.SelectRegion(0, len(text))
		}
	}
}

// Hide hides the find bar and clears highlights.
func (fb *FindBar) Hide() {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if fb.outerBox == nil {
		return
	}

	if fb.uc != nil {
		fb.uc.Finish()
		fb.uc.Unbind()
	}

	fb.outerBox.SetVisible(false)
	fb.visible = false
}

// IsVisible returns whether the find bar is visible.
func (fb *FindBar) IsVisible() bool {
	fb.mu.RLock()
	defer fb.mu.RUnlock()
	return fb.visible
}

// FindNext moves to the next match.
func (fb *FindBar) FindNext() {
	if fb.uc != nil {
		fb.uc.SearchNext()
	}
}

// FindPrevious moves to the previous match.
func (fb *FindBar) FindPrevious() {
	if fb.uc != nil {
		fb.uc.SearchPrevious()
	}
}

func (fb *FindBar) createWidgets() error {
	if err := fb.initContainerWidgets(); err != nil {
		return err
	}
	if err := fb.initInputRowWidgets(); err != nil {
		return err
	}
	if err := fb.initOptionsRowWidgets(); err != nil {
		return err
	}
	fb.assembleWidgets()
	return nil
}

func (fb *FindBar) setupHandlers() {
	fb.connectEntryHandlers()
	fb.connectNavHandlers()
	fb.connectToggleHandlers()
	fb.attachKeyController()
}

func (fb *FindBar) initContainerWidgets() error {
	fb.outerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if fb.outerBox == nil {
		return errNilWidget("findOuterBox")
	}
	fb.outerBox.AddCssClass("find-bar-outer")
	fb.outerBox.SetHalign(gtk.AlignEndValue)
	fb.outerBox.SetValign(gtk.AlignStartValue)
	fb.outerBox.SetVisible(false)
	fb.outerBox.SetMarginTop(findBarMarginPx)
	fb.outerBox.SetMarginEnd(findBarMarginPx)

	fb.containerBox = gtk.NewBox(gtk.OrientationVerticalValue, 0)
	if fb.containerBox == nil {
		return errNilWidget("findContainerBox")
	}
	fb.containerBox.AddCssClass("find-bar-container")
	return nil
}

func (fb *FindBar) initInputRowWidgets() error {
	fb.inputRow = gtk.NewBox(gtk.OrientationHorizontalValue, findBarRowSpacing)
	if fb.inputRow == nil {
		return errNilWidget("findInputRow")
	}
	fb.inputRow.AddCssClass("find-bar-input-row")
	fb.inputRow.SetHalign(gtk.AlignFillValue)

	fb.entry = gtk.NewSearchEntry()
	if fb.entry == nil {
		return errNilWidget("findEntry")
	}
	fb.entry.AddCssClass("find-bar-entry")
	fb.entry.SetHexpand(true)
	placeholder := "Find in page..."
	fb.entry.SetPlaceholderText(&placeholder)

	fb.prevBtn = gtk.NewButtonWithLabel("Prev")
	if fb.prevBtn == nil {
		return errNilWidget("findPrevBtn")
	}
	fb.prevBtn.AddCssClass("find-bar-nav")

	fb.nextBtn = gtk.NewButtonWithLabel("Next")
	if fb.nextBtn == nil {
		return errNilWidget("findNextBtn")
	}
	fb.nextBtn.AddCssClass("find-bar-nav")

	fb.countLabel = gtk.NewLabel(nil)
	if fb.countLabel == nil {
		return errNilWidget("findCountLabel")
	}
	fb.countLabel.AddCssClass("find-bar-count")
	fb.countLabel.SetWidthChars(findBarCountWidth)

	fb.closeBtn = gtk.NewButtonWithLabel("X")
	if fb.closeBtn == nil {
		return errNilWidget("findCloseBtn")
	}
	fb.closeBtn.AddCssClass("find-bar-close")
	return nil
}

func (fb *FindBar) initOptionsRowWidgets() error {
	fb.optionsRow = gtk.NewBox(gtk.OrientationHorizontalValue, findBarRowSpacing)
	if fb.optionsRow == nil {
		return errNilWidget("findOptionsRow")
	}
	fb.optionsRow.AddCssClass("find-bar-options-row")

	fb.caseToggle = gtk.NewToggleButtonWithLabel("Aa")
	if fb.caseToggle == nil {
		return errNilWidget("findCaseToggle")
	}
	fb.caseToggle.AddCssClass("find-bar-toggle")

	fb.wordToggle = gtk.NewToggleButtonWithLabel("W")
	if fb.wordToggle == nil {
		return errNilWidget("findWordToggle")
	}
	fb.wordToggle.AddCssClass("find-bar-toggle")

	fb.hlToggle = gtk.NewToggleButtonWithLabel("Hl")
	if fb.hlToggle == nil {
		return errNilWidget("findHlToggle")
	}
	fb.hlToggle.AddCssClass("find-bar-toggle")
	fb.hlToggle.SetActive(true)
	return nil
}

func (fb *FindBar) assembleWidgets() {
	fb.inputRow.Append(&fb.entry.Widget)
	fb.inputRow.Append(&fb.prevBtn.Widget)
	fb.inputRow.Append(&fb.nextBtn.Widget)
	fb.inputRow.Append(&fb.countLabel.Widget)
	fb.inputRow.Append(&fb.closeBtn.Widget)

	fb.optionsRow.Append(&fb.caseToggle.Widget)
	fb.optionsRow.Append(&fb.wordToggle.Widget)
	fb.optionsRow.Append(&fb.hlToggle.Widget)

	fb.containerBox.Append(&fb.inputRow.Widget)
	fb.containerBox.Append(&fb.optionsRow.Widget)
	fb.outerBox.Append(&fb.containerBox.Widget)
}

func (fb *FindBar) connectEntryHandlers() {
	if fb.entry == nil {
		return
	}
	changedCb := func(_ gtk.SearchEntry) {
		if fb.uc != nil {
			fb.uc.SetQuery(fb.entry.GetText())
		}
	}
	fb.entry.ConnectSearchChanged(&changedCb)
}

func (fb *FindBar) connectNavHandlers() {
	if fb.prevBtn != nil {
		prevCb := func(_ gtk.Button) {
			fb.FindPrevious()
		}
		fb.prevBtn.ConnectClicked(&prevCb)
	}

	if fb.nextBtn != nil {
		nextCb := func(_ gtk.Button) {
			fb.FindNext()
		}
		fb.nextBtn.ConnectClicked(&nextCb)
	}

	if fb.closeBtn != nil {
		closeCb := func(_ gtk.Button) {
			fb.Hide()
			if fb.onClose != nil {
				fb.onClose()
			}
		}
		fb.closeBtn.ConnectClicked(&closeCb)
	}
}

func (fb *FindBar) connectToggleHandlers() {
	if fb.caseToggle != nil {
		caseCb := func(_ gtk.ToggleButton) {
			if fb.uc != nil {
				fb.uc.SetCaseSensitiveEnabled(fb.caseToggle.GetActive())
			}
		}
		fb.caseToggle.ConnectToggled(&caseCb)
	}

	if fb.wordToggle != nil {
		wordCb := func(_ gtk.ToggleButton) {
			if fb.uc != nil {
				fb.uc.SetAtWordStarts(fb.wordToggle.GetActive())
			}
		}
		fb.wordToggle.ConnectToggled(&wordCb)
	}

	if fb.hlToggle != nil {
		hlCb := func(_ gtk.ToggleButton) {
			if fb.uc != nil {
				fb.uc.SetHighlightEnabled(fb.hlToggle.GetActive())
			}
		}
		fb.hlToggle.ConnectToggled(&hlCb)
	}
}

func (fb *FindBar) attachKeyController() {
	controller := gtk.NewEventControllerKey()
	if controller == nil {
		return
	}
	controller.SetPropagationPhase(gtk.PhaseCaptureValue)
	keyPressedCb := func(_ gtk.EventControllerKey, keyval uint, _ uint, state gdk.ModifierType) bool {
		switch keyval {
		case uint(gdk.KEY_Escape):
			fb.Hide()
			if fb.onClose != nil {
				fb.onClose()
			}
			return true
		case uint(gdk.KEY_Return), uint(gdk.KEY_KP_Enter):
			if state&gdk.ShiftMaskValue != 0 {
				fb.FindPrevious()
			} else {
				fb.FindNext()
			}
			return true
		}
		return false
	}
	controller.ConnectKeyPressed(&keyPressedCb)
	fb.outerBox.AddController(&controller.EventController)
}

func (fb *FindBar) bindUseCase() {
	if fb.uc == nil {
		return
	}

	fb.uc.SetOnStateChange(func(state usecase.FindState) {
		cb := glib.SourceFunc(func(_ uintptr) bool {
			if fb.entry == nil || fb.countLabel == nil {
				return false
			}

			text := ""
			if state.Query != "" {
				text = fmt.Sprintf("%d/%d", state.CurrentIndex, state.MatchCount)
			}
			fb.countLabel.SetText(text)

			if state.MatchCount > 0 {
				fb.countLabel.AddCssClass("find-bar-count-has")
			} else {
				fb.countLabel.RemoveCssClass("find-bar-count-has")
			}

			if state.NotFound && state.Query != "" {
				fb.entry.AddCssClass("not-found")
			} else {
				fb.entry.RemoveCssClass("not-found")
			}
			return false
		})
		glib.IdleAdd(&cb, 0)
	})
}
