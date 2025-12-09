package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// SearchHistoryUseCase handles history search and retrieval operations.
type SearchHistoryUseCase struct {
	historyRepo repository.HistoryRepository
}

// NewSearchHistoryUseCase creates a new history search use case.
func NewSearchHistoryUseCase(historyRepo repository.HistoryRepository) *SearchHistoryUseCase {
	return &SearchHistoryUseCase{
		historyRepo: historyRepo,
	}
}

// SearchInput contains search parameters.
type SearchInput struct {
	Query string
	Limit int
}

// SearchOutput contains search results.
type SearchOutput struct {
	Matches []entity.HistoryMatch
}

// Search performs a fuzzy search on history entries.
func (uc *SearchHistoryUseCase) Search(ctx context.Context, input SearchInput) (*SearchOutput, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("query", input.Query).
		Int("limit", input.Limit).
		Msg("searching history")

	if input.Query == "" {
		return &SearchOutput{Matches: []entity.HistoryMatch{}}, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20 // Default limit
	}

	matches, err := uc.historyRepo.Search(ctx, input.Query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search history: %w", err)
	}

	log.Debug().
		Str("query", input.Query).
		Int("results", len(matches)).
		Msg("history search completed")

	return &SearchOutput{Matches: matches}, nil
}

// GetRecent retrieves recent history entries with pagination.
func (uc *SearchHistoryUseCase) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Int("limit", limit).
		Int("offset", offset).
		Msg("getting recent history")

	if limit <= 0 {
		limit = 50 // Default limit
	}

	entries, err := uc.historyRepo.GetRecent(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	log.Debug().Int("count", len(entries)).Msg("retrieved recent history")
	return entries, nil
}

// FindByURL retrieves a history entry by its URL.
func (uc *SearchHistoryUseCase) FindByURL(ctx context.Context, url string) (*entity.HistoryEntry, error) {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", url).Msg("finding history entry by URL")

	entry, err := uc.historyRepo.FindByURL(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to find history entry: %w", err)
	}

	if entry == nil {
		log.Debug().Str("url", url).Msg("history entry not found")
	}

	return entry, nil
}

// ClearOlderThan deletes history entries older than the specified time.
func (uc *SearchHistoryUseCase) ClearOlderThan(ctx context.Context, before time.Time) error {
	log := logging.FromContext(ctx)
	log.Debug().Time("before", before).Msg("clearing old history")

	if err := uc.historyRepo.DeleteOlderThan(ctx, before); err != nil {
		return fmt.Errorf("failed to clear history: %w", err)
	}

	log.Info().Time("before", before).Msg("old history cleared")
	return nil
}

// ClearAll deletes all history entries.
func (uc *SearchHistoryUseCase) ClearAll(ctx context.Context) error {
	log := logging.FromContext(ctx)
	log.Debug().Msg("clearing all history")

	if err := uc.historyRepo.DeleteAll(ctx); err != nil {
		return fmt.Errorf("failed to clear all history: %w", err)
	}

	log.Info().Msg("all history cleared")
	return nil
}

// Delete removes a single history entry by ID.
func (uc *SearchHistoryUseCase) Delete(ctx context.Context, id int64) error {
	log := logging.FromContext(ctx)
	log.Debug().Int64("id", id).Msg("deleting history entry")

	if err := uc.historyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete history entry: %w", err)
	}

	log.Debug().Int64("id", id).Msg("history entry deleted")
	return nil
}
