package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// NavigateUseCase handles URL navigation with history recording and zoom application.
type NavigateUseCase struct {
	historyRepo repository.HistoryRepository
	zoomRepo    repository.ZoomRepository
	defaultZoom float64
}

// NewNavigateUseCase creates a new navigation use case.
// defaultZoom is the zoom level to use when no per-domain zoom is saved (typically from config).
func NewNavigateUseCase(
	historyRepo repository.HistoryRepository,
	zoomRepo repository.ZoomRepository,
	defaultZoom float64,
) *NavigateUseCase {
	if defaultZoom <= 0 {
		defaultZoom = entity.ZoomDefault
	}
	return &NavigateUseCase{
		historyRepo: historyRepo,
		zoomRepo:    zoomRepo,
		defaultZoom: defaultZoom,
	}
}

// NavigateInput contains parameters for navigation.
type NavigateInput struct {
	URL     string
	PaneID  string
	WebView port.WebView
}

// NavigateOutput contains the result of navigation.
type NavigateOutput struct {
	AppliedZoom float64
}

// Execute navigates to a URL and records history.
// Zoom is applied later via LoadCommitted event in ContentCoordinator.
func (uc *NavigateUseCase) Execute(ctx context.Context, input NavigateInput) (*NavigateOutput, error) {
	log := logging.FromContext(ctx)
	log.Debug().
		Str("url", input.URL).
		Str("pane_id", input.PaneID).
		Msg("navigating to URL")

	// Navigate to URL (zoom will be applied on LoadCommitted to avoid shift)
	if err := input.WebView.LoadURI(ctx, input.URL); err != nil {
		return nil, fmt.Errorf("failed to load URL: %w", err)
	}

	// Record in history asynchronously
	go uc.recordHistory(ctx, input.URL)

	log.Info().
		Str("url", input.URL).
		Msg("navigation initiated")

	return &NavigateOutput{
		AppliedZoom: uc.defaultZoom,
	}, nil
}

// recordHistory saves or updates the history entry.
func (uc *NavigateUseCase) recordHistory(ctx context.Context, url string) {
	log := logging.FromContext(ctx)

	// Normalize URL to avoid duplicates (e.g., github.com vs github.com/)
	url = normalizeURLForHistory(url)

	// Check if entry exists
	existing, err := uc.historyRepo.FindByURL(ctx, url)
	if err != nil {
		log.Warn().Err(err).Str("url", url).Msg("failed to check history")
		return
	}

	if existing != nil {
		// Increment visit count
		if err := uc.historyRepo.IncrementVisitCount(ctx, url); err != nil {
			log.Warn().Err(err).Str("url", url).Msg("failed to increment visit count")
		}
	} else {
		// Create new entry
		entry := entity.NewHistoryEntry(url, "")
		if err := uc.historyRepo.Save(ctx, entry); err != nil {
			log.Warn().Err(err).Str("url", url).Msg("failed to save history")
		}
	}
}

// UpdateHistoryTitle updates the title of a history entry after page load.
func (uc *NavigateUseCase) UpdateHistoryTitle(ctx context.Context, url, title string) error {
	log := logging.FromContext(ctx)

	// Normalize URL to match storage (e.g., github.com/ -> github.com)
	url = normalizeURLForHistory(url)
	log.Debug().Str("url", url).Str("title", title).Msg("updating history title")

	entry, err := uc.historyRepo.FindByURL(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to find history entry: %w", err)
	}

	if entry == nil {
		// Entry doesn't exist, create it with title
		entry = entity.NewHistoryEntry(url, title)
		if err := uc.historyRepo.Save(ctx, entry); err != nil {
			return fmt.Errorf("failed to save history: %w", err)
		}
		return nil
	}

	// Update existing entry's title
	entry.Title = title
	if err := uc.historyRepo.Save(ctx, entry); err != nil {
		return fmt.Errorf("failed to update history title: %w", err)
	}

	return nil
}

// Reload reloads the current page.
func (uc *NavigateUseCase) Reload(ctx context.Context, webview port.WebView, bypassCache bool) error {
	log := logging.FromContext(ctx)
	log.Debug().Bool("bypass_cache", bypassCache).Msg("reloading page")

	if bypassCache {
		return webview.ReloadBypassCache(ctx)
	}
	return webview.Reload(ctx)
}

// GoBack navigates back in history.
func (uc *NavigateUseCase) GoBack(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx)

	if !webview.CanGoBack() {
		log.Debug().Msg("cannot go back - no history")
		return nil
	}

	log.Debug().Msg("navigating back")
	return webview.GoBack(ctx)
}

// GoForward navigates forward in history.
func (uc *NavigateUseCase) GoForward(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx)

	if !webview.CanGoForward() {
		log.Debug().Msg("cannot go forward - no history")
		return nil
	}

	log.Debug().Msg("navigating forward")
	return webview.GoForward(ctx)
}

// Stop stops the current page load.
func (uc *NavigateUseCase) Stop(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx)
	log.Debug().Msg("stopping page load")
	return webview.Stop(ctx)
}

// normalizeURLForHistory normalizes a URL for history storage.
// Strips trailing slash to avoid duplicates like github.com vs github.com/
func normalizeURLForHistory(url string) string {
	// Don't strip if URL is just the protocol + domain (e.g., "https://github.com")
	// Only strip trailing slash from paths
	if strings.HasSuffix(url, "/") {
		// Check if it's just domain with trailing slash (e.g., "https://github.com/")
		// by counting slashes - protocol has 2, domain adds 0, path adds more
		if strings.Count(url, "/") == 3 {
			// It's "https://domain.com/" - strip the trailing slash
			return strings.TrimSuffix(url, "/")
		}
		// For paths like "https://github.com/user/" - also strip
		return strings.TrimSuffix(url, "/")
	}
	return url
}
