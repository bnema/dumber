package config

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
)

const noChangesMsg = "No changes detected."

// ConfigDiffFormatter implements port.DiffFormatter for formatting config changes.
type ConfigDiffFormatter struct{}

// NewDiffFormatter creates a new ConfigDiffFormatter.
func NewDiffFormatter() *ConfigDiffFormatter {
	return &ConfigDiffFormatter{}
}

// FormatChangesAsDiff returns changes formatted as a diff for display.
func (*ConfigDiffFormatter) FormatChangesAsDiff(changes []port.KeyChange) string {
	if len(changes) == 0 {
		return noChangesMsg
	}

	var sb strings.Builder
	sb.WriteString("Config migration changes:\n\n")

	for _, change := range changes {
		switch change.Type {
		case port.KeyChangeAdded:
			fmt.Fprintf(&sb, "  + %s = %s\n", change.NewKey, change.NewValue)
		case port.KeyChangeRemoved:
			fmt.Fprintf(&sb, "  - %s = %s (deprecated)\n", change.OldKey, change.OldValue)
		case port.KeyChangeRenamed:
			fmt.Fprintf(&sb, "  ~ %s -> %s\n", change.OldKey, change.NewKey)
			fmt.Fprintf(&sb, "    (value: %s)\n", change.OldValue)
		}
	}

	return sb.String()
}
