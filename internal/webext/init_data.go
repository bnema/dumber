package webext

import (
	"encoding/json"
	"fmt"
)

// InitData represents data passed to the WebProcess extension on initialization
// This is serialized to JSON and passed via WebContext.SetWebProcessExtensionsInitializationUserData
type InitData struct {
	Extensions []ExtensionInfo `json:"extensions"`
}

// ExtensionInfo contains minimal extension information for WebProcess
type ExtensionInfo struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Enabled         bool              `json:"enabled"`
	Path            string            `json:"path"`
	ContentScripts  []ContentScript   `json:"content_scripts"`
}

// SerializeInitData creates JSON string of extension data for WebProcess
func (m *Manager) SerializeInitData() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var extensions []ExtensionInfo

	// Add bundled extensions
	for _, ext := range m.bundled {
		if !m.enabled[ext.ID] {
			continue
		}

		extensions = append(extensions, ExtensionInfo{
			ID:             ext.ID,
			Name:           ext.Manifest.Name,
			Version:        ext.Manifest.Version,
			Enabled:        true,
			Path:           ext.Path,
			ContentScripts: ext.Manifest.ContentScripts,
		})
	}

	// Add user extensions
	for _, ext := range m.user {
		if !m.enabled[ext.ID] {
			continue
		}

		extensions = append(extensions, ExtensionInfo{
			ID:             ext.ID,
			Name:           ext.Manifest.Name,
			Version:        ext.Manifest.Version,
			Enabled:        true,
			Path:           ext.Path,
			ContentScripts: ext.Manifest.ContentScripts,
		})
	}

	initData := InitData{
		Extensions: extensions,
	}

	jsonData, err := json.Marshal(initData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal init data: %w", err)
	}

	return string(jsonData), nil
}

// ParseInitData parses initialization data from JSON string
func ParseInitData(jsonStr string) (*InitData, error) {
	var initData InitData
	if err := json.Unmarshal([]byte(jsonStr), &initData); err != nil {
		return nil, fmt.Errorf("failed to parse init data: %w", err)
	}
	return &initData, nil
}
