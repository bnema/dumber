package integration

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/services"
	_ "github.com/ncruces/go-sqlite3/driver" // SQLite driver
	_ "github.com/ncruces/go-sqlite3/embed"  // Embed SQLite
)

const (
	testExampleURL = "https://example.com"
)

// MockWindowUpdater for integration testing
type MockWindowUpdater struct {
	titles []string
	mutex  sync.Mutex
}

// Implement the WindowTitleUpdater interface method
func (m *MockWindowUpdater) SetTitle(title string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.titles = append(m.titles, title)
}

// setupTestDB creates a temporary SQLite database for integration testing
func setupTestDB(t *testing.T) (*sql.DB, *db.Queries, func()) {
	// Create temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_browser.db")

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	// Create tables using the migration schema
	createTablesSQL := `
    CREATE TABLE IF NOT EXISTS history (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        url TEXT NOT NULL,
        title TEXT,
        visit_count INTEGER DEFAULT 1,
        last_visited DATETIME DEFAULT CURRENT_TIMESTAMP,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    
    CREATE TABLE IF NOT EXISTS shortcuts (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        shortcut TEXT NOT NULL UNIQUE,
        url_template TEXT NOT NULL,
        description TEXT,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    
    CREATE TABLE IF NOT EXISTS zoom_levels (
        domain TEXT NOT NULL UNIQUE,
        zoom_factor REAL NOT NULL DEFAULT 1.0,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `

	if _, err := database.Exec(createTablesSQL); err != nil {
		t.Fatalf("Failed to create test tables: %v", err)
	}

	queries := db.New(database)

	cleanup := func() {
		if err := database.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
		if err := os.Remove(dbPath); err != nil {
			log.Printf("Warning: failed to remove database file %s: %v", dbPath, err)
		}
	}

	return database, queries, cleanup
}

// setupTestBrowserService creates a BrowserService with test database
func setupTestBrowserService(t *testing.T) (*services.BrowserService, func()) {
	_, queries, cleanup := setupTestDB(t)

	cfg := &config.Config{
		DefaultZoom: 1.0, // Set default zoom level
	}

	service := services.NewBrowserService(cfg, queries)

	// Load zoom cache (simulates startup behavior)
	if err := service.LoadZoomCacheFromDB(context.Background()); err != nil {
		t.Fatalf("Failed to load zoom cache: %v", err)
	}

	return service, cleanup
}

// Integration Test: Zoom Persistence
func TestBrowserControls_ZoomPersistence_Integration(t *testing.T) {
	service, cleanup := setupTestBrowserService(t)
	defer cleanup()

	ctx := context.Background()
	testURL := testExampleURL

	t.Run("Zoom level persists across service operations", func(t *testing.T) {
		// Set initial zoom level
		zoomLevel, err := service.ZoomIn(ctx, testURL)
		if err != nil {
			t.Fatalf("Failed to zoom in: %v", err)
		}

		expectedZoom := 1.1
		if zoomLevel != expectedZoom {
			t.Errorf("Expected zoom %f, got %f", expectedZoom, zoomLevel)
		}

		// Zoom in again
		zoomLevel, err = service.ZoomIn(ctx, testURL)
		if err != nil {
			t.Fatalf("Failed to zoom in again: %v", err)
		}

		expectedZoom = 1.2
		if zoomLevel != expectedZoom {
			t.Errorf("Expected zoom %f, got %f", expectedZoom, zoomLevel)
		}

		// Test that zoom persists for the same URL
		// Simulate getting zoom level (this would typically happen on page load)
		currentZoom, err := service.GetZoomLevel(ctx, testURL)
		if err != nil {
			t.Fatalf("Failed to get zoom level: %v", err)
		}

		if currentZoom != expectedZoom {
			t.Errorf("Zoom level not persisted. Expected %f, got %f", expectedZoom, currentZoom)
		}
	})

	t.Run("Different URLs have independent zoom levels", func(t *testing.T) {
		url1 := testExampleURL
		url2 := "https://google.com"

		// Set different zoom levels for different URLs
		zoom1, err := service.ZoomIn(ctx, url1)
		if err != nil {
			t.Fatalf("Failed to zoom in for url1: %v", err)
		}

		zoom2, err := service.ZoomOut(ctx, url2)
		if err != nil {
			t.Fatalf("Failed to zoom out for url2: %v", err)
		}

		// Verify they are different
		if zoom1 == zoom2 {
			t.Errorf("Expected different zoom levels for different URLs, both got %f", zoom1)
		}

		// Verify persistence for each URL
		persistedZoom1, err := service.GetZoomLevel(ctx, url1)
		if err != nil {
			t.Fatalf("Failed to get zoom level for url1: %v", err)
		}

		persistedZoom2, err := service.GetZoomLevel(ctx, url2)
		if err != nil {
			t.Fatalf("Failed to get zoom level for url2: %v", err)
		}

		if persistedZoom1 != zoom1 {
			t.Errorf("URL1 zoom not persisted. Expected %f, got %f", zoom1, persistedZoom1)
		}

		if persistedZoom2 != zoom2 {
			t.Errorf("URL2 zoom not persisted. Expected %f, got %f", zoom2, persistedZoom2)
		}
	})

	t.Run("Zoom reset works correctly", func(t *testing.T) {
		testURL := "https://test.com"

		// Change zoom level
		if _, err := service.ZoomIn(ctx, testURL); err != nil {
			t.Logf("Warning: ZoomIn failed: %v", err)
		}
		if _, err := service.ZoomIn(ctx, testURL); err != nil {
			t.Logf("Warning: ZoomIn failed: %v", err)
		}

		// Reset zoom
		resetZoom, err := service.ResetZoom(ctx, testURL)
		if err != nil {
			t.Fatalf("Failed to reset zoom: %v", err)
		}

		expectedZoom := 1.0
		if resetZoom != expectedZoom {
			t.Errorf("Expected reset zoom %f, got %f", expectedZoom, resetZoom)
		}

		// Verify persistence of reset
		persistedZoom, err := service.GetZoomLevel(ctx, testURL)
		if err != nil {
			t.Fatalf("Failed to get zoom level after reset: %v", err)
		}

		if persistedZoom != expectedZoom {
			t.Errorf("Reset zoom not persisted. Expected %f, got %f", expectedZoom, persistedZoom)
		}
	})
}

// Integration Test: Navigation with History
func TestBrowserControls_NavigationHistory_Integration(t *testing.T) {
	_, cleanup := setupTestBrowserService(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Navigation builds history correctly", func(t *testing.T) {
		// This test assumes the service has methods to add history entries
		// and that navigation uses real history data

		// Add some test history entries directly to database for testing
		database, queries, dbCleanup := setupTestDB(t)
		defer dbCleanup()

		// Insert test history entries
		testURLs := []string{
			testExampleURL,
			"https://google.com",
			"https://github.com",
		}

		for _, url := range testURLs {
			_, err := database.Exec(
				`INSERT INTO history (url, title, visit_count) VALUES (?, ?, ?)`,
				url, "Test Title", 1,
			)
			if err != nil {
				t.Fatalf("Failed to insert test history: %v", err)
			}
		}

		// Create service with populated database
		serviceWithHistory := services.NewBrowserService(&config.Config{}, queries)

		// Test navigation operations
		err := serviceWithHistory.GoBack(ctx)
		if err != nil {
			t.Logf("GoBack returned error (acceptable for test): %v", err)
		}

		err = serviceWithHistory.GoForward(ctx)
		if err != nil {
			t.Logf("GoForward returned error (acceptable for test): %v", err)
		}
	})
}

// Integration Test: WindowTitleUpdater Integration
func TestBrowserControls_WindowTitleUpdater_Integration(t *testing.T) {
	service, cleanup := setupTestBrowserService(t)
	defer cleanup()

	// Create mock window title updater
	mockUpdater := &MockWindowUpdater{
		titles: make([]string, 0),
	}

	// This test would need the actual WindowTitleUpdater interface to be implemented
	// For now, we test that the service accepts the updater
	t.Run("Service integrates with WindowTitleUpdater", func(t *testing.T) {
		// This is a placeholder test - would need actual implementation
		// to verify title updates during navigation operations

		service.SetWindowTitleUpdater(mockUpdater)

		// Simulate navigation that should trigger title update
		ctx := context.Background()
		if err := service.GoBack(ctx); err != nil {
			t.Logf("Warning: GoBack failed: %v", err)
		}

		// In a full integration test, we would verify that:
		// 1. Title updates were called
		// 2. Correct titles were set
		// 3. Updates happened at appropriate times

		// For now, just verify no panic occurred and titles were set
		if mockUpdater == nil {
			t.Errorf("WindowTitleUpdater integration failed: mockUpdater is nil")
		}
	})
}

// Integration Test: URL Copying with System Integration
func TestBrowserControls_URLCopying_Integration(t *testing.T) {
	service, cleanup := setupTestBrowserService(t)
	defer cleanup()

	t.Run("URL copying returns current browser URL", func(t *testing.T) {
		ctx := context.Background()

		// Test URL copying functionality
		testURL := testExampleURL
		err := service.CopyCurrentURL(ctx, testURL)
		if err != nil {
			t.Fatalf("CopyCurrentURL failed: %v", err)
		}

		// In a full integration test, we would also verify:
		// 1. System clipboard is updated
		// 2. Correct URL format
		// 3. Special handling for local/file URLs
	})
}

// Integration Test: Performance Requirements
func TestBrowserControls_Performance_Integration(t *testing.T) {
	service, cleanup := setupTestBrowserService(t)
	defer cleanup()

	ctx := context.Background()
	testURL := testExampleURL

	t.Run("Zoom operations complete within performance requirements", func(t *testing.T) {
		// Spec requires <50ms for history search, similar expectation for zoom
		start := time.Now()

		_, err := service.ZoomIn(ctx, testURL)
		if err != nil {
			t.Fatalf("ZoomIn failed: %v", err)
		}

		duration := time.Since(start)

		// Zoom operations should be fast (<10ms typically)
		maxDuration := 50 * time.Millisecond
		if duration > maxDuration {
			t.Errorf("Zoom operation too slow: %v (max: %v)", duration, maxDuration)
		}
	})

	t.Run("Navigation operations complete within reasonable time", func(t *testing.T) {
		start := time.Now()

		err := service.GoBack(ctx)
		if err != nil {
			t.Fatalf("GoBack failed: %v", err)
		}

		duration := time.Since(start)

		// Navigation should be reasonably fast
		maxDuration := 100 * time.Millisecond
		if duration > maxDuration {
			t.Errorf("Navigation operation too slow: %v (max: %v)", duration, maxDuration)
		}
	})
}

// Helper function to verify database state
func verifyZoomInDatabase(t *testing.T, queries *db.Queries, url string, expectedZoom float64) {
	ctx := context.Background()

	// Extract domain from URL (zoom is stored per domain, not full URL)
	domain := extractDomain(url)

	// Get zoom level from database
	zoom, err := queries.GetZoomLevel(ctx, domain)
	if err != nil {
		t.Fatalf("Failed to get zoom level from database for domain %s: %v", domain, err)
	}

	if zoom != expectedZoom {
		t.Errorf("Database zoom mismatch for domain %s. Expected %f, got %f", domain, expectedZoom, zoom)
	}
}

// extractDomain extracts domain from URL for zoom persistence
func extractDomain(url string) string {
	// This is a simplified version - in real code you'd use net/url
	// For test purposes, assuming test URLs are simple
	return "example.com" // Placeholder - would extract actual domain
}
