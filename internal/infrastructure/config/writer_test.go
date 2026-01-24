package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfigOrdered(t *testing.T) {
	// Create a temp directory for the test
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Get default config
	cfg := DefaultConfig()

	// Write it
	err := WriteConfigOrdered(cfg, configPath)
	if err != nil {
		t.Fatalf("WriteConfigOrdered failed: %v", err)
	}

	// Read back
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read written config: %v", err)
	}

	// Verify sections are in alphabetical order
	lines := strings.Split(string(content), "\n")
	var sections []string
	for _, line := range lines {
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sections = append(sections, line)
		}
	}

	// Check that sections are sorted
	for i := 1; i < len(sections); i++ {
		if sections[i-1] > sections[i] {
			t.Errorf("Sections not sorted: %s > %s", sections[i-1], sections[i])
		}
	}

	t.Logf("Found %d sections, all sorted", len(sections))

	// Print first 50 lines for debugging
	t.Logf("First 50 lines of output:\n%s", strings.Join(lines[:min(50, len(lines))], "\n"))
}

func TestSortTOMLSections(t *testing.T) {
	input := `default_search_engine = 'https://example.com'

[workspace]
new_pane_url = 'about:blank'

[appearance]
color_scheme = 'default'

[workspace.pane_mode]
timeout_ms = 3000

[appearance.dark_palette]
background = '#000'
`

	result := sortTOMLSections(input)

	// Extract section order
	lines := strings.Split(result, "\n")
	var sections []string
	for _, line := range lines {
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sections = append(sections, line)
		}
	}

	// Expected order: appearance, appearance.dark_palette, workspace, workspace.pane_mode
	expected := []string{
		"[appearance]",
		"[appearance.dark_palette]",
		"[workspace]",
		"[workspace.pane_mode]",
	}

	if len(sections) != len(expected) {
		t.Fatalf("Expected %d sections, got %d: %v", len(expected), len(sections), sections)
	}

	for i, exp := range expected {
		if sections[i] != exp {
			t.Errorf("Section %d: expected %s, got %s", i, exp, sections[i])
		}
	}

	t.Logf("Sorted output:\n%s", result)
}
