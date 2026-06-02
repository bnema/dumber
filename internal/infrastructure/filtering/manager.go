package filtering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Compile-time check that Manager satisfies port.FilterManager.
var _ port.FilterManager = (*Manager)(nil)

const (
	jsonDirPerm    = 0o755
	quarantinePerm = 0o755

	// CacheMaxAge is the maximum age of the filter cache before it's considered stale.
	CacheMaxAge = 24 * time.Hour
)

// ErrUpdateSkipped indicates a filter update was intentionally skipped after a recoverable
// update-path failure (e.g. invalid payload, merge failure, compile failure).
var ErrUpdateSkipped = errors.New("filter update skipped")

// Manager orchestrates the content filter lifecycle.
// It handles downloading, cache policy, validation, status updates, and delegates
// engine-specific activation to a backend.
type Manager struct {
	backend    Backend
	downloader FilterDownloader

	jsonDir string

	status atomic.Value // FilterStatus

	mu             sync.RWMutex
	enabled        bool
	autoUpdate     bool
	onStatusChange func(FilterStatus) // callback for status updates (e.g., for toast notifications)
}

// ManagerConfig holds configuration for the filter manager.
type ManagerConfig struct {
	JSONDir    string // Where to cache downloaded JSON files
	Enabled    bool   // Whether filtering is enabled
	AutoUpdate bool   // Whether to auto-update filters

	Backend    Backend          // Engine-specific filter activator
	Downloader FilterDownloader // If nil, creates default Downloader
}

// NewManager creates a new filter Manager.
func NewManager(cfg ManagerConfig) (*Manager, error) {
	if cfg.Backend == nil {
		return nil, fmt.Errorf("filter backend is required")
	}
	if err := os.MkdirAll(cfg.JSONDir, jsonDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create json dir: %w", err)
	}

	downloader := cfg.Downloader
	if downloader == nil {
		downloader = NewDownloader(cfg.JSONDir)
	}

	m := &Manager{
		backend:    cfg.Backend,
		downloader: downloader,
		jsonDir:    cfg.JSONDir,
		enabled:    cfg.Enabled,
		autoUpdate: cfg.AutoUpdate,
	}

	m.setStatus(FilterStatus{State: StateUninitialized})
	return m, nil
}

// SetStatusCallback sets a callback for status changes (e.g., for toast notifications).
func (m *Manager) SetStatusCallback(cb func(FilterStatus)) {
	m.mu.Lock()
	m.onStatusChange = cb
	m.mu.Unlock()
}

// setStatus updates the current status and notifies callback.
func (m *Manager) setStatus(status FilterStatus) {
	m.status.Store(status)
	m.mu.RLock()
	cb := m.onStatusChange
	m.mu.RUnlock()
	if cb != nil {
		cb(status)
	}
}

// Status returns the current filter status.
func (m *Manager) Status() FilterStatus {
	if s, ok := m.status.Load().(FilterStatus); ok {
		return s
	}
	return FilterStatus{State: StateUninitialized}
}

// Initialize performs fast initialization.
// This is called BEFORE GTK starts, so engine backends should only set up state.
// Actual filter loading happens in LoadAsync after the UI is running.
func (m *Manager) Initialize(ctx context.Context) error {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if !m.IsEnabled() {
		log.Info().Msg("content filtering disabled")
		m.setStatus(FilterStatus{State: StateDisabled})
		return nil
	}

	log.Debug().Msg("filter manager initialized (filters will load after UI startup)")
	m.setStatus(FilterStatus{State: StateUninitialized, Message: "Filters pending"})
	return nil
}

// LoadAsync loads or downloads filters in the background.
// First tries to load from backend cache, then local JSON files, then download.
func (m *Manager) LoadAsync(ctx context.Context) {
	if !m.IsEnabled() {
		return
	}

	go m.loadAsyncWorker(ctx)
}

func (m *Manager) loadAsyncWorker(ctx context.Context) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if m.backend.HasActive() {
		log.Debug().Msg("filters already loaded, skipping async load")
		return
	}

	if m.loadFromCache(ctx) {
		m.checkStaleCacheAndUpdate(ctx)
		return
	}

	// If local JSON files exist on disk, activate from them directly instead of
	// downloading. This respects auto_update=false on cold start with an empty
	// backend cache and allows testing with patched local JSON.
	if paths := m.downloader.GetCachedFilterPaths(); len(paths) > 0 {
		log.Info().Int("files", len(paths)).Msg("activating filters from local JSON (skipping download)")
		if err := m.activateFiles(ctx, paths, "Filters active"); err == nil {
			log.Info().Str("version", m.getCachedVersion()).Int("parts", len(paths)).Msg("filters activated from local JSON")
			return
		} else {
			log.Warn().Err(err).Msg("failed to activate local JSON, falling back to download")
		}
	}

	m.downloadCompileAndActivate(ctx)
}

func (m *Manager) loadFromCache(ctx context.Context) bool {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.setStatus(FilterStatus{State: StateLoading, Message: "Loading filters..."})

	loaded, err := m.backend.ActivateCached(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to activate cached filters, will download")
		return false
	}
	if !loaded {
		return false
	}

	version := m.setActiveStatus("Filters active")
	log.Info().Str("version", version).Msg("filters loaded from backend cache")
	return true
}

func (m *Manager) downloadCompileAndActivate(ctx context.Context) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	paths, err := m.downloadFiltersWithProgress(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to download filters")
		m.setStatus(FilterStatus{State: StateError, Message: "Download failed"})
		return
	}

	if validateErr := m.validateDownloadedFiles(ctx, paths); validateErr != nil {
		log.Error().Err(validateErr).Msg("downloaded filter validation failed")
		m.setStatus(FilterStatus{State: StateError, Message: "Invalid downloaded filters"})
		return
	}

	if err := m.activateFiles(ctx, paths, "Filters active"); err != nil {
		log.Error().Err(err).Msg("failed to activate filters")
		m.handleActivationFailure()
		return
	}

	log.Info().Str("version", m.getCachedVersion()).Int("parts", len(paths)).Msg("filters compiled and active")
}

func (m *Manager) downloadFiltersWithProgress(ctx context.Context) ([]string, error) {
	m.setStatus(FilterStatus{State: StateLoading, Message: "Downloading filters..."})
	return m.downloader.DownloadFilters(ctx, func(p DownloadProgress) {
		msg := fmt.Sprintf("Downloading filters (%d/%d)...", p.Current, p.Total)
		m.setStatus(FilterStatus{State: StateLoading, Message: msg})
	})
}

func (m *Manager) activateFiles(ctx context.Context, paths []string, message string) error {
	m.setStatus(FilterStatus{State: StateLoading, Message: "Compiling filters..."})
	if err := m.backend.ActivateFiles(ctx, paths); err != nil {
		return err
	}
	m.setActiveStatus(message)
	return nil
}

func (m *Manager) handleActivationFailure() {
	if m.backend.HasActive() {
		m.setStatus(FilterStatus{
			State:   StateActive,
			Message: "Filters active (update skipped)",
			Version: m.getCachedVersion(),
		})
		return
	}
	m.setStatus(FilterStatus{State: StateError, Message: "Compilation failed"})
}

func (m *Manager) setActiveStatus(message string) string {
	version := m.getCachedVersion()
	m.setStatus(FilterStatus{
		State:   StateActive,
		Message: message,
		Version: version,
	})
	return version
}

// checkStaleCacheAndUpdate checks if the cache is stale and triggers a background update.
func (m *Manager) checkStaleCacheAndUpdate(ctx context.Context) {
	if !m.downloader.IsCacheStale(CacheMaxAge) {
		return
	}
	log := logging.FromContext(ctx)
	log.Info().Msg("filter cache is stale, checking for updates in background")
	go func() {
		if err := m.CheckForUpdates(ctx); err != nil {
			log.Warn().Err(err).Msg("background filter update check failed")
		}
	}()
}

// getCachedVersion returns the version from the cached manifest.
func (m *Manager) getCachedVersion() string {
	manifest, err := m.downloader.GetCachedManifest()
	if err != nil || manifest == nil {
		return "unknown"
	}
	return manifest.Version
}

// CheckForUpdates checks if newer filters are available and downloads them.
// This should be called periodically in the background.
func (m *Manager) CheckForUpdates(ctx context.Context) error {
	if !m.shouldAutoUpdate() {
		return nil
	}

	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	needsUpdate, err := m.downloader.NeedsUpdate(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("failed to check for filter updates")
		return err
	}

	if !needsUpdate {
		log.Debug().Msg("filters are up to date")
		return nil
	}

	log.Info().Msg("filter update available, downloading")

	paths, err := m.downloader.DownloadFilters(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to download filter update: %w", err)
	}
	if validateErr := m.validateDownloadedFiles(ctx, paths); validateErr != nil {
		log.Warn().Err(validateErr).Msg("invalid downloaded filters, keeping existing active filter")
		return fmt.Errorf("%w: validation failed: %w", ErrUpdateSkipped, validateErr)
	}

	if err := m.activateFiles(ctx, paths, "Filters updated"); err != nil {
		log.Warn().Err(err).Msg("failed to activate updated filters, keeping existing active filter")
		if m.backend.HasActive() {
			m.setStatus(FilterStatus{
				State:   StateActive,
				Message: "Filters active (update skipped)",
				Version: m.getCachedVersion(),
			})
		} else {
			m.setStatus(FilterStatus{State: StateError, Message: "Compilation failed"})
		}
		return fmt.Errorf("%w: compile failed: %w", ErrUpdateSkipped, err)
	}

	log.Info().Str("version", m.getCachedVersion()).Int("parts", len(paths)).Msg("filters updated successfully")
	return nil
}

func (m *Manager) validateDownloadedFiles(ctx context.Context, paths []string) error {
	var validationErrs []error
	var quarantineErrs []error

	for _, path := range paths {
		if err := validateRuleFile(path); err != nil {
			if quarantineErr := m.quarantineInvalidFile(ctx, path, "rule_validation_failed"); quarantineErr != nil {
				quarantineErrs = append(quarantineErrs, fmt.Errorf("file=%s: %w", path, quarantineErr))
			}
			validationErrs = append(validationErrs, fmt.Errorf("file=%s: %w", path, err))
		}
	}

	if len(validationErrs) == 0 && len(quarantineErrs) == 0 {
		return nil
	}

	var parts []string
	if len(validationErrs) > 0 {
		parts = append(parts, fmt.Sprintf("%d invalid file(s)", len(validationErrs)))
	}
	if len(quarantineErrs) > 0 {
		parts = append(parts, fmt.Sprintf("%d quarantine failure(s)", len(quarantineErrs)))
	}

	return fmt.Errorf(
		"downloaded filter validation failed (%s): %w",
		strings.Join(parts, ", "),
		errors.Join(append(validationErrs, quarantineErrs...)...),
	)
}

func validateRuleFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	var rules []map[string]json.RawMessage
	if err := json.Unmarshal(data, &rules); err != nil {
		return fmt.Errorf("json parse failed: %w", err)
	}
	if len(rules) == 0 {
		return fmt.Errorf("empty rule set")
	}

	for i, rule := range rules {
		trigger, ok := rule["trigger"]
		if !ok || len(trigger) == 0 || string(trigger) == "null" {
			return fmt.Errorf("rule[%d] missing trigger", i)
		}
		action, ok := rule["action"]
		if !ok || len(action) == 0 || string(action) == "null" {
			return fmt.Errorf("rule[%d] missing action", i)
		}
		var actionObj struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(action, &actionObj); err != nil {
			return fmt.Errorf("rule[%d] action parse failed: %w", i, err)
		}
		if actionObj.Type == "" {
			return fmt.Errorf("rule[%d] action.type missing", i)
		}
	}

	return nil
}

func (m *Manager) quarantineInvalidFile(ctx context.Context, srcPath, cause string) error {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	quarantineDir := filepath.Join(m.jsonDir, "quarantine")
	if err := os.MkdirAll(quarantineDir, quarantinePerm); err != nil {
		return fmt.Errorf("create quarantine dir failed: %w", err)
	}

	baseName := filepath.Base(srcPath)
	quarantinePath := filepath.Join(
		quarantineDir,
		fmt.Sprintf("%d-%s", time.Now().UnixNano(), baseName),
	)
	if err := os.Rename(srcPath, quarantinePath); err != nil {
		return fmt.Errorf("move to quarantine failed: %w", err)
	}

	log.Error().
		Str("path", srcPath).
		Str("quarantine_path", quarantinePath).
		Str("cause", cause).
		Msg("quarantined invalid filter file")

	return nil
}

// Enable enables content filtering.
func (m *Manager) Enable(ctx context.Context) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.setEnabled(true)
	log.Info().Msg("content filtering enabled")

	if !m.backend.HasActive() {
		m.LoadAsync(ctx)
	} else {
		m.setActiveStatus("Filters active")
	}
}

// Disable disables content filtering.
func (m *Manager) Disable(ctx context.Context) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.setEnabled(false)
	m.setStatus(FilterStatus{State: StateDisabled})
	log.Info().Msg("content filtering disabled")
}

// IsEnabled returns whether content filtering is enabled.
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

func (m *Manager) setEnabled(enabled bool) {
	m.mu.Lock()
	m.enabled = enabled
	m.mu.Unlock()
}

func (m *Manager) shouldAutoUpdate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled && m.autoUpdate
}

// Clear removes all compiled and cached filters.
func (m *Manager) Clear(ctx context.Context) error {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if err := m.backend.Clear(ctx); err != nil {
		log.Warn().Err(err).Msg("failed to clear backend filters")
	}

	if err := m.downloader.ClearCache(); err != nil {
		log.Warn().Err(err).Msg("failed to clear download cache")
	}

	m.setStatus(FilterStatus{State: StateUninitialized})
	log.Info().Msg("filters cleared")
	return nil
}
