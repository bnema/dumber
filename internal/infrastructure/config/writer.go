package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// WriteConfigOrdered writes the configuration to disk with consistent ordering.
// - Struct fields are written in definition order (go-toml v2 behavior)
// - TOML sections are sorted alphabetically within each parent for deterministic output
func WriteConfigOrdered(cfg *Config, path string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)

	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	// Post-process to sort TOML sections alphabetically
	sorted := sortTOMLSections(buf.String())

	if err := os.WriteFile(path, []byte(sorted), filePerm); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// sortTOMLSections sorts TOML content so sections are in alphabetical order.
// This handles both top-level sections and indented nested sections.
func sortTOMLSections(content string) string {
	lines := strings.Split(content, "\n")

	// Parse into sections
	type section struct {
		header string   // e.g., "appearance" or "workspace.pane_mode.actions.cancel"
		lines  []string // lines belonging to this section (including header)
	}

	var sections []section
	var currentSection *section
	var preamble []string // lines before first section

	// Match section headers with optional leading whitespace (for indented sub-tables)
	sectionRegex := regexp.MustCompile(`^(\s*)\[([^\]]+)\]\s*$`)

	for _, line := range lines {
		if match := sectionRegex.FindStringSubmatch(line); match != nil {
			// New section found
			if currentSection != nil {
				sections = append(sections, *currentSection)
			}
			currentSection = &section{
				header: match[2], // Just the section name, without brackets or indent
				lines:  []string{line},
			}
		} else if currentSection != nil {
			currentSection.lines = append(currentSection.lines, line)
		} else {
			// Before any section (top-level keys)
			preamble = append(preamble, line)
		}
	}

	// Don't forget the last section
	if currentSection != nil {
		sections = append(sections, *currentSection)
	}

	// Sort sections alphabetically by header
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].header < sections[j].header
	})

	// Rebuild content
	var result strings.Builder

	// Write preamble (top-level keys) first
	for _, line := range preamble {
		result.WriteString(line)
		result.WriteString("\n")
	}

	// Write sorted sections
	for i, sec := range sections {
		// Add blank line before section (except first if preamble is empty)
		if i > 0 || len(preamble) > 0 {
			// Check if previous content already ends with blank line
			content := result.String()
			if !strings.HasSuffix(content, "\n\n") && content != "" {
				result.WriteString("\n")
			}
		}

		for _, line := range sec.lines {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	// Trim trailing whitespace but ensure single newline at end
	output := strings.TrimRight(result.String(), "\n")
	if output != "" {
		output += "\n"
	}

	return output
}
