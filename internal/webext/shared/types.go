package shared

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// ContentScript defines scripts to inject into web pages.
type ContentScript struct {
	Matches      []string `json:"matches"`
	ExcludeMatch []string `json:"exclude_matches,omitempty"`
	JS           []string `json:"js,omitempty"`
	CSS          []string `json:"css,omitempty"`
	RunAt        string   `json:"run_at,omitempty"` // document_start, document_end, document_idle
	AllFrames    bool     `json:"all_frames,omitempty"`
	MatchOrigin  bool     `json:"match_origin_as_fallback,omitempty"`
}

// GetJSFiles returns all content script JS file paths rooted at baseDir.
func (m *ContentScript) GetJSFiles(baseDir string) []string {
	var files []string
	for _, js := range m.JS {
		files = append(files, filepath.Join(baseDir, strings.TrimPrefix(js, "/")))
	}
	return files
}

// GetCSSFiles returns all content script CSS file paths rooted at baseDir.
func (m *ContentScript) GetCSSFiles(baseDir string) []string {
	var files []string
	for _, css := range m.CSS {
		files = append(files, filepath.Join(baseDir, strings.TrimPrefix(css, "/")))
	}
	return files
}

// ExtensionInfo contains minimal extension information for WebProcess.
type ExtensionInfo struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	Enabled        bool            `json:"enabled"`
	Path           string          `json:"path"`
	ContentScripts []ContentScript `json:"content_scripts"`
	ManifestJSON   string          `json:"manifest_json,omitempty"` // Full manifest as JSON string
	Translations   string          `json:"translations,omitempty"`  // i18n messages as JSON string
	UILanguage     string          `json:"ui_language,omitempty"`   // Resolved UI language (e.g., "en")
}

// InitData represents data passed to the WebProcess extension on initialization.
type InitData struct {
	Extensions             []ExtensionInfo `json:"extensions"`
	HasWebRequestListeners bool            `json:"has_webrequest_listeners,omitempty"`
}

// ParseInitData parses initialization data from JSON string.
func ParseInitData(jsonStr string) (*InitData, error) {
	var initData InitData
	if err := json.Unmarshal([]byte(jsonStr), &initData); err != nil {
		return nil, err
	}
	return &initData, nil
}
