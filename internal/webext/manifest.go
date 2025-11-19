package webext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Manifest represents a WebExtension manifest (MV2 format)
type Manifest struct {
	ManifestVersion int                    `json:"manifest_version"`
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	Description     string                 `json:"description,omitempty"`
	Author          string                 `json:"author,omitempty"`
	Homepage        string                 `json:"homepage_url,omitempty"`
	Icons           map[string]string      `json:"icons,omitempty"`
	Permissions     []string               `json:"permissions,omitempty"`
	Background      *Background            `json:"background,omitempty"`
	ContentScripts  []ContentScript        `json:"content_scripts,omitempty"`
	WebAccessible   []string               `json:"web_accessible_resources,omitempty"`
	BrowserAction   *BrowserAction         `json:"browser_action,omitempty"`
	Options         *OptionsPage           `json:"options_ui,omitempty"`
	Storage         map[string]interface{} `json:"storage,omitempty"`
}

// Background defines background scripts/page for the extension
type Background struct {
	Scripts    []string `json:"scripts,omitempty"`
	Page       string   `json:"page,omitempty"`
	Persistent bool     `json:"persistent"`
}

// ContentScript defines scripts to inject into web pages
type ContentScript struct {
	Matches      []string `json:"matches"`
	ExcludeMatch []string `json:"exclude_matches,omitempty"`
	JS           []string `json:"js,omitempty"`
	CSS          []string `json:"css,omitempty"`
	RunAt        string   `json:"run_at,omitempty"` // document_start, document_end, document_idle
	AllFrames    bool     `json:"all_frames,omitempty"`
	MatchOrigin  bool     `json:"match_origin_as_fallback,omitempty"`
}

// BrowserAction defines the browser action (toolbar button)
type BrowserAction struct {
	DefaultIcon  map[string]string `json:"default_icon,omitempty"`
	DefaultTitle string            `json:"default_title,omitempty"`
	DefaultPopup string            `json:"default_popup,omitempty"`
}

// OptionsPage defines the options/preferences page
type OptionsPage struct {
	Page      string `json:"page"`
	OpenInTab bool   `json:"open_in_tab,omitempty"`
}

// RunAtTiming represents when content scripts should be injected
type RunAtTiming int

const (
	RunAtDocumentStart RunAtTiming = iota
	RunAtDocumentEnd
	RunAtDocumentIdle
)

// ParseRunAt converts string to RunAtTiming
func ParseRunAt(s string) RunAtTiming {
	switch s {
	case "document_start":
		return RunAtDocumentStart
	case "document_end":
		return RunAtDocumentEnd
	case "document_idle":
		return RunAtDocumentIdle
	default:
		return RunAtDocumentIdle // Default
	}
}

// LoadManifest loads and parses a manifest.json file
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	return &manifest, nil
}

// Validate checks if the manifest is valid
func (m *Manifest) Validate() error {
	if m.ManifestVersion != 2 {
		return fmt.Errorf("unsupported manifest_version: %d (only v2 supported)", m.ManifestVersion)
	}

	if m.Name == "" {
		return fmt.Errorf("manifest must have a name")
	}

	if m.Version == "" {
		return fmt.Errorf("manifest must have a version")
	}

	return nil
}

// GetBackgroundScripts returns all background script paths
func (m *Manifest) GetBackgroundScripts(baseDir string) []string {
	if m.Background == nil {
		return nil
	}

	var scripts []string
	for _, script := range m.Background.Scripts {
		scripts = append(scripts, filepath.Join(baseDir, script))
	}

	return scripts
}

// GetContentScriptFiles returns all content script file paths
func (m *ContentScript) GetJSFiles(baseDir string) []string {
	var files []string
	for _, js := range m.JS {
		files = append(files, filepath.Join(baseDir, js))
	}
	return files
}

// GetCSSFiles returns all CSS file paths
func (m *ContentScript) GetCSSFiles(baseDir string) []string {
	var files []string
	for _, css := range m.CSS {
		files = append(files, filepath.Join(baseDir, css))
	}
	return files
}

// HasPermission checks if the extension has a specific permission
func (m *Manifest) HasPermission(perm string) bool {
	for _, p := range m.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}
