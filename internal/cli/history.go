package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

const (
	unknownValue        = "Unknown"
	defaultHistoryLimit = 20
	defaultSearchLimit  = 10
	maxURLDisplay       = 50
	maxTitleDisplay     = 40
	maxURLCompact       = 60
	maxTitleCompact     = 30
	tabSpacing          = 2
)

// NewHistoryCmd creates the history command
func NewHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Manage browsing history",
		Long: `Manage browsing history with various subcommands:
  list   - Show recent browsing history
  search - Search through history
  clear  - Clear history (with confirmation)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default to list if no subcommand
			return listHistory(cmd, args)
		},
	}

	// List subcommand (also default behavior)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List recent browsing history",
		Long:  `Display recent browsing history with visit counts and timestamps.`,
		RunE:  listHistory,
	}
	listCmd.Flags().IntP("limit", "n", defaultHistoryLimit, "Number of history entries to show")
	listCmd.Flags().BoolP("verbose", "v", false, "Show detailed information")

	// Search subcommand
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search through browsing history",
		Long:  `Search for URLs and titles in browsing history.`,
		Args:  cobra.ExactArgs(1),
		RunE:  searchHistory,
	}
	searchCmd.Flags().IntP("limit", "n", defaultSearchLimit, "Number of search results to show")

	// Clear subcommand
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear browsing history",
		Long:  `Clear all browsing history. This action cannot be undone.`,
		RunE:  clearHistory,
	}
	clearCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	// Stats subcommand
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show history statistics",
		Long:  `Display statistics about your browsing history.`,
		RunE:  showStats,
	}

	// Add global flags
	cmd.Flags().IntP("limit", "n", defaultHistoryLimit, "Number of history entries to show")
	cmd.Flags().BoolP("verbose", "v", false, "Show detailed information")

	// Add subcommands
	cmd.AddCommand(listCmd)
	cmd.AddCommand(searchCmd)
	cmd.AddCommand(clearCmd)
	cmd.AddCommand(statsCmd)

	return cmd
}

// listHistory displays recent browsing history
func listHistory(cmd *cobra.Command, _ []string) error {
	cli, err := NewCLI()
	if err != nil {
		return fmt.Errorf("failed to initialize CLI: %w", err)
	}
	defer func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
		}
	}()

	ctx := context.Background()

	limit, _ := cmd.Flags().GetInt("limit")
	verbose, _ := cmd.Flags().GetBool("verbose")

	history, err := cli.Queries.GetHistory(ctx, int64(limit))
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	if len(history) == 0 {
		fmt.Println("No browsing history found.")
		return nil
	}

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	if verbose {
		fmt.Fprintln(w, "ID\tVISITS\tLAST VISITED\tURL\tTITLE")
		fmt.Fprintln(w, "--\t------\t------------\t---\t-----")

		for _, entry := range history {
			visits := "1"
			if entry.VisitCount.Valid {
				visits = strconv.FormatInt(entry.VisitCount.Int64, 10)
			}

			lastVisited := unknownValue
			if entry.LastVisited.Valid {
				lastVisited = entry.LastVisited.Time.Format("2006-01-02 15:04")
			}

			title := entry.Title.String
			if !entry.Title.Valid || title == "" {
				title = "-"
			}

			url := truncateString(entry.Url, 50)
			title = truncateString(title, 40)

			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
				entry.ID, visits, lastVisited, url, title)
		}
	} else {
		fmt.Fprintln(w, "VISITS\tLAST VISITED\tURL")
		fmt.Fprintln(w, "------\t------------\t---")

		for _, entry := range history {
			visits := "1"
			if entry.VisitCount.Valid {
				visits = strconv.FormatInt(entry.VisitCount.Int64, 10)
			}

			lastVisited := unknownValue
			if entry.LastVisited.Valid {
				// Show relative time for recent entries
				if time.Since(entry.LastVisited.Time) < 24*time.Hour {
					lastVisited = entry.LastVisited.Time.Format("15:04")
				} else {
					lastVisited = entry.LastVisited.Time.Format("Jan 02")
				}
			}

			url := entry.Url
			if len(url) > 60 {
				url = url[:57] + "..."
			}

			fmt.Fprintf(w, "%s\t%s\t%s\n", visits, lastVisited, url)
		}
	}

	return nil
}

// searchHistory searches through browsing history
func searchHistory(cmd *cobra.Command, args []string) error {
	cli, err := NewCLI()
	if err != nil {
		return fmt.Errorf("failed to initialize CLI: %w", err)
	}
	defer func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
		}
	}()

	ctx := context.Background()
	query := args[0]
	limit, _ := cmd.Flags().GetInt("limit")

	// Search both URL and title
	searchQuery := sql.NullString{String: query, Valid: true}
	results, err := cli.Queries.SearchHistory(ctx, searchQuery, searchQuery, int64(limit))
	if err != nil {
		return fmt.Errorf("failed to search history: %w", err)
	}

	if len(results) == 0 {
		fmt.Printf("No history entries found matching '%s'.\n", query)
		return nil
	}

	fmt.Printf("Found %d result(s) for '%s':\n\n", len(results), query)

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "VISITS\tLAST VISITED\tURL\tTITLE")
	fmt.Fprintln(w, "------\t------------\t---\t-----")

	for _, entry := range results {
		visits := "1"
		if entry.VisitCount.Valid {
			visits = strconv.FormatInt(entry.VisitCount.Int64, 10)
		}

		lastVisited := "Unknown"
		if entry.LastVisited.Valid {
			lastVisited = entry.LastVisited.Time.Format("Jan 02 15:04")
		}

		title := entry.Title.String
		if !entry.Title.Valid || title == "" {
			title = "-"
		}

		url := truncateString(entry.Url, 50)
		title = truncateString(title, 30)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", visits, lastVisited, url, title)
	}

	return nil
}

// clearHistory clears all browsing history
func clearHistory(cmd *cobra.Command, _ []string) error {
	force, _ := cmd.Flags().GetBool("force")

	// Confirmation unless --force is used
	if !force {
		fmt.Print("This will permanently delete all browsing history. Continue? [y/N]: ")
		var response string
		_, _ = fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))

		if response != "y" && response != "yes" {
			fmt.Println("Operation cancelled.")
			return nil
		}
	}

	cli, err := NewCLI()
	if err != nil {
		return fmt.Errorf("failed to initialize CLI: %w", err)
	}
	defer func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
		}
	}()

	// Execute DELETE query
	result, err := cli.DB.Exec("DELETE FROM history")
	if err != nil {
		return fmt.Errorf("failed to clear history: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	fmt.Printf("Successfully cleared %d history entries.\n", rowsAffected)
	return nil
}

// showStats displays statistics about browsing history
func showStats(_ *cobra.Command, _ []string) error {
	cli, err := NewCLI()
	if err != nil {
		return fmt.Errorf("failed to initialize CLI: %w", err)
	}
	defer func() {
		if err := cli.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close database: %v\n", err)
		}
	}()

	ctx := context.Background()

	// Get total entries
	var totalEntries int64
	err = cli.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM history").Scan(&totalEntries)
	if err != nil {
		return fmt.Errorf("failed to get total entries: %w", err)
	}

	// Get total visits
	var totalVisits sql.NullInt64
	err = cli.DB.QueryRowContext(ctx, "SELECT SUM(visit_count) FROM history").Scan(&totalVisits)
	if err != nil {
		return fmt.Errorf("failed to get total visits: %w", err)
	}

	// Get date range
	var oldestDate, newestDate sql.NullTime
	err = cli.DB.QueryRowContext(ctx, "SELECT MIN(created_at), MAX(last_visited) FROM history").Scan(&oldestDate, &newestDate)
	if err != nil {
		return fmt.Errorf("failed to get date range: %w", err)
	}

	// Get top domains
	rows, err := cli.DB.QueryContext(ctx, `
		SELECT 
			CASE 
				WHEN url LIKE 'http://%' THEN substr(url, 8)
				WHEN url LIKE 'https://%' THEN substr(url, 9)
				ELSE url
			END as domain,
			COUNT(*) as count,
			SUM(visit_count) as total_visits
		FROM history 
		GROUP BY domain 
		ORDER BY total_visits DESC 
		LIMIT 5
	`)
	if err != nil {
		return fmt.Errorf("failed to get top domains: %w", err)
	}
	defer rows.Close()

	fmt.Println("Browsing History Statistics")
	fmt.Println("==========================")
	fmt.Printf("Total unique URLs: %d\n", totalEntries)

	if totalVisits.Valid {
		fmt.Printf("Total visits: %d\n", totalVisits.Int64)
	}

	if oldestDate.Valid && newestDate.Valid {
		fmt.Printf("Date range: %s to %s\n",
			oldestDate.Time.Format("2006-01-02"),
			newestDate.Time.Format("2006-01-02"))
	}

	fmt.Println("\nTop 5 domains:")
	fmt.Println("--------------")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "DOMAIN\tURLs\tVISITS")
	fmt.Fprintln(w, "------\t----\t------")

	for rows.Next() {
		var domain string
		var urlCount, visitCount int64

		if err := rows.Scan(&domain, &urlCount, &visitCount); err != nil {
			continue
		}

		// Extract just the domain part
		if idx := strings.Index(domain, "/"); idx > 0 {
			domain = domain[:idx]
		}

		fmt.Fprintf(w, "%s\t%d\t%d\n", domain, urlCount, visitCount)
	}

	return nil
}
