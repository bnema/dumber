package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/domain/autocomplete"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// AutocompleteUseCase handles inline autocomplete suggestions.
type AutocompleteUseCase struct {
	historyUC   *SearchHistoryUseCase
	favoritesUC *ManageFavoritesUseCase
	shortcutsUC *SearchShortcutsUseCase

	cacheMu         sync.Mutex
	favoritesCache  []*entity.Favorite
	favoritesCached time.Time
	historyCache    []*entity.HistoryEntry
	historyCached   time.Time
}

const autocompleteHistoryLimit = 200
const autocompleteCacheTTL = 2 * time.Second

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

// CompletionOptions controls how URL completions are resolved.
type CompletionOptions struct {
	VisibleURLs []string
	AllowBangs  bool
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

// ResolveCompletion resolves the best completion using visible list order first,
// then cached favorites/history as fallback.
func (uc *AutocompleteUseCase) ResolveCompletion(ctx context.Context, input string, opts CompletionOptions) *autocomplete.Suggestion {
	log := logging.FromContext(ctx)

	if input == "" {
		return nil
	}

	if len(opts.VisibleURLs) > 0 {
		if suggestion := findCompletionInURLs(ctx, input, opts.VisibleURLs, autocomplete.SourceHistory); suggestion != nil {
			log.Debug().
				Str("input", input).
				Str("match", suggestion.FullText).
				Msg("autocomplete: matched visible list")
			return suggestion
		}
	}

	if suggestion := uc.findFavoriteSuggestion(ctx, input); suggestion != nil {
		log.Debug().
			Str("input", input).
			Str("match", suggestion.FullText).
			Msg("autocomplete: matched favorite fallback")
		return suggestion
	}

	if suggestion := uc.findHistorySuggestion(ctx, input); suggestion != nil {
		log.Debug().
			Str("input", input).
			Str("match", suggestion.FullText).
			Msg("autocomplete: matched history fallback")
		return suggestion
	}

	if opts.AllowBangs {
		if suggestion := uc.findBangSuggestion(ctx, input); suggestion != nil {
			log.Debug().
				Str("input", input).
				Str("match", suggestion.FullText).
				Msg("autocomplete: matched bang fallback")
			return suggestion
		}
	}

	return nil
}

// findFavoriteSuggestion finds the best matching favorite for the input.
func (uc *AutocompleteUseCase) findFavoriteSuggestion(ctx context.Context, input string) *autocomplete.Suggestion {
	log := logging.FromContext(ctx)

	if uc.favoritesUC == nil {
		return nil
	}

	favorites, err := uc.getCachedFavorites(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("autocomplete: favorites cache fetch failed")
		return nil
	}

	for _, fav := range favorites {
		if fav == nil || fav.URL == "" {
			continue
		}
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
	log := logging.FromContext(ctx)

	if uc.historyUC == nil {
		return nil
	}

	entries, err := uc.getCachedHistory(ctx)
	if err != nil {
		log.Debug().Err(err).Msg("autocomplete: history cache fetch failed")
		return nil
	}

	for _, entry := range entries {
		if entry == nil || entry.URL == "" {
			continue
		}
		suffix, matchedURL, ok := autocomplete.ComputeURLCompletionSuffix(input, entry.URL)
		if ok {
			return &autocomplete.Suggestion{
				FullText: matchedURL,
				Suffix:   suffix,
				Source:   autocomplete.SourceHistory,
				Title:    entry.Title,
			}
		}
	}

	return nil
}

// findBangSuggestion finds the best matching bang shortcut for the input.
func (uc *AutocompleteUseCase) findBangSuggestion(ctx context.Context, input string) *autocomplete.Suggestion {
	_ = logging.FromContext(ctx)

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

func findCompletionInURLs(_ context.Context, input string, urls []string, source autocomplete.SuggestionSource) *autocomplete.Suggestion {
	for _, u := range urls {
		if u == "" {
			continue
		}
		suffix, matchedURL, ok := autocomplete.ComputeURLCompletionSuffix(input, u)
		if ok {
			return &autocomplete.Suggestion{
				FullText: matchedURL,
				Suffix:   suffix,
				Source:   source,
			}
		}
	}

	return nil
}

func (uc *AutocompleteUseCase) getCachedFavorites(ctx context.Context) ([]*entity.Favorite, error) {
	log := logging.FromContext(ctx)

	uc.cacheMu.Lock()
	if time.Since(uc.favoritesCached) < autocompleteCacheTTL && uc.favoritesCache != nil {
		favorites := uc.favoritesCache
		uc.cacheMu.Unlock()
		return favorites, nil
	}
	uc.cacheMu.Unlock()

	favorites, err := uc.favoritesUC.GetAll(ctx)
	if err != nil {
		return nil, err
	}

	uc.cacheMu.Lock()
	uc.favoritesCache = favorites
	uc.favoritesCached = time.Now()
	uc.cacheMu.Unlock()

	log.Debug().Int("count", len(favorites)).Msg("autocomplete: favorites cache refreshed")
	return favorites, nil
}

func (uc *AutocompleteUseCase) getCachedHistory(ctx context.Context) ([]*entity.HistoryEntry, error) {
	log := logging.FromContext(ctx)

	uc.cacheMu.Lock()
	if time.Since(uc.historyCached) < autocompleteCacheTTL && uc.historyCache != nil {
		history := uc.historyCache
		uc.cacheMu.Unlock()
		return history, nil
	}
	uc.cacheMu.Unlock()

	entries, err := uc.historyUC.GetRecent(ctx, autocompleteHistoryLimit, 0)
	if err != nil {
		return nil, err
	}

	uc.cacheMu.Lock()
	uc.historyCache = entries
	uc.historyCached = time.Now()
	uc.cacheMu.Unlock()

	log.Debug().Int("count", len(entries)).Msg("autocomplete: history cache refreshed")
	return entries, nil
}

// GetSuggestionForURL returns an autocomplete suggestion specifically for a URL.
// This is used when the user selects a row from the list to update ghost text.
func (*AutocompleteUseCase) GetSuggestionForURL(_ context.Context, input, targetURL string) *GetSuggestionOutput {
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
