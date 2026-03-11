package config

import (
	"testing"

	"github.com/bnema/dumber/internal/application/port"
)

func TestConfigDiffFormatter_FormatChangesAsDiff(t *testing.T) {
	f := NewDiffFormatter()

	t.Run("no changes", func(t *testing.T) {
		got := f.FormatChangesAsDiff(nil)
		want := noChangesMsg
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got := f.FormatChangesAsDiff([]port.KeyChange{})
		want := noChangesMsg
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("added change", func(t *testing.T) {
		changes := []port.KeyChange{
			{Type: port.KeyChangeAdded, NewKey: "update.enable", NewValue: "true"},
		}
		got := f.FormatChangesAsDiff(changes)
		want := "Config migration changes:\n\n  + update.enable = true\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("removed change", func(t *testing.T) {
		changes := []port.KeyChange{
			{Type: port.KeyChangeRemoved, OldKey: "old.key", OldValue: "42"},
		}
		got := f.FormatChangesAsDiff(changes)
		want := "Config migration changes:\n\n  - old.key = 42 (deprecated)\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("renamed change", func(t *testing.T) {
		changes := []port.KeyChange{
			{Type: port.KeyChangeRenamed, OldKey: "old.name", NewKey: "new.name", OldValue: "hello"},
		}
		got := f.FormatChangesAsDiff(changes)
		want := "Config migration changes:\n\n  ~ old.name -> new.name\n    (value: hello)\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("all change types combined", func(t *testing.T) {
		changes := []port.KeyChange{
			{Type: port.KeyChangeAdded, NewKey: "feature.flag", NewValue: "false"},
			{Type: port.KeyChangeRemoved, OldKey: "legacy.opt", OldValue: "1"},
			{Type: port.KeyChangeRenamed, OldKey: "foo.bar", NewKey: "foo.baz", OldValue: "qux"},
		}
		got := f.FormatChangesAsDiff(changes)
		want := "Config migration changes:\n\n" +
			"  + feature.flag = false\n" +
			"  - legacy.opt = 1 (deprecated)\n" +
			"  ~ foo.bar -> foo.baz\n" +
			"    (value: qux)\n"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
