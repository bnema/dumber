package bootstrap

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyGTKIMModuleFallback(t *testing.T) {
	tests := []struct {
		name            string
		gtkIMModule     string
		xdgSessionType  string
		waylandDisplay  string
		wantEnvSet      bool
		wantMessageSent bool
	}{
		{
			name:            "GTK_IM_MODULE already set => no override, no message",
			gtkIMModule:     "ibus",
			xdgSessionType:  "wayland",
			waylandDisplay:  "",
			wantEnvSet:      false,
			wantMessageSent: false,
		},
		{
			name:            "non-Wayland with GTK_IM_MODULE unset => no override, no message",
			gtkIMModule:     "",
			xdgSessionType:  "x11",
			waylandDisplay:  "",
			wantEnvSet:      false,
			wantMessageSent: false,
		},
		{
			name:            "Wayland via XDG_SESSION_TYPE with GTK_IM_MODULE unset => override + message",
			gtkIMModule:     "",
			xdgSessionType:  "wayland",
			waylandDisplay:  "",
			wantEnvSet:      true,
			wantMessageSent: true,
		},
		{
			name:            "Wayland via WAYLAND_DISPLAY with GTK_IM_MODULE unset => override + message",
			gtkIMModule:     "",
			xdgSessionType:  "",
			waylandDisplay:  "wayland-0",
			wantEnvSet:      true,
			wantMessageSent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			env := map[string]string{
				"GTK_IM_MODULE":    tt.gtkIMModule,
				"XDG_SESSION_TYPE": tt.xdgSessionType,
				"WAYLAND_DISPLAY":  tt.waylandDisplay,
			}
			getter := func(key string) string { return env[key] }
			var setKey, setValue string
			setter := func(key, val string) error {
				setKey = key
				setValue = val
				return nil
			}

			ApplyGTKIMModuleFallback(&stderr, getter, setter)

			msg := stderr.String()
			if tt.wantMessageSent {
				assert.Contains(t, msg, "GTK_IM_MODULE unset", "expected stderr message")
				assert.Equal(t, "GTK_IM_MODULE", setKey)
				assert.Equal(t, "gtk-im-context-simple", setValue)
			} else {
				assert.Empty(t, msg, "expected no stderr message")
				assert.Empty(t, setKey, "expected no env var set")
			}
		})
	}
}
