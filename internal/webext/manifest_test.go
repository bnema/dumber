package webext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/dumber/internal/webext/shared"
)

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid Firefox extension with browser_specific_settings",
			manifest: Manifest{
				ManifestVersion: 2,
				Name:            "Test Extension",
				Version:         "1.0.0",
				BrowserSpecificSettings: &BrowserSpecificSettings{
					Gecko: &GeckoSettings{
						ID:               "@test-extension",
						StrictMinVersion: "58.0",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid manifest version",
			manifest: Manifest{
				ManifestVersion: 3,
				Name:            "Test Extension",
				Version:         "1.0.0",
				BrowserSpecificSettings: &BrowserSpecificSettings{
					Gecko: &GeckoSettings{
						ID: "@test-extension",
					},
				},
			},
			wantErr: true,
			errMsg:  "unsupported manifest_version",
		},
		{
			name: "missing name",
			manifest: Manifest{
				ManifestVersion: 2,
				Version:         "1.0.0",
				BrowserSpecificSettings: &BrowserSpecificSettings{
					Gecko: &GeckoSettings{
						ID: "@test-extension",
					},
				},
			},
			wantErr: true,
			errMsg:  "must have a name",
		},
		{
			name: "missing version",
			manifest: Manifest{
				ManifestVersion: 2,
				Name:            "Test Extension",
				BrowserSpecificSettings: &BrowserSpecificSettings{
					Gecko: &GeckoSettings{
						ID: "@test-extension",
					},
				},
			},
			wantErr: true,
			errMsg:  "must have a version",
		},
		{
			name: "missing browser_specific_settings.gecko",
			manifest: Manifest{
				ManifestVersion: 2,
				Name:            "Chrome Extension",
				Version:         "1.0.0",
			},
			wantErr: true,
			errMsg:  "not a Firefox extension",
		},
		{
			name: "has browser_specific_settings but missing gecko",
			manifest: Manifest{
				ManifestVersion:         2,
				Name:                    "Extension",
				Version:                 "1.0.0",
				BrowserSpecificSettings: &BrowserSpecificSettings{},
			},
			wantErr: true,
			errMsg:  "not a Firefox extension",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validate() error = %q, should contain %q", err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestParseRunAt(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  RunAtTiming
	}{
		{"document_start", "document_start", RunAtDocumentStart},
		{"document_end", "document_end", RunAtDocumentEnd},
		{"document_idle", "document_idle", RunAtDocumentIdle},
		{"empty defaults to idle", "", RunAtDocumentIdle},
		{"unknown defaults to idle", "unknown", RunAtDocumentIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRunAt(tt.input)
			if got != tt.want {
				t.Errorf("ParseRunAt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestManifestGetBackgroundScripts(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		baseDir  string
		want     []string
	}{
		{
			name: "single background script",
			manifest: Manifest{
				Background: &Background{
					Scripts: []string{"background.js"},
				},
			},
			baseDir: "/ext",
			want:    []string{"/ext/background.js"},
		},
		{
			name: "multiple background scripts",
			manifest: Manifest{
				Background: &Background{
					Scripts: []string{"lib.js", "background.js"},
				},
			},
			baseDir: "/ext",
			want:    []string{"/ext/lib.js", "/ext/background.js"},
		},
		{
			name:     "no background",
			manifest: Manifest{},
			baseDir:  "/ext",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.GetBackgroundScripts(tt.baseDir)
			if len(got) != len(tt.want) {
				t.Errorf("GetBackgroundScripts() got %d scripts, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("GetBackgroundScripts()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestContentScriptGetFiles(t *testing.T) {
	tests := []struct {
		name    string
		cs      shared.ContentScript
		baseDir string
		wantJS  []string
		wantCSS []string
	}{
		{
			name: "js and css files",
			cs: shared.ContentScript{
				JS:  []string{"content.js", "lib.js"},
				CSS: []string{"style.css"},
			},
			baseDir: "/ext",
			wantJS:  []string{"/ext/content.js", "/ext/lib.js"},
			wantCSS: []string{"/ext/style.css"},
		},
		{
			name:    "no files",
			cs:      shared.ContentScript{},
			baseDir: "/ext",
			wantJS:  []string{},
			wantCSS: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotJS := tt.cs.GetJSFiles(tt.baseDir)
			if len(gotJS) != len(tt.wantJS) {
				t.Errorf("GetJSFiles() got %d files, want %d", len(gotJS), len(tt.wantJS))
			}
			for i := range gotJS {
				if i < len(tt.wantJS) && gotJS[i] != tt.wantJS[i] {
					t.Errorf("GetJSFiles()[%d] = %v, want %v", i, gotJS[i], tt.wantJS[i])
				}
			}

			gotCSS := tt.cs.GetCSSFiles(tt.baseDir)
			if len(gotCSS) != len(tt.wantCSS) {
				t.Errorf("GetCSSFiles() got %d files, want %d", len(gotCSS), len(tt.wantCSS))
			}
			for i := range gotCSS {
				if i < len(tt.wantCSS) && gotCSS[i] != tt.wantCSS[i] {
					t.Errorf("GetCSSFiles()[%d] = %v, want %v", i, gotCSS[i], tt.wantCSS[i])
				}
			}
		})
	}
}

func TestLoadManifest(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		manifestStr string
		wantErr     bool
	}{
		{
			name: "valid manifest",
			manifestStr: `{
				"manifest_version": 2,
				"name": "Test Extension",
				"version": "1.0.0",
				"description": "A test extension",
				"browser_specific_settings": {
					"gecko": {
						"id": "@test-extension"
					}
				}
			}`,
			wantErr: false,
		},
		{
			name: "invalid json",
			manifestStr: `{
				"manifest_version": 2,
				"name": "Test",
			}`,
			wantErr: true,
		},
		{
			name: "invalid manifest version",
			manifestStr: `{
				"manifest_version": 3,
				"name": "Test Extension",
				"version": "1.0.0",
				"browser_specific_settings": {
					"gecko": {
						"id": "@test-extension"
					}
				}
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create manifest file
			manifestPath := filepath.Join(tmpDir, tt.name+".json")
			err := os.WriteFile(manifestPath, []byte(tt.manifestStr), 0644)
			if err != nil {
				t.Fatalf("Failed to create test manifest: %v", err)
			}

			manifest, err := LoadManifest(manifestPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadManifest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && manifest == nil {
				t.Error("LoadManifest() returned nil manifest")
			}
		})
	}
}

func TestManifestHasPermission(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		perm     string
		want     bool
	}{
		{
			name: "has permission",
			manifest: Manifest{
				Permissions: []string{"tabs", "storage", "webRequest"},
			},
			perm: "storage",
			want: true,
		},
		{
			name: "missing permission",
			manifest: Manifest{
				Permissions: []string{"tabs", "storage"},
			},
			perm: "webRequest",
			want: false,
		},
		{
			name:     "no permissions",
			manifest: Manifest{},
			perm:     "tabs",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.HasPermission(tt.perm)
			if got != tt.want {
				t.Errorf("HasPermission() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveI18nString(t *testing.T) {
	messages := map[string]I18nMessage{
		"extName": {
			Message:     "Bitwarden",
			Description: "Extension name",
		},
		"appDesc": {
			Message:     "A secure password manager",
			Description: "Extension description",
		},
		"buttonLabel": {
			Message: "Click here",
		},
	}

	tests := []struct {
		name     string
		input    string
		messages map[string]I18nMessage
		want     string
	}{
		{
			name:     "simple placeholder",
			input:    "__MSG_extName__",
			messages: messages,
			want:     "Bitwarden",
		},
		{
			name:     "placeholder in sentence",
			input:    "Welcome to __MSG_extName__!",
			messages: messages,
			want:     "Welcome to Bitwarden!",
		},
		{
			name:     "multiple placeholders",
			input:    "__MSG_extName__: __MSG_appDesc__",
			messages: messages,
			want:     "Bitwarden: A secure password manager",
		},
		{
			name:     "no placeholder",
			input:    "Plain text",
			messages: messages,
			want:     "Plain text",
		},
		{
			name:     "unknown placeholder",
			input:    "__MSG_unknown__",
			messages: messages,
			want:     "__MSG_unknown__",
		},
		{
			name:     "empty input",
			input:    "",
			messages: messages,
			want:     "",
		},
		{
			name:     "malformed placeholder (no closing)",
			input:    "__MSG_extName",
			messages: messages,
			want:     "__MSG_extName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveI18nString(tt.input, tt.messages)
			if got != tt.want {
				t.Errorf("resolveI18nString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadI18nMessages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a valid messages.json
	localesDir := filepath.Join(tmpDir, "_locales", "en")
	err := os.MkdirAll(localesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create locales dir: %v", err)
	}

	messagesJSON := `{
		"extName": {
			"message": "Bitwarden",
			"description": "Extension name"
		},
		"appDesc": {
			"message": "A secure password manager"
		}
	}`

	messagesPath := filepath.Join(localesDir, "messages.json")
	err = os.WriteFile(messagesPath, []byte(messagesJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write messages.json: %v", err)
	}

	tests := []struct {
		name    string
		extDir  string
		locale  string
		wantErr bool
		wantLen int
	}{
		{
			name:    "valid messages.json",
			extDir:  tmpDir,
			locale:  "en",
			wantErr: false,
			wantLen: 2,
		},
		{
			name:    "missing locale",
			extDir:  tmpDir,
			locale:  "de",
			wantErr: true,
			wantLen: 0,
		},
		{
			name:    "missing _locales directory",
			extDir:  "/nonexistent",
			locale:  "en",
			wantErr: true,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := loadI18nMessages(tt.extDir, tt.locale)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadI18nMessages() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(messages) != tt.wantLen {
				t.Errorf("loadI18nMessages() got %d messages, want %d", len(messages), tt.wantLen)
			}
		})
	}
}

func TestManifestResolveI18n(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _locales/en/messages.json
	localesDir := filepath.Join(tmpDir, "_locales", "en")
	err := os.MkdirAll(localesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create locales dir: %v", err)
	}

	messagesJSON := `{
		"extName": {
			"message": "Bitwarden",
			"description": "Extension name"
		},
		"appDesc": {
			"message": "A secure password manager"
		},
		"buttonTitle": {
			"message": "Open Vault"
		}
	}`

	messagesPath := filepath.Join(localesDir, "messages.json")
	err = os.WriteFile(messagesPath, []byte(messagesJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write messages.json: %v", err)
	}

	tests := []struct {
		name         string
		manifest     Manifest
		extDir       string
		wantName     string
		wantDesc     string
		wantBtnTitle string
	}{
		{
			name: "resolve all i18n fields",
			manifest: Manifest{
				Name:        "__MSG_extName__",
				Description: "__MSG_appDesc__",
				BrowserAction: &BrowserAction{
					DefaultTitle: "__MSG_buttonTitle__",
				},
			},
			extDir:       tmpDir,
			wantName:     "Bitwarden",
			wantDesc:     "A secure password manager",
			wantBtnTitle: "Open Vault",
		},
		{
			name: "no i18n placeholders",
			manifest: Manifest{
				Name:        "Direct Name",
				Description: "Direct Description",
			},
			extDir:   tmpDir,
			wantName: "Direct Name",
			wantDesc: "Direct Description",
		},
		{
			name: "mixed i18n and direct",
			manifest: Manifest{
				Name:        "__MSG_extName__",
				Description: "Direct description",
			},
			extDir:   tmpDir,
			wantName: "Bitwarden",
			wantDesc: "Direct description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.ResolveI18n(tt.extDir)
			// Don't fail on error - some tests may not have _locales
			_ = err

			if tt.manifest.Name != tt.wantName {
				t.Errorf("After ResolveI18n() Name = %q, want %q", tt.manifest.Name, tt.wantName)
			}
			if tt.manifest.Description != tt.wantDesc {
				t.Errorf("After ResolveI18n() Description = %q, want %q", tt.manifest.Description, tt.wantDesc)
			}
			if tt.manifest.BrowserAction != nil && tt.wantBtnTitle != "" {
				if tt.manifest.BrowserAction.DefaultTitle != tt.wantBtnTitle {
					t.Errorf("After ResolveI18n() BrowserAction.DefaultTitle = %q, want %q",
						tt.manifest.BrowserAction.DefaultTitle, tt.wantBtnTitle)
				}
			}
		})
	}
}

func TestIsFirefoxExtension(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		want     bool
	}{
		{
			name: "valid Firefox extension with gecko settings",
			manifest: Manifest{
				BrowserSpecificSettings: &BrowserSpecificSettings{
					Gecko: &GeckoSettings{
						ID:               "{446900e4-71c2-419f-a6a7-df9c091e268b}",
						StrictMinVersion: "91.0",
					},
				},
			},
			want: true,
		},
		{
			name: "Firefox extension with minimal gecko settings",
			manifest: Manifest{
				BrowserSpecificSettings: &BrowserSpecificSettings{
					Gecko: &GeckoSettings{},
				},
			},
			want: true,
		},
		{
			name:     "missing browser_specific_settings",
			manifest: Manifest{},
			want:     false,
		},
		{
			name: "has browser_specific_settings but missing gecko",
			manifest: Manifest{
				BrowserSpecificSettings: &BrowserSpecificSettings{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.manifest.isFirefoxExtension()
			if got != tt.want {
				t.Errorf("isFirefoxExtension() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadManifestWithI18n(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _locales/en/messages.json
	localesDir := filepath.Join(tmpDir, "_locales", "en")
	err := os.MkdirAll(localesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create locales dir: %v", err)
	}

	messagesJSON := `{
		"extName": {
			"message": "Bitwarden"
		},
		"appDesc": {
			"message": "A secure password manager"
		}
	}`

	messagesPath := filepath.Join(localesDir, "messages.json")
	err = os.WriteFile(messagesPath, []byte(messagesJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write messages.json: %v", err)
	}

	// Create manifest.json with i18n placeholders
	manifestJSON := `{
		"manifest_version": 2,
		"name": "__MSG_extName__",
		"version": "2025.11.0",
		"description": "__MSG_appDesc__",
		"default_locale": "en",
		"browser_specific_settings": {
			"gecko": {
				"id": "{446900e4-71c2-419f-a6a7-df9c091e268b}",
				"strict_min_version": "91.0"
			}
		}
	}`

	manifestPath := filepath.Join(tmpDir, "manifest.json")
	err = os.WriteFile(manifestPath, []byte(manifestJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write manifest.json: %v", err)
	}

	// Load manifest - should resolve i18n automatically
	manifest, err := LoadManifest(manifestPath)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}

	// Verify i18n was resolved
	if manifest.Name != "Bitwarden" {
		t.Errorf("LoadManifest() Name = %q, want %q", manifest.Name, "Bitwarden")
	}
	if manifest.Description != "A secure password manager" {
		t.Errorf("LoadManifest() Description = %q, want %q",
			manifest.Description, "A secure password manager")
	}
}

func TestLoadManifestRejectsNonFirefoxExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a non-Firefox extension manifest (missing browser_specific_settings.gecko)
	nonFirefoxManifestJSON := `{
		"manifest_version": 2,
		"name": "Chrome Extension",
		"version": "1.0.0",
		"description": "An extension without Firefox-specific fields"
	}`

	manifestPath := filepath.Join(tmpDir, "manifest.json")
	err := os.WriteFile(manifestPath, []byte(nonFirefoxManifestJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write manifest.json: %v", err)
	}

	// Try to load non-Firefox manifest - should fail
	_, err = LoadManifest(manifestPath)
	if err == nil {
		t.Fatal("LoadManifest() should have rejected non-Firefox extension, but got no error")
	}

	if !strings.Contains(err.Error(), "not a Firefox extension") {
		t.Errorf("LoadManifest() error = %q, should mention not a Firefox extension", err.Error())
	}
}
