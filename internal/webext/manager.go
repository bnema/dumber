package webext

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/webext/api"
)

// Extension represents a loaded extension
type Extension struct {
	ID            string
	Path          string
	Manifest      *Manifest
	Enabled       bool
	Bundled       bool // True if bundled with browser
	DataDir       string
	BackgroundCtx *BackgroundContext // Background page context with goja runtime

	// WebExtension APIs
	Runtime *api.RuntimeAPI
	Storage *api.StorageAPI
	Tabs    *api.TabsAPI
}

// Manager manages all browser extensions
type Manager struct {
	mu         sync.RWMutex
	bundled    map[string]*Extension // Built-in extensions
	user       map[string]*Extension // User-installed extensions
	enabled    map[string]bool       // Enable state per extension
	dataDir    string                // Base directory for extension data
	database   *sql.DB               // Database for extension storage
	queries    db.ExtensionsQuerier  // Generated queries for extension metadata
	webRequest *api.WebRequestAPI    // Shared webRequest API for all extensions
}

// NewManager creates a new extension manager
func NewManager(dataDir string, dbConn *sql.DB, queries db.ExtensionsQuerier) *Manager {
	return &Manager{
		bundled:    make(map[string]*Extension),
		user:       make(map[string]*Extension),
		enabled:    make(map[string]bool),
		dataDir:    dataDir,
		database:   dbConn,
		queries:    queries,
		webRequest: api.NewWebRequestAPI(),
	}
}

// LoadExtensionsFromDB loads installed extensions from the database into memory.
// Must be called before loading from disk so the DB remains the source of truth.
func (m *Manager) LoadExtensionsFromDB() error {
	if m.queries == nil {
		return nil
	}

	ctx := context.Background()
	installed, err := m.queries.ListInstalledExtensions(ctx)
	if err != nil {
		return fmt.Errorf("failed to query installed extensions: %w", err)
	}

	for _, ext := range installed {
		if _, statErr := os.Stat(ext.InstallPath); errors.Is(statErr, os.ErrNotExist) {
			log.Printf("[webext] Extension %s in DB missing on disk, skipping", ext.ExtensionID)
			continue
		}

		loadedExt, loadErr := m.loadExtension(ext.InstallPath, ext.Bundled)
		if loadErr != nil {
			log.Printf("[webext] Failed to load extension %s: %v", ext.ExtensionID, loadErr)
			continue
		}

		bundleType := "user"
		if ext.Bundled {
			bundleType = "bundled"
		}

		m.mu.Lock()
		if ext.Bundled {
			m.bundled[ext.ExtensionID] = loadedExt
		} else {
			m.user[ext.ExtensionID] = loadedExt
		}
		m.enabled[ext.ExtensionID] = ext.Enabled
		m.mu.Unlock()

		log.Printf("[webext] Loaded %s extension from DB: %s v%s (enabled: %v)",
			bundleType, ext.Name, ext.Version, ext.Enabled)
	}

	return nil
}

// LoadExtensions loads extensions from a single directory, routing them to bundled or user maps
// based on the database flag when available. New on-disk extensions default to user-installed.
func (m *Manager) LoadExtensions(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create extensions directory: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read extensions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip if already loaded in either bundled or user extensions
		if _, exists := m.bundled[entry.Name()]; exists {
			continue
		}
		if _, exists := m.user[entry.Name()]; exists {
			continue
		}

		extPath := filepath.Join(dir, entry.Name())
		bundled := false
		enabled := false

		// If DB has a record, prefer that as source of truth for bundled/enabled flags.
		if m.queries != nil {
			if extMeta, err := m.queries.GetInstalledExtension(context.Background(), entry.Name()); err == nil {
				bundled = extMeta.Bundled
				enabled = extMeta.Enabled
			}
		}

		ext, err := m.loadExtension(extPath, bundled)
		if err != nil {
			log.Printf("Failed to load extension %s: %v", entry.Name(), err)
			continue
		}

		if bundled {
			m.bundled[ext.ID] = ext
		} else {
			m.user[ext.ID] = ext
		}
		// Default to stored enabled flag if present, otherwise user extensions remain disabled.
		if _, exists := m.enabled[ext.ID]; !exists {
			m.enabled[ext.ID] = enabled
		}
		if err := m.persistExtensionToDB(ext, bundled); err != nil {
			log.Printf("[webext] Warning: failed to persist extension %s: %v", ext.ID, err)
		}
		log.Printf("Loaded extension: %s v%s (bundled: %v, enabled: %v)", ext.Manifest.Name, ext.Manifest.Version, bundled, m.enabled[ext.ID])
	}

	return nil
}

// loadExtension loads a single extension from a directory
func (m *Manager) loadExtension(path string, bundled bool) (*Extension, error) {
	manifestPath := filepath.Join(path, "manifest.json")
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}

	// Use directory name as extension ID (could be improved with UUID)
	extID := filepath.Base(path)

	ext := &Extension{
		ID:       extID,
		Path:     path,
		Manifest: manifest,
		Bundled:  bundled,
		DataDir:  filepath.Join(m.dataDir, extID),
	}

	// Ensure extension data directory exists
	if err := os.MkdirAll(ext.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create extension data directory: %w", err)
	}

	// Initialize WebExtension APIs
	if err := m.InitializeAPIs(ext); err != nil {
		return nil, fmt.Errorf("failed to initialize APIs: %w", err)
	}

	return ext, nil
}

// persistExtensionToDB saves extension metadata to the database (no-op if queries are unavailable).
func (m *Manager) persistExtensionToDB(ext *Extension, bundled bool) error {
	if m.queries == nil {
		return nil
	}

	return m.queries.UpsertInstalledExtension(context.Background(), db.UpsertInstalledExtensionParams{
		ExtensionID: ext.ID,
		Name:        ext.Manifest.Name,
		Version:     ext.Manifest.Version,
		InstallPath: ext.Path,
		Bundled:     bundled,
		Enabled:     m.enabled[ext.ID],
	})
}

// GetExtension returns an extension by ID
func (m *Manager) GetExtension(id string) (*Extension, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ext, ok := m.bundled[id]; ok {
		return ext, true
	}

	if ext, ok := m.user[id]; ok {
		return ext, true
	}

	return nil, false
}

// ListExtensions returns all loaded extensions
func (m *Manager) ListExtensions() []*Extension {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var exts []*Extension
	for _, ext := range m.bundled {
		exts = append(exts, ext)
	}
	for _, ext := range m.user {
		exts = append(exts, ext)
	}

	return exts
}

// IsEnabled checks if an extension is enabled
func (m *Manager) IsEnabled(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.enabled[id]
}

// Enable enables an extension
func (m *Manager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.bundled[id]; !exists {
		if _, exists := m.user[id]; !exists {
			return fmt.Errorf("extension not found: %s", id)
		}
	}

	m.enabled[id] = true
	if m.queries != nil {
		if err := m.queries.SetExtensionEnabled(context.Background(), true, id); err != nil {
			log.Printf("[webext] Warning: failed to persist enabled state: %v", err)
		}
	}
	log.Printf("Enabled extension: %s", id)
	return nil
}

// Disable disables an extension
func (m *Manager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.bundled[id]; !exists {
		if _, exists := m.user[id]; !exists {
			return fmt.Errorf("extension not found: %s", id)
		}
	}

	m.enabled[id] = false
	if m.queries != nil {
		if err := m.queries.SetExtensionEnabled(context.Background(), false, id); err != nil {
			log.Printf("[webext] Warning: failed to persist disabled state: %v", err)
		}
	}
	log.Printf("Disabled extension: %s", id)
	return nil
}

// GetContentScriptsForURL returns all content scripts that match the given URL
func (m *Manager) GetContentScriptsForURL(url string) []ContentScriptMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []ContentScriptMatch

	for _, ext := range m.bundled {
		if !m.enabled[ext.ID] {
			continue
		}
		matches = append(matches, m.matchContentScripts(ext, url)...)
	}

	for _, ext := range m.user {
		if !m.enabled[ext.ID] {
			continue
		}
		matches = append(matches, m.matchContentScripts(ext, url)...)
	}

	return matches
}

// ContentScriptMatch represents a matched content script
type ContentScriptMatch struct {
	Extension     *Extension
	ContentScript *ContentScript
}

// matchContentScripts finds matching content scripts for a URL
func (m *Manager) matchContentScripts(ext *Extension, url string) []ContentScriptMatch {
	var matches []ContentScriptMatch

	for i := range ext.Manifest.ContentScripts {
		cs := &ext.Manifest.ContentScripts[i]
		if matchesPattern(url, cs.Matches, cs.ExcludeMatch) {
			matches = append(matches, ContentScriptMatch{
				Extension:     ext,
				ContentScript: cs,
			})
		}
	}

	return matches
}

// matchesPattern checks if a URL matches the given patterns
func matchesPattern(url string, matches []string, excludes []string) bool {
	// Check if URL is excluded
	if ExcludesURL(url, excludes) {
		return false
	}

	// Check if URL matches any pattern
	return MatchURL(url, matches)
}

// InitializeAPIs initializes WebExtension APIs for a loaded extension
func (m *Manager) InitializeAPIs(ext *Extension) error {
	// Initialize runtime API
	ext.Runtime = api.NewRuntimeAPI(ext.ID)

	// Initialize storage API (uses shared database)
	storageAPI, err := api.NewStorageAPI(ext.ID, m.database)
	if err != nil {
		return fmt.Errorf("failed to initialize storage API: %w", err)
	}
	ext.Storage = storageAPI

	// Initialize tabs API (bridge will be set by browser later)
	ext.Tabs = api.NewTabsAPI()

	log.Printf("[webext] Initialized APIs for extension %s", ext.ID)
	return nil
}

// GetWebRequestAPI returns the shared webRequest API instance
func (m *Manager) GetWebRequestAPI() *api.WebRequestAPI {
	return m.webRequest
}

// StartBackgroundContext initializes and starts the background context for an extension
func (m *Manager) StartBackgroundContext(ext *Extension) error {
	// Only start background context if extension has background scripts
	if ext.Manifest.Background == nil || len(ext.Manifest.Background.Scripts) == 0 {
		log.Printf("[webext] Extension %s has no background scripts", ext.ID)
		return nil
	}

	// Create background context
	ext.BackgroundCtx = NewBackgroundContext(ext)

	// Start the context (loads and executes background scripts)
	if err := ext.BackgroundCtx.Start(); err != nil {
		return fmt.Errorf("failed to start background context: %w", err)
	}

	return nil
}

// StopBackgroundContext stops the background context for an extension
func (m *Manager) StopBackgroundContext(ext *Extension) {
	if ext.BackgroundCtx != nil {
		ext.BackgroundCtx.Stop()
		ext.BackgroundCtx = nil
	}
}
