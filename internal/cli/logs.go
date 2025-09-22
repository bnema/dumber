package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/spf13/cobra"
)

// NewLogsCmd creates the logs command and its subcommands
func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "View and manage application logs",
		Long:  "Access logs of the aplication, including viewing recent entries, listing log files, cleaning old logs, and showing the log directory path.",
	}

	cmd.AddCommand(newLogsTailCmd())
	cmd.AddCommand(newLogsListCmd())
	cmd.AddCommand(newLogsCleanCmd())
	cmd.AddCommand(newLogsPathCmd())
	cmd.AddCommand(newLogsConsoleCmd())

	return cmd
}

// newLogsTailCmd creates the tail subcommand
func newLogsTailCmd() *cobra.Command {
	var lines int

	cmd := &cobra.Command{
		Use:   "tail [lines]",
		Short: "Show last N lines of current log",
		Long:  "Display the last N lines from the current log file. Defaults to 50 lines.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse lines argument if provided
			if len(args) > 0 {
				var err error
				lines, err = strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid number of lines: %s", args[0])
				}
			}

			logDir, err := config.GetLogDir()
			if err != nil {
				return fmt.Errorf("failed to get log directory: %w", err)
			}

			currentLog := filepath.Join(logDir, "dumber.log")
			return tailFile(currentLog, lines)
		},
	}

	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "Number of lines to show")
	return cmd
}

// newLogsListCmd creates the list subcommand
func newLogsListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available log files",
		Long:  "Display all log files in the log directory with their sizes and modification times.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, err := config.GetLogDir()
			if err != nil {
				return fmt.Errorf("failed to get log directory: %w", err)
			}

			return listLogFiles(logDir)
		},
	}

	return cmd
}

// newLogsCleanCmd creates the clean subcommand
func newLogsCleanCmd() *cobra.Command {
	var dryRun bool
	var maxAge int

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Remove old log files",
		Long:  "Remove log files older than the specified age in days. Use --dry-run to see what would be deleted.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, err := config.GetLogDir()
			if err != nil {
				return fmt.Errorf("failed to get log directory: %w", err)
			}

			return cleanOldLogs(logDir, maxAge, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted without actually deleting")
	cmd.Flags().IntVar(&maxAge, "max-age", 30, "Maximum age of log files in days")
	return cmd
}

// newLogsPathCmd creates the path subcommand
func newLogsPathCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Show the path to the log directory",
		Long:  "Display the XDG-compliant path where logs are stored.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, err := config.GetLogDir()
			if err != nil {
				return fmt.Errorf("failed to get log directory: %w", err)
			}

			fmt.Println(logDir)
			return nil
		},
	}

	return cmd
}

// newLogsConsoleCmd creates the console subcommand for filtering console logs
func newLogsConsoleCmd() *cobra.Command {
	var lines int
	var level string

	cmd := &cobra.Command{
		Use:   "console",
		Short: "View console logs from WebKit",
		Long:  "Display console.log, console.error, and other console messages from web pages.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logDir, err := config.GetLogDir()
			if err != nil {
				return fmt.Errorf("failed to get log directory: %w", err)
			}

			currentLog := filepath.Join(logDir, "dumber.log")
			return tailConsoleMessages(currentLog, lines, level)
		},
	}

	cmd.Flags().IntVarP(&lines, "lines", "n", 50, "Number of lines to show")
	cmd.Flags().StringVarP(&level, "level", "l", "", "Filter by console level (error, warn, info, log, debug)")

	return cmd
}

// tailFile displays the last N lines of a file
func tailFile(filePath string, lines int) error {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Log file does not exist: %s\n", filePath)
			return nil
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read all lines
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	// Display last N lines
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}

	for i := start; i < len(allLines); i++ {
		fmt.Println(allLines[i])
	}

	return nil
}

// listLogFiles displays all log files with their information
func listLogFiles(logDir string) error {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		fmt.Printf("Log directory does not exist: %s\n", logDir)
		return nil
	}

	files, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}

	// Filter and collect log files
	var logFiles []os.FileInfo
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if strings.HasSuffix(name, ".log") || strings.Contains(name, "dumber.log") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			logFiles = append(logFiles, info)
		}
	}

	if len(logFiles) == 0 {
		fmt.Println("No log files found")
		return nil
	}

	// Sort by modification time (newest first)
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].ModTime().After(logFiles[j].ModTime())
	})

	// Display files
	fmt.Printf("%-30s %-20s %-10s\n", "FILENAME", "MODIFIED", "SIZE")
	fmt.Printf("%-30s %-20s %-10s\n", "--------", "--------", "----")

	for _, file := range logFiles {
		size := formatSize(file.Size())
		modTime := file.ModTime().Format("2006-01-02 15:04:05")
		fmt.Printf("%-30s %-20s %-10s\n", file.Name(), modTime, size)
	}

	return nil
}

// cleanOldLogs removes log files older than maxAge days
func cleanOldLogs(logDir string, maxAge int, dryRun bool) error {
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		fmt.Printf("Log directory does not exist: %s\n", logDir)
		return nil
	}

	files, err := os.ReadDir(logDir)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}

	cutoffTime := time.Now().AddDate(0, 0, -maxAge)
	var deletedCount int
	var deletedSize int64

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		// Skip the current log file
		if name == "dumber.log" {
			continue
		}

		// Only process log files
		if !strings.HasSuffix(name, ".log") && !strings.Contains(name, "dumber.log") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(logDir, name)

			if dryRun {
				fmt.Printf("Would delete: %s (%.2f KB, modified %s)\n",
					name, float64(info.Size())/1024, info.ModTime().Format("2006-01-02 15:04:05"))
			} else {
				err := os.Remove(filePath)
				if err != nil {
					fmt.Printf("Warning: failed to delete %s: %v\n", name, err)
				} else {
					fmt.Printf("Deleted: %s (%.2f KB)\n", name, float64(info.Size())/1024)
				}
			}

			deletedCount++
			deletedSize += info.Size()
		}
	}

	if dryRun {
		fmt.Printf("\nDry run complete. Would delete %d files (%.2f KB total)\n",
			deletedCount, float64(deletedSize)/1024)
	} else {
		fmt.Printf("\nCleaned %d files (%.2f KB total)\n",
			deletedCount, float64(deletedSize)/1024)
	}

	return nil
}

// formatSize formats file size in human-readable format
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

// tailConsoleMessages displays console messages from the log file
func tailConsoleMessages(filePath string, lines int, level string) error {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("Log file does not exist: %s\n", filePath)
			return nil
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	// Read all lines and filter for console messages
	var consoleLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Filter for [CONSOLE] tagged messages
		if !strings.Contains(line, "[CONSOLE]") {
			continue
		}

		// Filter by level if specified
		if level != "" {
			levelFilter := fmt.Sprintf("[%s]", strings.ToUpper(level))
			if !strings.Contains(line, levelFilter) {
				continue
			}
		}

		consoleLines = append(consoleLines, line)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading log file: %w", err)
	}

	if len(consoleLines) == 0 {
		if level != "" {
			fmt.Printf("No console messages found with level '%s'\n", level)
		} else {
			fmt.Println("No console messages found")
		}
		return nil
	}

	// Display last N lines
	start := len(consoleLines) - lines
	if start < 0 {
		start = 0
	}

	for i := start; i < len(consoleLines); i++ {
		fmt.Println(consoleLines[i])
	}

	return nil
}
