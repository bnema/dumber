package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

type crashReportSummary struct {
	SessionID      string `json:"session_id"`
	GeneratedAt    string `json:"generated_at"`
	Classification struct {
		Class  string `json:"class"`
		Reason string `json:"reason"`
	} `json:"classification"`
}

var crashesCmd = &cobra.Command{
	Use:   "crashes",
	Short: "List unexpected-close crash reports",
	Long: `List crash reports generated after unexpected browser closes.

Use subcommands to inspect full details or print an issue template.

Examples:
  dumber crashes
  dumber crashes show latest
  dumber crashes issue latest`,
	RunE: runCrashes,
}

var crashesShowCmd = &cobra.Command{
	Use:   "show <report|latest>",
	Short: "Show a crash report",
	Args:  cobra.ExactArgs(1),
	RunE:  runCrashesShow,
}

var crashesIssueCmd = &cobra.Command{
	Use:   "issue <report|latest>",
	Short: "Print GitHub-ready issue body from report",
	Args:  cobra.ExactArgs(1),
	RunE:  runCrashesIssue,
}

func init() {
	rootCmd.AddCommand(crashesCmd)
	crashesCmd.AddCommand(crashesShowCmd)
	crashesCmd.AddCommand(crashesIssueCmd)
}

func runCrashes(_ *cobra.Command, _ []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}

	crashDir := getCrashReportsDir(app.Config)
	reports, err := loadCrashReports(crashDir)
	if err != nil {
		return err
	}
	if len(reports) == 0 {
		fmt.Println(app.Theme.Subtle.Render("No crash reports found."))
		return nil
	}

	fmt.Println(app.Theme.Title.Render("Crash reports (newest first):"))
	fmt.Println()
	for _, item := range reports {
		generated := item.GeneratedAt
		if ts, err := time.Parse(time.RFC3339Nano, item.GeneratedAt); err == nil {
			generated = ts.Format("2006-01-02 15:04:05")
		}
		shortID := shortCrashID(item.Path)
		line := fmt.Sprintf("  %s  %s  %s", app.Theme.Highlight.Render(shortID), app.Theme.Subtle.Render(generated), item.Classification.Class)
		fmt.Println(line)
	}

	fmt.Println()
	fmt.Println(app.Theme.Subtle.Render("Use 'dumber crashes show <id|latest>' for details"))
	return nil
}

func runCrashesShow(_ *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}
	renderer := styles.NewCrashesCLIRenderer(app.Theme)

	report, err := resolveCrashReport(getCrashReportsDir(app.Config), args[0])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n%s\n", renderer.RenderError(err), renderer.RenderHintList())
		return nil
	}

	body, err := os.ReadFile(report.MarkdownPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", renderer.RenderError(fmt.Errorf("read crash report markdown: %w", err)))
		return nil
	}
	fmt.Print(string(body))
	return nil
}

func runCrashesIssue(_ *cobra.Command, args []string) error {
	app := GetApp()
	if app == nil {
		return fmt.Errorf("app not initialized")
	}
	renderer := styles.NewCrashesCLIRenderer(app.Theme)

	report, err := resolveCrashReport(getCrashReportsDir(app.Config), args[0])
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n%s\n", renderer.RenderError(err), renderer.RenderHintList())
		return nil
	}

	body, err := os.ReadFile(report.MarkdownPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", renderer.RenderError(fmt.Errorf("read crash report markdown: %w", err)))
		return nil
	}
	all := string(body)
	start := strings.Index(all, "## GitHub Issue Template")
	if start < 0 {
		fmt.Print(all)
		return nil
	}
	fmt.Print(strings.TrimSpace(all[start:]) + "\n")
	return nil
}

type crashReportFile struct {
	Path         string
	MarkdownPath string
	crashReportSummary
}

func getCrashReportsDir(cfg *config.Config) string {
	logDir := ""
	if cfg != nil {
		logDir = cfg.Logging.LogDir
	}
	if logDir == "" {
		resolved, err := config.GetLogDir()
		if err == nil {
			logDir = resolved
		}
	}
	if logDir == "" {
		stateDir := os.Getenv("XDG_STATE_HOME")
		if stateDir == "" {
			home, _ := os.UserHomeDir()
			stateDir = filepath.Join(home, ".local", "state")
		}
		logDir = filepath.Join(stateDir, "dumber", "logs")
	}
	return filepath.Join(logDir, "crashes")
}

func loadCrashReports(crashDir string) ([]crashReportFile, error) {
	entries, err := os.ReadDir(crashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	reports := make([]crashReportFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".crash.json") {
			continue
		}
		path := filepath.Join(crashDir, entry.Name())
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var parsed crashReportSummary
		if unmarshalErr := json.Unmarshal(raw, &parsed); unmarshalErr != nil {
			continue
		}
		reports = append(reports, crashReportFile{
			Path:               path,
			MarkdownPath:       strings.TrimSuffix(path, ".json") + ".md",
			crashReportSummary: parsed,
		})
	}

	sort.Slice(reports, func(i, j int) bool {
		left, errLeft := time.Parse(time.RFC3339Nano, reports[i].GeneratedAt)
		right, errRight := time.Parse(time.RFC3339Nano, reports[j].GeneratedAt)
		if errLeft == nil && errRight == nil {
			return left.After(right)
		}
		return reports[i].Path > reports[j].Path
	})

	return reports, nil
}

func resolveCrashReport(crashDir, query string) (*crashReportFile, error) {
	reports, err := loadCrashReports(crashDir)
	if err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, fmt.Errorf("no crash reports found")
	}
	if strings.EqualFold(query, "latest") {
		return &reports[0], nil
	}

	query = strings.TrimSpace(strings.ToLower(query))
	for i := range reports {
		id := strings.ToLower(shortCrashID(reports[i].Path))
		if id == query {
			return &reports[i], nil
		}
	}

	matches := make([]int, 0, len(reports))
	for i := range reports {
		full := strings.ToLower(filepath.Base(reports[i].Path))
		if strings.Contains(full, query) || strings.Contains(strings.ToLower(reports[i].SessionID), query) {
			matches = append(matches, i)
		}
	}

	if len(matches) == 1 {
		return &reports[matches[0]], nil
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, idx := range matches {
			ids = append(ids, shortCrashID(reports[idx].Path))
		}
		return nil, fmt.Errorf("multiple reports match '%s': %s", query, strings.Join(ids, ", "))
	}

	return nil, fmt.Errorf("no crash report matching '%s' found", query)
}

func shortCrashID(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".crash.json")
	base = strings.TrimPrefix(base, "session_")
	return base
}
