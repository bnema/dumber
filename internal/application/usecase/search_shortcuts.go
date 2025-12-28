package usecase

import (
	"context"
	"sort"
	"strings"

	"github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
)

// SearchShortcut represents a search shortcut configuration.
// This is a domain-level type to avoid depending on infrastructure config.
type SearchShortcut struct {
	URL         string
	Description string
}

// BangSuggestion represents a configured bang shortcut for display.
type BangSuggestion struct {
	Key         string
	Description string
}

// SearchShortcutsUseCase handles bang shortcut filtering and resolution.
type SearchShortcutsUseCase struct {
	shortcuts map[string]SearchShortcut
}

// NewSearchShortcutsUseCase creates a new search shortcuts use case.
func NewSearchShortcutsUseCase(shortcuts map[string]SearchShortcut) *SearchShortcutsUseCase {
	return &SearchShortcutsUseCase{
		shortcuts: shortcuts,
	}
}

// FilterBangsInput contains parameters for filtering bang suggestions.
type FilterBangsInput struct {
	Query string // e.g., "!" or "!g" or "!g query"
}

// FilterBangsOutput contains filtered bang suggestions.
type FilterBangsOutput struct {
	Suggestions []BangSuggestion
}

// FilterBangs returns bang shortcuts matching the query prefix.
// Query should start with "!". The prefix is extracted before any space.
func (uc *SearchShortcutsUseCase) FilterBangs(ctx context.Context, input FilterBangsInput) *FilterBangsOutput {
	log := logging.FromContext(ctx)

	suggestions := uc.buildBangSuggestions(input.Query)

	log.Debug().
		Str("query", input.Query).
		Int("matches", len(suggestions)).
		Msg("filtered bang suggestions")

	return &FilterBangsOutput{Suggestions: suggestions}
}

// DetectBangKeyInput contains parameters for detecting a completed bang key.
type DetectBangKeyInput struct {
	Query string // e.g., "!gh query"
}

// DetectBangKeyOutput contains the detected bang key if found.
type DetectBangKeyOutput struct {
	Key         string // The matched key (empty if not found)
	Description string // Description of the matched shortcut
}

// DetectBangKey checks if the query contains a valid, completed bang key.
// A completed bang key requires "!<key> " with a space after the key.
func (uc *SearchShortcutsUseCase) DetectBangKey(ctx context.Context, input DetectBangKeyInput) *DetectBangKeyOutput {
	log := logging.FromContext(ctx)

	key := uc.detectBangKey(input.Query)
	if key == "" {
		return &DetectBangKeyOutput{}
	}

	shortcut, ok := uc.shortcuts[key]
	if !ok {
		return &DetectBangKeyOutput{}
	}

	description := shortcut.Description
	if description == "" {
		description = shortcut.URL
	}

	log.Debug().
		Str("query", input.Query).
		Str("detected_key", key).
		Msg("detected bang key")

	return &DetectBangKeyOutput{
		Key:         key,
		Description: description,
	}
}

// BuildNavigationTextInput contains parameters for building navigation text.
type BuildNavigationTextInput struct {
	EntryText string // e.g., "!GH dumber"
}

// BuildNavigationTextOutput contains the normalized navigation text.
type BuildNavigationTextOutput struct {
	Text  string // Normalized text (e.g., "!gh dumber") or empty if invalid
	Valid bool   // Whether the entry text represents a valid bang navigation
}

// BuildNavigationText normalizes a bang query for navigation.
// Returns the normalized text with the canonical key case.
func (uc *SearchShortcutsUseCase) BuildNavigationText(ctx context.Context, input BuildNavigationTextInput) *BuildNavigationTextOutput {
	log := logging.FromContext(ctx)

	text := uc.buildBangNavigationText(input.EntryText)
	if text == "" {
		return &BuildNavigationTextOutput{}
	}

	log.Debug().
		Str("entry_text", input.EntryText).
		Str("navigation_text", text).
		Msg("built bang navigation text")

	return &BuildNavigationTextOutput{
		Text:  text,
		Valid: true,
	}
}

// GetShortcut returns a shortcut by key (case-insensitive).
func (uc *SearchShortcutsUseCase) GetShortcut(key string) (SearchShortcut, bool) {
	normalizedKey, ok := uc.normalizeBangKey(key)
	if !ok {
		return SearchShortcut{}, false
	}
	shortcut, ok := uc.shortcuts[normalizedKey]
	return shortcut, ok
}

// ShortcutURLs returns a map of shortcut keys to URL templates.
// This is useful for passing to url.BuildSearchURL.
func (uc *SearchShortcutsUseCase) ShortcutURLs() map[string]string {
	result := make(map[string]string, len(uc.shortcuts))
	for key, shortcut := range uc.shortcuts {
		result[key] = shortcut.URL
	}
	return result
}

// buildBangSuggestions filters and sorts shortcuts matching the query prefix.
func (uc *SearchShortcutsUseCase) buildBangSuggestions(query string) []BangSuggestion {
	prefix := strings.TrimPrefix(query, "!")
	if idx := strings.Index(prefix, " "); idx >= 0 {
		prefix = prefix[:idx]
	}
	prefix = strings.ToLower(prefix)

	suggestions := make([]BangSuggestion, 0, len(uc.shortcuts))
	for key, shortcut := range uc.shortcuts {
		if !strings.HasPrefix(strings.ToLower(key), prefix) {
			continue
		}
		description := shortcut.Description
		if description == "" {
			description = shortcut.URL
		}
		suggestions = append(suggestions, BangSuggestion{
			Key:         key,
			Description: description,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return strings.ToLower(suggestions[i].Key) < strings.ToLower(suggestions[j].Key)
	})

	return suggestions
}

// detectBangKey returns the matched shortcut key if the query has a valid bang prefix.
func (uc *SearchShortcutsUseCase) detectBangKey(query string) string {
	spaceIdx := strings.Index(query, " ")
	if !strings.HasPrefix(query, "!") || spaceIdx <= 1 {
		return ""
	}

	candidate := query[1:spaceIdx]
	for key := range uc.shortcuts {
		if strings.EqualFold(key, candidate) {
			return key
		}
	}

	return ""
}

// normalizeBangKey returns the canonical key for a shortcut (case-insensitive lookup).
func (uc *SearchShortcutsUseCase) normalizeBangKey(shortcutKey string) (string, bool) {
	for key := range uc.shortcuts {
		if strings.EqualFold(key, shortcutKey) {
			return key, true
		}
	}
	return "", false
}

// buildBangNavigationText normalizes a bang query for navigation.
func (uc *SearchShortcutsUseCase) buildBangNavigationText(entryText string) string {
	shortcutKey, query, found := url.ParseBangShortcut(entryText)
	if !found || query == "" {
		return ""
	}

	resolvedKey, ok := uc.normalizeBangKey(shortcutKey)
	if !ok {
		return ""
	}

	return "!" + resolvedKey + " " + query
}
