package filtering

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk-webkit/webkit"
)

const (
	storeDirPerm  = 0o755
	jsonDirPerm   = 0o755
	mergedFilePerm = 0o644
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

	go func() {
		log := logging.FromContext(ctx).With().
			Str("component", "filter-manager").
			Logger()

		// If already loaded, skip
		m.filterMu.RLock()
		if m.filter != nil {
			m.filterMu.RUnlock()
			log.Debug().Msg("filters already loaded, skipping async load")
			return
		}
		m.filterMu.RUnlock()

		// Try to load from cache first (fast path)
		m.setStatus(FilterStatus{State: StateLoading, Message: "Loading filters..."})
		if m.store.HasCompiledFilter(ctx, FilterIdentifier) {
			log.Debug().Msg("found compiled filter, loading from cache")
			filter, err := m.store.Load(ctx, FilterIdentifier)
			if err == nil && filter != nil {
				m.filterMu.Lock()
				m.filter = filter
				m.filterMu.Unlock()

				version := m.getCachedVersion()
				m.setStatus(FilterStatus{
					State:   StateActive,
					Message: "Filters active",
					Version: version,
				})
				log.Info().Str("version", version).Msg("filters loaded from cache")
				return
			}
			log.Warn().Err(err).Msg("failed to load cached filter, will download")
		}

		// Download filters
		m.setStatus(FilterStatus{State: StateLoading, Message: "Downloading filters..."})
		paths, err := m.downloader.DownloadFilters(ctx, func(p DownloadProgress) {
			msg := fmt.Sprintf("Downloading filters (%d/%d)...", p.Current, p.Total)
			m.setStatus(FilterStatus{State: StateLoading, Message: msg})
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to download filters")
			m.setStatus(FilterStatus{State: StateError, Message: "Download failed"})
			return
		}

		m.setStatus(FilterStatus{State: StateLoading, Message: "Compiling filters..."})

		// Merge JSON files into one for compilation
		mergedPath, err := m.mergeJSONFiles(ctx, paths)
		if err != nil {
			log.Error().Err(err).Msg("failed to merge filter files")
			m.setStatus(FilterStatus{State: StateError, Message: "Filter merge failed"})
			return
		}

		// Compile the merged filter
		filter, err := m.store.Compile(ctx, FilterIdentifier, mergedPath)
		if err != nil {
			log.Error().Err(err).Msg("failed to compile filters")
			m.setStatus(FilterStatus{State: StateError, Message: "Compilation failed"})
			return
		}

		m.filterMu.Lock()
		m.filter = filter
		m.filterMu.Unlock()

		version := m.getCachedVersion()
		m.setStatus(FilterStatus{
			State:   StateActive,
			Message: "Filters active",
			Version: version,
		})
		log.Info().Str("version", version).Msg("filters compiled and active")
	}()
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

	// Merge and compile
	mergedPath, err := m.mergeJSONFiles(ctx, paths)
	if err != nil {
		return fmt.Errorf("failed to merge updated filters: %w", err)
	}

	filter, err := m.store.Compile(ctx, FilterIdentifier, mergedPath)
	if err != nil {
		return fmt.Errorf("failed to compile updated filters: %w", err)
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
