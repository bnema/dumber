package webkit

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type messageRouterContextKey struct{}

func TestMessageRouterBaseContextConcurrentUpdate(t *testing.T) {
	t.Parallel()

	router := NewMessageRouter(context.Background())
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range 1000 {
			router.SetBaseContext(context.WithValue(context.Background(), messageRouterContextKey{}, i))
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			if router.baseContext() == nil {
				t.Error("base context must never be nil")
			}
		}
	}()

	wg.Wait()
}

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
