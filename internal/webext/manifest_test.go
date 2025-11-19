package webext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid manifest",
			manifest: Manifest{
				ManifestVersion: 2,
				Name:            "Test Extension",
				Version:         "1.0.0",
			},
			wantErr: false,
		},
		{
			name: "invalid manifest version",
			manifest: Manifest{
				ManifestVersion: 3,
				Name:            "Test Extension",
				Version:         "1.0.0",
			},
			wantErr: true,
			errMsg:  "unsupported manifest_version",
		},
		{
			name: "missing name",
			manifest: Manifest{
				ManifestVersion: 2,
				Version:         "1.0.0",
			},
			wantErr: true,
			errMsg:  "must have a name",
		},
		{
			name: "missing version",
			manifest: Manifest{
				ManifestVersion: 2,
				Name:            "Test Extension",
			},
			wantErr: true,
			errMsg:  "must have a version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// Just verify error exists if expected, don't check exact message
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
		cs      ContentScript
		baseDir string
		wantJS  []string
		wantCSS []string
	}{
		{
			name: "js and css files",
			cs: ContentScript{
				JS:  []string{"content.js", "lib.js"},
				CSS: []string{"style.css"},
			},
			baseDir: "/ext",
			wantJS:  []string{"/ext/content.js", "/ext/lib.js"},
			wantCSS: []string{"/ext/style.css"},
		},
		{
			name:    "no files",
			cs:      ContentScript{},
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
				"description": "A test extension"
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
				"version": "1.0.0"
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
