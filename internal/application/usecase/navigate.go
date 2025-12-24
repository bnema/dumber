package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

const (
	// historyQueueSize is the buffer size for the async history queue.
	// If the queue is full, new records are dropped with a warning.
	historyQueueSize = 100
)

// historyRecord holds data for async history recording.
type historyRecord struct {
	url       string
	timestamp time.Time
}

// NavigateUseCase handles URL navigation with history recording and zoom application.
type NavigateUseCase struct {
	historyRepo  repository.HistoryRepository
	zoomRepo     repository.ZoomRepository
	defaultZoom  float64
	recentVisits sync.Map // key: url, value: time.Time - for deduplication

	// Async history recording
	historyQueue chan historyRecord
	done         chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context // Base context for background worker
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

	uc := &NavigateUseCase{
		historyRepo:  historyRepo,
		zoomRepo:     zoomRepo,
		defaultZoom:  defaultZoom,
		historyQueue: make(chan historyRecord, historyQueueSize),
		done:         make(chan struct{}),
		ctx:          context.Background(),
	}

	// Start background history worker
	uc.wg.Add(1)
	go uc.historyWorker()

	return uc
}

// Close shuts down the background history worker and drains any pending records.
// This should be called when the application is shutting down.
func (uc *NavigateUseCase) Close() {
	close(uc.done)
	uc.wg.Wait()
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
	// History is recorded on LoadCommitted when URI is guaranteed correct
	if err := input.WebView.LoadURI(ctx, input.URL); err != nil {
		return nil, fmt.Errorf("failed to load URL: %w", err)
	}

	log.Info().
		Str("url", input.URL).
		Msg("navigation initiated")

	return &NavigateOutput{
		AppliedZoom: uc.defaultZoom,
	}, nil
}

// historyDeduplicationWindow is the time window for deduplicating history visits.
// Visits to the same URL within this window count as a single visit.
// This prevents inflation from redirects and rapid navigation.
const historyDeduplicationWindow = 2 * time.Second

// RecordHistory queues a history entry for async recording.
// This is non-blocking to avoid SQLite I/O on the GTK main thread.
// Should be called on LoadCommitted when URI is guaranteed correct.
func (uc *NavigateUseCase) RecordHistory(ctx context.Context, url string) {
	log := logging.FromContext(ctx)

	// Normalize URL to avoid duplicates (e.g., github.com vs github.com/)
	url = normalizeURLForHistory(url)

	// Time-based deduplication: skip if same URL was recorded recently
	// This check is fast (sync.Map lookup) and stays on the main thread
	now := time.Now()
	if lastTime, ok := uc.recentVisits.Load(url); ok {
		if now.Sub(lastTime.(time.Time)) < historyDeduplicationWindow {
			log.Debug().Str("url", url).Msg("skipping duplicate history record within dedup window")
			return
		}
	}
	uc.recentVisits.Store(url, now)

	// Non-blocking send to async queue
	select {
	case uc.historyQueue <- historyRecord{url: url, timestamp: now}:
		log.Debug().Str("url", url).Msg("history record queued")
	default:
		// Queue full - log warning but don't block navigation
		log.Warn().Str("url", url).Msg("history queue full, dropping record")
	}
}

// historyWorker is a background goroutine that processes history records.
// It drains the queue and persists records to the database without blocking the UI.
func (uc *NavigateUseCase) historyWorker() {
	defer uc.wg.Done()

	log := logging.FromContext(uc.ctx).With().
		Str("component", "history-worker").
		Logger()

	for {
		select {
		case record := <-uc.historyQueue:
			uc.persistHistory(uc.ctx, record)
		case <-uc.done:
			// Drain remaining records before shutdown
			log.Debug().Int("remaining", len(uc.historyQueue)).Msg("draining history queue")
			for {
				select {
				case record := <-uc.historyQueue:
					uc.persistHistory(uc.ctx, record)
				default:
					log.Debug().Msg("history worker shutdown complete")
					return
				}
			}
		}
	}
}

// persistHistory writes a history record to the database.
// Called from the background worker goroutine.
func (uc *NavigateUseCase) persistHistory(ctx context.Context, record historyRecord) {
	log := logging.FromContext(ctx)

	// Check if entry exists
	existing, err := uc.historyRepo.FindByURL(ctx, record.url)
	if err != nil {
		log.Warn().Err(err).Str("url", record.url).Msg("failed to check history")
		return
	}

	if existing != nil {
		// Increment visit count
		if err := uc.historyRepo.IncrementVisitCount(ctx, record.url); err != nil {
			log.Warn().Err(err).Str("url", record.url).Msg("failed to increment visit count")
		}
	} else {
		// Create new entry
		entry := entity.NewHistoryEntry(record.url, "")
		if err := uc.historyRepo.Save(ctx, entry); err != nil {
			log.Warn().Err(err).Str("url", record.url).Msg("failed to save history")
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
		// Entry doesn't exist - don't create it here
		// Initial navigation should have already created the entry
		// This can happen if URL changed during page load (SPA, redirect)
		log.Debug().Str("url", url).Msg("no history entry found for URL, skipping title update")
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
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()
	log.Debug().Bool("bypass_cache", bypassCache).Msg("reloading page")

	if bypassCache {
		return webview.ReloadBypassCache(ctx)
	}
	return webview.Reload(ctx)
}

// GoBack navigates back in history.
func (uc *NavigateUseCase) GoBack(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()

	if !webview.CanGoBack() {
		log.Debug().Msg("cannot go back - no history")
		return nil
	}

	log.Debug().Msg("navigating back")
	return webview.GoBack(ctx)
}

// GoForward navigates forward in history.
func (uc *NavigateUseCase) GoForward(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()

	if !webview.CanGoForward() {
		log.Debug().Msg("cannot go forward - no history")
		return nil
	}

	log.Debug().Msg("navigating forward")
	return webview.GoForward(ctx)
}

// Stop stops the current page load.
func (uc *NavigateUseCase) Stop(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()
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
