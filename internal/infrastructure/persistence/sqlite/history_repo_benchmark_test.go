package sqlite_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/infrastructure/persistence/sqlite"
)

// newHistoryBenchmarkRepository creates a representative local history corpus
// outside the measured operation. Sidebar reads use GetRecent and omnibox
// history suggestions use Search.
func newHistoryBenchmarkRepository(b *testing.B) (repository.HistoryRepository, func()) {
	b.Helper()

	ctx := historyTestCtx()
	db, err := sqlite.NewConnection(ctx, filepath.Join(b.TempDir(), "dumber.db"))
	if err != nil {
		b.Fatal(err)
	}

	repo := sqlite.NewHistoryRepository(db)
	for i := range 200 {
		entry := &entity.HistoryEntry{
			URL:   fmt.Sprintf("https://example%d.test/dumber/history/%d", i%20, i),
			Title: fmt.Sprintf("Dumber history entry %d", i),
		}
		if err := repo.Save(ctx, entry); err != nil {
			_ = db.Close()
			b.Fatal(err)
		}
	}

	return repo, func() { _ = db.Close() }
}

func BenchmarkHistorySQLiteSidebarRecent(b *testing.B) {
	repo, closeDB := newHistoryBenchmarkRepository(b)
	defer closeDB()
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.GetRecent(ctx, 50, 0); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHistorySQLiteOmniboxSearch(b *testing.B) {
	repo, closeDB := newHistoryBenchmarkRepository(b)
	defer closeDB()
	ctx := historyTestCtx()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := repo.Search(ctx, "dumber history", 10); err != nil {
			b.Fatal(err)
		}
	}
}
