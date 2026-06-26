package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/cli/model"
	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
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
	cliApp := GetApp()
	if cliApp == nil {
		return fmt.Errorf("app not initialized")
	}

	if cliApp.ListSessionsUC == nil || cliApp.SessionUC == nil {
		return fmt.Errorf("session management not available")
	}

	// Get current session ID (empty if not running as browser)
	var currentSessionID entity.SessionID
	if active, err := cliApp.SessionUC.GetActiveSession(cliApp.Ctx()); err == nil && active != nil {
		currentSessionID = active.ID
	}

	// Run interactive TUI
	m := model.NewSessionsModel(cliApp.Ctx(), cliApp.Theme, model.SessionsModelConfig{
		ListSessionsUC:    cliApp.ListSessionsUC,
		RestoreUC:         cliApp.RestoreUC,
		DeleteSessionUC:   cliApp.DeleteSessionUC,
		SessionSpawner:    cliApp.SessionSpawner,
		CurrentSession:    currentSessionID,
		MaxExitedSessions: cliApp.Config.Session.MaxExitedSessions,
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
	cliApp := GetApp()
	if cliApp == nil {
		return fmt.Errorf("app not initialized")
	}
	renderer := styles.NewSessionsCLIRenderer(cliApp.Theme)

	if cliApp.ListSessionsUC == nil || cliApp.SessionUC == nil {
		err := fmt.Errorf("session management not available")
		fmt.Fprintln(os.Stderr, renderer.RenderError(err))
		return wrapPrintedError(err)
	}

	// Get current session ID (empty if not running as browser)
	var currentSessionID entity.SessionID
	if active, err := cliApp.SessionUC.GetActiveSession(cliApp.Ctx()); err == nil && active != nil {
		currentSessionID = active.ID
	}

	output, err := cliApp.ListSessionsUC.Execute(cliApp.Ctx(), currentSessionID, sessionsLimit)
	if err != nil {
		wrappedErr := fmt.Errorf("list sessions: %w", err)
		fmt.Fprintln(os.Stderr, renderer.RenderError(wrappedErr))
		return wrapPrintedError(wrappedErr)
	}

	if sessionsJSON {
		return outputSessionsJSON(output.Sessions)
	}

	fmt.Println(renderer.RenderList(output.Sessions, sessionsLimit))
	return nil
}

func outputSessionsJSON(sessions []entity.SessionInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(sessions)
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
	cliApp := GetApp()
	if cliApp == nil {
		return fmt.Errorf("app not initialized")
	}
	renderer := styles.NewSessionsCLIRenderer(cliApp.Theme)

	if cliApp.RestoreUC == nil || cliApp.ListSessionsUC == nil || cliApp.SessionUC == nil {
		err := fmt.Errorf("session restoration not available")
		fmt.Fprintln(os.Stderr, renderer.RenderError(err))
		return wrapPrintedError(err)
	}

	// Find session by ID or suffix
	sessionInfo, err := findSessionByIDOrSuffix(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, renderer.RenderError(err))
		return wrapPrintedError(err)
	}

	sessionID := sessionInfo.Session.ID

	// Validate the session has restorable state
	_, err = cliApp.RestoreUC.Execute(cliApp.Ctx(), usecase.RestoreInput{SessionID: sessionID})
	if err != nil {
		wrappedErr := fmt.Errorf("restore session: %w", err)
		fmt.Fprintln(os.Stderr, renderer.RenderError(wrappedErr))
		return wrapPrintedError(wrappedErr)
	}

	// Spawn a new dumber instance with the session
	if cliApp.SessionSpawner == nil {
		err := fmt.Errorf("session spawner not available")
		fmt.Fprintln(os.Stderr, renderer.RenderError(err))
		return wrapPrintedError(err)
	}
	if err := cliApp.SessionSpawner.SpawnWithSession(sessionID); err != nil {
		wrappedErr := fmt.Errorf("spawn browser: %w", err)
		fmt.Fprintln(os.Stderr, renderer.RenderError(wrappedErr))
		return wrapPrintedError(wrappedErr)
	}

	fmt.Println(renderer.RenderRestoreStarted(sessionID))
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
	cliApp := GetApp()
	if cliApp == nil {
		return fmt.Errorf("app not initialized")
	}
	renderer := styles.NewSessionsCLIRenderer(cliApp.Theme)

	if cliApp.DeleteSessionUC == nil || cliApp.ListSessionsUC == nil || cliApp.SessionUC == nil {
		err := fmt.Errorf("session management not available")
		fmt.Fprintln(os.Stderr, renderer.RenderError(err))
		return wrapPrintedError(err)
	}

	// Find session by ID or suffix
	sessionInfo, err := findSessionByIDOrSuffix(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, renderer.RenderError(err))
		return wrapPrintedError(err)
	}

	// Get current session ID
	var currentSessionID entity.SessionID
	if active, activeErr := cliApp.SessionUC.GetActiveSession(cliApp.Ctx()); activeErr == nil && active != nil {
		currentSessionID = active.ID
	}

	// Delete using use case (handles validation internally)
	if err := cliApp.DeleteSessionUC.Execute(cliApp.Ctx(), usecase.DeleteSessionInput{
		SessionID:        sessionInfo.Session.ID,
		CurrentSessionID: currentSessionID,
	}); err != nil {
		wrappedErr := fmt.Errorf("delete session: %w", err)
		fmt.Fprintln(os.Stderr, renderer.RenderError(wrappedErr))
		return wrapPrintedError(wrappedErr)
	}

	fmt.Println(renderer.RenderDeleted(sessionInfo.Session.ID))
	return nil
}

// findSessionByIDOrSuffix finds a session by exact ID or unique suffix.
// Users typically identify sessions by the last few characters (e.g., "dee5").
func findSessionByIDOrSuffix(idOrSuffix string) (*entity.SessionInfo, error) {
	cliApp := GetApp()
	if cliApp == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	if cliApp.ListSessionsUC == nil || cliApp.SessionUC == nil {
		return nil, fmt.Errorf("session management not available")
	}

	ctx := cliApp.Ctx()
	if ctx == nil {
		ctx = context.Background()
	}

	// Get current session ID (empty if not running as browser)
	var currentSessionID entity.SessionID
	if active, err := cliApp.SessionUC.GetActiveSession(ctx); err == nil && active != nil {
		currentSessionID = active.ID
	}

	// List all sessions so restore/delete suffix lookup can find older sessions.
	output, err := cliApp.ListSessionsUC.Execute(ctx, currentSessionID, usecase.ListAllSessionsLimit)
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
