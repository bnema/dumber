package textinput

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// GTKEntryTarget implements TextInputTarget for GTK SearchEntry widgets.
// It uses the Editable interface methods to insert and delete text.
type GTKEntryTarget struct {
	entry *gtk.SearchEntry
}

// Compile-time interface check.
var _ port.TextInputTarget = (*GTKEntryTarget)(nil)

// NewGTKEntryTarget creates a new GTK entry target for a SearchEntry.
func NewGTKEntryTarget(entry *gtk.SearchEntry) *GTKEntryTarget {
	return &GTKEntryTarget{entry: entry}
}

// InsertText inserts text at the current cursor position.
func (t *GTKEntryTarget) InsertText(ctx context.Context, text string) error {
	log := logging.FromContext(ctx)

	if t.entry == nil {
		log.Warn().Msg("GTKEntryTarget: entry is nil")
		return nil
	}

	// Get current text and cursor position
	currentText := t.entry.GetText()
	pos := t.entry.GetPosition()

	// Convert to runes for proper Unicode handling
	runes := []rune(currentText)
	textRunes := []rune(text)

	// Clamp position to valid range
	if pos < 0 {
		pos = 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}

	// Build new text with insertion
	newRunes := make([]rune, 0, len(runes)+len(textRunes))
	newRunes = append(newRunes, runes[:pos]...)
	newRunes = append(newRunes, textRunes...)
	newRunes = append(newRunes, runes[pos:]...)

	// Set new text and cursor position
	t.entry.SetText(string(newRunes))
	newPos := pos + len(textRunes)
	t.entry.SetPosition(newPos)

	log.Debug().
		Str("text", text).
		Int("pos", pos).
		Msg("inserted text into GTK entry")

	return nil
}

// DeleteBeforeCursor deletes n characters before the cursor position.
func (t *GTKEntryTarget) DeleteBeforeCursor(ctx context.Context, n int) error {
	log := logging.FromContext(ctx)

	if t.entry == nil {
		log.Warn().Msg("GTKEntryTarget: entry is nil")
		return nil
	}

	pos := t.entry.GetPosition()
	if pos < n {
		n = pos // Don't delete more than available
	}

	if n <= 0 {
		return nil
	}

	// DeleteText takes start and end positions
	t.entry.DeleteText(pos-n, pos)

	log.Debug().
		Int("n", n).
		Int("pos", pos).
		Msg("deleted text before cursor in GTK entry")

	return nil
}
