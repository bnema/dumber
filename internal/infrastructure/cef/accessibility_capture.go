package cef

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
)

const (
	accessibilityCaptureQueueCapacity = 64
	accessibilityCaptureDirPerm       = 0o700
	accessibilityCaptureFilePerm      = 0o600
	accessibilityCaptureEnvName       = "DUMBER_A11Y_CAPTURE"
	accessibilityCaptureEnvValue      = "1"
	accessibilityCaptureKindTree      = "tree"
	accessibilityCaptureKindLocation  = "location"

	// Per-WebView capture budgets for the durable .../a11y-capture/webview-%d
	// namespace. Sized to cover an ~80s measurement campaign with headroom for
	// frequent tree+location updates, while capping unbounded growth when the
	// same WebView-ID directory is reused across sessions. Existing strict
	// capture files count toward both budgets; writes reject before create
	// when either limit would be exceeded. Tests may inject smaller limits.
	accessibilityCaptureDefaultMaxFiles = 10_000
	accessibilityCaptureDefaultMaxBytes = 512 << 20 // 512 MiB
)

type accessibilityStatsSnapshot struct {
	Count                 int
	TreeCount             int
	LocationCount         int
	TotalBytes            int64
	MinBytes              int
	MaxBytes              int
	AverageBytes          float64
	TotalSerializeNanos   int64
	MaxSerializeNanos     int64
	AverageSerializeNanos float64
}

type accessibilityStats struct {
	mu                  sync.Mutex
	count               int
	treeCount           int
	locationCount       int
	totalBytes          int64
	minBytes            int
	maxBytes            int
	totalSerializeNanos int64
	maxSerializeNanos   int64
}

func (s *accessibilityStats) Observe(p accessibilityPayload) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	switch p.Kind {
	case accessibilityCaptureKindTree:
		s.treeCount++
	case accessibilityCaptureKindLocation:
		s.locationCount++
	}
	s.totalBytes += int64(p.Bytes)
	if s.count == 1 || p.Bytes < s.minBytes {
		s.minBytes = p.Bytes
	}
	if p.Bytes > s.maxBytes {
		s.maxBytes = p.Bytes
	}
	s.totalSerializeNanos += p.SerializeNanos
	if p.SerializeNanos > s.maxSerializeNanos {
		s.maxSerializeNanos = p.SerializeNanos
	}
}

func (s *accessibilityStats) Snapshot() accessibilityStatsSnapshot {
	if s == nil {
		return accessibilityStatsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == 0 {
		return accessibilityStatsSnapshot{}
	}
	return accessibilityStatsSnapshot{
		Count:                 s.count,
		TreeCount:             s.treeCount,
		LocationCount:         s.locationCount,
		TotalBytes:            s.totalBytes,
		MinBytes:              s.minBytes,
		MaxBytes:              s.maxBytes,
		AverageBytes:          float64(s.totalBytes) / float64(s.count),
		TotalSerializeNanos:   s.totalSerializeNanos,
		MaxSerializeNanos:     s.maxSerializeNanos,
		AverageSerializeNanos: float64(s.totalSerializeNanos) / float64(s.count),
	}
}

type accessibilityCaptureWorker struct {
	mu      sync.RWMutex
	closed  bool
	queue   chan accessibilityPayload
	wg      sync.WaitGroup
	consume func(accessibilityPayload)
	dropped atomic.Uint64
}

func newAccessibilityCaptureWorker(capacity int, consume func(accessibilityPayload)) *accessibilityCaptureWorker {
	if capacity < 1 {
		capacity = 1
	}
	w := &accessibilityCaptureWorker{
		queue:   make(chan accessibilityPayload, capacity),
		consume: consume,
	}
	w.wg.Add(1)
	go w.loop()
	return w
}

func (w *accessibilityCaptureWorker) loop() {
	defer w.wg.Done()
	for p := range w.queue {
		if w.consume != nil {
			w.consume(p)
		}
	}
}

func (w *accessibilityCaptureWorker) Submit(p accessibilityPayload) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return false
	}
	select {
	case w.queue <- p:
		return true
	default:
		w.dropped.Add(1)
		return false
	}
}

func (w *accessibilityCaptureWorker) Dropped() uint64 {
	if w == nil {
		return 0
	}
	return w.dropped.Load()
}

func (w *accessibilityCaptureWorker) Close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	close(w.queue)
	w.mu.Unlock()
	w.wg.Wait()
}

type accessibilityCapture struct {
	mu         sync.Mutex
	dir        string
	seq        uint64
	fileCount  int
	totalBytes int64
	maxFiles   int
	maxBytes   int64
}

func accessibilityCaptureEnabled() bool {
	return os.Getenv(accessibilityCaptureEnvName) == accessibilityCaptureEnvValue
}

func accessibilityCaptureDir(webViewID port.WebViewID) (string, error) {
	logDir, err := config.GetLogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(logDir, "a11y-capture", fmt.Sprintf("webview-%d", webViewID)), nil
}

func validateAccessibilityCaptureDir(dir string) error {
	if dir == "" || dir == "." {
		return fmt.Errorf("accessibility capture dir rejected: %q", dir)
	}
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		if part == ".." {
			return fmt.Errorf("accessibility capture dir rejected: %q", dir)
		}
	}
	return nil
}

// parseAccessibilityCaptureName accepts only strict numbered capture names
// produced by Write: NNNNNN-(tree|location).json with a non-zero sequence.
func parseAccessibilityCaptureName(name string) (seq uint64, ok bool) {
	if !strings.HasSuffix(name, ".json") {
		return 0, false
	}
	base := strings.TrimSuffix(name, ".json")
	seqPart, kind, cut := strings.Cut(base, "-")
	if !cut || len(seqPart) != 6 || !accessibilityCaptureKindAllowed(kind) {
		return 0, false
	}
	for _, r := range seqPart {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(seqPart, 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return n, true
}

// scanAccessibilityCaptureDir accounts only strict regular capture files.
// Symlinks and non-regular entries are ignored (never followed). Gaps are
// allowed; the returned sequence is the maximum numbered name present.
func scanAccessibilityCaptureDir(dir string) (maxSeq uint64, fileCount int, totalBytes int64, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read accessibility capture dir: %w", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		seq, ok := parseAccessibilityCaptureName(name)
		if !ok {
			continue
		}
		path := filepath.Join(dir, name)
		info, lstatErr := os.Lstat(path)
		if lstatErr != nil {
			return 0, 0, 0, fmt.Errorf("lstat accessibility capture %s: %w", name, lstatErr)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		fileCount++
		totalBytes += info.Size()
		if seq > maxSeq {
			maxSeq = seq
		}
	}
	return maxSeq, fileCount, totalBytes, nil
}

func newAccessibilityCapture(dir string) (*accessibilityCapture, error) {
	if err := validateAccessibilityCaptureDir(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, accessibilityCaptureDirPerm); err != nil {
		return nil, fmt.Errorf("mkdir accessibility capture dir: %w", err)
	}
	if err := os.Chmod(dir, accessibilityCaptureDirPerm); err != nil {
		return nil, fmt.Errorf("chmod accessibility capture dir: %w", err)
	}
	maxSeq, fileCount, totalBytes, err := scanAccessibilityCaptureDir(dir)
	if err != nil {
		return nil, err
	}
	return &accessibilityCapture{
		dir:        dir,
		seq:        maxSeq,
		fileCount:  fileCount,
		totalBytes: totalBytes,
		maxFiles:   accessibilityCaptureDefaultMaxFiles,
		maxBytes:   accessibilityCaptureDefaultMaxBytes,
	}, nil
}

func accessibilityCaptureKindAllowed(kind string) bool {
	switch kind {
	case accessibilityCaptureKindTree, accessibilityCaptureKindLocation:
		return true
	default:
		return false
	}
}

func (c *accessibilityCapture) Write(p accessibilityPayload) error {
	if c == nil || p.JSON == "" {
		return nil
	}
	if !accessibilityCaptureKindAllowed(p.Kind) {
		return fmt.Errorf("accessibility capture kind rejected: %q", p.Kind)
	}
	payloadBytes := int64(len(p.JSON))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fileCount >= c.maxFiles {
		return fmt.Errorf("accessibility capture file budget exceeded (%d files)", c.maxFiles)
	}
	if c.totalBytes+payloadBytes > c.maxBytes {
		return fmt.Errorf("accessibility capture byte budget exceeded (%d bytes)", c.maxBytes)
	}
	next := c.seq + 1
	name := fmt.Sprintf("%06d-%s.json", next, p.Kind)
	path := filepath.Join(c.dir, name)
	file, err := openFileNoFollow(path, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY, accessibilityCaptureFilePerm)
	if err != nil {
		return err
	}
	c.seq = next
	c.fileCount++
	written, writeErr := file.WriteString(p.JSON)
	c.totalBytes += int64(written)
	chmodErr := file.Chmod(accessibilityCaptureFilePerm)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if chmodErr != nil {
		return chmodErr
	}
	return closeErr
}

func (wv *WebView) enableAccessibilityCaptureIfRequested() error {
	if wv == nil || !accessibilityCaptureEnabled() {
		return nil
	}
	dir, err := accessibilityCaptureDir(wv.id)
	if err != nil {
		return err
	}
	capture, err := newAccessibilityCapture(dir)
	if err != nil {
		return err
	}
	wv.a11yCapture = capture
	wv.a11yWorker = newAccessibilityCaptureWorker(accessibilityCaptureQueueCapacity, wv.consumeAccessibilityPayload)
	logAccessibilityCaptureEnabled(wv.ctx, wv.id, dir)
	return nil
}

func (wv *WebView) consumeAccessibilityPayload(p accessibilityPayload) {
	if wv == nil {
		return
	}
	wv.a11yStats.Observe(p)
	logAccessibilityUpdate(wv.ctx, wv.id, p)
	if wv.a11yCapture == nil {
		return
	}
	if err := wv.a11yCapture.Write(p); err != nil {
		logging.FromContext(wv.ctx).Warn().Err(err).
			Uint64("webview_id", uint64(wv.id)).
			Msg("cef: accessibility capture write failed")
	}
}

// abortAccessibilityCaptureSetup closes a capture worker started during
// newWebView without emitting Destroy's summary. Safe to call when unset.
func (wv *WebView) abortAccessibilityCaptureSetup() {
	wv.finalizeAccessibilityCapture(false)
}

// failNewWebViewAfterAccessibilityCapture centralizes cleanup for every
// newWebView error return after enableAccessibilityCaptureIfRequested may have
// started the worker. Closes the worker exactly once without Destroy summary.
func (wv *WebView) failNewWebViewAfterAccessibilityCapture(
	operation string,
	result syncdispatch.SyncDispatchResult,
) error {
	wv.abortAccessibilityCaptureSetup()
	wv.destroyViewBridgeOnGTKAsync()
	return errGTKSyncDispatchIncomplete(operation, result)
}

// newWebViewInstallViewportSyncHooks is the viewport-hook install used by
// newWebView. Tests replace it to force incomplete GTK dispatch after capture
// init without a live display.
var newWebViewInstallViewportSyncHooks = func(wv *WebView) syncdispatch.SyncDispatchResult {
	return wv.installViewportSyncHooks()
}

// installNewWebViewViewportHooksAfterCapture installs viewport sync hooks and
// aborts capture setup if GTK dispatch does not complete.
func (wv *WebView) installNewWebViewViewportHooksAfterCapture() error {
	result := newWebViewInstallViewportSyncHooks(wv)
	if result.Completed() {
		return nil
	}
	return wv.failNewWebViewAfterAccessibilityCapture("install CEF viewport sync hooks", result)
}

func (wv *WebView) shutdownAccessibilityCapture() {
	wv.finalizeAccessibilityCapture(true)
}

// finalizeAccessibilityCapture claims one-shot ownership of worker close.
// Pointers stay published so concurrent enqueueAccessibilityPayload reads are
// race-free; Submit observes the closed worker and returns false.
func (wv *WebView) finalizeAccessibilityCapture(emitSummary bool) {
	if wv == nil {
		return
	}
	if !wv.a11yCaptureFinalized.CompareAndSwap(false, true) {
		return
	}
	worker := wv.a11yWorker
	if worker == nil {
		return
	}
	worker.Close()
	if emitSummary {
		logAccessibilitySummary(wv.ctx, wv.id, wv.a11yStats.Snapshot(), worker.Dropped())
	}
}
