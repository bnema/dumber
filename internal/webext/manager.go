package webext

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/webext/api"
)

// Extension represents a loaded extension
type Extension struct {
	ID              string
	Path            string
	Manifest        *Manifest
	Enabled         bool
	Bundled         bool // True if bundled with browser
	DataDir         string
	BackgroundCtx   *BackgroundContext // Background page context with goja runtime

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
	webRequest *api.WebRequestAPI    // Shared webRequest API for all extensions
}

// NewManager creates a new extension manager
func NewManager(dataDir string, db *sql.DB) *Manager {
	return &Manager{
		bundled:    make(map[string]*Extension),
		user:       make(map[string]*Extension),
		enabled:    make(map[string]bool),
		dataDir:    dataDir,
		database:   db,
		webRequest: api.NewWebRequestAPI(),
	}
}

// LoadBundledExtensions loads extensions from the bundled directory
func (m *Manager) LoadBundledExtensions(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		log.Printf("Bundled extensions directory does not exist: %s", dir)
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read bundled extensions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		extPath := filepath.Join(dir, entry.Name())
		ext, err := m.loadExtension(extPath, true)
		if err != nil {
			log.Printf("Failed to load bundled extension %s: %v", entry.Name(), err)
			continue
		}

		m.bundled[ext.ID] = ext
		m.enabled[ext.ID] = true // Bundled extensions enabled by default
		log.Printf("Loaded bundled extension: %s v%s", ext.Manifest.Name, ext.Manifest.Version)
	}

	return nil
}

// LoadUserExtensions loads user-installed extensions
func (m *Manager) LoadUserExtensions(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// Create directory if it doesn't exist
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create user extensions directory: %w", err)
		}
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read user extensions directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		extPath := filepath.Join(dir, entry.Name())
		ext, err := m.loadExtension(extPath, false)
		if err != nil {
			log.Printf("Failed to load user extension %s: %v", entry.Name(), err)
			continue
		}

		m.user[ext.ID] = ext
		// User extensions disabled by default unless explicitly enabled
		if _, exists := m.enabled[ext.ID]; !exists {
			m.enabled[ext.ID] = false
		}
		log.Printf("Loaded user extension: %s v%s (enabled: %v)", ext.Manifest.Name, ext.Manifest.Version, m.enabled[ext.ID])
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
