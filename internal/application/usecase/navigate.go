package usecase

import (
	"context"
	"fmt"
	"net/url"
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

	// logURLMaxLen is the max length for URLs in log messages.
	logURLMaxLen = 60

	// historyWorkerFlushInterval coalesces bursts into fewer persistence writes.
	historyWorkerFlushInterval = 100 * time.Millisecond

	// historyFallbackIncrementCap bounds DB writes when delta-increment API is unavailable.
	historyFallbackIncrementCap = 256
)

// historyRecord holds data for async history recording.
type historyRecord struct {
	url    string
	title  string // non-empty for title-update-only records
	visits int
}

type historyVisitDeltaIncrementer interface {
	IncrementVisitCountBy(ctx context.Context, url string, delta int) error
}

type historyMetadataUpdater interface {
	UpdateMetadata(ctx context.Context, entry *entity.HistoryEntry) error
}

type paneHistoryState struct {
	lastRawURL       string
	lastCanonicalURL string
	lastRecordedAt   time.Time
}

type historyCanonicalizationOptions struct {
	StripTrackingParams bool
}

// NavigateUseCase handles URL navigation with history recording and zoom application.
type NavigateUseCase struct {
	historyRepo  repository.HistoryRepository
	zoomRepo     repository.ZoomRepository
	defaultZoom  float64
	recentMu     sync.Mutex
	recentVisits map[string]paneHistoryState // key: pane ID, value: per-pane history state
	closeOnce    sync.Once

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
		recentVisits: make(map[string]paneHistoryState),
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
	uc.closeOnce.Do(func() {
		close(uc.done)
	})
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

// stripTrackingParamsForHistoryDedup controls whether known tracking parameters
// are removed while canonicalizing URLs for history deduplication/storage.
const stripTrackingParamsForHistoryDedup = true

// RecordHistory queues a history entry for async recording.
// This is non-blocking to avoid SQLite I/O on the GTK main thread.
// Should be called on LoadCommitted when URI is guaranteed correct.
func (uc *NavigateUseCase) RecordHistory(ctx context.Context, paneID, rawURL string) {
	log := logging.FromContext(ctx)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}

	canonicalURL := canonicalizeURLForHistory(rawURL, historyCanonicalizationOptions{
		StripTrackingParams: stripTrackingParamsForHistoryDedup,
	})
	if canonicalURL == "" {
		return
	}

	paneID = normalizePaneID(paneID)

	now := time.Now()

	uc.recentMu.Lock()
	state := uc.recentVisits[paneID]

	// Ignore in-page hash transitions for history persistence.
	if isHashOnlyTransition(state.lastRawURL, rawURL) {
		state.lastRawURL = rawURL
		uc.recentVisits[paneID] = state
		uc.recentMu.Unlock()
		return
	}

	// Per-pane short-window deduplication by canonical URL.
	if state.lastCanonicalURL == canonicalURL && now.Sub(state.lastRecordedAt) < historyDeduplicationWindow {
		state.lastRawURL = rawURL
		uc.recentVisits[paneID] = state
		uc.recentMu.Unlock()
		return
	}

	state.lastRawURL = rawURL
	state.lastCanonicalURL = canonicalURL
	state.lastRecordedAt = now
	uc.recentVisits[paneID] = state
	uc.recentMu.Unlock()

	// Non-blocking send to async queue
	select {
	case uc.historyQueue <- historyRecord{url: canonicalURL, visits: 1}:
	default:
		// Queue full - log warning but don't block navigation
		log.Warn().Str("url", logging.TruncateURL(canonicalURL, logURLMaxLen)).Msg("history queue full, dropping record")
	}
}

// ClearPaneHistory removes per-pane deduplication state to prevent unbounded growth.
// Callers should invoke this when a pane is closed.
func (uc *NavigateUseCase) ClearPaneHistory(paneID string) {
	paneID = normalizePaneID(paneID)
	uc.recentMu.Lock()
	delete(uc.recentVisits, paneID)
	uc.recentMu.Unlock()
}

// historyWorker is a background goroutine that processes history records.
// It drains the queue and persists records to the database without blocking the UI.
func (uc *NavigateUseCase) historyWorker() {
	defer uc.wg.Done()

	log := logging.FromContext(uc.ctx).With().
		Str("component", "history-worker").
		Logger()

	ticker := time.NewTicker(historyWorkerFlushInterval)
	defer ticker.Stop()

	pendingVisits := make(map[string]int)
	pendingTitles := make(map[string]string)

	processRecord := func(record historyRecord) {
		if record.title != "" {
			pendingTitles[record.url] = record.title
		}
		if record.visits > 0 {
			pendingVisits[record.url] += record.visits
		}
	}

	flushPending := func() {
		visitCount := len(pendingVisits)
		titleCount := len(pendingTitles)
		if visitCount == 0 && titleCount == 0 {
			return
		}

		log.Debug().Int("visits", visitCount).Int("titles", titleCount).Msg("flushing pending history")

		// Swap maps to avoid concurrent iteration/write issues
		// (persist callbacks may re-enqueue records).
		visits := pendingVisits
		titles := pendingTitles
		pendingVisits = make(map[string]int)
		pendingTitles = make(map[string]string)

		for historyURL, v := range visits {
			uc.persistHistory(uc.ctx, historyRecord{url: historyURL, visits: v})
		}
		for historyURL, title := range titles {
			uc.persistTitleUpdate(uc.ctx, historyURL, title)
		}
	}

	drainQueue := func() {
		for {
			select {
			case record := <-uc.historyQueue:
				processRecord(record)
			default:
				return
			}
		}
	}

	for {
		select {
		case record := <-uc.historyQueue:
			processRecord(record)
		case <-ticker.C:
			flushPending()
		case <-uc.done:
			log.Debug().Int("remaining", len(uc.historyQueue)).Msg("draining history queue")
			drainQueue()
			flushPending()
			log.Debug().Msg("history worker shutdown complete")
			return
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
		delta := max(1, record.visits)
		if deltaWriter, ok := uc.historyRepo.(historyVisitDeltaIncrementer); ok {
			if err := deltaWriter.IncrementVisitCountBy(ctx, record.url, delta); err != nil {
				log.Warn().Err(err).Str("url", record.url).Int("delta", delta).Msg("failed to increment visit count by delta")
				return
			}
			return
		}
		iterations := delta
		if iterations > historyFallbackIncrementCap {
			log.Warn().
				Str("url", record.url).
				Int("delta", delta).
				Int("cap", historyFallbackIncrementCap).
				Msg("visit count fallback increment capped")
			iterations = historyFallbackIncrementCap
		}
		for i := 0; i < iterations; i++ {
			if err := uc.historyRepo.IncrementVisitCount(ctx, record.url); err != nil {
				log.Warn().Err(err).Str("url", record.url).Msg("failed to increment visit count")
				return
			}
		}
	} else {
		// Create new entry
		entry := entity.NewHistoryEntry(record.url, "")
		entry.VisitCount = int64(max(1, record.visits))
		if err := uc.historyRepo.Save(ctx, entry); err != nil {
			log.Warn().Err(err).Str("url", record.url).Msg("failed to save history")
		}
	}
}

// persistTitleUpdate writes a title update to the database.
// Called from the background worker goroutine.
func (uc *NavigateUseCase) persistTitleUpdate(ctx context.Context, historyURL, title string) {
	log := logging.FromContext(ctx)

	if title == "" {
		return
	}

	entry, err := uc.historyRepo.FindByURL(ctx, historyURL)
	if err != nil {
		log.Warn().Err(err).Str("url", historyURL).Msg("failed to find history entry for title update")
		return
	}
	if entry == nil {
		return
	}

	entry.Title = title
	if updater, ok := uc.historyRepo.(historyMetadataUpdater); ok {
		if err := updater.UpdateMetadata(ctx, entry); err != nil {
			log.Warn().Err(err).Str("url", historyURL).Msg("failed to update history metadata")
		}
		return
	}
	if err := uc.historyRepo.Save(ctx, entry); err != nil {
		log.Warn().Err(err).Str("url", historyURL).Msg("failed to update history title")
	}
}

// UpdateHistoryTitle queues a title update for a history entry. The actual
// DB write happens in the background historyWorker to avoid concurrent
// repo access from the GTK main thread and the worker goroutine.
func (uc *NavigateUseCase) UpdateHistoryTitle(_ context.Context, historyURL, title string) {
	historyURL = canonicalizeURLForHistory(historyURL, historyCanonicalizationOptions{
		StripTrackingParams: stripTrackingParamsForHistoryDedup,
	})
	if historyURL == "" {
		return
	}

	// Non-blocking send — same queue as visit records.
	select {
	case uc.historyQueue <- historyRecord{url: historyURL, title: title}:
	default:
		logging.FromContext(uc.ctx).Warn().Str("url", historyURL).Msg("history queue full, title update dropped")
	}
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

// Stop stops the current page load.
func (uc *NavigateUseCase) Stop(ctx context.Context, webview port.WebView) error {
	log := logging.FromContext(ctx).With().Float64("default_zoom", uc.defaultZoom).Logger()
	log.Debug().Msg("stopping page load")
	return webview.Stop(ctx)
}

func canonicalizeURLForHistory(raw string, opts historyCanonicalizationOptions) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimSuffix(raw, "/")
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Fragment = ""
	parsed.Path = normalizePathForHistory(parsed.Path)

	query := parsed.Query()
	if opts.StripTrackingParams {
		for key := range query {
			if isTrackingQueryParam(key) {
				query.Del(key)
			}
		}
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func normalizePathForHistory(path string) string {
	if path == "/" {
		return ""
	}
	if strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/")
	}
	return path
}

func isTrackingQueryParam(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	if strings.HasPrefix(key, "utm_") {
		return true
	}
	switch key {
	case "fbclid", "gclid", "msclkid", "dclid", "yclid", "mc_cid", "mc_eid", "_hsenc", "_hsmi", "igshid":
		return true
	}
	return false
}

func isHashOnlyTransition(previous, current string) bool {
	if previous == "" || current == "" || previous == current {
		return false
	}

	prevParsed, prevErr := url.Parse(previous)
	currParsed, currErr := url.Parse(current)
	if prevErr != nil || currErr != nil {
		return false
	}

	if !strings.EqualFold(prevParsed.Scheme, currParsed.Scheme) {
		return false
	}
	if !strings.EqualFold(prevParsed.Host, currParsed.Host) {
		return false
	}
	if normalizePathForHistory(prevParsed.Path) != normalizePathForHistory(currParsed.Path) {
		return false
	}
	if prevParsed.RawQuery != currParsed.RawQuery {
		return false
	}
	return prevParsed.Fragment != currParsed.Fragment
}

func normalizePaneID(paneID string) string {
	if strings.TrimSpace(paneID) == "" {
		return "__default__"
	}
	return paneID
}
