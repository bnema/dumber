package ui

import (
	"slices"
	"strings"
	"unicode"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
)

const legendFlashDurationMs = 120

func legendActions(mode input.Mode, workspace entity.WorkspaceConfig, session entity.SessionConfig) map[string]entity.ActionBinding {
	switch mode {
	case input.ModePane:
		return workspace.PaneMode.Actions
	case input.ModeResize:
		return workspace.ResizeMode.Actions
	case input.ModeVim:
		return workspace.VimMode.Actions
	case input.ModeTab:
		return workspace.TabMode.Actions
	case input.ModeSession:
		return session.SessionMode.Actions
	default:
		return nil
	}
}

func (f *modeFrame) build(actions map[string]entity.ActionBinding) {
	if f.flashTimer != 0 {
		glib.SourceRemove(f.flashTimer)
		f.flashTimer = 0
	}
	f.content.RemoveAll()
	f.rows = make(map[string][]*gtk.Widget)
	f.keycaps = make(map[string][]*gtk.Widget)
	f.heading.SetText(f.mode.DisplayName() + "  ·  ESC to exit")
	groups := make(map[string][]string)
	for name, binding := range actions {
		if len(binding.Keys) == 0 {
			continue
		}
		group := modeLegendGroup(f.mode, name)
		groups[group] = append(groups[group], name)
	}
	order := []string{"SPLIT", "FOCUS", "SCROLL", "RESIZE", "SWITCH", "JUMP", "MANAGE", "EXIT"}
	index := 0
	for _, groupName := range order {
		names := groups[groupName]
		if len(names) == 0 {
			continue
		}
		slices.Sort(names)
		column := gtk.NewBox(gtk.OrientationVerticalValue, 0)
		column.SetCanFocus(false)
		column.SetCanTarget(false)
		column.AddCssClass("mode-legend-group")
		title := gtk.NewLabel(&groupName)
		title.SetCanFocus(false)
		title.SetCanTarget(false)
		title.SetHalign(gtk.AlignStartValue)
		title.AddCssClass("mode-legend-group-title")
		column.Append(&title.Widget)
		for _, name := range names {
			column.Append(&f.actionRow(name, actions[name]).Widget)
		}
		f.content.Append(&column.Widget)
		if child := f.content.GetChildAtIndex(index); child != nil {
			child.SetCanFocus(false)
			child.SetCanTarget(false)
		}
		index++
	}
}

func (f *modeFrame) actionRow(name string, binding entity.ActionBinding) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontalValue, 0)
	row.SetCanFocus(false)
	row.SetCanTarget(false)
	row.AddCssClass("mode-legend-row")
	f.rows[name] = append(f.rows[name], &row.Widget)
	for _, key := range binding.Keys {
		displayKey := displayLegendKey(key)
		keycap := gtk.NewLabel(&displayKey)
		keycap.SetCanFocus(false)
		keycap.SetCanTarget(false)
		keycap.AddCssClass("mode-legend-keycap")
		row.Append(&keycap.Widget)
		f.keycaps[name] = append(f.keycaps[name], &keycap.Widget)
	}
	label := binding.Desc
	if label == "" {
		label = strings.ReplaceAll(name, "-", " ")
	}
	text := gtk.NewLabel(&label)
	text.SetCanFocus(false)
	text.SetCanTarget(false)
	text.SetHalign(gtk.AlignStartValue)
	text.SetEllipsize(3) // PangoEllipsizeEnd
	text.AddCssClass("mode-legend-description")
	row.Append(&text.Widget)
	return row
}

func displayLegendKey(key string) string {
	switch strings.ToLower(key) {
	case "arrowright":
		return "→"
	case "arrowleft":
		return "←"
	case "arrowup":
		return "↑"
	case "arrowdown":
		return "↓"
	case "escape":
		return "esc"
	case "enter":
		return "↵"
	default:
		return key
	}
}

func (f *modeFrame) setPending(pending string, actions map[string]entity.ActionBinding) {
	if f == nil || f.lingering || f.mode != input.ModeVim {
		return
	}
	f.pending = pending
	for name, rows := range f.rows {
		matches := legendSequenceMatches(pending, actions[name].Keys)
		for _, row := range rows {
			if matches {
				row.RemoveCssClass("mode-legend-dim")
			} else {
				row.AddCssClass("mode-legend-dim")
			}
		}
	}
}

func legendSequenceMatches(pending string, keys []string) bool {
	sequence := strings.TrimLeftFunc(pending, unicode.IsDigit)
	if sequence == "" {
		return true
	}
	for _, key := range keys {
		if strings.HasPrefix(key, sequence) {
			return true
		}
	}
	return false
}

func (f *modeFrame) flash(action input.Action) {
	if f == nil || f.lingering || !f.visible || !f.config.ModeLegendAnimations {
		return
	}
	name := strings.ReplaceAll(string(action), "_", "-")
	keycaps := f.keycaps[name]
	if len(keycaps) == 0 {
		return
	}
	if f.flashTimer != 0 {
		glib.SourceRemove(f.flashTimer)
		f.flashTimer = 0
	}
	for _, caps := range f.keycaps {
		for _, cap := range caps {
			cap.RemoveCssClass("mode-legend-flash")
		}
	}
	for _, cap := range keycaps {
		cap.AddCssClass("mode-legend-flash")
	}
	cb := glib.SourceFunc(func(_ uintptr) bool {
		for _, cap := range keycaps {
			cap.RemoveCssClass("mode-legend-flash")
		}
		f.flashTimer = 0
		return false
	})
	f.flashTimer = glib.TimeoutAdd(legendFlashDurationMs, &cb, 0)
}
