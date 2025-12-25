package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/model"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/desktop"
)

const defaultSessionsLimit = 20

var (
	sessionsJSON  bool
	sessionsLimit int
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage browser sessions",
	Long: `View, restore, and manage browser sessions.

Sessions are automatically saved when the browser runs. You can list
past sessions and restore them to continue where you left off.

Run without arguments to open the interactive session browser.`,
	RunE: runSessions,
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
}

func runSessions(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	if app.ListSessionsUC == nil {
		return fmt.Errorf("session management not available")
	}

	// Get current session ID (empty if not running as browser)
	var currentSessionID entity.SessionID
	if active, err := app.SessionUC.GetActiveSession(app.Ctx()); err == nil && active != nil {
		currentSessionID = active.ID
	}

	// Run interactive TUI
	m := model.NewSessionsModel(app.Ctx(), app.Theme, model.SessionsModelConfig{
		ListSessionsUC:    app.ListSessionsUC,
		RestoreUC:         app.RestoreUC,
		DeleteSessionUC:   app.DeleteSessionUC,
		CurrentSession:    currentSessionID,
		MaxListedSessions: app.Config.Session.MaxListedSessions,
	})

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// sessions list
var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved sessions",
	Long: `List all saved browser sessions with their tab/pane counts.

Sessions are marked as:
  ● current  - the currently running session
  ○ active   - another running dumber instance
  (blank)    - exited session available for restoration`,
	RunE: runSessionsList,
}

func init() {
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsListCmd.Flags().BoolVar(&sessionsJSON, "json", false, "output as JSON")
	sessionsListCmd.Flags().IntVar(&sessionsLimit, "limit", defaultSessionsLimit, "maximum sessions to show")
}

func runSessionsList(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	if app.ListSessionsUC == nil {
		return fmt.Errorf("session management not available")
	}

	// Get current session ID (empty if not running as browser)
	var currentSessionID entity.SessionID
	if active, err := app.SessionUC.GetActiveSession(app.Ctx()); err == nil && active != nil {
		currentSessionID = active.ID
	}

	output, err := app.ListSessionsUC.Execute(app.Ctx(), currentSessionID, sessionsLimit)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}

	if sessionsJSON {
		return outputSessionsJSON(output.Sessions)
	}

	return outputSessionsTable(output.Sessions)
}

func outputSessionsJSON(sessions []entity.SessionInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(sessions)
}

func outputSessionsTable(sessions []entity.SessionInfo) error {
	if len(sessions) == 0 {
		fmt.Println("No saved sessions found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "STATUS\tSESSION ID\tTABS\tPANES\tLAST UPDATED")

	for _, info := range sessions {
		status := " "
		switch {
		case info.IsCurrent:
			status = "●"
		case info.IsActive:
			status = "○"
		}

		relTime := usecase.GetRelativeTime(info.UpdatedAt)
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n",
			status,
			info.Session.ID,
			info.TabCount,
			info.PaneCount,
			relTime,
		)
	}

	return w.Flush()
}

// sessions restore <id>
var sessionsRestoreCmd = &cobra.Command{
	Use:   "restore <session-id>",
	Short: "Restore a saved session",
	Long: `Restore a previously saved browser session.

This launches a new browser window with all tabs and panes from the
saved session. The session ID can be found using 'dumber sessions list'.

You can use a short suffix of the session ID as long as it's unique.

Example:
  dumber sessions restore 20251224_143022_abc1
  dumber sessions restore abc1`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionsRestore,
}

func init() {
	sessionsCmd.AddCommand(sessionsRestoreCmd)
}

func runSessionsRestore(_ *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	if app.RestoreUC == nil || app.ListSessionsUC == nil {
		return fmt.Errorf("session restoration not available")
	}

	// Find session by ID or suffix
	sessionInfo, err := findSessionByIDOrSuffix(args[0])
	if err != nil {
		return err
	}

	sessionID := sessionInfo.Session.ID

	// Validate the session has restorable state
	_, err = app.RestoreUC.Execute(app.Ctx(), usecase.RestoreInput{SessionID: sessionID})
	if err != nil {
		return fmt.Errorf("restore session: %w", err)
	}

	// Spawn a new dumber instance with the session
	spawner := desktop.NewSessionSpawner(app.Ctx())
	if err := spawner.SpawnWithSession(sessionID); err != nil {
		return fmt.Errorf("spawn browser: %w", err)
	}

	fmt.Printf("Restoring session %s...\n", sessionID)
	return nil
}

// sessions delete <id>
var sessionsDeleteCmd = &cobra.Command{
	Use:   "delete <session-id>",
	Short: "Delete a saved session",
	Long: `Delete a saved browser session and its state.

This permanently removes the session snapshot. Active sessions cannot
be deleted - close the browser first.

You can use a short suffix of the session ID as long as it's unique.

Example:
  dumber sessions delete 20251224_143022_abc1
  dumber sessions delete abc1`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionsDelete,
}

func init() {
	sessionsCmd.AddCommand(sessionsDeleteCmd)
}

func runSessionsDelete(_ *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	if app.DeleteSessionUC == nil || app.ListSessionsUC == nil {
		return fmt.Errorf("session management not available")
	}

	// Find session by ID or suffix
	sessionInfo, err := findSessionByIDOrSuffix(args[0])
	if err != nil {
		return err
	}

	// Get current session ID
	var currentSessionID entity.SessionID
	if active, activeErr := app.SessionUC.GetActiveSession(app.Ctx()); activeErr == nil && active != nil {
		currentSessionID = active.ID
	}

	// Delete using use case (handles validation internally)
	if err := app.DeleteSessionUC.Execute(app.Ctx(), usecase.DeleteSessionInput{
		SessionID:        sessionInfo.Session.ID,
		CurrentSessionID: currentSessionID,
	}); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	fmt.Printf("Session %s deleted.\n", sessionInfo.Session.ID)
	return nil
}

// findSessionByIDOrSuffix finds a session by exact ID or unique suffix.
// Users typically identify sessions by the last few characters (e.g., "dee5").
func findSessionByIDOrSuffix(idOrSuffix string) (*entity.SessionInfo, error) {
	app := GetApp()
	if app == nil || app.ListSessionsUC == nil {
		return nil, fmt.Errorf("app not initialized")
	}

	// Get current session ID (empty if not running as browser)
	var currentSessionID entity.SessionID
	if active, err := app.SessionUC.GetActiveSession(app.Ctx()); err == nil && active != nil {
		currentSessionID = active.ID
	}

	// List all sessions
	output, err := app.ListSessionsUC.Execute(app.Ctx(), currentSessionID, defaultSessionsLimit*5)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	// Find matching sessions
	var matches []entity.SessionInfo
	for _, info := range output.Sessions {
		if string(info.Session.ID) == idOrSuffix {
			// Exact match - return immediately
			return &info, nil
		}
		if strings.HasSuffix(string(info.Session.ID), idOrSuffix) {
			matches = append(matches, info)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session found matching '%s'", idOrSuffix)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous session ID '%s' matches %d sessions - be more specific", idOrSuffix, len(matches))
	}
}
