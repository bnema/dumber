package sqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/bnema/purego-sqlite/driver"
	"github.com/pressly/goose/v3"
)

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func runMigrationsThroughFavoritesSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set dialect: %v", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.UpTo(db, "migrations", 10); err != nil {
		t.Fatalf("migrate through favorites schema: %v", err)
	}
}

func runAllMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
}

func countRows(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		t.Fatalf("count rows %q: %v", query, err)
	}
	return count
}

func TestFavoritesTagsFirstMigrationFolderPathToTag(t *testing.T) {
	db := openMigrationTestDB(t)
	runMigrationsThroughFavoritesSchema(t, db)

	mustExec(t, db, `INSERT INTO favorite_folders(id, name, parent_id) VALUES (1, 'Research', NULL), (2, 'Browser Engines', 1)`)
	mustExec(t, db, `INSERT INTO favorites(id, url, title, folder_id) VALUES (1, 'https://webkit.org', 'WebKit', 2)`)

	runAllMigrations(t, db)

	var tagID int64
	if err := db.QueryRow(`SELECT id FROM favorite_tags WHERE name = 'research-browser-engines'`).Scan(&tagID); err != nil {
		t.Fatalf("expected folder path tag: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tag_assignments WHERE favorite_id = 1 AND tag_id = ?`, tagID); got != 1 {
		t.Fatalf("expected assignment to folder path tag, got %d", got)
	}
}

func TestFavoritesTagsFirstMigrationFolderTagCollisionSuffix(t *testing.T) {
	db := openMigrationTestDB(t)
	runMigrationsThroughFavoritesSchema(t, db)

	mustExec(t, db, `INSERT INTO favorite_tags(id, name, color) VALUES (1, 'dev-go', '#111111')`)
	mustExec(t, db, `INSERT INTO favorite_folders(id, name, parent_id) VALUES (7, 'Dev', NULL), (8, 'Go', 7)`)
	mustExec(t, db, `INSERT INTO favorites(id, url, title, folder_id) VALUES (1, 'https://go.dev', 'Go', 8)`)

	runAllMigrations(t, db)

	var tagID int64
	if err := db.QueryRow(`SELECT id FROM favorite_tags WHERE name = 'dev-go-folder-8'`).Scan(&tagID); err != nil {
		t.Fatalf("expected suffixed collision tag: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tag_assignments WHERE favorite_id = 1 AND tag_id = ?`, tagID); got != 1 {
		t.Fatalf("expected assignment to suffixed tag, got %d", got)
	}
}

func TestFavoritesTagsFirstMigrationGeneratedSuffixAvoidsExistingTag(t *testing.T) {
	db := openMigrationTestDB(t)
	runMigrationsThroughFavoritesSchema(t, db)

	mustExec(t, db, `INSERT INTO favorite_tags(id, name, color) VALUES (1, 'foo', '#111111'), (2, 'foo-folder-3', '#222222')`)
	mustExec(t, db, `INSERT INTO favorite_folders(id, name, parent_id) VALUES (3, 'Foo', NULL)`)
	mustExec(t, db, `INSERT INTO favorites(id, url, title, folder_id) VALUES (1, 'https://foo.example', 'Foo', 3)`)

	runAllMigrations(t, db)

	var tagID int64
	if err := db.QueryRow(`SELECT id FROM favorite_tags WHERE name = 'foo-folder-3-2'`).Scan(&tagID); err != nil {
		t.Fatalf("expected second-order suffixed collision tag: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tag_assignments WHERE favorite_id = 1 AND tag_id = ?`, tagID); got != 1 {
		t.Fatalf("expected assignment to second-order suffixed tag, got %d", got)
	}
}

func TestFavoritesTagsFirstMigrationGeneratedSuffixAvoidsFolderNaturalSlug(t *testing.T) {
	db := openMigrationTestDB(t)
	runMigrationsThroughFavoritesSchema(t, db)

	mustExec(t, db, `INSERT INTO favorite_tags(id, name, color) VALUES (1, 'foo', '#111111')`)
	mustExec(t, db, `INSERT INTO favorite_folders(id, name, parent_id) VALUES (3, 'Foo', NULL), (4, 'foo-folder-3', NULL)`)
	mustExec(t, db, `INSERT INTO favorites(id, url, title, folder_id) VALUES (1, 'https://foo.example', 'Foo', 3), (2, 'https://folder.example', 'Folder', 4)`)

	runAllMigrations(t, db)

	var folder3TagID int64
	if err := db.QueryRow(`SELECT id FROM favorite_tags WHERE name = 'foo-folder-3-2'`).Scan(&folder3TagID); err != nil {
		t.Fatalf("expected generated suffix to avoid folder natural slug: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tag_assignments WHERE favorite_id = 1 AND tag_id = ?`, folder3TagID); got != 1 {
		t.Fatalf("expected assignment to second-order suffixed tag, got %d", got)
	}

	var folder4TagID int64
	if err := db.QueryRow(`SELECT id FROM favorite_tags WHERE name = 'foo-folder-3'`).Scan(&folder4TagID); err != nil {
		t.Fatalf("expected natural folder slug tag: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tag_assignments WHERE favorite_id = 2 AND tag_id = ?`, folder4TagID); got != 1 {
		t.Fatalf("expected assignment to natural slug tag, got %d", got)
	}
}

func TestFavoritesTagsFirstMigrationMergesFoldedDuplicateTags(t *testing.T) {
	db := openMigrationTestDB(t)
	runMigrationsThroughFavoritesSchema(t, db)

	mustExec(t, db, `INSERT INTO favorite_tags(id, name, color) VALUES (1, ' Dev-Go ', '#111111'), (2, 'dev-go', '#222222')`)
	mustExec(t, db, `INSERT INTO favorites(id, url, title) VALUES (1, 'https://one.example', 'One'), (2, 'https://two.example', 'Two')`)
	mustExec(t, db, `INSERT INTO favorite_tag_assignments(favorite_id, tag_id) VALUES (1, 1), (2, 2)`)

	runAllMigrations(t, db)

	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tags WHERE lower(trim(name)) = 'dev-go'`); got != 1 {
		t.Fatalf("expected one folded dev-go tag, got %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM favorite_tag_assignments WHERE tag_id = 1`); got != 2 {
		t.Fatalf("expected assignments moved to canonical tag 1, got %d", got)
	}
	mustExec(t, db, `INSERT INTO favorite_tags(name, color) VALUES ('another', '#333333')`)
	if _, err := db.Exec(`INSERT INTO favorite_tags(name, color) VALUES (' DEV-GO ', '#444444')`); err == nil {
		t.Fatal("expected folded unique index to reject duplicate tag")
	}
}

func mustExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("exec %s: %v", query, err)
	}
}
