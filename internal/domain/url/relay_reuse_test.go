package url

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShouldReuseRelayPane(t *testing.T) {
	const u = "https://claude.com/cai/login/app-google-auth?open_in_browser=1&hop_nonce=XYZ"
	opened := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		recorded  string
		requested string
		now       time.Time
		window    time.Duration
		want      bool
	}{
		{"same url within window", u, u, opened.Add(30 * time.Second), RelayReuseWindow, true},
		{"same url at exact window boundary", u, u, opened.Add(RelayReuseWindow), RelayReuseWindow, true},
		{"same url just after window", u, u, opened.Add(RelayReuseWindow + time.Nanosecond), RelayReuseWindow, false},
		{"different nonce", u, "https://claude.com/cai/login/app-google-auth?open_in_browser=1&hop_nonce=ABC", opened.Add(time.Second), RelayReuseWindow, false},
		{"different host", u, "https://example.com/", opened.Add(time.Second), RelayReuseWindow, false},
		{"equivalent after normalization", "example.com/path", "https://example.com/path", opened.Add(time.Second), RelayReuseWindow, true},
		{"surrounding whitespace ignored", u, "  " + u + "\n", opened.Add(time.Second), RelayReuseWindow, true},
		{"empty requested", "", "", opened, RelayReuseWindow, false},
		{"empty recorded", "", u, opened, RelayReuseWindow, false},
		{"recorded in the future", u, u, opened.Add(-time.Second), RelayReuseWindow, false},
		{"zero window", u, u, opened, 0, false},
		{"negative window", u, u, opened, -time.Second, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldReuseRelayPane(tt.recorded, opened, tt.requested, tt.now, tt.window))
		})
	}
}

func TestRelayURLKey_Normalizes(t *testing.T) {
	assert.Equal(t, "https://example.com", RelayURLKey(" example.com "))
	assert.Empty(t, RelayURLKey("   "))
}
