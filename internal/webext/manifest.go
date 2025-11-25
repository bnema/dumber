package webext

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/webext/shared"
	"github.com/bnema/dumber/internal/webext/validation"
)

// Manifest represents a WebExtension manifest (MV2 format)
type Manifest struct {
	ManifestVersion         int                      `json:"manifest_version"`
	Name                    string                   `json:"name"`
	Version                 string                   `json:"version"`
	Description             string                   `json:"description,omitempty"`
	Author                  string                   `json:"author,omitempty"`
	Homepage                string                   `json:"homepage_url,omitempty"`
	DefaultLocale           string                   `json:"default_locale,omitempty"`
	Icons                   map[string]string        `json:"icons,omitempty"`
	Permissions             []string                 `json:"permissions,omitempty"`
	HostPermissions         []string                 `json:"host_permissions,omitempty"` // MV3 only
	ContentSecurityPolicy   string                   `json:"content_security_policy,omitempty"`
	Background              *Background              `json:"background,omitempty"`
	ContentScripts          []shared.ContentScript   `json:"content_scripts,omitempty"`
	WebAccessible           []string                 `json:"web_accessible_resources,omitempty"`
	BrowserAction           *BrowserAction           `json:"browser_action,omitempty"`
	Options                 *OptionsPage             `json:"options_ui,omitempty"`
	Storage                 map[string]interface{}   `json:"storage,omitempty"`
	BrowserSpecificSettings *BrowserSpecificSettings `json:"browser_specific_settings,omitempty"`
	Commands                map[string]interface{}   `json:"commands,omitempty"`
}

// BrowserSpecificSettings contains browser-specific configuration
type BrowserSpecificSettings struct {
	Gecko        *GeckoSettings `json:"gecko,omitempty"`
	GeckoAndroid *GeckoSettings `json:"gecko_android,omitempty"`
}

// Background defines background scripts/page for the extension
type Background struct {
	Scripts    []string `json:"scripts,omitempty"`
	Page       string   `json:"page,omitempty"`
	Persistent bool     `json:"persistent"`
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

// GeckoSettings contains Firefox-specific settings
type GeckoSettings struct {
	ID               string `json:"id,omitempty"`
	StrictMinVersion string `json:"strict_min_version,omitempty"`
	StrictMaxVersion string `json:"strict_max_version,omitempty"`
	UpdateURL        string `json:"update_url,omitempty"`
}

// RunAtTiming represents when content scripts should be injected
type RunAtTiming int

const (
	RunAtDocumentStart RunAtTiming = iota
	RunAtDocumentEnd
	RunAtDocumentIdle
)

// DefaultContentSecurityPolicy is the CSP used when the extension manifest
// doesn't specify one. Matches Firefox's default per MDN documentation.
// See: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Content_Security_Policy#default_content_security_policy
const DefaultContentSecurityPolicy = "script-src 'self'; object-src 'self';"

// GetContentSecurityPolicy returns the extension's CSP or the default if not specified.
// This matches Epiphany's behavior of providing a sane default CSP.
func (m *Manifest) GetContentSecurityPolicy() string {
	if m == nil {
		return DefaultContentSecurityPolicy
	}
	if m.ContentSecurityPolicy != "" {
		return m.ContentSecurityPolicy
	}
	return DefaultContentSecurityPolicy
}

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
	extDir := filepath.Dir(path)

	// Run comprehensive validation first
	result, rawManifest, err := validation.ValidateManifestFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read/parse manifest: %w", err)
	}

	// Log any warnings
	for _, w := range result.Warnings {
		log.Printf("[webext] manifest warning in %s: %s", filepath.Base(extDir), w.Error())
	}

	// Fail on errors
	if result.HasErrors() {
		return nil, result.Error()
	}

	// Parse into our struct (validation already confirmed required fields exist)
	data, err := json.Marshal(rawManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Resolve i18n placeholders (e.g., __MSG_extName__)
	if err := manifest.ResolveI18n(extDir); err != nil {
		// Log but don't fail - some extensions may not have i18n
		// log.Printf("Warning: failed to resolve i18n for %s: %v", path, err)
	}

	// Run additional struct-level validation
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

	// Check if this is a Firefox extension
	if !m.isFirefoxExtension() {
		return fmt.Errorf("not a Firefox extension (missing 'browser_specific_settings.gecko' field). Only Firefox extensions are supported")
	}

	return nil
}

// isFirefoxExtension checks if the extension is explicitly for Firefox
// Firefox extensions must have browser_specific_settings.gecko field
func (m *Manifest) isFirefoxExtension() bool {
	return m.BrowserSpecificSettings != nil && m.BrowserSpecificSettings.Gecko != nil
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

// HasPermission checks if the extension has a specific permission
func (m *Manifest) HasPermission(perm string) bool {
	for _, p := range m.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// I18nMessage represents a single i18n message entry
type I18nMessage struct {
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
}

// ResolveI18n resolves __MSG_*__ placeholders in manifest fields
// Looks for _locales/{locale}/messages.json (Firefox extension i18n format)
func (m *Manifest) ResolveI18n(extDir string) error {
	// Determine which locale to use
	locale := "en"
	if m.DefaultLocale != "" {
		locale = m.DefaultLocale
	}

	// Try to load messages from _locales/{locale}/messages.json
	messages, err := loadI18nMessages(extDir, locale)
	if err != nil {
		// No i18n files found, or error loading - skip resolution
		return err
	}

	// Resolve placeholders in manifest fields
	m.Name = resolveI18nString(m.Name, messages)
	m.Description = resolveI18nString(m.Description, messages)

	// Resolve browser action title if present
	if m.BrowserAction != nil {
		m.BrowserAction.DefaultTitle = resolveI18nString(m.BrowserAction.DefaultTitle, messages)
	}

	return nil
}

// loadI18nMessages loads messages.json for a given locale
func loadI18nMessages(extDir, locale string) (map[string]I18nMessage, error) {
	messagesPath := filepath.Join(extDir, "_locales", locale, "messages.json")

	data, err := os.ReadFile(messagesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read messages.json: %w", err)
	}

	var messages map[string]I18nMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("failed to parse messages.json: %w", err)
	}

	return messages, nil
}

// resolveI18nString replaces __MSG_messageName__ with actual message text
func resolveI18nString(input string, messages map[string]I18nMessage) string {
	if !strings.Contains(input, "__MSG_") {
		return input
	}

	// Extract message name from __MSG_messageName__
	result := input
	for strings.Contains(result, "__MSG_") {
		start := strings.Index(result, "__MSG_")
		if start == -1 {
			break
		}

		// Find the closing "__" after "__MSG_"
		closeStart := start + 6 // Length of "__MSG_"
		closeIdx := strings.Index(result[closeStart:], "__")
		if closeIdx == -1 {
			break
		}

		end := closeStart + closeIdx + 2 // Include the closing "__"
		placeholder := result[start:end]
		messageName := result[closeStart : closeStart+closeIdx]

		// Look up message in the messages map
		if msg, ok := messages[messageName]; ok {
			result = strings.ReplaceAll(result, placeholder, msg.Message)
		} else {
			// If message not found, break to avoid infinite loop
			break
		}
	}

	return result
}
