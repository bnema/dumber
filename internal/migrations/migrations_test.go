package migrations

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

// TestMigrationsAreIdempotent verifies that running migrations multiple times doesn't cause errors
func TestMigrationsAreIdempotent(t *testing.T) {
	// Create temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Run migrations first time
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("First migration run failed: %v", err)
	}

	// Get applied migrations after first run
	appliedFirst, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("Failed to get applied migrations after first run: %v", err)
	}

	if len(appliedFirst) == 0 {
		t.Fatal("No migrations were applied on first run")
	}

	// Run migrations second time - should be idempotent
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Second migration run failed (not idempotent): %v", err)
	}

	// Get applied migrations after second run
	appliedSecond, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("Failed to get applied migrations after second run: %v", err)
	}

	// Verify same migrations are applied
	if len(appliedFirst) != len(appliedSecond) {
		t.Errorf("Migration count changed: first=%d, second=%d", len(appliedFirst), len(appliedSecond))
	}

	for i, v := range appliedFirst {
		if i >= len(appliedSecond) || v != appliedSecond[i] {
			t.Errorf("Applied migrations differ at index %d: first=%d, second=%d", i, v, appliedSecond[i])
		}
	}

	// Run migrations third time to be extra sure
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Third migration run failed (not idempotent): %v", err)
	}

	appliedThird, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("Failed to get applied migrations after third run: %v", err)
	}

	if len(appliedThird) != len(appliedFirst) {
		t.Errorf("Migration count changed on third run: expected=%d, got=%d", len(appliedFirst), len(appliedThird))
	}
}

// TestMigrationTrackingPreventsReapplication verifies that applied migrations are tracked and not re-run
func TestMigrationTrackingPreventsReapplication(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Migration run failed: %v", err)
	}

	// Check that schema_migrations table exists and has entries
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query schema_migrations table: %v", err)
	}

	if count == 0 {
		t.Fatal("No migration tracking entries found")
	}

	// Verify each migration has exactly one entry
	rows, err := db.Query("SELECT version, COUNT(*) FROM schema_migrations GROUP BY version HAVING COUNT(*) > 1")
	if err != nil {
		t.Fatalf("Failed to check for duplicate migration entries: %v", err)
	}
	defer rows.Close()

	duplicates := []int{}
	for rows.Next() {
		var version, dupeCount int
		if err := rows.Scan(&version, &dupeCount); err != nil {
			t.Fatalf("Failed to scan duplicate check: %v", err)
		}
		duplicates = append(duplicates, version)
	}

	if len(duplicates) > 0 {
		t.Errorf("Found duplicate migration entries for versions: %v", duplicates)
	}
}

// TestAllMigrationsApplyOnFreshDatabase verifies all migrations work on a new database
func TestAllMigrationsApplyOnFreshDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Get expected migrations
	expectedMigrations, err := GetMigrations()
	if err != nil {
		t.Fatalf("Failed to get migrations: %v", err)
	}

	if len(expectedMigrations) == 0 {
		t.Fatal("No migration files found")
	}

	// Run migrations
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Verify all migrations were applied
	appliedVersions, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("Failed to get applied migrations: %v", err)
	}

	if len(appliedVersions) != len(expectedMigrations) {
		t.Errorf("Expected %d migrations to be applied, got %d", len(expectedMigrations), len(appliedVersions))
	}

	// Verify each expected migration was applied
	for i, migration := range expectedMigrations {
		if i >= len(appliedVersions) {
			t.Errorf("Migration %d (%s) was not applied", migration.Version, migration.Name)
			continue
		}
		if appliedVersions[i] != migration.Version {
			t.Errorf("Migration version mismatch at index %d: expected %d, got %d", i, migration.Version, appliedVersions[i])
		}
	}
}

// TestMigrationOrdering verifies migrations are applied in correct order
func TestMigrationOrdering(t *testing.T) {
	migrations, err := GetMigrations()
	if err != nil {
		t.Fatalf("Failed to get migrations: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatal("No migrations found")
	}

	// Verify migrations are in ascending order
	for i := 1; i < len(migrations); i++ {
		if migrations[i].Version <= migrations[i-1].Version {
			t.Errorf("Migrations not in ascending order: migration[%d].Version=%d, migration[%d].Version=%d",
				i-1, migrations[i-1].Version, i, migrations[i].Version)
		}
	}

	// Verify no gaps in version numbers (should be 1, 2, 3, 4, 5...)
	for i, migration := range migrations {
		expectedVersion := i + 1
		if migration.Version != expectedVersion {
			t.Errorf("Migration version gap detected: expected version %d, got %d", expectedVersion, migration.Version)
		}
	}
}

// TestMigrationTableCreationIsIdempotent verifies the tracking table can be created multiple times
func TestMigrationTableCreationIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create tracking table multiple times
	for i := 0; i < 3; i++ {
		if err := createMigrationsTable(db); err != nil {
			t.Fatalf("Failed to create migrations table on attempt %d: %v", i+1, err)
		}
	}

	// Verify table exists and has correct schema
	var tableName string
	err = db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type='table' AND name='schema_migrations'
	`).Scan(&tableName)

	if err != nil {
		t.Fatalf("Migrations table not found: %v", err)
	}

	if tableName != "schema_migrations" {
		t.Errorf("Expected table name 'schema_migrations', got '%s'", tableName)
	}

	// Verify table has expected columns
	rows, err := db.Query("PRAGMA table_info(schema_migrations)")
	if err != nil {
		t.Fatalf("Failed to get table info: %v", err)
	}
	defer rows.Close()

	expectedColumns := map[string]bool{
		"version":    false,
		"name":       false,
		"applied_at": false,
	}

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString

		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("Failed to scan table info: %v", err)
		}

		if _, expected := expectedColumns[name]; expected {
			expectedColumns[name] = true
		}
	}

	for col, found := range expectedColumns {
		if !found {
			t.Errorf("Expected column '%s' not found in schema_migrations table", col)
		}
	}
}

// TestPartialMigrationState verifies that a database with some migrations can have new ones applied
func TestPartialMigrationState(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create tracking table
	if err := createMigrationsTable(db); err != nil {
		t.Fatalf("Failed to create migrations table: %v", err)
	}

	// Get all migrations
	allMigrations, err := GetMigrations()
	if err != nil {
		t.Fatalf("Failed to get migrations: %v", err)
	}

	if len(allMigrations) < 2 {
		t.Skip("Need at least 2 migrations for this test")
	}

	// Manually apply only the first migration
	firstMigration := allMigrations[0]
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}

	if _, err := tx.Exec(firstMigration.SQL); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to execute first migration: %v", err)
	}

	if _, err := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", firstMigration.Version, firstMigration.Name); err != nil {
		tx.Rollback()
		t.Fatalf("Failed to record first migration: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit first migration: %v", err)
	}

	// Verify only one migration is applied
	applied, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("Failed to get applied migrations: %v", err)
	}

	if len(applied) != 1 {
		t.Fatalf("Expected 1 applied migration, got %d", len(applied))
	}

	// Now run all migrations - should apply the remaining ones
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Failed to run remaining migrations: %v", err)
	}

	// Verify all migrations are now applied
	appliedAfter, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("Failed to get applied migrations after full run: %v", err)
	}

	if len(appliedAfter) != len(allMigrations) {
		t.Errorf("Expected %d migrations applied, got %d", len(allMigrations), len(appliedAfter))
	}

	// Verify the first migration wasn't re-applied
	if appliedAfter[0] != firstMigration.Version {
		t.Errorf("First migration version changed: expected %d, got %d", firstMigration.Version, appliedAfter[0])
	}
}

// TestGetAppliedMigrationsEmptyDatabase verifies behavior on a database with no migrations
func TestGetAppliedMigrationsEmptyDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Query applied migrations on empty database
	applied, err := GetAppliedMigrations(db)
	if err != nil {
		t.Fatalf("GetAppliedMigrations failed on empty database: %v", err)
	}

	if len(applied) != 0 {
		t.Errorf("Expected 0 applied migrations on empty database, got %d", len(applied))
	}
}

// TestVerifyAllMigrationsApplied verifies the verification function works correctly
func TestVerifyAllMigrationsApplied(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Verify should pass
	if err := VerifyAllMigrationsApplied(db); err != nil {
		t.Errorf("VerifyAllMigrationsApplied() failed after running all migrations: %v", err)
	}

	// Test with missing migration - manually delete one from tracking
	_, err = db.Exec("DELETE FROM schema_migrations WHERE version = 3")
	if err != nil {
		t.Fatalf("Failed to delete migration record: %v", err)
	}

	// Verify should now fail
	err = VerifyAllMigrationsApplied(db)
	if err == nil {
		t.Error("VerifyAllMigrationsApplied() should have failed with missing migration, but it passed")
	}
}

// TestExpectedTablesExistAfterMigrations verifies that all expected tables are created
func TestExpectedTablesExistAfterMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := RunEmbeddedMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Expected tables based on current migrations
	expectedTables := []string{
		"schema_migrations",      // Migration tracking
		"history",                // 001_initial.sql
		"shortcuts",              // 001_initial.sql
		"zoom_levels",            // 003_add_zoom_levels.sql
		"certificate_validations", // 005_add_certificate_validations.sql
	}

	// Check each expected table exists
	for _, tableName := range expectedTables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Expected table '%s' does not exist", tableName)
		} else if err != nil {
			t.Fatalf("Failed to check for table '%s': %v", tableName, err)
		}
	}
}
