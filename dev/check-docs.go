//go:build devtools
// +build devtools

package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type issueLevel string

const (
	levelError   issueLevel = "ERROR"
	levelWarning issueLevel = "WARNING"
	levelInfo    issueLevel = "INFO"
)

type Issue struct {
	Level   issueLevel
	File    string
	Line    int
	Message string
	Detail  string
}

type Report struct {
	GeneratedAt time.Time
	Issues      []Issue
}

func (r *Report) Add(level issueLevel, file string, line int, message, detail string) {
	r.Issues = append(r.Issues, Issue{Level: level, File: file, Line: line, Message: message, Detail: detail})
}

func (r *Report) Count(level issueLevel) int {
	count := 0
	for _, it := range r.Issues {
		if it.Level == level {
			count++
		}
	}
	return count
}

func main() {
	failOnError := flag.Bool("fail-on-error", false, "exit non-zero if errors found")
	out := flag.String("out", "dev/docs-report.txt", "write report to file (relative to repo root unless absolute)")
	flag.Parse()

	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	report := &Report{GeneratedAt: time.Now()}

	schemaKeys, err := extractConfigKeys(filepath.Join(repoRoot, "internal", "infrastructure", "config", "schema.go"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to parse config schema:", err)
		os.Exit(2)
	}

	actionConstByName, err := extractActions(filepath.Join(repoRoot, "internal", "ui", "input", "shortcuts.go"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to parse shortcuts:", err)
		os.Exit(2)
	}

	stubActionConstNames, err := extractStubbedActionConstNames(filepath.Join(repoRoot, "internal", "ui", "dispatcher", "keyboard.go"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to parse keyboard dispatcher:", err)
		os.Exit(2)
	}
	stubActionValues := make(map[string]struct{}, len(stubActionConstNames))
	for _, name := range stubActionConstNames {
		if val, ok := actionConstByName[name]; ok {
			stubActionValues[val] = struct{}{}
		}
	}

	cliUses, err := extractCobraUseStrings(filepath.Join(repoRoot, "internal", "cli", "cmd"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to parse cli commands:", err)
		os.Exit(2)
	}

	readmePath := filepath.Join(repoRoot, "README.md")
	readmeLines, err := readLines(readmePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read README:", err)
		os.Exit(2)
	}
	configDocPath := filepath.Join(repoRoot, "docs", "CONFIG.md")
	configDocLines, err := readLines(configDocPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read docs/CONFIG.md:", err)
		os.Exit(2)
	}

	verifyConfigDoc(report, schemaKeys, configDocPath, configDocLines)
	verifyReadmeShortcuts(report, stubActionValues, readmePath, readmeLines)
	verifyReadmeCommands(report, cliUses, readmePath, readmeLines)
	verifyReadmeBuildClaims(report, readmePath, readmeLines)

	reportPath := *out
	if !filepath.IsAbs(reportPath) {
		reportPath = filepath.Join(repoRoot, reportPath)
	}
	if err := writeReport(reportPath, report); err != nil {
		fmt.Fprintln(os.Stderr, "failed to write report:", err)
		os.Exit(2)
	}

	printSummary(report, reportPath)

	if *failOnError && report.Count(levelError) > 0 {
		os.Exit(1)
	}
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := wd
	for {
		gitDir := filepath.Join(dir, ".git")
		if st, err := os.Stat(gitDir); err == nil && st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find repo root (.git)")
		}
		dir = parent
	}
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

type schemaField struct {
	Tag      string
	TypeName string
}

func extractConfigKeys(schemaPath string) (map[string]struct{}, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, schemaPath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	typeFields := map[string][]schemaField{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			var fields []schemaField
			for _, f := range st.Fields.List {
				if len(f.Names) == 0 {
					continue
				}
				tag := extractStructTagValue(f.Tag, "mapstructure")
				if tag == "" || tag == "-" {
					continue
				}
				fields = append(fields, schemaField{Tag: tag, TypeName: typeExprName(f.Type)})
			}
			typeFields[ts.Name.Name] = fields
		}
	}

	keys := make(map[string]struct{})
	seen := map[string]bool{}
	var walkType func(typeName, prefix string)
	walkType = func(typeName, prefix string) {
		if seen[prefix+"::"+typeName] {
			return
		}
		seen[prefix+"::"+typeName] = true

		fields, ok := typeFields[typeName]
		if !ok {
			return
		}
		for _, f := range fields {
			full := f.Tag
			if prefix != "" {
				full = prefix + "." + f.Tag
			}
			keys[full] = struct{}{}
			if _, ok := typeFields[f.TypeName]; ok {
				walkType(f.TypeName, full)
			}
		}
	}

	walkType("Config", "")
	return keys, nil
}

func extractStructTagValue(tagLit *ast.BasicLit, key string) string {
	if tagLit == nil {
		return ""
	}
	tag := strings.Trim(tagLit.Value, "`")
	parts := strings.Split(tag, " ")
	for _, part := range parts {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue
		}
		k := kv[0]
		v := strings.Trim(kv[1], "\"")
		if k != key {
			continue
		}
		if idx := strings.IndexByte(v, ','); idx >= 0 {
			return v[:idx]
		}
		return v
	}
	return ""
}

func typeExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		// pkg.Type
		return t.Sel.Name
	case *ast.StarExpr:
		return typeExprName(t.X)
	default:
		return ""
	}
}

func extractActions(shortcutsPath string) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, shortcutsPath, nil, 0)
	if err != nil {
		return nil, err
	}

	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				bl, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					continue
				}
				if strings.HasPrefix(name.Name, "Action") {
					out[name.Name] = strings.Trim(bl.Value, "\"")
				}
			}
		}
	}
	return out, nil
}

func extractStubbedActionConstNames(keyboardDispatcherPath string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, keyboardDispatcherPath, nil, 0)
	if err != nil {
		return nil, err
	}

	stubbedSet := map[string]struct{}{}

	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if _, ok := cl.Type.(*ast.MapType); !ok {
			return true
		}
		for _, elt := range cl.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyName := selectorName(kv.Key)
			if keyName == "" {
				continue
			}
			fn, ok := kv.Value.(*ast.FuncLit)
			if !ok {
				continue
			}
			if funcCallsSelector(fn, "logNoop") {
				stubbedSet[keyName] = struct{}{}
			}
		}
		return true
	})

	var stubbed []string
	for k := range stubbedSet {
		stubbed = append(stubbed, k)
	}
	sort.Strings(stubbed)
	return stubbed, nil
}

func selectorName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}

func funcCallsSelector(fn *ast.FuncLit, selName string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name == selName {
			found = true
			return false
		}
		return true
	})
	return found
}

func extractCobraUseStrings(cmdDir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return nil, err
	}

	uses := map[string]struct{}{}
	useRe := regexp.MustCompile(`\bUse:\s*"([^"]+)"`)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(cmdDir, e.Name()))
		if err != nil {
			return nil, err
		}
		matches := useRe.FindAllSubmatch(b, -1)
		for _, m := range matches {
			use := string(m[1])
			fields := strings.Fields(use)
			if len(fields) == 0 {
				continue
			}
			uses[fields[0]] = struct{}{}
		}
	}

	return uses, nil
}

func verifyConfigDoc(report *Report, schemaKeys map[string]struct{}, configDocPath string, lines []string) {
	docKeys := extractDocKeys(lines, schemaKeys)
	for key, line := range docKeys {
		if _, ok := schemaKeys[key]; ok {
			continue
		}
		report.Add(levelWarning, rel(configDocPath), line, "config key documented but not present in schema", fmt.Sprintf("key: %s", key))
	}
}

func extractDocKeys(lines []string, schemaKeys map[string]struct{}) map[string]int {
	keyRe := regexp.MustCompile("`([a-zA-Z0-9_.-]+)`")
	tableRowRe := regexp.MustCompile(`^\|`)
	inCode := false

	seen := map[string]int{}
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}

		// Prefer extracting keys from the first column of Markdown tables.
		// This avoids false-positives from enum values shown in other columns.
		if tableRowRe.MatchString(trim) {
			// Skip table separator rows.
			if strings.Contains(trim, "---") {
				continue
			}
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				firstCell := strings.TrimSpace(parts[1])
				matches := keyRe.FindAllStringSubmatch(firstCell, -1)
				for _, m := range matches {
					k := m[1]
					if k == "" || k[0] < 'a' || k[0] > 'z' {
						continue
					}
					if strings.ContainsAny(k, " /") {
						continue
					}
					if _, already := seen[k]; !already {
						seen[k] = i + 1
					}
				}
			}
			continue
		}

		// Outside of tables, only accept dotted paths that look like config keys.
		matches := keyRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			k := m[1]
			if _, ok := schemaKeys[k]; ok {
				if _, already := seen[k]; !already {
					seen[k] = i + 1
				}
				continue
			}
			if !strings.Contains(k, ".") {
				continue
			}
			if k == "" || k[0] < 'a' || k[0] > 'z' {
				continue
			}
			if strings.ContainsAny(k, " /") {
				continue
			}
			if _, already := seen[k]; !already {
				seen[k] = i + 1
			}
		}
	}
	return seen
}

func verifyReadmeShortcuts(report *Report, stubActionValues map[string]struct{}, readmePath string, lines []string) {
	stubToDocPatterns := map[string][]string{
		"rename_tab":        {"Rename Tab"},
		"toggle_fullscreen": {"Toggle Fullscreen"},
	}
	for action, patterns := range stubToDocPatterns {
		if _, stubbed := stubActionValues[action]; !stubbed {
			continue
		}
		for i, line := range lines {
			for _, p := range patterns {
				if strings.Contains(line, p) {
					report.Add(levelError, rel(readmePath), i+1, "documented shortcut/feature appears stubbed in code", fmt.Sprintf("action: %s", action))
					goto nextAction
				}
			}
		}
	nextAction:
	}
}

func verifyReadmeCommands(report *Report, cliUses map[string]struct{}, readmePath string, lines []string) {
	cmdRe := regexp.MustCompile("`dumber\\s+([^`]+)`")

	for i, line := range lines {
		matches := cmdRe.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			cmdLine := strings.TrimSpace(m[1])
			fields := strings.Fields(cmdLine)
			if len(fields) == 0 {
				continue
			}
			cmd := fields[0]
			// Ignore root flags like `dumber --dmenu`.
			if strings.HasPrefix(cmd, "-") {
				continue
			}
			if _, ok := cliUses[cmd]; !ok {
				report.Add(levelWarning, rel(readmePath), i+1, "documented CLI command not found in cobra commands", fmt.Sprintf("command: dumber %s", cmd))
			}
		}
	}
}

func verifyReadmeBuildClaims(report *Report, readmePath string, lines []string) {
	for i, line := range lines {
		low := strings.ToLower(line)
		if strings.Contains(low, "webkit_cgo") {
			report.Add(levelWarning, rel(readmePath), i+1, "README references old CGO build tag", "reference: webkit_cgo")
		}
		if strings.Contains(low, "cgo") && strings.Contains(low, "enabled") {
			report.Add(levelWarning, rel(readmePath), i+1, "README references CGO build requirements", "reference: CGO enabled")
		}
		if strings.Contains(low, "gotk4") {
			report.Add(levelWarning, rel(readmePath), i+1, "README references gotk4 (refactor moved to puregotk)", "reference: gotk4")
		}
	}
}

func writeReport(path string, report *Report) error {
	if report == nil {
		return errors.New("missing report")
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSeparator(w)
	_, _ = fmt.Fprintln(w, "DUMBER DOCUMENTATION VERIFICATION REPORT")
	writeSeparator(w)
	_, _ = fmt.Fprintf(w, "Generated: %s\n\n", report.GeneratedAt.Format(time.RFC3339))

	_, _ = fmt.Fprintf(w, "Summary: %d errors, %d warnings, %d info\n\n", report.Count(levelError), report.Count(levelWarning), report.Count(levelInfo))

	issues := append([]Issue(nil), report.Issues...)
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Level != issues[j].Level {
			return issues[i].Level < issues[j].Level
		}
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})

	for _, lvl := range []issueLevel{levelError, levelWarning, levelInfo} {
		writeSeparator(w)
		_, _ = fmt.Fprintf(w, "%sS\n", lvl)
		writeSeparator(w)
		idx := 0
		for _, it := range issues {
			if it.Level != lvl {
				continue
			}
			idx++
			loc := it.File
			if it.Line > 0 {
				loc = fmt.Sprintf("%s:%d", it.File, it.Line)
			}
			_, _ = fmt.Fprintf(w, "[%d] %s\n", idx, loc)
			_, _ = fmt.Fprintf(w, "    %s\n", it.Message)
			if it.Detail != "" {
				_, _ = fmt.Fprintf(w, "    %s\n", it.Detail)
			}
			_, _ = fmt.Fprintln(w)
		}
		if idx == 0 {
			_, _ = fmt.Fprintln(w, "(none)")
			_, _ = fmt.Fprintln(w)
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func writeSeparator(w io.Writer) {
	_, _ = fmt.Fprintln(w, strings.Repeat("=", 80))
}

func printSummary(report *Report, reportPath string) {
	fmt.Printf("check-docs: wrote %s\n", rel(reportPath))
	fmt.Printf("check-docs: %d errors, %d warnings, %d info\n", report.Count(levelError), report.Count(levelWarning), report.Count(levelInfo))
	if report.Count(levelError) > 0 {
		fmt.Printf("check-docs: FAIL\n")
	} else {
		fmt.Printf("check-docs: OK\n")
	}
}

func rel(path string) string {
	wd, err := os.Getwd()
	if err != nil {
		return path
	}
	rel, err := filepath.Rel(wd, path)
	if err != nil {
		return path
	}
	if strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}

var _ = errors.Is // keep go vet happy for future.
