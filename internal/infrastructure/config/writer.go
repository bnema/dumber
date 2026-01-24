package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/logging"
	"github.com/pelletier/go-toml/v2"
)

// WriteConfigOrdered writes the configuration to disk with consistent ordering.
// Uses atomic write (temp file + rename) to prevent race conditions with file watchers.
func WriteConfigOrdered(cfg *Config, path string) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	content, err := encodeConfigToTOML(cfg)
	if err != nil {
		return err
	}

	return atomicWriteFile(path, content, filePerm)
}

// encodeConfigToTOML encodes the config struct to sorted TOML content.
func encodeConfigToTOML(cfg *Config) (string, error) {
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.SetIndentTables(true)

	if err := enc.Encode(cfg); err != nil {
		return "", fmt.Errorf("failed to encode config: %w", err)
	}

	return sortTOMLSections(buf.String()), nil
}

// atomicWriteFile writes content to a file atomically using temp file + rename.
// This ensures file watchers only see complete content, not partial writes.
func atomicWriteFile(path, content string, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Cleanup on error
	success := false
	defer func() {
		if !success {
			if removeErr := os.Remove(tmpPath); removeErr != nil {
				log := logging.NewFromEnv()
				log.Warn().Err(removeErr).Str("path", tmpPath).Msg("failed to cleanup temp config file")
			}
		}
	}()

	if err := writeAndSync(tmpFile, content); err != nil {
		return err
	}

	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// writeAndSync writes content to file and syncs to disk.
func writeAndSync(f *os.File, content string) (err error) {
	// Ensure file is closed, capturing close error if no prior error
	defer func() {
		closeErr := f.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	if _, err = f.WriteString(content); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if err = f.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// tomlSection represents a parsed TOML section with its header and content lines.
type tomlSection struct {
	header string   // Section name without brackets (e.g., "workspace.shortcuts")
	lines  []string // All lines belonging to this section, including the header
}

// sortTOMLSections sorts TOML content so sections appear in alphabetical order.
func sortTOMLSections(content string) string {
	lines := strings.Split(content, "\n")
	preamble, sections := parseTOMLSections(lines)

	sort.Slice(sections, func(i, j int) bool {
		return sections[i].header < sections[j].header
	})

	return buildTOMLOutput(preamble, sections)
}

// parseTOMLSections parses TOML lines into preamble (top-level keys) and sections.
func parseTOMLSections(lines []string) (preamble []string, sections []tomlSection) {
	sectionRegex := regexp.MustCompile(`^(\s*)\[([^\]]+)\]\s*$`)
	var current *tomlSection

	for _, line := range lines {
		if match := sectionRegex.FindStringSubmatch(line); match != nil {
			if current != nil {
				sections = append(sections, *current)
			}
			current = &tomlSection{
				header: match[2],
				lines:  []string{line},
			}
		} else if current != nil {
			current.lines = append(current.lines, line)
		} else {
			preamble = append(preamble, line)
		}
	}

	if current != nil {
		sections = append(sections, *current)
	}

	return preamble, sections
}

// buildTOMLOutput reconstructs TOML content from preamble and sorted sections.
func buildTOMLOutput(preamble []string, sections []tomlSection) string {
	var result strings.Builder

	for _, line := range preamble {
		result.WriteString(line)
		result.WriteString("\n")
	}

	for i, sec := range sections {
		if i > 0 || len(preamble) > 0 {
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

	output := strings.TrimRight(result.String(), "\n")
	if output != "" {
		output += "\n"
	}

	return output
}
