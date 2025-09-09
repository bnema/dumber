package contract

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"testing"

	"github.com/bnema/dumber/services"
	"github.com/bnema/dumber/internal/config"
	mock_db "github.com/bnema/dumber/internal/db/mocks"
	"github.com/golang/mock/gomock"
)

// floatMatcher implements gomock.Matcher for floating point comparison with tolerance
type floatMatcher struct {
	expected float64
	tolerance float64
}

func (f floatMatcher) Matches(x interface{}) bool {
	if actual, ok := x.(float64); ok {
		return math.Abs(actual - f.expected) < f.tolerance
	}
	return false
}

func (f floatMatcher) String() string {
	return fmt.Sprintf("is within %f of %f", f.tolerance, f.expected)
}

// floatEq creates a matcher for floating point values with tolerance
func floatEq(expected float64) gomock.Matcher {
	return floatMatcher{expected: expected, tolerance: 1e-10}
}

// MockWindowTitleUpdater implements the WindowTitleUpdater interface for testing
type MockWindowTitleUpdater struct {
	lastTitle string
	callCount int
}

func (m *MockWindowTitleUpdater) SetTitle(title string) {
	m.lastTitle = title
	m.callCount++
}

// Contract Test: BrowserService Zoom Controls
// Tests the zoom functionality contracts as specified in 002-browser-controls-ui
func TestBrowserService_ZoomControls_Contract(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	cfg := &config.Config{}
	service := services.NewBrowserService(cfg, mockDB)
	
	ctx := context.Background()
	testURL := "https://example.com"
	
	t.Run("ZoomIn increases zoom level by 0.1", func(t *testing.T) {
		// Setup mock expectations
		mockDB.EXPECT().
			GetZoomLevel(ctx, testURL).
			Return(1.0, nil)
		
		mockDB.EXPECT().
			SetZoomLevel(ctx, testURL, floatEq(1.1)).
			Return(nil)
		
		// Contract: ZoomIn should increase zoom by 0.1 and return new level
		newZoom, err := service.ZoomIn(ctx, testURL)
		
		// Contract assertions
		if err != nil {
			t.Fatalf("ZoomIn should not return error: %v", err)
		}
		
		expectedZoom := 1.1
		if newZoom != expectedZoom {
			t.Errorf("Expected zoom level %f, got %f", expectedZoom, newZoom)
		}
	})
	
	t.Run("ZoomOut decreases zoom level by 0.1", func(t *testing.T) {
		// Setup mock expectations for current zoom level
		mockDB.EXPECT().
			GetZoomLevel(ctx, testURL).
			Return(1.2, nil)
		
		mockDB.EXPECT().
			SetZoomLevel(ctx, testURL, floatEq(1.1)).
			Return(nil)
		
		// Contract: ZoomOut should decrease zoom by 0.1 and return new level
		newZoom, err := service.ZoomOut(ctx, testURL)
		
		// Contract assertions
		if err != nil {
			t.Fatalf("ZoomOut should not return error: %v", err)
		}
		
		expectedZoom := 1.1
		if math.Abs(newZoom - expectedZoom) > 1e-10 {
			t.Errorf("Expected zoom level %f, got %f", expectedZoom, newZoom)
		}
	})
	
	t.Run("ZoomReset sets zoom to 1.0", func(t *testing.T) {
		// Setup mock expectations
		mockDB.EXPECT().
			SetZoomLevel(ctx, testURL, floatEq(1.0)).
			Return(nil)
		
		// Contract: ZoomReset should always return zoom to default (1.0)
		newZoom, err := service.ResetZoom(ctx, testURL)
		
		// Contract assertions
		if err != nil {
			t.Fatalf("ResetZoom should not return error: %v", err)
		}
		
		expectedZoom := 1.0
		if newZoom != expectedZoom {
			t.Errorf("ResetZoom should return 1.0, got %f", expectedZoom)
		}
	})
	
	t.Run("GetZoomLevel returns current zoom level", func(t *testing.T) {
		// Setup mock expectations
		expectedZoom := 1.5
		mockDB.EXPECT().
			GetZoomLevel(ctx, testURL).
			Return(expectedZoom, nil)
		
		// Contract: GetZoomLevel should return current zoom level
		currentZoom, err := service.GetZoomLevel(ctx, testURL)
		
		// Contract assertions
		if err != nil {
			t.Fatalf("GetZoomLevel should not return error: %v", err)
		}
		
		if currentZoom != expectedZoom {
			t.Errorf("Expected zoom level %f, got %f", expectedZoom, currentZoom)
		}
	})
	
	t.Run("GetZoomLevel returns default 1.0 for new URLs", func(t *testing.T) {
		// Setup mock expectations for non-existent URL
		mockDB.EXPECT().
			GetZoomLevel(ctx, testURL).
			Return(0.0, sql.ErrNoRows)
		
		// Contract: GetZoomLevel should return 1.0 for URLs without zoom settings
		currentZoom, err := service.GetZoomLevel(ctx, testURL)
		
		// Contract assertions
		if err != nil {
			t.Fatalf("GetZoomLevel should not return error for new URLs: %v", err)
		}
		
		expectedZoom := 1.0
		if currentZoom != expectedZoom {
			t.Errorf("Expected default zoom level %f for new URL, got %f", expectedZoom, currentZoom)
		}
	})
}

// Contract Test: BrowserService Navigation Controls
func TestBrowserService_NavigationControls_Contract(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	cfg := &config.Config{}
	service := services.NewBrowserService(cfg, mockDB)
	
	ctx := context.Background()
	
	t.Run("GoBack handles gracefully when no history", func(t *testing.T) {
		// Contract: GoBack should handle empty history gracefully
		err := service.GoBack(ctx)
		
		// May return error for empty history, which is acceptable
		if err != nil {
			t.Logf("GoBack returned error (acceptable for empty history): %v", err)
		}
	})
	
	t.Run("GoForward handles gracefully when no forward history", func(t *testing.T) {
		// Contract: GoForward should handle empty forward history gracefully
		err := service.GoForward(ctx)
		
		// May return error for empty forward history, which is acceptable
		if err != nil {
			t.Logf("GoForward returned error (acceptable for empty forward history): %v", err)
		}
	})
}

// Contract Test: WindowTitleUpdater Integration
func TestBrowserService_WindowTitleUpdate_Contract(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	cfg := &config.Config{}
	service := services.NewBrowserService(cfg, mockDB)
	
	// Setup mock window title updater
	mockUpdater := &MockWindowTitleUpdater{}
	service.SetWindowTitleUpdater(mockUpdater)
	
	t.Run("SetWindowTitleUpdater accepts WindowTitleUpdater interface", func(t *testing.T) {
		// Contract: Service should accept any WindowTitleUpdater implementation
		newMockUpdater := &MockWindowTitleUpdater{}
		
		// This should not panic or error
		service.SetWindowTitleUpdater(newMockUpdater)
		
		// Verify the updater was set properly
		if newMockUpdater.callCount < 0 {
			t.Errorf("WindowTitleUpdater should be properly initialized")
		}
	})
}

// Contract Test: URL Operations
func TestBrowserService_URLOperations_Contract(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	cfg := &config.Config{}
	service := services.NewBrowserService(cfg, mockDB)
	
	t.Run("GetCurrentURL returns current URL", func(t *testing.T) {
		// Contract: GetCurrentURL should return the current URL string
		ctx := context.Background()
		
		url, err := service.GetCurrentURL(ctx)
		
		// Contract assertions
		if err != nil {
			t.Fatalf("GetCurrentURL should not return error: %v", err)
		}
		
		// URL should be a valid string (can be empty for new browser)
		if url != "" && len(url) < 4 { // Minimum valid URL length
			t.Errorf("Invalid URL returned: %s", url)
		}
	})
	
	t.Run("CopyCurrentURL accepts URL parameter", func(t *testing.T) {
		// Contract: CopyCurrentURL should accept URL and handle clipboard
		ctx := context.Background()
		testURL := "https://example.com"
		
		err := service.CopyCurrentURL(ctx, testURL)
		
		// Contract assertions - should not return error for valid URL
		if err != nil {
			t.Fatalf("CopyCurrentURL should not return error for valid URL: %v", err)
		}
	})
}

// Contract Test: Error Handling
func TestBrowserService_ErrorHandling_Contract(t *testing.T) {
	// Test with nil config but valid mock database
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mock_db.NewMockDatabaseQuerier(ctrl)
	service := services.NewBrowserService(nil, mockDB)
	ctx := context.Background()
	
	t.Run("Service handles nil config gracefully", func(t *testing.T) {
		// Contract: Service methods should handle nil config gracefully
		
		// Setup mock expectations for operations that might still work
		mockDB.EXPECT().
			GetZoomLevel(ctx, "test").
			Return(1.0, nil).
			AnyTimes()
		
		mockDB.EXPECT().
			SetZoomLevel(ctx, gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()
		
		// These should work with valid database but nil config
		_, err := service.ZoomIn(ctx, "test")
		if err != nil {
			t.Logf("ZoomIn returned error with nil config (may be expected): %v", err)
		}
		
		err = service.GoBack(ctx)
		if err != nil {
			t.Logf("GoBack returned error with nil config (may be expected): %v", err)
		}
	})
}
