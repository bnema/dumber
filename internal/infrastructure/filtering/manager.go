package filtering

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	storeDirPerm   = 0o755
	jsonDirPerm    = 0o755
	quarantinePerm = 0o755
	mergedFilePerm = 0o644

	// CacheMaxAge is the maximum age of the filter cache before it's considered stale.
	CacheMaxAge = 24 * time.Hour
)

// Manager orchestrates the content filter lifecycle.
// It handles downloading, compiling, loading, and applying filters to WebViews.
type Manager struct {
	store      FilterStore
	downloader FilterDownloader

	storeDir string
	jsonDir  string

	filter     *webkit.UserContentFilter
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
	if !m.store.HasCompiledFilter(ctx, FilterIdentifier) {
		return false
	}

	log.Debug().Msg("found compiled filter, loading from cache")
	filter, err := m.store.Load(ctx, FilterIdentifier)
	if err != nil || filter == nil {
		log.Warn().Err(err).Msg("failed to load cached filter, will download")
		return false
	}

	version := m.setActiveFilter(filter, "Filters active")
	log.Info().Str("version", version).Msg("filters loaded from cache")
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

	m.setStatus(FilterStatus{State: StateLoading, Message: "Compiling filters..."})
	mergedPath, err := m.mergeJSONFiles(ctx, paths)
	if err != nil {
		log.Error().Err(err).Msg("failed to merge filter files")
		m.setStatus(FilterStatus{State: StateError, Message: "Filter merge failed"})
		return
	}

	filter, err := m.store.Compile(ctx, FilterIdentifier, mergedPath)
	if err != nil {
		log.Error().Err(err).Msg("failed to compile filters")
		m.handleCompilationFailure()
		return
	}
	if filter == nil {
		log.Error().Msg("filter compilation returned nil filter")
		m.handleCompilationFailure()
		return
	}

	version := m.setActiveFilter(filter, "Filters active")
	log.Info().Str("version", version).Msg("filters compiled and active")
}

func (m *Manager) downloadFiltersWithProgress(ctx context.Context) ([]string, error) {
	m.setStatus(FilterStatus{State: StateLoading, Message: "Downloading filters..."})
	return m.downloader.DownloadFilters(ctx, func(p DownloadProgress) {
		msg := fmt.Sprintf("Downloading filters (%d/%d)...", p.Current, p.Total)
		m.setStatus(FilterStatus{State: StateLoading, Message: msg})
	})
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

func (m *Manager) setActiveFilter(filter *webkit.UserContentFilter, message string) string {
	m.filterMu.Lock()
	m.filter = filter
	m.filterMu.Unlock()

	version := m.getCachedVersion()
	m.setStatus(FilterStatus{
		State:   StateActive,
		Message: message,
		Version: version,
	})
	return version
}

// mergeJSONFiles combines multiple JSON rule files into one.
// WebKit's UserContentFilterStore expects a single JSON array.
func (m *Manager) mergeJSONFiles(ctx context.Context, paths []string) (string, error) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	var allRules []json.RawMessage

	for _, path := range paths {
		log.Debug().Str("path", path).Msg("reading filter file")

		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read %s: %w", path, err)
		}

		var rules []json.RawMessage
		if err := json.Unmarshal(data, &rules); err != nil {
			return "", fmt.Errorf("failed to parse %s: %w", path, err)
		}

		allRules = append(allRules, rules...)
	}

	log.Debug().Int("total_rules", len(allRules)).Msg("merging filter rules")

	merged, err := json.Marshal(allRules)
	if err != nil {
		return "", fmt.Errorf("failed to marshal merged rules: %w", err)
	}

	// Write merged file
	mergedPath := filepath.Join(m.jsonDir, "merged.json")
	if err := os.WriteFile(mergedPath, merged, mergedFilePerm); err != nil {
		return "", fmt.Errorf("failed to write merged file: %w", err)
	}

	return mergedPath, nil
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

// ApplyTo adds the active filter to a WebView's UserContentManager.
// Safe to call even if filters are not yet loaded (no-op in that case).
func (m *Manager) ApplyTo(ctx context.Context, ucm *webkit.UserContentManager) {
	log := logging.FromContext(ctx).With().
		Str("component", "filter-manager").
		Logger()

	if ucm == nil {
		return
	}

	m.filterMu.RLock()
	filter := m.filter
	m.filterMu.RUnlock()

	if filter != nil {
		ucm.AddFilter(filter)
		log.Debug().Msg("content filter applied to webview")
	} else {
		log.Debug().Msg("no filter available to apply (filters still loading?)")
	}
}

// GetFilter returns the currently loaded filter, or nil if none.
func (m *Manager) GetFilter() *webkit.UserContentFilter {
	m.filterMu.RLock()
	defer m.filterMu.RUnlock()
	return m.filter
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
		return nil
	}

	// Merge and compile
	mergedPath, err := m.mergeJSONFiles(ctx, paths)
	if err != nil {
		log.Warn().Err(err).Msg("failed to merge updated filters, keeping existing active filter")
		return nil
	}

	filter, err := m.store.Compile(ctx, FilterIdentifier, mergedPath)
	if err != nil {
		log.Warn().Err(err).Msg("failed to compile updated filters, keeping existing active filter")
		return nil
	}
	if filter == nil {
		log.Warn().Msg("updated filter compilation returned nil filter, keeping existing active filter")
		return nil
	}

	m.filterMu.Lock()
	m.filter = filter
	m.filterMu.Unlock()

	version := m.getCachedVersion()
	m.setStatus(FilterStatus{
		State:   StateActive,
		Message: fmt.Sprintf("Filters updated to %s", version),
		Version: version,
	})
	log.Info().Str("version", version).Msg("filters updated successfully")

	return nil
}

func (m *Manager) hasActiveFilter() bool {
	m.filterMu.RLock()
	defer m.filterMu.RUnlock()
	return m.filter != nil
}

func (m *Manager) validateDownloadedFiles(ctx context.Context, paths []string) error {
	for _, path := range paths {
		if err := validateRuleFile(path); err != nil {
			if quarantineErr := m.quarantineInvalidFile(ctx, path, "rule_validation_failed"); quarantineErr != nil {
				return fmt.Errorf("invalid filter file %s: %w (quarantine failed: %s)", path, err, quarantineErr.Error())
			}
			return fmt.Errorf("invalid filter file %s: %w", path, err)
		}
	}
	return nil
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
	hasFilter := m.filter != nil
	m.filterMu.RUnlock()

	if !hasFilter {
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
	m.filter = nil
	m.filterMu.Unlock()

	if err := m.store.Remove(ctx, FilterIdentifier); err != nil {
		log.Warn().Err(err).Msg("failed to remove compiled filter")
	}

	if err := m.downloader.ClearCache(); err != nil {
		log.Warn().Err(err).Msg("failed to clear download cache")
	}

	m.setStatus(FilterStatus{State: StateUninitialized})
	log.Info().Msg("filters cleared")
	return nil
}
