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
	"github.com/bnema/puregotk-webkit/webkit"
)

// Compile-time check that Manager satisfies port.FilterManager.
var _ port.FilterManager = (*Manager)(nil)

const (
	storeDirPerm   = 0o755
	jsonDirPerm    = 0o755
	quarantinePerm = 0o755
	mergedFilePerm = 0o644

	// CacheMaxAge is the maximum age of the filter cache before it's considered stale.
	CacheMaxAge = 24 * time.Hour
)

// ErrUpdateSkipped indicates a filter update was intentionally skipped after a recoverable
// update-path failure (e.g. invalid payload, merge failure, compile failure).
var ErrUpdateSkipped = errors.New("filter update skipped")

// Manager orchestrates the content filter lifecycle.
// It handles downloading, compiling, loading, and applying filters to WebViews.
type Manager struct {
	store      FilterStore
	downloader FilterDownloader

	storeDir string
	jsonDir  string

	filters    []*webkit.UserContentFilter
	filterMu   sync.RWMutex
	status     atomic.Value // FilterStatus
	enabled    bool
	autoUpdate bool

	// Callbacks for status updates (e.g., toast notifications)
	onStatusChange func(FilterStatus)
}

// ManagerConfig holds configuration for the filter manager.
type ManagerConfig struct {
	StoreDir   string // Where to store compiled filter bytecode
	JSONDir    string // Where to cache downloaded JSON files
	Enabled    bool   // Whether filtering is enabled
	AutoUpdate bool   // Whether to auto-update filters

	// Optional: custom implementations for testing
	Store      FilterStore      // If nil, creates default Store
	Downloader FilterDownloader // If nil, creates default Downloader
}

// NewManager creates a new filter Manager.
func NewManager(cfg ManagerConfig) (*Manager, error) {
	// Ensure directories exist
	if err := os.MkdirAll(cfg.StoreDir, storeDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create store dir: %w", err)
	}
	if err := os.MkdirAll(cfg.JSONDir, jsonDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create json dir: %w", err)
	}

	// Use provided implementations or create defaults
	store := cfg.Store
	if store == nil {
		newStore := NewStore(cfg.StoreDir)
		if newStore == nil {
			return nil, fmt.Errorf("failed to create filter store at %s", cfg.StoreDir)
		}
		store = newStore
	}

	downloader := cfg.Downloader
	if downloader == nil {
		downloader = NewDownloader(cfg.JSONDir)
	}

	m := &Manager{
		store:      store,
		downloader: downloader,
		storeDir:   cfg.StoreDir,
		jsonDir:    cfg.JSONDir,
		enabled:    cfg.Enabled,
		autoUpdate: cfg.AutoUpdate,
	}

	m.setStatus(FilterStatus{State: StateUninitialized})
	return m, nil
}

// SetStatusCallback sets a callback for status changes (e.g., for toast notifications).
func (m *Manager) SetStatusCallback(cb func(FilterStatus)) {
	m.onStatusChange = cb
}

// setStatus updates the current status and notifies callback.
func (m *Manager) setStatus(status FilterStatus) {
	m.status.Store(status)
	if m.onStatusChange != nil {
		m.onStatusChange(status)
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
// This is called BEFORE GTK starts, so we cannot use GIO async operations.
// Just sets up state - actual filter loading happens in LoadAsync after GTK starts.
func (m *Manager) Initialize(ctx context.Context) error {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if !m.enabled {
		log.Info().Msg("content filtering disabled")
		m.setStatus(FilterStatus{State: StateDisabled})
		return nil
	}

	log.Debug().Msg("filter manager initialized (filters will load after GTK starts)")
	m.setStatus(FilterStatus{State: StateUninitialized, Message: "Filters pending"})
	return nil
}

// LoadAsync loads or downloads filters in the background.
// Call this after the browser window is shown (GTK main loop running).
// First tries to load from cache, then downloads if needed.
func (m *Manager) LoadAsync(ctx context.Context) {
	if !m.enabled {
		return
	}

	go m.loadAsyncWorker(ctx)
}

func (m *Manager) loadAsyncWorker(ctx context.Context) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if m.hasActiveFilter() {
		log.Debug().Msg("filters already loaded, skipping async load")
		return
	}

	if m.loadFromCache(ctx) {
		m.checkStaleCacheAndUpdate(ctx)
		return
	}

	m.downloadCompileAndActivate(ctx)
}

func (m *Manager) loadFromCache(ctx context.Context) bool {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.setStatus(FilterStatus{State: StateLoading, Message: "Loading filters..."})

	// Discover how many compiled parts exist via stored identifiers
	identifiers, err := m.store.FetchIdentifiers(ctx)
	if err != nil {
		return false
	}
	var partIDs []string
	for _, id := range identifiers {
		if strings.HasPrefix(id, FilterIdentifierPrefix) {
			partIDs = append(partIDs, id)
		}
	}
	if len(partIDs) == 0 {
		return false
	}

	log.Debug().Int("parts", len(partIDs)).Msg("found compiled filters, loading from cache")
	var filters []*webkit.UserContentFilter
	for _, id := range partIDs {
		filter, loadErr := m.store.Load(ctx, id)
		if loadErr != nil || filter == nil {
			log.Warn().Err(loadErr).Str("id", id).Msg("failed to load cached filter part, will download")
			return false
		}
		filters = append(filters, filter)
	}

	version := m.setActiveFilters(filters, "Filters active")
	log.Info().Str("version", version).Int("parts", len(filters)).Msg("filters loaded from cache")
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

	validateErr := m.validateDownloadedFiles(ctx, paths)
	if validateErr != nil {
		log.Error().Err(validateErr).Msg("downloaded filter validation failed")
		m.setStatus(FilterStatus{State: StateError, Message: "Invalid downloaded filters"})
		return
	}

	filters, err := m.compileFilterParts(ctx, paths)
	if err != nil {
		log.Error().Err(err).Msg("failed to compile filters")
		m.handleCompilationFailure()
		return
	}

	version := m.setActiveFilters(filters, "Filters active")
	log.Info().Str("version", version).Int("parts", len(filters)).Msg("filters compiled and active")
}

func (m *Manager) downloadFiltersWithProgress(ctx context.Context) ([]string, error) {
	m.setStatus(FilterStatus{State: StateLoading, Message: "Downloading filters..."})
	return m.downloader.DownloadFilters(ctx, func(p DownloadProgress) {
		msg := fmt.Sprintf("Downloading filters (%d/%d)...", p.Current, p.Total)
		m.setStatus(FilterStatus{State: StateLoading, Message: msg})
	})
}

// compileFilterParts compiles each downloaded part file as a separate WebKit filter.
// Each part gets a unique identifier (e.g., "ublock-combined-0", "ublock-combined-1").
func (m *Manager) compileFilterParts(ctx context.Context, paths []string) ([]*webkit.UserContentFilter, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.setStatus(FilterStatus{State: StateLoading, Message: "Compiling filters..."})

	filters := make([]*webkit.UserContentFilter, 0, len(paths))
	for i, path := range paths {
		identifier := fmt.Sprintf("%s-%d", FilterIdentifierPrefix, i)
		log.Debug().Str("id", identifier).Str("path", path).Msg("compiling filter part")

		filter, err := m.store.Compile(ctx, identifier, path)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", identifier, err)
		}
		if filter == nil {
			return nil, fmt.Errorf("compile %s returned nil filter", identifier)
		}
		filters = append(filters, filter)
	}

	return filters, nil
}

func (m *Manager) handleCompilationFailure() {
	if m.hasActiveFilter() {
		version := m.getCachedVersion()
		m.setStatus(FilterStatus{
			State:   StateActive,
			Message: "Filters active (update skipped)",
			Version: version,
		})
		return
	}
	m.setStatus(FilterStatus{State: StateError, Message: "Compilation failed"})
}

func (m *Manager) setActiveFilters(filters []*webkit.UserContentFilter, message string) string {
	m.filterMu.Lock()
	m.filters = filters
	m.filterMu.Unlock()

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

// ApplyTo adds the active filters to a WebView's UserContentManager.
// Safe to call even if filters are not yet loaded (no-op in that case).
func (m *Manager) ApplyTo(ctx context.Context, ucm *webkit.UserContentManager) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if ucm == nil {
		return
	}

	m.filterMu.RLock()
	filters := m.filters
	m.filterMu.RUnlock()

	if len(filters) > 0 {
		for _, filter := range filters {
			ucm.AddFilter(filter)
		}
		log.Debug().Int("parts", len(filters)).Msg("content filters applied to webview")
	} else {
		log.Debug().Msg("no filter available to apply (filters still loading?)")
	}
}

// GetFilters returns the currently loaded filters, or nil if none.
func (m *Manager) GetFilters() []*webkit.UserContentFilter {
	m.filterMu.RLock()
	defer m.filterMu.RUnlock()
	return m.filters
}

// CheckForUpdates checks if newer filters are available and downloads them.
// This should be called periodically in the background.
func (m *Manager) CheckForUpdates(ctx context.Context) error {
	if !m.enabled || !m.autoUpdate {
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

	// Download new filters
	paths, err := m.downloader.DownloadFilters(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to download filter update: %w", err)
	}
	validateErr := m.validateDownloadedFiles(ctx, paths)
	if validateErr != nil {
		log.Warn().Err(validateErr).Msg("invalid downloaded filters, keeping existing active filter")
		return fmt.Errorf("%w: validation failed: %w", ErrUpdateSkipped, validateErr)
	}

	filters, err := m.compileFilterParts(ctx, paths)
	if err != nil {
		log.Warn().Err(err).Msg("failed to compile updated filters, keeping existing active filter")
		// Restore status if we still have an active filter
		if m.hasActiveFilter() {
			version := m.getCachedVersion()
			m.setStatus(FilterStatus{
				State:   StateActive,
				Message: "Filters active (update skipped)",
				Version: version,
			})
		}
		return fmt.Errorf("%w: compile failed: %w", ErrUpdateSkipped, err)
	}

	version := m.setActiveFilters(filters, "Filters updated")
	log.Info().Str("version", version).Int("parts", len(filters)).Msg("filters updated successfully")

	return nil
}

func (m *Manager) hasActiveFilter() bool {
	m.filterMu.RLock()
	defer m.filterMu.RUnlock()
	return len(m.filters) > 0
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

	m.enabled = true
	log.Info().Msg("content filtering enabled")

	// Try to load filters if not already loaded
	m.filterMu.RLock()
	hasFilters := len(m.filters) > 0
	m.filterMu.RUnlock()

	if !hasFilters {
		m.LoadAsync(ctx)
	} else {
		m.setStatus(FilterStatus{
			State:   StateActive,
			Message: "Filters active",
			Version: m.getCachedVersion(),
		})
	}
}

// Disable disables content filtering.
func (m *Manager) Disable(ctx context.Context) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.enabled = false
	m.setStatus(FilterStatus{State: StateDisabled})
	log.Info().Msg("content filtering disabled")
}

// IsEnabled returns whether content filtering is enabled.
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// Clear removes all compiled and cached filters.
func (m *Manager) Clear(ctx context.Context) error {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	m.filterMu.Lock()
	m.filters = nil
	m.filterMu.Unlock()

	// Remove all compiled filter parts
	identifiers, _ := m.store.FetchIdentifiers(ctx)
	for _, id := range identifiers {
		if strings.HasPrefix(id, FilterIdentifierPrefix) {
			if err := m.store.Remove(ctx, id); err != nil {
				log.Warn().Err(err).Str("id", id).Msg("failed to remove compiled filter")
			}
		}
	}

	if err := m.downloader.ClearCache(); err != nil {
		log.Warn().Err(err).Msg("failed to clear download cache")
	}

	m.setStatus(FilterStatus{State: StateUninitialized})
	log.Info().Msg("filters cleared")
	return nil
}
