package bootstrap

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	corelogging "github.com/bnema/dumber/internal/logging"
)

const (
	crashReportsDirName = "crashes"
	logTailLineCount    = 120
	maxCrashReportsKept = 20
	scannerMaxTokenSize = 256 * 1024
)

type crashReport struct {
	ReportVersion        int                `json:"report_version"`
	GeneratedAt          string             `json:"generated_at"`
	SessionID            string             `json:"session_id"`
	CrashedPID           int                `json:"crashed_pid,omitempty"`
	SessionLogFile       string             `json:"session_log_file,omitempty"`
	SessionLogTail       []string           `json:"session_log_tail_redacted,omitempty"`
	ReporterProcess      crashReporter      `json:"reporter_process"`
	CoreDumpDiagnostics  crashCoreDump      `json:"core_dump_diagnostics"`
	IssueTemplate        crashIssueTemplate `json:"issue_template"`
	GeneratedMarkdownRef string             `json:"generated_markdown_ref,omitempty"`
}

type crashReporter struct {
	GeneratedBy string `json:"generated_by"`
	GoVersion   string `json:"go_version"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	PID         int    `json:"pid"`
	PPID        int    `json:"ppid"`
}

type crashCoreDump struct {
	RLimitCoreSoft string `json:"rlimit_core_soft"`
	RLimitCoreHard string `json:"rlimit_core_hard"`
	Hint           string `json:"hint"`
}

type crashIssueTemplate struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// writeCrashReport generates JSON + Markdown crash reports for a crashed session.
// Returns the JSON report path, or empty string if the report already exists.
func writeCrashReport(logDir, sessionID string, crashedPID int) (string, error) {
	if logDir == "" || sessionID == "" {
		return "", errors.New("logDir and sessionID are required")
	}

	reportsDir := filepath.Join(logDir, crashReportsDirName)
	if err := os.MkdirAll(reportsDir, dirPerm); err != nil {
		return "", err
	}

	pruneOldCrashReports(reportsDir, maxCrashReportsKept)

	jsonPath := filepath.Join(reportsDir, fmt.Sprintf("session_%s.crash.json", sessionID))
	markdownPath := filepath.Join(reportsDir, fmt.Sprintf("session_%s.crash.md", sessionID))

	// Skip if report already exists.
	if _, err := os.Stat(jsonPath); err == nil {
		return jsonPath, nil
	}

	report := crashReport{
		ReportVersion: 2,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     sessionID,
		CrashedPID:    crashedPID,
		ReporterProcess: crashReporter{
			GeneratedBy: "dumber",
			GoVersion:   runtime.Version(),
			GOOS:        runtime.GOOS,
			GOARCH:      runtime.GOARCH,
			PID:         os.Getpid(),
			PPID:        os.Getppid(),
		},
		CoreDumpDiagnostics: collectCoreDumpDiagnostics(),
		IssueTemplate: crashIssueTemplate{
			Title:   fmt.Sprintf("Unexpected close in session %s", sessionID),
			Summary: "Dumber closed unexpectedly and generated this report automatically.",
		},
	}

	logPath := filepath.Join(logDir, corelogging.SessionFilename(sessionID))
	if _, err := os.Stat(logPath); err == nil {
		report.SessionLogFile = logPath
		report.SessionLogTail = readRedactedLogTail(logPath, logTailLineCount)
	}

	report.GeneratedMarkdownRef = markdownPath

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(jsonPath, payload, filePerm); err != nil {
		return "", err
	}

	markdown := buildCrashMarkdown(report)
	if err := os.WriteFile(markdownPath, []byte(markdown), filePerm); err != nil {
		return "", err
	}

	return jsonPath, nil
}

func buildCrashMarkdown(report crashReport) string {
	lines := []string{
		"# Unexpected Close Report",
		"",
		fmt.Sprintf("Generated: `%s`", report.GeneratedAt),
		fmt.Sprintf("Session: `%s`", report.SessionID),
		fmt.Sprintf("Crashed PID: `%d`", report.CrashedPID),
		"",
		"## Reporter",
		fmt.Sprintf("- pid: `%d`", report.ReporterProcess.PID),
		fmt.Sprintf("- ppid: `%d`", report.ReporterProcess.PPID),
		fmt.Sprintf("- go: `%s`", report.ReporterProcess.GoVersion),
		"",
		"## Core Dump Diagnostics",
		fmt.Sprintf("- RLIMIT_CORE soft: `%s`", report.CoreDumpDiagnostics.RLimitCoreSoft),
		fmt.Sprintf("- RLIMIT_CORE hard: `%s`", report.CoreDumpDiagnostics.RLimitCoreHard),
		fmt.Sprintf("- hint: %s", report.CoreDumpDiagnostics.Hint),
	}

	if len(report.SessionLogTail) > 0 {
		lines = append(lines,
			"",
			"## Redacted Log Tail",
			"```text",
		)
		lines = append(lines, report.SessionLogTail...)
		lines = append(lines, "```")
	}

	lines = append(lines,
		"",
		"## GitHub Issue Template",
		fmt.Sprintf("Title: %s", report.IssueTemplate.Title),
		"",
		"```markdown",
		"### What happened",
		"Describe what you were doing just before Dumber closed unexpectedly.",
		"",
		"### Crash report",
		fmt.Sprintf("- session id: `%s`", report.SessionID),
		fmt.Sprintf("- crashed pid: `%d`", report.CrashedPID),
		"",
		"### Additional context",
		"- distro:",
		"- compositor/window manager:",
		"- steps to reproduce:",
		"```",
	)

	return strings.Join(lines, "\n") + "\n"
}

// ---------------------------------------------------------------------------
// Redaction helpers (kept from previous implementation)
// ---------------------------------------------------------------------------

func readRedactedLogTail(path string, lines int) []string {
	if lines <= 0 {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	ring := make([]string, lines)
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, scannerMaxTokenSize), scannerMaxTokenSize)
	for scanner.Scan() {
		ring[count%lines] = redactSensitiveContent(scanner.Text())
		count++
	}

	var result []string
	if count <= lines {
		result = make([]string, count)
		copy(result, ring[:count])
	} else {
		result = make([]string, lines)
		start := count % lines
		copy(result, ring[start:])
		copy(result[lines-start:], ring[:start])
	}

	if scanner.Err() != nil {
		result = append(result, fmt.Sprintf("[log tail truncated: %v]", scanner.Err()))
	}
	return result
}

var urlRegex = regexp.MustCompile(`(?i)\b(?:https?|wss?)://[^\s\]\)\"'<>]+`)

var secretKeyRegex = regexp.MustCompile(`(?i)(token|access_token|id_token|code|password|passwd|secret|authorization)=([^&\s]+)`)

var secretJSONRegex = regexp.MustCompile(`(?i)"(token|access_token|id_token|password|secret)"\s*:\s*"(.*?)"`)

var secretHeaderRegex = regexp.MustCompile(`(?i)(authorization|auth):\s*(\S[^\n,;]*)`)

func redactSensitiveContent(line string) string {
	redacted := urlRegex.ReplaceAllStringFunc(line, redactURLString)
	redacted = secretKeyRegex.ReplaceAllString(redacted, "$1=[REDACTED]")
	redacted = secretJSONRegex.ReplaceAllString(redacted, `"$1":"[REDACTED]"`)
	redacted = secretHeaderRegex.ReplaceAllString(redacted, "$1: [REDACTED]")
	return redacted
}

func redactURLString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimRight(trimmed, ".,;:")
	u, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func pruneOldCrashReports(reportsDir string, maxKeep int) {
	matches, err := filepath.Glob(filepath.Join(reportsDir, "session_*.crash.json"))
	if err != nil || len(matches) <= maxKeep {
		return
	}

	type reportEntry struct {
		path    string
		modTime time.Time
	}
	entries := make([]reportEntry, 0, len(matches))
	for _, p := range matches {
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		entries = append(entries, reportEntry{path: p, modTime: info.ModTime()})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.After(entries[j].modTime)
	})

	for _, e := range entries[maxKeep:] {
		_ = os.Remove(e.path)
		mdPath := strings.TrimSuffix(e.path, ".json") + ".md"
		_ = os.Remove(mdPath)
	}
}
