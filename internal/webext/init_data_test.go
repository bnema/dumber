package webext

import "testing"

func TestParseInitData(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		wantLen int
		wantErr bool
	}{
		{
			name: "valid single extension",
			jsonStr: `{
				"extensions": [{
					"id": "ext1",
					"name": "Test Extension",
					"version": "1.0.0",
					"enabled": true,
					"path": "/path/to/ext1",
					"content_scripts": [{
						"matches": ["<all_urls>"],
						"js": ["content.js"],
						"run_at": "document_end"
					}]
				}]
			}`,
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "multiple extensions",
			jsonStr: `{
				"extensions": [
					{"id": "ext1", "name": "Ext1", "version": "1.0.0", "enabled": true, "path": "/path1", "content_scripts": []},
					{"id": "ext2", "name": "Ext2", "version": "2.0.0", "enabled": true, "path": "/path2", "content_scripts": []}
				]
			}`,
			wantLen: 2,
			wantErr: false,
		},
		{
			name:    "empty extensions",
			jsonStr: `{"extensions": []}`,
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "invalid json",
			jsonStr: `{invalid json}`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "missing extensions key",
			jsonStr: `{}`,
			wantLen: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInitData(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseInitData() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(got.Extensions) != tt.wantLen {
				t.Errorf("ParseInitData() got %d extensions, want %d", len(got.Extensions), tt.wantLen)
			}
		})
	}
}

func TestSerializeInitData(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Manager)
		wantLen int
		wantIDs []string
	}{
		{
			name: "single enabled extension",
			setup: func(m *Manager) {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.bundled["test-ext"] = &Extension{
					ID:   "test-ext",
					Path: "/path/to/ext",
					Manifest: &Manifest{
						Name:    "Test Extension",
						Version: "1.0.0",
						ContentScripts: []ContentScript{
							{Matches: []string{"<all_urls>"}, JS: []string{"content.js"}},
						},
					},
				}
				m.enabled["test-ext"] = true
			},
			wantLen: 1,
			wantIDs: []string{"test-ext"},
		},
		{
			name: "disabled extension excluded",
			setup: func(m *Manager) {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.bundled["enabled"] = &Extension{
					ID:       "enabled",
					Manifest: &Manifest{Name: "Enabled", Version: "1.0.0"},
				}
				m.bundled["disabled"] = &Extension{
					ID:       "disabled",
					Manifest: &Manifest{Name: "Disabled", Version: "1.0.0"},
				}
				m.enabled["enabled"] = true
				m.enabled["disabled"] = false
			},
			wantLen: 1,
			wantIDs: []string{"enabled"},
		},
		{
			name: "both bundled and user extensions",
			setup: func(m *Manager) {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.bundled["bundled-ext"] = &Extension{
					ID:       "bundled-ext",
					Manifest: &Manifest{Name: "Bundled", Version: "1.0.0"},
				}
				m.user["user-ext"] = &Extension{
					ID:       "user-ext",
					Manifest: &Manifest{Name: "User", Version: "1.0.0"},
				}
				m.enabled["bundled-ext"] = true
				m.enabled["user-ext"] = true
			},
			wantLen: 2,
			wantIDs: []string{"bundled-ext", "user-ext"},
		},
		{
			name:    "no extensions",
			setup:   func(m *Manager) {},
			wantLen: 0,
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewManager("/tmp/test", nil, nil)
			tt.setup(manager)

			jsonStr, err := manager.SerializeInitData()
			if err != nil {
				t.Fatalf("SerializeInitData() error = %v", err)
			}

			// Parse back and verify
			initData, err := ParseInitData(jsonStr)
			if err != nil {
				t.Fatalf("ParseInitData() error = %v", err)
			}

			if len(initData.Extensions) != tt.wantLen {
				t.Errorf("SerializeInitData() got %d extensions, want %d", len(initData.Extensions), tt.wantLen)
			}

			// Check IDs are present
			gotIDs := make(map[string]bool)
			for _, ext := range initData.Extensions {
				gotIDs[ext.ID] = true
			}
			for _, wantID := range tt.wantIDs {
				if !gotIDs[wantID] {
					t.Errorf("SerializeInitData() missing extension ID %q", wantID)
				}
			}
		})
	}
}
