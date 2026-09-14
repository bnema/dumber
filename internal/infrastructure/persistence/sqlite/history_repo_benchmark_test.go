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
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
)

// historyBenchmarkCorpusSize is the number of rows seeded for persistence
// benchmarks.
const historyBenchmarkCorpusSize = 200

// historyBenchmarkEligibleRows is the number of rows left inside the bounded
// window; the remainder are stale so bounded benchmarks still exercise a large
// history with few eligible rows.
const historyBenchmarkEligibleRows = 40

// historyBenchmarkStaleRows is the number of stale, high-visit-count rows seeded
// for bounded most-visited benchmarks. They outrank every eligible row, so an
// unfiltered most-visited scan would never reach the eligible rows within the
// result limit.
const historyBenchmarkStaleRows = 2000

// newHistoryBenchmarkRepository creates a representative local history corpus
// outside the measured operation. Sidebar reads use GetRecent and omnibox
// history suggestions use Search.
func newHistoryBenchmarkRepository(b *testing.B) (*sql.DB, repository.HistoryRepository) {
	b.Helper()

	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(b.TempDir(), "dumber.db"))
	if err != nil {
		b.Fatal(err)
	}

	b.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			b.Errorf("close benchmark database: %v", closeErr)
		}
	})

	repo := sqlite.NewHistoryRepository(db)
	for i := range historyBenchmarkCorpusSize {
		entry := &entity.HistoryEntry{
			URL:   fmt.Sprintf("https://example%d.test/dumber/history/%d", i%20, i),
			Title: fmt.Sprintf("Dumber history entry %d", i),
		}
		if err := repo.Save(ctx, entry); err != nil {
			b.Fatal(err)
		}
	}

	return db, repo
}

// stageBoundedHistoryBenchmark ages all but historyBenchmarkEligibleRows beyond
// the cutoff so bounded-window benchmarks measure a mostly-stale history, and
// returns the cutoff to pass to the repository.
func stageBoundedHistoryBenchmark(b *testing.B, db *sql.DB) time.Time {
	b.Helper()

	ctx := historyTestCtx()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(
		ctx,
		`UPDATE history SET last_visited = ? WHERE id <= ?`,
		sqliteTimestamp(now.Add(-time.Hour)),
		historyBenchmarkEligibleRows,
	); err != nil {
		b.Fatal(err)
	}
	if _, err := db.ExecContext(
		ctx,
		`UPDATE history SET last_visited = ? WHERE id > ?`,
		sqliteTimestamp(now.AddDate(0, 0, -400)),
		historyBenchmarkEligibleRows,
	); err != nil {
		b.Fatal(err)
	}

	return now.AddDate(0, 0, -30)
}

// newMostVisitedBenchmarkUseCase seeds thousands of stale high-count rows plus
// eligibleRows recent low-count rows, then returns the application use case so
// benchmarks exercise the real omnibox most-visited path.
func newMostVisitedBenchmarkUseCase(b *testing.B, eligibleRows int) *usecase.SearchHistoryUseCase {
	b.Helper()

	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(b.TempDir(), "dumber.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			b.Errorf("close benchmark database: %v", closeErr)
		}
	})

	now := time.Now().UTC()
	stale := sqliteTimestamp(now.AddDate(0, 0, -400))
	recent := sqliteTimestamp(now.Add(-time.Hour))

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		b.Fatal(err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO history (url, title, last_visited, visit_count) VALUES (?, ?, ?, ?)`)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range historyBenchmarkStaleRows {
		if _, err := stmt.ExecContext(
			ctx,
			fmt.Sprintf("https://stale%d.test/page", i),
			fmt.Sprintf("Stale %d", i),
			stale,
			10_000+i,
		); err != nil {
			b.Fatal(err)
		}
	}
	for i := range eligibleRows {
		if _, err := stmt.ExecContext(
			ctx,
			fmt.Sprintf("https://eligible%d.test/page", i),
			fmt.Sprintf("Eligible %d", i),
			recent,
			1+i,
		); err != nil {
			b.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		b.Fatal(err)
	}

	logBoundedMostVisitedPlan(b, ctx, db, now.AddDate(0, 0, -30))

	return usecase.NewSearchHistoryUseCase(sqlite.NewHistoryRepository(db))
}

// logBoundedMostVisitedPlan reports (without asserting) the planner choice for
// the bounded most-visited query, which is informational and not part of the
// contract.
func logBoundedMostVisitedPlan(b *testing.B, ctx context.Context, db *sql.DB, cutoff time.Time) {
	b.Helper()

	rows, err := db.QueryContext(
		ctx,
		"EXPLAIN QUERY PLAN SELECT * FROM history WHERE last_visited >= ? ORDER BY visit_count DESC, last_visited DESC LIMIT ?",
		sqliteTimestamp(cutoff),
		10,
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
	_, repo := newHistoryBenchmarkRepository(b)
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.GetRecent(ctx, 50, 0, repository.HistoryScope{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteSidebarRecentBounded(b *testing.B) {
	db, repo := newHistoryBenchmarkRepository(b)
	scope := repository.HistoryScope{Cutoff: stageBoundedHistoryBenchmark(b, db)}
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.GetRecent(ctx, 50, 0, scope); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHistorySQLiteOmniboxMostVisitedBoundedFewEligible measures the
// bounded most-visited path when only a handful of rows fall inside the window
// despite thousands of higher-count stale rows.
func BenchmarkHistorySQLiteOmniboxMostVisitedBoundedFewEligible(b *testing.B) {
	uc := newMostVisitedBenchmarkUseCase(b, 5)
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := uc.GetMostVisitedWithin(ctx, 10, 30); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHistorySQLiteOmniboxMostVisitedBoundedNoEligible measures the
// bounded most-visited path when no row falls inside the window.
func BenchmarkHistorySQLiteOmniboxMostVisitedBoundedNoEligible(b *testing.B) {
	uc := newMostVisitedBenchmarkUseCase(b, 0)
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := uc.GetMostVisitedWithin(ctx, 10, 30); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteOmniboxSearch(b *testing.B) {
	_, repo := newHistoryBenchmarkRepository(b)
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.Search(ctx, "dumber history", 10, repository.HistoryScope{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteOmniboxSearchBounded(b *testing.B) {
	db, repo := newHistoryBenchmarkRepository(b)
	scope := repository.HistoryScope{Cutoff: stageBoundedHistoryBenchmark(b, db)}
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.Search(ctx, "dumber history", 10, scope); err != nil {
			b.Fatal(err)
		}
	}
}
