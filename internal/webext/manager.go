package webext

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bnema/dumber/internal/app/schemes"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/webext/api"
	"github.com/bnema/dumber/internal/webext/shared"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// Extension represents a loaded extension
type Extension struct {
	ID              string
	Path            string
	Manifest        *Manifest
	Enabled         bool
	Bundled         bool // True if bundled with browser
	DataDir         string
	Background      *BackgroundContext // Goja-based background context
	HostPermissions []string           // CORS allowlist patterns for extension resources

	// WebExtension APIs
	Runtime *api.RuntimeAPI
	Storage *api.StorageAPI

	// CSS tracking (for tabs.insertCSS/removeCSS)
	customCSS     map[string]*webkit.UserStyleSheet // CSS code -> UserStyleSheet mapping
	customCSSLock sync.RWMutex
}

// GetID returns the extension ID.
func (e *Extension) GetID() string {
	return e.ID
}

// GetInstallDir returns the extension installation directory.
func (e *Extension) GetInstallDir() string {
	return e.Path
}

// HasHostPermission checks if the extension has permission to access the given URL.
// This checks the manifest's permissions array for matching host patterns.
func (e *Extension) HasHostPermission(urlStr string) bool {
	if e.Manifest == nil {
		return false
	}

	// Parse the URL to check
	targetURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Check each host permission pattern
	for _, pattern := range e.HostPermissions {
		if matchesHostPattern(pattern, targetURL) {
			return true
		}
	}

	return false
}

// HasPermission checks if the extension has a specific API permission.
func (e *Extension) HasPermission(permission string) bool {
	if e.Manifest == nil {
		return false
	}
	return e.Manifest.HasPermission(permission)
}

// AddCustomCSS creates and stores a UserStyleSheet for the given CSS code.
// Returns the created stylesheet or existing one if already added.
func (e *Extension) AddCustomCSS(code string) *webkit.UserStyleSheet {
	e.customCSSLock.Lock()
	defer e.customCSSLock.Unlock()

	// Initialize map if needed
	if e.customCSS == nil {
		e.customCSS = make(map[string]*webkit.UserStyleSheet)
	}

	// Return existing if already added
	if sheet, exists := e.customCSS[code]; exists {
		return sheet
	}

	// Create new UserStyleSheet
	// webkit.NewUserStyleSheet(source, injectedFrames, level, allowlist, blocklist)
	sheet := webkit.NewUserStyleSheet(
		code,
		webkit.UserContentInjectAllFrames,
		webkit.UserStyleLevelUser,
		nil, // allowlist (nil = all pages)
		nil, // blocklist
	)

	e.customCSS[code] = sheet
	return sheet
}

// GetCustomCSS retrieves a previously added UserStyleSheet by CSS code.
// Returns nil if not found.
func (e *Extension) GetCustomCSS(code string) *webkit.UserStyleSheet {
	e.customCSSLock.RLock()
	defer e.customCSSLock.RUnlock()

	if e.customCSS == nil {
		return nil
	}

	return e.customCSS[code]
}

// matchesHostPattern checks if a URL matches a WebExtension host permission pattern.
// Patterns can include wildcards like https://*.example.com/* or <all_urls>
func matchesHostPattern(pattern string, targetURL *url.URL) bool {
	// Special case: <all_urls> matches everything
	if pattern == "<all_urls>" {
		return true
	}

	// Parse the pattern as a URL
	patternURL, err := url.Parse(pattern)
	if err != nil {
		return false
	}

	// Check scheme
	if patternURL.Scheme != "*" && patternURL.Scheme != targetURL.Scheme {
		return false
	}

	// Check host with wildcard support
	patternHost := patternURL.Host
	targetHost := targetURL.Host

	// Handle *:// wildcard scheme
	if strings.HasPrefix(pattern, "*://") {
		// Only match http and https
		if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
			return false
		}
	}

	// Handle wildcard in host (e.g., *.example.com)
	if strings.HasPrefix(patternHost, "*.") {
		// Match exact domain or any subdomain
		domain := patternHost[2:] // Remove "*."
		if targetHost == domain || strings.HasSuffix(targetHost, "."+domain) {
			// Hosts match, now check path
			return matchesPath(patternURL.Path, targetURL.Path)
		}
		return false
	}

	// Handle exact host match or wildcard host (*)
	if patternHost == "*" || patternHost == targetHost {
		return matchesPath(patternURL.Path, targetURL.Path)
	}

	return false
}

// matchesPath checks if a target path matches a pattern path.
// Pattern paths can end with * to match any subpath.
func matchesPath(pattern, target string) bool {
	// Exact match
	if pattern == target {
		return true
	}

	// Wildcard at end (e.g., /foo/*)
	if strings.HasSuffix(pattern, "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(target, prefix)
	}

	return false
}

// PaneInfo represents information about a browser pane (what extensions call a "tab")
type PaneInfo struct {
	ID       uint64 // Pane/WebView ID
	Index    int    // Position in workspace
	Active   bool   // Is this the active pane
	URL      string
	Title    string
	WindowID uint64 // Workspace ID (what extensions call a "window")
}

// GetAllPanesFunc returns all panes in the active workspace
type GetAllPanesFunc func() []PaneInfo

// GetActivePaneFunc returns the currently active pane
type GetActivePaneFunc func() *PaneInfo

// Manager manages all browser extensions
type Manager struct {
	mu            sync.RWMutex
	bundled       map[string]*Extension  // Built-in extensions
	user          map[string]*Extension  // User-installed extensions
	enabled       map[string]bool        // Enable state per extension
	extensionsDir string                 // Base directory for installed extension code
	dataDir       string                 // Base directory for extension data
	database      *sql.DB                // Database for extension storage
	queries       db.ExtensionsQuerier   // Generated queries for extension metadata
	schemeHandler *schemes.WebExtHandler // Handler for dumb-extension:// URIs
	dispatcher    *Dispatcher            // API dispatcher for webext:api messages
	getAllPanes   GetAllPanesFunc        // Callback to get all panes from workspace
	getActivePane GetActivePaneFunc      // Callback to get active pane from workspace
	viewLookup    ViewLookup             // Lookup for finding WebViews by ID
	portCallbacks map[string]api.PortCallbacks // Port callback targets keyed by port ID
	// Callback to notify popups of storage changes
	storageChangeNotifier func(extID string, changes map[string]api.StorageChange, areaName string)
}

// NewManager creates a new extension manager
func NewManager(extensionsDir, dataDir string, dbConn *sql.DB, queries db.ExtensionsQuerier) *Manager {
	m := &Manager{
		bundled:       make(map[string]*Extension),
		user:          make(map[string]*Extension),
		enabled:       make(map[string]bool),
		extensionsDir: extensionsDir,
		dataDir:       dataDir,
		database:      dbConn,
		queries:       queries,
		portCallbacks: make(map[string]api.PortCallbacks),
	}

	m.schemeHandler = schemes.NewWebExtHandler(
		m.getExtensionByID,
		m.isExtensionEnabled,
	)

	// Dispatcher will be initialized after viewLookup is set via SetViewLookup()

	return m
}

// SetViewLookup sets the ViewLookup and initializes the dispatcher
func (m *Manager) SetViewLookup(viewLookup ViewLookup) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.viewLookup = viewLookup
	m.dispatcher = NewDispatcher(m, viewLookup)
}

// SetStorageChangeNotifier registers a callback to notify popups of storage changes
func (m *Manager) SetStorageChangeNotifier(notifier func(extID string, changes map[string]api.StorageChange, areaName string)) {
	m.mu.Lock()
	m.storageChangeNotifier = notifier
	m.mu.Unlock()
}

// InitializeCookieManager initializes the cookie manager after network session is created
func (m *Manager) InitializeCookieManager() {
	m.mu.RLock()
	dispatcher := m.dispatcher
	m.mu.RUnlock()

	if dispatcher != nil {
		dispatcher.InitializeCookieManager()
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

	// Build CORS allowlist patterns for this extension
	ext.HostPermissions = buildHostPermissions(ext)

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

// Remove disables and removes an extension from memory and storage.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var ext *Extension
	if e, ok := m.bundled[id]; ok {
		ext = e
		delete(m.bundled, id)
	} else if e, ok := m.user[id]; ok {
		ext = e
		delete(m.user, id)
	} else {
		return fmt.Errorf("extension not found: %s", id)
	}

	delete(m.enabled, id)

	if m.queries != nil {
		if err := m.queries.MarkExtensionDeleted(context.Background(), id); err != nil {
			log.Printf("[webext] Warning: failed to mark extension %s deleted: %v", id, err)
		}
	}

	if ext != nil {
		// Stop background context (handles webRequest listener cleanup)
		m.StopBackgroundContext(ext)
		if ext.DataDir != "" {
			if err := os.RemoveAll(ext.DataDir); err != nil && !os.IsNotExist(err) {
				log.Printf("[webext] Warning: failed to remove data dir for %s: %v", id, err)
			}
		}
		if !ext.Bundled && ext.Path != "" {
			if err := os.RemoveAll(ext.Path); err != nil && !os.IsNotExist(err) {
				log.Printf("[webext] Warning: failed to remove extension dir for %s: %v", id, err)
			}
		}
	}

	log.Printf("[webext] Removed extension: %s", id)
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
	ContentScript *shared.ContentScript
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
	if shared.ExcludesURL(url, excludes) {
		return false
	}

	// Check if URL matches any pattern
	return shared.MatchURL(url, matches)
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

	// Register storage change listener
	ext.Storage.Local().OnChanged(func(changes map[string]api.StorageChange, areaName string) {
		// Notify background context directly (background scripts run in Goja context)
		if ext.Background != nil {
			ext.Background.NotifyStorageChange(changes, areaName)
		}
		// Notify popups via callback (popup manager handles this)
		m.mu.RLock()
		notifier := m.storageChangeNotifier
		m.mu.RUnlock()
		if notifier != nil {
			notifier(ext.ID, changes, areaName)
		}
	})

	// Register commands from manifest if present
	if ext.Manifest.Commands != nil && m.dispatcher != nil && m.dispatcher.commandsAPI != nil {
		if err := m.dispatcher.commandsAPI.RegisterExtensionCommands(ext.ID, ext.Manifest.Commands); err != nil {
			log.Printf("[webext] Warning: failed to register commands for %s: %v", ext.ID, err)
		}
	}

	log.Printf("[webext] Initialized APIs for extension %s", ext.ID)
	return nil
}

// GetDispatcher returns the API dispatcher for webext:api messages
func (m *Manager) GetDispatcher() *Dispatcher {
	return m.dispatcher
}

// anyExtensionHasPermission checks if any enabled extension has the given permission.
// Used for permission-based initialization rather than runtime state tracking.
func (m *Manager) anyExtensionHasPermission(perm string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for id, ext := range m.bundled {
		if m.enabled[id] && ext.Manifest != nil && ext.Manifest.HasPermission(perm) {
			return true
		}
	}
	for id, ext := range m.user {
		if m.enabled[id] && ext.Manifest != nil && ext.Manifest.HasPermission(perm) {
			return true
		}
	}
	return false
}

// HasWebRequestCapability returns true if any enabled extension has webRequest or webRequestBlocking permission.
func (m *Manager) HasWebRequestCapability() bool {
	return m.anyExtensionHasPermission("webRequest") || m.anyExtensionHasPermission("webRequestBlocking")
}

// GetEnabledExtensionsWithWebRequest returns IDs of enabled extensions with webRequest permission.
// Used by browser to dispatch webRequest events to all capable extensions.
func (m *Manager) GetEnabledExtensionsWithWebRequest() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var ids []string
	for id, ext := range m.bundled {
		if m.enabled[id] && ext.Manifest != nil {
			if ext.Manifest.HasPermission("webRequest") || ext.Manifest.HasPermission("webRequestBlocking") {
				ids = append(ids, id)
			}
		}
	}
	for id, ext := range m.user {
		if m.enabled[id] && ext.Manifest != nil {
			if ext.Manifest.HasPermission("webRequest") || ext.Manifest.HasPermission("webRequestBlocking") {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// GetBackgroundContext returns the Goja background context for an extension.
func (m *Manager) GetBackgroundContext(extID string) *BackgroundContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ext := m.bundled[extID]; ext != nil {
		return ext.Background
	}
	if ext := m.user[extID]; ext != nil {
		return ext.Background
	}
	return nil
}

// DispatchRuntimeMessage routes a runtime.onMessage payload into the background context.
func (m *Manager) DispatchRuntimeMessage(extID string, sender api.MessageSender, message interface{}) (interface{}, error) {
	bg := m.GetBackgroundContext(extID)
	if bg == nil {
		return nil, fmt.Errorf("background not running for %s", extID)
	}
	return bg.DispatchRuntimeMessage(sender, message)
}

// ConnectBackgroundPort registers a port connection with the background context and records callbacks.
func (m *Manager) ConnectBackgroundPort(extID string, desc api.PortDescriptor) error {
	bg := m.GetBackgroundContext(extID)
	if bg == nil {
		return fmt.Errorf("background not running for %s", extID)
	}
	m.mu.Lock()
	m.portCallbacks[desc.ID] = desc.Callbacks
	m.mu.Unlock()
	return bg.ConnectPort(desc)
}

// DeliverPortMessage sends a message from an external view to the background port.
func (m *Manager) DeliverPortMessage(extID, portID string, message interface{}) error {
	bg := m.GetBackgroundContext(extID)
	if bg == nil {
		return fmt.Errorf("background not running for %s", extID)
	}
	return bg.DeliverPortMessage(portID, message)
}

// DisconnectPort tears down a port and clears callbacks.
func (m *Manager) DisconnectPort(extID, portID string) {
	bg := m.GetBackgroundContext(extID)
	if bg != nil {
		_ = bg.DisconnectPort(portID)
	}
	m.mu.Lock()
	delete(m.portCallbacks, portID)
	m.mu.Unlock()
}

// NotifyPortMessage is invoked by background ports to reach external views.
func (m *Manager) NotifyPortMessage(portID string, message interface{}) {
	m.mu.RLock()
	cb := m.portCallbacks[portID]
	m.mu.RUnlock()
	if cb.OnMessage != nil {
		cb.OnMessage(message)
	}
}

// NotifyPortDisconnect is invoked by background ports to inform external views.
func (m *Manager) NotifyPortDisconnect(portID string) {
	m.mu.RLock()
	cb := m.portCallbacks[portID]
	m.mu.RUnlock()
	if cb.OnDisconnect != nil {
		cb.OnDisconnect()
	}
}

// DispatchWebRequestEvent forwards a webRequest event to the background context.
func (m *Manager) DispatchWebRequestEvent(extID string, event string, payload interface{}) (*api.BlockingResponse, error) {
	bg := m.GetBackgroundContext(extID)
	if bg == nil {
		return nil, fmt.Errorf("background not running for %s", extID)
	}
	return bg.DispatchWebRequestEvent(event, payload)
}

// connectBackgroundToActiveView creates a port from background -> active view.
func (m *Manager) connectBackgroundToActiveView(ext *Extension, name string) (string, error) {
	if ext == nil {
		return "", fmt.Errorf("missing extension")
	}

	m.mu.RLock()
	viewLookup := m.viewLookup
	getActive := m.getActivePane
	m.mu.RUnlock()
	if viewLookup == nil || getActive == nil {
		return "", fmt.Errorf("no view lookup available")
	}

	active := getActive()
	if active == nil {
		return "", fmt.Errorf("no active view")
	}

	view := viewLookup.GetViewByID(active.ID)
	if view == nil {
		return "", fmt.Errorf("active view not found")
	}

	portID := fmt.Sprintf("port-%d", time.Now().UnixNano())
	sender := api.MessageSender{
		ID:  ext.ID,
		URL: active.URL,
		Tab: &api.Tab{
			ID:       int(active.ID),
			Index:    active.Index,
			WindowID: int(active.WindowID),
			URL:      active.URL,
			Title:    active.Title,
			Active:   active.Active,
		},
	}

	callbacks := api.PortCallbacks{
		OnMessage: func(msg interface{}) {
			_ = m.injectPortEvent(view.ID(), map[string]interface{}{
				"type":    "port-message",
				"portId":  portID,
				"message": msg,
			})
		},
		OnDisconnect: func() {
			_ = m.injectPortEvent(view.ID(), map[string]interface{}{
				"type":   "port-disconnect",
				"portId": portID,
			})
			m.DisconnectPort(ext.ID, portID)
		},
	}

	desc := api.PortDescriptor{
		ID:        portID,
		Name:      name,
		Sender:    sender,
		Callbacks: callbacks,
	}

	m.mu.Lock()
	m.portCallbacks[portID] = callbacks
	m.mu.Unlock()

	if err := ext.Background.ConnectPort(desc); err != nil {
		return "", err
	}

	if err := m.injectPortEvent(view.ID(), map[string]interface{}{
		"type":   "port-connect",
		"portId": portID,
		"name":   name,
		"sender": sender,
	}); err != nil {
		return "", err
	}

	return portID, nil
}

func (m *Manager) injectPortEvent(viewID uint64, event map[string]interface{}) error {
	m.mu.RLock()
	viewLookup := m.viewLookup
	m.mu.RUnlock()
	if viewLookup == nil {
		return fmt.Errorf("no view lookup available")
	}
	view := viewLookup.GetViewByID(viewID)
	if view == nil {
		return fmt.Errorf("view %d not found", viewID)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`try { window.__dumberWebExtReceive && window.__dumberWebExtReceive(%s); } catch (e) { console.error(e); }`, string(data))
	return view.InjectScript(script)
}

// SetPaneCallbacks sets the callbacks for getting pane information from the workspace
func (m *Manager) SetPaneCallbacks(getAllPanes GetAllPanesFunc, getActivePane GetActivePaneFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getAllPanes = getAllPanes
	m.getActivePane = getActivePane
}

// GetAllPanes returns all panes from the workspace (if callback is set)
func (m *Manager) GetAllPanes() []api.PaneInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getAllPanes != nil {
		// Convert internal PaneInfo to api.PaneInfo
		internalPanes := m.getAllPanes()
		apiPanes := make([]api.PaneInfo, len(internalPanes))
		for i, p := range internalPanes {
			apiPanes[i] = api.PaneInfo{
				ID:       p.ID,
				Index:    p.Index,
				Active:   p.Active,
				URL:      p.URL,
				Title:    p.Title,
				WindowID: p.WindowID,
			}
		}
		return apiPanes
	}
	return nil
}

// GetActivePane returns the active pane from the workspace (if callback is set)
func (m *Manager) GetActivePane() *api.PaneInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.getActivePane != nil {
		// Convert internal PaneInfo to api.PaneInfo
		p := m.getActivePane()
		if p == nil {
			return nil
		}
		return &api.PaneInfo{
			ID:       p.ID,
			Index:    p.Index,
			Active:   p.Active,
			URL:      p.URL,
			Title:    p.Title,
			WindowID: p.WindowID,
		}
	}
	return nil
}

// managerPaneProvider wraps manager callbacks as PaneProvider
type managerPaneProvider struct {
	manager *Manager
}

func (p *managerPaneProvider) GetAllPanes() []api.PaneInfo {
	return p.manager.GetAllPanes()
}

func (p *managerPaneProvider) GetActivePane() *api.PaneInfo {
	return p.manager.GetActivePane()
}

// StartBackgroundContext initializes and starts the background context for an extension
func (m *Manager) StartBackgroundContext(ext *Extension) error {
	if ext == nil || ext.Manifest == nil || ext.Manifest.Background == nil {
		log.Printf("[webext] Extension %s has no background scripts", ext.ID)
		return nil
	}

	if ext.Background != nil {
		return nil
	}

	ctx := NewBackgroundContext(ext)

	// Set pane provider for tabs API
	ctx.SetPaneProvider(&managerPaneProvider{manager: m})

	if err := ctx.Start(); err != nil {
		return fmt.Errorf("failed to start background for %s: %w", ext.ID, err)
	}
	ctx.SetPortConnector(func(name string) (string, error) {
		return m.connectBackgroundToActiveView(ext, name)
	})
	ext.Background = ctx
	log.Printf("[webext] Started Sobek background for %s", ext.ID)
	return nil
}

// StopBackgroundContext stops the background context for an extension
func (m *Manager) StopBackgroundContext(ext *Extension) {
	if ext == nil {
		return
	}
	if ext.Background != nil {
		ext.Background.Stop()
		ext.Background = nil
	}
}

func (m *Manager) getExtensionByID(id string) (schemes.Extension, bool) {
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

func (m *Manager) isExtensionEnabled(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled[id]
}

// HandleSchemeRequest delegates dumb-extension:// handling to the scheme handler.
func (m *Manager) HandleSchemeRequest(req *webkit.URISchemeRequest) {
	m.schemeHandler.Handle(req)
}

// buildHostPermissions creates CORS allowlist patterns for an extension.
// Always includes the extension's own dumb-extension:// URL to enable ES6 module loading.
// Optionally includes URL patterns from manifest permissions for web access.
func buildHostPermissions(ext *Extension) []string {
	permissions := make([]string, 0, 16)
	seen := make(map[string]struct{})
	add := func(pattern string) {
		if pattern == "" {
			return
		}
		if _, ok := seen[pattern]; ok {
			return
		}
		seen[pattern] = struct{}{}
		permissions = append(permissions, pattern)
	}

	// CRITICAL: Always allow extension's own resources for ES6 module imports.
	add(fmt.Sprintf("%s://%s/*", schemes.SchemeDumbExtension, ext.ID))

	// Prefer manifest.host_permissions when present (MV3), otherwise fall back to permissions array.
	var hostPerms []string
	if ext.Manifest != nil && len(ext.Manifest.HostPermissions) > 0 {
		hostPerms = ext.Manifest.HostPermissions
	} else if ext.Manifest != nil {
		hostPerms = ext.Manifest.Permissions
	}

	for _, perm := range hostPerms {
		if perm == "" {
			continue
		}

		// Handle <all_urls> special case with both wildcard and explicit schemes.
		if perm == "<all_urls>" {
			add("*://*/*")
			add("http://*/*")
			add("https://*/*")
			continue
		}

		// Check if this is a host permission (contains ://) or wildcard scheme.
		if strings.Contains(perm, "://") || strings.HasPrefix(perm, "*://") {
			if isSupportedScheme(perm) {
				add(perm)
			}
			continue
		}

		// Regular API permissions (storage, tabs, etc.) - ignore for CORS.
	}

	return permissions
}

// isSupportedScheme checks if a permission URL uses a supported scheme.
// Returns true for web schemes and our custom dumb-extension:// scheme.
func isSupportedScheme(permission string) bool {
	// Handle *:// wildcard
	if strings.HasPrefix(permission, "*://") {
		return true
	}

	// Parse URL to get scheme
	u, err := url.Parse(permission)
	if err != nil {
		return false
	}

	// Allow common web schemes plus our custom scheme
	switch u.Scheme {
	case "http", "https", "ws", "wss", "data", "file", schemes.SchemeDumbExtension:
		return true
	default:
		return false
	}
}
