package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite/sqlc"
)

const (
	// historyBenchmarkCorpusSize is the number of rows seeded for the
	// recent/search persistence benchmarks.
	historyBenchmarkCorpusSize = 200
	// historyBenchmarkEligibleRows is the number of rows left inside the bounded
	// window; the remainder are stale so bounded benchmarks still exercise a
	// large history with few eligible rows.
	historyBenchmarkEligibleRows = 40
	// historyBenchmarkStaleRows is the number of stale, high-visit-count rows
	// seeded for bounded most-visited benchmarks. They outrank every eligible
	// row, so an unfiltered most-visited scan would never reach the eligible rows
	// within the result limit.
	historyBenchmarkStaleRows = 2000

	historyBenchmarkRecentLimit      = 50
	historyBenchmarkSearchLimit      = 10
	historyBenchmarkMostVisitedLimit = 10
	historyBenchmarkMaxAgeDays       = 30
	// historyBenchmarkStaleAgeDays places corpus rows well outside the window.
	historyBenchmarkStaleAgeDays = 400
)

// historyBenchmarkRow is one explicitly categorized corpus row. Category is
// encoded in the URL, title, and timestamp rather than an AUTOINCREMENT id so
// benchmarks do not depend on insertion order.
type historyBenchmarkRow struct {
	url         string
	title       string
	lastVisited time.Time
	visitCount  int
}

// historyBenchmarkURL returns the stable URL for corpus row i.
func historyBenchmarkURL(i int) string {
	return fmt.Sprintf("https://example%d.test/dumber/history/%d", i%20, i)
}

// newHistoryBenchmarkDB opens an isolated benchmark database and registers
// cleanup.
func newHistoryBenchmarkDB(b *testing.B) *sql.DB {
	b.Helper()

	db, err := sqlite.NewConnection(historyTestCtx(), filepath.Join(b.TempDir(), "dumber.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			b.Errorf("close benchmark database: %v", closeErr)
		}
	})

	return db
}

// seedHistoryBenchmarkRows inserts corpus rows in a single transaction without
// going through the logging repository path.
func seedHistoryBenchmarkRows(b *testing.B, ctx context.Context, db *sql.DB, rows []historyBenchmarkRow) {
	b.Helper()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO history (url, title, last_visited, visit_count) VALUES (?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		b.Fatal(err)
	}
	defer func() { _ = stmt.Close() }()

	for _, row := range rows {
		if _, err := stmt.ExecContext(
			ctx,
			row.url,
			row.title,
			sqliteTimestamp(row.lastVisited),
			row.visitCount,
		); err != nil {
			_ = tx.Rollback()
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}
}

// historyBenchmarkCorpusRows builds historyBenchmarkCorpusSize rows with the
// first eligibleRows rows recent and the rest stale.
func historyBenchmarkCorpusRows(now time.Time, eligibleRows int) []historyBenchmarkRow {
	recent := now.Add(-time.Hour)
	stale := now.AddDate(0, 0, -historyBenchmarkStaleAgeDays)

	rows := make([]historyBenchmarkRow, 0, historyBenchmarkCorpusSize)
	for i := range historyBenchmarkCorpusSize {
		lastVisited := stale
		if i < eligibleRows {
			lastVisited = recent
		}
		rows = append(rows, historyBenchmarkRow{
			url:         historyBenchmarkURL(i),
			title:       fmt.Sprintf("Dumber history entry %d", i),
			lastVisited: lastVisited,
			visitCount:  1,
		})
	}
	return rows
}

// newHistoryBenchmarkRepository seeds an unbounded corpus where every row is
// inside the window.
func newHistoryBenchmarkRepository(b *testing.B) repository.HistoryRepository {
	b.Helper()

	db := newHistoryBenchmarkDB(b)
	seedHistoryBenchmarkRows(b, historyTestCtx(), db, historyBenchmarkCorpusRows(time.Now().UTC(), historyBenchmarkCorpusSize))

	return sqlite.NewHistoryRepository(db)
}

// newBoundedHistoryBenchmarkRepository seeds a corpus where only
// historyBenchmarkEligibleRows rows fall inside the window, and returns the
// cutoff for the bounded repository paths.
func newBoundedHistoryBenchmarkRepository(b *testing.B) (repository.HistoryRepository, time.Time) {
	b.Helper()

	now := time.Now().UTC()
	db := newHistoryBenchmarkDB(b)
	seedHistoryBenchmarkRows(b, historyTestCtx(), db, historyBenchmarkCorpusRows(now, historyBenchmarkEligibleRows))

	return sqlite.NewHistoryRepository(db), now.AddDate(0, 0, -historyBenchmarkMaxAgeDays)
}

// newMostVisitedBenchmarkUseCase seeds thousands of stale high-count rows plus
// eligibleRows recent low-count rows, then returns the application use case so
// benchmarks exercise the real omnibox most-visited path.
func newMostVisitedBenchmarkUseCase(b *testing.B, eligibleRows int) *usecase.SearchHistoryUseCase {
	b.Helper()

	now := time.Now().UTC()
	db := newHistoryBenchmarkDB(b)

	rows := make([]historyBenchmarkRow, 0, historyBenchmarkStaleRows+eligibleRows)
	for i := range historyBenchmarkStaleRows {
		rows = append(rows, historyBenchmarkRow{
			url:         fmt.Sprintf("https://stale%d.test/page", i),
			title:       fmt.Sprintf("Stale %d", i),
			lastVisited: now.AddDate(0, 0, -historyBenchmarkStaleAgeDays),
			visitCount:  10_000 + i,
		})
	}
	for i := range eligibleRows {
		rows = append(rows, historyBenchmarkRow{
			url:         fmt.Sprintf("https://eligible%d.test/page", i),
			title:       fmt.Sprintf("Eligible %d", i),
			lastVisited: now.Add(-time.Hour),
			visitCount:  1 + i,
		})
	}
	seedHistoryBenchmarkRows(b, historyTestCtx(), db, rows)

	logBoundedMostVisitedPlan(b, historyTestCtx(), db, now.AddDate(0, 0, -historyBenchmarkMaxAgeDays))

	return usecase.NewSearchHistoryUseCase(sqlite.NewHistoryRepository(db))
}

// requireBenchmarkLen fails the benchmark if a fixture or query regression
// changes the expected result count, so benchmarks never silently measure empty
// work.
func requireBenchmarkLen(b *testing.B, got, want int, label string) {
	b.Helper()

	if got != want {
		b.Fatalf("%s: got %d results, want %d", label, got, want)
	}
}

// logBoundedMostVisitedPlan reports (without asserting) the planner choice for
// the bounded most-visited query, which is informational and not part of the
// contract.
func logBoundedMostVisitedPlan(b *testing.B, ctx context.Context, db *sql.DB, cutoff time.Time) {
	b.Helper()

	rows, err := db.QueryContext(
		ctx,
		"EXPLAIN QUERY PLAN "+sqlc.GetMostVisitedHistorySinceCutoff,
		sqliteTimestamp(cutoff),
		historyBenchmarkMostVisitedLimit,
	)
	if err != nil {
		b.Logf("bounded most-visited explain failed: %v", err)
		return
	}
	defer func() { _ = rows.Close() }()

	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			b.Logf("bounded most-visited explain scan failed: %v", err)
			return
		}
		details = append(details, detail)
	}
	b.Logf("bounded most-visited plan: %s", strings.Join(details, "; "))
}

func BenchmarkHistorySQLiteSidebarRecent(b *testing.B) {
	repo := newHistoryBenchmarkRepository(b)
	ctx := historyTestCtx()

	results, err := repo.GetRecent(ctx, historyBenchmarkRecentLimit, 0, repository.HistoryScope{})
	if err != nil {
		b.Fatal(err)
	}
	requireBenchmarkLen(b, len(results), historyBenchmarkRecentLimit, "unbounded recent")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.GetRecent(ctx, historyBenchmarkRecentLimit, 0, repository.HistoryScope{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteSidebarRecentBounded(b *testing.B) {
	repo, cutoff := newBoundedHistoryBenchmarkRepository(b)
	ctx := historyTestCtx()
	scope := repository.HistoryScope{Cutoff: cutoff}

	results, err := repo.GetRecent(ctx, historyBenchmarkRecentLimit, 0, scope)
	if err != nil {
		b.Fatal(err)
	}
	requireBenchmarkLen(b, len(results), historyBenchmarkEligibleRows, "bounded recent")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.GetRecent(ctx, historyBenchmarkRecentLimit, 0, scope); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHistorySQLiteOmniboxMostVisitedBoundedFewEligible measures the
// bounded most-visited path when only a handful of rows fall inside the window
// despite thousands of higher-count stale rows.
func BenchmarkHistorySQLiteOmniboxMostVisitedBoundedFewEligible(b *testing.B) {
	const eligibleRows = 5

	uc := newMostVisitedBenchmarkUseCase(b, eligibleRows)
	ctx := historyTestCtx()

	results, err := uc.GetMostVisitedWithin(ctx, historyBenchmarkMostVisitedLimit, historyBenchmarkMaxAgeDays)
	if err != nil {
		b.Fatal(err)
	}
	requireBenchmarkLen(b, len(results), eligibleRows, "bounded most-visited few eligible")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := uc.GetMostVisitedWithin(ctx, historyBenchmarkMostVisitedLimit, historyBenchmarkMaxAgeDays); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHistorySQLiteOmniboxMostVisitedBoundedNoEligible measures the
// bounded most-visited path when no row falls inside the window.
func BenchmarkHistorySQLiteOmniboxMostVisitedBoundedNoEligible(b *testing.B) {
	uc := newMostVisitedBenchmarkUseCase(b, 0)
	ctx := historyTestCtx()

	results, err := uc.GetMostVisitedWithin(ctx, historyBenchmarkMostVisitedLimit, historyBenchmarkMaxAgeDays)
	if err != nil {
		b.Fatal(err)
	}
	requireBenchmarkLen(b, len(results), 0, "bounded most-visited no eligible")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := uc.GetMostVisitedWithin(ctx, historyBenchmarkMostVisitedLimit, historyBenchmarkMaxAgeDays); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteOmniboxSearch(b *testing.B) {
	repo := newHistoryBenchmarkRepository(b)
	ctx := historyTestCtx()

	results, err := repo.Search(ctx, "dumber history", historyBenchmarkSearchLimit, repository.HistoryScope{})
	if err != nil {
		b.Fatal(err)
	}
	requireBenchmarkLen(b, len(results), historyBenchmarkSearchLimit, "unbounded search")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.Search(ctx, "dumber history", historyBenchmarkSearchLimit, repository.HistoryScope{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteOmniboxSearchBounded(b *testing.B) {
	repo, cutoff := newBoundedHistoryBenchmarkRepository(b)
	ctx := historyTestCtx()
	scope := repository.HistoryScope{Cutoff: cutoff}

	results, err := repo.Search(ctx, "dumber history", historyBenchmarkSearchLimit, scope)
	if err != nil {
		b.Fatal(err)
	}
	requireBenchmarkLen(b, len(results), historyBenchmarkSearchLimit, "bounded search")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.Search(ctx, "dumber history", historyBenchmarkSearchLimit, scope); err != nil {
			b.Fatal(err)
		}
	}
}
