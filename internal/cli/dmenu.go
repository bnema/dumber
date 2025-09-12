package cli

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/cache"
	"github.com/spf13/cobra"
)

const (
	historyType = "history"
)

// NewDmenuCmd creates the dmenu command for launcher integration
func NewDmenuCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dmenu",
		Short: "Fast fuzzy history browser for launcher integration",
		Long: `Fast dmenu mode showing cached browsing history with fuzzy search.
Uses binary tree cache for sub-millisecond performance.

Usage with rofi:
  dumber dmenu | rofi -dmenu -p "Browse: " | dumber dmenu --select

Usage with fuzzel:
  dumber dmenu | fuzzel --dmenu -p "Browse: " | dumber dmenu --select`,
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

// generateOptions outputs all available options for the launcher using fuzzy cache
func generateOptions(cli *CLI) error {
	ctx := context.Background()

	// Initialize cache manager
	cacheConfig := cache.DefaultCacheConfig()
	cacheConfig.MaxResults = 50 // Match the old maxHistoryEntries
	cacheManager := cache.NewCacheManager(cli.Queries, cacheConfig)

	// Initialize favicon cache
	faviconCache, err := cache.NewFaviconCache()
	if err != nil {
		// Continue without favicon cache if it fails
		faviconCache = nil
	}

	// Get top entries from cache (this is blazingly fast!)
	result, err := cacheManager.GetTopEntries(ctx)
	if err != nil {
		// Fallback to old method if cache fails
		return generateOptionsFallback(cli)
	}

	// Fast dmenu mode: only shows cached history entries sorted by relevance

	// Generate options - only show history entries from fast cache
	options := make([]DmenuOption, 0, len(result.Matches))

	// Add history entries from cache (already sorted by relevance!)
	for _, match := range result.Matches {
		entry := match.Entry

		// Parse URL for domain extraction
		parsedURL, err := url.Parse(entry.URL)
		var domain string
		if err != nil || parsedURL.Host == "" {
			domain = "local"
		} else {
			domain = parsedURL.Host
		}

		// Determine title
		title := entry.Title
		if title == "" || title == entry.URL {
			// Use meaningful fallback for untitled pages
			if parsedURL != nil && parsedURL.Path != "" && parsedURL.Path != "/" {
				title = fmt.Sprintf("[%s%s]", domain, parsedURL.Path)
			} else {
				title = fmt.Sprintf("[%s]", domain)
			}
		}

		// Check if it's a Google search result and format accordingly
		isGoogleSearch := domain == "www.google.com" && strings.Contains(entry.URL, "/search?q=")
		if isGoogleSearch {
			if parsedURL != nil {
				if q := parsedURL.Query().Get("q"); q != "" {
					title = fmt.Sprintf("Google: \"%s\"", q)
				}
			}
		}

		// Format: "Title | domain.com | full-url"
		// Using pipe separator for rofi/dmenu compatibility
		display := fmt.Sprintf("%s | %s | %s",
			truncateString(title, 50),
			domain,
			truncateString(entry.URL, 70))

		// Get favicon URL from cached entry
		faviconURL := match.Entry.FaviconURL

		options = append(options, DmenuOption{
			Display:     display,
			Value:       entry.URL,
			Type:        historyType,
			Description: entry.URL,
			FaviconURL:  faviconURL,
		})
	}

	// No sorting needed - history entries are already sorted by relevance from cache

	// Output options to stdout with icon specifications
	for _, option := range options {
		iconName := getIconName(option, faviconCache)
		if iconName != "" {
			fmt.Printf("%s\x00icon\x1f%s\n", option.Display, iconName)
		} else {
			fmt.Println(option.Display)
		}
	}

	// Trigger background cache refresh for next time if needed
	defer cacheManager.OnApplicationExit(ctx)

	return nil
}

// generateOptionsFallback is the legacy method used as a fallback if fast cache fails
func generateOptionsFallback(cli *CLI) error {
	ctx := context.Background()

	// Initialize favicon cache
	faviconCache, err := cache.NewFaviconCache()
	if err != nil {
		// Continue without favicon cache if it fails
		faviconCache = nil
	}

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
			Type:        historyType,
			Description: entry.Url,
		})
	}

	// Sort options: input first, then shortcuts, then history by visit count/recency
	sort.Slice(options, func(i, j int) bool {
		if options[i].Type != options[j].Type {
			// Order: input, shortcut, history
			typeOrder := map[string]int{"input": 0, "shortcut": 1, historyType: 2} //nolint:mnd
			return typeOrder[options[i].Type] < typeOrder[options[j].Type]
		}

		if options[i].Type == historyType && options[j].Type == historyType {
			// History already sorted by recency from database
			return i < j
		}

		return options[i].Display < options[j].Display
	})

	// Output options to stdout with icon specifications
	for _, option := range options {
		iconName := getIconName(option, faviconCache)
		if iconName != "" {
			fmt.Printf("%s\x00icon\x1f%s\n", option.Display, iconName)
		} else {
			fmt.Println(option.Display)
		}
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

	// Parse the selection to get the actual URL from history entry
	input := parseSelection(selection)

	if input == "" {
		return fmt.Errorf("empty input")
	}

	// Browse the selected/entered URL
	err := browse(cli, input)

	// After successful browse, invalidate cache so next dmenu shows updated order
	if err == nil {
		ctx := context.Background()
		cacheConfig := cache.DefaultCacheConfig()
		cacheManager := cache.NewCacheManager(cli.Queries, cacheConfig)
		cacheManager.InvalidateAndRefresh(ctx)
	}

	return err
}

// DmenuOption represents a selectable option in dmenu mode
type DmenuOption struct {
	Display     string
	Value       string
	Type        string // "input", "shortcut", "history"
	Description string
	FaviconURL  string // URL to favicon for this entry
}

// parseSelection extracts the actual URL from a dmenu selection
func parseSelection(selection string) string {
	selection = strings.TrimSpace(selection)

	// Strip icon protocol if present (format: "text\0icon\x1ficonname")
	if iconIndex := strings.Index(selection, "\x00icon\x1f"); iconIndex > 0 {
		selection = selection[:iconIndex]
	}

	// 1. Handle new pipe-separated format: "Title | domain.com | full-url"
	if strings.Contains(selection, " | ") {
		parts := strings.Split(selection, " | ")
		if len(parts) >= 3 {
			// The URL is the last part (third field)
			return strings.TrimSpace(parts[len(parts)-1])
		} else if len(parts) == 2 {
			// Might be "title | url" format, check if second part looks like URL
			lastPart := strings.TrimSpace(parts[1])
			if strings.Contains(lastPart, "://") || strings.Contains(lastPart, ".") {
				return lastPart
			}
		}
	}

	// Legacy format handling for backward compatibility
	if strings.HasPrefix(selection, "🌐 ") {
		// Input option selected (shouldn't happen with new format but just in case)
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
		// History entry selected - remove emoji prefix
		selection = strings.TrimPrefix(selection, "🕒 ")
	}

	// If no pipes and looks like a URL, return as-is
	if strings.Contains(selection, "://") {
		return selection
	}

	// Fallback: treat as search query
	return selection
}

// getIconName determines the appropriate icon name for a dmenu option
func getIconName(option DmenuOption, faviconCache *cache.FaviconCache) string {
	if option.Type != historyType {
		return ""
	}

	// Only use cached favicons - no system theme fallbacks for consistent sizing
	if option.FaviconURL != "" && faviconCache != nil {
		cachedPath := faviconCache.GetCachedPath(option.FaviconURL)
		if cachedPath != "" {
			return cachedPath
		}
		// Start async download for next time
		faviconCache.CacheAsync(option.FaviconURL)
	}

	// No favicon available - return empty string to show no icon
	return ""
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
