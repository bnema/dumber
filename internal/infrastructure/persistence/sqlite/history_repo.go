package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	domainurl "github.com/bnema/dumber/internal/domain/url"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
	"github.com/bnema/dumber/internal/logging"
)

const logURLMaxLen = 60

const (
	historyURLMatchScore        = 300.0
	historyTitleMatchScore      = 120.0
	historyExactHostScore       = 220.0
	historyTrimmedHostScore     = 215.0
	historyHostPrefixScore      = 180.0
	historyURLPrefixScore       = 170.0
	historyURLContainsScore     = 80.0
	historyTitlePrefixScore     = 90.0
	historyTitleContainsScore   = 40.0
	historyWordHostPrefixScore  = 40.0
	historyWordURLContainsScore = 15.0
	historyWordTitlePrefixScore = 20.0
	historyWordTitleScore       = 8.0
	historyPathDepthPenalty     = 18
	historyRootPathBonus        = 20.0
	historyDomainBoostScore     = 35.0
	historyVisitCountCap        = 50.0
	historyRecentVisitScore     = 10.0
)

var expectedFTSFailureCount atomic.Uint64

type historyRepo struct {
	db      *sql.DB
	queries *sqlc.Queries
}

// NewHistoryRepository creates a new SQLite-backed history repository.
func NewHistoryRepository(db *sql.DB) repository.HistoryRepository {
	return &historyRepo{db: db, queries: sqlc.New(db)}
}

// aboutBlankURL is the special URL for blank pages that should not accumulate visit counts.
const aboutBlankURL = "about:blank"

func (r *historyRepo) Save(ctx context.Context, entry *entity.HistoryEntry) error {
	log := logging.FromContext(ctx)
	log.Debug().Str("url", logging.TruncateURL(entry.URL, logURLMaxLen)).Msg("saving history entry")

	err := r.queries.UpsertHistory(ctx, sqlc.UpsertHistoryParams{
		Url:        entry.URL,
		Title:      sql.NullString{String: entry.Title, Valid: entry.Title != ""},
		FaviconUrl: sql.NullString{String: entry.FaviconURL, Valid: entry.FaviconURL != ""},
	})
	if err != nil {
		return err
	}

	// Cap about:blank visit count to 1 so it exists but never dominates suggestions
	if entry.URL == aboutBlankURL {
		capErr := r.queries.CapVisitCount(ctx, sqlc.CapVisitCountParams{
			VisitCount:   sql.NullInt64{Int64: 1, Valid: true},
			Url:          aboutBlankURL,
			VisitCount_2: sql.NullInt64{Int64: 1, Valid: true},
		})
		if capErr != nil {
			log.Debug().Err(capErr).Msg("failed to cap about:blank visit count")
		}
	}

	return nil
}

func (r *historyRepo) FindByURL(ctx context.Context, rawURL string) (*entity.HistoryEntry, error) {
	row, err := r.queries.GetHistoryByURL(ctx, rawURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return historyFromRow(row), nil
}

func (r *historyRepo) UpdateMetadata(ctx context.Context, entry *entity.HistoryEntry) error {
	if entry == nil || entry.URL == "" {
		return fmt.Errorf("history entry URL cannot be empty")
	}
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE history
		 SET title = COALESCE(?, title),
		     favicon_url = COALESCE(?, favicon_url)
		 WHERE url = ?`,
		nullString(entry.Title),
		nullString(entry.FaviconURL),
		entry.URL,
	)
	return err
}

func (r *historyRepo) Search(ctx context.Context, query string, limit int) ([]entity.HistoryMatch, error) {
	if limit <= 0 {
		return []entity.HistoryMatch{}, nil
	}

	words := sanitizeFTS5QueryTokens(query)
	if len(words) == 0 {
		return []entity.HistoryMatch{}, nil
	}

	// Build FTS5 query: "word1* word2*" (implicit AND between terms)
	parts := make([]string, len(words))
	for i, word := range words {
		parts[i] = word + "*"
	}
	ftsQuery := strings.Join(parts, " ")

	// Search both URL and title columns, then merge results
	// Use domain-boosted query for URL search to prioritize domain matches
	// Use sanitized first word for domain boost (intentional: domain boost targets first search term)
	urlRows, urlErr := r.queries.SearchHistoryFTSUrlWithDomainBoost(ctx, sqlc.SearchHistoryFTSUrlWithDomainBoostParams{
		Term:  sql.NullString{String: words[0], Valid: true},
		Query: ftsQuery,
		Limit: int64(limit),
	})
	if urlErr != nil {
		logFTSSearchError(ctx, "url", query, urlErr)
		urlRows = []sqlc.SearchHistoryFTSUrlWithDomainBoostRow{}
	}

	titleRows, titleErr := r.queries.SearchHistoryFTSTitle(ctx, sqlc.SearchHistoryFTSTitleParams{
		Query: ftsQuery,
		Limit: int64(limit),
	})
	if titleErr != nil {
		logFTSSearchError(ctx, "title", query, titleErr)
		titleRows = []sqlc.History{}
	}

	ranked := rankHistoryMatches(query, words, urlRows, titleRows)
	if limit < len(ranked) {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

func historyFromDomainBoostRow(row sqlc.SearchHistoryFTSUrlWithDomainBoostRow) *entity.HistoryEntry {
	return &entity.HistoryEntry{
		ID:          row.ID,
		URL:         row.Url,
		Title:       row.Title.String,
		FaviconURL:  row.FaviconUrl.String,
		VisitCount:  row.VisitCount.Int64,
		LastVisited: row.LastVisited.Time,
		CreatedAt:   row.CreatedAt.Time,
	}
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

type historySearchCandidate struct {
	entry         *entity.HistoryEntry
	score         float64
	urlMatch      bool
	titleMatch    bool
	domainBoost   int64
	exactURLMatch bool
	prefixURL     bool
	prefixTitle   bool
}

func rankHistoryMatches(
	query string,
	words []string,
	urlRows []sqlc.SearchHistoryFTSUrlWithDomainBoostRow,
	titleRows []sqlc.History,
) []entity.HistoryMatch {
	candidates := make(map[int64]*historySearchCandidate, len(urlRows)+len(titleRows))

	for i := range urlRows {
		row := urlRows[i]
		entry := historyFromDomainBoostRow(row)
		candidate := ensureHistoryCandidate(candidates, entry)
		candidate.urlMatch = true
		candidate.domainBoost = maxInt64(candidate.domainBoost, row.DomainBoost)
	}

	for i := range titleRows {
		row := titleRows[i]
		entry := historyFromRow(row)
		candidate := ensureHistoryCandidate(candidates, entry)
		candidate.titleMatch = true
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	matches := make([]entity.HistoryMatch, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.score = computeHistoryCandidateScore(candidate, queryLower, words)
		matches = append(matches, entity.HistoryMatch{
			Entry: candidate.entry,
			Score: candidate.score,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		left := candidates[matches[i].Entry.ID]
		right := candidates[matches[j].Entry.ID]
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if left.exactURLMatch != right.exactURLMatch {
			return left.exactURLMatch
		}
		if left.prefixURL != right.prefixURL {
			return left.prefixURL
		}
		if left.urlMatch != right.urlMatch {
			return left.urlMatch
		}
		if left.prefixTitle != right.prefixTitle {
			return left.prefixTitle
		}
		if matches[i].Entry.VisitCount != matches[j].Entry.VisitCount {
			return matches[i].Entry.VisitCount > matches[j].Entry.VisitCount
		}
		if !matches[i].Entry.LastVisited.Equal(matches[j].Entry.LastVisited) {
			return matches[i].Entry.LastVisited.After(matches[j].Entry.LastVisited)
		}
		return matches[i].Entry.URL < matches[j].Entry.URL
	})

	return matches
}

func ensureHistoryCandidate(candidates map[int64]*historySearchCandidate, entry *entity.HistoryEntry) *historySearchCandidate {
	candidate, ok := candidates[entry.ID]
	if ok {
		return candidate
	}
	candidate = &historySearchCandidate{entry: entry}
	candidates[entry.ID] = candidate
	return candidate
}

func computeHistoryCandidateScore(candidate *historySearchCandidate, queryLower string, words []string) float64 {
	if candidate == nil || candidate.entry == nil {
		return 0
	}

	urlLower := strings.ToLower(candidate.entry.URL)
	titleLower := strings.ToLower(candidate.entry.Title)
	hostLower := strings.ToLower(domainurl.ExtractDomain(candidate.entry.URL))

	score := 0.0
	if candidate.urlMatch {
		score += historyURLMatchScore
	}
	if candidate.titleMatch {
		score += historyTitleMatchScore
	}

	score += scoreQueryMatch(candidate, queryLower, hostLower, urlLower, titleLower)
	score += scoreHistoryWords(words, hostLower, urlLower, titleLower)

	if looksLikeHostStyleQuery(queryLower) {
		pathDepth := historyPathDepth(candidate.entry.URL)
		score -= float64(pathDepth * historyPathDepthPenalty)
		if pathDepth == 0 {
			score += historyRootPathBonus
		}
	}

	score += float64(candidate.domainBoost) * historyDomainBoostScore
	score += minFloat64(float64(candidate.entry.VisitCount), historyVisitCountCap)
	if time.Since(candidate.entry.LastVisited) <= 7*24*time.Hour {
		score += historyRecentVisitScore
	}
	return score
}

func scoreQueryMatch(
	candidate *historySearchCandidate,
	queryLower, hostLower, urlLower, titleLower string,
) float64 {
	if queryLower == "" {
		return 0
	}

	switch {
	case hostLower == queryLower:
		candidate.exactURLMatch = true
		return historyExactHostScore + scoreTitleQueryMatch(candidate, queryLower, titleLower)
	case strings.TrimPrefix(hostLower, "www.") == queryLower:
		candidate.exactURLMatch = true
		return historyTrimmedHostScore + scoreTitleQueryMatch(candidate, queryLower, titleLower)
	case strings.HasPrefix(hostLower, queryLower):
		candidate.prefixURL = true
		return historyHostPrefixScore + scoreTitleQueryMatch(candidate, queryLower, titleLower)
	case strings.HasPrefix(urlLower, queryLower):
		candidate.prefixURL = true
		return historyURLPrefixScore + scoreTitleQueryMatch(candidate, queryLower, titleLower)
	case strings.Contains(urlLower, queryLower):
		return historyURLContainsScore + scoreTitleQueryMatch(candidate, queryLower, titleLower)
	default:
		return scoreTitleQueryMatch(candidate, queryLower, titleLower)
	}
}

func scoreTitleQueryMatch(candidate *historySearchCandidate, queryLower, titleLower string) float64 {
	switch {
	case strings.HasPrefix(titleLower, queryLower):
		candidate.prefixTitle = true
		return historyTitlePrefixScore
	case strings.Contains(titleLower, queryLower):
		return historyTitleContainsScore
	default:
		return 0
	}
}

func scoreHistoryWords(words []string, hostLower, urlLower, titleLower string) float64 {
	score := 0.0
	for _, word := range words {
		wordLower := strings.ToLower(word)
		if wordLower == "" {
			continue
		}
		if strings.HasPrefix(hostLower, wordLower) {
			score += historyWordHostPrefixScore
		} else if strings.Contains(urlLower, wordLower) {
			score += historyWordURLContainsScore
		}
		if strings.HasPrefix(titleLower, wordLower) {
			score += historyWordTitlePrefixScore
		} else if strings.Contains(titleLower, wordLower) {
			score += historyWordTitleScore
		}
	}
	return score
}

func looksLikeHostStyleQuery(query string) bool {
	return query != "" && !strings.ContainsAny(query, " /?#=&")
}

func historyPathDepth(rawURL string) int {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	trimmed := strings.Trim(parsed.Path, "/")
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "/"))
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func minFloat64(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

// sanitizeFTS5QueryTokens normalizes separators and returns only valid FTS tokens.
func sanitizeFTS5QueryTokens(query string) []string {
	normalized := normalizeFTSSeparators(query)
	rawTokens := strings.Fields(normalized)
	if len(rawTokens) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(rawTokens))
	for _, token := range rawTokens {
		sanitized := sanitizeFTS5Word(token)
		if sanitized == "" {
			continue
		}
		upper := strings.ToUpper(sanitized)
		if upper == "AND" || upper == "OR" || upper == "NOT" || upper == "NEAR" {
			continue
		}
		tokens = append(tokens, sanitized)
	}
	return tokens
}

// normalizeFTSSeparators replaces non-alphanumeric separators with spaces.
func normalizeFTSSeparators(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// sanitizeFTS5Word keeps only alphanumeric runes in a single token.
func sanitizeFTS5Word(word string) string {
	var result strings.Builder
	result.Grow(len(word))
	for _, r := range word {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func logFTSSearchError(ctx context.Context, part, query string, err error) {
	log := logging.FromContext(ctx)
	if isExpectedFTSError(err) {
		count := expectedFTSFailureCount.Add(1)
		if count == 1 || count%100 == 0 {
			log.Debug().
				Uint64("expected_error_count", count).
				Str("part", part).
				Msg("expected FTS errors observed")
		}
		return
	}
	log.Debug().Err(err).Str("part", part).Str("query", query).Msg("FTS search failed")
}

func isExpectedFTSError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "malformed match") ||
		strings.Contains(msg, "fts5: syntax error") ||
		strings.Contains(msg, "unterminated string")
}

func (r *historyRepo) GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	rows, err := r.queries.GetRecentHistory(ctx, sqlc.GetRecentHistoryParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetRecentSince(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}
	// Format: "-N days" for SQLite datetime modifier
	daysModifier := fmt.Sprintf("-%d days", days)
	rows, err := r.queries.GetRecentHistorySince(ctx, daysModifier)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetMostVisited(ctx context.Context, days int) ([]*entity.HistoryEntry, error) {
	if days <= 0 {
		return nil, fmt.Errorf("days must be positive, got %d", days)
	}
	// Format: "-N days" for SQLite datetime modifier
	daysModifier := fmt.Sprintf("-%d days", days)
	rows, err := r.queries.GetMostVisited(ctx, daysModifier)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetAllRecentHistory(ctx context.Context) ([]*entity.HistoryEntry, error) {
	rows, err := r.queries.GetAllRecentHistory(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) GetAllMostVisited(ctx context.Context) ([]*entity.HistoryEntry, error) {
	rows, err := r.queries.GetAllMostVisited(ctx)
	if err != nil {
		return nil, err
	}

	entries := make([]*entity.HistoryEntry, len(rows))
	for i := range rows {
		entries[i] = historyFromRow(rows[i])
	}
	return entries, nil
}

func (r *historyRepo) IncrementVisitCount(ctx context.Context, rawURL string) error {
	// Skip incrementing about:blank - it should always have visit_count = 1
	if rawURL == aboutBlankURL {
		return nil
	}
	return r.queries.IncrementVisitCount(ctx, rawURL)
}

func (r *historyRepo) IncrementVisitCountBy(ctx context.Context, rawURL string, delta int) error {
	// Skip incrementing about:blank - it should always have visit_count = 1
	if rawURL == aboutBlankURL {
		return nil
	}
	if delta <= 0 {
		return fmt.Errorf("visit delta must be > 0, got %d", delta)
	}
	return r.queries.IncrementVisitCountByDelta(ctx, sqlc.IncrementVisitCountByDeltaParams{
		VisitCount: sql.NullInt64{Int64: int64(delta), Valid: true},
		Url:        rawURL,
	})
}

func (r *historyRepo) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteHistoryByID(ctx, id)
}

func (r *historyRepo) DeleteOlderThan(ctx context.Context, before time.Time) error {
	return r.queries.DeleteHistoryOlderThan(ctx, sql.NullTime{Time: before, Valid: true})
}

func (r *historyRepo) DeleteAll(ctx context.Context) error {
	return r.queries.DeleteAllHistory(ctx)
}

func (r *historyRepo) DeleteByDomain(ctx context.Context, domain string) error {
	return r.queries.DeleteHistoryByDomain(ctx, sqlc.DeleteHistoryByDomainParams{
		Column1: sql.NullString{String: domain, Valid: true},
		Column2: sql.NullString{String: domain, Valid: true},
		Column3: sql.NullString{String: domain, Valid: true},
		Column4: sql.NullString{String: domain, Valid: true},
	})
}

func (r *historyRepo) GetStats(ctx context.Context) (*entity.HistoryStats, error) {
	row, err := r.queries.GetHistoryStats(ctx)
	if err != nil {
		return nil, err
	}

	// Handle the interface{} type for total_visits
	var totalVisits int64
	switch v := row.TotalVisits.(type) {
	case int64:
		totalVisits = v
	case float64:
		totalVisits = int64(v)
	}

	return &entity.HistoryStats{
		TotalEntries: row.TotalEntries,
		TotalVisits:  totalVisits,
		UniqueDays:   row.UniqueDays,
	}, nil
}

func (r *historyRepo) GetDomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error) {
	rows, err := r.queries.GetDomainStats(ctx, int64(limit))
	if err != nil {
		return nil, err
	}

	stats := make([]*entity.DomainStat, len(rows))
	for i, row := range rows {
		var totalVisits int64
		if row.TotalVisits.Valid {
			totalVisits = int64(row.TotalVisits.Float64)
		}

		var lastVisit time.Time
		if v, ok := row.LastVisit.(string); ok && v != "" {
			lastVisit, _ = time.Parse("2006-01-02 15:04:05", v)
		}

		stats[i] = &entity.DomainStat{
			Domain:      row.Domain,
			PageCount:   row.PageCount,
			TotalVisits: totalVisits,
			LastVisit:   lastVisit,
		}
	}
	return stats, nil
}

func (r *historyRepo) GetHourlyDistribution(ctx context.Context) ([]*entity.HourlyDistribution, error) {
	rows, err := r.queries.GetHourlyDistribution(ctx)
	if err != nil {
		return nil, err
	}

	dist := make([]*entity.HourlyDistribution, len(rows))
	for i, row := range rows {
		dist[i] = &entity.HourlyDistribution{
			Hour:       int(row.Hour),
			VisitCount: row.VisitCount,
		}
	}
	return dist, nil
}

func (r *historyRepo) GetDailyVisitCount(ctx context.Context, daysAgo string) ([]*entity.DailyVisitCount, error) {
	rows, err := r.queries.GetDailyVisitCount(ctx, daysAgo)
	if err != nil {
		return nil, err
	}

	counts := make([]*entity.DailyVisitCount, len(rows))
	for i, row := range rows {
		var day string
		if v, ok := row.Day.(string); ok {
			day = v
		}

		var visits int64
		if row.Visits.Valid {
			visits = int64(row.Visits.Float64)
		}

		counts[i] = &entity.DailyVisitCount{
			Day:     day,
			Entries: row.Entries,
			Visits:  visits,
		}
	}
	return counts, nil
}

func historyFromRow(row sqlc.History) *entity.HistoryEntry {
	return &entity.HistoryEntry{
		ID:          row.ID,
		URL:         row.Url,
		Title:       row.Title.String,
		FaviconURL:  row.FaviconUrl.String,
		VisitCount:  row.VisitCount.Int64,
		LastVisited: row.LastVisited.Time,
		CreatedAt:   row.CreatedAt.Time,
	}
}
