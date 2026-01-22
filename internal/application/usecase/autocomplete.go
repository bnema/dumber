package usecase

import (
	"context"

	"github.com/bnema/dumber/internal/domain/autocomplete"
	"github.com/bnema/dumber/internal/logging"
)

// AutocompleteUseCase handles inline autocomplete suggestions.
type AutocompleteUseCase struct {
	historyUC   *SearchHistoryUseCase
	favoritesUC *ManageFavoritesUseCase
	shortcutsUC *SearchShortcutsUseCase
}

// NewAutocompleteUseCase creates a new autocomplete use case.
func NewAutocompleteUseCase(
	historyUC *SearchHistoryUseCase,
	favoritesUC *ManageFavoritesUseCase,
	shortcutsUC *SearchShortcutsUseCase,
) *AutocompleteUseCase {
	return &AutocompleteUseCase{
		historyUC:   historyUC,
		favoritesUC: favoritesUC,
		shortcutsUC: shortcutsUC,
	}
}

// GetSuggestionInput contains parameters for getting an autocomplete suggestion.
type GetSuggestionInput struct {
	Input string // Current user input
}

// GetSuggestionOutput contains the best autocomplete suggestion.
type GetSuggestionOutput struct {
	Suggestion *autocomplete.Suggestion
	Found      bool
}

// GetSuggestion returns the best autocomplete suggestion for the given input.
// Priority: Favorites > History (by frecency) > Bang shortcuts.
func (uc *AutocompleteUseCase) GetSuggestion(ctx context.Context, input GetSuggestionInput) *GetSuggestionOutput {
	log := logging.FromContext(ctx)

	if input.Input == "" {
		return &GetSuggestionOutput{Found: false}
	}

	// Try favorites first (highest priority)
	if suggestion := uc.findFavoriteSuggestion(ctx, input.Input); suggestion != nil {
		log.Debug().
			Str("input", input.Input).
			Str("match", suggestion.FullText).
			Msg("autocomplete: matched favorite")
		return &GetSuggestionOutput{Suggestion: suggestion, Found: true}
	}

	// Try history (second priority)
	if suggestion := uc.findHistorySuggestion(ctx, input.Input); suggestion != nil {
		log.Debug().
			Str("input", input.Input).
			Str("match", suggestion.FullText).
			Msg("autocomplete: matched history")
		return &GetSuggestionOutput{Suggestion: suggestion, Found: true}
	}

	// Try bang shortcuts (lowest priority for URL-style completion)
	if suggestion := uc.findBangSuggestion(ctx, input.Input); suggestion != nil {
		log.Debug().
			Str("input", input.Input).
			Str("match", suggestion.FullText).
			Msg("autocomplete: matched bang shortcut")
		return &GetSuggestionOutput{Suggestion: suggestion, Found: true}
	}

	return &GetSuggestionOutput{Found: false}
}

// findFavoriteSuggestion finds the best matching favorite for the input.
func (uc *AutocompleteUseCase) findFavoriteSuggestion(ctx context.Context, input string) *autocomplete.Suggestion {
	if uc.favoritesUC == nil {
		return nil
	}

	favorites, err := uc.favoritesUC.GetAll(ctx)
	if err != nil {
		return nil
	}

	for _, fav := range favorites {
		suffix, matchedURL, ok := autocomplete.ComputeURLCompletionSuffix(input, fav.URL)
		if ok {
			return &autocomplete.Suggestion{
				FullText: matchedURL,
				Suffix:   suffix,
				Source:   autocomplete.SourceFavorite,
				Title:    fav.Title,
			}
		}
	}

	return nil
}

// findHistorySuggestion finds the best matching history entry for the input.
func (uc *AutocompleteUseCase) findHistorySuggestion(ctx context.Context, input string) *autocomplete.Suggestion {
	if uc.historyUC == nil {
		return nil
	}

	// Search history for matches
	output, err := uc.historyUC.Search(ctx, SearchInput{
		Query: input,
		Limit: 10, // Get top 10 to find best prefix match
	})
	if err != nil || output == nil {
		return nil
	}

	// Find the first result that is a prefix match
	for _, match := range output.Matches {
		suffix, matchedURL, ok := autocomplete.ComputeURLCompletionSuffix(input, match.Entry.URL)
		if ok {
			return &autocomplete.Suggestion{
				FullText: matchedURL,
				Suffix:   suffix,
				Source:   autocomplete.SourceHistory,
				Title:    match.Entry.Title,
			}
		}
	}

	return nil
}

// findBangSuggestion finds the best matching bang shortcut for the input.
func (uc *AutocompleteUseCase) findBangSuggestion(ctx context.Context, input string) *autocomplete.Suggestion {
	if uc.shortcutsUC == nil || len(input) < 1 || input[0] != '!' {
		return nil
	}

	// Get filtered bang suggestions
	output := uc.shortcutsUC.FilterBangs(ctx, FilterBangsInput{Query: input})
	if len(output.Suggestions) == 0 {
		return nil
	}

	// Use the first (best) match
	bang := output.Suggestions[0]
	fullBang := "!" + bang.Key

	suffix, ok := autocomplete.ComputeCompletionSuffix(input, fullBang)
	if ok {
		return &autocomplete.Suggestion{
			FullText: fullBang,
			Suffix:   suffix,
			Source:   autocomplete.SourceBangShortcut,
			Title:    bang.Description,
		}
	}

	return nil
}

// GetSuggestionForURL returns an autocomplete suggestion specifically for a URL.
// This is used when the user selects a row from the list to update ghost text.
func (uc *AutocompleteUseCase) GetSuggestionForURL(ctx context.Context, input, targetURL string) *GetSuggestionOutput {
	if input == "" || targetURL == "" {
		return &GetSuggestionOutput{Found: false}
	}

	suffix, matchedURL, ok := autocomplete.ComputeURLCompletionSuffix(input, targetURL)
	if !ok {
		return &GetSuggestionOutput{Found: false}
	}

	return &GetSuggestionOutput{
		Suggestion: &autocomplete.Suggestion{
			FullText: matchedURL,
			Suffix:   suffix,
			Source:   autocomplete.SourceHistory, // Default to history for selected rows
		},
		Found: true,
	}
}
