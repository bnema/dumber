package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
)

var (
	logsFollow   bool
	logsLines    int
	logsClearAll bool
)

const defaultLogsLines = 50

var logsCmd = &cobra.Command{
	Use:   "logs [session]",
	Short: "View application logs",
	Long: `View dumber application logs by session.

Without arguments, lists all available sessions.
With a session ID (or partial match), shows logs for that session.

Examples:
  dumber logs                 # List all sessions
  dumber logs a7b3            # View logs for session ending in 'a7b3'
  dumber logs -f a7b3         # Follow logs in real-time
  dumber logs -n 100 a7b3     # Show last 100 lines`,
	RunE: runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow log output in real-time")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", defaultLogsLines, "number of lines to show")
}

// SessionInfo holds metadata about a log session.
type SessionInfo struct {
	SessionID string
	ShortID   string
	Filename  string
	Path      string
	Size      int64
	ModTime   time.Time
}

func runLogs(_ *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	logDir := getLogDir()

	// List sessions if no argument provided
	if len(args) == 0 {
		return listSessions(logDir, app.Theme)
	}

	// Find session by partial match
	sessionQuery := args[0]
	session, err := findSession(logDir, sessionQuery)
	if err != nil {
		return err
	}

	if logsFollow {
		return tailSession(session.Path, app.Theme)
	}

	return showSession(session.Path, logsLines, app.Theme)
}

// getLogDir returns the log directory path.
func getLogDir() string {
	logDir, err := config.GetLogDir()
	if err != nil {
		// Fallback to XDG default
		stateDir := os.Getenv("XDG_STATE_HOME")
		if stateDir == "" {
			home, _ := os.UserHomeDir()
			stateDir = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(stateDir, "dumber", "logs")
	}
	return logDir
}

// listSessions displays all available log sessions.
func listSessions(logDir string, theme *styles.Theme) error {
	sessions, err := getSessions(logDir)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println(theme.Subtle.Render("No sessions found. Run 'dumber browse' to create logs."))
		return nil
	}

	// Header
	fmt.Println(theme.Title.Render("Sessions (newest first):"))
	fmt.Println()

	// Table format: ShortID | DateTime | Size
	for _, s := range sessions {
		dateStr := s.ModTime.Format("2006-01-02 15:04:05")
		sizeStr := formatSize(s.Size)

		line := fmt.Sprintf("  %s  %s  %s",
			theme.Highlight.Render(s.ShortID),
			theme.Subtle.Render(dateStr),
			theme.Subtle.Render(fmt.Sprintf("(%s)", sizeStr)),
		)
		fmt.Println(line)
	}

	fmt.Println()
	fmt.Println(theme.Subtle.Render("Use 'dumber logs <id>' to view a session"))
	return nil
}

// getSessions returns all session log files, sorted by modification time (newest first).
func getSessions(logDir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read log directory: %w", err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		sessionID, ok := logging.ParseSessionFilename(entry.Name())
		if !ok {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		sessions = append(sessions, SessionInfo{
			SessionID: sessionID,
			ShortID:   logging.ShortSessionID(sessionID),
			Filename:  entry.Name(),
			Path:      filepath.Join(logDir, entry.Name()),
			Size:      info.Size(),
			ModTime:   info.ModTime(),
		})
	}

	// Sort by modification time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	return sessions, nil
}

// findSession finds a session by partial ID match.
func findSession(logDir, query string) (*SessionInfo, error) {
	sessions, err := getSessions(logDir)
	if err != nil {
		return nil, err
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	queryLower := strings.ToLower(query)

	// Try exact short ID match first
	for _, s := range sessions {
		if strings.EqualFold(s.ShortID, queryLower) {
			return &s, nil
		}
	}

	// Try partial match on full session ID
	var matches []SessionInfo
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.SessionID), queryLower) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session matching '%s' found", query)
	case 1:
		return &matches[0], nil
	default:
		// Multiple matches - show them and ask user to be more specific
		var ids []string
		for _, m := range matches {
			ids = append(ids, m.ShortID)
		}
		return nil, fmt.Errorf("multiple sessions match '%s': %s", query, strings.Join(ids, ", "))
	}
}

// showSession displays the last N lines of a session log.
func showSession(logPath string, lines int, theme *styles.Theme) (retErr error) {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("close log file: %w", closeErr)
		}
	}()

	// Read all lines
	var allLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read log file: %w", err)
	}

	// Get last N lines
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}

	for _, line := range allLines[start:] {
		fmt.Println(colorizeLogLine(line, theme))
	}

	return nil
}

// tailSession follows a session log in real-time.
func tailSession(logPath string, theme *styles.Theme) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Seek to end
	_, _ = file.Seek(0, io.SeekEnd)

	fmt.Println(theme.Subtle.Render("Following logs... (Ctrl+C to stop)"))
	fmt.Println()

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// No new data, wait a bit
			time.Sleep(100 * time.Millisecond)
			continue
		}
		fmt.Print(colorizeLogLine(line, theme))
	}
}

// logEntry represents a parsed JSON log entry.
type logEntry struct {
	Level   string `json:"level"`
	Time    string `json:"time"`
	Message string `json:"message"`
	Session string `json:"session"`
}

// colorizeLogLine adds color based on log level.
func colorizeLogLine(line string, theme *styles.Theme) string {
	// Try to parse as JSON
	var entry logEntry
	if err := json.Unmarshal([]byte(line), &entry); err == nil {
		return formatJSONLogLine(entry, theme)
	}

	// Fallback to pattern matching for non-JSON logs
	switch {
	case containsAny(line, "ERR", "ERROR", "error"):
		return theme.ErrorStyle.Render(line)
	case containsAny(line, "WRN", "WARN", "warn"):
		return theme.WarningStyle.Render(line)
	case containsAny(line, "DBG", "DEBUG", "debug"):
		return theme.Subtle.Render(line)
	default:
		return line
	}
}

// formatJSONLogLine formats a parsed JSON log entry with colors.
func formatJSONLogLine(entry logEntry, theme *styles.Theme) string {
	// Parse time if present
	timeStr := ""
	if entry.Time != "" {
		if t, err := time.Parse(time.RFC3339, entry.Time); err == nil {
			timeStr = t.Format("15:04:05")
		} else {
			timeStr = entry.Time
		}
	}

	// Level styling
	var levelStr string
	switch entry.Level {
	case "error":
		levelStr = theme.ErrorStyle.Render("ERR")
	case "warn":
		levelStr = theme.WarningStyle.Render("WRN")
	case "info":
		levelStr = theme.Highlight.Render("INF")
	case "debug":
		levelStr = theme.Subtle.Render("DBG")
	case "trace":
		levelStr = theme.Subtle.Render("TRC")
	default:
		levelStr = entry.Level
	}

	return fmt.Sprintf("%s %s %s", theme.Subtle.Render(timeStr), levelStr, entry.Message)
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	sLower := strings.ToLower(s)
	for _, substr := range substrs {
		if strings.Contains(sLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

// logsClearCmd clears old session logs.
var logsClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear old log files",
	Long: `Remove old session log files.

By default, removes sessions older than the configured MaxAge (default 7 days).
Use --all to remove all sessions.`,
	RunE: runLogsClear,
}

func init() {
	logsCmd.AddCommand(logsClearCmd)
	logsClearCmd.Flags().BoolVar(&logsClearAll, "all", false, "remove all session logs")
}

func runLogsClear(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	logDir := getLogDir()
	sessions, err := getSessions(logDir)
	if err != nil {
		return err
	}

	if len(sessions) == 0 {
		fmt.Println(app.Theme.Subtle.Render("No logs to clear"))
		return nil
	}

	// Get MaxAge from config (default 7 days)
	maxAge := 7
	if app.Config != nil && app.Config.Logging.MaxAge > 0 {
		maxAge = app.Config.Logging.MaxAge
	}

	cutoff := time.Now().AddDate(0, 0, -maxAge)
	var removed int

	for _, s := range sessions {
		shouldRemove := logsClearAll || s.ModTime.Before(cutoff)
		if !shouldRemove {
			continue
		}

		if err := os.Remove(s.Path); err != nil {
			fmt.Printf("%s %s: %v\n",
				app.Theme.ErrorStyle.Render("\u2717"),
				s.ShortID,
				err,
			)
			continue
		}

		fmt.Printf("%s %s (%s)\n",
			app.Theme.SuccessStyle.Render("\u2713"),
			s.ShortID,
			formatSize(s.Size),
		)
		removed++
	}

	if removed == 0 {
		fmt.Println(app.Theme.Subtle.Render("No sessions older than " + fmt.Sprintf("%d days", maxAge)))
	} else {
		fmt.Printf("\n%s\n", app.Theme.SuccessStyle.Render(fmt.Sprintf("Cleared %d session(s)", removed)))
	}

	return nil
}
