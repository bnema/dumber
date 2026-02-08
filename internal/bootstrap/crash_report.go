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
	"strconv"
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

type unexpectedCloseReport struct {
	ReportVersion        int                          `json:"report_version"`
	GeneratedAt          string                       `json:"generated_at"`
	SessionID            string                       `json:"session_id"`
	Classification       SessionExitClassification    `json:"classification"`
	StartupPID           int                          `json:"startup_pid,omitempty"`
	StartupPPID          int                          `json:"startup_ppid,omitempty"`
	MarkerFiles          map[string]string            `json:"marker_files"`
	SessionLogFile       string                       `json:"session_log_file,omitempty"`
	SessionLogTail       []string                     `json:"session_log_tail_redacted,omitempty"`
	ReporterProcess      unexpectedCloseReporter      `json:"reporter_process"`
	CoreDumpDiagnostics  unexpectedCloseCoreDump      `json:"core_dump_diagnostics"`
	IssueTemplate        unexpectedCloseIssueTemplate `json:"issue_template"`
	GeneratedMarkdownRef string                       `json:"generated_markdown_ref,omitempty"`
}

type unexpectedCloseReporter struct {
	GeneratedBy string `json:"generated_by"`
	GoVersion   string `json:"go_version"`
	GOOS        string `json:"goos"`
	GOARCH      string `json:"goarch"`
	PID         int    `json:"pid"`
	PPID        int    `json:"ppid"`
}

type unexpectedCloseCoreDump struct {
	RLimitCoreSoft string `json:"rlimit_core_soft"`
	RLimitCoreHard string `json:"rlimit_core_hard"`
	Hint           string `json:"hint"`
}

type unexpectedCloseIssueTemplate struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

func writeUnexpectedCloseReport(lockDir, sessionID string) (string, error) {
	if lockDir == "" || sessionID == "" {
		return "", errors.New("lockDir and sessionID are required")
	}

	classification, classifyErr := ClassifySessionExitFromMarkers(lockDir, sessionID)
	if classifyErr != nil {
		return "", classifyErr
	}

	if classification.Class == SessionExitCleanExit {
		return "", nil
	}

	reportsDir := filepath.Join(lockDir, crashReportsDirName)
	if err := os.MkdirAll(reportsDir, lockDirPerm); err != nil {
		return "", err
	}

	pruneOldCrashReports(reportsDir, maxCrashReportsKept)

	jsonPath := filepath.Join(reportsDir, fmt.Sprintf("session_%s.crash.json", sessionID))
	markdownPath := filepath.Join(reportsDir, fmt.Sprintf("session_%s.crash.md", sessionID))
	if _, statErr := os.Stat(jsonPath); statErr == nil {
		if _, mdErr := os.Stat(markdownPath); mdErr == nil {
			return jsonPath, nil
		}
		raw, readErr := os.ReadFile(jsonPath)
		if readErr != nil {
			return "", readErr
		}
		var existing unexpectedCloseReport
		if err := json.Unmarshal(raw, &existing); err != nil {
			return "", err
		}
		markdown := buildUnexpectedCloseMarkdown(existing)
		if err := os.WriteFile(markdownPath, []byte(markdown), markerFilePerm); err != nil {
			return "", err
		}
		return jsonPath, nil
	} else if !os.IsNotExist(statErr) {
		return "", statErr
	}

	report := unexpectedCloseReport{
		ReportVersion:       1,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:           sessionID,
		Classification:      classification,
		MarkerFiles:         reportMarkerFiles(lockDir, sessionID),
		ReporterProcess:     currentReporter(),
		CoreDumpDiagnostics: collectCoreDumpDiagnostics(),
		IssueTemplate: unexpectedCloseIssueTemplate{
			Title:   fmt.Sprintf("Unexpected close in session %s", sessionID),
			Summary: "Dumber closed unexpectedly and generated this report automatically.",
		},
	}

	startupPID, startupPPID := readStartupProcessIDs(lockDir, sessionID)
	report.StartupPID = startupPID
	report.StartupPPID = startupPPID

	logPath := filepath.Join(lockDir, corelogging.SessionFilename(sessionID))
	if _, statErr := os.Stat(logPath); statErr == nil {
		report.SessionLogFile = logPath
		report.SessionLogTail = readRedactedLogTail(logPath, logTailLineCount)
	}

	report.GeneratedMarkdownRef = markdownPath

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(jsonPath, payload, markerFilePerm); err != nil {
		return "", err
	}

	markdown := buildUnexpectedCloseMarkdown(report)
	if err := os.WriteFile(markdownPath, []byte(markdown), markerFilePerm); err != nil {
		return "", err
	}

	return jsonPath, nil
}

func reportMarkerFiles(lockDir, sessionID string) map[string]string {
	return map[string]string{
		"startup":  startupMarkerPath(lockDir, sessionID),
		"shutdown": shutdownMarkerPath(lockDir, sessionID),
		"abrupt":   abruptMarkerPath(lockDir, sessionID),
	}
}

func currentReporter() unexpectedCloseReporter {
	return unexpectedCloseReporter{
		GeneratedBy: "dumber",
		GoVersion:   runtime.Version(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
		PID:         os.Getpid(),
		PPID:        os.Getppid(),
	}
}

func readStartupProcessIDs(lockDir, sessionID string) (int, int) {
	raw, err := os.ReadFile(startupMarkerPath(lockDir, sessionID))
	if err != nil {
		raw, err = os.ReadFile(abruptMarkerPath(lockDir, sessionID))
		if err != nil {
			return 0, 0
		}
	}

	pid, _ := strconv.Atoi(markerValue(raw, "pid="))
	ppid, _ := strconv.Atoi(markerValue(raw, "ppid="))
	return pid, ppid
}

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

func buildUnexpectedCloseMarkdown(report unexpectedCloseReport) string {
	lines := []string{
		"# Unexpected Close Report",
		"",
		fmt.Sprintf("Generated: `%s`", report.GeneratedAt),
		fmt.Sprintf("Session: `%s`", report.SessionID),
		fmt.Sprintf("Class: `%s`", report.Classification.Class),
		fmt.Sprintf("Inference: `%s`", report.Classification.Inference),
		fmt.Sprintf("Reason: `%s`", report.Classification.Reason),
		"",
		"## Process Context",
		fmt.Sprintf("- startup pid: `%d`", report.StartupPID),
		fmt.Sprintf("- startup ppid: `%d`", report.StartupPPID),
		fmt.Sprintf("- reporter pid: `%d`", report.ReporterProcess.PID),
		fmt.Sprintf("- reporter ppid: `%d`", report.ReporterProcess.PPID),
		"",
		"## Core Dump Diagnostics",
		fmt.Sprintf("- RLIMIT_CORE soft: `%s`", report.CoreDumpDiagnostics.RLimitCoreSoft),
		fmt.Sprintf("- RLIMIT_CORE hard: `%s`", report.CoreDumpDiagnostics.RLimitCoreHard),
		fmt.Sprintf("- hint: %s", report.CoreDumpDiagnostics.Hint),
		"",
		"## Marker Files",
		fmt.Sprintf("- startup: `%s`", report.MarkerFiles["startup"]),
		fmt.Sprintf("- shutdown: `%s`", report.MarkerFiles["shutdown"]),
		fmt.Sprintf("- abrupt: `%s`", report.MarkerFiles["abrupt"]),
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
		fmt.Sprintf("- class: `%s`", report.Classification.Class),
		fmt.Sprintf("- reason: `%s`", report.Classification.Reason),
		"",
		"### Additional context",
		"- distro:",
		"- compositor/window manager:",
		"- steps to reproduce:",
		"```",
	)

	return strings.Join(lines, "\n") + "\n"
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
