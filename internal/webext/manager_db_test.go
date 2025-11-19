package webext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/db"
	mock_db "github.com/bnema/dumber/internal/db/mocks"
	"go.uber.org/mock/gomock"
)

func writeManifest(t *testing.T, dir, name, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	manifest := Manifest{
		ManifestVersion: 2,
		Name:            name,
		Version:         version,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func TestLoadExtensionsFromDB(t *testing.T) {
	tests := []struct {
		name           string
		dbExtensions   []db.ListInstalledExtensionsRow
		wantBundled    map[string]bool // extensionID -> should exist
		wantUser       map[string]bool
		wantEnabled    map[string]bool
		wantErr        bool
	}{
		{
			name: "loads_bundled_and_user_extensions",
			dbExtensions: []db.ListInstalledExtensionsRow{
				{
					ExtensionID: "bundled-ext",
					Name:        "Bundled",
					Version:     "1.0.0",
					InstallPath: "", // will be set in test
					Bundled:     true,
					Enabled:     true,
				},
				{
					ExtensionID: "user-ext",
					Name:        "User",
					Version:     "1.1.0",
					InstallPath: "", // will be set in test
					Bundled:     false,
					Enabled:     false,
				},
			},
			wantBundled: map[string]bool{"bundled-ext": true},
			wantUser:    map[string]bool{"user-ext": true},
			wantEnabled: map[string]bool{"bundled-ext": true, "user-ext": false},
			wantErr:     false,
		},
		{
			name:         "empty_db_returns_no_extensions",
			dbExtensions: []db.ListInstalledExtensionsRow{},
			wantBundled:  map[string]bool{},
			wantUser:     map[string]bool{},
			wantEnabled:  map[string]bool{},
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockQueries := mock_db.NewMockExtensionsQuerier(ctrl)
			baseDir := t.TempDir()

			// Create manifests and set paths
			for i := range tt.dbExtensions {
				ext := &tt.dbExtensions[i]
				extPath := filepath.Join(baseDir, ext.ExtensionID)
				writeManifest(t, extPath, ext.Name, ext.Version)
				ext.InstallPath = extPath
			}

			mockQueries.EXPECT().ListInstalledExtensions(gomock.Any()).Return(tt.dbExtensions, nil)

			manager := NewManager(filepath.Join(baseDir, "data"), nil, mockQueries)

			err := manager.LoadExtensionsFromDB()
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadExtensionsFromDB() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			manager.mu.RLock()
			defer manager.mu.RUnlock()

			// Check bundled extensions
			for extID, shouldExist := range tt.wantBundled {
				_, exists := manager.bundled[extID]
				if exists != shouldExist {
					t.Errorf("bundled[%q] exists=%v, want %v", extID, exists, shouldExist)
				}
			}

			// Check user extensions
			for extID, shouldExist := range tt.wantUser {
				_, exists := manager.user[extID]
				if exists != shouldExist {
					t.Errorf("user[%q] exists=%v, want %v", extID, exists, shouldExist)
				}
			}

			// Check enabled state
			for extID, shouldBeEnabled := range tt.wantEnabled {
				enabled := manager.enabled[extID]
				if enabled != shouldBeEnabled {
					t.Errorf("enabled[%q] = %v, want %v", extID, enabled, shouldBeEnabled)
				}
			}
		})
	}
}

func TestEnableDisable(t *testing.T) {
	tests := []struct {
		name           string
		extensionID    string
		initialEnabled bool
		operations     []struct {
			op      string // "enable" or "disable"
			wantErr bool
		}
		wantFinalEnabled bool
	}{
		{
			name:           "enable_then_disable",
			extensionID:    "ext",
			initialEnabled: false,
			operations: []struct {
				op      string
				wantErr bool
			}{
				{op: "enable", wantErr: false},
				{op: "disable", wantErr: false},
			},
			wantFinalEnabled: false,
		},
		{
			name:           "disable_already_disabled",
			extensionID:    "ext",
			initialEnabled: false,
			operations: []struct {
				op      string
				wantErr bool
			}{
				{op: "disable", wantErr: false},
			},
			wantFinalEnabled: false,
		},
		{
			name:           "enable_already_enabled",
			extensionID:    "ext",
			initialEnabled: true,
			operations: []struct {
				op      string
				wantErr bool
			}{
				{op: "enable", wantErr: false},
			},
			wantFinalEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockQueries := mock_db.NewMockExtensionsQuerier(ctrl)
			manager := NewManager(t.TempDir(), nil, mockQueries)

			// Seed with a user extension
			manager.user[tt.extensionID] = &Extension{
				ID:       tt.extensionID,
				Manifest: &Manifest{Name: "Ext", Version: "1.0.0"},
			}
			manager.enabled[tt.extensionID] = tt.initialEnabled

			// Set up mock expectations for all operations
			for _, op := range tt.operations {
				expectedState := op.op == "enable"
				mockQueries.EXPECT().SetExtensionEnabled(gomock.Any(), expectedState, tt.extensionID).Return(nil)
			}

			// Execute operations
			for _, op := range tt.operations {
				var err error
				if op.op == "enable" {
					err = manager.Enable(tt.extensionID)
				} else {
					err = manager.Disable(tt.extensionID)
				}

				if (err != nil) != op.wantErr {
					t.Errorf("%s error = %v, wantErr %v", op.op, err, op.wantErr)
				}
			}

			// Verify final state
			if manager.enabled[tt.extensionID] != tt.wantFinalEnabled {
				t.Errorf("final enabled state = %v, want %v", manager.enabled[tt.extensionID], tt.wantFinalEnabled)
			}
		})
	}
}

func TestEnsureUBlockOrigin(t *testing.T) {
	tests := []struct {
		name         string
		dbExtension  *db.GetInstalledExtensionRow
		needsUpdate  int64 // 0 = up to date, 1 = needs update
		wantLoaded   bool
		wantEnabled  bool
		wantErr      bool
	}{
		{
			name: "loads_from_db_when_up_to_date",
			dbExtension: &db.GetInstalledExtensionRow{
				ExtensionID: uBlockExtensionID,
				Name:        "uBlock",
				Version:     "1.0.0",
				InstallPath: "", // will be set in test
				Bundled:     true,
				Enabled:     true,
			},
			needsUpdate: 0,
			wantLoaded:  true,
			wantEnabled: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			mockQueries := mock_db.NewMockExtensionsQuerier(ctrl)
			baseDir := t.TempDir()

			if tt.dbExtension != nil {
				installPath := filepath.Join(baseDir, "ublock-origin")
				writeManifest(t, installPath, tt.dbExtension.Name, tt.dbExtension.Version)
				tt.dbExtension.InstallPath = installPath

				mockQueries.EXPECT().GetInstalledExtension(gomock.Any(), uBlockExtensionID).Return(*tt.dbExtension, nil)
				mockQueries.EXPECT().CheckExtensionNeedsUpdate(gomock.Any(), gomock.Any(), uBlockExtensionID).Return(db.CheckExtensionNeedsUpdateRow{
					ExtensionID: uBlockExtensionID,
					NeedsUpdate: tt.needsUpdate,
				}, nil)
			}

			manager := NewManager(filepath.Join(baseDir, "data"), nil, mockQueries)

			err := manager.EnsureUBlockOrigin()
			if (err != nil) != tt.wantErr {
				t.Fatalf("EnsureUBlockOrigin() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			manager.mu.RLock()
			_, exists := manager.bundled[uBlockExtensionID]
			enabled := manager.enabled[uBlockExtensionID]
			manager.mu.RUnlock()

			if exists != tt.wantLoaded {
				t.Errorf("extension loaded = %v, want %v", exists, tt.wantLoaded)
			}
			if tt.wantLoaded && enabled != tt.wantEnabled {
				t.Errorf("extension enabled = %v, want %v", enabled, tt.wantEnabled)
			}
		})
	}
}

var _ db.ExtensionsQuerier = (*mock_db.MockExtensionsQuerier)(nil)
