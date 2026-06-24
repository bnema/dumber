package webkit

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/puregotk/v4/webkit"
)

func TestNeedsExplicitContextMenuSignalOnAcquire(t *testing.T) {
	t.Run("true for prewarmed view that already has other signals", func(t *testing.T) {
		wv := &WebView{signalIDs: []uintptr{1, 2, 3}}
		if !needsExplicitContextMenuSignalOnAcquire(wv, &contextMenuPipeline{}) {
			t.Fatal("expected prewarmed webview to require explicit context-menu signal connection")
		}
	})

	t.Run("false when pipeline missing", func(t *testing.T) {
		wv := &WebView{signalIDs: []uintptr{1}}
		if needsExplicitContextMenuSignalOnAcquire(wv, nil) {
			t.Fatal("expected missing pipeline to skip explicit context-menu signal connection")
		}
	})

	t.Run("false for reused view with disconnected signals", func(t *testing.T) {
		wv := &WebView{}
		if needsExplicitContextMenuSignalOnAcquire(wv, &contextMenuPipeline{}) {
			t.Fatal("expected reused webview to rely on full signal reconnection")
		}
	})
}

func TestShouldReapplySettingsOnAcquire(t *testing.T) {
	settings := NewSettingsManager(context.Background(), entity.EngineSettingsPayload{})

	tests := []struct {
		name     string
		settings *SettingsManager
		wv       *WebView
		want     bool
	}{
		{
			name:     "true for current settings and native webview",
			settings: settings,
			wv:       &WebView{inner: &webkit.WebView{}},
			want:     true,
		},
		{
			name:     "false without settings manager",
			settings: nil,
			wv:       &WebView{inner: &webkit.WebView{}},
		},
		{
			name:     "false without pooled webview",
			settings: settings,
			wv:       nil,
		},
		{
			name:     "false before native webview exists",
			settings: settings,
			wv:       &WebView{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldReapplySettingsOnAcquire(tt.settings, tt.wv); got != tt.want {
				t.Fatalf("shouldReapplySettingsOnAcquire() = %v, want %v", got, tt.want)
			}
		})
	}
}
