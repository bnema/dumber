package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	historydomain "github.com/bnema/dumber/internal/domain/history"
	"github.com/bnema/dumber/internal/domain/repository"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time check: SearchHistoryUseCase must satisfy port.HomepageHistory.
var _ port.HomepageHistory = (*SearchHistoryUseCase)(nil)

const (
	historyWindowDuration = 24 * time.Hour

	defaultHistoryPageLimit   = 50
	maxHistoryPageLimit       = 500
	defaultHistorySearchLimit = 20
	maxHistorySearchLimit     = 100
	defaultDomainStatsLimit   = 20
	maxDomainStatsLimit       = 100
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

// SearchInput is an alias for dto.HistorySearchInput.
type SearchInput = dto.HistorySearchInput

// SearchOutput is an alias for dto.HistorySearchOutput.
type SearchOutput = dto.HistorySearchOutput

// Search performs a full-text search on history entries using SQLite FTS5.
// Returns only entries that actually match the query terms.
func (uc *SearchHistoryUseCase) Search(ctx context.Context, input SearchInput) (*SearchOutput, error) {
	log := logging.FromContext(ctx)

	if input.Query == "" {
		return &SearchOutput{Matches: []entity.HistoryMatch{}}, nil
	}

	limit := clampPositiveLimit(input.Limit, defaultHistorySearchLimit, maxHistorySearchLimit)

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

// GetRecent retrieves recent history entries. A zero limit means all entries;
// negative limits retain the historical default page size. Positive limits are
// capped to keep WebUI-originated queries bounded.
func (uc *SearchHistoryUseCase) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	limit = clampOptionalLimit(limit, defaultHistoryPageLimit, maxHistoryPageLimit)

	entries, err := uc.historyRepo.GetRecent(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	return entries, nil
}

// GetRecentByDomain retrieves recent history entries for a canonical domain. A
// zero limit means all matching entries; negative limits use the default page
// size. Positive limits are capped to keep WebUI-originated queries bounded.
func (uc *SearchHistoryUseCase) GetRecentByDomain(ctx context.Context, domain string, limit, offset int) ([]*entity.HistoryEntry, error) {
	limit = clampOptionalLimit(limit, defaultHistoryPageLimit, maxHistoryPageLimit)
	domain, err := canonicalHistoryDomain(domain)
	if err != nil {
		return nil, err
	}

	entries, err := uc.historyRepo.GetRecentByDomain(ctx, domain, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent history by domain: %w", err)
	}
	return entries, nil
}

// canonicalHistoryDomain normalizes a domain and rejects empty values before
// the domain reaches exact-match persistence queries.
func canonicalHistoryDomain(domain string) (string, error) {
	domain = domainurl.CanonicalDomain(domain)
	if domain == "" {
		return "", fmt.Errorf("domain is required")
	}
	return domain, nil
}

// GetRecentWindow retrieves the 24-hour history window ending before the given cursor.
func (uc *SearchHistoryUseCase) GetRecentWindow(ctx context.Context, before time.Time, domain string) (*entity.HistoryWindow, error) {
	if before.IsZero() {
		before = time.Now()
	}
	before = before.UTC()
	after := before.Add(-historyWindowDuration)

	var (
		entries []*entity.HistoryEntry
		hasMore bool
		err     error
	)
	originalDomain := domain
	domain = domainurl.CanonicalDomain(domain)
	if strings.TrimSpace(originalDomain) != "" && domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if domain != "" {
		entries, err = uc.historyRepo.GetRecentWindowByDomain(ctx, domain, before, after)
		if err == nil {
			hasMore, err = uc.historyRepo.HasEntriesByDomainBefore(ctx, domain, after)
		}
	} else {
		entries, err = uc.historyRepo.GetRecentWindow(ctx, before, after)
		if err == nil {
			hasMore, err = uc.historyRepo.HasEntriesBefore(ctx, after)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get recent history window: %w", err)
	}

	return &entity.HistoryWindow{
		Entries: entries,
		Before:  before,
		After:   after,
		HasMore: hasMore,
	}, nil
}

// GetRecentSince retrieves history entries visited within the last N days.
// If days is 0, returns all history entries.
// If days is negative, defaults to 30 days.
func (uc *SearchHistoryUseCase) GetRecentSince(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if days < 0 {
		days = 30 // Default to 30 days for negative values
	}

	var entries []*entity.HistoryEntry
	var err error

	switch days {
	case 0:
		entries, err = uc.historyRepo.GetAllRecentHistory(ctx)
	default:
		entries, err = uc.historyRepo.GetRecentSince(ctx, days)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get recent history: %w", err)
	}

	return entries, nil
}

// GetMostVisited retrieves history entries sorted by visit count within the last N days.
// If days is 0, returns all history entries sorted by visit count.
// If days is negative, defaults to 30 days.
func (uc *SearchHistoryUseCase) GetMostVisited(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if days < 0 {
		days = 30 // Default to 30 days for negative values
	}

	var entries []*entity.HistoryEntry
	var err error

	switch days {
	case 0:
		entries, err = uc.historyRepo.GetAllMostVisited(ctx)
	default:
		entries, err = uc.historyRepo.GetMostVisited(ctx, days)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get most visited history: %w", err)
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

// ClearRange deletes history entries inside a named recent range.
func (uc *SearchHistoryUseCase) ClearRange(ctx context.Context, rangeID string) error {
	cutoff, all, ok := historydomain.DeleteRangeCutoff(rangeID, time.Now())
	if !ok {
		return fmt.Errorf("unknown history delete range: %q", rangeID)
	}
	if all {
		return uc.ClearAll(ctx)
	}

	log := logging.FromContext(ctx)
	log.Debug().Time("since", cutoff).Str("range", rangeID).Msg("clearing recent history range")
	if err := uc.historyRepo.DeleteSince(ctx, cutoff); err != nil {
		return fmt.Errorf("failed to clear history range: %w", err)
	}
	log.Info().Time("since", cutoff).Str("range", rangeID).Msg("recent history range cleared")
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
	domain, err := canonicalHistoryDomain(domain)
	if err != nil {
		return err
	}

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

	limit = clampPositiveLimit(limit, defaultDomainStatsLimit, maxDomainStatsLimit)

	return uc.historyRepo.GetDomainStats(ctx, limit)
}

func clampOptionalLimit(limit, defaultLimit, maxLimit int) int {
	if limit < 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func clampPositiveLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// GetStats retrieves lightweight aggregate history statistics.
func (uc *SearchHistoryUseCase) GetStats(ctx context.Context) (*entity.HistoryStats, error) {
	stats, err := uc.historyRepo.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get history stats: %w", err)
	}
	return stats, nil
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
