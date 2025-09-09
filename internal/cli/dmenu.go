package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// NewDmenuCmd creates the dmenu command for launcher integration
func NewDmenuCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dmenu",
		Short: "Run in dmenu mode for launcher integration",
		Long: `Run in dmenu mode to integrate with rofi, dmenu, or other launchers.
This mode reads from stdin and outputs selectable options to stdout.

Usage with rofi:
  dumber dmenu | rofi -dmenu -p "Browse: " | dumber dmenu --select

Usage with dmenu:
  dumber dmenu | dmenu -p "Browse: " | dumber dmenu --select`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			selectFlag := cmd.Flag("select").Changed

			cli, err := NewCLI()
			if err != nil {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			}
			defer func() {
				if closeErr := cli.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", closeErr)
				}
			}()

			if selectFlag {
				return handleSelection(cli)
			}
			return generateOptions(cli)
		},
	}

	cmd.Flags().Bool("select", false, "Process selection from launcher (reads from stdin)")
	return cmd
}

// generateOptions outputs all available options for the launcher
func generateOptions(cli *CLI) error {
	ctx := context.Background()

	const maxHistoryEntries = 50
	// Get history entries
	history, err := cli.Queries.GetHistory(ctx, maxHistoryEntries)
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	// Get shortcuts
	shortcuts, err := cli.Queries.GetShortcuts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get shortcuts: %w", err)
	}

	// Generate options
	options := make([]DmenuOption, 0, len(history)+len(shortcuts)+1)

	// Add direct URL input option
	options = append(options, DmenuOption{
		Display:     "🌐 Enter URL or search query...",
		Value:       "",
		Type:        "input",
		Description: "Type any URL or search term",
	})

	// Add shortcuts with examples
	for _, shortcut := range shortcuts {
		desc := shortcut.Description.String
		if !shortcut.Description.Valid {
			desc = "Custom shortcut"
		}

		options = append(options, DmenuOption{
			Display:     fmt.Sprintf("🔍 %s: (%s)", shortcut.Shortcut, desc),
			Value:       shortcut.Shortcut + ":",
			Type:        "shortcut",
			Description: desc,
		})
	}

	// Add history entries
	for _, entry := range history {
		title := entry.Url
		if entry.Title.Valid && entry.Title.String != "" {
			title = entry.Title.String
		}

		const maxDisplayLength = 80
		// Truncate long titles/URLs for display
		display := truncateString(title, maxDisplayLength)

		options = append(options, DmenuOption{
			Display:     fmt.Sprintf("🕒 %s", display),
			Value:       entry.Url,
			Type:        "history",
			Description: entry.Url,
		})
	}

	// Sort options: input first, then shortcuts, then history by visit count/recency
	sort.Slice(options, func(i, j int) bool {
		if options[i].Type != options[j].Type {
			// Order: input, shortcut, history
			typeOrder := map[string]int{"input": 0, "shortcut": 1, "history": 2} //nolint:mnd
			return typeOrder[options[i].Type] < typeOrder[options[j].Type]
		}

		if options[i].Type == "history" && options[j].Type == "history" {
			// History already sorted by recency from database
			return i < j
		}

		return options[i].Display < options[j].Display
	})

	// Output options to stdout
	for _, option := range options {
		fmt.Println(option.Display)
	}

	return nil
}

// handleSelection processes the user's selection from the launcher
func handleSelection(cli *CLI) error {
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return fmt.Errorf("no input received")
	}

	selection := strings.TrimSpace(scanner.Text())
	if selection == "" {
		return fmt.Errorf("empty selection")
	}

	// Parse the selection to get the actual input
	input := parseSelection(selection)

	// If it's just the input prompt, read another line for the actual input
	if input == "" || input == "Enter URL or search query..." {
		fmt.Print("Enter URL or search query: ")
		if !scanner.Scan() {
			return fmt.Errorf("no input received")
		}
		input = strings.TrimSpace(scanner.Text())
	}

	if input == "" {
		return fmt.Errorf("empty input")
	}

	// Browse the selected/entered URL
	return browse(cli, input)
}

// DmenuOption represents a selectable option in dmenu mode
type DmenuOption struct {
	Display     string
	Value       string
	Type        string // "input", "shortcut", "history"
	Description string
}

// parseSelection extracts the actual URL/query from a dmenu selection
func parseSelection(selection string) string {
	// Remove emoji prefixes and clean up the selection
	selection = strings.TrimSpace(selection)

	// Handle different option types
	if strings.HasPrefix(selection, "🌐 ") {
		// Input option selected
		return ""
	}

	if strings.HasPrefix(selection, "🔍 ") {
		// Shortcut selected - extract the shortcut prefix
		rest := strings.TrimPrefix(selection, "🔍 ")
		if idx := strings.Index(rest, ":"); idx > 0 {
			return rest[:idx+1] // Return "g:" format
		}
	}

	if strings.HasPrefix(selection, "🕒 ") {
		// History entry selected - this is the display title, need to extract URL
		// For now, we'll assume the display is the URL or close enough
		return strings.TrimPrefix(selection, "🕒 ")
	}

	// Fallback: return the selection as-is
	return selection
}

// truncateString truncates a string to the specified length with ellipsis
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	const minEllipsisLength = 3
	if maxLen <= minEllipsisLength {
		return s[:maxLen]
	}

	return s[:maxLen-minEllipsisLength] + "..."
}
