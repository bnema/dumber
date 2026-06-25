package usecase

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
)

const (
	// historyQueueSize is the buffer size for the async history queue.
	// If the queue is full, new records are dropped with a warning.
	historyQueueSize = 100

	// historyWorkerFlushInterval coalesces bursts into fewer persistence writes.
	historyWorkerFlushInterval = 100 * time.Millisecond

	// historyWorkerShutdownFlushTimeout bounds final persistence attempts during shutdown.
	historyWorkerShutdownFlushTimeout = 2 * time.Second

	// historyWorkerShutdownWaitTimeout bounds Close even if a repository call ignores context cancellation.
	historyWorkerShutdownWaitTimeout = historyWorkerShutdownFlushTimeout + 500*time.Millisecond
)

// historyDeduplicationWindow is the time window for deduplicating history visits.
// Visits to the same URL within this window count as a single visit.
// This prevents inflation from redirects and rapid navigation.
const historyDeduplicationWindow = 2 * time.Second

// stripTrackingParamsForHistoryDedup controls whether known tracking parameters
// are removed while canonicalizing URLs for history deduplication/storage.
const stripTrackingParamsForHistoryDedup = true

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

type historyControlRequest struct {
	kind historyControlKind
	ctx  context.Context
	done chan error
}

type historyControlKind int

type titlePersistenceResult int

const historyControlFlushAndReset historyControlKind = iota

const (
	titlePersistenceRetry titlePersistenceResult = iota
	titlePersistenceSaved
	titlePersistenceDiscard
)

var (
	errHistoryRecorderClosing       = errors.New("history recorder closing")
	errHistoryPersistenceIncomplete = errors.New("history persistence incomplete")
)

type pendingHistoryRecords struct {
	visits map[string]int
	titles map[string]string
}

func newPendingHistoryRecords() *pendingHistoryRecords {
	return &pendingHistoryRecords{
		visits: make(map[string]int),
		titles: make(map[string]string),
	}
}

func (p *pendingHistoryRecords) add(record historyRecord) {
	if record.title != "" {
		p.titles[record.url] = record.title
	}
	if record.visits > 0 {
		p.visits[record.url] += record.visits
	}
}

func (p *pendingHistoryRecords) empty() bool {
	return len(p.visits) == 0 && len(p.titles) == 0
}

func (p *pendingHistoryRecords) reset() {
	p.visits = make(map[string]int)
	p.titles = make(map[string]string)
}

// HistoryRecorderUseCase records persisted history visits and title updates asynchronously.
type HistoryRecorderUseCase struct {
	historyRepo  repository.HistoryRepository
	changeSinkMu sync.RWMutex
	changeSink   port.HistoryChangeSink
	recentMu     sync.Mutex
	recentVisits map[string]paneHistoryState // key: pane ID, value: per-pane history state
	mutationMu   sync.Mutex
	closeOnce    sync.Once
	enqueueMu    sync.Mutex
	closing      bool

	historyQueue chan historyRecord
	controlQueue chan historyControlRequest
	done         chan struct{}
	wg           sync.WaitGroup
	ctx          context.Context // Base context for background worker
	cancel       context.CancelFunc
}

// NewHistoryRecorderUseCase creates a new history recorder use case.
func NewHistoryRecorderUseCase(historyRepo repository.HistoryRepository, changeSink port.HistoryChangeSink) *HistoryRecorderUseCase {
	changeSink = normalizeHistoryChangeSink(changeSink)
	ctx, cancel := context.WithCancel(context.Background())

	uc := &HistoryRecorderUseCase{
		historyRepo:  historyRepo,
		changeSink:   changeSink,
		recentVisits: make(map[string]paneHistoryState),
		historyQueue: make(chan historyRecord, historyQueueSize),
		controlQueue: make(chan historyControlRequest),
		done:         make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
	}

	uc.wg.Add(1)
	go uc.historyWorker()

	return uc
}

// SetHistoryChangeSink sets the sink for persisted history change notifications.
func (uc *HistoryRecorderUseCase) SetHistoryChangeSink(changeSink port.HistoryChangeSink) {
	if uc == nil {
		return
	}
	changeSink = normalizeHistoryChangeSink(changeSink)
	uc.changeSinkMu.Lock()
	uc.changeSink = changeSink
	uc.changeSinkMu.Unlock()
}

// Close shuts down the background history worker and drains any pending records.
func (uc *HistoryRecorderUseCase) Close() {
	if uc == nil {
		return
	}
	uc.closeOnce.Do(func() {
		uc.enqueueMu.Lock()
		uc.closing = true
		uc.enqueueMu.Unlock()
		uc.cancelWorkerContext()
		if uc.done != nil {
			close(uc.done)
		}
	})
	if uc.waitForHistoryWorker(historyWorkerShutdownWaitTimeout) {
		return
	}
	logging.FromContext(uc.workerContext()).Warn().
		Dur("timeout", historyWorkerShutdownWaitTimeout).
		Msg("history worker shutdown timed out")
}

func (uc *HistoryRecorderUseCase) workerContext() context.Context {
	if uc == nil || uc.ctx == nil {
		return context.Background()
	}
	return uc.ctx
}

func (uc *HistoryRecorderUseCase) cancelWorkerContext() {
	if uc != nil && uc.cancel != nil {
		uc.cancel()
	}
}

func (uc *HistoryRecorderUseCase) waitForHistoryWorker(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		uc.wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// RecordHistory queues a history entry for async recording.
// This is non-blocking to avoid SQLite I/O on the GTK main thread.
// Should be called on LoadCommitted when URI is guaranteed correct.
func (uc *HistoryRecorderUseCase) RecordHistory(ctx context.Context, paneID, rawURL string) {
	if uc == nil {
		return
	}
	log := logging.FromContext(ctx)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return
	}

	if !isRecordableHistoryURL(rawURL) {
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

	uc.mutationMu.Lock()
	defer uc.mutationMu.Unlock()

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
	enqueued := uc.enqueueHistoryRecord(historyRecord{url: canonicalURL, visits: 1})
	if enqueued {
		uc.recentVisits[paneID] = state
	}
	uc.recentMu.Unlock()

	if !enqueued {
		log.Warn().Str("url", logging.RedactURL(canonicalURL)).Msg("history queue full or recorder closed, dropping record")
	}
}

// ClearPaneHistory removes per-pane deduplication state to prevent unbounded growth.
// Callers should invoke this when a pane is closed.
func (uc *HistoryRecorderUseCase) ClearPaneHistory(paneID string) {
	if uc == nil {
		return
	}
	paneID = normalizePaneID(paneID)
	uc.mutationMu.Lock()
	defer uc.mutationMu.Unlock()
	uc.recentMu.Lock()
	delete(uc.recentVisits, paneID)
	uc.recentMu.Unlock()
}

// UpdateHistoryTitle queues a title update for a history entry. The actual
// DB write happens in the background historyWorker to avoid concurrent
// repo access from the GTK main thread and the worker goroutine.
func (uc *HistoryRecorderUseCase) UpdateHistoryTitle(_ context.Context, historyURL, title string) {
	if uc == nil {
		return
	}
	historyURL = strings.TrimSpace(historyURL)
	if historyURL == "" || !isRecordableHistoryURL(historyURL) {
		return
	}
	historyURL = canonicalizeURLForHistory(historyURL, historyCanonicalizationOptions{
		StripTrackingParams: stripTrackingParamsForHistoryDedup,
	})
	if historyURL == "" {
		return
	}

	uc.mutationMu.Lock()
	defer uc.mutationMu.Unlock()

	if !uc.enqueueHistoryRecord(historyRecord{url: historyURL, title: title}) {
		logging.FromContext(uc.workerContext()).Warn().
			Str("url", logging.RedactURL(historyURL)).
			Msg("history queue full or recorder closed, title update dropped")
	}
}

func (uc *HistoryRecorderUseCase) enqueueHistoryRecord(record historyRecord) bool {
	uc.enqueueMu.Lock()
	defer uc.enqueueMu.Unlock()
	if uc.closing {
		return false
	}
	select {
	case uc.historyQueue <- record:
		return true
	default:
		return false
	}
}

// historyWorker is a background goroutine that processes history records.
// It drains the queue and persists records to the database without blocking the UI.
func (uc *HistoryRecorderUseCase) historyWorker() {
	defer uc.wg.Done()

	ticker := time.NewTicker(historyWorkerFlushInterval)
	defer ticker.Stop()
	workerCtx := uc.workerContext()

	pending := newPendingHistoryRecords()
	for {
		select {
		case record := <-uc.historyQueue:
			pending.add(record)
		case <-ticker.C:
			_ = uc.flushPendingHistory(workerCtx, pending)
		case req := <-uc.controlQueue:
			uc.handleHistoryControl(pending, req)
		case <-uc.done:
			uc.shutdownHistoryWorker(pending)
			return
		case <-workerCtx.Done():
			uc.shutdownHistoryWorker(pending)
			return
		}
	}
}

func (uc *HistoryRecorderUseCase) flushPendingHistory(ctx context.Context, pending *pendingHistoryRecords) error {
	if ctx == nil {
		ctx = uc.workerContext()
	}
	if pending.empty() {
		return nil
	}

	visitCount := len(pending.visits)
	titleCount := len(pending.titles)
	logging.FromContext(ctx).Debug().
		Int("visits", visitCount).
		Int("titles", titleCount).
		Msg("flushing pending history")

	var flushErr error
	successfulVisits := 0
	for historyURL, visits := range pending.visits {
		if err := ctx.Err(); err != nil {
			uc.publishHistoryChange(successfulVisits, 0)
			return err
		}
		record := historyRecord{url: historyURL, visits: visits}
		writtenVisits := uc.persistHistory(ctx, record)
		if writtenVisits > 0 {
			successfulVisits += writtenVisits
			if writtenVisits >= visits {
				delete(pending.visits, historyURL)
			} else {
				pending.visits[historyURL] = visits - writtenVisits
			}
		}
		if writtenVisits < visits {
			flushErr = errHistoryPersistenceIncomplete
		}
	}

	successfulTitles := 0
	for historyURL, title := range pending.titles {
		if err := ctx.Err(); err != nil {
			uc.publishHistoryChange(successfulVisits, successfulTitles)
			return err
		}
		if _, visitStillPending := pending.visits[historyURL]; visitStillPending {
			flushErr = errHistoryPersistenceIncomplete
			continue
		}
		switch uc.persistTitleUpdate(ctx, historyURL, title) {
		case titlePersistenceSaved:
			successfulTitles++
			delete(pending.titles, historyURL)
		case titlePersistenceDiscard:
			delete(pending.titles, historyURL)
		case titlePersistenceRetry:
			flushErr = errHistoryPersistenceIncomplete
		}
	}
	uc.publishHistoryChange(successfulVisits, successfulTitles)
	if flushErr != nil {
		return flushErr
	}
	return ctx.Err()
}

func (uc *HistoryRecorderUseCase) handleHistoryControl(pending *pendingHistoryRecords, req historyControlRequest) {
	var err error
	switch req.kind {
	case historyControlFlushAndReset:
		uc.drainHistoryQueue(pending)
		err = uc.flushPendingHistory(req.ctx, pending)
		if err == nil {
			uc.resetRecentVisits()
		}
	}
	req.done <- err
	close(req.done)
}

func (uc *HistoryRecorderUseCase) drainHistoryQueue(pending *pendingHistoryRecords) {
	for {
		select {
		case record := <-uc.historyQueue:
			pending.add(record)
		default:
			return
		}
	}
}

func (uc *HistoryRecorderUseCase) shutdownHistoryWorker(pending *pendingHistoryRecords) {
	log := logging.FromContext(uc.workerContext()).With().
		Str("component", "history-worker").
		Logger()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), historyWorkerShutdownFlushTimeout)
	defer cancel()

	for {
		if err := shutdownCtx.Err(); err != nil {
			if !pending.empty() {
				log.Warn().Err(err).Msg("dropping unflushed history during shutdown after timeout")
				pending.reset()
			}
			return
		}
		log.Debug().Int("remaining", len(uc.historyQueue)).Msg("draining history queue")
		uc.drainHistoryQueue(pending)
		uc.flushPendingHistoryForShutdown(shutdownCtx, log, pending)

		uc.enqueueMu.Lock()
		uc.drainHistoryQueue(pending)
		if pending.empty() && len(uc.historyQueue) == 0 {
			uc.closing = true
			uc.enqueueMu.Unlock()
			log.Debug().Msg("history worker shutdown complete")
			return
		}
		uc.enqueueMu.Unlock()
	}
}

func (uc *HistoryRecorderUseCase) flushPendingHistoryForShutdown(ctx context.Context, log zerolog.Logger, pending *pendingHistoryRecords) {
	const shutdownFlushAttempts = 3
	for attempt := 1; attempt <= shutdownFlushAttempts; attempt++ {
		err := uc.flushPendingHistory(ctx, pending)
		if err == nil || pending.empty() {
			return
		}
		if attempt == shutdownFlushAttempts {
			log.Warn().Err(err).Msg("dropping unflushed history during shutdown after retries")
			pending.reset()
			return
		}
		log.Warn().Err(err).Int("attempt", attempt).Msg("retrying unflushed history during shutdown")
		select {
		case <-ctx.Done():
			log.Warn().Err(ctx.Err()).Msg("dropping unflushed history during shutdown after timeout")
			pending.reset()
			return
		case <-time.After(historyWorkerFlushInterval):
		}
	}
}

// BeginHistoryMutation flushes pending recorder writes and blocks new recorder enqueues until release is called.
func (uc *HistoryRecorderUseCase) BeginHistoryMutation(ctx context.Context) (func(), error) {
	if uc == nil {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	uc.mutationMu.Lock()
	if err := uc.flushAndResetHistory(ctx); err != nil {
		uc.mutationMu.Unlock()
		if errors.Is(err, errHistoryRecorderClosing) {
			uc.wg.Wait()
			uc.resetRecentVisits()
			return func() {}, nil
		}
		return nil, err
	}
	return func() { uc.mutationMu.Unlock() }, nil
}

func (uc *HistoryRecorderUseCase) flushAndResetHistory(ctx context.Context) error {
	req := historyControlRequest{
		kind: historyControlFlushAndReset,
		ctx:  ctx,
		done: make(chan error, 1),
	}
	select {
	case uc.controlQueue <- req:
		select {
		case err := <-req.done:
			return err
		case <-uc.done:
			return uc.waitForAcceptedFlushOrShutdown(ctx, req.done)
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-uc.done:
		return errHistoryRecorderClosing
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (uc *HistoryRecorderUseCase) waitForAcceptedFlushOrShutdown(ctx context.Context, reqDone <-chan error) error {
	wgDone := make(chan struct{})
	go func() {
		uc.wg.Wait()
		close(wgDone)
	}()

	select {
	case err := <-reqDone:
		return err
	case <-wgDone:
		uc.resetRecentVisits()
		return errHistoryRecorderClosing
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (uc *HistoryRecorderUseCase) resetRecentVisits() {
	uc.recentMu.Lock()
	uc.recentVisits = make(map[string]paneHistoryState)
	uc.recentMu.Unlock()
}

func (uc *HistoryRecorderUseCase) publishHistoryChange(successfulVisits, successfulTitles int) {
	if successfulVisits == 0 && successfulTitles == 0 {
		return
	}
	reasons := make([]dto.HistoryChangeReason, 0, 2)
	if successfulVisits > 0 {
		reasons = append(reasons, dto.HistoryChangeReasonVisit)
	}
	if successfulTitles > 0 {
		reasons = append(reasons, dto.HistoryChangeReasonTitle)
	}
	sink := uc.historyChangeSink()
	sink.OnHistoryChanged(uc.workerContext(), dto.HistoryChange{
		Reasons:    reasons,
		VisitCount: successfulVisits,
		TitleCount: successfulTitles,
		ChangedAt:  time.Now(),
	})
}

func (uc *HistoryRecorderUseCase) historyChangeSink() port.HistoryChangeSink {
	uc.changeSinkMu.RLock()
	sink := uc.changeSink
	uc.changeSinkMu.RUnlock()
	return normalizeHistoryChangeSink(sink)
}

// persistHistory writes a history record to the database.
// Called from the background worker goroutine.
func (uc *HistoryRecorderUseCase) persistHistory(ctx context.Context, record historyRecord) int {
	log := logging.FromContext(ctx)

	existing, err := uc.historyRepo.FindByURL(ctx, record.url)
	if err != nil {
		log.Warn().Err(err).Str("url", logging.RedactURL(record.url)).Msg("failed to check history")
		return 0
	}

	delta := max(1, record.visits)
	if existing != nil {
		if deltaWriter, ok := uc.historyRepo.(historyVisitDeltaIncrementer); ok {
			if err := deltaWriter.IncrementVisitCountBy(ctx, record.url, delta); err != nil {
				log.Warn().Err(err).Str("url", logging.RedactURL(record.url)).Int("delta", delta).Msg("failed to increment visit count by delta")
				return 0
			}
			return delta
		}
		written := 0
		for i := 0; i < delta; i++ {
			if err := uc.historyRepo.IncrementVisitCount(ctx, record.url); err != nil {
				log.Warn().Err(err).Str("url", logging.RedactURL(record.url)).Msg("failed to increment visit count")
				return written
			}
			written++
		}
		return written
	}

	entry := entity.NewHistoryEntry(record.url, "")
	entry.VisitCount = int64(delta)
	if err := uc.historyRepo.Save(ctx, entry); err != nil {
		log.Warn().Err(err).Str("url", logging.RedactURL(record.url)).Msg("failed to save history")
		return 0
	}
	return delta
}

// persistTitleUpdate writes a title update to the database.
// Called from the background worker goroutine.
func (uc *HistoryRecorderUseCase) persistTitleUpdate(ctx context.Context, historyURL, title string) titlePersistenceResult {
	log := logging.FromContext(ctx)

	if title == "" {
		return titlePersistenceDiscard
	}

	entry, err := uc.historyRepo.FindByURL(ctx, historyURL)
	if err != nil {
		log.Warn().Err(err).Str("url", logging.RedactURL(historyURL)).Msg("failed to find history entry for title update")
		return titlePersistenceRetry
	}
	if entry == nil {
		return titlePersistenceDiscard
	}

	entry.Title = title
	if updater, ok := uc.historyRepo.(historyMetadataUpdater); ok {
		if err := updater.UpdateMetadata(ctx, entry); err != nil {
			log.Warn().Err(err).Str("url", logging.RedactURL(historyURL)).Msg("failed to update history metadata")
			return titlePersistenceRetry
		}
		return titlePersistenceSaved
	}
	if err := uc.historyRepo.Save(ctx, entry); err != nil {
		log.Warn().Err(err).Str("url", logging.RedactURL(historyURL)).Msg("failed to update history title")
		return titlePersistenceRetry
	}
	return titlePersistenceSaved
}

func isRecordableHistoryURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
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
	parsed.Host = canonicalHistoryHost(parsed)
	parsed.User = nil
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

func canonicalHistoryHost(parsed *url.URL) string {
	host := strings.ToLower(parsed.Hostname())
	portValue := parsed.Port()
	if portValue == "" || (parsed.Scheme == "http" && portValue == "80") || (parsed.Scheme == "https" && portValue == "443") {
		if strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, portValue)
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

	if prevParsed.Fragment == currParsed.Fragment {
		return false
	}

	opts := historyCanonicalizationOptions{StripTrackingParams: stripTrackingParamsForHistoryDedup}
	prevCanonical := canonicalizeURLForHistory(previous, opts)
	currCanonical := canonicalizeURLForHistory(current, opts)
	return prevCanonical != "" && prevCanonical == currCanonical
}

func normalizePaneID(paneID string) string {
	if strings.TrimSpace(paneID) == "" {
		return "__default__"
	}
	return paneID
}
