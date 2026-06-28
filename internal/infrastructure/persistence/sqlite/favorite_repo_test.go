package sqlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	_ "github.com/bnema/purego-sqlite/driver"
)

func openFavoriteRepoTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func seedFavoriteWithTags(t *testing.T, db *sql.DB) (entity.FavoriteID, entity.TagID) {
	t.Helper()
	mustExec(t, db, `INSERT INTO favorites(id, url, title, shortcut_key, position) VALUES (1, 'https://one.example', 'One', 1, 1), (2, 'https://two.example', 'Two', NULL, 2)`)
	mustExec(t, db, `INSERT INTO favorite_tags(id, name, color) VALUES (1, 'research', '#111111'), (2, 'engines', '#222222')`)
	mustExec(t, db, `INSERT INTO favorite_tag_assignments(favorite_id, tag_id) VALUES (1, 1), (1, 2), (2, 1)`)
	return 1, 1
}

func TestFavoriteRepositoryGetAllHydratesTags(t *testing.T) {
	db := openFavoriteRepoTestDB(t)
	seedFavoriteWithTags(t, db)
	repo := NewFavoriteRepository(db)

	favorites, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("expected 2 favorites, got %d", len(favorites))
	}
	assertTagNames(t, favorites[0], "engines", "research")
	assertTagNames(t, favorites[1], "research")
}

func TestFavoriteRepositoryFindByIDHydratesTags(t *testing.T) {
	db := openFavoriteRepoTestDB(t)
	favID, _ := seedFavoriteWithTags(t, db)
	repo := NewFavoriteRepository(db)

	fav, err := repo.FindByID(context.Background(), favID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	assertTagNames(t, fav, "engines", "research")
}

func TestFavoriteRepositoryFindByURLHydratesTags(t *testing.T) {
	db := openFavoriteRepoTestDB(t)
	seedFavoriteWithTags(t, db)
	repo := NewFavoriteRepository(db)

	fav, err := repo.FindByURL(context.Background(), "https://one.example")
	if err != nil {
		t.Fatalf("FindByURL: %v", err)
	}
	assertTagNames(t, fav, "engines", "research")
}

func TestFavoriteRepositoryGetByTagHydratesTags(t *testing.T) {
	db := openFavoriteRepoTestDB(t)
	_, tagID := seedFavoriteWithTags(t, db)
	repo := NewFavoriteRepository(db)

	favorites, err := repo.GetByTag(context.Background(), tagID)
	if err != nil {
		t.Fatalf("GetByTag: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("expected 2 tagged favorites, got %d", len(favorites))
	}
	assertTagNames(t, favorites[0], "engines", "research")
	assertTagNames(t, favorites[1], "research")
}

func TestFavoriteRepositoryGetByShortcutHydratesTags(t *testing.T) {
	db := openFavoriteRepoTestDB(t)
	seedFavoriteWithTags(t, db)
	repo := NewFavoriteRepository(db)

	fav, err := repo.GetByShortcut(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetByShortcut: %v", err)
	}
	assertTagNames(t, fav, "engines", "research")
}

func TestFavoriteRepositoryNoTagsHydratesEmptyTags(t *testing.T) {
	db := openFavoriteRepoTestDB(t)
	mustExec(t, db, `INSERT INTO favorites(id, url, title, position) VALUES (1, 'https://none.example', 'None', 1)`)
	repo := NewFavoriteRepository(db)

	fav, err := repo.FindByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if fav == nil {
		t.Fatal("expected favorite")
	}
	if fav.Tags == nil || len(fav.Tags) != 0 {
		t.Fatalf("expected non-nil empty Tags, got %#v", fav.Tags)
	}
}

func assertTagNames(t *testing.T, fav *entity.Favorite, names ...string) {
	t.Helper()
	if fav == nil {
		t.Fatal("favorite is nil")
	}
	if len(fav.Tags) != len(names) {
		t.Fatalf("expected %d tags, got %d (%#v)", len(names), len(fav.Tags), fav.Tags)
	}
	for i, name := range names {
		if fav.Tags[i].Name != name {
			t.Fatalf("tag %d: expected %q, got %q", i, name, fav.Tags[i].Name)
		}
	}
}
