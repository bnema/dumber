package styles

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bnema/dumber/internal/domain/entity"
)

// ConfigSchemaRenderer renders configuration schema information.
type ConfigSchemaRenderer struct {
	theme *Theme
}

// NewConfigSchemaRenderer creates a new ConfigSchemaRenderer.
func NewConfigSchemaRenderer(theme *Theme) *ConfigSchemaRenderer {
	return &ConfigSchemaRenderer{theme: theme}
}

// Render renders the configuration schema in styled format.
func (r *ConfigSchemaRenderer) Render(keys []entity.ConfigKeyInfo) string {
	if len(keys) == 0 {
		return r.theme.Subtle.Render("No configuration keys found")
	}

	// Group keys by section
	sections := groupBySection(keys)

	// Build output
	var parts []string

	// Header
	header := r.renderHeader()
	parts = append(parts, header, "")

	// Render each section
	sectionOrder := []string{
		"Appearance",
		"Logging",
		"History",
		"Search",
		"Dmenu",
		"Workspace",
		"Session",
		"Omnibox",
		"Content Filtering",
		"Clipboard",
		"Rendering",
		"Media",
		"Update",
		"Downloads",
		"Debug",
		"Performance",
		"Runtime",
		"Database",
	}

	for _, section := range sectionOrder {
		if sectionKeys, ok := sections[section]; ok {
			parts = append(parts, r.renderSection(section, sectionKeys), "")
		}
	}

	return strings.Join(parts, "\n")
}

// RenderJSON renders the configuration schema as JSON.
func (*ConfigSchemaRenderer) RenderJSON(keys []entity.ConfigKeyInfo) (string, error) {
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal schema: %w", err)
	}
	return string(data), nil
}

func (r *ConfigSchemaRenderer) renderHeader() string {
	iconStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	title := fmt.Sprintf("%s %s", iconStyle.Render(IconConfig), r.theme.Title.Render("Config Schema Reference"))
	return title
}

func groupBySection(keys []entity.ConfigKeyInfo) map[string][]entity.ConfigKeyInfo {
	sections := make(map[string][]entity.ConfigKeyInfo)
	for _, key := range keys {
		sections[key.Section] = append(sections[key.Section], key)
	}
	return sections
}

func (r *ConfigSchemaRenderer) renderSection(name string, keys []entity.ConfigKeyInfo) string {
	var lines []string

	for _, key := range keys {
		lines = append(lines, r.renderKey(key))
	}

	body := strings.Join(lines, "\n")

	// Create section header (no extra padding/margin)
	sectionHeader := r.theme.Highlight.Render(name)

	// Build box content: header directly followed by keys
	boxContent := sectionHeader + "\n" + body

	// Use box style without top padding
	boxStyle := r.theme.Box.PaddingTop(0)

	return boxStyle.Render(boxContent)
}

func (r *ConfigSchemaRenderer) renderKey(key entity.ConfigKeyInfo) string {
	keyStyle := r.theme.Normal.Bold(true)
	typeStyle := r.theme.Subtle
	defaultStyle := lipgloss.NewStyle().Foreground(r.theme.Accent)
	descStyle := r.theme.Subtle
	valuesStyle := r.theme.Normal

	// Line 1: key name, type, default
	line1 := fmt.Sprintf(
		"%s  %s  %s",
		keyStyle.Render(key.Key),
		typeStyle.Render(key.Type),
		defaultStyle.Render(key.Default),
	)

	// Line 2: description (indented)
	line2 := fmt.Sprintf("  %s", descStyle.Render(key.Description))

	result := line1 + "\n" + line2

	// Line 3 (optional): values or range
	if len(key.Values) > 0 {
		valuesText := "Values: " + strings.Join(key.Values, ", ")
		line3 := fmt.Sprintf("  %s", valuesStyle.Render(valuesText))
		result += "\n" + line3
	} else if key.Range != "" {
		rangeText := "Range: " + key.Range
		line3 := fmt.Sprintf("  %s", valuesStyle.Render(rangeText))
		result += "\n" + line3
	}

	return result
}
