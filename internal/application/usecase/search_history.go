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

// Search performs a full-text search on history entries using SQLite FTS5.
// Returns only entries that actually match the query terms.
func (uc *SearchHistoryUseCase) Search(ctx context.Context, input SearchInput) (*SearchOutput, error) {
	log := logging.FromContext(ctx)

	if input.Query == "" {
		return &SearchOutput{Matches: []entity.HistoryMatch{}}, nil
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 20 // Default limit
	}

	// Use repository's FTS5 search
	matches, err := uc.historyRepo.Search(ctx, input.Query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search history: %w", err)
	}

	log.Debug().
		Str("query", input.Query).
		Int("matches", len(matches)).
		Msg("FTS5 search completed")

	return &SearchOutput{Matches: matches}, nil
}

// GetRecent retrieves recent history entries with pagination.
func (uc *SearchHistoryUseCase) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}

	entries, err := uc.historyRepo.GetRecent(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

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

// DeleteByDomain removes all history entries for a domain.
func (uc *SearchHistoryUseCase) DeleteByDomain(ctx context.Context, domain string) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("domain", domain).Msg("deleting history by domain")

	if err := uc.historyRepo.DeleteByDomain(ctx, domain); err != nil {
		return fmt.Errorf("failed to delete history by domain: %w", err)
	}

	log.Info().Str("domain", domain).Msg("history deleted for domain")
	return nil
}

// GetDomainStats retrieves per-domain visit statistics.
func (uc *SearchHistoryUseCase) GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error) {
	log := logging.FromContext(ctx)
	log.Debug().Int("limit", limit).Msg("getting domain stats")

	if limit <= 0 {
		limit = 20
	}

	return uc.historyRepo.GetDomainStats(ctx, limit)
}

// GetAnalytics retrieves aggregated history analytics for the homepage.
func (uc *SearchHistoryUseCase) GetAnalytics(ctx context.Context) (*entity.HistoryAnalytics, error) {
	log := logging.FromContext(ctx)
	log.Debug().Msg("getting history analytics")

	// Get basic stats
	stats, err := uc.historyRepo.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get history stats: %w", err)
	}

	// Get top domains
	domains, err := uc.historyRepo.GetDomainStats(ctx, 10)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain stats: %w", err)
	}

	// Get daily visits for last 30 days
	dailyVisits, err := uc.historyRepo.GetDailyVisitCount(ctx, "-30 days")
	if err != nil {
		return nil, fmt.Errorf("failed to get daily visits: %w", err)
	}

	// Get hourly distribution
	hourlyDist, err := uc.historyRepo.GetHourlyDistribution(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get hourly distribution: %w", err)
	}

	return &entity.HistoryAnalytics{
		TotalEntries:       stats.TotalEntries,
		TotalVisits:        stats.TotalVisits,
		UniqueDays:         stats.UniqueDays,
		TopDomains:         domains,
		DailyVisits:        dailyVisits,
		HourlyDistribution: hourlyDist,
	}, nil
}
