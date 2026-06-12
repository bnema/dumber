package favicon

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCache_SetAfterCloseIsNoOp(t *testing.T) {
	ctx := context.Background()
	cache := NewCache(t.TempDir())
	cache.Close()

	require.NotPanics(t, func() {
		cache.Set(ctx, "example.com", []byte("favicon"))
	})

	_, ok := cache.Get(ctx, "example.com")
	require.False(t, ok)
}
