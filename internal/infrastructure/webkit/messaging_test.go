package webkit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsTrustedBridgeURI(t *testing.T) {
	t.Parallel()

	trusted := []string{
		"dumb://history",
		"dumb://favorites",
		"dumb://config",
		"dumb://error",
		"dumb://crash",
		"dumb://history/path?cursor=1",
	}
	for _, raw := range trusted {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			require.True(t, isTrustedBridgeURI(raw))
		})
	}

	untrusted := []string{
		"",
		"https://example.com",
		"http://localhost",
		"file:///tmp/history.html",
		"dumb://evil/history",
		"dumb://example.com",
		"javascript:alert(1)",
	}
	for _, raw := range untrusted {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			require.False(t, isTrustedBridgeURI(raw))
		})
	}
}
