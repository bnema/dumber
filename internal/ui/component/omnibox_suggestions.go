package component

import (
	stdurl "net/url"
	"strings"

	"github.com/bnema/dumber/internal/domain/autocomplete"
	"github.com/bnema/dumber/internal/domain/url"
)

// Omnibox suggestion logic: pure functions over the entry text and the
// visible result list. The GTK widget in omnibox.go only feeds state in and
// renders the answer, so this file is the test surface for ghost completion,
// selection targets, and search-query resolution.

// ghostCompletionRequest is everything ghost completion depends on.
type ghostCompletionRequest struct {
	EntryText            string
	SelectedURL          string
	HasExplicitSelection bool
	Mode                 ViewMode
	MaxVisible           int
	Suggestions          []Suggestion
	Favorites            []Favorite
}

// ghostCompletion returns the inline completion suffix to show after the
// entry text, and the full text it completes to. ok is false when nothing
// should be shown. Host-like input completes to a domain rather than a full
// redirect URL.
func ghostCompletion(req ghostCompletionRequest) (fullText, suffix string, ok bool) {
	_, input, ok := ghostCompletionInput(req.EntryText)
	if !ok {
		return "", "", false
	}
	fullText, suffix, ok = visibleGhostSuggestion(
		input, req.SelectedURL, req.HasExplicitSelection, req.Mode, req.MaxVisible, req.Suggestions, req.Favorites,
	)
	if !ok {
		return "", "", false
	}
	fullText, suffix = normalizeGhostSuggestion(input, fullText, suffix)
	if fullText == "" || suffix == "" {
		return "", "", false
	}
	return fullText, suffix, true
}

// isGhostEcho reports whether the entry text is the debounced echo of Dumber's
// own ghost completion: realInput plus the suffix currently displayed. A
// matching echo must leave the ghost state untouched, but it cannot be told
// apart from a paste that replaced the selected suffix with the same text, so a
// deferred Enter still wins and the ghost state is rebuilt from the paste.
func isGhostEcho(entryText, realInput, ghostSuffix string) bool {
	return ghostSuffix != "" && entryText == realInput+ghostSuffix
}

// reconcileEntryState decides how to resync the entry buffer with the
// realInput/ghostSuffix shadow state. The buffer is the source of truth:
// search-changed is debounced, so realInput lags behind fresh keystrokes.
// The buffer is only ever rewritten to strip a still-intact ghost suffix —
// never to restore older text, which would delete characters the user just
// typed.
func reconcileEntryState(buffer, realInput, ghostSuffix string) (newRealInput string, rewrite bool) {
	if ghostSuffix != "" && buffer == realInput+ghostSuffix {
		return realInput, true
	}
	return buffer, false
}

func ghostCompletionInput(entryText string) (leadingWhitespace, completionInput string, ok bool) {
	trimmed := strings.TrimLeft(entryText, " \t")
	leadingWhitespace = entryText[:len(entryText)-len(trimmed)]
	if trimmed == "" {
		return leadingWhitespace, "", false
	}
	return leadingWhitespace, trimmed, true
}

// normalizeGhostSuggestion trims noisy URL completions to a domain completion when input
// appears to be host-like (e.g. "google" -> "google.com" instead of full redirect URL).
func normalizeGhostSuggestion(queryText, fullText, fallbackSuffix string) (normalizedFullText, suffix string) {
	if queryText == "" || fullText == "" {
		return "", ""
	}

	// If input already contains path/query-ish delimiters, keep original completion behavior.
	if strings.ContainsAny(queryText, "/?#=& ") {
		return fullText, fallbackSuffix
	}

	hostOnly := extractHostForCompletion(fullText)
	if hostOnly != "" {
		for _, candidate := range []string{hostOnly, strings.TrimPrefix(hostOnly, "www.")} {
			if completionSuffix, ok := autocomplete.ComputeCompletionSuffix(queryText, candidate); ok {
				return candidate, completionSuffix
			}
		}
	}

	return fullText, fallbackSuffix
}

func extractHostForCompletion(raw string) string {
	if raw == "" {
		return ""
	}
	if parsed, err := stdurl.Parse(raw); err == nil && parsed.Host != "" {
		return strings.ToLower(parsed.Hostname())
	}
	trimmed := autocomplete.StripProtocol(raw)
	trimmed = strings.ToLower(trimmed)
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(trimmed, sep); idx >= 0 {
			trimmed = trimmed[:idx]
		}
	}
	return strings.TrimSpace(trimmed)
}

func visibleGhostSuggestion(
	query, selectedURL string,
	hasExplicitSelection bool,
	mode ViewMode,
	maxVisible int,
	suggestions []Suggestion,
	favorites []Favorite,
) (fullText, suffix string, ok bool) {
	if hasExplicitSelection {
		suffix, fullText, ok = autocomplete.ComputeURLCompletionSuffix(query, selectedURL)
		return fullText, suffix, ok
	}

	visibleURLs := visibleURLsForMode(mode, maxVisible, suggestions, favorites)
	suffix, fullText, ok = autocomplete.BestURLCompletion(query, visibleURLs)
	return fullText, suffix, ok
}

func visibleURLsForMode(mode ViewMode, maxVisible int, suggestions []Suggestion, favorites []Favorite) []string {
	if mode == ViewModeHistory {
		visibleCount := visibleResultCount(len(suggestions), maxVisible)
		urls := make([]string, 0, visibleCount)
		for _, s := range suggestions[:visibleCount] {
			if s.URL != "" {
				urls = append(urls, s.URL)
			}
		}
		return urls
	}

	visibleCount := visibleResultCount(len(favorites), maxVisible)
	urls := make([]string, 0, visibleCount)
	for _, f := range favorites[:visibleCount] {
		if f.URL != "" {
			urls = append(urls, f.URL)
		}
	}
	return urls
}

func selectedTargetURL(mode ViewMode, idx, maxVisible int, suggestions []Suggestion, favorites []Favorite) (string, bool) {
	if idx < 0 {
		return "", false
	}
	return resolveTargetURLForSelection(mode, idx, maxVisible, suggestions, favorites), true
}

func bangSuggestionTextAt(idx int, bangSuggestions []BangSuggestion) (string, bool) {
	if idx < 0 || idx >= len(bangSuggestions) {
		return "", false
	}
	return "!" + bangSuggestions[idx].Key + " ", true
}

func visibleResultCount(total, maxVisible int) int {
	if total <= 0 {
		return 0
	}
	if maxVisible <= 0 || total < maxVisible {
		return total
	}
	return maxVisible
}

func effectiveSearchQuery(entryText, realInput string, hasGhost bool) string {
	if hasGhost && realInput != "" {
		return realInput
	}
	return entryText
}

func resolveTargetURLForSelection(mode ViewMode, idx, maxVisible int, suggestions []Suggestion, favorites []Favorite) string {
	if mode == ViewModeHistory {
		visibleCount := visibleResultCount(len(suggestions), maxVisible)
		if idx >= 0 && idx < visibleCount {
			return suggestions[idx].URL
		}
		return ""
	}
	visibleCount := visibleResultCount(len(favorites), maxVisible)
	if idx >= 0 && idx < visibleCount {
		return favorites[idx].URL
	}
	return ""
}

func shouldPreferTypedURLNavigation(entryText string) bool {
	entryText = strings.TrimSpace(entryText)
	if entryText == "" {
		return false
	}
	return url.LooksLikeURL(entryText)
}
