package webext

import (
	"encoding/json"
	"fmt"

	"github.com/bnema/dumber/internal/webext/shared"
)

// InitData represents data passed to the WebProcess extension on initialization
// This is serialized to JSON and passed via WebContext.SetWebProcessExtensionsInitializationUserData
type InitData = shared.InitData

// ExtensionInfo contains minimal extension information for WebProcess
type ExtensionInfo = shared.ExtensionInfo

// SerializeInitData creates JSON string of extension data for WebProcess
func (m *Manager) SerializeInitData() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var extensions []ExtensionInfo

	// Helper to serialize extension info
	serializeExtension := func(ext *Extension) (ExtensionInfo, error) {
		// Serialize manifest to JSON
		manifestJSON, err := json.Marshal(ext.Manifest)
		if err != nil {
			return ExtensionInfo{}, fmt.Errorf("failed to marshal manifest for %s: %w", ext.ID, err)
		}

		// Load i18n translations using proper locale resolution
		i18nData, err := LoadTranslationsForExtension(ext)
		if err != nil {
			// Log but don't fail - extension may not have i18n
			// Continue with empty translations
			i18nData = &I18nTranslations{
				Locale:   "en",
				Messages: make(map[string]I18nMessage),
			}
		}

		// Serialize translations to JSON
		translations, err := SerializeTranslations(i18nData)
		if err != nil {
			return ExtensionInfo{}, fmt.Errorf("failed to serialize translations for %s: %w", ext.ID, err)
		}

		return ExtensionInfo{
			ID:             ext.ID,
			Name:           ext.Manifest.Name,
			Version:        ext.Manifest.Version,
			Enabled:        true,
			Path:           ext.Path,
			ContentScripts: ext.Manifest.ContentScripts,
			ManifestJSON:   string(manifestJSON),
			Translations:   translations,
			UILanguage:     i18nData.Locale,
		}, nil
	}

	// Add bundled extensions
	for _, ext := range m.bundled {
		if !m.enabled[ext.ID] {
			continue
		}

		extInfo, err := serializeExtension(ext)
		if err != nil {
			return "", err
		}
		extensions = append(extensions, extInfo)
	}

	// Add user extensions
	for _, ext := range m.user {
		if !m.enabled[ext.ID] {
			continue
		}

		extInfo, err := serializeExtension(ext)
		if err != nil {
			return "", err
		}
		extensions = append(extensions, extInfo)
	}

	initData := InitData{
		Extensions:             extensions,
		HasWebRequestListeners: m.webRequest.HasListeners(),
	}

	jsonData, err := json.Marshal(initData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal init data: %w", err)
	}

	return string(jsonData), nil
}

// ParseInitData parses initialization data from JSON string
func ParseInitData(jsonStr string) (*InitData, error) {
	initData, err := shared.ParseInitData(jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse init data: %w", err)
	}
	return initData, nil
}
