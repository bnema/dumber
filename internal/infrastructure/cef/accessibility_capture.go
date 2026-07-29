package cef

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
)

const (
	accessibilityCaptureQueueCapacity = 64
	accessibilityCaptureDirPerm       = 0o700
	accessibilityCaptureFilePerm      = 0o600
	accessibilityCaptureEnvName       = "DUMBER_A11Y_CAPTURE"
	accessibilityCaptureEnvValue      = "1"
	accessibilityCaptureKindTree      = "tree"
	accessibilityCaptureKindLocation  = "location"
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
	mu  sync.Mutex
	dir string
	seq uint64
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
	return &accessibilityCapture{dir: dir}, nil
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	name := fmt.Sprintf("%06d-%s.json", c.seq, p.Kind)
	path := filepath.Join(c.dir, name)
	file, err := openFileNoFollow(path, syscall.O_CREAT|syscall.O_EXCL|syscall.O_WRONLY, accessibilityCaptureFilePerm)
	if err != nil {
		return err
	}
	_, writeErr := file.WriteString(p.JSON)
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

func (wv *WebView) shutdownAccessibilityCapture() {
	if wv == nil || wv.a11yWorker == nil {
		return
	}
	worker := wv.a11yWorker
	worker.Close()
	logAccessibilitySummary(wv.ctx, wv.id, wv.a11yStats.Snapshot(), worker.Dropped())
}
